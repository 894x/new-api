# 渠道模型级路由覆盖

## 目标与边界

渠道与模型仍然是多对多关系。渠道上的 `priority`、`weight` 继续作为该渠道全部模型的默认值；本功能只为特定的“渠道 + 精确模型名”增加稀疏覆盖，不改变现有渠道配置的含义。

本功能的目标是：

- 同一渠道中的不同模型可以使用不同优先级和权重；
- 未配置覆盖的模型继续继承渠道默认值，现有数据无需回填；
- 可从渠道视角编辑该渠道的全部模型，也可从模型视角聚合编辑支持该精确模型的全部渠道；
- 数据库选路和内存缓存选路使用相同的有效优先级、有效权重及重试规则。

本功能不做以下事情：

- 不把模型元数据改造成路由配置的权威来源；
- 不创建脱离渠道的“模型全局优先级/权重”；
- 不改变渠道与模型的多对多关系，也不要求一个渠道只能配置一个模型；
- 不对正则或模糊模型规则批量展开覆盖，路由覆盖始终匹配精确模型名。

## 数据模型与继承语义

新增表 `channel_model_overrides`，每行表示一个渠道模型对的稀疏覆盖：

| 字段 | 类型与约束 | 含义 |
| --- | --- | --- |
| `channel_id` | 整数，复合主键 | 渠道 ID |
| `model` | `varchar(255)`，复合主键 | 精确模型名，最长 255 字节 |
| `priority` | 可空 `bigint` | 模型在该渠道下的优先级覆盖 |
| `weight` | 可空无符号整数 | 模型在该渠道下的权重覆盖；接口限制为 `0..2147483637` |

`(channel_id, model)` 是复合主键，因此一个渠道模型对最多有一条覆盖记录。两个覆盖字段分别继承，规则如下：

| `priority_override` / `weight_override` | 语义 |
| --- | --- |
| `null` | 继承对应的渠道默认值 |
| `0` | 显式保存数值 `0`，不是继承 |
| 其他合法整数 | 显式覆盖渠道默认值 |

当两个字段都为 `null` 时，不保留空记录，而是删除该渠道模型对的稀疏行。单个字段为 `null` 时只恢复该字段的继承，另一个字段仍可继续覆盖。

渠道默认 `weight` 和模型级 `weight_override` 使用同一范围限制 `0..2147483637`（`MaxInt32 - 10`）。预留的 `10` 对应现有加权算法为每个候选增加的基础权重，保证单项有效权重可在 32 位与 64 位运行环境安全转换。新增、单个或批量更新渠道、按标签批量编辑，以及模型覆盖 PATCH 都会校验此边界，防止无符号大数进入物化能力或权重求和。优先级使用有符号 64 位整数；权重不接受负数。

读取接口同时返回默认值、覆盖值和有效值：

```json
{
  "channel_id": 12,
  "channel_name": "primary-openai",
  "channel_type": 1,
  "channel_status": 1,
  "model": "gpt-4.1",
  "default_priority": 10,
  "default_weight": 100,
  "priority_override": 30,
  "weight_override": null,
  "effective_priority": 30,
  "effective_weight": 100
}
```

## 选路与一致性

`channel_model_overrides` 是管理员配置层，`abilities` 是运行时物化层。每次新增、修改或清除覆盖时，会在同一数据库事务中重建受影响渠道的 `abilities`：每个分组、模型、渠道组合保存该渠道模型对的有效优先级和有效权重。覆盖写入或能力重建任一步失败，整个修改都会回滚。

数据库直读路径与内存缓存路径采用相同的候选处理顺序：

1. 先查找已启用且分组、模型名精确匹配的能力；没有候选时，再使用模型名规范化结果查找；
2. 对候选执行请求路径过滤。Advanced Custom 渠道必须同时支持实际请求路径和原始请求模型，其他渠道直接通过；
3. 路径过滤完成后，才按有效优先级降序分层。首次请求使用最高优先级，后续重试按不同优先级层依次下探；
4. 同一优先级层内按有效权重随机选择。为兼容现有行为，每个候选参与随机选择时使用 `有效权重 + 10`，所以显式权重 `0` 仍有基础选中概率。

关闭内存缓存时，选路直接读取 `abilities`；开启内存缓存时，缓存也从已启用的 `abilities` 加载渠道 ID、有效优先级和有效权重。两条路径都遵循“精确模型 → 规范化模型 → 请求路径过滤 → 有效优先级分层”，并共用权重选择逻辑，因此覆盖值、优先级重试、路径过滤和权重边界语义一致。覆盖修改成功后会刷新渠道缓存；缓存同步读取数据库失败时不会用不完整快照替换当前缓存。

## 管理 API

四个接口都位于管理员渠道路由组下。GET 需要 `ChannelRead`，PATCH 需要 `ChannelWrite`。

| 方法 | 路径 | 作用 |
| --- | --- | --- |
| GET | `/api/channel/:id/model-routing-overrides` | 列出一个渠道当前配置的全部模型及其默认、覆盖、有效值 |
| PATCH | `/api/channel/:id/model-routing-overrides` | 修改一个渠道中的一个或多个模型覆盖 |
| GET | `/api/channel/model-routing-overrides?model=<精确模型名>` | 聚合支持该精确模型的全部渠道 |
| PATCH | `/api/channel/model-routing-overrides?model=<精确模型名>` | 修改该精确模型在一个或多个渠道中的覆盖 |

PATCH 请求体统一为：

```json
{
  "overrides": [
    {
      "channel_id": 12,
      "model": "gpt-4.1",
      "priority_override": 30,
      "weight_override": 0
    },
    {
      "channel_id": 18,
      "model": "gpt-4.1",
      "priority_override": null,
      "weight_override": 200
    }
  ]
}
```

接口契约：

- 请求是补丁集合，未列出的渠道模型对保持不变；
- 同一请求中重复的渠道模型对会被拒绝；任一项非法时整批请求不会部分生效；
- 渠道视角 PATCH 以路径中的渠道 ID 为准。条目中的 `channel_id` 可为 `0`，如果填写非零值则必须与路径一致；每个条目必须给出该渠道实际支持的模型；
- 模型视角 PATCH 以查询参数中的精确模型名为准。条目中的 `model` 可留空，如果填写则必须与查询参数一致；每个条目必须给出正数 `channel_id`，且对应渠道必须支持该模型；
- 两个覆盖值均为 `null` 时删除该对的稀疏记录；
- PATCH 成功后返回更新范围的完整列表，并记录 `channel.model_routing_override` 或 `model.channel_routing_override` 管理审计事件。

## 管理界面

有两个入口编辑同一份覆盖数据：

- **渠道视角**：编辑已有渠道时，在模型配置区域查看该渠道的全部模型。每行可分别输入优先级和权重、预览有效值，或重置为渠道默认值。若渠道表单中的模型列表尚未保存，覆盖编辑会暂时禁用；应先保存模型列表，避免针对过期模型集合写入配置。
- **模型视角**：编辑精确名称的模型元数据时，界面聚合展示所有支持该模型的渠道及渠道状态。正则名称规则不显示此编辑器，避免把一条元数据规则误当成多个精确路由模型。

输入框留空表示继承，输入 `0` 表示显式覆盖为零；“重置”会将该行两个字段都恢复为继承。界面只提交发生变化的行。只有 `ChannelRead` 权限时可只读查看；没有 `ChannelRead` 时不发起读取请求；编辑控件要求 `ChannelWrite`。

## 配置生命周期

- **渠道默认值变化**：修改渠道优先级或权重会重建能力。仍为 `null` 的字段随新默认值变化；显式覆盖保持不变。
- **渠道模型变化**：移除模型时会删除该渠道模型对的覆盖并移除相应能力；新增模型没有覆盖，自动继承渠道默认值。
- **上游模型同步**：应用上游模型增删时，模型列表、设置、覆盖清理和能力重建在同一事务中完成。只清理被移除模型对应的覆盖，新模型继承默认值。
- **渠道删除**：单个删除和批量删除都会同时清理该渠道的能力与全部模型覆盖。
- **渠道复制**：复制渠道时，会在同一个锁定事务中读取源渠道完整快照和稀疏覆盖，再创建新渠道、复制覆盖并物化能力，避免渠道字段与覆盖来自不同时间点。
- **FixAbility**：修复渠道能力会根据当前渠道、模型和覆盖重新生成 `abilities`；不会把覆盖清空或改写为渠道默认值。正常编辑流程已负责清理不再受支持的覆盖。
- **标签批量操作**：修改模型、分组、渠道默认优先级或默认权重时会重建能力并保留仍有效的模型覆盖；只改标签时同步能力标签。批量设置标签也会按当前覆盖重建能力。
- **启用状态**：渠道启停会同步能力的启用状态并在启用内存缓存时刷新缓存，不改变覆盖记录。多 Key 渠道的单 Key 变化会重新推导渠道总状态：所有 Key 均不可用时自动禁用，有可用 Key 时仅恢复此前自动禁用的渠道，不覆盖人工禁用；渠道总状态、`abilities.enabled` 和缓存候选随事务提交后保持同步。

## 与模型元数据的关系

模型元数据和路由能力是两个独立控制面。模型元数据的重命名或删除只影响元数据本身，不会重写 `channels.models`、`abilities` 或 `channel_model_overrides`，也不会把旧模型名的覆盖迁移到新模型名。

如需真正重命名路由模型，必须在渠道配置或上游同步流程中显式修改渠道支持的模型集合；移除旧模型时，其对应覆盖会按渠道模型生命周期规则清理。模型界面的聚合入口只为 `name_rule = 0` 的精确模型元数据展示，并使用编辑开始时的原模型名读取路由，避免尚未保存的元数据名称影响选路。

## 迁移与回滚

启动迁移通过 GORM AutoMigrate 创建 `channel_model_overrides`。数据模型、事务和迁移按 SQLite、MySQL 与 PostgreSQL 兼容方式设计，不使用数据库专属 SQL。迁移不需要回填：新表初始为空，所有既有渠道模型对自然继承渠道默认值，现有 `abilities` 的行为保持不变。

本地验证实际运行的是 SQLite：迁移测试确认 GORM 生成的实际表名为 `channel_model_overrides`，复合主键和两个可空覆盖字段符合预期，并验证连续执行 AutoMigrate 的幂等性。本地未配置 MySQL/PostgreSQL DSN，因此本次没有对这两种数据库执行真实实例迁移；上线前仍应在对应数据库的预发布环境执行迁移与 PATCH 回归。

首次启用后建议执行下列检查：

1. 确认新表创建成功且复合主键为 `(channel_id, model)`；
2. 在测试渠道为单个模型写入覆盖，比较 GET 返回的默认值、覆盖值和有效值；
3. 分别在开启与关闭内存缓存的环境验证相同优先级重试结果；
4. 清除覆盖并确认有效值恢复为渠道默认值。

若需要回滚到不识别该表的旧版本，应先停止管理端覆盖写入，然后在仍运行新版本时清空覆盖并重建能力，避免旧版本继续读取已经物化在 `abilities` 中的模型级值。下列实际表名 `channel_model_overrides` 已由 GORM/SQLite 迁移测试确认：

```sql
DELETE FROM channel_model_overrides;
```

随后通过管理端“修复渠道能力”操作（`POST /api/channel/fix`，需要 `ChannelOperate`）把所有能力恢复为渠道默认值，验证选路后再部署旧版本。旧版本运行稳定后可按各数据库的变更流程选择保留或删除空表；保留空表不会影响旧版本。不要只删除表而跳过能力重建。

## 验证命令

后端重点测试与静态检查：

```bash
/opt/homebrew/bin/go test ./model ./service ./controller ./router -count=1
/opt/homebrew/bin/go vet ./model ./service ./controller ./router
git diff --check
```

上述 Go 重点包测试和 `go vet` 在本地 SQLite 测试环境执行；测试覆盖覆盖值继承与显式零、事务回滚、模型增删清理、渠道复制、FixAbility/标签更新、多 Key 总状态、缓存开关两条选路路径，以及请求路径过滤先于有效优先级分层。MySQL/PostgreSQL 实例因本地未配置 DSN 未实跑。

前端重点测试、格式和受影响文件检查（从 `web/` 目录执行）：

```bash
node node_modules/vitest/vitest.mjs run src/features/channels/lib/__tests__/model-routing-overrides.test.ts
node scripts/format-with-protected-headers.mjs --check
./node_modules/.bin/oxlint -c .oxlintrc.json \
  src/features/channels/api.ts \
  src/features/channels/types.ts \
  src/features/channels/components/model-routing-overrides-editor.tsx \
  src/features/channels/components/drawers/channel-mutate-drawer.tsx \
  src/features/channels/lib/channel-actions.ts \
  src/features/channels/lib/model-routing-overrides.ts \
  src/features/models/components/drawers/model-mutate-drawer.tsx
```

完整前端类型检查可执行：

```bash
./node_modules/.bin/tsgo -b
```

当前本地工作区的依赖安装不完整时，该命令可能因缺少 `yace`、`yace/highlighters/code`、`yace/plugins` 或测试环境依赖 `happy-dom` 而失败。这是本地 `node_modules` 依赖缺失，不应通过修改本功能源码或提交临时声明文件绕过；先按项目锁文件使用 Bun 完整安装依赖，再重新执行 `bun run typecheck` 和 `bun run build`。

本地已运行模型路由覆盖的聚焦 Vitest、受影响文件格式检查和 Oxlint。完整 `tsgo -b` 受上述既有依赖缺失限制，不能据此宣称完整前端类型检查或生产构建已通过；补齐依赖后应重新执行完整检查。

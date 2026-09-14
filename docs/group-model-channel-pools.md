# 分组的模型渠道池

管理员可在系统设置的分组设置中配置“模型渠道池”，通过 `GroupModelChannelGroups` 选项持久化：

```json
{
  "enterprise": {
    "model-a": ["official", "preferred"],
    "model-b": ["official"],
    "model-c": []
  }
}
```

第一层是用户所属分组，第二层是客户端请求的公开模型名称，数组是提供底层渠道池的分组名称。

每次请求选择渠道前，先合并该模型配置的所有渠道池，再按渠道 ID 与 Key 分组的候选渠道取交集。渠道无需以同一个分组名同时出现：用户策略引用分组 A、Key 选择分组 B 时，若渠道 X 同时属于 A、B 且支持该模型，X 可以使用。只属于其中一侧的渠道不能使用。

现有的 Key 分组使用权限、模型限制、渠道状态、接口和参数能力、资产副本限制仍需满足。优先级、权重、动态路由和容量溢出只在交集内选路。Auto 保留 Key 的分组顺序，逐组计算渠道交集；不会将用户策略的渠道池改成 Key 的 Auto 列表。计费仍采用原有 Key 分组或 Auto 最终选中的分组。

- 未配置用户组或模型：保持现有选路行为。
- 显式空数组 `[]`：禁止该用户组调用该模型，不回退到全部渠道。
- `null`、无效结构、重复的渠道分组、`auto` 渠道池名称：保存时拒绝。
- 模型名精确匹配公开模型名称，不使用上游模型映射名或通配符。
- 多个渠道池取并集；策略与 Key、资产约束之间取交集。
- 未知或无可用渠道的渠道池不会扩大权限；交集为空返回无可用渠道。
- 保存后已有 Key 的后续请求和重试会重新读取策略；不需要重新创建 Key。
- 可用模型列表遵循相同的渠道交集。全局模型目录和全局价格配置不是用户渠道授权列表。

配置复用现有 options 存储，无需新增数据库表或迁移。可视化编辑新增规则后默认为空池，选择渠道分组再保存；JSON 模式支持批量配置。

## 本地端到端验证

`tools/group-model-channel-e2e/run.py` 启动真实网关进程，使用独立 SQLite 数据库和本地模拟上游。初始化、管理员登录、用户与 Key 创建、配置保存、渠道缓存、鉴权、选路、上游 HTTP、扣费、退款和日志都走实际代码，不连接生产环境。

先构建前端和网关，再运行测试。以下命令在仓库根目录执行；输出目录必须尚不存在：

```powershell
bun run --cwd web build
go build -o .codex-tmp/group-channel-e2e/new-api.exe .
python tools/group-model-channel-e2e/run.py --binary .codex-tmp/group-channel-e2e/new-api.exe --output .codex-tmp/group-channel-e2e/cache-on
python tools/group-model-channel-e2e/run.py --binary .codex-tmp/group-channel-e2e/new-api.exe --output .codex-tmp/group-channel-e2e/cache-off --memory-cache false
```

每种缓存模式执行 35 项检查，覆盖静态/动态路由、底层渠道交集、空池和无交集拒绝、模型列表、公开模型映射、Auto、SSE、重试、退款、多池并集、删除规则恢复、特殊分组倍率、月度阶梯跨档、亲和缓存、Key 模型限制、管理员指定渠道、容量限制、无效配置和进程重启。`report.json` 保留每项结论和模拟上游收到的请求，`gateway.log` 保留服务日志。

可加 `--hold` 保留测试进程进行浏览器验证。脚本在输出目录创建仅供本地测试的 `ui-session.json`；创建该目录下的 `stop` 文件即可结束。测试结束后删除 `ui-session.json`。浏览器验证应检查新增、重复规则校验、选择/清空渠道池、保存、刷新和移除，并用同一个已创建的 Key 验证实际选路变化；同时核对保存没有更改其他分组计费选项。

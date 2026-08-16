# 渠道化素材库接入说明（首版）

> 状态说明：本文描述本分支首版已经实现的契约和运维边界。上线前仍需在部署环境完成数据库迁移，并使用真实上游凭据进行验收。

## 1. 目标与适用范围

本功能在 New API 内提供账号级素材库，同时将上游素材库凭据收敛到管理员维护的渠道配置中。

对普通用户而言：

- 只需要一个 New API Key，不需要持有或理解火山引擎 AK/SK。
- 看到的是自己账号下唯一的一套逻辑素材组和素材，不需要选择渠道。
- 使用本地素材 URI（例如 `asset://asset-na-...`）提交视频生成请求。

对管理员而言：

- 每个视频渠道独立配置其素材库 Base URL、认证方式、Region 和 Project。
- 新建素材组或素材时，New API 尝试在所有已启用素材库的渠道中创建副本。
- 视频请求只会在拥有全部所需素材副本的渠道集合中继续执行原有选路。

首版目标覆盖火山方舟 `Version=2024-01-01` 的 10 个素材库操作、渠道级配置、多渠道副本、历史回填、视频请求中的本地 Asset ID 重写，以及账号侧聚合管理界面。

首版不提供跨 New API 账号共享素材、从其他火山账号直接导入既有 Asset ID、跨上游的强一致事务，也不保证兼容上游具备火山官方全部语义。

## 2. 核心模型

### 2.1 渠道素材库配置

素材库配置跟随渠道，一条视频渠道最多有一份素材库配置。配置只允许管理员读取和修改。

| 字段 | 含义 |
|---|---|
| `enabled` | 是否让该渠道参与素材创建、回填和带素材的视频选路 |
| `base_url` | 素材库上游地址。火山官方默认使用 `https://ark.cn-beijing.volcengineapi.com` |
| `auth_type` | `aksk` 或 `bearer` |
| `access_key` | `aksk` 模式的 AK |
| `secret_key` | `aksk` 模式的 SK |
| `api_key` | `bearer` 模式的兼容上游 Key |
| `region` | AK/SK 签名 Region，火山官方默认 `cn-beijing` |
| `project_name` | 该渠道创建和访问素材副本时使用的上游 Project，默认 `default` |

管理接口为：

```text
GET    /api/channel/:id/asset-library
PUT    /api/channel/:id/asset-library
DELETE /api/channel/:id/asset-library
POST   /api/channel/:id/asset-library/sync
```

配置示例（官方 AK/SK）：

```json
{
  "enabled": true,
  "base_url": "https://ark.cn-beijing.volcengineapi.com",
  "auth_type": "aksk",
  "access_key": "<VOLC_ACCESS_KEY>",
  "secret_key": "<VOLC_SECRET_KEY>",
  "region": "cn-beijing",
  "project_name": "default"
}
```

配置示例（兼容 Bearer 上游）：

```json
{
  "enabled": true,
  "base_url": "https://asset-upstream.example.com",
  "auth_type": "bearer",
  "api_key": "<UPSTREAM_API_KEY>",
  "region": "cn-beijing",
  "project_name": "default"
}
```

`bearer` 是 New API 对兼容上游的扩展，不是火山官方素材库的认证方式。兼容上游仍需接受 `Action`、`Version=2024-01-01` 和 JSON 请求体；其状态、Project、错误码等行为由上游自行定义。

### 2.2 账号级逻辑资源

New API 为每个账号维护本地逻辑资源：

- 逻辑素材组 ID：`group-na-...`
- 逻辑素材 ID：`asset-na-...`

这些 ID 属于 New API 账号，不等于任何一个渠道的上游 ID。所有查询和修改都会先按当前认证用户校验所有权。

客户端请求中的 `ProjectName` 保留用于兼容官方请求格式及逻辑查询；每个渠道实际访问哪个上游 Project，以该渠道素材库配置中的 `project_name` 为准。因此不同渠道可以使用不同的上游 Project，而用户无需感知。

### 2.3 渠道副本

每个逻辑资源可对应多个渠道副本：

```text
asset-na-abc
  ├─ channel 2  -> asset-2026...-aaa
  ├─ channel 7  -> asset-2026...-bbb
  └─ channel 11 -> 创建失败，记录错误
```

素材组副本保存：逻辑 Group ID、Channel ID、上游 Group ID、同步状态和最近错误。

素材副本保存：逻辑 Asset ID、Channel ID、上游 Asset ID、同步状态、上游原始状态、最近错误和最近推理时间。

本地副本状态的含义是：

- `ready`：上游创建成功且已取得可用于重写的上游 ID。
- `processing`：正在创建或等待同步。
- `failed`：该渠道的创建或同步失败。

`ready` 不等价于火山官方的 `Status=Active`。New API 不把官方状态枚举提升为所有上游都必须实现的全局规则。

## 3. 创建、同步与历史回填

### 3.1 新建逻辑素材组

1. New API 在当前账号下生成 `group-na-...`。
2. 读取所有启用了素材库的渠道。
3. 对每个渠道使用管理员配置的凭据和 Project 调用 `CreateAssetGroup`。
4. 为每个渠道分别保存成功映射或失败原因。
5. 返回逻辑 Group ID；用户不会看到渠道选择器。

### 3.2 新建逻辑素材

1. 校验逻辑素材组属于当前账号。
2. 在本地生成 `asset-na-...`，并保存原始素材 URL、类型和名称。
3. 对每个启用渠道查找对应的素材组副本；缺失时先补建素材组副本。
4. 使用该渠道的上游 Group ID 调用 `CreateAsset`。
5. 保存逻辑 Asset ID 到各渠道上游 Asset ID 的映射及错误信息。

火山官方 `CreateAsset` 是异步操作。取得上游 Asset ID 只表示上游接受了创建请求，不代表素材已经完成预处理。

### 3.3 新增渠道的历史回填

管理员为新渠道启用素材库后，New API 按以下顺序回填历史数据：

1. 按创建时间同步所有账号的逻辑素材组。
2. 在素材组副本成功后，同步组内素材。
3. 已存在的 `(逻辑资源, Channel ID)` 映射跳过或更新，不重复制造逻辑资源。
4. 单项失败只记录到对应副本，不中断其他账号、素材组或素材的同步。

回填期间，已有渠道仍可正常使用。某个素材尚未回填到新渠道时，带该素材的视频请求不会选择新渠道；待映射成功后，新渠道会自动进入候选集合。

首版历史回填依赖创建素材时保存的原始 URL。管理员应要求用户提供长期稳定、上游可访问的 HTTPS URL。若原始 URL 已过期、需要登录或被源站删除，该渠道副本会回填失败。Get/List 返回的临时预览 URL 不能代替长期源 URL。

## 4. 对外 API

### 4.1 认证与调用形式

统一入口：

```text
POST /api/asset-library?Action=<Action>&Version=2024-01-01
Authorization: Bearer <NEW_API_KEY>
Content-Type: application/json
```

外部调用只使用 New API Key。New API 根据 Key 解析账号，绝不把管理员配置的上游 AK、SK 或 Bearer Key返回给用户。

首版仅接受 `Version=2024-01-01` 和下表列出的 Action。请求字段沿用火山官方 PascalCase 命名。返回中的 `Id`、`GroupId` 均为 New API 逻辑 ID；List/Get 结果可增加 `Replication` 字段展示聚合后的副本覆盖情况。

原始火山 HTTP 响应使用 `ResponseMetadata + Result` 外层；本文各操作的“结果”列仅列出 `Result` 的业务字段。

### 4.2 操作矩阵

| Action | 请求体 | Result |
|---|---|---|
| `CreateAssetGroup` | `Name` 必填；`Description?`；`GroupType?`；`ProjectName?` | `{Id}`，返回 `group-na-...` |
| `CreateAsset` | `GroupId`、`URL`、`AssetType` 必填；`Name?`；`ProjectName?` | `{Id}`，返回 `asset-na-...` |
| `ListAssetGroups` | `Filter?`、`PageNumber?`、`PageSize?`、`SortBy?`、`SortOrder?`、`ProjectName?` | `{TotalCount, Items, PageNumber, PageSize}` |
| `ListAssets` | `Filter?`、`PageNumber?`、`PageSize?`、`SortBy?`、`SortOrder?`、`ProjectName?` | `{TotalCount, Items, PageNumber, PageSize}` |
| `GetAssetGroup` | `Id` 必填；`ProjectName?` | 素材组信息及 `Replication?` |
| `GetAsset` | `Id` 必填；`ProjectName?` | 素材信息、临时 `URL?` 及 `Replication?` |
| `UpdateAssetGroup` | `Id` 必填；`Name?`；`Description?`；`ProjectName?` | `{Id}` |
| `UpdateAsset` | `Id` 必填；`Name?`；`ProjectName?` | `{Id}` |
| `DeleteAsset` | `Id` 必填；`ProjectName?` | `{}` |
| `DeleteAssetGroup` | `Id` 必填；`ProjectName?`；组内必须无素材 | `{}` |

`AssetType` 常用值为 `Image`、`Video`、`Audio`。官方火山上游当前对虚拟人像组使用 `GroupType=AIGC`；兼容上游是否支持其他值由其自身决定。

筛选对象字段：

```json
{
  "Filter": {
    "GroupIds": ["group-na-..."],
    "GroupType": "AIGC",
    "Statuses": ["Active", "Processing", "Failed"],
    "Name": "人物名称",
    "AssetType": "Image"
  },
  "PageNumber": 1,
  "PageSize": 20,
  "SortBy": "CreateTime",
  "SortOrder": "Desc",
  "ProjectName": "default"
}
```

其中：

- `ListAssetGroups.Filter` 使用 `GroupIds`、`GroupType`、`Name`。
- `ListAssets.Filter` 使用 `GroupIds`、`GroupType`、`Statuses`、`Name`、`AssetType`。
- `SortBy`：素材组支持 `CreateTime`、`UpdateTime`；素材支持 `CreateTime`、`UpdateTime`、`GroupId`。
- `SortOrder`：`Asc` 或 `Desc`。
- `PageSize` 最大为 100。

素材组结果字段：

```text
Id, Name, Description, GroupType, ProjectName,
CreateTime, UpdateTime, Replication?
```

素材结果字段：

```text
Id, Name, URL?, GroupId, AssetType, Status?, Error?,
ProjectName, CreateTime, UpdateTime, LastInferenceTime?, Replication?
```

`Replication` 是 New API 扩展字段：

```json
{
  "Status": "partial",
  "Ready": 2,
  "Processing": 1,
  "Failed": 1,
  "Total": 4
}
```

兼容客户端应忽略不认识的扩展字段。

### 4.3 API 示例

创建素材组：

```bash
curl -X POST \
  "$NEW_API_BASE/api/asset-library?Action=CreateAssetGroup&Version=2024-01-01" \
  -H "Authorization: Bearer $NEW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "品牌虚拟人物",
    "Description": "同一人物的参考素材",
    "GroupType": "AIGC",
    "ProjectName": "default"
  }'
```

创建素材：

```bash
curl -X POST \
  "$NEW_API_BASE/api/asset-library?Action=CreateAsset&Version=2024-01-01" \
  -H "Authorization: Bearer $NEW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "GroupId": "group-na-xxxxxxxx",
    "URL": "https://cdn.example.com/assets/avatar-front.png",
    "AssetType": "Image",
    "Name": "正面全身图",
    "ProjectName": "default"
  }'
```

查询素材：

```bash
curl -X POST \
  "$NEW_API_BASE/api/asset-library?Action=ListAssets&Version=2024-01-01" \
  -H "Authorization: Bearer $NEW_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "Filter": {
      "GroupIds": ["group-na-xxxxxxxx"],
      "Name": "正面"
    },
    "PageNumber": 1,
    "PageSize": 20,
    "SortBy": "CreateTime",
    "SortOrder": "Desc"
  }'
```

## 5. 在视频生成中使用逻辑素材

客户端在视频生成请求的 `content.<模态>_url.url` 中传入本地素材 URI：

```json
{
  "type": "image_url",
  "image_url": {
    "url": "asset://asset-na-xxxxxxxx"
  },
  "role": "reference_image"
}
```

当一次请求引用多个本地素材时，选路流程为：

1. 提取全部 `asset://asset-na-*` 引用并去重。
2. 校验每个素材都属于当前 New API 账号。
3. 分别取得每个素材拥有可重写上游 ID 的渠道集合。
4. 对这些集合求交集。
5. 将交集作为本次请求的渠道约束，再由原有分组、模型、优先级、权重、熔断和重试逻辑选择最终渠道。
6. 根据最终 Channel ID，把每个本地 URI 改写为该渠道对应的 `asset://<upstream-asset-id>`。
7. 将重写后的请求发送给上游。

示例：

```text
素材 A 的副本渠道：{2, 7}
素材 B 的副本渠道：{7, 11}
交集：              {7}

最终只能选择渠道 7，并分别使用 A、B 在渠道 7 的上游 Asset ID。
```

若交集为空，New API 在请求到达视频上游前返回明确错误，不会把某个渠道的 Asset ID 发给另一个渠道。发生渠道重试时，需要按新的最终渠道重新执行 ID 重写。

这里的“可重写”只要求本地存在该渠道的副本映射，不统一强制检查火山官方的 `Active` 或 Project 规则。官方上游仍可能因素材正在处理、权益不足、Project 不匹配或内容审核失败而拒绝请求；兼容上游则按其自己的规则处理。

不包含 `asset://asset-na-*` 的普通视频请求完全沿用原有选路，不会因为某渠道未配置素材库而被排除。

直接传入某个上游的原始 `asset://asset-...` 不属于渠道透明协议，New API 无法据此安全推导所属渠道或账号，因此视频入口会拒绝这类引用。客户端必须在原生豆包视频入口 `/api/v3/contents/generations/tasks` 使用当前 New API 账号返回的逻辑 Asset ID；兼容入口 `/v1/video/generations` 不接受素材库 URI。

## 6. 管理界面

### 6.1 用户素材库

用户看到聚合后的逻辑资源，而不是每个渠道一行副本。首版界面提供：

- 素材组创建、编辑、删除和列表。
- 素材创建、编辑、删除和列表。
- 按素材组、名称、类型、逻辑状态和副本覆盖情况筛选。
- 查看副本总数、成功数、处理中数量和失败数。
- 展示账号级副本覆盖摘要，不向普通用户暴露渠道 ID 或上游错误详情。
- 图片、视频和音频的界面预览。

创建素材组和素材时不展示渠道选择器。

### 6.2 预览 URL

火山官方 `GetAsset` 和 `ListAssets` 返回的 URL 当前只有 12 小时有效期。New API 的预览应按需从一个可查询的副本获取临时 URL：

- 打开预览时重新请求，不能把 URL 当作永久地址。
- 前端不应长期缓存，也不能用于新渠道的历史回填。
- URL 过期后重新打开预览即可刷新。
- 多渠道均可预览时，只选择一个副本返回，不向用户暴露上游凭据。
- 兼容上游是否返回 URL、有效期多长均为可选能力；无 URL 时界面展示不可预览，而不是认定素材不可用于推理。

### 6.3 管理员渠道界面

管理员在每个渠道的维护抽屉中配置素材库能力，可查看启用状态、认证类型、凭据是否已保存和副本数量，并可手动触发回填。普通用户创建资源时看不到这些渠道细节。

## 7. 官方上游与兼容上游的边界

### 7.1 火山官方约束

火山官方素材库具有以下约束：

- 素材 OpenAPI 仅支持 AK/SK 鉴权；不是 Ark 视频推理 API Key。
- Service 为 `ark`，Version 为 `2024-01-01`，北京 Region 通常为 `cn-beijing`。
- 素材组、素材和视频推理接入点受 Project 隔离。
- 创建素材为异步操作，官方状态为 `Processing`、`Active`、`Failed`。
- Get/List 返回的访问 URL 是临时 URL。
- 还可能受 IAM 策略、权益包、授权函、内容审核和账号级限流约束。

官方文档：

- [私域虚拟人像素材资产库使用指南](https://docs.volcengine.com/docs/82379/2333565?lang=zh)
- [创建素材组 CreateAssetGroup](https://docs.volcengine.com/docs/82379/2318270?lang=zh)
- [创建素材 CreateAsset](https://docs.volcengine.com/docs/82379/2318271?lang=zh)
- [使用预置虚拟人像](https://docs.volcengine.com/docs/82379/2608626?lang=zh)

### 7.2 非官方 Bearer 上游

New API 不对 Bearer 兼容上游统一强制以下语义：

- 不强制其实现 `Active`、`Processing`、`Failed` 状态集合。
- 不强制所有素材属于相同 Project，也不替它验证 Project 隔离。
- 不强制 IAM、账号所有权、授权函或火山权益包规则。
- 不假定其错误码、预览 URL 有效期或内容审核字段与火山相同。
- 不发送 AK/SK HMAC 签名头，只发送 `Authorization: Bearer <key>`。

兼容上游必须至少能接受管理员配置的 Base URL、官方 Action/Version 查询参数和对应 JSON 请求体，并在创建成功时返回可保存的上游 ID。首版不做自动能力探测；管理员应先在测试渠道验证兼容性。

## 8. 失败、一致性与删除行为

多渠道调用不是分布式事务。首版采用“逻辑资源成功落库、渠道副本独立记录结果”的最终一致模型：

- 一个渠道失败不会回滚其他渠道已经创建的副本。
- 部分成功时，列表和详情通过 `Replication` 展示覆盖情况。
- 带素材的视频请求仍可使用成功副本所在的渠道。
- 更新和删除同样可能部分成功；失败副本需要后续重试或人工处理。
- 为避免官方上游级联删除导致逻辑素材与副本失配，New API 只允许删除空素材组；须先逐个删除组内素材。
- 外部直接在上游控制台修改或删除副本不会立即自动反映到逻辑库，需以同步/调用错误为准。

启用新渠道不会阻塞已有渠道的请求。相反，禁用素材库配置会使该渠道退出新的素材同步和带逻辑素材请求的候选集合；已有上游资源是否保留由管理员决定。

首版历史回填为后台的尽力而为任务，不提供跨实例分布式作业调度或跨上游强一致保证。进程重启、限流或网络错误可能中断本轮回填；管理员应通过界面检查失败副本并重新触发同步。

## 9. 安全注意事项

- 上游 AK、SK 和 Bearer Key 仅允许管理员配置，API 响应不得返回明文。
- 已保存凭据仅在 Base URL 不变时支持留空保留；修改 Base URL 必须同时提交对应的新凭据，避免把旧凭据发送到新的上游地址。
- 当前凭据与渠道 Key 一样属于敏感数据库数据；生产环境应保护数据库、备份和日志，并限制管理员权限。首版不承诺独立 KMS 托管。
- 不在错误信息、审计日志或前端状态中输出完整 Authorization、AK、SK、API Key 或带签名的临时 URL。
- 每个逻辑 Group/Asset 操作都必须校验当前 New API 用户所有权，避免通过猜测 ID 跨账号访问。
- Base URL 由管理员控制，不接受普通用户覆盖；生产环境建议仅配置 HTTPS 和可信域名。
- `CreateAsset.URL` 会被上游服务端下载。应使用可信、公开可访问且不包含长期敏感查询参数的地址。
- 预览 URL 可能携带短期签名，应只在当前登录会话中展示，并避免写入持久日志。
- 删除操作不可逆；管理员排查部分删除时，不应直接清空本地映射，否则可能遗留无法追踪的上游资源。

## 10. 部署与运维步骤

1. 完成数据库迁移，确认渠道配置、逻辑素材、素材组副本和素材副本表已创建。
2. 管理员选择一个视频渠道，配置素材库 Base URL、认证方式、Region、Project 并启用。
3. 使用管理员界面或测试请求创建一个素材组，确认该渠道生成了副本映射。
4. 添加一个长期可访问的测试素材 URL，观察副本从创建成功到上游可用的过程。
5. 使用普通用户的 New API Key 调用 List/Get，确认只能看到该账号的逻辑 ID。
6. 使用 `asset://asset-na-*` 发起视频生成，确认请求被约束到有副本的渠道且上游收到自己的 Asset ID。
7. 再启用第二个素材渠道，检查历史素材组先于素材回填，且回填完成前已有渠道仍可服务。
8. 监控上游限流、鉴权失败、源 URL 下载失败、素材审核失败和空交集错误。
9. 定期检查 `failed` 或长期 `processing` 的副本；确认凭据、Project、权益和源 URL 后重试。

官方素材接口存在账号维度限流，新增渠道全量回填时应控制并发并考虑上游重试间隔。大量历史素材首次回填可能需要较长时间。

## 11. 首版限制与后续方向

首版明确限制：

- 逻辑素材仅属于创建它的 New API 账号，暂不提供账号间授权共享。
- 不接受“其他火山账号的 Asset ID”作为可移植素材；只有管理员配置并获授权的渠道凭据能访问其上游库。
- 历史回填依赖原始 URL 的长期可访问性，暂不自动托管到 New API 自有对象存储。
- 不保证外部直接修改上游资源后的双向同步。
- 不对所有兼容上游统一解释素材状态、Project、审核和预览能力。
- 不把预览 URL 视为永久素材地址。
- 不提供跨上游事务回滚、分布式同步队列或自动清理上游孤儿资源。

后续可按实际规模增加持久化同步任务、退避重试、渠道能力探测、自有对象存储、管理员批量重试、孤儿资源审计，以及账号间显式授权共享。

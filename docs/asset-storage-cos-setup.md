# 腾讯 COS 参考素材托管：第一版接入说明

这份文档描述当前实现；架构演进见 [方案](asset-storage-cos-proposal-v1.md)。功能默认关闭，需要配置私有 COS 桶后开启。以下示例不包含真实凭证。

## 部署配置

```dotenv
ASSET_STORAGE_ENABLED=true
COS_BUCKET=your-bucket-1250000000
COS_REGION=ap-guangzhou
COS_SECRET_ID=replace-with-server-secret-id
COS_SECRET_KEY=replace-with-server-secret-key
# 可选：临时凭证 token，必须与上面的 ID/key 配套并按期轮换
COS_SESSION_TOKEN=
# 预签名 GET URL 有效期：60–604800 秒，默认 86400 秒
ASSET_STORAGE_URL_TTL_SECONDS=86400
```

对象读写、校验、签名、删除使用腾讯官方 Go SDK；远程素材下载继续使用现有 SSRF 防护及 Worker 配置。COS 密钥仅从部署环境读取，不保存或回显到管理后台。凭证应限定到目标桶的 `assets/` 前缀，包含上传、读取元数据、下载和删除权限。临时凭证的剩余寿命必须覆盖签名有效期，当前实现不自动申请或续期 STS 凭证。

使用私有读写桶。原件对象记录保存实际 bucket、region、key，修改默认桶不会重定向历史对象；更换凭证后仍需保留对旧桶的权限。当前删除流程针对未开启版本控制的桶；开启版本控制会留下历史版本，必须另外管理其生命周期。不要给正式 `assets/` 原件配置统一短期过期规则。

## 免费额度（MB）

后台入口：系统设置 → 运维设置 → 素材存储 → 默认免费素材额度（MB）。

- 设置键：`asset_storage_setting.default_quota_mb`，初始值 **1024 MB**。
- 接受 0–1,000,000 的整数；**1 MB = 1,000,000 字节**。
- 默认额度同时适用于未设置专属额度的已有用户与新用户。管理员可在素材库选择用户后点击“设置用户额度”，按 MB 覆盖；输入框留空恢复继承，显式 0 禁止新增占用。
- 按同一账号的内容 SHA-256 和媒体类型去重，重复引用同一原件不重复占用额度；不同账号分别计量。
- 上传前原子预占实际字节数，失败释放；租约失效和未完成的逻辑资产创建由维护任务回收预占。
- 调低额度不删除已有素材；超过额度时只能复用已占用容量的原件。设为 0 禁止新增占用。
- 最后一个素材库引用删除后释放免费额度。COS 字节保留至少七天，再由后台删除，避免立即破坏已发出的下载链接；**这段宽限期仍产生平台实际存储用量，不计入用户免费额度**。删除失败会重试，维护任务每分钟处理一个候选对象。

查询自己用量：`GET /api/asset-library/storage`，使用现有账号或 API Token 鉴权。响应 `data` 包含 `enabled`、有效额度 `quota_mb`、`quota_override_mb`（未覆盖为 null）、`default_quota_mb`、`used_bytes`、`remaining_bytes`、`bytes_per_mb`。余额或 token 消费额度不参与此计算。

管理员读取和修改指定用户额度：`GET/PUT /api/asset-library/admin/users/:user_id/storage`。PUT 请求为 `{"quota_mb": 512}`，清除覆盖为 `{"quota_mb": null}`；缺少字段、负数、小数或超出上限会返回 400。仅超级管理员可修改任意用户，普通管理员只能修改低于自己角色的用户，普通用户无修改权限。GET 返回 `can_edit` 供页面控制入口。覆盖配置保存于账号存储记录，不修改已用字节数；并发预占在数据库原子更新中解析有效额度。

## 导入、推理和同步

1. 素材库原有 `CreateAsset` URL 导入先保存已下载并校验的原件，再创建素材记录，随后执行现有渠道同步。
2. 视频请求中的已支持媒体字段统一导入：HTTP(S)、data URL、原始 base64、`asset://asset-na-<32 位十六进制 ID>`，以及 multipart 文件。纯文本和回调 URL 不转存。
3. 同一请求内的渠道重试复用已解析的原件，不再拉取原 URL。新的独立请求再次提交外部 URL 时会下载并校验内容，以识别同 URL 内容变化；内容未变不会再次上传 COS 或重复占用额度。后续使用返回的资产 ID 可避免这次下载。
4. 响应头 `X-New-Api-Asset-Ids` 返回涉及的账号素材 ID。任务私有数据记录资产 ID 与原件对象 ID；不把短期签名作为资产身份。
5. URL 型视频接口得到 COS 预签名 HTTPS 地址；multipart 文件保留原请求文件形式，同时托管同一文件内容。Gemini 原生 `bytesBase64Encoded` 保持原字段编码并额外托管原件。
6. Seedance/Doubao 等现有渠道资产同步仍使用各自供应商协议，从 COS 原件创建副本。自动导入只为所选渠道准备必要副本，不自动向所有渠道扇出。其他明确上传到素材库的操作沿用现有全渠道同步入口。

当前存储解析沿用项目支持的图片、音频和视频格式；单文件上限沿用现有素材库限制：图片严格小于 30 MiB、音频不超过 15 MiB、视频不超过 200 MiB。MB 免费额度与这几个既有 MiB 上传限制是不同概念。供应商额外格式、尺寸和时长要求仍在其同步/推理路径校验。

## 上传与预览接口

本地文件上传：`POST /api/asset-library/upload`，multipart 字段为 `file`、`GroupId`、`Name`、`AssetType`（Image/Audio/Video），需使用已有且属于当前账号的分组。响应沿用素材库 Action 格式，`Result.Id` 为素材 ID。当前页面继续提供原有 URL 导入表单，本地上传可通过此接口调用。

受控内容：`GET` / `HEAD /api/asset-library/assets/:id/content`。支持 Range，验证所有者后通过 COS SDK 读取。管理员检查使用 `/api/asset-library/admin/users/:user_id/assets/:id/content`。图片预览沿用已有 `/preview` 路由。页面音视频先通过带鉴权的请求获取 Blob 再播放，不直接依赖 COS 默认桶域名的浏览器预览行为。

素材返回 `StorageStatus: ready | unmanaged`；供应商副本状态仍单独保留。获取素材时签发新的 `URL`。删除素材后停止通过素材 API 签发新 URL；已经发出的 URL 在失效前仍可读取宽限期内原件。

## 当前边界与上线验收

- 本版在请求内完成转存，上传登记、租约、额度和孤儿清理持久化；尚未引入独立后台导入队列、可跨请求恢复的客户端幂等键或批量历史迁移界面。
- 历史素材在实际用于视频请求，或需要新建渠道副本时按需转存。原 URL 失效不能还原原件，需重新上传。
- 未接入具体自部署 GPU worker 的任务领取及 URL 续期协议；提交时默认签名 24 小时，必须覆盖排队和下载时间。客户端可在素材仍存在时通过素材查询获取新 URL；供应商排队后的链接续期能力需要其协议配合。
- 生成结果自动归档、付费存储和 COS 历史版本清理不在本版范围。
- 本地验证覆盖 SQLite 下去重、额度、失败恢复、七天清理、COS SDK 请求/签名协议、实际 JSON/multipart 请求重写和现有素材库回归。SDK 测试使用可控 HTTP transport，不代表真实 COS 或 GPU 网络验收。MySQL/PostgreSQL 使用 GORM 通用路径，需在目标环境补充实际数据库验证。

上线前应使用真实私有测试桶完成上传→关闭原 URL→预览→供应商同步→GPU 下载的验收，并核实配置凭证、签名有效期及节点网络。此实现不自动启用线上配置。

## 本地 E2E 模拟

运行 `go test ./router -run 'TestAssetStorage.*EndToEnd' -count=1 -v`。测试使用隔离的 SQLite、真实路由与鉴权、腾讯 COS Go SDK，以及本地 HTTP 素材源、COS 和视频/素材同步服务，不需要真实凭证。

覆盖 URL 转存与同内容去重、原 URL 失效后的资产复用与渠道同步、跨用户隔离、MB 额度配置、并发额度竞争、上传失败恢复、文件上传、视频 multipart 原件一致性、206/416 Range 下载，以及删除后的额度释放与延迟对象清理。COS 模拟器检查 SDK 签名字段和内容哈希，不执行腾讯服务端签名认证；视频模拟器校验收到的素材字节，不执行模型推理。

腾讯官方资料：[Go SDK 上传](https://cloud.tencent.com/document/product/436/65644)、[预签名 URL](https://cloud.tencent.com/document/product/436/35059)、[CRC64 校验](https://cloud.tencent.com/document/product/436/62808)。

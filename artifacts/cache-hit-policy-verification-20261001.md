# 用户模型缓存计费策略验证记录

入口：超级管理员 → 用户列表 → 用户菜单 → 缓存计费策略。

按用户、公开模型名和北京时间的请求开始日期累计输入 Token。每个请求随机抽取目标比例，按当天累计 Token 尽量限制缓存读取折扣。单次真实命中率低于随机范围起点时按真实值计费；累计真实比例低于起点时也不补造命中。减少的缓存读取转为普通输入，缓存写入、5 分钟与 1 小时写入数量均不变。

JSON、SSE 和结算使用同一请求分配结果。最终输入用量修正时替换该请求的日累计贡献，保留首次随机目标；重复用量事件去重。未结算请求释放日累计预留，已结算的断连请求保留实际计费贡献。管理员消费日志在 `admin_info.cache_hit_policy` 保存真实量、计费量、差额、目标和日累计快照；普通用户日志沿用现有权限裁剪。

配置存入现有 Options 表，无新增表或列。按用户配置版本检查保存冲突；通用 Option 更新接口不能绕过专用接口。需要 Redis 才能启用。首次分配失败时响应及计费保留真实值；已经向客户端发布的分配结果在后续 Redis 异常时继续复用，避免用量与扣费不一致。

## 实际数据库与 Redis

| 引擎 | 实测版本 | 验证结果 |
| --- | --- | --- |
| SQLite | 3.50.4 | 通过 |
| MySQL | 8.0.46-0ubuntu0.24.04.4 | 通过 |
| PostgreSQL | 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) | 通过 |
| Redis | 7.0.15 | 通过 |

三种数据库均验证策略保存、冲突拒绝、缺失字段拒绝、显式零范围、保留其他模型配置与其他 Options 数据、重复启动迁移、真实缓存较低时直通、响应改写。MySQL 和 PostgreSQL 同时使用独立日志数据库，所有实例为本地测试实例。现有表结构没有改变，因此没有新 schema 的升级步骤。

结算矩阵使用真实钱包、API Token、预扣、结算和消费日志链路，验证输出与上下文长度、普通输入、缓存读取、5 分钟和 1 小时写入以及管理员真实用量。输入 1000、真实读取 900、普通输入 50、5m 写入 25、1h 写入 25、输出 10 的 50% 策略，得到读取 500、普通输入 450、写入各 25；按测试表达式 `p*2 + cr*0.2 + cc*2 + cc1h*4 + c*6` 实际扣 605 quota。该表达式是测试价格，不是模型公开报价。

运行命令（环境变量中的 DSN 为专用本地测试库，未使用线上连接）：

```powershell
$env:TEST_CACHE_POLICY_REDIS_ADDR = '127.0.0.1:16379'
# 配置 TEST_MYSQL_DSN / TEST_POSTGRES_DSN；开启独立日志库。
$env:TEST_MANAGE_USER_SEPARATE_LOG_DB = '1'
foreach ($dialect in @('sqlite', 'mysql', 'postgres')) {
  $env:TEST_MANAGE_USER_DIALECT = $dialect
  go test -p 2 ./controller -run '^TestUserCacheHitPolicy' -count=1 -v
}
# 配置 TEST_FIXED_MYSQL_DSN / TEST_FIXED_MYSQL_LOG_DSN /
# TEST_FIXED_POSTGRES_DSN / TEST_FIXED_POSTGRES_LOG_DSN。
go test -p 2 ./service ./relay/helper -count=1
go test -p 2 ./model
```

以上命令通过。构建临时目录使用 `E:/tmp/cache-policy-build`，Go 构建缓存使用 `E:/tmp/cache-policy-gocache`。

## 前端

```text
bun run typecheck
bun run build
bun run test src/features/users/components/__tests__/cache-hit-policy.test.tsx src/features/usage-logs/components/__tests__/detail-preview.test.tsx
```

类型检查、构建通过；两份相关回归测试共 26 条通过。另运行用户权限测试，共 3 条通过。修改的 TS/TSX 文件通过 oxlint 和 oxfmt 检查。七种界面语言均补齐翻译。

复用项目 Dialog、Combobox、Form、Switch、Input、LoadingState 与 ErrorState。已有 UserChannelRoutingRuleEditor 处理渠道排序与可用性，不能承载缓存计费规则；新增组件只负责这一业务表单，没有新增通用弹窗或选择器实现。

## 基线问题与验证范围

完整 `go test -p 2 ./service ./relay/helper ./model ./controller` 中，controller 有以下三条失败：

- TestKlingNativeRouteSubmitPollSettleAndQuery
- TestServeTaskPluginProtocolDisconnectBeforeDurableBarrierPersistsAndSettlesWithoutRefund
- TestServeTaskPluginImageProtocolDisconnectDuringSubmissionKeepsDurableSettlement

这些测试缺少 `user_channel_routing_overrides` 表，导致 `access_denied` 或 `submission did not start`。通过 Go overlay 移除本功能的响应、结算、路由和 Relay 接入，同时保留工作区其他改动，再单独运行这三条用例，仍得到相同失败。未修改这些无关用例。

支持标准文本 Chat Completions、Completions、Messages、Responses 与 Responses Compact 的 JSON/SSE 用量。含图像或音频缓存明细的用量保留真实值并记录原因；原生 Gemini、Realtime、音视频及异步任务入口不启用这项策略。日统计涵盖本策略接入后的可统计请求，不回填历史日志。

验证使用模拟上游响应、组件 DOM、实际数据库及 Redis，没有调用真实上游或进行线上部署，也未进行真实登录页面的浏览器验收。验证阶段未推送或部署。

# 路由故障演示控制台

启动独立 new-api 进程、三个 OpenAI 兼容的本地上游、持续流量泵和浏览器故障控制台。可重复演示折扣优先、基础池继承、用户覆盖、质量降级、故障切换及自然恢复。

## 启动

需要 Python 3.11+，仅使用标准库。网关二进制需包含折扣路由及路由监控 API；浏览器页面来自二进制内嵌的 `web/dist`。

```powershell
# 已有当前分支的网关二进制时直接启动
python -B tools/routing-fault-demo/run.py --binary ./bin/new-api.exe

# 自行构建：先在 web/ 执行 bun install、bun run build，再回到仓库根目录
go build -o ./bin/new-api.exe .

# 调整总请求速率、并发数，启动自动轮播，5 分钟后自动退出
python -B tools/routing-fault-demo/run.py --binary ./bin/new-api.exe --rate 16 --workers 10 --auto --duration 300

# Linux/macOS：--binary 指向对应平台的可执行文件即可
python3 -B tools/routing-fault-demo/run.py --binary ./bin/new-api
```

终端输出 `CONTROL`（故障控制台）、`GATEWAY`（独立网关）与 `OUTPUT`（本轮数据目录）。默认分配空闲端口，默认关闭轮播；也可指定 `--control-port`、`--gateway-port`。默认数据目录位于系统临时目录；`--output <新目录>` 可指定位置，已有目录会被拒绝，避免覆盖历史运行。

用本轮 `credentials.json` 中的 `monitorroot` 密码登录网关，在模型路由中选择 `route-demo-model`、`default` 分组，打开卡片的监控视图。密码每轮随机生成，不写入源码、网页或终端。该文件也含本轮合成访问令牌，请保留在本机。

`Ctrl+C`，或在本轮 `OUTPUT` 目录新建名为 `stop` 的文件，即可结束本轮进程。数据和 `gateway.log` 保留供排查；下一轮使用新目录。

## 场景与观测

| 场景 | 实际上游行为 |
| --- | --- |
| 正常 | A / B / C 折扣分别为 0.5 / 0.6 / 0.8 |
| 首字变慢 | A 首 Token 等待 900 ms |
| 出字变慢 | A 的 Token 间隔为 300 ms |
| 限流 / 错误 | A 返回真实 HTTP 429 / 502 |
| 流式断连 | A 返回部分内容后断开，流不完整 |
| 双渠道故障 | A、B 同时返回 502 |
| 全部恢复 | 上游恢复正常，等待网关探测和逐步回流 |

基础池采用折扣优先，质量门槛 TTFT 350 ms、TPOT 100 ms，探索流量合计 1.5%。本地演示缩短冷却并加快回流；这些参数只写入本轮独立数据库。root 和 bob 继承基础池；alice 覆盖为 B / C。所有工作线程共享 `--rate` 总速率上限，`--workers` 为并发上限，慢上游时实际速率可能下降。

目标份额、性能与趋势来自真实网关监控 API；控制台不修改控制器内部状态，也不伪造流量份额。选路次数包含重试，不能当作独立客户端请求数。恢复时间依赖采样、流量和故障历史，轮播时间不是路由完成切换的断言。`state.json` 是启动时的无凭据快照，运行中状态可访问控制台同源的 `/api/state`。

## 隔离与复用边界

- 模块只创建自己的进程、新 SQLite 数据库和模拟渠道；无连接已有网关的参数。子进程不继承数据库、Redis、代理、遥测等业务环境配置，工作目录中也没有项目 `.env`。
- 上游与控制台监听 `127.0.0.1`。网关沿用应用本身的 `:PORT` 监听行为，建议在本机开发环境使用。
- 管理及 Relay HTTP 客户端复用 `tools/channel-capacity-e2e/run.py` 的 `Client`。完整源码树需保留该文件。
- 控制台复用原演示 HTML，集中统计全部流量，所有 URL 从本轮运行读取。它在独立 Python 服务中运行；`web/src/components/ui/button.tsx` 等 React 组件依赖应用构建与运行时，无法直接用于该无构建入口。后续集成管理页面时应复用项目组件与 i18n。
- 这是一项开发演示工具，不是生产压测、数据库兼容性或路由算法正确性的完整验证套件。

```powershell
python -B -m unittest discover -s tools/routing-fault-demo -p "test_*.py" -v
```

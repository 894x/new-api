# 用参数覆盖将 OpenAI allowed_tools 转换为 Kimi K3

同时接收 Chat Completions 和 Responses 的 OpenAI 渠道，请使用
[kimi-k3-allowed-tools-chat-responses.json](examples/kimi-k3-allowed-tools-chat-responses.json)。
它按 `request_path` 分别处理 `/v1/chat/completions` 和 `/v1/responses`，
适用于两个接口都直接转发同种协议的渠道，不适用于 Responses → Chat 转换路线。
路径条件为精确匹配，客户端请使用上述路径且不附加查询参数。
该配置整体替换下面的 Chat 单接口配置，不要将两份规则叠加。

本地“from 模力”渠道（`https://api.moark.com`，`kimi-k3`）已验证原生 Responses
接口可用，无需新增渠道或修改渠道类型。这里只说明该上游的实测结果，不代表所有 Kimi 上游。

## Responses 输入

按照 [OpenAI Responses 定义](https://developers.openai.com/api/reference/cli/resources/responses/methods/create)，
Responses 的白名单与工具定义使用扁平名称：

```json
{
  "model": "kimi-k3",
  "input": "查询北京天气。",
  "max_output_tokens": 1024,
  "reasoning": { "effort": "high" },
  "store": false,
  "tool_choice": {
    "type": "allowed_tools",
    "mode": "auto",
    "tools": [{ "type": "function", "name": "get_weather" }]
  },
  "tools": [
    {
      "type": "function",
      "name": "get_weather",
      "description": "查询天气",
      "parameters": {
        "type": "object",
        "properties": { "city": { "type": "string" } },
        "required": ["city"]
      }
    },
    {
      "type": "function",
      "name": "get_time",
      "description": "查询时间",
      "parameters": { "type": "object", "properties": {} }
    }
  ]
}
```

转换后只保留 `get_weather` 的完整定义，`tool_choice` 变为 `"auto"`；
`required` 同样保留其强制调用语义。`input`、`reasoning`、`store`、`stream`
等字段不因白名单转换而修改。Responses 使用原有 `reasoning`，规则不会注入 Chat 的 thinking。
这份配置仅适配 function 白名单，不支持内置工具、custom、MCP 或 namespace 白名单。

2026-09-11 本地网关转发至模力的实测覆盖：auto 调用、auto 文本回答、required 多工具、
required 流式调用、排除工具、空 auto 和非法名称拒绝。捕获实际上游请求验证了筛选、模式和推理参数。
流式工具事件和参数完整，且有 `response.completed`；但上游该结束事件的 `response.output`
为空数组。仅从结束事件读取结果的客户端可能受影响，这个上游响应问题无法通过请求参数覆盖修复。

## Chat 单接口配置

完整配置见 [kimi-k3-allowed-tools.json](examples/kimi-k3-allowed-tools.json)。
部署包含本次扩展的后端和前端后，将该文件内容粘贴到渠道的“参数覆盖”JSON 编辑器。
配置按映射后的模型名 `kimi-k3` 生效；模型别名不同则修改各操作的模型条件。
需要关闭请求体透传，使用 Chat → Chat 转发路径。

如果渠道已有把 `allowed_tools` 转成第一个指定函数或关闭 thinking 的旧规则，
用本配置替换那部分规则；无关参数覆盖可以保留。规则顺序必须保持：校验、筛选、转换。
不要在旧版本服务上直接启用此配置。

## 标准输入与转换结果

输入采用 [OpenAI Chat Completions 的 allowed_tools 定义](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)：

```json
{
  "tool_choice": {
    "type": "allowed_tools",
    "allowed_tools": {
      "mode": "auto",
      "tools": [
        { "type": "function", "function": { "name": "get_weather" } },
        { "type": "function", "function": { "name": "search_docs" } }
      ]
    }
  }
}
```

这些名称必须对应顶层 `tools` 已声明的函数。转换后顶层 `tools` 仅保留这两个函数的
完整定义，保留原顺序，`tool_choice` 为 `"auto"`。`mode: "required"` 同理转为
`"required"`，允许调用白名单内一个或多个工具，不会强制选择第一个函数。

- 不修改 messages、thinking、reasoning_effort、并行调用设置或工具 schema。
- 普通 auto、required、none、指定函数对象及其他模型不触发转换。
- 空白名单配合 auto 转为 `tools: []`、`tool_choice: "none"`；required 配合空白名单返回 400。
- 白名单不存在、不是数组、缺少 function.name、包含非 function 工具、引用未声明函数、
  mode 非 auto/required 时拒绝请求，不丢弃限制后放行。
- 这是 Chat function 工具的适配配置，不转换 Responses 扁平格式或 custom 工具。

[Kimi 的工具选择文档](https://platform.kimi.com/docs/guide/use-tool-choice)没有列出独立的
allowed_tools 参数。本配置通过筛选声明工具实现选择约束，不能承诺模型输出与 OpenAI 相同。
调整 tools 可能减少前缀缓存命中；保持原始完整 tools 又强制限制本轮白名单，需要上游原生支持。
本扩展不解决上游自身的 thinking 参数限制，也不会为了工具转换自动关闭 thinking。

## 新增的通用配置能力

条件可用 `value_path` 从**当前操作执行前的请求根对象**读取比较值，替代固定 `value`：

```json
{
  "path": "function.name",
  "mode": "in",
  "value_path": "tool_choice.allowed_tools.tools.#.function.name",
  "invert": true,
  "pass_missing_key": true
}
```

这条条件放在 `prune_objects.value.conditions` 内，删除不在白名单里的函数。
`path` 相对于当前待筛选对象，`value_path` 相对于请求根对象。`recursive: false`
避免进入工具 schema 删除嵌套字段。动态取值在遍历前解析，因此各对象使用同一份白名单，
不会读取对象内部同名字段，也不会跨请求保存解析结果。

| 字段或模式 | 语义 |
| --- | --- |
| `value_path` | 使用 GJSON 路径读取比较值，支持数组投影 `#`；也适用于原有 full/gt 等模式 |
| `in` | 左值是一个标量，右值是标量数组，按类型和值精确匹配 |
| `subset` | 左右值均为标量数组，左侧每个元素都必须存在于右侧；忽略顺序和重复数量 |
| `invert: true` | 对匹配结果取反；可用于“不属于”或“不是子集” |

`value` 和 `value_path` 互斥。引用路径不存在、集合操作数类型不正确会返回错误，
不会把缺失引用当成空白名单。空数组是有效集合；空集合是任意集合的子集。
`pass_missing_key` 仅控制左侧 path 缺失时的匹配结果，不忽略右侧引用错误。

条件按配置数组顺序短路求值：AND 遇到 false 停止，OR 遇到 true 停止。
动态引用之前应放模型、tool_choice.type 等适用范围条件；需要顺序时使用条件数组，
不要使用无序条件对象。每个操作看到前序操作修改后的请求，因此最后才覆盖 tool_choice。

可视化编辑器提供“属于集合”“是其子集”和“匹配值路径（可选）”；填写路径时，
固定匹配值输入框禁用，保存时只输出 value_path。

## 验证

`relay/common/override_membership_test.go` 直接加载完整示例配置，通过渠道的请求策略入口
验证筛选、模式转换和最终参数能力校验，并比较完整请求，检查工具 schema 和其他字段不变。
前端回归测试覆盖完整配置保存以及动态筛选条件编辑。
`router/allowed_tools_e2e_test.go` 进一步通过真实鉴权、选渠道、计费和 HTTP/SSE 上游边界
验证非流式 auto/required、流式 auto 和非法白名单拦截，捕获实际上游请求进行断言。
这些自动化检查验证本地转换链路；上游实测范围见前面的 Responses 说明，未部署到线上。

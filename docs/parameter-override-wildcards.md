# 参数覆盖的通配符

`operations` 支持以独立路径段 `*` 遍历数组元素或对象字段，可使用多层通配符。
例如 `messages.*.content.*.video_url` 匹配所有消息内的 `video_url`。
这不是递归搜索，也不是字段名的部分匹配；不支持 `**` 或 `video_*`。
顶层普通覆盖对象仍把键当作字面字段名，通配符操作需要放在 `operations` 中。

## 操作与绑定

- `set`、`delete`、字符串修改、`prune_objects` 等路径操作以 `path` 确定匹配项。
- `move` / `copy` 以 `from` 确定匹配项；`to` 按从左到右的顺序复用捕获的下标或对象键。
  两边的通配符数量必须相同。目标字段可以不存在，沿用普通点号路径的写入规则。
- 不会把两个数组独立展开再组合，也不会把所有来源反复写入同一个固定目标。
  多个目标相互覆盖、目标覆盖另一个匹配项的来源时，返回配置错误。
- 空数组、缺失的中间路径、字符串形式的 `content` 不产生匹配。
  `move/copy` 只匹配已存在的来源；`set` 可以创建现有对象上缺少的末级字段。
- 支持数组负下标，例如 `groups.*.items.-1`。对象键中的点、星号等符号按字面键处理。

例如 `from: items.*.name` 匹配 `items.3.name` 时，`to: output.*.label`
写入 `output.3.label`。跳过部分来源不会重新编号；写入数组的原始下标可能留下 `null` 空位。

## 条件

操作的 `conditions[].path` 和 `conditions[].value_path` 可以引用同一匹配范围：

- `messages.*.content.*.video_url` 对应当前内容项。
- `messages.*.role` 对应当前内容项所在的消息。
- `model`、`request_path` 或不带通配符的 `value_path` 保持原有请求/上下文读取语义。
- 无关集合如 `tools.*.name` 无法从 `messages.*.content.*` 推断绑定，返回明确错误。

一条操作的全部匹配项及条件在该操作开始时确定，然后执行修改。
`operations` 仍按顺序执行，后一条操作能看到前一条的结果。
数组删除和移动按反向下标执行，避免数组收缩造成错位；匹配项保留具体路径用于审计和报错。
`logic` 仍只表示当前项的多个条件之间的 `AND` / `OR`，`invert` 和
`pass_missing_key` 仍分别对当前条件生效。

`return_error`、请求头操作和 `sync_fields` 没有上述逐项绑定入口，不能在条件中使用未绑定的 `*`。
需要全局“任意/全部”判断时，继续使用现有 GJSON 查询和计数，例如
`tools.#(type=="function")#|#` 与 `tools.#` 比较；本次没有增加隐式的聚合含义。
不带逐项通配符的 GJSON 查询维持原有语义。用于确定匹配项的 `path/from` 应为点号路径，
不要混合通配符遍历和 GJSON 查询表达式。
`prune_objects.value.conditions` 仍使用原有的候选对象相对路径和请求根级 `value_path`，
不参与外层操作的通配符绑定。

## 图片、视频字符串包装成 URL 对象

`move` 支持直接把字段移入其子字段：`video_url` → `video_url.url`。
整个原值被包进新结构，不再需要临时字段；移动到相同路径保持原值。
移入父字段时，用来源值替换父字段。`copy` 则保留来源，沿用目标写入语义。
传递 JSON 原值时保留大整数、布尔值和 `null`。

```json
{
  "operations": [
    {
      "mode": "move",
      "from": "messages.*.content.*.video_url",
      "to": "messages.*.content.*.video_url.url",
      "conditions": [
        {"path": "messages.*.content.*.video_url", "mode": "prefix", "value": "https://"},
        {"path": "messages.*.content.*.video_url", "mode": "prefix", "value": "http://"},
        {"path": "messages.*.content.*.video_url", "mode": "prefix", "value": "data:"}
      ],
      "logic": "OR"
    },
    {
      "mode": "move",
      "from": "messages.*.content.*.image_url",
      "to": "messages.*.content.*.image_url.url",
      "conditions": [
        {"path": "messages.*.content.*.image_url", "mode": "prefix", "value": "https://"},
        {"path": "messages.*.content.*.image_url", "mode": "prefix", "value": "http://"},
        {"path": "messages.*.content.*.image_url", "mode": "prefix", "value": "data:"}
      ],
      "logic": "OR"
    }
  ]
}
```

字符串 URL 转为 `{"url":"..."}`；已是对象的值不满足上述前缀条件，保留 `detail/fps` 等字段。
以上配置只调整 JSON 结构。URL 下载、Base64 转换仍由参数能力执行，媒体服务器的 HTTP 403
需要单独处理。

包含 Kimi `allowed_tools` 规则的完整 12 条示例见
[回归配置](../relay/common/testdata/override_wildcard_media_allowed_tools.json)。

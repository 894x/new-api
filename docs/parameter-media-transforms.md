# 参数能力中的媒体 URL 转换

在渠道的“参数能力”编辑器中，给媒体参数选择“输入转换”。配置仍保存在
`other_settings.parameter_capabilities`，没有新的渠道开关或数据库表。

支持三种内置转换：`image_url_to_base64`、`audio_url_to_base64`、
`video_url_to_base64`。未设置时继承，`none` 显式关闭继承的转换。
只下载 HTTP/HTTPS 地址，已存在的 Data URL 保持不变。

## 配置示例

以下规则只作用于该渠道的上游模型 `vision-model`：

```json
{
  "defaults": {},
  "rules": [
    {
      "selector": { "type": "exact", "value": "vision-model" },
      "parameters": {
        "messages.*.content.*.image_url": {
          "supported": true,
          "transform": "image_url_to_base64"
        },
        "messages.*.content.*.input_audio": {
          "supported": true,
          "transform": "audio_url_to_base64"
        },
        "messages.*.content.*.video_url": {
          "supported": true,
          "transform": "video_url_to_base64"
        }
      }
    }
  ]
}
```

规则优先级沿用渠道默认、模型通配、精确模型。模型名按映射后的上游名称匹配。
参数路径沿用现有参数能力执行位置：通常对应协议转换后的请求体。
现有 Chat → Responses 全局兼容模式在 Chat 结构上执行请求策略，配置仍使用
`messages.*.content.*`；原生 Responses 图片使用 `input.*.content.*.image_url`。
不同提供方的字段结构不同，可通过自定义参数路径指定实际的媒体字段。

## 输出与限制

- 字符串 URL 替换为 `data:<mime>;base64,<data>`；`{ "url": "..." }`
  只替换 `url`，保留 `detail`、`fps` 等字段。
- `input_audio` 或 `input_audio.data` 使用裸 Base64，依据下载内容补充 MP3/WAV
  的 `format`。已声明的格式必须一致，已有裸 Base64 保持不变。
- `audio_url` 可用于接受音频 Data URL 的上游；本功能不会把未知协议字段自动
  改造成 `input_audio`，也不会让原本不支持某媒体类型的模型获得该能力。
- 执行顺序为参数覆盖、媒体下载转换、能力校验；已删除或缺失的参数不下载。
  不能同时在媒体转换参数上配置数值边界、允许值列表或计费参数转换。
- 渠道筛选只检查能力，不下载、不修改候选请求。重试保留原始请求，下载结果
  在请求内复用。转换不兼容全局或渠道请求体透传，冲突时返回明确错误。
- 下载由本机执行，使用现有 SSRF 防护客户端与重定向校验，不经过 Worker，
  不携带客户端认证头或渠道密钥。应用现有单文件下载上限，并限制每次请求
  最多 16 次下载、累计 64 MiB、下载阶段 60 秒；转换后的请求上限为 96 MiB。
- 下载失败、错误媒体类型或超限会使请求失败，不回退为远程 URL。
  转换审计只保存参数路径和动作，不保存 URL/Base64；启用转换时跳过相关
  转发处理器的请求体调试输出。
- 浏览器中的请求预览不下载媒体，只提示“需要下载”；最终结果以服务端为准。

## 视频文件大小与 Messages 来源

视频的 `media` 能力支持 Chat 的 `messages.*.content.*.video_url`、Responses 的
`input.*.content.*.video_url`，以及 Messages 的 `messages.*.content.*.source`。
Messages 只检查 `type: "video"` 或 `source.media_type` 为 `video/*` 的内容块，
不会把图片和普通文档的 `source` 当成视频。URL 转换为 Base64 时，保留原生
`source.type`、`source.media_type`、`source.data` 结构；反向转换使用
`source.type: "url"` 和 `source.url`。Messages 转 Chat / Responses 时也保留视频。

`media.formats.url.max_media_bytes` 与 `media.formats.base64.max_media_bytes`
都按单个视频的原始文件字节数限制，Base64 按解码后的字节数计算。
例如 10 MiB 应填写 `10485760`。两种格式均允许时优先原样传递；
`media.conversions.url_to_base64: true` 允许需要时下载转换，并不强制每个 URL 下载。

URL 会先通过 HEAD / Range 探测大小；未知大小仍可原样传递，源站声明也可能不准确。
需要严格限制时，设置 `media.formats.url.supported: false`、
`media.formats.base64.supported: true`、Base64 上限为 `10485760`，并启用
`media.conversions.url_to_base64`。这样 URL 必须下载，实际超限时不发送给上游。
启用 `participate_in_selection` 可筛除不兼容的渠道；下载后才确认的大小会参与重试筛选，
所有候选都不兼容时返回 HTTP 400。转换继续受全局下载和请求体上限约束。

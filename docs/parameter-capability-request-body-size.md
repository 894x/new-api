# 参数能力中的请求体大小过滤

参数能力支持虚拟参数 `$request.body_size_bytes`，用于在优先级、权重和动态路由之前，按客户端请求体的实际字节数过滤候选渠道。它不是请求 JSON 中的字段，也不会写入或修改上游请求。

```json
{
  "defaults": {
    "$request.body_size_bytes": {
      "max": 1048576,
      "on_violation": "reject",
      "participate_in_selection": true
    }
  }
}
```

- `min`、`max` 的单位均为字节，必须是非负整数；边界包含在允许范围内。
- 只支持 `reject`，不支持 `drop`、`clamp`、`supported`、`allowed_values` 或媒体转换。
- 必须通过 `participate_in_selection: true` 启用候选渠道过滤；模型 pattern/exact 规则仍按既有优先级覆盖渠道默认值。
- 统计口径是网关读取到的客户端请求体。压缩请求在解压中间件之后计量；协议转换、参数覆盖、媒体 URL 下载和 Base64 转换不会改变本次选择所用的大小。
- JSON、表单和 multipart 请求均使用 BodyStorage 的实际大小；不会依赖客户端可伪造或缺失的 `Content-Length`。

如果所有候选渠道都不满足大小限制，请求会按现有参数能力选择失败路径返回“不存在兼容渠道”。全局 `MAX_REQUEST_BODY_MB` 仍是独立的安全上限，会更早拒绝超大请求；`http1_large_body_threshold_bytes` 只控制上游 HTTP 传输策略，也不参与此过滤。

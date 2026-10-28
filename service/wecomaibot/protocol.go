// Package wecomaibot 实现企业微信智能机器人（API 模式）长连接对话服务：
// 维持 wss 单连接、解析回调消息、用 Function Calling 驱动平台内部查询，
// 并以覆盖式流式帧回复用户。协议参考官方文档 101463（智能机器人长连接）。
package wecomaibot

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// wecomAIBotWSURL 官方长连接地址；单机器人仅允许一条连接，新连接会踢掉旧连接。
// 声明为变量供测试注入本地假服务器。
var wecomAIBotWSURL = "wss://openws.work.weixin.qq.com"

const (
	cmdSubscribe      = "aibot_subscribe"
	cmdPing           = "ping"
	cmdMsgCallback    = "aibot_msg_callback"
	cmdEventCallback  = "aibot_event_callback"
	cmdRespondMsg     = "aibot_respond_msg"
	cmdRespondWelcome = "aibot_respond_welcome_msg"

	cmdUploadMediaInit   = "aibot_upload_media_init"
	cmdUploadMediaChunk  = "aibot_upload_media_chunk"
	cmdUploadMediaFinish = "aibot_upload_media_finish"

	cmdSendMsg = "aibot_send_msg"
)

const (
	chatTypeSingle = "single"
	chatTypeGroup  = "group"

	eventEnterChat    = "enter_chat"
	eventDisconnected = "disconnected_event"
)

// newReqID 按官方 SDK 约定生成 req_id：{cmd前缀}_{毫秒时间戳}_{随机串}。
// 服务端回执没有 cmd 字段，只能靠 req_id 与请求关联。
func newReqID(prefix string) string {
	return fmt.Sprintf("%s_%d_%s", prefix, time.Now().UnixMilli(), common.GetRandomString(8))
}

type frameHeaders struct {
	ReqID string `json:"req_id"`
}

type outFrame struct {
	Cmd     string       `json:"cmd,omitempty"`
	Headers frameHeaders `json:"headers"`
	Body    any          `json:"body,omitempty"`
}

// inFrame 入站帧：带 cmd 的是回调（消息/事件），不带 cmd 的是请求回执，
// errcode/errmsg 位于 JSON 顶层（与 headers 平级）。
type inFrame struct {
	Cmd     string          `json:"cmd,omitempty"`
	Headers frameHeaders    `json:"headers"`
	Body    json.RawMessage `json:"body,omitempty"`
	ErrCode int             `json:"errcode"`
	ErrMsg  string          `json:"errmsg,omitempty"`
}

type subscribeBody struct {
	BotID  string `json:"bot_id"`
	Secret string `json:"secret"`
}

// msgCallback 一条用户消息回调。群聊才有 chatid；from.userid 可能是加密的
// open_userid；群聊仅支持 text/mixed/引用，图片语音文件视频只在单聊出现。
type msgCallback struct {
	MsgID    string `json:"msgid"`
	AIBotID  string `json:"aibotid"`
	ChatID   string `json:"chatid"`
	ChatType string `json:"chattype"`
	From     struct {
		UserID string `json:"userid"`
	} `json:"from"`
	MsgType string        `json:"msgtype"`
	Text    *textContent  `json:"text"`
	Voice   *textContent  `json:"voice"`
	Mixed   *mixedContent `json:"mixed"`
	Quote   *quoteContent `json:"quote"`
}

type textContent struct {
	Content string `json:"content"`
}

type mixedContent struct {
	MsgItem []mixedItem `json:"msg_item"`
}

type mixedItem struct {
	MsgType string       `json:"msgtype"`
	Text    *textContent `json:"text"`
}

type quoteContent struct {
	MsgType string       `json:"msgtype"`
	Text    *textContent `json:"text"`
}

type eventCallback struct {
	MsgID    string `json:"msgid"`
	ChatType string `json:"chattype"`
	Event    struct {
		EventType string `json:"eventtype"`
	} `json:"event"`
}

// streamBody 覆盖式流式回复：同一 stream.id 的每帧 content 为全量覆盖，
// finish=true 结束本轮回复；所有帧必须透传回调的 req_id。
type streamBody struct {
	MsgType string      `json:"msgtype"`
	Stream  streamField `json:"stream"`
}

type streamField struct {
	ID      string `json:"id"`
	Finish  bool   `json:"finish"`
	Content string `json:"content"`
}

// uploadInitBody 开始分片上传：md5 对原始图片字节；total_size 为 base64 编码前的字节数。
type uploadInitBody struct {
	Type        string `json:"type"`
	Filename    string `json:"filename"`
	TotalSize   int    `json:"total_size"`
	TotalChunks int    `json:"total_chunks"`
	MD5         string `json:"md5"`
}

type uploadChunkBody struct {
	UploadID   string `json:"upload_id"`
	ChunkIndex int    `json:"chunk_index"`
	Base64Data string `json:"base64_data"`
}

type uploadFinishBody struct {
	UploadID string `json:"upload_id"`
}

// uploadInitAck / uploadFinishAck 解析上传各步回执 body。
type uploadInitAck struct {
	UploadID string `json:"upload_id"`
}

type uploadFinishAck struct {
	MediaID   string `json:"media_id"`
	CreatedAt int64  `json:"created_at"`
}

// mediaReplyBody 以独立图片消息回复（media_id 三天内有效），
// 不依赖流式帧 msg_item 的客户端渲染支持。
type mediaReplyBody struct {
	MsgType string     `json:"msgtype"`
	Image   mediaImage `json:"image"`
}

type mediaImage struct {
	MediaID string `json:"media_id"`
}

// 主动推送（aibot_send_msg）：用户须先在会话中给机器人发过消息；ChatType
// 1=单聊（chatid 填 userid）、2=群聊。回复 markdown 或已上传的图片素材。
type sendMarkdownBody struct {
	ChatID   string `json:"chatid"`
	ChatType int    `json:"chat_type"`
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

type sendImageBody struct {
	ChatID   string     `json:"chatid"`
	ChatType int        `json:"chat_type"`
	MsgType  string     `json:"msgtype"`
	Image    mediaImage `json:"image"`
}

type textBody struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

func newStreamFrame(reqID string, streamID string, content string, finish bool) *outFrame {
	return &outFrame{
		Cmd:     cmdRespondMsg,
		Headers: frameHeaders{ReqID: reqID},
		Body: streamBody{
			MsgType: "stream",
			Stream: streamField{
				ID:      streamID,
				Finish:  finish,
				Content: content,
			},
		},
	}
}

func newTextFrame(cmd string, reqID string, content string) *outFrame {
	body := textBody{MsgType: "text"}
	body.Text.Content = content
	return &outFrame{
		Cmd:     cmd,
		Headers: frameHeaders{ReqID: reqID},
		Body:    body,
	}
}

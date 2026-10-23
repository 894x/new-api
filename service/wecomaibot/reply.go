package wecomaibot

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
)

// 企微流式帧 content 上限 20480 字节，预留 JSON 结构与转义开销。
const maxStreamContentBytes = 20000

// streamReplier 在同一条回调 req_id 上推送覆盖式流式回复。
type streamReplier struct {
	w        *wsConn
	reqID    string
	streamID string
}

func newStreamReplier(w *wsConn, reqID string) *streamReplier {
	return &streamReplier{w: w, reqID: reqID, streamID: newReqID("stream")}
}

// progress 推送中间帧。中间帧丢失可接受，不因回执超时打断查询流程。
func (r *streamReplier) progress(content string) {
	frame := newStreamFrame(r.reqID, r.streamID, truncateStreamContent(content), false)
	if _, err := r.w.sendRequest(frame, ackWait); err != nil {
		common.SysLog("wecom ai bot progress frame dropped: " + err.Error())
	}
}

// finish 推送最终帧，失败后间隔重试一次，尽量保证结果送达。
func (r *streamReplier) finish(content string) {
	r.finishWithImages(content, nil)
}

// finishWithImages 推送文本最终帧，随后把图表经「分片上传 + 独立图片消息」
// 回传（图片作为独立气泡跟在文字后，不依赖流式帧 msg_item 的渲染支持）。
func (r *streamReplier) finishWithImages(content string, images [][]byte) {
	frame := newStreamFrame(r.reqID, r.streamID, truncateStreamContent(content), true)
	if _, err := r.w.sendRequest(frame, ackWait); err != nil {
		time.Sleep(time.Second)
		if _, err := r.w.sendRequest(frame, ackWait); err != nil {
			common.SysLog("wecom ai bot failed to send final reply: " + err.Error())
			return
		}
	}
	for i, png := range images {
		mediaID, err := r.w.uploadMedia(png, fmt.Sprintf("chart-%d.png", i+1))
		if err != nil {
			common.SysLog(fmt.Sprintf("wecom ai bot chart upload failed (index %d): %v", i, err))
			continue
		}
		reply := &outFrame{
			Cmd:     cmdRespondMsg,
			Headers: frameHeaders{ReqID: r.reqID},
			Body:    mediaReplyBody{MsgType: "image", Image: mediaImage{MediaID: mediaID}},
		}
		if _, err := r.w.sendRequest(reply, ackWait); err != nil {
			common.SysLog("wecom ai bot chart image reply failed: " + err.Error())
		}
	}
}

// truncateStreamContent 按 UTF-8 边界截断超长内容。
func truncateStreamContent(content string) string {
	if len(content) <= maxStreamContentBytes {
		return content
	}
	cut := maxStreamContentBytes
	for cut > 0 && !utf8.RuneStart(content[cut]) {
		cut--
	}
	return content[:cut] + "…"
}

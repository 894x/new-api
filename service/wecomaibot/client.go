package wecomaibot

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gorilla/websocket"
)

const (
	heartbeatInterval   = 30 * time.Second
	ackWait             = 5 * time.Second
	subscribeAckWait    = 10 * time.Second
	sendQueueWait       = 5 * time.Second
	writeWait           = 10 * time.Second
	dialTimeout         = 15 * time.Second
	sendQueueSize       = 64
	missedPingLimit     = 2
	reconnectMinBackoff = time.Second
	reconnectMaxBackoff = 30 * time.Second
	maxAuthFailures     = 5
)

type clientExit int

const (
	exitRetry clientExit = iota
	exitCancelled
	exitKicked
	exitAuthFailed
)

// botCredential 是建立长连接所需的凭证快照，配置变更时由运行器重建客户端。
type botCredential struct {
	BotID  string
	Secret string
}

// client 维持单机器人的长连接生命周期：断线按指数退避重连；鉴权失败累计到上限
// 后退出；被服务端踢下线（另一处连接顶替）时立即退出交回租约。
type client struct {
	credential botCredential
	kicked     atomic.Bool
}

// wsConn 包装一条已建立的连接：gorilla 要求并发写串行化，所有出站帧经
// sendCh 由单写协程发出；回执按 req_id 投递给等待中的请求。
type wsConn struct {
	ws        *websocket.Conn
	sendCh    chan []byte
	pending   sync.Map
	done      chan struct{}
	closeOnce sync.Once
}

func (w *wsConn) close() {
	w.closeOnce.Do(func() {
		close(w.done)
		_ = w.ws.Close()
	})
}

func (w *wsConn) writePump() {
	for {
		select {
		case data := <-w.sendCh:
			_ = w.ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := w.ws.WriteMessage(websocket.TextMessage, data); err != nil {
				w.close()
				return
			}
		case <-w.done:
			return
		}
	}
}

// sendRequest 发送一帧并等待服务端回执（按 req_id 关联）。errcode 非零时返回错误。
func (w *wsConn) sendRequest(frame *outFrame, ackTimeout time.Duration) (*inFrame, error) {
	reqID := frame.Headers.ReqID
	ackCh := make(chan *inFrame, 1)
	w.pending.Store(reqID, ackCh)
	defer w.pending.Delete(reqID)
	data, err := common.Marshal(frame)
	if err != nil {
		return nil, err
	}
	select {
	case w.sendCh <- data:
	case <-w.done:
		return nil, errors.New("wecom ai bot connection is closed")
	case <-time.After(sendQueueWait):
		return nil, errors.New("wecom ai bot send queue is full")
	}
	select {
	case ack := <-ackCh:
		if ack.ErrCode != 0 {
			return ack, fmt.Errorf("wecom ai bot api error %d: %s", ack.ErrCode, ack.ErrMsg)
		}
		return ack, nil
	case <-time.After(ackTimeout):
		return nil, errors.New("wecom ai bot ack timeout")
	case <-w.done:
		return nil, errors.New("wecom ai bot connection is closed")
	}
}

// dispatchAck 将无 cmd 的回执帧投递给等待方；投递不阻塞读循环。
func (w *wsConn) dispatchAck(frame *inFrame) {
	if ch, ok := w.pending.Load(frame.Headers.ReqID); ok {
		select {
		case ch.(chan *inFrame) <- frame:
		default:
		}
	}
}

func (w *wsConn) readLoop(onFrame func(frame *inFrame)) error {
	for {
		_, data, err := w.ws.ReadMessage()
		if err != nil {
			return err
		}
		var frame inFrame
		if err := common.Unmarshal(data, &frame); err != nil {
			// 非法帧直接忽略，保持连接
			continue
		}
		if frame.Cmd == "" {
			w.dispatchAck(&frame)
			continue
		}
		onFrame(&frame)
	}
}

// heartbeatLoop 每 30 秒发送一次心跳；连续 missedPingLimit 次无回执判定连接
// 已死亡，主动关闭触发重连。
func (w *wsConn) heartbeatLoop(ctx context.Context, onPong func()) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	missed := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.done:
			return
		case <-ticker.C:
			ping := &outFrame{Cmd: cmdPing, Headers: frameHeaders{ReqID: newReqID(cmdPing)}}
			if _, err := w.sendRequest(ping, ackWait); err != nil {
				missed++
				if missed >= missedPingLimit {
					w.close()
					return
				}
				continue
			}
			missed = 0
			onPong()
		}
	}
}

// run 维持连接直到 ctx 取消、鉴权连续失败或被踢下线，返回终态原因。
func (cl *client) run(ctx context.Context) clientExit {
	backoff := reconnectMinBackoff
	authFailures := 0
	for ctx.Err() == nil {
		exit := cl.runOnce(ctx)
		switch exit {
		case exitCancelled:
			return exitCancelled
		case exitKicked:
			cl.kicked.Store(false)
			return exitKicked
		case exitAuthFailed:
			authFailures++
			if authFailures >= maxAuthFailures {
				return exitAuthFailed
			}
		default:
			authFailures = 0
		}
		botState.set(stateReconnecting, "")
		select {
		case <-ctx.Done():
			return exitCancelled
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, reconnectMaxBackoff)
	}
	return exitCancelled
}

// runOnce 走完一条连接的完整生命周期：拨号 → 订阅鉴权 → 心跳+读循环 → 结束。
func (cl *client) runOnce(ctx context.Context) clientExit {
	botState.set(stateConnecting, "")
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	ws, _, err := websocket.DefaultDialer.DialContext(dialCtx, wecomAIBotWSURL, nil)
	cancel()
	if err != nil {
		botState.set(stateReconnecting, err.Error())
		return exitRetry
	}
	w := &wsConn{ws: ws, sendCh: make(chan []byte, sendQueueSize), done: make(chan struct{})}
	defer w.close()
	gopool.Go(w.writePump)

	// 读循环必须先于任何 sendRequest 启动：服务端回执没有 cmd 字段，
	// 只能由读循环按 req_id 投递给等待方，否则订阅等待自身回执会死等超时。
	var workers sync.WaitGroup
	workers.Add(1)
	gopool.Go(func() {
		defer workers.Done()
		_ = w.readLoop(func(frame *inFrame) { cl.dispatch(w, frame) })
	})

	subscribe := &outFrame{
		Cmd:     cmdSubscribe,
		Headers: frameHeaders{ReqID: newReqID(cmdSubscribe)},
		Body:    &subscribeBody{BotID: cl.credential.BotID, Secret: cl.credential.Secret},
	}
	ack, err := w.sendRequest(subscribe, subscribeAckWait)
	if err != nil {
		// 收到回执但 errcode 非零说明凭证被拒绝，与网络类超时区分处理。
		w.close()
		workers.Wait()
		if ack != nil && ack.ErrCode != 0 {
			botState.set(stateConnecting, fmt.Sprintf("subscribe rejected: errcode=%d %s", ack.ErrCode, ack.ErrMsg))
			return exitAuthFailed
		}
		botState.set(stateReconnecting, err.Error())
		return exitRetry
	}
	botState.set(stateConnected, "")

	workers.Add(1)
	gopool.Go(func() {
		defer workers.Done()
		w.heartbeatLoop(ctx, botState.markPong)
	})

	select {
	case <-ctx.Done():
		w.close()
		workers.Wait()
		return exitCancelled
	case <-w.done:
		workers.Wait()
		if cl.kicked.Load() {
			return exitKicked
		}
		return exitRetry
	}
}

// dispatch 按回调帧类型分发给处理协程，不阻塞读循环。
func (cl *client) dispatch(w *wsConn, frame *inFrame) {
	switch frame.Cmd {
	case cmdMsgCallback:
		gopool.Go(func() { handleMessage(w, frame) })
	case cmdEventCallback:
		gopool.Go(func() { handleEvent(cl, w, frame) })
	}
}

// uploadMedia 经长连接三步分片上传图片素材，返回三天内有效的 media_id。
// 单片 base64 载荷 ≤512KB、最多 100 片；串行等回执规避官方记录的并发限制。
func (w *wsConn) uploadMedia(png []byte, filename string) (string, error) {
	const (
		uploadChunkSize = 512 * 1024
		uploadMaxChunks = 100
		uploadAckWait   = 15 * time.Second
	)
	encoded := base64.StdEncoding.EncodeToString(png)
	totalChunks := (len(encoded) + uploadChunkSize - 1) / uploadChunkSize
	if totalChunks > uploadMaxChunks {
		return "", fmt.Errorf("chart image needs %d chunks (max %d)", totalChunks, uploadMaxChunks)
	}
	digest := md5.Sum(png)
	init := &outFrame{
		Cmd:     cmdUploadMediaInit,
		Headers: frameHeaders{ReqID: newReqID(cmdUploadMediaInit)},
		Body: uploadInitBody{
			Type:        "image",
			Filename:    filename,
			TotalSize:   len(png),
			TotalChunks: totalChunks,
			MD5:         hex.EncodeToString(digest[:]),
		},
	}
	initAck, err := w.sendRequest(init, uploadAckWait)
	if err != nil {
		return "", fmt.Errorf("upload init failed: %w", err)
	}
	var initResult uploadInitAck
	if err := common.Unmarshal(initAck.Body, &initResult); err != nil || initResult.UploadID == "" {
		return "", errors.New("upload init returned no upload_id")
	}
	for i := range totalChunks {
		end := min((i+1)*uploadChunkSize, len(encoded))
		chunk := &outFrame{
			Cmd:     cmdUploadMediaChunk,
			Headers: frameHeaders{ReqID: newReqID(cmdUploadMediaChunk)},
			Body: uploadChunkBody{
				UploadID:   initResult.UploadID,
				ChunkIndex: i,
				Base64Data: encoded[i*uploadChunkSize : end],
			},
		}
		if _, err := w.sendRequest(chunk, uploadAckWait); err != nil {
			return "", fmt.Errorf("upload chunk %d failed: %w", i, err)
		}
	}
	finish := &outFrame{
		Cmd:     cmdUploadMediaFinish,
		Headers: frameHeaders{ReqID: newReqID(cmdUploadMediaFinish)},
		Body:    uploadFinishBody{UploadID: initResult.UploadID},
	}
	finishAck, err := w.sendRequest(finish, uploadAckWait)
	if err != nil {
		return "", fmt.Errorf("upload finish failed: %w", err)
	}
	var finishResult uploadFinishAck
	if err := common.Unmarshal(finishAck.Body, &finishResult); err != nil || finishResult.MediaID == "" {
		return "", errors.New("upload finish returned no media_id")
	}
	return finishResult.MediaID, nil
}

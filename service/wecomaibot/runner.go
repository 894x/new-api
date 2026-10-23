package wecomaibot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	// 长连接持有权复用 system_task_locks 表做跨实例租约：企微限制单机器人
	// 单连接、新连互踢，多实例部署时必须只有一个实例发起连接。
	connLockType    = "wecom_aibot_connection"
	connLockTaskID  = "wecom_aibot_connection"
	connLeaseTTL    = 45
	runnerTick      = 10 * time.Second
	kickedBackoff   = 2 * time.Minute
	authFailBackoff = 5 * time.Minute
)

const (
	stateDisabled     = "disabled"
	stateStandby      = "standby"
	stateConnecting   = "connecting"
	stateConnected    = "connected"
	stateReconnecting = "reconnecting"
	stateKicked       = "kicked"
	stateAuthFailed   = "auth_failed"
)

// Status 是管理端可见的连接状态快照。
type Status struct {
	Enabled     bool   `json:"enabled"`
	Holding     bool   `json:"holding"`
	State       string `json:"state"`
	LastError   string `json:"last_error"`
	ConnectedAt int64  `json:"connected_at"`
	LastPongAt  int64  `json:"last_pong_at"`
}

type botStatusType struct {
	mu          sync.Mutex
	holding     bool
	state       string
	lastError   string
	connectedAt int64
	lastPongAt  int64
}

var botState = &botStatusType{state: stateDisabled}

func (s *botStatusType) set(state string, lastError string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.lastError = lastError
	if state == stateConnected {
		s.connectedAt = time.Now().Unix()
	}
}

func (s *botStatusType) setHolding(holding bool) {
	s.mu.Lock()
	s.holding = holding
	s.mu.Unlock()
}

func (s *botStatusType) markPong() {
	s.mu.Lock()
	s.lastPongAt = time.Now().Unix()
	s.mu.Unlock()
}

// GetStatus 返回当前长连接状态快照。
func GetStatus() Status {
	setting := operation_setting.GetWecomAIBotSetting()
	s := botState
	s.mu.Lock()
	defer s.mu.Unlock()
	return Status{
		Enabled:     setting.Enabled && setting.ReadyForEnable(),
		Holding:     s.holding,
		State:       s.state,
		LastError:   s.lastError,
		ConnectedAt: s.connectedAt,
		LastPongAt:  s.lastPongAt,
	}
}

var runnerOnce sync.Once

// Start 启动长连接运行器：仅 master 节点参与竞争租约，持有租约的实例与企微
// 维持一条连接；凭证或开关变更在下一个 tick 由配置指纹检测并重建连接。
func Start() {
	runnerOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		runnerID = fmt.Sprintf("%s-%s", common.NodeName, common.GetRandomString(8))
		gopool.Go(runLoop)
	})
}

var runnerID string

func runLoop() {
	ticker := time.NewTicker(runnerTick)
	defer ticker.Stop()

	var (
		holding      bool
		clientCancel context.CancelFunc
		clientDone   chan clientExit
		clientFP     string
		backoffUntil time.Time
	)

	stopClient := func() {
		if clientCancel == nil {
			return
		}
		clientCancel()
		<-clientDone
		clientCancel = nil
		clientDone = nil
	}

	release := func() {
		stopClient()
		_ = model.ReleaseSystemTaskLock(connLockTaskID, runnerID)
		holding = false
		botState.setHolding(false)
	}

	for {
		setting := operation_setting.GetWecomAIBotSetting()
		ready := setting.Enabled && setting.ReadyForEnable()
		// 配置指纹仅含连接凭证：变化即需要重建连接
		fingerprint := fmt.Sprintf("%s|%s", setting.BotID, setting.Secret)

		switch {
		case !ready:
			if holding {
				release()
			} else {
				stopClient()
			}
			botState.set(stateDisabled, "")
		case time.Now().Before(backoffUntil):
			// 被踢/鉴权失败后的冷却期，结束后重新竞争租约
		default:
			if !holding {
				acquired, err := model.AcquireSystemTaskLock(connLockType, connLockTaskID, runnerID, common.GetTimestamp()+connLeaseTTL)
				if err != nil {
					common.SysLog("wecom ai bot lease acquire failed: " + err.Error())
					break
				}
				if !acquired {
					botState.set(stateStandby, "")
					break
				}
				holding = true
				botState.setHolding(true)
			} else if err := model.RenewSystemTaskLock(connLockTaskID, runnerID, common.GetTimestamp()+connLeaseTTL); err != nil {
				common.SysLog("wecom ai bot lease lost: " + err.Error())
				release()
				botState.set(stateStandby, err.Error())
				break
			}
			// 凭证变更时重建连接，让新配置立即生效
			if clientCancel != nil && clientFP != fingerprint {
				stopClient()
			}
			if clientCancel == nil {
				ctx, cancel := context.WithCancel(context.Background())
				clientFP = fingerprint
				clientCancel = cancel
				clientDone = make(chan clientExit, 1)
				bot := &client{credential: botCredential{BotID: setting.BotID, Secret: setting.Secret}}
				gopool.Go(func() { clientDone <- bot.run(ctx) })
			} else {
				// 回收已结束的客户端：被踢或鉴权连续失败时退位并冷却
				select {
				case exit := <-clientDone:
					clientCancel()
					clientCancel = nil
					clientDone = nil
					if exit == exitKicked || exit == exitAuthFailed {
						release()
						if exit == exitKicked {
							backoffUntil = time.Now().Add(kickedBackoff)
							botState.set(stateKicked, "connection was taken over by another client")
						} else {
							backoffUntil = time.Now().Add(authFailBackoff)
							botState.set(stateAuthFailed, "subscribe rejected by wecom server, check BotID/Secret")
						}
					}
				default:
				}
			}
		}
		<-ticker.C
	}
}

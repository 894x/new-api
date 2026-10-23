package wecomaibot

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	sessionKeyPrefix = "wecom_aibot:session:"
	msgIDKeyPrefix   = "wecom_aibot:msgid:"
	toolSlotPrefix   = "wecom_aibot:slot:"
	msgIDDedupTTL    = 24 * time.Hour
)

// chatTurn 会话历史中的一轮 user/assistant 消息。工具调用过程不落会话，
// 避免历史膨胀；多轮指代（如"它的余额"）依靠最终回答中的实体信息即可还原。
type chatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// sessionKey 会话联合键：群聊按 chatid+userid，单聊只有 userid。
func sessionKey(chat *msgCallback) string {
	if chat.ChatType == chatTypeGroup {
		return fmt.Sprintf("%s%s:%s:%s", sessionKeyPrefix, chatTypeGroup, chat.ChatID, chat.From.UserID)
	}
	return fmt.Sprintf("%s%s:%s", sessionKeyPrefix, chatTypeSingle, chat.From.UserID)
}

func loadSession(key string) []chatTurn {
	if common.RedisEnabled {
		data, err := common.RedisGet(key)
		if err != nil || data == "" {
			return nil
		}
		var turns []chatTurn
		if err := common.UnmarshalJsonStr(data, &turns); err != nil {
			return nil
		}
		return turns
	}
	return memorySessions.get(key)
}

// saveSession 保存会话并滑动续期，超过 maxRounds 轮时只保留最近的轮次。
func saveSession(key string, turns []chatTurn, ttl time.Duration, maxRounds int) {
	if maxRounds > 0 && len(turns) > maxRounds*2 {
		turns = turns[len(turns)-maxRounds*2:]
	}
	if common.RedisEnabled {
		encoded, err := common.Marshal(turns)
		if err != nil {
			return
		}
		if err := common.RedisSet(key, string(encoded), ttl); err != nil {
			common.SysLog("wecom ai bot save session failed: " + err.Error())
		}
		return
	}
	memorySessions.set(key, turns, ttl)
}

// acquireMsgID 按 msgid 排重：企微可能因网络原因重复推送同一消息。
// Redis 异常时放行，宁可重复回复也不吞消息。
func acquireMsgID(msgID string) bool {
	if msgID == "" {
		return true
	}
	if common.RedisEnabled {
		ok, err := common.RDB.SetNX(context.Background(), msgIDKeyPrefix+msgID, "1", msgIDDedupTTL).Result()
		if err != nil {
			common.SysLog("wecom ai bot msgid dedup check failed: " + err.Error())
			return true
		}
		return ok
	}
	return memorySlots.setIfAbsent(msgIDKeyPrefix+msgID, msgIDDedupTTL)
}

// AcquireToolSlot 对同一 key 在冷却窗口内只放行一次，限制会真实触达上游的
// 高频工具（如渠道余额实时刷新）的调用频率。
func AcquireToolSlot(key string, cooldown time.Duration) bool {
	full := toolSlotPrefix + key
	if common.RedisEnabled {
		ok, err := common.RDB.SetNX(context.Background(), full, "1", cooldown).Result()
		if err != nil {
			common.SysLog("wecom ai bot tool slot check failed: " + err.Error())
			return true
		}
		return ok
	}
	return memorySlots.setIfAbsent(full, cooldown)
}

// memorySessionStore / memorySlotStore 是未配置 Redis 时的进程内降级存储。
// 长连接由 DB 租约保证单实例持有，内存会话不存在跨实例一致性问题。
type memorySessionStore struct {
	mu      sync.Mutex
	entries map[string]memorySession
}

type memorySession struct {
	turns    []chatTurn
	expireAt time.Time
}

func (s *memorySessionStore) get(key string) []chatTurn {
	memoryCleanOnce.Do(startMemoryCleanup)
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[key]
	if !ok || time.Now().After(entry.expireAt) {
		return nil
	}
	return entry.turns
}

func (s *memorySessionStore) set(key string, turns []chatTurn, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = memorySession{turns: turns, expireAt: time.Now().Add(ttl)}
}

type memorySlotStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func (s *memorySlotStore) setIfAbsent(key string, ttl time.Duration) bool {
	memoryCleanOnce.Do(startMemoryCleanup)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if until, ok := s.entries[key]; ok && now.Before(until) {
		return false
	}
	s.entries[key] = now.Add(ttl)
	return true
}

var (
	memorySessions  = &memorySessionStore{entries: map[string]memorySession{}}
	memorySlots     = &memorySlotStore{entries: map[string]time.Time{}}
	memoryCleanOnce sync.Once
)

// startMemoryCleanup 每小时清理一次过期条目，防止内存降级路径无限增长。
func startMemoryCleanup() {
	gopool.Go(func() {
		for {
			time.Sleep(time.Hour)
			now := time.Now()
			memorySessions.mu.Lock()
			for key, entry := range memorySessions.entries {
				if now.After(entry.expireAt) {
					delete(memorySessions.entries, key)
				}
			}
			memorySessions.mu.Unlock()
			memorySlots.mu.Lock()
			for key, until := range memorySlots.entries {
				if now.After(until) {
					delete(memorySlots.entries, key)
				}
			}
			memorySlots.mu.Unlock()
		}
	})
}

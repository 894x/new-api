package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/dynamicrouting"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
)

const (
	recoveryProbeInterval = dynamicrouting.RecoveryProbeInterval
	recoveryProbeTimeout  = 15 * time.Second
	recoverySampleTTL     = 2 * time.Minute
)

type DynamicRoutingRecoveryProbe func(context.Context, dynamicrouting.ObservationKey) (dynamicrouting.Sample, bool)

type recoveryProbeResult struct {
	ID        string `json:"id"`
	StartedAt int64  `json:"started_at"`
	EndedAt   int64  `json:"ended_at"`
	Success   bool   `json:"success"`
}

type recoveryProbeRunner struct {
	sync.Mutex
	client  *redis.Client
	probe   DynamicRoutingRecoveryProbe
	clock   func() time.Time
	launch  func(func())
	workers chan struct{}
	running map[dynamicrouting.ObservationKey]bool
	next    map[dynamicrouting.ObservationKey]time.Time
	seen    map[dynamicrouting.ObservationKey]int64
}

var recoveryProbeRunnerOnce sync.Once

var recoveryProbeRenewLeaseScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('PEXPIRE', KEYS[1], ARGV[2])
return 1`)

// Publish only while the probe still owns its lease. The remaining lease then
// enforces the same five-second cadence on every relay node.
var recoveryProbePublishScript = redis.NewScript(`
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
redis.call('SET', KEYS[2], ARGV[2], 'PX', ARGV[3])
if tonumber(ARGV[4]) > 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[4])
else
  redis.call('DEL', KEYS[1])
end
return 1`)

// StartDynamicRoutingRecoveryProbes runs on every relay node. Redis leases dedup
// upstream probes and share the health-only feedback with the other nodes.
func StartDynamicRoutingRecoveryProbes(probe DynamicRoutingRecoveryProbe) {
	recoveryProbeRunnerOnce.Do(func() {
		runner := &recoveryProbeRunner{
			probe: probe, clock: time.Now, launch: gopool.Go,
			workers: make(chan struct{}, 4), running: make(map[dynamicrouting.ObservationKey]bool),
			next: make(map[dynamicrouting.ObservationKey]time.Time), seen: make(map[dynamicrouting.ObservationKey]int64),
		}
		if common.RedisEnabled {
			runner.client = common.RDB
		}
		gopool.Go(func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for now := range ticker.C {
				dynamicRoutingRuntime.Lock()
				controller := dynamicRoutingRuntime.controller
				dynamicRoutingRuntime.Unlock()
				if controller != nil {
					runner.tick(context.Background(), controller, now)
				}
			}
		})
	})
}

func recoveryProbeKeys(key dynamicrouting.ObservationKey) (string, string) {
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", key.ChannelID, key.Model)))
	base := fmt.Sprintf("new-api:routing-recovery:v1:{%x}", hash)
	return base + ":lease", base + ":sample"
}

func (r *recoveryProbeRunner) tick(ctx context.Context, controller *dynamicrouting.Controller, now time.Time) {
	targets := controller.RecoveryTargets(now)
	active := make(map[dynamicrouting.ObservationKey]bool, len(targets))
	for _, target := range targets {
		active[target.ObservationKey] = true
	}
	r.Lock()
	for key := range r.next {
		if !active[key] && !r.running[key] {
			delete(r.next, key)
		}
	}
	for key := range r.seen {
		if !active[key] && !r.running[key] {
			delete(r.seen, key)
		}
	}
	r.Unlock()
	// Read the latest cluster result before deciding whether another probe is due.
	// A failed Redis read fails closed instead of producing duplicate probes.
	if r.client != nil {
		for _, target := range targets {
			_, sampleKey := recoveryProbeKeys(target.ObservationKey)
			readCtx, cancel := context.WithTimeout(ctx, time.Second)
			payload, err := r.client.GetRange(readCtx, sampleKey, 0, 4096).Result()
			cancel()
			if err != nil {
				return
			}
			if payload == "" || len(payload) > 4096 {
				continue
			}
			var result recoveryProbeResult
			if common.UnmarshalJsonStr(payload, &result) != nil || result.ID == "" || result.StartedAt > result.EndedAt || result.EndedAt < now.Add(-recoverySampleTTL).UnixMilli() || result.EndedAt > now.Add(5*time.Second).UnixMilli() {
				continue
			}
			r.applyResult(controller, target.ObservationKey, result)
		}
		targets = controller.RecoveryTargets(now)
	}
	for _, target := range targets {
		key := target.ObservationKey
		r.Lock()
		if now.Before(r.next[key]) || r.running[key] {
			r.Unlock()
			continue
		}
		select {
		case r.workers <- struct{}{}:
		default:
			r.Unlock()
			return
		}
		r.running[key] = true
		r.next[key] = now.Add(recoveryProbeInterval)
		r.Unlock()
		token := uuid.NewString()
		if r.client != nil {
			leaseKey, _ := recoveryProbeKeys(key)
			leaseCtx, cancel := context.WithTimeout(ctx, time.Second)
			acquired, err := r.client.SetNX(leaseCtx, leaseKey, token, recoveryProbeTimeout+recoveryProbeInterval).Result()
			cancel()
			if err != nil || !acquired {
				r.Lock()
				delete(r.running, key)
				r.Unlock()
				<-r.workers
				continue
			}
		}
		r.launch(func() { r.run(ctx, controller, key, token) })
	}
}

func (r *recoveryProbeRunner) run(ctx context.Context, controller *dynamicrouting.Controller, key dynamicrouting.ObservationKey, token string) {
	defer func() {
		r.Lock()
		delete(r.running, key)
		if completed := r.clock(); r.next[key].Before(completed) {
			r.next[key] = completed
		}
		r.Unlock()
		<-r.workers
	}()
	if r.client != nil {
		leaseKey, _ := recoveryProbeKeys(key)
		leaseCtx, cancel := context.WithTimeout(ctx, time.Second)
		renewed, err := recoveryProbeRenewLeaseScript.Run(leaseCtx, r.client, []string{leaseKey}, token, (recoveryProbeTimeout + recoveryProbeInterval).Milliseconds()).Int()
		cancel()
		if err != nil || renewed != 1 {
			return
		}
	}
	probeCtx, cancel := context.WithTimeout(ctx, recoveryProbeTimeout)
	sample, observed := r.probe(probeCtx, key)
	cancel()
	if !observed || ctx.Err() != nil {
		return
	}
	result := recoveryProbeResult{ID: token, StartedAt: sample.UpstreamStartedAt.UnixMilli(), EndedAt: sample.ObservedAt.UnixMilli(), Success: sample.Success && !sample.HardFailure}
	if r.client != nil {
		payload, err := common.Marshal(result)
		if err != nil {
			return
		}
		leaseKey, sampleKey := recoveryProbeKeys(key)
		writeCtx, cancel := context.WithTimeout(ctx, time.Second)
		remainingInterval := max(0, recoveryProbeInterval.Milliseconds()-(result.EndedAt-result.StartedAt))
		written, err := recoveryProbePublishScript.Run(writeCtx, r.client, []string{leaseKey, sampleKey}, token, string(payload), recoverySampleTTL.Milliseconds(), remainingInterval).Int()
		cancel()
		if err != nil || written != 1 {
			return
		}
	}
	r.applyResult(controller, key, result)
}

func (r *recoveryProbeRunner) applyResult(controller *dynamicrouting.Controller, key dynamicrouting.ObservationKey, result recoveryProbeResult) {
	r.Lock()
	defer r.Unlock()
	if result.EndedAt <= r.seen[key] {
		return
	}
	r.seen[key] = result.EndedAt
	controller.Observe(key, dynamicrouting.Sample{
		RecoveryProbe: true, ObservedAt: time.UnixMilli(result.EndedAt), UpstreamStartedAt: time.UnixMilli(result.StartedAt),
		Success: result.Success, HardFailure: !result.Success,
	})
}

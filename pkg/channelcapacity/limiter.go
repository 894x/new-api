package channelcapacity

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
)

const windowDuration = time.Minute

type LimitKind string

const (
	LimitNone LimitKind = ""
	LimitRPM  LimitKind = "rpm"
	LimitTPM  LimitKind = "tpm"
	LimitBoth LimitKind = "rpm_tpm"
)

type Key struct {
	ChannelID int
	Model     string
}

type Limits struct {
	RPM int64
	TPM int64
}

type Decision struct {
	Allowed    bool
	LimitedBy  LimitKind
	UsedRPM    int64
	UsedTPM    int64
	RetryAfter time.Duration
}

type Limiter interface {
	Acquire(ctx context.Context, key Key, limits Limits, tokens int64, now time.Time) (Decision, error)
}

type minuteUsage struct {
	window int64
	rpm    int64
	tpm    int64
}

type MemoryLimiter struct {
	mu              sync.Mutex
	usage           map[Key]minuteUsage
	lastSweepWindow int64
}

func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{usage: make(map[Key]minuteUsage)}
}

func (l *MemoryLimiter) Acquire(ctx context.Context, key Key, limits Limits, tokens int64, now time.Time) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}
	if err := validateAcquire(key, limits, tokens); err != nil {
		return Decision{}, err
	}
	if limits.RPM == 0 && limits.TPM == 0 {
		return Decision{Allowed: true}, nil
	}

	window := now.Unix() / int64(windowDuration/time.Second)
	l.mu.Lock()
	defer l.mu.Unlock()
	if window > l.lastSweepWindow {
		for usageKey, previous := range l.usage {
			if previous.window < window {
				delete(l.usage, usageKey)
			}
		}
		l.lastSweepWindow = window
	}

	usage := l.usage[key]
	if usage.window != window {
		usage = minuteUsage{window: window}
	}
	rpmExceeded := exceeds(limits.RPM, usage.rpm, 1)
	tpmExceeded := exceeds(limits.TPM, usage.tpm, tokens)
	if rpmExceeded || tpmExceeded {
		return Decision{
			Allowed:    false,
			LimitedBy:  limitedBy(rpmExceeded, tpmExceeded),
			UsedRPM:    usage.rpm,
			UsedTPM:    usage.tpm,
			RetryAfter: retryAfter(now),
		}, nil
	}

	if limits.RPM > 0 {
		usage.rpm++
	}
	if limits.TPM > 0 {
		usage.tpm += tokens
	}
	l.usage[key] = usage
	return Decision{
		Allowed:    true,
		UsedRPM:    usage.rpm,
		UsedTPM:    usage.tpm,
		RetryAfter: retryAfter(now),
	}, nil
}

func validateAcquire(key Key, limits Limits, tokens int64) error {
	if key.ChannelID <= 0 {
		return errors.New("channel id must be positive")
	}
	if strings.TrimSpace(key.Model) == "" {
		return errors.New("model must not be empty")
	}
	if limits.RPM < 0 || limits.TPM < 0 {
		return errors.New("capacity limits must not be negative")
	}
	if tokens < 0 {
		return errors.New("token reservation must not be negative")
	}
	return nil
}

func exceeds(limit int64, used int64, requested int64) bool {
	if limit == 0 {
		return false
	}
	return requested > limit || used > limit-requested
}

func limitedBy(rpmExceeded bool, tpmExceeded bool) LimitKind {
	if rpmExceeded && tpmExceeded {
		return LimitBoth
	}
	if rpmExceeded {
		return LimitRPM
	}
	return LimitTPM
}

func retryAfter(now time.Time) time.Duration {
	windowSeconds := int64(windowDuration / time.Second)
	nextWindow := time.Unix((now.Unix()/windowSeconds+1)*windowSeconds, 0)
	remaining := nextWindow.Sub(now)
	if remaining <= 0 {
		return time.Second
	}
	return remaining
}

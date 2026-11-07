package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const channelRetryPolicyContextKey = "user_channel_retry_times"

// ApplyChannelRetryPolicy selects the retry budget of the channel being tried.
// Counts exclude the first attempt and do not restart on channel changes within a group.
// Selection, status-code eligibility and channel constraints remain independent.
func (p *RetryParam) ApplyChannelRetryPolicy(group string, channelID int) error {
	if group == "auto" {
		group = common.GetContextKeyString(p.Ctx, constant.ContextKeyAutoGroup)
	}
	routing, err := p.UserChannelRoutingForGroup(group)
	if err != nil {
		return err
	}
	p.retryTimesOverride = routing[channelID].RetryTimes
	if p.Ctx != nil {
		p.Ctx.Set(channelRetryPolicyContextKey, p.retryTimesOverride)
	}
	return nil
}

func (p *RetryParam) MaxRetries() int {
	if p.retryTimesOverride != nil {
		return *p.retryTimesOverride
	}
	return common.RetryTimes
}

// ChannelRetryOverride distinguishes inheritance from explicitly disabling
// retries with zero. A channel switch must replace even a previous override.
func ChannelRetryOverride(c *gin.Context) (int, bool) {
	if c == nil {
		return 0, false
	}
	value, ok := c.Get(channelRetryPolicyContextKey)
	limit, typed := value.(*int)
	if !ok || !typed || limit == nil {
		return 0, false
	}
	return *limit, true
}

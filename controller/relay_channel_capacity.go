package controller

import (
	"errors"
	"math"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func channelCapacityAPIError(c *gin.Context, err error) *types.NewAPIError {
	var capacityErr *service.ChannelModelCapacityError
	if errors.As(err, &capacityErr) {
		c.Header("Retry-After", strconv.Itoa(max(1, int(math.Ceil(capacityErr.RetryAfter.Seconds())))))
		return types.NewErrorWithStatusCode(err, types.ErrorCodeChannelModelCapacityExhausted, http.StatusTooManyRequests, types.ErrOptionWithSkipRetry())
	}
	return types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
}

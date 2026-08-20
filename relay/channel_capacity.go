package relay

import (
	"errors"
	"io"
	"net/http"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func admitFinalChannelModelCapacity(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) *types.NewAPIError {
	err := service.AdmitFinalChannelModelCapacity(c, info, body)
	if err == nil {
		return nil
	}
	var capacityErr *service.ChannelModelCapacityAdmissionError
	if errors.As(err, &capacityErr) {
		return types.NewErrorWithStatusCode(
			err,
			types.ErrorCodeChannelModelCapacityExhausted,
			http.StatusTooManyRequests,
			types.ErrOptionWithSkipRetry(),
		)
	}
	return types.NewError(err, types.ErrorCodeCountTokenFailed, types.ErrOptionWithSkipRetry())
}

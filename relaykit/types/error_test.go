package types

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseCommittedErrorCannotRetry(t *testing.T) {
	err := NewOpenAIError(
		errors.New("upstream stream failed after output"),
		ErrorCodeBadResponse,
		http.StatusBadGateway,
		ErrOptionWithResponseCommitted(),
	)

	require.NotNil(t, err)
	assert.True(t, IsResponseCommittedError(err))
	assert.True(t, IsSkipRetryError(err))
}

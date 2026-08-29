package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionMapUpdatesModelRequestRateLimitTPM(t *testing.T) {
	previous := setting.ModelRequestRateLimitTPM
	previousOptionMap := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() {
		setting.ModelRequestRateLimitTPM = previous
		common.OptionMap = previousOptionMap
	})

	require.NoError(t, updateOptionMap("ModelRequestRateLimitTPM", "60000"))
	assert.Equal(t, 60000, setting.ModelRequestRateLimitTPM)
}

func TestValidateOptionValueRejectsInvalidModelRequestRateLimitTPM(t *testing.T) {
	for _, value := range []string{"-1", "1.5", "2147483648", "invalid"} {
		t.Run(value, func(t *testing.T) {
			assert.Error(t, validateOptionValue("ModelRequestRateLimitTPM", value))
		})
	}
}

func TestValidateOptionValueAcceptsModelRequestRateLimitTPMBounds(t *testing.T) {
	for _, value := range []string{"0", "2147483647"} {
		t.Run(value, func(t *testing.T) {
			assert.NoError(t, validateOptionValue("ModelRequestRateLimitTPM", value))
		})
	}
}

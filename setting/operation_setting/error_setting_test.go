package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorDetailsAreHiddenByDefault(t *testing.T) {
	assert.True(t, ShouldHideErrorDetails())
}

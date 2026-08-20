package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSeedsRequiresAReproducibleNonEmptyList(t *testing.T) {
	seeds, err := parseSeeds("3, 7,11")
	require.NoError(t, err)
	assert.Equal(t, []int64{3, 7, 11}, seeds)

	_, err = parseSeeds("")
	assert.Error(t, err)
	_, err = parseSeeds("3,broken")
	assert.Error(t, err)
}

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupModelChannelGroupsPreservesExplicitDenyAndRejectsMalformedUpdates(t *testing.T) {
	original := GroupModelChannelGroupsJSON()
	t.Cleanup(func() { require.NoError(t, UpdateGroupModelChannelGroups(original)) })
	const valid = `{"enterprise":{"model-a":["official","preferred"],"model-b":[]}}`
	require.NoError(t, UpdateGroupModelChannelGroups(valid))
	groups, configured := GetGroupModelChannelGroups("enterprise", "model-a")
	assert.True(t, configured)
	assert.Equal(t, []string{"official", "preferred"}, groups)
	groups[0] = "mutated"
	groups, _ = GetGroupModelChannelGroups("enterprise", "model-a")
	assert.Equal(t, "official", groups[0])
	groups, configured = GetGroupModelChannelGroups("enterprise", "model-b")
	assert.True(t, configured)
	assert.Empty(t, groups)
	_, configured = GetGroupModelChannelGroups("enterprise", "unconfigured")
	assert.False(t, configured)

	for _, value := range []string{
		`null`, `[]`, `{"enterprise":null}`, `{"enterprise":{"m":null}}`,
		`{"enterprise":{"m":"official"}}`, `{"enterprise":{"m":["official","official"]}}`,
		`{"enterprise":{"m":["auto"]}}`, `{"enterprise":{" m":["official"]}}`,
		`{"auto":{"m":[]}}`, `{"__proto__":{"m":[]}}`,
	} {
		t.Run(value, func(t *testing.T) {
			require.Error(t, UpdateGroupModelChannelGroups(value))
			assert.JSONEq(t, valid, GroupModelChannelGroupsJSON())
		})
	}
}

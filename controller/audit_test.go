package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAuditContentENRendersAssetLibraryOperations(t *testing.T) {
	testCases := []struct {
		action   string
		params   map[string]interface{}
		expected string
	}{
		{
			action:   "asset_library.group.create",
			params:   map[string]interface{}{"id": "group-na-1", "name": "Characters"},
			expected: "Created asset group Characters (ID: group-na-1)",
		},
		{
			action:   "asset_library.group.update",
			params:   map[string]interface{}{"id": "group-na-1", "name": "Heroes"},
			expected: "Updated asset group Heroes (ID: group-na-1)",
		},
		{
			action:   "asset_library.group.delete",
			params:   map[string]interface{}{"id": "group-na-1", "name": "Heroes"},
			expected: "Deleted asset group Heroes (ID: group-na-1)",
		},
		{
			action: "asset_library.asset.create",
			params: map[string]interface{}{
				"id": "asset-na-1", "asset_type": "Image", "group_id": "group-na-1",
			},
			expected: "Created asset (ID: asset-na-1, type: Image, group: group-na-1)",
		},
		{
			action: "asset_library.asset.update",
			params: map[string]interface{}{
				"id": "asset-na-1", "asset_type": "Image", "group_id": "group-na-1",
			},
			expected: "Updated asset (ID: asset-na-1, type: Image, group: group-na-1)",
		},
		{
			action: "asset_library.asset.delete",
			params: map[string]interface{}{
				"id": "asset-na-1", "asset_type": "Image", "group_id": "group-na-1",
			},
			expected: "Deleted asset (ID: asset-na-1, type: Image, group: group-na-1)",
		},
		{
			action:   "asset_library.asset.sync",
			params:   map[string]interface{}{"id": "asset-na-1", "error_count": 1},
			expected: "Synchronized asset asset-na-1 (errors: 1)",
		},
		{
			action:   "asset_library.group.sync",
			params:   map[string]interface{}{"id": "group-na-1", "error_count": 2},
			expected: "Synchronized asset group group-na-1 (errors: 2)",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.action, func(t *testing.T) {
			assert.Equal(t, testCase.expected, auditContentEN(testCase.action, testCase.params))
		})
	}
}

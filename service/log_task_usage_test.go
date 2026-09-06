package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnrichTaskUsageLogsAddsFinalInputAndOutputUsage(t *testing.T) {
	truncate(t)
	task := &model.Task{
		TaskID: "task_usage_log",
		UserId: 1,
		PrivateData: model.TaskPrivateData{Usage: &hosttypes.TaskUsage{
			Kind:   hosttypes.TaskUsageKindVideoDuration,
			Unit:   hosttypes.TaskUsageUnitSecond,
			Input:  2.5,
			Output: 5,
			Total:  7.5,
		}},
	}
	require.NoError(t, model.DB.Create(task).Error)
	logs := []*model.Log{{
		UserId: 1,
		Other:  `{"is_task":true,"task_id":"task_usage_log"}`,
	}, {
		UserId: 2,
		Other:  `{"is_task":true,"task_id":"task_usage_log"}`,
	}}

	require.NoError(t, EnrichTaskUsageLogs(logs))

	var other struct {
		TaskUsage *hosttypes.TaskUsage `json:"task_usage"`
	}
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
	require.NotNil(t, other.TaskUsage)
	assert.Equal(t, 2.5, other.TaskUsage.Input)
	assert.Equal(t, 5.0, other.TaskUsage.Output)

	var otherUser struct {
		TaskUsage *hosttypes.TaskUsage `json:"task_usage"`
	}
	require.NoError(t, common.UnmarshalJsonStr(logs[1].Other, &otherUser))
	assert.Nil(t, otherUser.TaskUsage)
}

func TestLogTaskConsumptionLinksTheConsumeLogToItsTask(t *testing.T) {
	truncate(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/services/aigc/video-generation/video-synthesis", nil)
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_linked_usage",
		},
		ChannelMeta:     &relaycommon.ChannelMeta{},
		OriginModelName: "wan3.0-video",
		UserId:          1,
		PriceData: hosttypes.PriceData{
			Quota: 10,
			GroupRatioInfo: hosttypes.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}

	LogTaskConsumption(c, info)

	var log model.Log
	require.NoError(t, model.LOG_DB.Order("id DESC").First(&log).Error)
	var other struct {
		TaskID string `json:"task_id"`
	}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	assert.Equal(t, "task_linked_usage", other.TaskID)
}

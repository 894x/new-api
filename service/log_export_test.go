package service

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamLogExportKeepsEveryChannelAttempt(t *testing.T) {
	logs := []*model.Log{
		{
			Id:               1,
			CreatedAt:        1_700_000_001,
			Type:             model.LogTypeError,
			RequestId:        "req-1",
			ChannelId:        10,
			ChannelName:      "supplier-a",
			ModelName:        "client-model",
			PromptTokens:     999,
			CompletionTokens: 888,
			Quota:            777,
			Other: common.MapToJsonStr(map[string]interface{}{
				"upstream_model_name": "provider-model",
				"cache_tokens":        666,
				"group_ratio":         2.0,
				"model_ratio":         3.0,
			}),
		},
		{
			Id:                2,
			CreatedAt:         1_700_000_002,
			Type:              model.LogTypeConsume,
			RequestId:         "req-1",
			UpstreamRequestId: "upstream-2",
			ChannelId:         12,
			ChannelName:       "supplier-b",
			ModelName:         "client-model",
			PromptTokens:      120,
			CompletionTokens:  30,
			Quota:             1500,
			Other: common.MapToJsonStr(map[string]interface{}{
				"cache_tokens":        20,
				"model_ratio":         2.0,
				"completion_ratio":    3.0,
				"cache_ratio":         0.5,
				"group_ratio":         1.5,
				"quota_per_unit":      500000.0,
				"upstream_model_name": "provider-model",
			}),
		},
	}

	rows := BuildLogExportRows(LogExportViewUpstream, logs)

	require.Len(t, rows, 2)
	assert.Equal(t, "failed", rows[0].Status)
	assert.Equal(t, 10, rows[0].ChannelID)
	assert.Nil(t, rows[0].OriginalAmountUSD)
	assert.Zero(t, rows[0].InputTokens)
	assert.Zero(t, rows[0].CachedInputTokens)
	assert.Zero(t, rows[0].OutputTokens)
	assert.Nil(t, rows[0].GroupRatio)
	assert.Nil(t, rows[0].ModelRatio)
	assert.Equal(t, "success", rows[1].Status)
	assert.Equal(t, 12, rows[1].ChannelID)
	assert.Equal(t, 20, rows[1].CachedInputTokens)
	require.NotNil(t, rows[1].OriginalAmountUSD)
	assert.InDelta(t, 0.002, *rows[1].OriginalAmountUSD, 0.0000001)
}

func TestBuildLogExportRowsDoesNotUseCurrentQuotaPerUnitForHistoricalPrice(t *testing.T) {
	previousQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 1
	t.Cleanup(func() { common.QuotaPerUnit = previousQuotaPerUnit })

	withoutSnapshot := &model.Log{
		Id:        1,
		Type:      model.LogTypeConsume,
		Quota:     1000,
		RequestId: "missing-snapshot",
		Other: common.MapToJsonStr(map[string]interface{}{
			"group_ratio": 1.0,
		}),
	}
	withSnapshot := &model.Log{
		Id:        2,
		Type:      model.LogTypeConsume,
		Quota:     1000,
		RequestId: "stored-snapshot",
		Other: common.MapToJsonStr(map[string]interface{}{
			"group_ratio":    1.0,
			"quota_per_unit": 500000.0,
		}),
	}

	rows := BuildLogExportRows(LogExportViewUpstream, []*model.Log{withoutSnapshot, withSnapshot})

	require.Len(t, rows, 2)
	assert.Nil(t, rows[0].OriginalAmountUSD)
	require.NotNil(t, rows[1].OriginalAmountUSD)
	assert.InDelta(t, 0.002, *rows[1].OriginalAmountUSD, 0.0000001)
}

func TestBuildUpstreamLogExportUsesActualUpstreamModelForSummary(t *testing.T) {
	logs := []*model.Log{
		{Id: 1, Type: model.LogTypeConsume, ChannelId: 10, ModelName: "client-model"},
		{Id: 2, Type: model.LogTypeConsume, ChannelId: 10, ModelName: "client-model", Other: common.MapToJsonStr(map[string]interface{}{"upstream_model_name": "mapped-model"})},
	}

	rows := BuildLogExportRows(LogExportViewUpstream, logs)
	summary := BuildLogExportSummary(LogExportViewUpstream, rows)

	require.Len(t, rows, 2)
	assert.Equal(t, "client-model", rows[0].UpstreamModelName)
	require.Len(t, summary, 2)
	assert.Equal(t, "client-model", summary[0].ModelName)
	assert.Equal(t, "mapped-model", summary[1].ModelName)
}

func TestBuildUpstreamLogExportDoesNotInventMissingErrorModel(t *testing.T) {
	logs := []*model.Log{
		{Id: 1, Type: model.LogTypeError, ChannelId: 10, ModelName: "client-model"},
		{Id: 2, Type: model.LogTypeError, ChannelId: 10, ModelName: "client-model", Other: common.MapToJsonStr(map[string]interface{}{"upstream_model_name": "mapped-model"})},
	}

	rows := BuildLogExportRows(LogExportViewUpstream, logs)

	require.Len(t, rows, 2)
	assert.Empty(t, rows[0].UpstreamModelName)
	assert.Equal(t, "mapped-model", rows[1].UpstreamModelName)
}

func TestBuildDownstreamLogExportKeepsOnlyTheFinalRequestResult(t *testing.T) {
	logs := []*model.Log{
		{Id: 1, CreatedAt: 10, Type: model.LogTypeError, RequestId: "req-success", ChannelId: 10, UpstreamRequestId: "hidden-1"},
		{Id: 2, CreatedAt: 11, Type: model.LogTypeConsume, RequestId: "req-success", UserId: 7, Username: "alice", ModelName: "gpt-x", PromptTokens: 10, CompletionTokens: 2},
		{Id: 3, CreatedAt: 12, Type: model.LogTypeError, RequestId: "req-failed", UserId: 8, Username: "bob", ChannelId: 12, UpstreamRequestId: "hidden-2", Content: "provider-secret-error"},
	}

	rows := BuildLogExportRows(LogExportViewDownstream, logs)

	require.Len(t, rows, 2)
	assert.Equal(t, "req-success", rows[0].RequestID)
	assert.Equal(t, "success", rows[0].Status)
	assert.Zero(t, rows[0].ChannelID)
	assert.Empty(t, rows[0].ChannelName)
	assert.Empty(t, rows[0].UpstreamRequestID)
	assert.Empty(t, rows[0].UpstreamModelName)
	assert.Equal(t, "req-failed", rows[1].RequestID)
	assert.Equal(t, "failed", rows[1].Status)
	assert.Zero(t, rows[1].ChannelID)
	assert.Empty(t, rows[1].ErrorMessage)
}

func TestBuildDownstreamLogExportDoesNotTreatViolationFeeAsSuccess(t *testing.T) {
	logs := []*model.Log{
		{Id: 1, CreatedAt: 10, Type: model.LogTypeError, RequestId: "req-1", UserId: 7, Username: "alice"},
		{
			Id:        2,
			CreatedAt: 11,
			Type:      model.LogTypeConsume,
			RequestId: "req-1",
			Quota:     500,
			Other: common.MapToJsonStr(map[string]interface{}{
				"violation_fee": true,
			}),
		},
	}

	rows := BuildLogExportRows(LogExportViewDownstream, logs)

	require.Len(t, rows, 1)
	assert.Equal(t, "failed", rows[0].Status)
}

func TestBuildDownstreamLogExportPrefersSuccessfulTerminalRecordWithinSameSecond(t *testing.T) {
	logs := []*model.Log{
		{Id: 1, CreatedAt: 10, Type: model.LogTypeConsume, RequestId: "req-1"},
		{Id: 2, CreatedAt: 10, Type: model.LogTypeError, RequestId: "req-1"},
	}

	rows := BuildLogExportRows(LogExportViewDownstream, logs)

	require.Len(t, rows, 1)
	assert.Equal(t, "success", rows[0].Status)
}

func TestBuildLogExportSummaryTotalsUsageAndKnownOriginalPrice(t *testing.T) {
	knownAmount := 0.12
	rows := []LogExportRow{
		{ChannelID: 10, ChannelName: "supplier-a", ModelName: "gpt-x", Status: "success", InputTokens: 100, CachedInputTokens: 40, OutputTokens: 20, OriginalAmountUSD: &knownAmount},
		{ChannelID: 10, ChannelName: "supplier-a", ModelName: "gpt-x", Status: "success", InputTokens: 10},
		{ChannelID: 10, ChannelName: "supplier-a", ModelName: "gpt-x", Status: "failed"},
	}

	summary := BuildLogExportSummary(LogExportViewUpstream, rows)

	require.Len(t, summary, 1)
	assert.Equal(t, 3, summary[0].RequestCount)
	assert.Equal(t, 2, summary[0].SuccessCount)
	assert.Equal(t, 1, summary[0].FailureCount)
	assert.Equal(t, 110, summary[0].InputTokens)
	assert.Equal(t, 40, summary[0].CachedInputTokens)
	assert.Equal(t, 20, summary[0].OutputTokens)
	assert.Equal(t, 1, summary[0].PricedRequestCount)
	assert.Equal(t, 1, summary[0].UnpricedRequestCount)
	assert.InDelta(t, 0.12, summary[0].KnownOriginalAmountUSD, 0.0000001)
}

func TestWriteLogExportArchiveUsesSelectedFieldsAndSafeCSVCells(t *testing.T) {
	rows := []LogExportRow{
		{
			CreatedAt:   1_700_000_000,
			RequestID:   "=formula",
			ChannelID:   10,
			ChannelName: "supplier-a",
			Status:      "success",
		},
		{
			CreatedAt: 1_700_000_001,
			RequestID: "\t=hidden-formula",
			ChannelID: 11,
			Status:    "failed",
		},
	}
	summary := BuildLogExportSummary(LogExportViewUpstream, rows)
	var output bytes.Buffer

	err := WriteLogExportArchive(&output, LogExportViewUpstream, []string{"request_id", "channel_id", "status"}, rows, summary)

	require.NoError(t, err)
	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	require.NoError(t, err)
	require.Len(t, reader.File, 2)
	detail, err := reader.File[0].Open()
	require.NoError(t, err)
	defer detail.Close()
	detailBytes, err := io.ReadAll(detail)
	require.NoError(t, err)
	assert.Equal(t, []byte{0xEF, 0xBB, 0xBF}, detailBytes[:3])
	assert.Contains(t, string(detailBytes), "request_id,channel_id,status")
	assert.Contains(t, string(detailBytes), "'=formula,10,success")
	assert.Contains(t, string(detailBytes), "'\t=hidden-formula,11,failed")
}

func TestValidateLogExportFieldsRejectsUpstreamDataInDownstreamExport(t *testing.T) {
	err := ValidateLogExportFields(LogExportViewDownstream, []string{"request_id", "channel_id"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel_id")

	err = ValidateLogExportFields(LogExportViewDownstream, []string{"request_id", "error_message"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error_message")
}

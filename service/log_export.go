package service

import (
	"archive/zip"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type LogExportView string

const (
	LogExportViewUpstream   LogExportView = "upstream"
	LogExportViewDownstream LogExportView = "downstream"
)

type LogExportRow struct {
	CreatedAt         int64
	RequestID         string
	UpstreamRequestID string
	UserID            int
	Username          string
	TokenID           int
	TokenName         string
	Group             string
	ChannelID         int
	ChannelName       string
	Status            string
	ModelName         string
	UpstreamModelName string
	InputTokens       int
	CachedInputTokens int
	OutputTokens      int
	ModelPrice        *float64
	ModelRatio        *float64
	CompletionRatio   *float64
	CacheRatio        *float64
	GroupRatio        *float64
	QuotaPerUnit      *float64
	OriginalAmountUSD *float64
	ErrorMessage      string
}

type LogExportSummary struct {
	UserID                 int
	Username               string
	ChannelID              int
	ChannelName            string
	ModelName              string
	RequestCount           int
	SuccessCount           int
	FailureCount           int
	InputTokens            int
	CachedInputTokens      int
	OutputTokens           int
	PricedRequestCount     int
	UnpricedRequestCount   int
	KnownOriginalAmountUSD float64
}

type logExportField struct {
	upstreamOnly bool
	text         bool
	value        func(LogExportRow) string
}

var logExportFields = map[string]logExportField{
	"created_at":          {text: true, value: func(row LogExportRow) string { return time.Unix(row.CreatedAt, 0).UTC().Format(time.RFC3339) }},
	"request_id":          {text: true, value: func(row LogExportRow) string { return row.RequestID }},
	"upstream_request_id": {upstreamOnly: true, text: true, value: func(row LogExportRow) string { return row.UpstreamRequestID }},
	"status":              {text: true, value: func(row LogExportRow) string { return row.Status }},
	"user_id":             {value: func(row LogExportRow) string { return strconv.Itoa(row.UserID) }},
	"username":            {text: true, value: func(row LogExportRow) string { return row.Username }},
	"token_id":            {value: func(row LogExportRow) string { return strconv.Itoa(row.TokenID) }},
	"token_name":          {text: true, value: func(row LogExportRow) string { return row.TokenName }},
	"group":               {text: true, value: func(row LogExportRow) string { return row.Group }},
	"channel_id":          {upstreamOnly: true, value: func(row LogExportRow) string { return strconv.Itoa(row.ChannelID) }},
	"channel_name":        {upstreamOnly: true, text: true, value: func(row LogExportRow) string { return row.ChannelName }},
	"model_name":          {text: true, value: func(row LogExportRow) string { return row.ModelName }},
	"upstream_model_name": {upstreamOnly: true, text: true, value: func(row LogExportRow) string { return row.UpstreamModelName }},
	"input_tokens":        {value: func(row LogExportRow) string { return strconv.Itoa(row.InputTokens) }},
	"cached_input_tokens": {value: func(row LogExportRow) string { return strconv.Itoa(row.CachedInputTokens) }},
	"output_tokens":       {value: func(row LogExportRow) string { return strconv.Itoa(row.OutputTokens) }},
	"model_price":         {value: func(row LogExportRow) string { return formatOptionalFloat(row.ModelPrice) }},
	"model_ratio":         {value: func(row LogExportRow) string { return formatOptionalFloat(row.ModelRatio) }},
	"completion_ratio":    {value: func(row LogExportRow) string { return formatOptionalFloat(row.CompletionRatio) }},
	"cache_ratio":         {value: func(row LogExportRow) string { return formatOptionalFloat(row.CacheRatio) }},
	"group_ratio":         {value: func(row LogExportRow) string { return formatOptionalFloat(row.GroupRatio) }},
	"quota_per_unit":      {value: func(row LogExportRow) string { return formatOptionalFloat(row.QuotaPerUnit) }},
	"original_amount_usd": {value: func(row LogExportRow) string { return formatLogExportAmount(row.OriginalAmountUSD) }},
	"error_message":       {upstreamOnly: true, text: true, value: func(row LogExportRow) string { return row.ErrorMessage }},
}

func DefaultLogExportFields(view LogExportView) []string {
	commonFields := []string{
		"created_at", "request_id", "status", "user_id", "username", "token_id", "token_name", "group",
		"model_name", "input_tokens", "cached_input_tokens", "output_tokens", "model_price", "model_ratio",
		"completion_ratio", "cache_ratio", "group_ratio", "quota_per_unit", "original_amount_usd",
	}
	if view != LogExportViewUpstream {
		return commonFields
	}
	return []string{
		"created_at", "request_id", "upstream_request_id", "status", "user_id", "username", "token_id", "token_name", "group",
		"channel_id", "channel_name", "model_name", "upstream_model_name", "input_tokens", "cached_input_tokens", "output_tokens",
		"model_price", "model_ratio", "completion_ratio", "cache_ratio", "group_ratio", "quota_per_unit", "original_amount_usd", "error_message",
	}
}

func ValidateLogExportFields(view LogExportView, fields []string) error {
	if view != LogExportViewUpstream && view != LogExportViewDownstream {
		return fmt.Errorf("unsupported export view %q", view)
	}
	if len(fields) == 0 {
		return errors.New("at least one export field is required")
	}
	seen := make(map[string]struct{}, len(fields))
	for _, name := range fields {
		field, ok := logExportFields[name]
		if !ok {
			return fmt.Errorf("unsupported export field %q", name)
		}
		if view == LogExportViewDownstream && field.upstreamOnly {
			return fmt.Errorf("field %q is not available in downstream exports", name)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate export field %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func BuildLogExportRows(view LogExportView, logs []*model.Log) []LogExportRow {
	orderedLogs := append([]*model.Log(nil), logs...)
	sort.SliceStable(orderedLogs, func(i, j int) bool {
		if orderedLogs[i].CreatedAt != orderedLogs[j].CreatedAt {
			return orderedLogs[i].CreatedAt < orderedLogs[j].CreatedAt
		}
		return orderedLogs[i].Id < orderedLogs[j].Id
	})

	if view == LogExportViewUpstream {
		rows := make([]LogExportRow, 0, len(orderedLogs))
		for _, log := range orderedLogs {
			if !isExportableUsageLog(log) || isViolationFeeLog(log) {
				continue
			}
			rows = append(rows, buildLogExportRow(log, false))
		}
		return rows
	}

	finalLogs := make(map[string]*model.Log)
	requestOrder := make([]string, 0)
	for _, log := range orderedLogs {
		if !isExportableUsageLog(log) || isViolationFeeLog(log) {
			continue
		}
		requestKey := log.RequestId
		if requestKey == "" {
			requestKey = fmt.Sprintf("log:%d", log.Id)
		}
		if _, ok := finalLogs[requestKey]; !ok {
			requestOrder = append(requestOrder, requestKey)
		}
		current := finalLogs[requestKey]
		if current == nil || current.Type != model.LogTypeConsume || log.Type == model.LogTypeConsume {
			finalLogs[requestKey] = log
		}
	}

	rows := make([]LogExportRow, 0, len(requestOrder))
	for _, requestKey := range requestOrder {
		rows = append(rows, buildLogExportRow(finalLogs[requestKey], true))
	}
	return rows
}

func BuildLogExportSummary(view LogExportView, rows []LogExportRow) []LogExportSummary {
	type summaryKey struct {
		partyID int
		model   string
	}

	summaries := make(map[summaryKey]*LogExportSummary)
	for _, row := range rows {
		key := summaryKey{partyID: row.UserID, model: row.ModelName}
		if view == LogExportViewUpstream {
			key.partyID = row.ChannelID
			key.model = row.UpstreamModelName
		}
		summary, ok := summaries[key]
		if !ok {
			summary = &LogExportSummary{
				UserID:      row.UserID,
				Username:    row.Username,
				ChannelID:   row.ChannelID,
				ChannelName: row.ChannelName,
				ModelName:   key.model,
			}
			summaries[key] = summary
		}
		summary.RequestCount++
		if row.Status == "success" {
			summary.SuccessCount++
			summary.InputTokens += row.InputTokens
			summary.CachedInputTokens += row.CachedInputTokens
			summary.OutputTokens += row.OutputTokens
			if row.OriginalAmountUSD == nil {
				summary.UnpricedRequestCount++
			} else {
				summary.PricedRequestCount++
				summary.KnownOriginalAmountUSD += *row.OriginalAmountUSD
			}
		} else {
			summary.FailureCount++
		}
	}

	result := make([]LogExportSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, *summary)
	}
	sort.Slice(result, func(i, j int) bool {
		if view == LogExportViewUpstream && result[i].ChannelID != result[j].ChannelID {
			return result[i].ChannelID < result[j].ChannelID
		}
		if view == LogExportViewDownstream && result[i].UserID != result[j].UserID {
			return result[i].UserID < result[j].UserID
		}
		return result[i].ModelName < result[j].ModelName
	})
	return result
}

func WriteLogExportArchive(writer io.Writer, view LogExportView, fields []string, rows []LogExportRow, summaries []LogExportSummary) error {
	if err := ValidateLogExportFields(view, fields); err != nil {
		return err
	}

	archive := zip.NewWriter(writer)
	detailFile, err := archive.Create(fmt.Sprintf("%s-details.csv", view))
	if err != nil {
		return err
	}
	if err = writeLogExportDetails(detailFile, fields, rows); err != nil {
		return err
	}
	summaryFile, err := archive.Create(fmt.Sprintf("%s-summary.csv", view))
	if err != nil {
		return err
	}
	if err = writeLogExportSummary(summaryFile, view, summaries); err != nil {
		return err
	}
	return archive.Close()
}

func buildLogExportRow(log *model.Log, hideUpstream bool) LogExportRow {
	other := parseLogExportOther(log.Other)
	row := LogExportRow{
		CreatedAt: log.CreatedAt,
		RequestID: log.RequestId,
		UserID:    log.UserId,
		Username:  log.Username,
		TokenID:   log.TokenId,
		TokenName: log.TokenName,
		Group:     log.Group,
		Status:    "failed",
		ModelName: log.ModelName,
	}
	if log.Type == model.LogTypeConsume {
		row.Status = "success"
		row.InputTokens = log.PromptTokens
		row.CachedInputTokens = optionalInt(other, "cache_tokens")
		row.OutputTokens = log.CompletionTokens
		row.ModelPrice = optionalFloat(other, "model_price")
		row.ModelRatio = optionalFloat(other, "model_ratio")
		row.CompletionRatio = optionalFloat(other, "completion_ratio")
		row.CacheRatio = optionalFloat(other, "cache_ratio")
		row.GroupRatio = optionalFloat(other, "group_ratio")
		row.QuotaPerUnit = optionalFloat(other, "quota_per_unit")
		if row.GroupRatio != nil && *row.GroupRatio > 0 && row.QuotaPerUnit != nil && *row.QuotaPerUnit > 0 {
			amount := float64(log.Quota) / *row.GroupRatio / *row.QuotaPerUnit
			row.OriginalAmountUSD = &amount
		}
	}
	if !hideUpstream {
		if log.Type == model.LogTypeError {
			row.ErrorMessage = log.Content
		}
		row.UpstreamRequestID = log.UpstreamRequestId
		row.ChannelID = log.ChannelId
		row.ChannelName = log.ChannelName
		if recordedChannelName := logExportString(other, "channel_name"); recordedChannelName != "" {
			row.ChannelName = recordedChannelName
		}
		row.UpstreamModelName = logExportString(other, "upstream_model_name")
		if row.UpstreamModelName == "" && log.Type == model.LogTypeConsume {
			row.UpstreamModelName = log.ModelName
		}
	}
	return row
}

func isExportableUsageLog(log *model.Log) bool {
	return log != nil && (log.Type == model.LogTypeConsume || log.Type == model.LogTypeError)
}

func isViolationFeeLog(log *model.Log) bool {
	if log == nil || log.Type != model.LogTypeConsume {
		return false
	}
	other := parseLogExportOther(log.Other)
	violationFee, ok := other["violation_fee"].(bool)
	return ok && violationFee
}

func parseLogExportOther(value string) map[string]interface{} {
	if value == "" {
		return nil
	}
	other, err := common.StrToMap(value)
	if err != nil {
		return nil
	}
	return other
}

func optionalFloat(values map[string]interface{}, key string) *float64 {
	value, ok := values[key]
	if !ok {
		return nil
	}
	number, ok := value.(float64)
	if !ok {
		return nil
	}
	return &number
}

func optionalInt(values map[string]interface{}, key string) int {
	value := optionalFloat(values, key)
	if value == nil {
		return 0
	}
	return int(*value)
}

func logExportString(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return value
}

func formatOptionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func formatLogExportAmount(value *float64) string {
	if value == nil {
		return ""
	}
	formatted := strconv.FormatFloat(*value, 'f', 10, 64)
	formatted = strings.TrimRight(formatted, "0")
	return strings.TrimRight(formatted, ".")
}

func writeLogExportDetails(writer io.Writer, fields []string, rows []LogExportRow) error {
	if _, err := writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	csvWriter := csv.NewWriter(writer)
	if err := csvWriter.Write(fields); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, 0, len(fields))
		for _, name := range fields {
			field := logExportFields[name]
			value := field.value(row)
			if field.text {
				value = safeSpreadsheetText(value)
			}
			record = append(record, value)
		}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func writeLogExportSummary(writer io.Writer, view LogExportView, summaries []LogExportSummary) error {
	if _, err := writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	csvWriter := csv.NewWriter(writer)
	header := []string{"model_name", "request_count", "success_count", "failure_count", "input_tokens", "cached_input_tokens", "output_tokens", "priced_success_count", "unpriced_success_count", "known_original_amount_usd"}
	if view == LogExportViewUpstream {
		header = append([]string{"channel_id", "channel_name"}, header...)
	} else {
		header = append([]string{"user_id", "username"}, header...)
	}
	if err := csvWriter.Write(header); err != nil {
		return err
	}
	for _, summary := range summaries {
		knownOriginalAmount := ""
		if summary.PricedRequestCount > 0 {
			knownOriginalAmount = formatLogExportAmount(&summary.KnownOriginalAmountUSD)
		}
		record := []string{
			safeSpreadsheetText(summary.ModelName),
			strconv.Itoa(summary.RequestCount),
			strconv.Itoa(summary.SuccessCount),
			strconv.Itoa(summary.FailureCount),
			strconv.Itoa(summary.InputTokens),
			strconv.Itoa(summary.CachedInputTokens),
			strconv.Itoa(summary.OutputTokens),
			strconv.Itoa(summary.PricedRequestCount),
			strconv.Itoa(summary.UnpricedRequestCount),
			knownOriginalAmount,
		}
		if view == LogExportViewUpstream {
			record = append([]string{strconv.Itoa(summary.ChannelID), safeSpreadsheetText(summary.ChannelName)}, record...)
		} else {
			record = append([]string{strconv.Itoa(summary.UserID), safeSpreadsheetText(summary.Username)}, record...)
		}
		if err := csvWriter.Write(record); err != nil {
			return err
		}
	}
	csvWriter.Flush()
	return csvWriter.Error()
}

func safeSpreadsheetText(value string) string {
	if strings.HasPrefix(value, "=") || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || strings.HasPrefix(value, "@") ||
		strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	return value
}

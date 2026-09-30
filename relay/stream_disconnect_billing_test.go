package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type disconnectingStreamWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
	after  int
	writes int
}

func (w *disconnectingStreamWriter) Write(data []byte) (int, error) {
	if w.writes == w.after {
		if w.cancel != nil {
			w.cancel()
		}
		return 0, errors.New("downstream disconnected")
	}
	w.writes++
	return w.ResponseWriter.Write(data)
}

// The provider leaves the transport open after its terminal event. Closing
// during cleanup also cancels the client, reproducing the completion race.
type terminalStreamBody struct {
	*strings.Reader
	closed chan struct{}
	once   sync.Once
	cancel context.CancelFunc
}

func (b *terminalStreamBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	if err == io.EOF {
		<-b.closed
	}
	return n, err
}

func (b *terminalStreamBody) Close() error {
	b.once.Do(func() { b.cancel(); close(b.closed) })
	return nil
}

func TestResponsesTerminalStopsOpenTransport(t *testing.T) {
	for _, converted := range []bool{false, true} {
		for _, event := range []string{"response.completed", "response.done"} {
			t.Run(fmt.Sprintf("converted=%t/%s", converted, event), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				body := &terminalStreamBody{
					Reader: strings.NewReader(fmt.Sprintf("data: {\"type\":%q,\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1000,\"output_tokens\":20,\"total_tokens\":1020}}}\n\n", event)),
					closed: make(chan struct{}), cancel: cancel,
				}
				defer body.Close()
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil).WithContext(ctx)
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}, IsStream: true, DisablePing: true, RelayFormat: types.RelayFormatOpenAIResponses}
				resp := &http.Response{StatusCode: http.StatusOK, Body: body, Header: http.Header{}}
				if converted {
					info.RelayFormat = types.RelayFormatOpenAI
				}
				type result struct {
					usage *dto.Usage
					err   *types.NewAPIError
				}
				done := make(chan result, 1)
				go func() {
					if converted {
						usage, err := openaichannel.OaiResponsesToChatStreamHandler(c, info, resp)
						done <- result{usage, err}
					} else {
						usage, err := openaichannel.OaiResponsesStreamHandler(c, info, resp)
						done <- result{usage, err}
					}
				}()
				select {
				case got := <-done:
					require.NotNil(t, got.usage)
					assert.Equal(t, 1000, got.usage.PromptTokens)
					assert.Equal(t, 20, got.usage.CompletionTokens)
					if converted {
						// The final Chat [DONE] is still a required downstream write.
						require.NotNil(t, got.err)
						assert.Equal(t, types.ErrorCodeClientGone, got.err.GetErrorCode())
					} else {
						require.Nil(t, got.err)
						reason, _ := info.StreamStatus.End()
						assert.Equal(t, relaycommon.StreamEndReasonDone, reason)
					}
				case <-time.After(3 * time.Second):
					body.Close()
					<-done
					t.Fatal("terminal event did not stop the open upstream transport")
				}
			})
		}
	}
}

func TestTextStreamDisconnectSettlesObservedUsage(t *testing.T) {
	service.InitHttpClient()
	oldDB, oldLogDB := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	oldBatch, oldRedis, oldConsume, oldErrors := common.BatchUpdateEnabled, common.RedisEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		common.SetDatabaseTypes(oldMainType, oldLogType)
		common.BatchUpdateEnabled, common.RedisEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled = oldBatch, oldRedis, oldConsume, oldErrors
	})
	common.BatchUpdateEnabled, common.RedisEnabled, common.LogConsumeEnabled, constant.ErrorLogEnabled = false, false, true, true

	for caseIndex, tc := range []struct {
		name, body           string
		after, prepaid       int
		charged, estimated   bool
		withoutCancel        bool
		responses, converted bool
	}{
		{name: "usage before write failure keeps cache", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}],\"usage\":{\"prompt_tokens\":1000,\"completion_tokens\":20,\"total_tokens\":1020,\"prompt_tokens_details\":{\"cached_tokens\":800}}}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n", charged: true},
		{name: "reasoning and partial output are estimated", body: "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"Let me think about this question.\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"The answer is forty two.\"}}]}\n\n", charged: true, estimated: true},
		{name: "input usage keeps cache and estimates missing output", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}],\"usage\":{\"prompt_tokens\":1000,\"prompt_tokens_details\":{\"cached_tokens\":800}}}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n", charged: true, estimated: true, withoutCancel: true},
		{name: "final usage write failure settles prepaid balance", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1000,\"completion_tokens\":20,\"total_tokens\":1020,\"prompt_tokens_details\":{\"cached_tokens\":800}}}\n\ndata: [DONE]\n\n", after: 1, prepaid: 1500, charged: true},
		{name: "role only cancellation has no billable output", body: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\ndata: {\"choices\":[]}\n\n"},
		{name: "role only cancellation on final write has no billable output", prepaid: 1500, body: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"}}]}\n\ndata: [DONE]\n\n"},
		{name: "malformed upstream error during disconnect remains a failure", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {malformed}\n\n"},
		{name: "upstream error stays uncharged", body: "data: {\"error\":{\"message\":\"upstream failed\",\"type\":\"upstream_error\",\"code\":\"failed\"},\"status_code\":502}\n\n"},
		{name: "upstream error during disconnect stays an upstream failure", body: "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"error\":{\"message\":\"upstream failed\",\"type\":\"upstream_error\",\"code\":\"failed\"},\"status_code\":502}\n\n"},
		{name: "native Responses terminal usage survives first write failure", responses: true, charged: true, body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1000,\"output_tokens\":20,\"total_tokens\":1020,\"input_tokens_details\":{\"cached_tokens\":800}}}}\n\n"},
		{name: "converted Responses terminal usage survives first write failure", converted: true, charged: true, prepaid: 1500, body: "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{\"input_tokens\":1000,\"output_tokens\":20,\"total_tokens\":1020,\"input_tokens_details\":{\"cached_tokens\":800}}}}\n\n"},
		{name: "converted Responses partial output is estimated", converted: true, charged: true, estimated: true, body: "data: {\"type\":\"response.output_text.delta\",\"delta\":\"The answer is forty two.\"}\n\n"},
		{name: "converted Responses role only cancellation on final write is uncharged", converted: true, after: 1, body: "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-test\"}}\n\n"},
		{name: "native Responses upstream error remains a failure", responses: true, body: "data: {\"type\":\"response.failed\",\"response\":{\"status\":\"failed\",\"error\":{\"type\":\"server_error\",\"code\":\"server_error\",\"message\":\"upstream failed\"}},\"status_code\":502}\n\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := model_setting.GetGlobalSettings()
			previousSettings := *settings
			t.Cleanup(func() { *settings = previousSettings })
			settings.PassThroughRequestEnabled = false
			settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: tc.converted, AllChannels: true, ModelPatterns: []string{"^gpt-test$"}}
			var dialector gorm.Dialector = sqlite.Open(":memory:")
			databaseType := common.DatabaseTypeSQLite
			dsn := os.Getenv("TEST_STREAM_DISCONNECT_DSN")
			switch os.Getenv("TEST_STREAM_DISCONNECT_DB") {
			case "mysql":
				require.Contains(t, dsn, "/codex_stream_disconnect_")
				dialector, databaseType = mysql.Open(dsn), common.DatabaseTypeMySQL
			case "postgres":
				require.Contains(t, dsn, "dbname=codex_stream_disconnect_")
				dialector, databaseType = postgres.Open(dsn), common.DatabaseTypePostgreSQL
			}
			db, err := gorm.Open(dialector, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			models := []any{&model.User{}, &model.Token{}, &model.Channel{}, &model.Log{}, &model.BillingAdmissionReserveOperation{}, &model.BillingRefundOperation{}}
			require.NoError(t, db.AutoMigrate(models...))
			t.Cleanup(func() {
				if databaseType != common.DatabaseTypeSQLite {
					require.NoError(t, db.Migrator().DropTable(models...))
				}
				require.NoError(t, sqlDB.Close())
			})
			model.DB, model.LOG_DB = db, db
			common.SetDatabaseTypes(databaseType, databaseType)
			const balance = 1000000
			require.NoError(t, db.Create(&model.User{Id: 701, Username: "disconnect-test", Quota: balance, Group: "default"}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 702, UserId: 701, Key: "disconnect-test-key", RemainQuota: balance}).Error)
			require.NoError(t, db.Create(&model.Channel{Id: 703, Name: "disconnect-test"}).Error)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.converted || tc.responses {
					assert.Equal(t, "/v1/responses", r.URL.Path)
				} else {
					assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				}
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(upstream.Close)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","stream":true,"messages":[{"role":"user","content":"hello"}]}`)).WithContext(requestCtx)
			writer := &disconnectingStreamWriter{ResponseWriter: c.Writer, cancel: cancel, after: tc.after}
			if tc.withoutCancel {
				writer.cancel = nil
			}
			c.Writer = writer
			c.Set("id", 701)
			c.Set("username", "disconnect-test")
			c.Set("token_id", 702)
			c.Set("token_name", "disconnect-test")
			c.Set("group", "default")
			c.Set(common.RequestIdKey, fmt.Sprintf("disconnect-%d", caseIndex))
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(c, constant.ContextKeyChannelId, 703)
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-test")
			common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{ForceFormat: true})
			common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now())
			common.SetContextKey(c, constant.ContextKeyIsStream, true)
			info := &relaycommon.RelayInfo{
				RelayFormat: types.RelayFormatOpenAI, RelayMode: relayconstant.RelayModeChatCompletions,
				RequestURLPath:  "/v1/chat/completions",
				Request:         &dto.GeneralOpenAIRequest{Model: "gpt-test", Stream: common.GetPointer(true)},
				OriginModelName: "gpt-test", UserId: 701, TokenId: 702, TokenKey: "disconnect-test-key",
				UserGroup: "default", UsingGroup: "default", RequestId: c.GetString(common.RequestIdKey), StartTime: time.Now(),
				IsStream: true, ForcePreConsume: true,
			}
			info.UserSetting.BillingPreference = "wallet_only"
			info.PriceData.ModelRatio, info.PriceData.CompletionRatio, info.PriceData.CacheRatio = 1, 1, 0.1
			info.PriceData.GroupRatioInfo.GroupRatio = 1
			info.SetEstimatePromptTokens(1000)
			if tc.responses {
				c.Request.URL.Path = "/v1/responses"
				info.RequestURLPath = "/v1/responses"
				info.RelayFormat = types.RelayFormatOpenAIResponses
				info.RelayMode = relayconstant.RelayModeResponses
				info.Request = &dto.OpenAIResponsesRequest{Model: "gpt-test", Stream: common.GetPointer(true), Input: []byte(`"hello"`)}
			}
			require.Nil(t, service.PreConsumeBilling(c, tc.prepaid, info))

			var apiErr *types.NewAPIError
			if tc.responses {
				apiErr = ResponsesHelper(c, info)
			} else {
				apiErr = TextHelper(c, info)
			}
			if tc.charged {
				require.Nil(t, apiErr)
			} else {
				require.NotNil(t, apiErr)
			}
			if tc.charged || strings.Contains(tc.name, "cancellation") {
				assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
				assert.Equal(t, perfmetrics.OutcomeIgnored, perfmetrics.ClassifyRelayOutcome(context.Background(), info, apiErr))
			}
			if strings.Contains(tc.name, "cancellation") {
				assert.Equal(t, types.ErrorCodeClientGone, apiErr.GetErrorCode())
				assert.Equal(t, 499, apiErr.StatusCode)
				assert.False(t, service.ShouldDisableChannel(apiErr))
				assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "client_gone", Source: "client"}, service.DecideRelayRetry(c, apiErr, 3))
			}
			service.RecordPolicyFailure(c, 703, apiErr, service.DecideRelayRetry(c, apiErr, 3))
			service.ProcessChannelError(c, types.ChannelError{ChannelId: 703}, apiErr, info)
			RefundFailedRequestBilling(c, info, apiErr)
			if !tc.charged && tc.prepaid > 0 {
				// Refund runs asynchronously; wait for its durable completion
				// before reading balances or closing this test's database.
				require.Eventually(t, func() bool {
					var count int64
					return db.Model(&model.BillingRefundOperation{}).Where("request_id = ? AND status = ?", info.RequestId, model.BillingRefundStatusApplied).Count(&count).Error == nil && count == 1
				}, 3*time.Second, 10*time.Millisecond)
			}
			var logs []model.Log
			require.NoError(t, db.Order("id").Find(&logs).Error)
			var consumed, failed *model.Log
			for i := range logs {
				if logs[i].Type == model.LogTypeConsume {
					consumed = &logs[i]
				}
				if logs[i].Type == model.LogTypeError {
					failed = &logs[i]
				}
			}
			if strings.Contains(tc.name, "cancellation") {
				require.NotNil(t, failed)
				assert.Equal(t, "client_gone", gjson.Get(failed.Other, "stream_status.end_reason").String())
				assert.Equal(t, int64(499), gjson.Get(failed.Other, "status_code").Int())
			}
			var user model.User
			var token model.Token
			require.NoError(t, db.First(&user, 701).Error)
			require.NoError(t, db.First(&token, 702).Error)
			if !tc.charged {
				require.NotNil(t, failed)
				if strings.Contains(tc.name, "upstream error") {
					assert.NotEqual(t, types.ErrorCodeClientGone, apiErr.GetErrorCode())
					assert.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					assert.Equal(t, perfmetrics.OutcomeFailure, perfmetrics.ClassifyRelayOutcome(context.Background(), info, apiErr))
					assert.Equal(t, perfmetrics.OutcomeFailure, perfmetrics.ClassifyRelayOutcome(c.Request.Context(), info, apiErr))
				}
				assert.Nil(t, consumed)
				assert.Equal(t, balance, user.Quota)
				assert.Equal(t, balance, token.RemainQuota)
				return
			}
			require.NotNil(t, consumed)
			assert.Nil(t, failed)
			assert.Len(t, logs, 1)
			for _, event := range service.RequestPolicy(c).Events() {
				assert.NotEqual(t, "failure", event.Decision.Action)
			}
			events := service.RequestPolicy(c).Events()
			require.NotEmpty(t, events)
			assert.Equal(t, service.PolicyDecision{Action: "stop", Reason: "client_gone", Source: "client"}, events[len(events)-1].Decision)
			if tc.after > 0 {
				assert.Equal(t, http.StatusOK, c.Writer.Status())
			}
			assert.Equal(t, 1000, consumed.PromptTokens)
			assert.Greater(t, consumed.CompletionTokens, 0)
			assert.Equal(t, "client_gone", gjson.Get(consumed.Other, "stream_status.end_reason").String())
			if tc.estimated {
				assert.True(t, gjson.Get(consumed.Other, "admin_info.local_count_tokens").Bool())
				if strings.Contains(tc.name, "keeps cache") {
					assert.Equal(t, int64(800), gjson.Get(consumed.Other, "cache_tokens").Int())
					assert.Equal(t, 280+consumed.CompletionTokens, consumed.Quota)
				}
			} else {
				assert.Equal(t, 20, consumed.CompletionTokens)
				assert.Equal(t, int64(800), gjson.Get(consumed.Other, "cache_tokens").Int())
				assert.Equal(t, 300, consumed.Quota)
			}
			assert.Equal(t, balance-consumed.Quota, user.Quota)
			assert.Equal(t, balance-consumed.Quota, token.RemainQuota)
			assert.False(t, info.Billing.NeedsRefund())
			require.NoError(t, info.Billing.Settle(consumed.Quota))
			RefundFailedRequestBilling(c, info, apiErr)
			require.NoError(t, db.First(&user, 701).Error)
			assert.Equal(t, balance-consumed.Quota, user.Quota)
		})
	}
}

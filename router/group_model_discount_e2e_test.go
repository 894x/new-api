package router

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/groupdiscount"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupModelSpecialRatioE2E(t *testing.T) func(*testing.T) {
	t.Helper()
	waitForRefunds := setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{}, &model.ChannelModelOverride{}, &model.Log{}, &model.UserSubscription{}, &model.Option{},
		&model.UserGroupModelMonthlyUsage{}, &model.GroupModelDiscountSettlement{}, &model.GroupModelDiscountAdjustment{},
	))
	ratio_setting.InitRatioSettings()
	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	quotaPerUnit, logEnabled := common.QuotaPerUnit, common.LogConsumeEnabled
	batchEnabled, cacheEnabled, retries := common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.RetryTimes
	usableGroups := setting.UserUsableGroups2JSONString()
	modelRatios, modelPrices := ratio_setting.ModelRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	completionRatios := ratio_setting.CompletionRatio2JSONString()
	maxAutoGroups := setting.GetMaxTokenAutoGroups()
	retryCodes := operation_setting.AutomaticRetryStatusCodesToString()
	t.Cleanup(func() {
		waitForRefunds(t)
		common.QuotaPerUnit, common.LogConsumeEnabled = quotaPerUnit, logEnabled
		common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.RetryTimes = batchEnabled, cacheEnabled, retries
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(modelRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(modelPrices))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(completionRatios))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(usableGroups))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(maxAutoGroups)))
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(retryCodes))
	})
	common.QuotaPerUnit, common.LogConsumeEnabled = 1_000, true
	common.BatchUpdateEnabled, common.MemoryCacheEnabled, common.RetryTimes = false, true, 0
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"pool-a":"A","pool-b":"B","auto":"Auto"}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"group_ratio_setting.group_ratio":             `{"pool-a":2,"pool-b":3}`,
		"group_ratio_setting.group_group_ratio":       `{}`,
		ratio_setting.GroupGroupModelRatioOptionKey:   `{}`,
		"group_ratio_setting.model_tiered_ratios":     `{}`,
		"billing_setting.billing_mode":                `{}`,
		"billing_setting.billing_expr":                `{}`,
		"quota_setting.trust_quota_usd":               "0",
		"quota_setting.enable_free_model_pre_consume": "false",
		"quota_setting.pre_consume_multiplier":        "1",
	}))
	return waitForRefunds
}

func TestModelSpecialRatioUserKeyGroupAndPublicModelAccountingE2E(t *testing.T) {
	waitForRefunds := setupModelSpecialRatioE2E(t)
	models := []string{"public-ratio", "public-price", "public-expr", "public-fixed"}
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"public-ratio":1.5}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"public-ratio":2}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public-price":0.3}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":          `{"public-expr":"tiered_expr","public-fixed":"tiered_expr"}`,
		"billing_setting.billing_expr":          `{"public-expr":"tier(\"base\", p * 1000 + c * 4000)","public-fixed":"tier(\"request\", fixed(0.3))"}`,
		"group_ratio_setting.group_group_ratio": `{"user-a":{"pool-a":0.91,"pool-b":0.92},"user-b":{"pool-a":0.93,"pool-b":0.94}}`,
		ratio_setting.GroupGroupModelRatioOptionKey: `{
			"user-a":{"pool-a":{"public-ratio":0.1,"public-price":0.2,"public-expr":0.3,"public-fixed":0,"mapped-shared":9},"pool-b":{"public-ratio":0.4,"public-price":0.5,"public-expr":0.6,"public-fixed":0.7}},
			"user-b":{"pool-a":{"public-ratio":0.8,"public-price":0.9,"public-expr":1,"public-fixed":1.25},"pool-b":{"public-ratio":0.75,"public-price":0.65,"public-expr":0.55,"public-fixed":0.45}}
		}`,
	}))
	monthly := map[string]map[string]any{}
	for _, group := range []string{"pool-a", "pool-b"} {
		monthly[group] = map[string]any{}
		for _, name := range models {
			monthly[group][name] = map[string]any{"enabled": true, "effective_from": 0, "effective_until": nil, "timezone": "UTC", "tiers": []map[string]any{{"min_monthly_original_quota": 0, "ratio": 0.01}}}
		}
	}
	monthlyJSON, err := common.Marshal(monthly)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(string(monthlyJSON)))
	var fail atomic.Bool
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		var body struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if !assert.NoError(t, common.DecodeJson(r.Body, &body)) {
			w.WriteHeader(500)
			return
		}
		assert.Equal(t, "mapped-shared", body.Model, "all public IDs share the same upstream ID")
		if fail.Load() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{"error":{"message":"failed","type":"upstream_error"}}`))
			return
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl-special\",\"object\":\"chat.completion.chunk\",\"model\":\"mapped-shared\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-special\",\"object\":\"chat.completion.chunk\",\"model\":\"mapped-shared\",\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":50,\"total_tokens\":150}}\n\ndata: [DONE]\n\n"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-special","object":"chat.completion","model":"mapped-shared","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":100,"completion_tokens":50,"total_tokens":150}}`))
	}))
	t.Cleanup(upstream.Close)
	mapping := `{"public-ratio":"mapped-shared","public-price":"mapped-shared","public-expr":"mapped-shared","public-fixed":"mapped-shared"}`
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "shared-special", Key: "local-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: strings.Join(models, ","), Group: "pool-a,pool-b", ModelMapping: &mapping}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetRelayRouter(engine)
	ratios := [][]float64{{0.1, 0.2, 0.3, 0, 0.4, 0.5, 0.6, 0.7}, {0.8, 0.9, 1, 1.25, 0.75, 0.65, 0.55, 0.45}}
	total := 0
	for userIndex, group := range []string{"user-a", "user-b"} {
		user := model.User{Username: fmt.Sprintf("special-%d", userIndex), AffCode: fmt.Sprintf("specialaff%d", userIndex), Group: group, Status: common.UserStatusEnabled, Quota: 10_000}
		require.NoError(t, model.DB.Create(&user).Error)
		userTotal := 0
		for groupIndex, keyGroup := range []string{"pool-a", "pool-b"} {
			token := model.Token{UserId: user.Id, Key: fmt.Sprintf("specialkey%d%d", userIndex, groupIndex), Name: keyGroup, Group: keyGroup, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 10_000}
			require.NoError(t, model.DB.Create(&token).Error)
			tokenTotal := 0
			for modelIndex, name := range models {
				t.Run(fmt.Sprintf("%s/%s/%s", group, keyGroup, name), func(t *testing.T) {
					ratio := ratios[userIndex][groupIndex*len(models)+modelIndex]
					// Each pricing engine independently yields a 300-quota base charge.
					want := int(300 * ratio)
					body := fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, name)
					request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
					request.Header.Set("Authorization", "Bearer "+token.Key)
					request.Header.Set("Content-Type", "application/json")
					recorder := httptest.NewRecorder()
					engine.ServeHTTP(recorder, request)
					require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					tokenTotal += want
					userTotal += want
					total += want
					var log model.Log
					require.NoError(t, model.DB.Where("type = ? AND token_id = ? AND model_name = ?", model.LogTypeConsume, token.Id, name).First(&log).Error)
					assert.Equal(t, want, log.Quota)
					assert.Equal(t, keyGroup, log.Group)
					assert.Equal(t, channel.Id, log.ChannelId)
					var other struct {
						GroupRatio   float64 `json:"group_ratio"`
						SpecialRatio float64 `json:"group_special_ratio"`
					}
					require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
					assert.Equal(t, ratio, other.GroupRatio)
					require.NoError(t, model.DB.First(&token, token.Id).Error)
					assert.Equal(t, 10_000-tokenTotal, token.RemainQuota)
					assert.Equal(t, tokenTotal, token.UsedQuota)
					require.NoError(t, model.DB.First(&user, user.Id).Error)
					assert.Equal(t, 10_000-userTotal, user.Quota)
					assert.Equal(t, userTotal, user.UsedQuota)
				})
			}
			// The same fixed-price request as SSE must charge once, not per chunk.
			if userIndex == 0 && groupIndex == 0 {
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-price","stream":true,"stream_options":{"include_usage":true},"messages":[{"role":"user","content":"hello"}]}`))
				request.Header.Set("Authorization", "Bearer "+token.Key)
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), "[DONE]")
				tokenTotal += 60
				userTotal += 60
				total += 60
				// A failed upstream attempt returns the full model-specific reservation.
				fail.Store(true)
				request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-price","messages":[{"role":"user","content":"fail"}]}`))
				request.Header.Set("Authorization", "Bearer "+token.Key)
				request.Header.Set("Content-Type", "application/json")
				recorder = httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				require.NotEqual(t, http.StatusOK, recorder.Code, recorder.Body.String())
				waitForRefunds(t)
				fail.Store(false)
				require.NoError(t, model.DB.First(&token, token.Id).Error)
				assert.Equal(t, 10_000-tokenTotal, token.RemainQuota)
				assert.Equal(t, tokenTotal, token.UsedQuota)
				require.NoError(t, model.DB.First(&user, user.Id).Error)
				assert.Equal(t, 10_000-userTotal, user.Quota)
			}
		}
		wantRequests := 8
		if userIndex == 0 {
			wantRequests++
		}
		assert.Equal(t, wantRequests, user.RequestCount)
	}
	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, total, channel.UsedQuota)
	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	assert.Len(t, logs, 17)
	for _, table := range []any{&model.UserGroupModelMonthlyUsage{}, &model.GroupModelDiscountSettlement{}} {
		var count int64
		require.NoError(t, model.DB.Model(table).Count(&count).Error)
		assert.Zero(t, count, "fixed model-specific contracts must suppress monthly discounts")
	}
	assert.EqualValues(t, 18, attempts.Load())
}

func TestModelSpecialRatioRequestSnapshotE2E(t *testing.T) {
	setupModelSpecialRatioE2E(t)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public-snapshot":0.3}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupModelRatioByJSONString(`{"customer":{"pool-a":{"public-snapshot":0.2}}}`))
	var updateErr error
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		updateErr = ratio_setting.UpdateGroupGroupModelRatioByJSONString(`{"customer":{"pool-a":{"public-snapshot":0.9}}}`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"snapshot","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(upstream.Close)
	user := model.User{Username: "snapshot", Group: "customer", Status: common.UserStatusEnabled, Quota: 1_000}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: "snapshotkey", Group: "pool-a", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1_000}
	require.NoError(t, model.DB.Create(&token).Error)
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "snapshot", Key: "local-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: "public-snapshot", Group: "pool-a"}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()
	engine := gin.New()
	SetRelayRouter(engine)
	for _, want := range []int{940, 670} {
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-snapshot","messages":[{"role":"user","content":"hello"}]}`))
		request.Header.Set("Authorization", "Bearer "+token.Key)
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.NoError(t, updateErr)
		require.NoError(t, model.DB.First(&user, user.Id).Error)
		assert.Equal(t, want, user.Quota)
		require.NoError(t, model.DB.First(&token, token.Id).Error)
		assert.Equal(t, want, token.RemainQuota)
	}
}

func TestModelSpecialRatioRetryReservationE2E(t *testing.T) {
	for _, mode := range []string{"per-call", "fixed-expr"} {
		for _, scenario := range []struct {
			name          string
			first, final  float64
			balance, want int
			blocked       bool
			tokenBalance  int
			price         float64
		}{
			{"paid-to-paid", 0.1, 0.8, 1_000, 240, false, 1_000, 0.3},
			{"free-to-paid", 0, 0.8, 1_000, 240, false, 1_000, 0.3},
			{"paid-to-free", 0.8, 0, 1_000, 0, false, 1_000, 0.3},
			{"insufficient-wallet", 0.1, 0.8, 100, 0, true, 1_000, 0.3},
			{"insufficient-token", 0.1, 0.8, 1_000, 0, true, 100, 0.3},
			{"fractional-reservation-freezes-price", 0.1, 2, 1_000, 4, false, 1_000, 0.0018},
		} {
			t.Run(mode+"/"+scenario.name, func(t *testing.T) {
				waitForRefunds := setupModelSpecialRatioE2E(t)
				common.RetryTimes = 1
				require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("500"))
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(fmt.Sprintf(`{"public-retry":%g}`, scenario.price)))
				if mode == "fixed-expr" {
					require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
						"billing_setting.billing_mode": `{"public-retry":"tiered_expr"}`,
						"billing_setting.billing_expr": fmt.Sprintf(`{"public-retry":"tier(\"request\", fixed(%g))"}`, scenario.price),
					}))
				}
				require.NoError(t, ratio_setting.UpdateGroupGroupModelRatioByJSONString(fmt.Sprintf(`{"customer":{"pool-a":{"public-retry":%g},"pool-b":{"public-retry":%g}}}`, scenario.first, scenario.final)))
				user := model.User{Username: "retry", Group: "customer", Status: common.UserStatusEnabled, Quota: scenario.balance}
				require.NoError(t, model.DB.Create(&user).Error)
				token := model.Token{UserId: user.Id, Key: "specialretrykey", Group: "auto", CrossGroupRetry: true, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: scenario.tokenBalance}
				require.NoError(t, token.SetAutoGroups([]string{"pool-a", "pool-b"}))
				require.NoError(t, model.DB.Create(&token).Error)
				var firstAttempts, finalAttempts atomic.Int32
				first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					firstAttempts.Add(1)
					if scenario.price == 0.0018 {
						assert.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public-retry":99}`))
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(500)
					_, _ = w.Write([]byte(`{"error":{"message":"retry","type":"upstream_error"}}`))
				}))
				t.Cleanup(first.Close)
				final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					finalAttempts.Add(1)
					var current model.User
					if assert.NoError(t, model.DB.First(&current, user.Id).Error) {
						finalReserve := scenario.want
						if mode == "per-call" {
							// Legacy admission truncates the raw estimate; final
							// settlement rounds it according to the existing policy.
							finalReserve = int(scenario.price * common.QuotaPerUnit * scenario.final)
						}
						reserved := max(int(scenario.price*common.QuotaPerUnit*scenario.first), finalReserve)
						assert.Equal(t, scenario.balance-reserved, current.Quota, "reserve the final group's price before submission")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"id":"retry","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
				}))
				t.Cleanup(final.Close)
				channels := []model.Channel{}
				for index, server := range []*httptest.Server{first, final} {
					channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: fmt.Sprintf("retry-%d", index), Key: "local-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(server.URL), Models: "public-retry", Group: []string{"pool-a", "pool-b"}[index]}
					require.NoError(t, model.DB.Create(&channel).Error)
					require.NoError(t, channel.AddAbilities(nil))
					channels = append(channels, channel)
				}
				model.InitChannelCache()
				engine := gin.New()
				SetRelayRouter(engine)
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-retry","messages":[{"role":"user","content":"hello"}]}`))
				request.Header.Set("Authorization", "Bearer "+token.Key)
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				if scenario.blocked {
					assert.NotEqual(t, http.StatusOK, recorder.Code, recorder.Body.String())
					assert.Zero(t, finalAttempts.Load(), "insufficient funds must reject before the second upstream submission")
					waitForRefunds(t)
				} else {
					assert.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
					assert.EqualValues(t, 1, finalAttempts.Load())
				}
				assert.GreaterOrEqual(t, firstAttempts.Load(), int32(1))
				require.NoError(t, model.DB.First(&user, user.Id).Error)
				assert.Equal(t, scenario.balance-scenario.want, user.Quota)
				assert.Equal(t, scenario.want, user.UsedQuota)
				require.NoError(t, model.DB.First(&token, token.Id).Error)
				assert.Equal(t, scenario.tokenBalance-scenario.want, token.RemainQuota)
				assert.Equal(t, scenario.want, token.UsedQuota)
				require.NoError(t, model.DB.First(&channels[0], channels[0].Id).Error)
				assert.Zero(t, channels[0].UsedQuota)
				require.NoError(t, model.DB.First(&channels[1], channels[1].Id).Error)
				assert.EqualValues(t, scenario.want, channels[1].UsedQuota)
			})
		}
	}
}

func TestModelSpecialRatioFallbackAndBoundsE2E(t *testing.T) {
	for _, tc := range []struct {
		name, special, scalar, keyGroup string
		monthly                         bool
		ratio                           float64
		want                            int
		rejected                        bool
	}{
		{"old-group-contract", `{}`, `{"customer":{"pool-a":0.6}}`, "pool-a", true, 0.6, 180, false},
		{"monthly-fallback", `{}`, `{}`, "pool-a", true, 2, 240, false},
		{"base-group-fallback", `{}`, `{}`, "pool-a", false, 2, 600, false},
		{"mapped-model-does-not-match", `{"customer":{"pool-a":{"mapped-fallback":0.01}}}`, `{}`, "pool-a", false, 2, 600, false},
		{"other-user-does-not-match", `{"other":{"pool-a":{"public-fallback":0.01}}}`, `{}`, "pool-a", false, 2, 600, false},
		{"other-key-group-does-not-match", `{"customer":{"pool-b":{"public-fallback":0.01}}}`, `{}`, "pool-a", false, 2, 600, false},
		{"model-id-is-exact", `{"customer":{"pool-a":{"PUBLIC-fallback":0.01}}}`, `{}`, "pool-a", false, 2, 600, false},
		{"empty-key-inherits-user-group", `{"customer":{"customer":{"public-fallback":0.4}}}`, `{}`, "", false, 0.4, 120, false},
		{"explicit-zero", `{"customer":{"pool-a":{"public-fallback":0}}}`, `{}`, "pool-a", true, 0, 0, false},
		{"tiny-positive-legacy-rounding", `{"customer":{"pool-a":{"public-fallback":0.0001}}}`, `{}`, "pool-a", false, 0.0001, 1, false},
		{"huge-ratio-fails-before-submission", `{"customer":{"pool-a":{"public-fallback":1e308}}}`, `{}`, "pool-a", false, 0, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupModelSpecialRatioE2E(t)
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"public-fallback":0.3}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"pool-a":2,"pool-b":3,"customer":1}`))
			require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(tc.scalar))
			require.NoError(t, ratio_setting.UpdateGroupGroupModelRatioByJSONString(tc.special))
			if tc.monthly {
				require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{"pool-a":{"public-fallback":{"enabled":true,"effective_from":0,"effective_until":null,"timezone":"UTC","tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]}}}`))
			}
			var attempts atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"fallback","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
			}))
			t.Cleanup(upstream.Close)
			user := model.User{Username: "fallback", Group: "customer", Status: common.UserStatusEnabled, Quota: 1_000}
			require.NoError(t, model.DB.Create(&user).Error)
			token := model.Token{UserId: user.Id, Key: "fallbackkey", Group: tc.keyGroup, Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 1_000}
			require.NoError(t, model.DB.Create(&token).Error)
			mapping := `{"public-fallback":"mapped-fallback"}`
			channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "fallback", Key: "local-key", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(upstream.URL), Models: "public-fallback", ModelMapping: &mapping, Group: "pool-a,customer"}
			require.NoError(t, model.DB.Create(&channel).Error)
			require.NoError(t, channel.AddAbilities(nil))
			model.InitChannelCache()
			engine := gin.New()
			SetRelayRouter(engine)
			request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"public-fallback","messages":[{"role":"user","content":"hello"}]}`))
			request.Header.Set("Authorization", "Bearer "+token.Key)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			if tc.rejected {
				assert.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				assert.Zero(t, attempts.Load())
			} else {
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			}
			require.NoError(t, model.DB.First(&user, user.Id).Error)
			assert.Equal(t, 1_000-tc.want, user.Quota)
			assert.Equal(t, tc.want, user.UsedQuota)
			require.NoError(t, model.DB.First(&token, token.Id).Error)
			assert.Equal(t, 1_000-tc.want, token.RemainQuota)
			assert.Equal(t, tc.want, token.UsedQuota)
			require.NoError(t, model.DB.First(&channel, channel.Id).Error)
			assert.EqualValues(t, tc.want, channel.UsedQuota)
			var settlements []model.GroupModelDiscountSettlement
			require.NoError(t, model.DB.Find(&settlements).Error)
			if tc.name == "monthly-fallback" {
				require.Len(t, settlements, 1)
				assert.EqualValues(t, 300, settlements[0].OriginalQuota)
				assert.EqualValues(t, 240, settlements[0].ChargedQuota)
			} else {
				assert.Empty(t, settlements)
			}
			var logs []model.Log
			require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
			if tc.rejected {
				assert.Empty(t, logs)
			} else {
				require.Len(t, logs, 1)
				assert.Equal(t, tc.want, logs[0].Quota)
				var other struct {
					GroupRatio float64 `json:"group_ratio"`
				}
				require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
				assert.Equal(t, tc.ratio, other.GroupRatio)
			}
		})
	}
}

func TestGroupModelMonthlyDiscountSettlesOriginalPriceAcrossTierBoundaryE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalModelPrices := ratio_setting.ModelPrice2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(originalTieredRatios))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-monthly-e2e":0.3}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"vip":0.5}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"vip":{"gpt-monthly-e2e":{
			"enabled":true,
			"effective_from":0,
			"effective_until":null,
			"timezone":"UTC",
			"tiers":[
				{"min_monthly_original_quota":0,"ratio":0.9},
				{"min_monthly_original_quota":500,"ratio":0.8}
			]
		}}
	}`))

	var upstreamFails atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if upstreamFails.Load() {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"error":{"message":"upstream failed","type":"upstream_error"}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-monthly-discount",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-monthly-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "monthly-discount-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "vip",
		Quota:    5_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "monthlydiscounte2ekey",
		Name:           "monthly-discount-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    5_000,
		UnlimitedQuota: false,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "monthly-discount-upstream",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(upstream.URL),
		Models:   "gpt-monthly-e2e",
		Group:    "vip",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	for requestNumber := 0; requestNumber < 2; requestNumber++ {
		body := []byte(`{"model":"gpt-monthly-e2e","messages":[{"role":"user","content":"hello"}]}`)
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer monthlydiscounte2ekey")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var payload struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Len(t, payload.Choices, 1)
		assert.Equal(t, "ok", payload.Choices[0].Message.Content)
	}

	// A failed upstream request is pre-consumed at the original price but must
	// be fully returned. It never advances the authoritative monthly cursor and
	// never creates a consume log/settlement.
	upstreamFails.Store(true)
	failedRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(
		[]byte(`{"model":"gpt-monthly-e2e","messages":[{"role":"user","content":"fail"}]}`),
	))
	failedRequest.Header.Set("Authorization", "Bearer monthlydiscounte2ekey")
	failedRequest.Header.Set("Content-Type", "application/json")
	failedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(failedRecorder, failedRequest)
	require.NotEqual(t, http.StatusOK, failedRecorder.Code, failedRecorder.Body.String())
	require.Eventually(t, func() bool {
		var currentUser model.User
		var currentToken model.Token
		if model.DB.First(&currentUser, user.Id).Error != nil || model.DB.First(&currentToken, token.Id).Error != nil {
			return false
		}
		return currentUser.Quota == 4_470 && currentToken.RemainQuota == 4_470
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 4_470, user.Quota)
	assert.Equal(t, 530, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)

	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 4_470, token.RemainQuota)
	assert.Equal(t, 530, token.UsedQuota)

	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 530, channel.UsedQuota)

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where("user_id = ? AND using_group = ? AND origin_model = ?", user.Id, "vip", "gpt-monthly-e2e").First(&usage).Error)
	assert.EqualValues(t, 600, usage.OriginalQuota)
	assert.EqualValues(t, 530, usage.ChargedQuota)
	assert.Equal(t, "600", usage.ProgressQuota)

	var settlements []model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.Order("id ASC").Find(&settlements).Error)
	require.Len(t, settlements, 2)
	assert.Equal(t, []int64{300, 300}, []int64{settlements[0].OriginalQuota, settlements[1].OriginalQuota})
	assert.Equal(t, []int64{270, 260}, []int64{settlements[0].ChargedQuota, settlements[1].ChargedQuota})
	assert.Equal(t, groupdiscount.ProgressBasisOriginal, settlements[0].ProgressBasis)
	assert.Equal(t, groupdiscount.ProgressBasisOriginal, settlements[1].ProgressBasis)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[0].Status)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[1].Status)

	var secondSegments []groupdiscount.Segment
	require.NoError(t, common.UnmarshalJsonStr(settlements[1].Segments, &secondSegments))
	require.Len(t, secondSegments, 2)
	assert.EqualValues(t, 200, secondSegments[0].OriginalQuota)
	assert.Equal(t, 0.9, secondSegments[0].Ratio)
	assert.EqualValues(t, 100, secondSegments[1].OriginalQuota)
	assert.Equal(t, 0.8, secondSegments[1].Ratio)

	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, []int{270, 260}, []int{logs[0].Quota, logs[1].Quota})
	assert.Contains(t, logs[0].Other, `"progress_basis":"original"`)
	assert.Contains(t, logs[0].Other, `"original_quota":300`)
	assert.Contains(t, logs[1].Other, `"monthly_original_after":600`)
}

func TestGroupModelMonthlyDiscountSettledPriceProgressAcrossTierBoundaryE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalModelPrices := ratio_setting.ModelPrice2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalTieredRatios := ratio_setting.ModelTieredRatios2JSONString()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalModelPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(originalTieredRatios))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"gpt-settled-progress-e2e":0.3}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"settled-progress":0.5}`))
	require.NoError(t, ratio_setting.UpdateModelTieredRatiosByJSONString(`{
		"settled-progress":{
			"progress_basis":"charged",
			"models":{"gpt-settled-progress-e2e":{
				"enabled":true,
				"effective_from":0,
				"effective_until":null,
				"timezone":"UTC",
				"tiers":[
					{"min_monthly_original_quota":0,"ratio":0.8},
					{"min_monthly_original_quota":401,"ratio":0.7}
				]
			}}
		}
	}`))

	var upstreamFails atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if upstreamFails.Load() {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusInternalServerError)
			_, _ = writer.Write([]byte(`{"error":{"message":"upstream failed","type":"upstream_error"}}`))
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-settled-progress",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-settled-progress-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}
		}`))
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "settled-progress-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "settled-progress",
		Quota:    5_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "settledprogresse2ekey",
		Name:           "settled-progress-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    5_000,
		UnlimitedQuota: false,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "settled-progress-upstream",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(upstream.URL),
		Models:   "gpt-settled-progress-e2e",
		Group:    "settled-progress",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	for requestNumber := 0; requestNumber < 2; requestNumber++ {
		body := []byte(`{"model":"gpt-settled-progress-e2e","messages":[{"role":"user","content":"hello"}]}`)
		request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer settledprogresse2ekey")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()

		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		var payload struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
		require.Len(t, payload.Choices, 1)
		assert.Equal(t, "ok", payload.Choices[0].Message.Content)
	}

	// A failed request must return the original-price admission reserve without
	// advancing either the exact discounted progress cursor or actual charges.
	upstreamFails.Store(true)
	failedRequest := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(
		[]byte(`{"model":"gpt-settled-progress-e2e","messages":[{"role":"user","content":"fail"}]}`),
	))
	failedRequest.Header.Set("Authorization", "Bearer settledprogresse2ekey")
	failedRequest.Header.Set("Content-Type", "application/json")
	failedRecorder := httptest.NewRecorder()
	engine.ServeHTTP(failedRecorder, failedRequest)
	require.NotEqual(t, http.StatusOK, failedRecorder.Code, failedRecorder.Body.String())
	require.Eventually(t, func() bool {
		var currentUser model.User
		var currentToken model.Token
		if model.DB.First(&currentUser, user.Id).Error != nil || model.DB.First(&currentToken, token.Id).Error != nil {
			return false
		}
		return currentUser.Quota == 4_530 && currentToken.RemainQuota == 4_530
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 4_530, user.Quota)
	assert.Equal(t, 470, user.UsedQuota)
	assert.Equal(t, 2, user.RequestCount)

	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 4_530, token.RemainQuota)
	assert.Equal(t, 470, token.UsedQuota)

	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 470, channel.UsedQuota)

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where("user_id = ? AND using_group = ? AND origin_model = ?", user.Id, "settled-progress", "gpt-settled-progress-e2e").First(&usage).Error)
	assert.EqualValues(t, 600, usage.OriginalQuota)
	assert.EqualValues(t, 470, usage.ChargedQuota)
	assert.Equal(t, "470.125", usage.ProgressQuota)

	var settlements []model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.Order("id ASC").Find(&settlements).Error)
	require.Len(t, settlements, 2)
	assert.Equal(t, []int64{300, 300}, []int64{settlements[0].OriginalQuota, settlements[1].OriginalQuota})
	assert.Equal(t, []int64{240, 230}, []int64{settlements[0].ChargedQuota, settlements[1].ChargedQuota})
	assert.Equal(t, []string{"240", "230.125"}, []string{settlements[0].ProgressQuota, settlements[1].ProgressQuota})
	assert.Equal(t, "470.125", settlements[1].MonthlyProgressAfter)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[0].Status)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlements[1].Status)

	var secondSegments []groupdiscount.Segment
	require.NoError(t, common.UnmarshalJsonStr(settlements[1].Segments, &secondSegments))
	require.Len(t, secondSegments, 2)
	assert.Equal(t, "201.25", secondSegments[0].OriginalQuotaExact)
	assert.Equal(t, "161", secondSegments[0].ProgressQuota)
	assert.Equal(t, 0.8, secondSegments[0].Ratio)
	assert.Equal(t, "98.75", secondSegments[1].OriginalQuotaExact)
	assert.Equal(t, "69.125", secondSegments[1].ProgressQuota)
	assert.Equal(t, 0.7, secondSegments[1].Ratio)

	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Order("id ASC").Find(&logs).Error)
	require.Len(t, logs, 2)
	assert.Equal(t, []int{240, 230}, []int{logs[0].Quota, logs[1].Quota})
	assert.Contains(t, logs[0].Other, `"progress_basis":"charged"`)
	assert.Contains(t, logs[1].Other, `"monthly_progress_after":"470.125"`)
}

func TestGroupModelMonthlyDiscountTieredExprChargesWhenFixedGroupRatioIsZeroE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":    `{"gpt-monthly-zero-tiered-e2e":"tiered_expr"}`,
		"billing_setting.billing_expr":    `{"gpt-monthly-zero-tiered-e2e":"tier(\"base\", c * 15)"}`,
		"group_ratio_setting.group_ratio": `{"monthly-zero-e2e":0}`,
		"group_ratio_setting.model_tiered_ratios": `{
			"monthly-zero-e2e":{"gpt-monthly-zero-tiered-e2e":{
				"enabled":true,
				"effective_from":0,
				"effective_until":null,
				"timezone":"UTC",
				"tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]
			}}
		}`,
	}))

	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-monthly-zero-tiered",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-monthly-zero-tiered-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"paid"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1000,"total_tokens":1001}
		}`))
	}))
	t.Cleanup(upstream.Close)

	user := model.User{
		Username: "monthly-zero-tiered-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "monthly-zero-e2e",
		Quota:    1_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:         user.Id,
		Key:            "monthlyzerotierede2ekey",
		Name:           "monthly-zero-tiered-token",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		RemainQuota:    1_000,
		UnlimitedQuota: false,
	}
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	channel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "monthly-zero-tiered-upstream",
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(upstream.URL),
		Models:   "gpt-monthly-zero-tiered-e2e",
		Group:    "monthly-zero-e2e",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, channel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	body := []byte(`{"model":"gpt-monthly-zero-tiered-e2e","messages":[{"role":"user","content":"hello"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer monthlyzerotierede2ekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.Eventually(t, func() bool {
		var currentUser model.User
		var currentToken model.Token
		if model.DB.First(&currentUser, user.Id).Error != nil || model.DB.First(&currentToken, token.Id).Error != nil {
			return false
		}
		return currentUser.Quota == 988 && currentToken.RemainQuota == 988
	}, time.Second, 10*time.Millisecond)

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 988, user.Quota)
	assert.Equal(t, 12, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 988, token.RemainQuota)
	assert.Equal(t, 12, token.UsedQuota)
	require.NoError(t, model.DB.First(&channel, channel.Id).Error)
	assert.EqualValues(t, 12, channel.UsedQuota)

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where(
		"user_id = ? AND using_group = ? AND origin_model = ?",
		user.Id,
		"monthly-zero-e2e",
		"gpt-monthly-zero-tiered-e2e",
	).First(&usage).Error)
	assert.EqualValues(t, 15, usage.OriginalQuota)
	assert.EqualValues(t, 12, usage.ChargedQuota)
	assert.Equal(t, "15", usage.ProgressQuota)

	var settlement model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.First(&settlement).Error)
	assert.EqualValues(t, 15, settlement.OriginalQuota)
	assert.EqualValues(t, 12, settlement.ChargedQuota)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
	assert.True(t, settlement.AccountingApplied)
}

func TestGroupModelMonthlyDiscountAutoGroupRetryCreatesBillingSessionForFinalPaidGroupE2E(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(
		&model.Channel{},
		&model.ChannelModelOverride{},
		&model.Log{},
		&model.UserSubscription{},
		&model.UserGroupModelMonthlyUsage{},
		&model.GroupModelDiscountSettlement{},
		&model.GroupModelDiscountAdjustment{},
		&model.BillingRefundOperation{},
		&model.BillingAdmissionReserveOperation{},
	))
	ratio_setting.InitRatioSettings()

	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	originalQuotaPerUnit := common.QuotaPerUnit
	originalLogConsumeEnabled := common.LogConsumeEnabled
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalUserUsableGroups := setting.UserUsableGroups2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()
	originalRetryStatusCodes := operation_setting.AutomaticRetryStatusCodesToString()
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
		common.LogConsumeEnabled = originalLogConsumeEnabled
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUserUsableGroups))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(strconv.Itoa(originalMaxTokenAutoGroups)))
		require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString(originalRetryStatusCodes))
	})

	common.QuotaPerUnit = 1_000
	common.LogConsumeEnabled = true
	common.BatchUpdateEnabled = false
	common.MemoryCacheEnabled = true
	common.RetryTimes = 1
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"auto":"Auto",
		"auto-free-e2e":"Auto free",
		"auto-monthly-e2e":"Auto monthly"
	}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))
	require.NoError(t, operation_setting.AutomaticRetryStatusCodesFromString("500"))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode":                `{"gpt-auto-monthly-e2e":"tiered_expr"}`,
		"billing_setting.billing_expr":                `{"gpt-auto-monthly-e2e":"tier(\"base\", c * 15)"}`,
		"quota_setting.enable_free_model_pre_consume": "false",
		"group_ratio_setting.group_ratio":             `{"auto-free-e2e":0,"auto-monthly-e2e":0.5}`,
		"group_ratio_setting.model_tiered_ratios": `{
			"auto-monthly-e2e":{"gpt-auto-monthly-e2e":{
				"enabled":true,
				"effective_from":0,
				"effective_until":null,
				"timezone":"UTC",
				"tiers":[{"min_monthly_original_quota":0,"ratio":0.8}]
			}}
		}`,
	}))

	var freeGroupAttempts atomic.Int32
	freeGroupUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		freeGroupAttempts.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"error":{"message":"retry on paid group","type":"upstream_error"}}`))
	}))
	t.Cleanup(freeGroupUpstream.Close)

	var monthlyGroupAttempts atomic.Int32
	monthlyGroupUpstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		monthlyGroupAttempts.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"id":"chatcmpl-auto-monthly",
			"object":"chat.completion",
			"created":1,
			"model":"gpt-auto-monthly-e2e",
			"choices":[{"index":0,"message":{"role":"assistant","content":"paid retry"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":1,"completion_tokens":1000,"total_tokens":1001}
		}`))
	}))
	t.Cleanup(monthlyGroupUpstream.Close)

	user := model.User{
		Username: "auto-monthly-discount-e2e",
		Status:   common.UserStatusEnabled,
		Group:    "auto-free-e2e",
		Quota:    1_000,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{
		UserId:          user.Id,
		Key:             "automonthlydiscounte2ekey",
		Name:            "auto-monthly-discount-token",
		Status:          common.TokenStatusEnabled,
		ExpiredTime:     -1,
		RemainQuota:     1_000,
		UnlimitedQuota:  false,
		Group:           "auto",
		CrossGroupRetry: true,
	}
	require.NoError(t, token.SetAutoGroups([]string{"auto-free-e2e", "auto-monthly-e2e"}))
	require.NoError(t, model.DB.Create(&token).Error)

	priority := int64(100)
	freeGroupChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "auto-free-monthly-upstream",
		Key:      "free-test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(freeGroupUpstream.URL),
		Models:   "gpt-auto-monthly-e2e",
		Group:    "auto-free-e2e",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&freeGroupChannel).Error)
	require.NoError(t, freeGroupChannel.AddAbilities(nil))
	monthlyGroupChannel := model.Channel{
		Type:     constant.ChannelTypeOpenAI,
		Name:     "auto-monthly-upstream",
		Key:      "monthly-test-key",
		Status:   common.ChannelStatusEnabled,
		BaseURL:  common.GetPointer(monthlyGroupUpstream.URL),
		Models:   "gpt-auto-monthly-e2e",
		Group:    "auto-monthly-e2e",
		Priority: &priority,
	}
	require.NoError(t, model.DB.Create(&monthlyGroupChannel).Error)
	require.NoError(t, monthlyGroupChannel.AddAbilities(nil))
	model.InitChannelCache()

	engine := gin.New()
	SetRelayRouter(engine)
	body := []byte(`{"model":"gpt-auto-monthly-e2e","messages":[{"role":"user","content":"hello"}]}`)
	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer automonthlydiscounte2ekey")
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Choices, 1)
	assert.Equal(t, "paid retry", payload.Choices[0].Message.Content)
	assert.GreaterOrEqual(t, freeGroupAttempts.Load(), int32(1))
	assert.Equal(t, int32(1), monthlyGroupAttempts.Load())

	require.NoError(t, model.DB.First(&user, user.Id).Error)
	assert.Equal(t, 988, user.Quota)
	assert.Equal(t, 12, user.UsedQuota)
	assert.Equal(t, 1, user.RequestCount)
	require.NoError(t, model.DB.First(&token, token.Id).Error)
	assert.Equal(t, 988, token.RemainQuota)
	assert.Equal(t, 12, token.UsedQuota)
	require.NoError(t, model.DB.First(&freeGroupChannel, freeGroupChannel.Id).Error)
	assert.Zero(t, freeGroupChannel.UsedQuota)
	require.NoError(t, model.DB.First(&monthlyGroupChannel, monthlyGroupChannel.Id).Error)
	assert.EqualValues(t, 12, monthlyGroupChannel.UsedQuota)

	var admissionOperations []model.BillingAdmissionReserveOperation
	require.NoError(t, model.DB.Order("id ASC").Find(&admissionOperations).Error)
	assert.Empty(t, admissionOperations, "output-only pricing reserves no input cost; final settlement must still charge the monthly group")

	var freeGroupUsageCount int64
	require.NoError(t, model.DB.Model(&model.UserGroupModelMonthlyUsage{}).
		Where("user_id = ? AND using_group = ? AND origin_model = ?", user.Id, "auto-free-e2e", "gpt-auto-monthly-e2e").
		Count(&freeGroupUsageCount).Error)
	assert.Zero(t, freeGroupUsageCount, "the free first group must not create a monthly cursor")

	var usage model.UserGroupModelMonthlyUsage
	require.NoError(t, model.DB.Where(
		"user_id = ? AND using_group = ? AND origin_model = ?",
		user.Id,
		"auto-monthly-e2e",
		"gpt-auto-monthly-e2e",
	).First(&usage).Error)
	assert.EqualValues(t, 15, usage.OriginalQuota)
	assert.EqualValues(t, 12, usage.ChargedQuota)
	assert.Equal(t, "15", usage.ProgressQuota)

	var settlements []model.GroupModelDiscountSettlement
	require.NoError(t, model.DB.Order("id ASC").Find(&settlements).Error)
	require.Len(t, settlements, 1, "only the final using group may own the settlement")
	settlement := settlements[0]
	assert.Equal(t, "auto-monthly-e2e", settlement.UsingGroup)
	assert.Equal(t, "gpt-auto-monthly-e2e", settlement.OriginModel)
	assert.EqualValues(t, 15, settlement.OriginalQuota)
	assert.EqualValues(t, 12, settlement.ChargedQuota)
	assert.Equal(t, groupdiscount.ProgressBasisOriginal, settlement.ProgressBasis)
	assert.Equal(t, model.GroupModelDiscountStatusSettled, settlement.Status)
	assert.True(t, settlement.AccountingApplied)
	assert.Equal(t, user.Id, settlement.AccountingUserID)
	assert.Equal(t, monthlyGroupChannel.Id, settlement.AccountingChannelID)
	assert.Equal(t, 12, settlement.AccountingQuotaDelta)
	assert.Equal(t, 1, settlement.AccountingRequestCountDelta)

	var logs []model.Log
	require.NoError(t, model.DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, "auto-monthly-e2e", logs[0].Group)
	assert.Equal(t, monthlyGroupChannel.Id, logs[0].ChannelId)
	assert.Equal(t, 12, logs[0].Quota)
	assert.Contains(t, logs[0].Other, `"progress_basis":"original"`)
}

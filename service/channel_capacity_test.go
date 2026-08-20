package service

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelcapacity"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	relaytypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type unavailableCapacityReplayBody struct{}

func (unavailableCapacityReplayBody) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (unavailableCapacityReplayBody) Size() int64 {
	return 0
}

func (unavailableCapacityReplayBody) NewReader() (io.ReadCloser, error) {
	return nil, errors.New("body replay unavailable")
}

func configureChannelCapacityTestRuntime(t *testing.T, now time.Time) {
	t.Helper()
	originalRedisEnabled := common.RedisEnabled
	originalLimiter := channelCapacityMemoryLimiter
	originalNow := channelCapacityNow
	common.RedisEnabled = false
	channelCapacityMemoryLimiter = channelcapacity.NewMemoryLimiter()
	channelCapacityNow = func() time.Time { return now }
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		channelCapacityMemoryLimiter = originalLimiter
		channelCapacityNow = originalNow
	})
}

func setCandidateCapacity(t *testing.T, channelID int, modelName string, rpm int64, tpm int64) {
	t.Helper()
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{
		ChannelId: channelID,
		Model:     modelName,
		RPM:       &rpm,
		TPM:       &tpm,
	}}))
}

func runFinalCapacityAdmissionTestRequest(
	t *testing.T,
	param *RetryParam,
	tokens int64,
) (*model.Channel, error) {
	t.Helper()
	param.CapacityTokens = &tokens
	BindChannelModelCapacityRequest(param)
	for {
		selected, _, err := CacheGetRandomSatisfiedChannel(param)
		if err != nil || selected == nil {
			return selected, err
		}
		param.MarkAttempted(selected.Id)
		info := &relaycommon.RelayInfo{
			OriginModelName: param.ModelName,
			ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: selected.Id},
		}
		err = AcquireFinalChannelModelCapacity(param.Ctx, info, tokens)
		if err == nil {
			return selected, nil
		}
		var denied *ChannelModelCapacityAdmissionError
		if !errors.As(err, &denied) {
			return nil, err
		}
	}
}

func TestStaticSelectionSkipsChannelWhenEitherRPMOrTPMMinuteBudgetIsFull(t *testing.T) {
	tests := []struct {
		name         string
		highRPM      int64
		highTPM      int64
		firstTokens  int64
		secondTokens int64
	}{
		{name: "rpm", highRPM: 1, firstTokens: 1, secondTokens: 1},
		{name: "tpm", highTPM: 100, firstTokens: 60, secondTokens: 50},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, false)
			configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
			const modelName = "capacity-model"
			createPrioritizedChannelSelectFixture(t, model.DB, 6101, "default", modelName, 100)
			createPrioritizedChannelSelectFixture(t, model.DB, 6102, "default", modelName, 10)
			setCandidateCapacity(t, 6101, modelName, test.highRPM, test.highTPM)
			model.InitChannelCache()

			firstRetry := 0
			first, err := runFinalCapacityAdmissionTestRequest(t, &RetryParam{
				Ctx:         newChannelSelectContext(),
				TokenGroup:  "default",
				ModelName:   modelName,
				RequestPath: "/v1/chat/completions",
				Retry:       &firstRetry,
			}, test.firstTokens)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 6101, first.Id)

			secondRetry := 0
			second, err := runFinalCapacityAdmissionTestRequest(t, &RetryParam{
				Ctx:         newChannelSelectContext(),
				TokenGroup:  "default",
				ModelName:   modelName,
				RequestPath: "/v1/chat/completions",
				Retry:       &secondRetry,
			}, test.secondTokens)
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Equal(t, 6102, second.Id)
			assert.Zero(t, secondRetry, "capacity spillover must not consume an upstream retry")
		})
	}
}

func TestEstimateFinalChannelModelCapacityUsesTransformedOutboundBody(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "gpt-4o-mini"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)

	tests := []struct {
		name        string
		format      relaytypes.RelayFormat
		finalFormat relaytypes.RelayFormat
		mode        int
		body        string
		wantTokens  int64
	}{
		{
			name:       "claude handler raised explicit zero to thinking minimum",
			format:     relaytypes.RelayFormatClaude,
			mode:       relayconstant.RelayModeChatCompletions,
			body:       `{"model":"claude","messages":[{"role":"user","content":"hello"}],"max_tokens":1280}`,
			wantTokens: int64(CountTextToken("user\nhello", modelName) + 1280),
		},
		{
			name:       "openai output choices multiply the maximum output",
			format:     relaytypes.RelayFormatOpenAI,
			mode:       relayconstant.RelayModeCompletions,
			body:       `{"model":"gpt","prompt":"hello","max_tokens":100,"n":3}`,
			wantTokens: int64(CountTextToken("hello", modelName) + 3 + 300),
		},
		{
			name:        "converted openai body uses final format overhead",
			format:      relaytypes.RelayFormatClaude,
			finalFormat: relaytypes.RelayFormatOpenAI,
			mode:        relayconstant.RelayModeChatCompletions,
			body:        `{"model":"gpt","messages":[{"role":"user","content":"hello"}],"max_tokens":10}`,
			wantTokens:  int64(CountTextToken("user\nhello", modelName) + 6 + 10),
		},
		{
			name:   "gemini batch sums every request and candidate",
			format: relaytypes.RelayFormatGemini,
			mode:   relayconstant.RelayModeChatCompletions,
			body: `{"requests":[
				{"contents":[{"role":"user","parts":[{"text":"first"}]}],"generationConfig":{"maxOutputTokens":10,"candidateCount":2}},
				{"contents":[{"role":"user","parts":[{"text":"second"}]}],"generationConfig":{"maxOutputTokens":20}}
			]}`,
			wantTokens: int64(CountTextToken("first", modelName) + CountTextToken("second", modelName) + 40),
		},
		{
			name:       "embedding counts transformed inputs",
			format:     relaytypes.RelayFormatEmbedding,
			mode:       relayconstant.RelayModeEmbeddings,
			body:       `{"model":"embed","input":["first","second"]}`,
			wantTokens: int64(CountTextToken("first\nsecond", modelName)),
		},
		{
			name:       "rerank counts transformed query and documents",
			format:     relaytypes.RelayFormatRerank,
			mode:       relayconstant.RelayModeRerank,
			body:       `{"model":"rerank","query":"needle","documents":["first","second"]}`,
			wantTokens: int64(CountTextToken("first\nsecond\nneedle", modelName)),
		},
		{
			name:       "image counts transformed prompt only",
			format:     relaytypes.RelayFormatOpenAIImage,
			mode:       relayconstant.RelayModeImagesGenerations,
			body:       `{"model":"image","prompt":"painted lighthouse","n":4}`,
			wantTokens: int64(CountTextToken("painted lighthouse", modelName)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, closer, err := relaycommon.NewOutboundJSONBody([]byte(test.body))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, closer.Close()) })
			info := &relaycommon.RelayInfo{
				OriginModelName:         modelName,
				RelayFormat:             test.format,
				FinalRequestRelayFormat: test.finalFormat,
				RelayMode:               test.mode,
			}

			got, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 1)

			require.NoError(t, err)
			assert.Equal(t, test.wantTokens, got)
		})
	}
}

func TestEstimateFinalGeminiCapacityIncludesToolAndResponseSchemas(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "gemini-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatGemini,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}

	estimate := func(bodyJSON string) int64 {
		t.Helper()
		body, closer, err := relaycommon.NewOutboundJSONBody([]byte(bodyJSON))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closer.Close()) })
		tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 0)
		require.NoError(t, err)
		return tokens
	}

	withoutSchema := estimate(`{
		"contents":[{"role":"user","parts":[{"text":"hello"}]}],
		"generationConfig":{"maxOutputTokens":10}
	}`)
	withSchema := estimate(`{
		"contents":[{"role":"user","parts":[{"text":"hello"}]}],
		"tools":[{"functionDeclarations":[{"name":"lookup","description":"search a private knowledge base","parameters":{"type":"object","properties":{"query":{"type":"string","description":"the exact search phrase"}}}}]}],
		"generationConfig":{"maxOutputTokens":10,"responseSchema":{"type":"object","properties":{"answer":{"type":"string","description":"the final answer"}}}}
	}`)

	assert.Greater(t, withSchema, withoutSchema)
}

func TestEstimateFinalGeminiEmbeddingCapacityNeverDropsInitialReservation(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "gemini-embedding-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatGemini,
		RelayMode:       relayconstant.RelayModeEmbeddings,
		RequestURLPath:  "/v1beta/models/text-embedding:embedContent",
	}

	tests := []struct {
		name     string
		body     string
		fallback int64
		want     int64
	}{
		{name: "single preserves fallback", body: `{"content":{"parts":[{"text":"embedded text"}]}}`, fallback: 123, want: 123},
		{name: "batch preserves fallback", body: `{"requests":[{"model":"models/text-embedding","content":{"parts":[{"text":"first"}]}},{"model":"models/text-embedding","content":{"parts":[{"text":"second"}]}}]}`, fallback: 123, want: 123},
		{name: "batch counts transformed content", body: `{"requests":[{"model":"models/text-embedding","content":{"parts":[{"text":"first transformed input"}]}},{"model":"models/text-embedding","content":{"parts":[{"text":"second transformed input"}]}}]}`, fallback: 1, want: int64(CountTextToken("first transformed input\nsecond transformed input", modelName))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, closer, err := relaycommon.NewOutboundJSONBody([]byte(test.body))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, closer.Close()) })

			tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, test.fallback)

			require.NoError(t, err)
			assert.Equal(t, test.want, tokens)
		})
	}
}

func TestFinalGeminiEmbeddingCapacityIncludesNativeTitle(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
	const (
		channelID = 6169
		modelName = "gemini-embedding-title-capacity-model"
	)
	createPrioritizedChannelSelectFixture(t, model.DB, channelID, "default", modelName, 100)

	c := newChannelSelectContext()
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	fallback := int64(1)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatGemini,
		RelayMode:       relayconstant.RelayModeEmbeddings,
		RequestURLPath:  "/v1beta/models/text-embedding:embedContent",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: channelID},
	}
	baselineBody, baselineCloser, err := relaycommon.NewOutboundJSONBody([]byte(`{
		"content":{"parts":[{"text":"a"}]}
	}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, baselineCloser.Close()) })
	baseline, err := EstimateFinalChannelModelCapacityTokens(c, info, baselineBody, fallback)
	require.NoError(t, err)
	setCandidateCapacity(t, channelID, modelName, 0, baseline)
	model.InitChannelCache()

	param := &RetryParam{
		Ctx:            c,
		TokenGroup:     "default",
		ModelName:      modelName,
		CapacityTokens: &fallback,
	}
	BindChannelModelCapacityRequest(param)
	body, closer, err := relaycommon.NewOutboundJSONBody([]byte(`{
		"content":{"parts":[{"text":"a"}]},
		"title":"a native Gemini document title that must consume the same TPM window"
	}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })

	err = AdmitFinalChannelModelCapacity(c, info, body)

	var admissionErr *ChannelModelCapacityAdmissionError
	require.ErrorAs(t, err, &admissionErr)
	assert.Equal(t, channelID, admissionErr.ChannelID)
}

func TestFinalCapacityIncludesOpaquePromptAndToolHistory(t *testing.T) {
	const modelName = "opaque-final-prompt-capacity-model"
	tests := []struct {
		name        string
		relayFormat relaytypes.RelayFormat
		relayMode   int
		requestPath string
		baseline    string
		expanded    string
	}{
		{
			name:        "OpenAI tool calls and response schema",
			relayFormat: relaytypes.RelayFormatOpenAI,
			relayMode:   relayconstant.RelayModeChatCompletions,
			baseline:    `{"messages":[{"role":"user","content":"a"}],"max_tokens":0}`,
			expanded: `{
				"messages":[
					{"role":"user","content":"a"},
					{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":"a very long serialized function argument that is part of the final upstream prompt history"}}]}
				],
				"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object","properties":{"answer":{"type":"string","description":"a detailed structured answer returned for the caller"}}}}},
				"max_tokens":0
			}`,
		},
		{
			name:        "OpenAI completions numeric token prompt",
			relayFormat: relaytypes.RelayFormatOpenAI,
			relayMode:   relayconstant.RelayModeCompletions,
			baseline:    `{"prompt":[],"max_tokens":0}`,
			expanded:    `{"prompt":[101,102,103,104,105,106,107,108,109,110],"max_tokens":0}`,
		},
		{
			name:        "Responses function output",
			relayFormat: relaytypes.RelayFormatOpenAIResponses,
			relayMode:   relayconstant.RelayModeResponses,
			baseline:    `{"input":[{"role":"user","content":[{"type":"input_text","text":"a"}]}],"max_output_tokens":0}`,
			expanded: `{
				"input":[
					{"role":"user","content":[{"type":"input_text","text":"a"}]},
					{"type":"function_call_output","call_id":"call_1","output":"a large tool result that the model must read as part of the next Responses turn"}
				],
				"max_output_tokens":0
			}`,
		},
		{
			name:        "OpenAI embedding numeric token input",
			relayFormat: relaytypes.RelayFormatEmbedding,
			relayMode:   relayconstant.RelayModeEmbeddings,
			baseline:    `{"input":[]}`,
			expanded:    `{"input":[[101,102,103,104,105],[106,107,108,109,110]]}`,
		},
		{
			name:        "Gemini function response and executable history",
			relayFormat: relaytypes.RelayFormatGemini,
			relayMode:   relayconstant.RelayModeChatCompletions,
			requestPath: "/v1beta/models/gemini:generateContent",
			baseline:    `{"contents":[{"role":"user","parts":[{"text":"a"}]}],"generationConfig":{"maxOutputTokens":0}}`,
			expanded: `{
				"contents":[{"role":"user","parts":[
					{"text":"a"},
					{"functionResponse":{"name":"lookup","response":{"result":"a large function response that remains in the native Gemini prompt history"}}},
					{"executableCode":{"language":"PYTHON","code":"print('a nontrivial program supplied to the model')"}},
					{"codeExecutionResult":{"outcome":"OUTCOME_OK","output":"a long execution result supplied back to the model"}}
				]}],
				"generationConfig":{"maxOutputTokens":0}
			}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := newChannelSelectContext()
			common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
			info := &relaycommon.RelayInfo{
				OriginModelName: modelName,
				RelayFormat:     test.relayFormat,
				RelayMode:       test.relayMode,
				RequestURLPath:  test.requestPath,
			}
			estimate := func(data string) int64 {
				t.Helper()
				body, closer, err := relaycommon.NewOutboundJSONBody([]byte(data))
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, closer.Close()) })
				tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 0)
				require.NoError(t, err)
				return tokens
			}

			assert.Greater(t, estimate(test.expanded), estimate(test.baseline))
		})
	}
}

func TestFinalCapacityAddsOpaqueToolHistoryToTypedMediaTokens(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "media-and-tool-history-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}
	estimate := func(data string) int64 {
		t.Helper()
		body, closer, err := relaycommon.NewOutboundJSONBody([]byte(data))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, closer.Close()) })
		tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 0)
		require.NoError(t, err)
		return tokens
	}
	mediaOnly := estimate(`{
		"messages":[{"role":"user","content":[{"type":"video_url","video_url":{"url":"https://example.invalid/video.mp4"}}]}],
		"max_tokens":0
	}`)
	opaqueOnly := estimate(`{
		"messages":[{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"a large opaque tool result carried into the next request for the model to inspect carefully"}}]}],
		"max_tokens":0
	}`)
	combined := estimate(`{
		"messages":[
			{"role":"user","content":[{"type":"video_url","video_url":{"url":"https://example.invalid/video.mp4"}}]},
			{"role":"assistant","content":"","tool_calls":[{"type":"function","function":{"name":"lookup","arguments":"a large opaque tool result carried into the next request for the model to inspect carefully"}}]}
		],
		"max_tokens":0
	}`)

	assert.Greater(t, combined, mediaOnly)
	assert.Greater(t, combined, opaqueOnly)
}

func TestFinalCapacityEstimationDoesNotOverwriteBillingPromptTokens(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "capacity-billing-context-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	common.SetContextKey(c, constant.ContextKeyPromptTokens, 77)
	body, closer, err := relaycommon.NewOutboundJSONBody([]byte(`{
		"model":"gpt","messages":[{"role":"user","content":"a much longer final outbound prompt"}],"max_tokens":10
	}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}

	_, err = EstimateFinalChannelModelCapacityTokens(c, info, body, 1)

	require.NoError(t, err)
	assert.Equal(t, 77, common.GetContextKeyInt(c, constant.ContextKeyPromptTokens))
}

func TestEstimateFinalProviderSpecificOutputLimitsFromOutboundJSON(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "provider-specific-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}

	tests := []struct {
		name        string
		body        string
		outputLimit int64
		prompt      string
	}{
		{
			name:        "aws nova nested inference config",
			body:        `{"schemaVersion":"messages-v1","messages":[{"role":"user","content":[{"text":"hello nova"}]}],"inferenceConfig":{"maxTokens":10000}}`,
			outputLimit: 10000,
			prompt:      "hello nova",
		},
		{
			name:        "ollama nested options",
			body:        `{"model":"llama","messages":[{"role":"user","content":"hello ollama"}],"options":{"num_predict":9000}}`,
			outputLimit: 9000,
			prompt:      "hello ollama",
		},
		{
			name:        "xunfei nested chat parameters",
			body:        `{"header":{"app_id":"app"},"parameter":{"chat":{"max_tokens":10000}},"payload":{"message":{"text":"hello spark"}}}`,
			outputLimit: 10000,
			prompt:      "hello spark",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, closer, err := relaycommon.NewOutboundJSONBody([]byte(test.body))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, closer.Close()) })

			tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 100)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, tokens, test.outputLimit+int64(CountTextToken(test.prompt, modelName)))
		})
	}
}

func TestEstimateFinalXunfeiPromptSupplementDoesNotDependOnMaxTokens(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "xunfei-omitted-max-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}
	fallback := defaultChannelModelCapacityOutputTokens + 1
	body, closer, err := relaycommon.NewOutboundJSONBody([]byte(`{
		"header":{"app_id":"app"},
		"parameter":{"chat":{"domain":"4.0Ultra"}},
		"payload":{"message":{"text":[
			{"role":"user","content":"a long channel system prompt added only after the Xunfei provider conversion"},
			{"role":"assistant","content":"Okay"},
			{"role":"user","content":"a"}
		]}}
	}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })

	tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, fallback)

	require.NoError(t, err)
	assert.Greater(t, tokens, fallback)
}

func TestEstimateFinalProviderSpecificSmallOutputLimitDoesNotUseDefault(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "provider-small-output-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}
	tests := []struct {
		name  string
		body  string
		limit int64
	}{
		{name: "Xunfei", body: `{"parameter":{"chat":{"max_tokens":321}},"payload":{"message":{"text":[{"role":"user","content":"hello"}]}}}`, limit: 321},
		{name: "AWS Nova", body: `{"messages":[{"role":"user","content":[{"text":"hello"}]}],"inferenceConfig":{"maxTokens":1000}}`, limit: 1000},
		{name: "Ollama", body: `{"messages":[{"role":"user","content":"hello"}],"options":{"num_predict":1000}}`, limit: 1000},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, closer, err := relaycommon.NewOutboundJSONBody([]byte(test.body))
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, closer.Close()) })

			tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 0)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, tokens, test.limit)
			assert.Less(t, tokens, defaultChannelModelCapacityOutputTokens)
		})
	}
}

func TestEstimateFinalCapacityUsesLargestMixedOutputLimit(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "mixed-output-limit-capacity-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}
	body, closer, err := relaycommon.NewOutboundJSONBody([]byte(`{
		"messages":[{"role":"user","content":"hello"}],
		"max_tokens":1000,
		"options":{"num_predict":10000}
	}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })

	tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, 0)

	require.NoError(t, err)
	assert.GreaterOrEqual(t, tokens, int64(10_000))
}

func TestEstimateFinalCapacityUsesReducedOutboundOutputLimit(t *testing.T) {
	c := newChannelSelectContext()
	const modelName = "capacity-policy-output-reduction-model"
	common.SetContextKey(c, constant.ContextKeyOriginalModel, modelName)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		RelayMode:       relayconstant.RelayModeChatCompletions,
	}
	initialPromptTokens := CountTextToken("user\nhello", modelName) + 6
	info.SetEstimatePromptTokens(initialPromptTokens)
	body, closer, err := relaycommon.NewOutboundJSONBody([]byte(`{
		"messages":[{"role":"user","content":"hello"}],
		"max_tokens":1000
	}`))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, closer.Close()) })
	initialReservation := int64(initialPromptTokens + 10_000)

	tokens, err := EstimateFinalChannelModelCapacityTokens(c, info, body, initialReservation)

	require.NoError(t, err)
	assert.Equal(t, int64(initialPromptTokens+1000), tokens)
	assert.Less(t, tokens, initialReservation)
}

func TestFinalChannelModelCapacityAdmissionIsAtomicAndMarksDeniedChannel(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
	const modelName = "final-admission-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6171, "default", modelName, 100)
	limit := int64(100)
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{
		ChannelId: 6171,
		Model:     modelName,
		TPM:       &limit,
	}}))
	model.InitChannelCache()

	firstContext := newChannelSelectContext()
	first := &RetryParam{Ctx: firstContext, TokenGroup: "default", ModelName: modelName}
	BindChannelModelCapacityRequest(first)
	info := &relaycommon.RelayInfo{OriginModelName: modelName, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 6171}}
	require.NoError(t, AcquireFinalChannelModelCapacity(firstContext, info, 60))

	secondContext := newChannelSelectContext()
	second := &RetryParam{Ctx: secondContext, TokenGroup: "default", ModelName: modelName}
	BindChannelModelCapacityRequest(second)
	err := AcquireFinalChannelModelCapacity(secondContext, info, 50)
	var denied *ChannelModelCapacityAdmissionError
	require.ErrorAs(t, err, &denied)
	assert.Equal(t, 6171, denied.ChannelID)
	assert.Equal(t, modelName, denied.Model)
	assert.Contains(t, second.CapacityBlockedChannelIds, 6171)
}

func TestRPMOnlyAdmissionDoesNotRequireFinalTokenReplay(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
	const modelName = "rpm-only-final-admission-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6181, "default", modelName, 100)
	setCandidateCapacity(t, 6181, modelName, 1, 0)
	model.InitChannelCache()

	ctx := newChannelSelectContext()
	param := &RetryParam{Ctx: ctx, TokenGroup: "default", ModelName: modelName}
	BindChannelModelCapacityRequest(param)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		RelayFormat:     relaytypes.RelayFormatOpenAI,
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: 6181},
	}

	err := AdmitFinalChannelModelCapacity(ctx, info, unavailableCapacityReplayBody{})

	require.NoError(t, err)
}

func TestSelectionReturnsCapacityErrorOnlyAfterEveryCandidateIsFull(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, false)
	configureChannelCapacityTestRuntime(t, time.Unix(125, 0))
	const modelName = "all-full-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6201, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, model.DB, 6202, "default", modelName, 10)
	setCandidateCapacity(t, 6201, modelName, 1, 0)
	setCandidateCapacity(t, 6202, modelName, 1, 0)
	model.InitChannelCache()

	for _, wantChannelID := range []int{6201, 6202} {
		retry := 0
		tokens := int64(1)
		selected, err := runFinalCapacityAdmissionTestRequest(t, &RetryParam{
			Ctx:         newChannelSelectContext(),
			TokenGroup:  "default",
			ModelName:   modelName,
			RequestPath: "/v1/chat/completions",
			Retry:       &retry,
		}, tokens)
		require.NoError(t, err)
		require.NotNil(t, selected)
		assert.Equal(t, wantChannelID, selected.Id)
	}

	retry := 0
	tokens := int64(1)
	selected, err := runFinalCapacityAdmissionTestRequest(t, &RetryParam{
		Ctx:         newChannelSelectContext(),
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}, tokens)
	assert.Nil(t, selected)
	var capacityErr *ChannelModelCapacityError
	require.True(t, errors.As(err, &capacityErr))
	assert.Equal(t, 55*time.Second, capacityErr.RetryAfter)
}

func TestAutoCapacityExhaustionRespectsCrossGroupRetry(t *testing.T) {
	tests := []struct {
		name            string
		crossGroupRetry bool
		wantSecondID    int
		wantExhausted   bool
	}{
		{name: "disabled returns current group capacity error", wantExhausted: true},
		{name: "enabled spills into next group", crossGroupRetry: true, wantSecondID: 6232},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setupChannelSelectAutoGroupsTest(t)
			configureDynamicRoutingForTest(t, false)
			configureChannelCapacityTestRuntime(t, time.Unix(125, 0))
			const modelName = "auto-capacity-model"
			createPrioritizedChannelSelectFixture(t, model.DB, 6231, "vip", modelName, 100)
			createPrioritizedChannelSelectFixture(t, model.DB, 6232, "default", modelName, 100)
			setCandidateCapacity(t, 6231, modelName, 1, 0)
			setCandidateCapacity(t, 6232, modelName, 1, 0)
			model.InitChannelCache()

			newParam := func() *RetryParam {
				ctx := newChannelSelectContext()
				common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
				common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, test.crossGroupRetry)
				retry := 0
				param := &RetryParam{
					Ctx:         ctx,
					TokenGroup:  "auto",
					ModelName:   modelName,
					RequestPath: "/v1/chat/completions",
					Retry:       &retry,
				}
				plan, err := ResolveChannelModelCapacityPlan(param)
				require.NoError(t, err)
				require.True(t, plan.Enabled)
				return param
			}

			first, err := runFinalCapacityAdmissionTestRequest(t, newParam(), 0)
			require.NoError(t, err)
			require.NotNil(t, first)
			assert.Equal(t, 6231, first.Id)

			second, err := runFinalCapacityAdmissionTestRequest(t, newParam(), 0)
			if test.wantExhausted {
				assert.Nil(t, second)
				var capacityErr *ChannelModelCapacityError
				require.ErrorAs(t, err, &capacityErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, second)
			assert.Equal(t, test.wantSecondID, second.Id)
		})
	}
}

func TestAutoCapacityErrorPrecedesUnsupportedLaterGroup(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, false)
	configureChannelCapacityTestRuntime(t, time.Unix(125, 0))
	const modelName = "auto-video-capacity-model"
	createPrioritizedChannelSelectFixture(t, db, 6241, "vip", modelName, 100)
	createPrioritizedChannelSelectFixture(t, db, 6242, "default", modelName, 100)
	setChannelSelectVideoResolutions(t, db, 6241, modelName, "1080p")
	setChannelSelectVideoResolutions(t, db, 6242, modelName, "720p")
	setCandidateCapacity(t, 6241, modelName, 1, 0)
	model.InitChannelCache()

	newParam := func() *RetryParam {
		ctx := newChannelSelectContext()
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
		common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)
		retry := 0
		param := &RetryParam{
			Ctx:             ctx,
			TokenGroup:      "auto",
			ModelName:       modelName,
			RequestPath:     "/v1/video/generations",
			VideoResolution: "1080p",
			Retry:           &retry,
		}
		plan, err := ResolveChannelModelCapacityPlan(param)
		require.NoError(t, err)
		require.True(t, plan.Enabled)
		return param
	}

	first, err := runFinalCapacityAdmissionTestRequest(t, newParam(), 0)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 6241, first.Id)

	second, err := runFinalCapacityAdmissionTestRequest(t, newParam(), 0)
	assert.Nil(t, second)
	var capacityErr *ChannelModelCapacityError
	require.ErrorAs(t, err, &capacityErr)
}

func TestDynamicSelectionDoesNotReportCapacityExhaustionWhenAnAttemptedCandidateIsNotFull(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, true)
	configureChannelCapacityTestRuntime(t, time.Unix(125, 0))
	const modelName = "mixed-attempted-capacity-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6251, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, model.DB, 6252, "default", modelName, 10)
	setCandidateCapacity(t, 6252, modelName, 1, 0)
	model.InitChannelCache()

	tokens := int64(0)
	prefillContext := newChannelSelectContext()
	prefill := &RetryParam{Ctx: prefillContext, TokenGroup: "default", ModelName: modelName}
	BindChannelModelCapacityRequest(prefill)
	info := &relaycommon.RelayInfo{OriginModelName: modelName, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 6252}}
	require.NoError(t, AcquireFinalChannelModelCapacity(prefillContext, info, tokens))

	retry := 0
	param := &RetryParam{
		Ctx:                    newChannelSelectContext(),
		TokenGroup:             "default",
		ModelName:              modelName,
		RequestPath:            "/v1/chat/completions",
		DynamicRoutingEligible: true,
		CapacityTokens:         &tokens,
		Retry:                  &retry,
	}
	param.MarkAttempted(6251)
	selected, err := runFinalCapacityAdmissionTestRequest(t, param, tokens)
	assert.Nil(t, selected)
	assert.NoError(t, err, "an attempted but non-full channel means capacity is not globally exhausted")
	assert.Contains(t, param.CapacityBlockedChannelIds, 6252)
}

func TestStaticCapacitySelectionNeverMovesBackToHigherPriorityAfterCurrentTierFills(t *testing.T) {
	candidates := []model.SatisfiedChannelCandidate{
		{ChannelId: 6261, Priority: 10, Weight: 1},
		{ChannelId: 6262, Priority: 5, Weight: 1},
		{ChannelId: 6263, Priority: 0, Weight: 1},
	}

	selected, ok := selectStaticCapacityCandidate(candidates, 2, map[int]struct{}{6263: {}})
	assert.False(t, ok)
	assert.Zero(t, selected.ChannelId)
}

func TestStaticCapacityRetryPreservesExistingSingleChannelRetry(t *testing.T) {
	param := &RetryParam{}
	param.MarkAttempted(6272)
	selected, ok, dynamic, err := chooseCapacityCandidate(
		param,
		"default",
		1,
		[]model.SatisfiedChannelCandidate{{ChannelId: 6272, Priority: 5, Weight: 1, RPM: 10_000}},
	)

	require.NoError(t, err)
	require.True(t, ok)
	assert.False(t, dynamic)
	assert.Equal(t, 6272, selected.ChannelId, "an upstream attempt must not make a capacity-enabled static route skip its existing retry tier")
}

func TestSelectedMiddlewareChannelCanBeRejectedWithoutChargingRetry(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, false)
	configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
	const modelName = "initial-capacity-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6301, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, model.DB, 6302, "default", modelName, 10)
	limit := int64(1)
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{
		ChannelId: 6301,
		Model:     modelName,
		RPM:       &limit,
	}}))
	model.InitChannelCache()

	tokens := int64(1)
	prefillContext := newChannelSelectContext()
	prefill := &RetryParam{Ctx: prefillContext, TokenGroup: "default", ModelName: modelName, CapacityTokens: &tokens}
	BindChannelModelCapacityRequest(prefill)
	info := &relaycommon.RelayInfo{OriginModelName: modelName, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 6301}}
	require.NoError(t, AcquireFinalChannelModelCapacity(prefillContext, info, tokens))

	retry := 0
	requestContext := newChannelSelectContext()
	param := &RetryParam{
		Ctx:            requestContext,
		TokenGroup:     "default",
		ModelName:      modelName,
		RequestPath:    "/v1/chat/completions",
		CapacityTokens: &tokens,
		Retry:          &retry,
	}
	BindChannelModelCapacityRequest(param)
	err := AcquireFinalChannelModelCapacity(requestContext, info, tokens)
	var denied *ChannelModelCapacityAdmissionError
	require.ErrorAs(t, err, &denied)
	assert.Zero(t, retry)
	assert.NotContains(t, param.AttemptedChannelIds, 6301)

	replacement, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, replacement)
	assert.Equal(t, 6302, replacement.Id)
}

func TestDynamicSelectionSpillsAcrossPriorityWhenCapacityIsFullWithoutChargingRetry(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureDynamicRoutingForTest(t, true)
	configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
	const modelName = "dynamic-capacity-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6351, "default", modelName, 100)
	createPrioritizedChannelSelectFixture(t, model.DB, 6352, "default", modelName, 10)
	setCandidateCapacity(t, 6351, modelName, 1, 0)
	model.InitChannelCache()

	selectChannel := func() (*model.Channel, int) {
		t.Helper()
		retry := 0
		tokens := int64(0)
		selected, err := runFinalCapacityAdmissionTestRequest(t, &RetryParam{
			Ctx:                    newChannelSelectContext(),
			TokenGroup:             "default",
			ModelName:              modelName,
			RequestPath:            "/v1/chat/completions",
			DynamicRoutingEligible: true,
			Retry:                  &retry,
		}, tokens)
		require.NoError(t, err)
		return selected, retry
	}

	first, firstRetry := selectChannel()
	require.NotNil(t, first)
	assert.Equal(t, 6351, first.Id)
	assert.Zero(t, firstRetry)

	second, secondRetry := selectChannel()
	require.NotNil(t, second)
	assert.Equal(t, 6352, second.Id)
	assert.Zero(t, secondRetry, "capacity spillover must not consume an upstream retry")
}

func TestResolveChannelModelCapacityPlanOnlyRequiresTokenCountingForTPM(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	const modelName = "capacity-plan-model"
	createPrioritizedChannelSelectFixture(t, model.DB, 6401, "default", modelName, 100)
	model.InitChannelCache()

	param := &RetryParam{
		Ctx:         newChannelSelectContext(),
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
	}
	plan, err := ResolveChannelModelCapacityPlan(param)
	require.NoError(t, err)
	assert.False(t, plan.Enabled)
	assert.False(t, plan.RequiresTokenEstimate)

	setCandidateCapacity(t, 6401, modelName, 10, 0)
	model.InitChannelCache()
	plan, err = ResolveChannelModelCapacityPlan(param)
	require.NoError(t, err)
	assert.True(t, plan.Enabled)
	assert.False(t, plan.RequiresTokenEstimate)

	setCandidateCapacity(t, 6401, modelName, 10, 1000)
	model.InitChannelCache()
	plan, err = ResolveChannelModelCapacityPlan(param)
	require.NoError(t, err)
	assert.True(t, plan.Enabled)
	assert.True(t, plan.RequiresTokenEstimate)
}

func TestResolveChannelModelCapacityPlanSkipsUnsupportedAutoGroupLikeSelection(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "capacity-plan-auto-video-model"
	createChannelSelectAutoGroupsChannel(t, db, 6451, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 6452, "default", modelName)
	setChannelSelectVideoResolutions(t, db, 6451, modelName, "720p")
	setChannelSelectVideoResolutions(t, db, 6452, modelName, "1080p")
	model.InitChannelCache()

	ctx := newChannelSelectContext()
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	plan, err := ResolveChannelModelCapacityPlan(&RetryParam{
		Ctx:             ctx,
		TokenGroup:      "auto",
		ModelName:       modelName,
		RequestPath:     "/v1/video/generations",
		VideoResolution: "1080p",
	})
	require.NoError(t, err)
	assert.False(t, plan.Enabled)
}

func TestForcedChannelCapacityUsesNormalizedModelOverrideOutsideSelectedGroup(t *testing.T) {
	setupChannelSelectAutoGroupsTest(t)
	configureChannelCapacityTestRuntime(t, time.Unix(120, 0))
	const (
		configuredModel = "gpt-4o-gizmo-*"
		publicModel     = "gpt-4o-gizmo-capacity"
	)
	createPrioritizedChannelSelectFixture(t, model.DB, 6501, "vip", configuredModel, 100)
	limit := int64(100)
	require.NoError(t, model.PatchChannelModelOverrides([]model.ChannelModelOverridePatch{{
		ChannelId: 6501,
		Model:     configuredModel,
		TPM:       &limit,
	}}))
	model.InitChannelCache()

	ctx := newChannelSelectContext()
	ctx.Set("specific_channel_id", "6501")
	ctx.Set("channel_id", 6501)
	tokens := int64(101)
	param := &RetryParam{
		Ctx:            ctx,
		TokenGroup:     "default",
		ModelName:      publicModel,
		RequestPath:    "/v1/chat/completions",
		CapacityTokens: &tokens,
	}
	BindChannelModelCapacityRequest(param)
	info := &relaycommon.RelayInfo{OriginModelName: publicModel, ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 6501}}

	err := AcquireFinalChannelModelCapacity(ctx, info, tokens)
	var denied *ChannelModelCapacityAdmissionError
	require.ErrorAs(t, err, &denied)
	assert.Contains(t, param.CapacityBlockedChannelIds, 6501)
}

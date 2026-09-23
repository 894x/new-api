package common

import (
	"fmt"
	"strings"
	"testing"

	commonjson "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWildcardWrappingMatchesSequentialOperations(t *testing.T) {
	for _, tc := range []struct {
		name, input, from, to string
	}{
		{"mixed values", ` {"items":["data:image/png;base64,AAAA",false,null,9007199254740993,{"x":1}]} `, "items.*", "items.*.url"},
		{"nested wrapper", `{"items":[{"media":"a"},{"media":"b"}]}`, "items.*.media", "items.*.media.source.url"},
		{"numeric child", `{"items":["a","b"]}`, "items.*", "items.*.0.url"},
		{"escaped child", `{"items":["a","b"]}`, "items.*", `items.*.a\.b`},
		{"escaped source", `{"items":{"a.b":"a","*":"b","a\\b":"c"}}`, "items.*", "items.*.url"},
		{"escaped strings", `{"items":["line\nquote\"slash\\","\u0061\/b"]}`, "items.*", "items.*.url"},
		{"existing object", `{"items":[{"url":{"url":"a"}},{"url":{}}]}`, "items.*.url", "items.*.url.url"},
		{"negative source", `{"items":[["a","b"],["c"]]}`, "items.*.-1", "items.*.-1.url"},
		{"sibling fallback", `{"items":[{"old":"a"},{"old":"b"}]}`, "items.*.old", "items.*.new"},
		{"child to parent fallback", `{"items":[{"old":"a"},{"old":"b"}]}`, "items.*.old", "items.*"},
		{"empty", `{"items":[]}`, "items.*", "items.*.url"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			op := ParamOperation{Mode: "move", From: tc.from, To: tc.to}
			matches, expanded, err := expandWildcardOperation(input, "", op)
			require.NoError(t, err)
			require.True(t, expanded)
			wantAudit := &paramOverrideAuditRecorder{}
			want, err := applyOperations(input, matches, map[string]any{paramOverrideContextAuditRecorder: wantAudit})
			require.NoError(t, err)
			gotAudit := &paramOverrideAuditRecorder{}
			got, err := applyOperations(input, []ParamOperation{op}, map[string]any{paramOverrideContextAuditRecorder: gotAudit})
			require.NoError(t, err)
			assert.Equal(t, string(want), string(got))
			assert.Equal(t, wantAudit.lines, gotAudit.lines)
			assert.Equal(t, tc.input, string(input), "retry input must not be modified")
		})
	}
}

func TestWildcardMediaWrappingKeepsOrderAndRetryIsolation(t *testing.T) {
	media := "data:image/png;base64," + strings.Repeat("A", 256<<10)
	input := []byte(fmt.Sprintf(`{"messages":[{"content":[{"image_url":%q},{"image_url":%q},{"image_url":{"url":"existing"}}]}],"id":9007199254740993}`, media, media))
	original := string(input)
	var config map[string]any
	require.NoError(t, commonjson.UnmarshalJsonStr(`{"operations":[
		{"mode":"move","from":"messages.*.content.*.image_url","to":"messages.*.content.*.image_url.url","conditions":[{"path":"messages.*.content.*.image_url","mode":"prefix","value":"data:"}]},
		{"mode":"set","path":"checked","value":true,"conditions":[{"path":"messages.0.content.0.image_url.url","mode":"prefix","value":"data:"}]}
	]}`, &config))
	first, err := ApplyParamOverride(input, config, nil)
	require.NoError(t, err)
	want := fmt.Sprintf(`{"messages":[{"content":[{"image_url":{"url":%q}},{"image_url":{"url":%q}},{"image_url":{"url":"existing"}}]}],"id":9007199254740993,"checked":true}`, media, media)
	assert.Equal(t, want, string(first))
	assert.Equal(t, original, string(input))
	retry, err := ApplyParamOverride(input, config, nil)
	require.NoError(t, err)
	assert.Equal(t, first, retry)
	again, err := ApplyParamOverride(first, config, nil)
	require.NoError(t, err)
	assert.Equal(t, first, again)
}

func TestWildcardConditionsKeepQueryValuePathsAndContext(t *testing.T) {
	input := []byte(`{"items":[{"name":"first","enabled":true,"actual":false,"expected":false},{"name":"second","actual":true,"expected":true}],"a.b":{"flag":false}}`)
	operations := []ParamOperation{{
		Mode: "set", Path: "items.*.selected", Value: true, Logic: "AND",
		Conditions: []ConditionOperation{
			{Path: "items.*.actual", Mode: "full", ValuePath: "items.*.expected"},
			{Path: "items.*.name", Mode: "full", ValuePath: `items.#(enabled==true).name`},
			{Path: `a\.b.flag`, Mode: "full", Value: false},
			{Path: "items.-1.name", Mode: "full", Value: "second"},
			{Path: "request_path", Mode: "full", Value: "/v1/chat/completions"},
		},
	}}
	got, err := applyOperations(input, operations, map[string]any{"request_path": "/v1/chat/completions"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"items":[{"name":"first","enabled":true,"actual":false,"expected":false,"selected":true},{"name":"second","actual":true,"expected":true}],"a.b":{"flag":false}}`, string(got))
}

func TestRemoveDisabledFieldsPreservesRawValues(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
		settings          dto.ChannelOtherSettings
	}{
		{
			name:  "raw numbers strings and explicit zero values",
			input: `{"service_tier":"flex","id":9007199254740993,"n":0,"flag":false,"null":null,"media":"data:image/png;base64,AA\/AA","text":"\u0061","stream_options":{"include_usage":false,"include_obfuscation":false}}`,
			want:  `{"id":9007199254740993,"n":0,"flag":false,"null":null,"media":"data:image/png;base64,AA\/AA","text":"\u0061","stream_options":{"include_usage":false}}`,
		},
		{
			name:  "remove all disabled and empty stream options",
			input: `{"service_tier":null,"inference_geo":"eu","speed":"fast","store":false,"safety_identifier":"x","stream_options":{"include_obfuscation":true}}`,
			want:  `{}`, settings: dto.ChannelOtherSettings{DisableStore: true},
		},
		{
			name:  "escaped disabled name and nested names",
			input: `{"service\u005ftier":"flex","nested":{"service_tier":"keep"},"stream_options":"keep"}`,
			want:  `{"nested":{"service_tier":"keep"},"stream_options":"keep"}`,
		},
		{
			name:     "allow stream option while filtering service tier",
			input:    `{"service_tier":"flex","stream_options":{"include_obfuscation":false},"store":false}`,
			want:     `{"stream_options":{"include_obfuscation":false},"store":false}`,
			settings: dto.ChannelOtherSettings{AllowIncludeObfuscation: true},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := []byte(tc.input)
			got, err := RemoveDisabledFields(input, tc.settings, false)
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(got))
			assert.Equal(t, tc.input, string(input))
		})
	}
}

package common

import (
	"os"
	"strings"
	"testing"

	commonjson "github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParamOverrideWildcardBindings(t *testing.T) {
	for _, tc := range []struct {
		name, input, operations, want, errorText string
	}{
		{
			name:       "move nested matches with item and global conditions",
			input:      `{"model":"vision","messages":[{"content":[{"video_url":"https://a"},{"video_url":{"url":"https://b","fps":2}},{"text":"hi"}]},{"content":"text"},{"content":[{"video_url":"http://c"},{"video_url":"https://d"}]}]}`,
			operations: `[{"mode":"move","from":"messages.*.content.*.video_url","to":"messages.*.content.*.video_url.url","logic":"AND","conditions":[{"path":"model","mode":"full","value":"vision"},{"path":"messages.*.content.*.video_url","mode":"prefix","value":"https://"}]}]`,
			want:       `{"model":"vision","messages":[{"content":[{"video_url":{"url":"https://a"}},{"video_url":{"url":"https://b","fps":2}},{"text":"hi"}]},{"content":"text"},{"content":[{"video_url":"http://c"},{"video_url":{"url":"https://d"}}]}]}`,
		},
		{
			name:       "false item condition skips invalid target shape",
			input:      `{"items":[{"url":[]},{"url":"https://a"}]}`,
			operations: `[{"mode":"move","from":"items.*.url","to":"items.*.url.url","conditions":[{"path":"items.*.url","mode":"prefix","value":"https://"}]}]`,
			want:       `{"items":[{"url":[]},{"url":{"url":"https://a"}}]}`,
		},
		{
			name:       "copy binds target captures and keeps exact numeric value",
			input:      `{"items":[{"id":9007199254740993},{"id":false},{"id":null}]}`,
			operations: `[{"mode":"copy","from":"items.*.id","to":"result.*.value"}]`,
			want:       `{"items":[{"id":9007199254740993},{"id":false},{"id":null}],"result":[{"value":9007199254740993},{"value":false},{"value":null}]}`,
		},
		{
			name:       "copy into existing destination array",
			input:      `{"items":[{"id":1},{"id":2}],"result":[]}`,
			operations: `[{"mode":"copy","from":"items.*.id","to":"result.*.value"}]`,
			want:       `{"items":[{"id":1},{"id":2}],"result":[{"value":1},{"value":2}]}`,
		},
		{
			name:       "set condition value_path follows same item and ancestor",
			input:      `{"groups":[{"enabled":true,"items":[{"actual":1,"expected":1},{"actual":2,"expected":1}]},{"enabled":false,"items":[{"actual":1,"expected":1}]}]}`,
			operations: `[{"mode":"set","path":"groups.*.items.*.ok","value":true,"logic":"AND","conditions":[{"path":"groups.*.enabled","mode":"full","value":true},{"path":"groups.*.items.*.actual","mode":"full","value_path":"groups.*.items.*.expected"}]}]`,
			want:       `{"groups":[{"enabled":true,"items":[{"actual":1,"expected":1,"ok":true},{"actual":2,"expected":1}]},{"enabled":false,"items":[{"actual":1,"expected":1}]}]}`,
		},
		{
			name:       "missing condition and invert are item local",
			input:      `{"items":[{"enabled":true},{"enabled":false},{}]}`,
			operations: `[{"mode":"set","path":"items.*.ok","value":true,"conditions":[{"path":"items.*.enabled","mode":"full","value":true,"invert":true,"pass_missing_key":true}]}]`,
			want:       `{"items":[{"enabled":true},{"enabled":false,"ok":true},{"ok":true}]}`,
		},
		{
			name:       "conditions see snapshot before deleting arrays",
			input:      `{"items":[{"remove":true},{"remove":false},{"remove":true},{"remove":true}]}`,
			operations: `[{"mode":"delete","path":"items.*","logic":"AND","conditions":[{"path":"items.#","mode":"full","value":4},{"path":"items.*.remove","mode":"full","value":true}]}]`,
			want:       `{"items":[{"remove":false}]}`,
		},
		{
			name:       "nested arrays delete without shifting remaining matches",
			input:      `{"groups":[[0,1,2],[3,4]]}`,
			operations: `[{"mode":"delete","path":"groups.*.*"}]`,
			want:       `{"groups":[[],[]]}`,
		},
		{
			name:       "move array elements preserves destination indices",
			input:      `{"items":[10,20,30],"result":[]}`,
			operations: `[{"mode":"move","from":"items.*","to":"result.*"}]`,
			want:       `{"items":[],"result":[10,20,30]}`,
		},
		{
			name:       "operations see earlier writes",
			input:      `{"items":[{"old":" x "},{"old":" y "}]}`,
			operations: `[{"mode":"move","from":"items.*.old","to":"items.*.new"},{"mode":"trim_space","path":"items.*.new","conditions":[{"path":"items.*.new","mode":"prefix","value":" "}]}]`,
			want:       `{"items":[{"new":"x"},{"new":"y"}]}`,
		},
		{
			name:       "empty missing and scalar collections skip",
			input:      `{"messages":[{}, {"content":[]}, {"content":"hello"}, {"content":[{"text":"hi"}]}]}`,
			operations: `[{"mode":"move","from":"messages.*.content.*.video_url","to":"messages.*.content.*.video_url.url"}]`,
			want:       `{"messages":[{}, {"content":[]}, {"content":"hello"}, {"content":[{"text":"hi"}]}]}`,
		},
		{
			name:       "object keys are captured literally",
			input:      `{"items":{"a.b":{"old":"x"},"*":{"old":"y"},"a\\b":{"old":"z"},"#":{"old":"q"},"0":{"old":"n"}}}`,
			operations: `[{"mode":"move","from":"items.*.old","to":"items.*.new","conditions":[{"path":"items.*.old","mode":"prefix","value":""}]}]`,
			want:       `{"items":{"a.b":{"new":"x"},"*":{"new":"y"},"a\\b":{"new":"z"},"#":{"new":"q"},"0":{"new":"n"}}}`,
		},
		{
			name:       "negative index below wildcard",
			input:      `{"groups":[["a","b"],["c"]]}`,
			operations: `[{"mode":"set","path":"groups.*.-1","value":"last"}]`,
			want:       `{"groups":[["a","last"],["last"]]}`,
		},
		{
			name:       "replace strings containing stars are not paths",
			input:      `{"items":["a.*.b"]}`,
			operations: `[{"mode":"replace","path":"items.*","from":".*.","to":"-"}]`,
			want:       `{"items":["a-b"]}`,
		},
		{
			name:       "query string containing stars is not item binding",
			input:      `{"items":[{"name":"a.*.b"}]}`,
			operations: `[{"mode":"set","path":"ok","value":true,"conditions":[{"path":"items.#(name==\"a.*.b\")#|#","mode":"full","value":1}]}]`,
			want:       `{"items":[{"name":"a.*.b"}],"ok":true}`,
		},
		{
			name:       "target count mismatch rejected even without matches",
			input:      `{"items":[]}`,
			operations: `[{"mode":"move","from":"items.*.old","to":"result.value"}]`,
			errorText:  "target wildcard count",
		},
		{
			name:       "extra target wildcard rejected",
			input:      `{"items":[]}`,
			operations: `[{"mode":"copy","from":"items.*.old","to":"result.*.*.value"}]`,
			errorText:  "target wildcard count",
		},
		{
			name:       "unrelated condition collection rejected",
			input:      `{"items":[]}`,
			operations: `[{"mode":"set","path":"items.*.ok","value":true,"conditions":[{"path":"other.*.enabled","mode":"full","value":true}]}]`,
			errorText:  "unbound condition wildcard",
		},
		{
			name:       "unbound value_path rejected",
			input:      `{"items":[]}`,
			operations: `[{"mode":"set","path":"items.*.ok","value":true,"conditions":[{"path":"items.*.name","mode":"full","value_path":"other.*.name"}]}]`,
			errorText:  "unbound condition wildcard",
		},
		{
			name:       "return_error cannot guess wildcard aggregation",
			input:      `{}`,
			operations: `[{"mode":"return_error","value":"bad","conditions":[{"path":"items.*.bad","mode":"full","value":true}]}]`,
			errorText:  "unbound condition wildcard",
		},
		{
			name:       "writes into another source rejected",
			input:      `{"items":[1,2]}`,
			operations: `[{"mode":"copy","from":"items.*","to":"items.0.children.*"}]`,
			errorText:  "overlaps another source",
		},
		{
			name:       "array index aliases cannot bypass overlap validation",
			input:      `{"items":[1,2]}`,
			operations: `[{"mode":"copy","from":"items.*","to":"items.00.children.*"}]`,
			errorText:  "overlaps another source",
		},
		{
			name:       "numeric object captures cannot collide in new target array",
			input:      `{"items":{"0":1,"00":2}}`,
			operations: `[{"mode":"copy","from":"items.*","to":"result.*"}]`,
			errorText:  "targets overlap",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var config map[string]interface{}
			require.NoError(t, commonjson.UnmarshalJsonStr(`{"operations":`+tc.operations+`}`, &config))
			input := []byte(tc.input)
			out, err := ApplyParamOverride(input, config, nil)
			assert.Equal(t, tc.input, string(input), "original request must remain available for retry")
			if tc.errorText != "" {
				require.ErrorContains(t, err, tc.errorText)
				assert.Nil(t, out)
				return
			}
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(out))
			if tc.name == "copy binds target captures and keeps exact numeric value" {
				assert.Contains(t, string(out), `9007199254740993`)
				assert.NotContains(t, string(out), `9007199254740992`)
			}
		})
	}
}

func TestParamOverrideOverlappingMove(t *testing.T) {
	for _, tc := range []struct{ name, input, from, to, want string }{
		{"string to child", `{"a":"https://a"}`, "a", "a.url", `{"a":{"url":"https://a"}}`},
		{"object to child", `{"a":{"x":1}}`, "a", "a.wrapped", `{"a":{"wrapped":{"x":1}}}`},
		{"array element to child", `{"a":["x","y"]}`, "a.0", "a.0.url", `{"a":[{"url":"x"},"y"]}`},
		{"child to parent", `{"a":{"b":{"b":1}}}`, "a.b", "a", `{"a":{"b":1}}`},
		{"same path", `{"a":false}`, "a", "a", `{"a":false}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ApplyParamOverride([]byte(tc.input), map[string]interface{}{
				"operations": []map[string]interface{}{{"mode": "move", "from": tc.from, "to": tc.to}},
			}, nil)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, string(out))
		})
	}
}

func TestParamOverrideWildcardMediaAndAllowedToolsConfig(t *testing.T) {
	configBytes, err := os.ReadFile("testdata/override_wildcard_media_allowed_tools.json")
	require.NoError(t, err)
	var config map[string]interface{}
	require.NoError(t, commonjson.Unmarshal(configBytes, &config))
	input := []byte(`{"model":"kimi-k3","tools":[{"type":"function","function":{"name":"weather"}},{"type":"function","function":{"name":"search"}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"function","function":{"name":"weather"}}]}},"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"user","content":[{"type":"video_url","video_url":"https://video"},{"type":"image_url","image_url":"data:image/png;base64,eA=="},{"type":"image_url","image_url":{"url":"https://image","detail":"high"}}]}]}`)
	out, err := ApplyParamOverride(input, config, map[string]interface{}{"request_path": "/v1/chat/completions"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"kimi-k3","tools":[{"type":"function","function":{"name":"weather"}}],"tool_choice":"required","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},{"role":"user","content":[{"type":"video_url","video_url":{"url":"https://video"}},{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="}},{"type":"image_url","image_url":{"url":"https://image","detail":"high"}}]}]}`, string(out))
	// A retry or a second application must not wrap existing URL objects again.
	again, err := ApplyParamOverride(out, config, map[string]interface{}{"request_path": "/v1/chat/completions"})
	require.NoError(t, err)
	assert.JSONEq(t, string(out), string(again))
	// The former enumerated configuration stopped at content.20.
	longInput := []byte(`{"messages":[{"content":[` + strings.Repeat(`{"type":"text","text":"padding"},`, 21) + `{"video_url":"http://after-20"}]}]}`)
	longOut, err := ApplyParamOverride(longInput, config, nil)
	require.NoError(t, err)
	assert.Contains(t, string(longOut), `"video_url":{"url":"http://after-20"}`)
}

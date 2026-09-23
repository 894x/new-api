package common

import (
	"fmt"
	"os"
	"strings"
	"testing"

	commonjson "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

// These benchmarks exercise the production copy/encoding/override boundaries
// with inline media. The checked-in rules include allowed-tools adaptation and
// conditional wildcard wrapping; they are not a snapshot of a live channel.
func BenchmarkRequestBodyPreparation(b *testing.B) {
	rules, err := os.ReadFile("testdata/override_wildcard_media_allowed_tools.json")
	require.NoError(b, err)
	var config map[string]any
	require.NoError(b, commonjson.Unmarshal(rules, &config))
	for _, size := range []int{194, 8 << 20, 38 << 20} {
		for _, images := range []int{1, 8} {
			b.Run(fmt.Sprintf("bytes=%d/images=%d", size, images), func(b *testing.B) {
				media := `{"type":"image_url","image_url":"data:image/png;base64,` + strings.Repeat("A", size/images) + `"}`
				body := []byte(`{"model":"kimi-k3","service_tier":"auto","messages":[{"role":"user","content":[` + strings.TrimSuffix(strings.Repeat(media+",", images), ",") + `]}],"tools":[{"type":"function","function":{"name":"weather"}},{"type":"function","function":{"name":"search"}}],"tool_choice":{"type":"allowed_tools","allowed_tools":{"mode":"required","tools":[{"type":"function","function":{"name":"weather"}}]}}}`)
				context := map[string]any{"request_path": "/v1/chat/completions"}
				var request dto.GeneralOpenAIRequest
				require.NoError(b, commonjson.Unmarshal(body, &request))
				b.Run("copy", func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						_, err := commonjson.DeepCopy(&request)
						require.NoError(b, err)
					}
				})
				b.Run("override", func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(body)))
					for b.Loop() {
						_, err := ApplyParamOverride(body, config, context)
						require.NoError(b, err)
					}
				})
				b.Run("prepare", func(b *testing.B) {
					b.ReportAllocs()
					b.SetBytes(int64(len(body)))
					for b.Loop() {
						copy, err := commonjson.DeepCopy(&request)
						require.NoError(b, err)
						data, err := commonjson.Marshal(copy)
						require.NoError(b, err)
						data, err = RemoveDisabledFields(data, dto.ChannelOtherSettings{}, false)
						require.NoError(b, err)
						_, err = ApplyParamOverride(data, config, context)
						require.NoError(b, err)
					}
				})
			})
		}
	}
}

func BenchmarkRequestBodyRawMessageCopy(b *testing.B) {
	for _, size := range []int{194, 8 << 20, 38 << 20} {
		b.Run(fmt.Sprintf("bytes=%d", size), func(b *testing.B) {
			request := dto.GeneralOpenAIRequest{ExtraBody: []byte(`{"media":"` + strings.Repeat("A", size) + `"}`)}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			for b.Loop() {
				_, err := commonjson.DeepCopy(&request)
				require.NoError(b, err)
			}
		})
	}
}

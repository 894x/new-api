package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignAssetLibraryRequestMatchesCanonicalContract(t *testing.T) {
	body := []byte(`{"Name":"test","Description":"test","GroupType":"AIGC","ProjectName":"default"}`)
	request, err := http.NewRequest(
		http.MethodPost,
		"https://ark.cn-beijing.volcengineapi.com/?Action=CreateAssetGroup&Version=2024-01-01",
		strings.NewReader(string(body)),
	)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	now := time.Date(2026, time.March, 28, 0, 0, 0, 0, time.UTC)

	signAssetLibraryRequest(request, body, "AKTEST", "secret", "cn-beijing", now)

	assert.Equal(t, "20260328T000000Z", request.Header.Get("X-Date"))
	assert.Equal(t, "432606e1bdc8e4577272b798cf9ed056164f0ff348beceebe47b5892d9e72589", request.Header.Get("X-Content-Sha256"))
	assert.Equal(t,
		"HMAC-SHA256 Credential=AKTEST/20260328/cn-beijing/ark/request, "+
			"SignedHeaders=content-type;host;x-content-sha256;x-date, "+
			"Signature=0fa1b583515032a14b0f717870111e3ef5bc6e714e9c9f43e72d8d2a1aea3715",
		request.Header.Get("Authorization"),
	)
}

func TestCallAssetLibraryUpstreamBearerCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "CreateAssetGroup", request.URL.Query().Get("Action"))
		assert.Equal(t, AssetLibraryVersion, request.URL.Query().Get("Version"))
		assert.Equal(t, "Bearer compatible-key", request.Header.Get("Authorization"))
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ResponseMetadata":{"RequestId":"req-1"},"Result":{"Id":"group-upstream"}}`))
	}))
	t.Cleanup(server.Close)

	config := &model.ChannelAssetConfig{
		Enabled:  true,
		BaseURL:  server.URL,
		AuthType: AssetLibraryAuthBearer,
		APIKey:   "compatible-key",
	}
	var result struct {
		Id string `json:"Id"`
	}
	err := CallAssetLibraryUpstream(t.Context(), config, "CreateAssetGroup", map[string]any{"Name": "logical"}, &result)
	require.NoError(t, err)
	assert.Equal(t, "group-upstream", result.Id)
}

func TestChannelAssetConfigJSONNeverContainsCredentials(t *testing.T) {
	config := model.ChannelAssetConfig{
		ChannelId: 1,
		AccessKey: "access-secret",
		SecretKey: "signing-secret",
		APIKey:    "bearer-secret",
	}

	data, err := common.Marshal(config)
	require.NoError(t, err)
	serialized := string(data)
	assert.NotContains(t, serialized, "access-secret")
	assert.NotContains(t, serialized, "signing-secret")
	assert.NotContains(t, serialized, "bearer-secret")
}

package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

const (
	AssetLibraryAuthAKSK       = "aksk"
	AssetLibraryAuthBearer     = "bearer"
	AssetLibraryVersion        = "2024-01-01"
	AssetLibraryService        = "ark"
	DefaultAssetLibraryBaseURL = "https://ark.cn-beijing.volcengineapi.com"
	DefaultAssetLibraryRegion  = "cn-beijing"
	DefaultAssetLibraryProject = "default"
)

type AssetLibraryUpstreamError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *AssetLibraryUpstreamError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type assetLibraryRawResponse struct {
	ResponseMetadata struct {
		RequestId string `json:"RequestId"`
		Error     *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
	} `json:"ResponseMetadata"`
	Result any `json:"Result"`
}

type assetLibraryResultEnvelope struct {
	Result any `json:"Result"`
}

func CallAssetLibraryUpstream(ctx context.Context, config *model.ChannelAssetConfig, action string, input any, output any) error {
	if config == nil || !config.Enabled {
		return errors.New("asset library is not enabled for channel")
	}
	body, err := common.Marshal(input)
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAssetLibraryBaseURL
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid asset library base URL: %w", err)
	}
	query := endpoint.Query()
	query.Set("Action", action)
	query.Set("Version", AssetLibraryVersion)
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	switch config.AuthType {
	case AssetLibraryAuthAKSK:
		if strings.TrimSpace(config.AccessKey) == "" || strings.TrimSpace(config.SecretKey) == "" {
			return errors.New("asset library AK/SK is incomplete")
		}
		region := strings.TrimSpace(config.Region)
		if region == "" {
			region = DefaultAssetLibraryRegion
		}
		signAssetLibraryRequest(request, body, config.AccessKey, config.SecretKey, region, time.Now().UTC())
	case AssetLibraryAuthBearer:
		if strings.TrimSpace(config.APIKey) == "" {
			return errors.New("asset library API key is empty")
		}
		request.Header.Set("Authorization", "Bearer "+config.APIKey)
	default:
		return fmt.Errorf("unsupported asset library auth type %q", config.AuthType)
	}

	client := GetHttpClient()
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	var envelope assetLibraryRawResponse
	if err := common.Unmarshal(responseBody, &envelope); err != nil {
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return &AssetLibraryUpstreamError{StatusCode: response.StatusCode, Message: http.StatusText(response.StatusCode)}
		}
		return fmt.Errorf("decode asset library response: %w", err)
	}
	if envelope.Result == nil {
		var resultEnvelope assetLibraryResultEnvelope
		if err := common.Unmarshal(responseBody, &resultEnvelope); err == nil && resultEnvelope.Result != nil {
			envelope.Result = resultEnvelope.Result
		} else {
			var generic map[string]any
			if err := common.Unmarshal(responseBody, &generic); err == nil {
				if result, ok := generic["data"]; ok {
					envelope.Result = result
				} else {
					envelope.Result = generic
				}
			}
		}
	}
	if envelope.ResponseMetadata.Error != nil {
		return &AssetLibraryUpstreamError{
			StatusCode: response.StatusCode,
			Code:       envelope.ResponseMetadata.Error.Code,
			Message:    common.MaskSensitiveInfo(common.LocalLogPreview(envelope.ResponseMetadata.Error.Message)),
		}
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &AssetLibraryUpstreamError{StatusCode: response.StatusCode, Message: http.StatusText(response.StatusCode)}
	}
	if output == nil {
		return nil
	}
	result, err := common.Marshal(envelope.Result)
	if err != nil {
		return err
	}
	return common.Unmarshal(result, output)
}

func signAssetLibraryRequest(request *http.Request, body []byte, accessKey string, secretKey string, region string, now time.Time) {
	date := now.UTC().Format("20060102T150405Z")
	shortDate := date[:8]
	payloadHash := sha256Hex(body)
	request.Header.Set("X-Date", date)
	request.Header.Set("X-Content-Sha256", payloadHash)

	signedHeaderNames := []string{"content-type", "host", "x-content-sha256", "x-date"}
	canonicalHeaders := strings.Builder{}
	for _, name := range signedHeaderNames {
		value := request.Header.Get(name)
		if name == "host" {
			value = request.URL.Host
		}
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(strings.TrimSpace(value))
		canonicalHeaders.WriteByte('\n')
	}
	canonicalPath := request.URL.EscapedPath()
	if canonicalPath == "" {
		canonicalPath = "/"
	}
	canonicalRequest := strings.Join([]string{
		request.Method,
		canonicalPath,
		strings.ReplaceAll(request.URL.Query().Encode(), "+", "%20"),
		canonicalHeaders.String(),
		strings.Join(signedHeaderNames, ";"),
		payloadHash,
	}, "\n")
	credentialScope := shortDate + "/" + region + "/" + AssetLibraryService + "/request"
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		date,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signingKey := assetLibraryHMAC([]byte(secretKey), shortDate)
	signingKey = assetLibraryHMAC(signingKey, region)
	signingKey = assetLibraryHMAC(signingKey, AssetLibraryService)
	signingKey = assetLibraryHMAC(signingKey, "request")
	signature := hex.EncodeToString(assetLibraryHMAC(signingKey, stringToSign))
	request.Header.Set("Authorization", "HMAC-SHA256 Credential="+accessKey+"/"+credentialScope+
		", SignedHeaders="+strings.Join(signedHeaderNames, ";")+", Signature="+signature)
}

func assetLibraryHMAC(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func sha256Hex(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

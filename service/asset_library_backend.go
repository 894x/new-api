package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	AssetLibraryBackendAction      = "volcengine"
	AssetLibraryBackendSeedanceSLS = "seedance_sls"
	AssetLibraryBackendOpenAPI     = "openapi"
)

type assetLibraryCreateGroupResult struct {
	GroupID  string
	Deferred bool
}

type assetLibraryCreateAssetResult struct {
	AssetID string
	GroupID string
	Status  string
}

type assetLibraryBackend interface {
	CreateGroup(context.Context, *model.ChannelAssetConfig, *model.UserAssetGroup) (*assetLibraryCreateGroupResult, error)
	CreateAsset(context.Context, *model.ChannelAssetConfig, *model.UserAssetGroup, *model.UserAssetGroupReplica, *model.UserAsset) (*assetLibraryCreateAssetResult, error)
	UpdateGroup(context.Context, *model.ChannelAssetConfig, *model.UserAssetGroup, string) error
	UpdateAsset(context.Context, *model.ChannelAssetConfig, *model.UserAsset, string) error
	DeleteGroup(context.Context, *model.ChannelAssetConfig, string) error
	DeleteAsset(context.Context, *model.ChannelAssetConfig, string) error
	GetAsset(context.Context, *model.ChannelAssetConfig, string) (*AssetLibraryAssetDetails, error)
	FormatAssetReference(string) string
}

type actionAssetLibraryBackend struct{}

type seedanceSLSAssetLibraryBackend struct{}

func assetLibraryBackendForChannelType(channelType int) assetLibraryBackend {
	if DefaultAssetLibraryBackend(channelType) == AssetLibraryBackendSeedanceSLS {
		return seedanceSLSAssetLibraryBackend{}
	}
	return actionAssetLibraryBackend{}
}

func assetLibraryBackendForChannel(channelId int) (assetLibraryBackend, error) {
	config, err := model.GetChannelAssetConfig(channelId)
	if err != nil {
		return nil, err
	}
	channel, err := model.GetChannelById(channelId, false)
	if err != nil {
		return nil, err
	}
	backend := strings.TrimSpace(config.Backend)
	if backend == "" {
		backend = DefaultAssetLibraryBackend(channel.Type)
	}
	switch backend {
	case AssetLibraryBackendAction:
		return actionAssetLibraryBackend{}, nil
	case AssetLibraryBackendSeedanceSLS:
		return seedanceSLSAssetLibraryBackend{}, nil
	case AssetLibraryBackendOpenAPI:
		return openAPIAssetLibraryBackend{}, nil
	default:
		return nil, fmt.Errorf("unsupported asset library backend %q", backend)
	}
}

func DefaultAssetLibraryBackend(channelType int) string {
	if channelType == constant.ChannelTypeSeedanceSLS {
		return AssetLibraryBackendSeedanceSLS
	}
	return AssetLibraryBackendAction
}

func DefaultAssetLibraryBackendBaseURL(backend string, channel *model.Channel) string {
	switch backend {
	case AssetLibraryBackendSeedanceSLS:
		return constant.ChannelBaseURLs[constant.ChannelTypeSeedanceSLS]
	case AssetLibraryBackendOpenAPI:
		if channel == nil {
			return ""
		}
		return strings.TrimRight(strings.TrimSpace(channel.GetBaseURL()), "/")
	default:
		return DefaultAssetLibraryBaseURL
	}
}

func DefaultAssetLibraryBackendAuthType(backend string) string {
	if backend == AssetLibraryBackendAction {
		return AssetLibraryAuthAKSK
	}
	return AssetLibraryAuthBearer
}

func IsSupportedAssetLibraryBackend(backend string) bool {
	switch backend {
	case AssetLibraryBackendAction, AssetLibraryBackendSeedanceSLS, AssetLibraryBackendOpenAPI:
		return true
	default:
		return false
	}
}

func AssetLibraryBackendRequiresBearer(backend string) bool {
	return backend == AssetLibraryBackendSeedanceSLS || backend == AssetLibraryBackendOpenAPI
}

func (actionAssetLibraryBackend) CreateGroup(ctx context.Context, config *model.ChannelAssetConfig, group *model.UserAssetGroup) (*assetLibraryCreateGroupResult, error) {
	projectName := assetLibraryProject(config)
	groupType := group.GroupType
	request := dto.CreateAssetGroupRequest{
		Name:        group.Name,
		Description: &group.Description,
		GroupType:   &groupType,
		ProjectName: &projectName,
	}
	var result struct {
		Id string `json:"Id"`
	}
	if err := CallAssetLibraryUpstream(ctx, config, "CreateAssetGroup", request, &result); err != nil {
		return nil, err
	}
	return &assetLibraryCreateGroupResult{GroupID: result.Id}, nil
}

func (actionAssetLibraryBackend) CreateAsset(ctx context.Context, config *model.ChannelAssetConfig, _ *model.UserAssetGroup, groupReplica *model.UserAssetGroupReplica, asset *model.UserAsset) (*assetLibraryCreateAssetResult, error) {
	if groupReplica == nil || strings.TrimSpace(groupReplica.UpstreamGroupId) == "" {
		return nil, errors.New("asset library group replica is unavailable")
	}
	projectName := assetLibraryProject(config)
	request := dto.CreateAssetRequest{
		GroupId:     groupReplica.UpstreamGroupId,
		URL:         asset.SourceURL,
		AssetType:   asset.AssetType,
		Name:        &asset.Name,
		ProjectName: &projectName,
	}
	var result struct {
		Id string `json:"Id"`
	}
	if err := CallAssetLibraryUpstream(ctx, config, "CreateAsset", request, &result); err != nil {
		return nil, err
	}
	return &assetLibraryCreateAssetResult{
		AssetID: result.Id,
		GroupID: groupReplica.UpstreamGroupId,
		Status:  "Processing",
	}, nil
}

func (actionAssetLibraryBackend) UpdateGroup(ctx context.Context, config *model.ChannelAssetConfig, group *model.UserAssetGroup, upstreamGroupId string) error {
	projectName := assetLibraryProject(config)
	request := dto.UpdateAssetGroupRequest{
		Id:          upstreamGroupId,
		Name:        &group.Name,
		Description: &group.Description,
		ProjectName: &projectName,
	}
	return CallAssetLibraryUpstream(ctx, config, "UpdateAssetGroup", request, nil)
}

func (actionAssetLibraryBackend) UpdateAsset(ctx context.Context, config *model.ChannelAssetConfig, asset *model.UserAsset, upstreamAssetId string) error {
	projectName := assetLibraryProject(config)
	request := dto.UpdateAssetRequest{Id: upstreamAssetId, Name: &asset.Name, ProjectName: &projectName}
	return CallAssetLibraryUpstream(ctx, config, "UpdateAsset", request, nil)
}

func (actionAssetLibraryBackend) DeleteGroup(ctx context.Context, config *model.ChannelAssetConfig, upstreamGroupId string) error {
	projectName := assetLibraryProject(config)
	request := dto.DeleteAssetGroupRequest{Id: upstreamGroupId, ProjectName: &projectName}
	return CallAssetLibraryUpstream(ctx, config, "DeleteAssetGroup", request, nil)
}

func (actionAssetLibraryBackend) DeleteAsset(ctx context.Context, config *model.ChannelAssetConfig, upstreamAssetId string) error {
	projectName := assetLibraryProject(config)
	request := dto.DeleteAssetRequest{Id: upstreamAssetId, ProjectName: &projectName}
	return CallAssetLibraryUpstream(ctx, config, "DeleteAsset", request, nil)
}

func (actionAssetLibraryBackend) GetAsset(ctx context.Context, config *model.ChannelAssetConfig, upstreamAssetId string) (*AssetLibraryAssetDetails, error) {
	projectName := assetLibraryProject(config)
	request := dto.GetAssetRequest{Id: upstreamAssetId, ProjectName: &projectName}
	var details AssetLibraryAssetDetails
	if err := CallAssetLibraryUpstream(ctx, config, "GetAsset", request, &details); err != nil {
		return nil, err
	}
	return &details, nil
}

func (actionAssetLibraryBackend) FormatAssetReference(upstreamAssetId string) string {
	return "asset://" + upstreamAssetId
}

func (seedanceSLSAssetLibraryBackend) CreateGroup(context.Context, *model.ChannelAssetConfig, *model.UserAssetGroup) (*assetLibraryCreateGroupResult, error) {
	return &assetLibraryCreateGroupResult{Deferred: true}, nil
}

type seedanceSLSCreateAssetRequest struct {
	SourceURL string  `json:"source_url"`
	AssetType string  `json:"asset_type"`
	Name      *string `json:"name,omitempty"`
	GroupID   *string `json:"group_id,omitempty"`
	GroupName *string `json:"group_name,omitempty"`
}

type seedanceSLSAssetData struct {
	LogicalID      string `json:"logical_id"`
	LogicalGroupID string `json:"logical_group_id"`
	GroupName      string `json:"group_name"`
	GroupID        string `json:"group_id"`
	Status         string `json:"status"`
	Name           string `json:"name"`
	AssetType      string `json:"asset_type"`
	SourceURL      string `json:"source_url"`
}

func (seedanceSLSAssetLibraryBackend) CreateAsset(ctx context.Context, config *model.ChannelAssetConfig, group *model.UserAssetGroup, groupReplica *model.UserAssetGroupReplica, asset *model.UserAsset) (*assetLibraryCreateAssetResult, error) {
	request := seedanceSLSCreateAssetRequest{
		SourceURL: asset.SourceURL,
		AssetType: asset.AssetType,
	}
	if strings.TrimSpace(asset.Name) != "" {
		request.Name = &asset.Name
	}
	if groupReplica != nil && strings.TrimSpace(groupReplica.UpstreamGroupId) != "" {
		request.GroupID = &groupReplica.UpstreamGroupId
	} else {
		request.GroupName = &group.Name
	}
	var result seedanceSLSAssetData
	if err := callSeedanceSLSAssetLibrary(ctx, config, http.MethodPost, "", request, &result); err != nil {
		return nil, err
	}
	groupID := strings.TrimSpace(result.LogicalGroupID)
	if groupID == "" && groupReplica != nil {
		groupID = strings.TrimSpace(groupReplica.UpstreamGroupId)
	}
	return &assetLibraryCreateAssetResult{
		AssetID: strings.TrimSpace(result.LogicalID),
		GroupID: groupID,
		Status:  strings.TrimSpace(result.Status),
	}, nil
}

func (seedanceSLSAssetLibraryBackend) UpdateGroup(context.Context, *model.ChannelAssetConfig, *model.UserAssetGroup, string) error {
	return nil
}

func (seedanceSLSAssetLibraryBackend) UpdateAsset(context.Context, *model.ChannelAssetConfig, *model.UserAsset, string) error {
	return nil
}

func (seedanceSLSAssetLibraryBackend) DeleteGroup(context.Context, *model.ChannelAssetConfig, string) error {
	return nil
}

func (seedanceSLSAssetLibraryBackend) DeleteAsset(ctx context.Context, config *model.ChannelAssetConfig, upstreamAssetId string) error {
	return callSeedanceSLSAssetLibrary(ctx, config, http.MethodDelete, upstreamAssetId, nil, nil)
}

func (seedanceSLSAssetLibraryBackend) GetAsset(ctx context.Context, config *model.ChannelAssetConfig, upstreamAssetId string) (*AssetLibraryAssetDetails, error) {
	var result seedanceSLSAssetData
	if err := callSeedanceSLSAssetLibrary(ctx, config, http.MethodGet, upstreamAssetId, nil, &result); err != nil {
		return nil, err
	}
	return &AssetLibraryAssetDetails{
		Id:          result.LogicalID,
		Name:        result.Name,
		URL:         result.SourceURL,
		GroupId:     result.LogicalGroupID,
		AssetType:   result.AssetType,
		Status:      result.Status,
		ProjectName: assetLibraryProject(config),
	}, nil
}

func (seedanceSLSAssetLibraryBackend) FormatAssetReference(upstreamAssetId string) string {
	return "asset://" + upstreamAssetId
}

type seedanceSLSResponseEnvelope struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
}

func callSeedanceSLSAssetLibrary(ctx context.Context, config *model.ChannelAssetConfig, method string, assetId string, input any, output any) error {
	if config == nil || !config.Enabled {
		return errors.New("asset library is not enabled for channel")
	}
	if config.AuthType != AssetLibraryAuthBearer {
		return errors.New("Seedance SLS asset library requires Bearer authentication")
	}
	apiKey := strings.TrimSpace(config.APIKey)
	if apiKey == "" {
		return errors.New("asset library API key is empty")
	}
	endpoint, err := seedanceSLSAssetLibraryURL(config.BaseURL, assetId)
	if err != nil {
		return err
	}
	var body []byte
	if input != nil {
		body, err = common.Marshal(input)
		if err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Authorization", "Bearer "+apiKey)
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
	var envelope seedanceSLSResponseEnvelope
	if len(responseBody) == 0 || common.Unmarshal(responseBody, &envelope) != nil {
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return &AssetLibraryUpstreamError{StatusCode: response.StatusCode, Message: http.StatusText(response.StatusCode)}
		}
		return errors.New("decode Seedance SLS asset library response")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices || !envelope.Success {
		message := common.MaskSensitiveInfo(common.LocalLogPreview(envelope.Message))
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return &AssetLibraryUpstreamError{StatusCode: response.StatusCode, Code: envelope.Code, Message: message}
	}
	if output == nil {
		return nil
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("Seedance SLS asset library response is missing data")
	}
	return common.Unmarshal(envelope.Data, output)
}

func seedanceSLSAssetLibraryURL(baseURL string, assetId string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = constant.ChannelBaseURLs[constant.ChannelTypeSeedanceSLS]
	}
	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", errors.New("invalid Seedance SLS asset library base URL")
	}
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/v1/volcengine/assets"
	if assetId != "" {
		if strings.Contains(assetId, "/") {
			return "", fmt.Errorf("invalid Seedance SLS asset id %q", assetId)
		}
		endpoint.Path += "/" + assetId
	}
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

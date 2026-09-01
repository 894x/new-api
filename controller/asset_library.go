package controller

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type assetLibraryResponseMetadata struct {
	RequestId string                 `json:"RequestId"`
	Action    string                 `json:"Action"`
	Version   string                 `json:"Version"`
	Service   string                 `json:"Service"`
	Region    string                 `json:"Region"`
	Error     *dto.AssetLibraryError `json:"Error,omitempty"`
}

type assetLibraryResponse struct {
	ResponseMetadata assetLibraryResponseMetadata `json:"ResponseMetadata"`
	Result           any                          `json:"Result,omitempty"`
}

type assetLibraryMutationResult struct {
	Id          string                   `json:"Id"`
	Replication *dto.AssetReplicaSummary `json:"Replication,omitempty"`
}

func AssetLibraryAction(c *gin.Context) {
	action := strings.TrimSpace(c.Query("Action"))
	version := strings.TrimSpace(c.Query("Version"))
	if version != service.AssetLibraryVersion {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.Version", "Version must be "+service.AssetLibraryVersion, nil)
		return
	}
	userId := c.GetInt("id")
	if userId <= 0 {
		writeAssetLibraryError(c, action, http.StatusUnauthorized, "Unauthorized", "user identity is missing", nil)
		return
	}
	includeReplication := model.IsAdmin(userId)
	switch action {
	case "CreateAssetGroup":
		createAssetLibraryGroup(c, userId, includeReplication)
	case "CreateAsset":
		createAssetLibraryAsset(c, userId, includeReplication)
	case "ListAssetGroups":
		listAssetLibraryGroups(c, userId, includeReplication)
	case "ListAssets":
		listAssetLibraryAssets(c, userId, includeReplication)
	case "GetAssetGroup":
		getAssetLibraryGroup(c, userId, includeReplication)
	case "GetAsset":
		getAssetLibraryAsset(c, userId, includeReplication)
	case "UpdateAssetGroup":
		updateAssetLibraryGroup(c, userId, includeReplication)
	case "UpdateAsset":
		updateAssetLibraryAsset(c, userId, includeReplication)
	case "DeleteAsset":
		deleteAssetLibraryAsset(c, userId)
	case "DeleteAssetGroup":
		deleteAssetLibraryGroup(c, userId)
	default:
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.Action", "unsupported asset library action", nil)
	}
}

func createAssetLibraryGroup(c *gin.Context, userId int, includeReplication bool) {
	var request dto.CreateAssetGroupRequest
	if !decodeAssetLibraryRequest(c, "CreateAssetGroup", &request) {
		return
	}
	request.Name = strings.TrimSpace(request.Name)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 64 {
		writeAssetLibraryError(c, "CreateAssetGroup", http.StatusBadRequest, "InvalidParameter.Name", "Name is required and must not exceed 64 characters", nil)
		return
	}
	description := ""
	if request.Description != nil {
		description = strings.TrimSpace(*request.Description)
		if utf8.RuneCountInString(description) > 300 {
			writeAssetLibraryError(c, "CreateAssetGroup", http.StatusBadRequest, "InvalidParameter.Description", "Description must not exceed 300 characters", nil)
			return
		}
	}
	groupType := "AIGC"
	if request.GroupType != nil && strings.TrimSpace(*request.GroupType) != "" {
		groupType = strings.TrimSpace(*request.GroupType)
	}
	projectName := assetLibraryLogicalProject(request.ProjectName)
	if utf8.RuneCountInString(groupType) > 32 {
		writeAssetLibraryError(c, "CreateAssetGroup", http.StatusBadRequest, "InvalidParameter.GroupType", "GroupType must not exceed 32 characters", nil)
		return
	}
	if utf8.RuneCountInString(projectName) > 128 {
		writeAssetLibraryError(c, "CreateAssetGroup", http.StatusBadRequest, "InvalidParameter.ProjectName", "ProjectName must not exceed 128 characters", nil)
		return
	}
	group := &model.UserAssetGroup{
		Id:          "group-na-" + common.GetUUID(),
		UserId:      userId,
		Name:        request.Name,
		Description: description,
		GroupType:   groupType,
		ProjectName: projectName,
	}
	if err := model.CreateUserAssetGroup(group); err != nil {
		writeAssetLibraryInternalError(c, "CreateAssetGroup", err)
		return
	}
	recordAssetLibraryAudit(c, userId, "asset_library.group.create", map[string]interface{}{
		"id":         group.Id,
		"name":       group.Name,
		"group_type": group.GroupType,
	})
	report, err := service.ReplicateAssetGroup(c.Request.Context(), group)
	if err != nil {
		writeAssetLibraryInternalError(c, "CreateAssetGroup", err)
		return
	}
	result := assetLibraryMutationResult{Id: group.Id}
	if includeReplication {
		result.Replication = report.Summary
	}
	writeAssetLibrarySuccess(c, "CreateAssetGroup", result)
}

func createAssetLibraryAsset(c *gin.Context, userId int, includeReplication bool) {
	var request dto.CreateAssetRequest
	if !decodeAssetLibraryRequest(c, "CreateAsset", &request) {
		return
	}
	request.GroupId = strings.TrimSpace(request.GroupId)
	if !isLogicalAssetLibraryId(request.GroupId, "group-na-") {
		writeAssetLibraryError(c, "CreateAsset", http.StatusBadRequest, "InvalidParameter.GroupId", "GroupId must be a New API logical asset group id", nil)
		return
	}
	group, err := model.GetUserAssetGroup(userId, request.GroupId)
	if err != nil {
		writeAssetLibraryLookupError(c, "CreateAsset", "NotFound.GroupId", "asset group not found", err)
		return
	}
	sourceURL, err := validateAssetLibrarySourceURL(request.URL)
	if err != nil {
		writeAssetLibraryError(c, "CreateAsset", http.StatusBadRequest, "InvalidParameter.URL", err.Error(), nil)
		return
	}
	assetType := strings.TrimSpace(request.AssetType)
	if assetType != "Image" && assetType != "Video" && assetType != "Audio" {
		writeAssetLibraryError(c, "CreateAsset", http.StatusBadRequest, "InvalidParameter.AssetType", "AssetType must be Image, Video, or Audio", nil)
		return
	}
	name := ""
	if request.Name != nil {
		name = strings.TrimSpace(*request.Name)
		if utf8.RuneCountInString(name) > 64 {
			writeAssetLibraryError(c, "CreateAsset", http.StatusBadRequest, "InvalidParameter.Name", "Name must not exceed 64 characters", nil)
			return
		}
	}
	asset := &model.UserAsset{
		Id:          "asset-na-" + common.GetUUID(),
		UserId:      userId,
		GroupId:     group.Id,
		Name:        name,
		SourceURL:   sourceURL,
		AssetType:   assetType,
		ProjectName: group.ProjectName,
	}
	if err := model.CreateUserAsset(asset); err != nil {
		writeAssetLibraryInternalError(c, "CreateAsset", err)
		return
	}
	recordAssetLibraryAudit(c, userId, "asset_library.asset.create", map[string]interface{}{
		"id":         asset.Id,
		"name":       asset.Name,
		"group_id":   asset.GroupId,
		"asset_type": asset.AssetType,
	})
	report, err := service.ReplicateAsset(c.Request.Context(), asset)
	if err != nil {
		writeAssetLibraryInternalError(c, "CreateAsset", err)
		return
	}
	result := assetLibraryMutationResult{Id: asset.Id}
	if includeReplication {
		result.Replication = report.Summary
	}
	writeAssetLibrarySuccess(c, "CreateAsset", result)
}

func listAssetLibraryGroups(c *gin.Context, userId int, includeReplication bool) {
	var request dto.ListAssetGroupsRequest
	if !decodeAssetLibraryRequest(c, "ListAssetGroups", &request) {
		return
	}
	pageNumber, pageSize, ok := validateAssetLibraryPagination(c, "ListAssetGroups", request.PageNumber, request.PageSize)
	if !ok {
		return
	}
	params := model.AssetGroupListParams{
		ProjectName: assetLibraryOptionalString(request.ProjectName),
		PageNumber:  pageNumber,
		PageSize:    pageSize,
		SortBy:      assetLibraryOptionalString(request.SortBy),
		SortOrder:   assetLibraryOptionalString(request.SortOrder),
	}
	if request.Filter != nil {
		params.GroupIds = request.Filter.GroupIds
		params.GroupType = strings.TrimSpace(request.Filter.GroupType)
		params.Name = strings.TrimSpace(request.Filter.Name)
	}
	if !validateAssetLibrarySort(c, "ListAssetGroups", params.SortBy, params.SortOrder, map[string]struct{}{"": {}, "CreateTime": {}, "UpdateTime": {}}) {
		return
	}
	groups, total, err := model.ListUserAssetGroups(userId, params)
	if err != nil {
		writeAssetLibraryInternalError(c, "ListAssetGroups", err)
		return
	}
	items := make([]dto.AssetGroupResult, 0, len(groups))
	for i := range groups {
		item, err := buildAssetLibraryGroupResult(&groups[i], includeReplication)
		if err != nil {
			writeAssetLibraryInternalError(c, "ListAssetGroups", err)
			return
		}
		items = append(items, item)
	}
	writeAssetLibrarySuccess(c, "ListAssetGroups", dto.ListAssetGroupsResult{
		TotalCount: total,
		Items:      items,
		PageNumber: pageNumber,
		PageSize:   pageSize,
	})
}

func listAssetLibraryAssets(c *gin.Context, userId int, includeReplication bool) {
	var request dto.ListAssetsRequest
	if !decodeAssetLibraryRequest(c, "ListAssets", &request) {
		return
	}
	pageNumber, pageSize, ok := validateAssetLibraryPagination(c, "ListAssets", request.PageNumber, request.PageSize)
	if !ok {
		return
	}
	params := model.AssetListParams{
		ProjectName: assetLibraryOptionalString(request.ProjectName),
		PageNumber:  pageNumber,
		PageSize:    pageSize,
		SortBy:      assetLibraryOptionalString(request.SortBy),
		SortOrder:   assetLibraryOptionalString(request.SortOrder),
	}
	if request.Filter != nil {
		params.GroupIds = request.Filter.GroupIds
		params.GroupType = strings.TrimSpace(request.Filter.GroupType)
		params.Name = strings.TrimSpace(request.Filter.Name)
		params.AssetType = strings.TrimSpace(request.Filter.AssetType)
		params.Statuses = request.Filter.Statuses
	}
	if !validateAssetLibrarySort(c, "ListAssets", params.SortBy, params.SortOrder, map[string]struct{}{"": {}, "CreateTime": {}, "UpdateTime": {}, "GroupId": {}}) {
		return
	}
	assets, total, err := model.ListUserAssets(userId, params)
	if err != nil {
		writeAssetLibraryInternalError(c, "ListAssets", err)
		return
	}
	items := make([]dto.AssetResult, 0, len(assets))
	for i := range assets {
		item, err := buildAssetLibraryResult(&assets[i], nil, includeReplication)
		if err != nil {
			writeAssetLibraryInternalError(c, "ListAssets", err)
			return
		}
		items = append(items, item)
	}
	writeAssetLibrarySuccess(c, "ListAssets", dto.ListAssetsResult{
		TotalCount: total,
		Items:      items,
		PageNumber: pageNumber,
		PageSize:   pageSize,
	})
}

func getAssetLibraryGroup(c *gin.Context, userId int, includeReplication bool) {
	var request dto.GetAssetGroupRequest
	if !decodeAssetLibraryRequest(c, "GetAssetGroup", &request) {
		return
	}
	group, err := model.GetUserAssetGroup(userId, strings.TrimSpace(request.Id))
	if err != nil {
		writeAssetLibraryLookupError(c, "GetAssetGroup", "NotFound.GroupId", "asset group not found", err)
		return
	}
	result, err := buildAssetLibraryGroupResult(group, includeReplication)
	if err != nil {
		writeAssetLibraryInternalError(c, "GetAssetGroup", err)
		return
	}
	writeAssetLibrarySuccess(c, "GetAssetGroup", result)
}

func getAssetLibraryAsset(c *gin.Context, userId int, includeReplication bool) {
	var request dto.GetAssetRequest
	if !decodeAssetLibraryRequest(c, "GetAsset", &request) {
		return
	}
	asset, err := model.GetUserAsset(userId, strings.TrimSpace(request.Id))
	if err != nil {
		writeAssetLibraryLookupError(c, "GetAsset", "NotFound.AssetId", "asset not found", err)
		return
	}
	details, refreshErr := service.RefreshAssetLibraryAsset(c.Request.Context(), asset.Id)
	result, err := buildAssetLibraryResult(asset, details, includeReplication)
	if err != nil {
		writeAssetLibraryInternalError(c, "GetAsset", err)
		return
	}
	if refreshErr != nil {
		common.SysError("asset library GetAsset preview refresh failed")
		if result.Error == nil && strings.TrimSpace(result.URL) == "" {
			result.Error = &dto.AssetLibraryError{Code: "PreviewUnavailable", Message: "Asset preview is temporarily unavailable"}
		}
	}
	writeAssetLibrarySuccess(c, "GetAsset", result)
}

func updateAssetLibraryGroup(c *gin.Context, userId int, includeReplication bool) {
	var request dto.UpdateAssetGroupRequest
	if !decodeAssetLibraryRequest(c, "UpdateAssetGroup", &request) {
		return
	}
	if request.Name == nil && request.Description == nil {
		writeAssetLibraryError(c, "UpdateAssetGroup", http.StatusBadRequest, "MissingParameter", "Name or Description is required", nil)
		return
	}
	group, err := model.GetUserAssetGroup(userId, strings.TrimSpace(request.Id))
	if err != nil {
		writeAssetLibraryLookupError(c, "UpdateAssetGroup", "NotFound.GroupId", "asset group not found", err)
		return
	}
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" || utf8.RuneCountInString(name) > 64 {
			writeAssetLibraryError(c, "UpdateAssetGroup", http.StatusBadRequest, "InvalidParameter.Name", "Name must be non-empty and not exceed 64 characters", nil)
			return
		}
		group.Name = name
	}
	if request.Description != nil {
		description := strings.TrimSpace(*request.Description)
		if utf8.RuneCountInString(description) > 300 {
			writeAssetLibraryError(c, "UpdateAssetGroup", http.StatusBadRequest, "InvalidParameter.Description", "Description must not exceed 300 characters", nil)
			return
		}
		group.Description = description
	}
	if err := model.UpdateUserAssetGroup(group); err != nil {
		writeAssetLibraryInternalError(c, "UpdateAssetGroup", err)
		return
	}
	recordAssetLibraryAudit(c, userId, "asset_library.group.update", map[string]interface{}{
		"id":         group.Id,
		"name":       group.Name,
		"group_type": group.GroupType,
	})
	report, err := service.UpdateAssetGroupReplicas(c.Request.Context(), group)
	if err != nil {
		writeAssetLibraryInternalError(c, "UpdateAssetGroup", err)
		return
	}
	result := assetLibraryMutationResult{Id: group.Id}
	if includeReplication {
		result.Replication = report.Summary
	}
	writeAssetLibrarySuccess(c, "UpdateAssetGroup", result)
}

func updateAssetLibraryAsset(c *gin.Context, userId int, includeReplication bool) {
	var request dto.UpdateAssetRequest
	if !decodeAssetLibraryRequest(c, "UpdateAsset", &request) {
		return
	}
	if request.Name == nil {
		writeAssetLibraryError(c, "UpdateAsset", http.StatusBadRequest, "MissingParameter.Name", "Name is required", nil)
		return
	}
	asset, err := model.GetUserAsset(userId, strings.TrimSpace(request.Id))
	if err != nil {
		writeAssetLibraryLookupError(c, "UpdateAsset", "NotFound.AssetId", "asset not found", err)
		return
	}
	name := strings.TrimSpace(*request.Name)
	if utf8.RuneCountInString(name) > 64 {
		writeAssetLibraryError(c, "UpdateAsset", http.StatusBadRequest, "InvalidParameter.Name", "Name must not exceed 64 characters", nil)
		return
	}
	asset.Name = name
	if err := model.UpdateUserAsset(asset); err != nil {
		writeAssetLibraryInternalError(c, "UpdateAsset", err)
		return
	}
	recordAssetLibraryAudit(c, userId, "asset_library.asset.update", map[string]interface{}{
		"id":         asset.Id,
		"name":       asset.Name,
		"group_id":   asset.GroupId,
		"asset_type": asset.AssetType,
	})
	report, err := service.UpdateAssetReplicas(c.Request.Context(), asset)
	if err != nil {
		writeAssetLibraryInternalError(c, "UpdateAsset", err)
		return
	}
	result := assetLibraryMutationResult{Id: asset.Id}
	if includeReplication {
		result.Replication = report.Summary
	}
	writeAssetLibrarySuccess(c, "UpdateAsset", result)
}

func deleteAssetLibraryAsset(c *gin.Context, userId int) {
	var request dto.DeleteAssetRequest
	if !decodeAssetLibraryRequest(c, "DeleteAsset", &request) {
		return
	}
	asset, err := model.GetUserAsset(userId, strings.TrimSpace(request.Id))
	if err != nil {
		writeAssetLibraryLookupError(c, "DeleteAsset", "NotFound.AssetId", "asset not found", err)
		return
	}
	channelErrors, err := service.DeleteAssetReplicas(c.Request.Context(), asset.Id)
	if err != nil {
		writeAssetLibraryInternalError(c, "DeleteAsset", err)
		return
	}
	if len(channelErrors) > 0 {
		writeAssetLibraryError(c, "DeleteAsset", http.StatusBadGateway, "UpstreamDeleteFailed", "one or more upstream replicas could not be deleted", nil)
		return
	}
	if err := model.DeleteUserAsset(userId, asset.Id); err != nil {
		writeAssetLibraryInternalError(c, "DeleteAsset", err)
		return
	}
	recordAssetLibraryAudit(c, userId, "asset_library.asset.delete", map[string]interface{}{
		"id":         asset.Id,
		"name":       asset.Name,
		"group_id":   asset.GroupId,
		"asset_type": asset.AssetType,
	})
	writeAssetLibrarySuccess(c, "DeleteAsset", gin.H{})
}

func deleteAssetLibraryGroup(c *gin.Context, userId int) {
	var request dto.DeleteAssetGroupRequest
	if !decodeAssetLibraryRequest(c, "DeleteAssetGroup", &request) {
		return
	}
	group, err := model.GetUserAssetGroup(userId, strings.TrimSpace(request.Id))
	if err != nil {
		writeAssetLibraryLookupError(c, "DeleteAssetGroup", "NotFound.GroupId", "asset group not found", err)
		return
	}
	assetCount, err := model.CountUserAssetsInGroup(userId, group.Id)
	if err != nil {
		writeAssetLibraryInternalError(c, "DeleteAssetGroup", err)
		return
	}
	if assetCount > 0 {
		writeAssetLibraryError(c, "DeleteAssetGroup", http.StatusConflict, "AssetGroupNotEmpty", "delete all assets in the group first", nil)
		return
	}
	channelErrors, err := service.DeleteAssetGroupReplicas(c.Request.Context(), group.Id)
	if err != nil {
		writeAssetLibraryInternalError(c, "DeleteAssetGroup", err)
		return
	}
	if len(channelErrors) > 0 {
		writeAssetLibraryError(c, "DeleteAssetGroup", http.StatusBadGateway, "UpstreamDeleteFailed", "one or more upstream replicas could not be deleted", nil)
		return
	}
	if err := model.DeleteUserAssetGroup(userId, group.Id); err != nil {
		writeAssetLibraryInternalError(c, "DeleteAssetGroup", err)
		return
	}
	recordAssetLibraryAudit(c, userId, "asset_library.group.delete", map[string]interface{}{
		"id":         group.Id,
		"name":       group.Name,
		"group_type": group.GroupType,
	})
	writeAssetLibrarySuccess(c, "DeleteAssetGroup", gin.H{})
}

func recordAssetLibraryAudit(c *gin.Context, userId int, action string, params map[string]interface{}) {
	model.RecordOperationAuditLog(userId, auditContentEN(action, params), c.ClientIP(), action, params, nil, nil)
}

func buildAssetLibraryGroupResult(group *model.UserAssetGroup, includeReplication bool) (dto.AssetGroupResult, error) {
	var summary *dto.AssetReplicaSummary
	if includeReplication {
		var err error
		summary, err = service.GetAssetGroupReplicationSummary(group.Id)
		if err != nil {
			return dto.AssetGroupResult{}, err
		}
	}
	return dto.AssetGroupResult{
		Id:          group.Id,
		Name:        group.Name,
		Description: group.Description,
		GroupType:   group.GroupType,
		ProjectName: group.ProjectName,
		CreateTime:  assetLibraryFormatTime(group.CreatedTime),
		UpdateTime:  assetLibraryFormatTime(group.UpdatedTime),
		Replication: summary,
	}, nil
}

func buildAssetLibraryResult(asset *model.UserAsset, details *service.AssetLibraryAssetDetails, includeReplication bool) (dto.AssetResult, error) {
	var summary *dto.AssetReplicaSummary
	if includeReplication {
		var err error
		summary, err = service.GetAssetReplicationSummary(asset.Id)
		if err != nil {
			return dto.AssetResult{}, err
		}
	}
	status, assetError, lastInferenceTime, err := service.GetAssetLibraryAggregateState(asset.Id)
	if err != nil {
		return dto.AssetResult{}, err
	}
	result := dto.AssetResult{
		Id:                asset.Id,
		Name:              asset.Name,
		URL:               asset.SourceURL,
		GroupId:           asset.GroupId,
		AssetType:         asset.AssetType,
		Status:            status,
		Error:             assetError,
		ProjectName:       asset.ProjectName,
		CreateTime:        assetLibraryFormatTime(asset.CreatedTime),
		UpdateTime:        assetLibraryFormatTime(asset.UpdatedTime),
		LastInferenceTime: lastInferenceTime,
		Replication:       summary,
	}
	if details != nil {
		result.Status = details.Status
		if details.Error != nil && (details.Error.Code != "" || details.Error.Message != "") {
			result.Error = &dto.AssetLibraryError{Code: "AssetProcessingFailed", Message: "Asset processing failed"}
		}
		result.LastInferenceTime = details.LastInferenceTime
	}
	return result, nil
}

func decodeAssetLibraryRequest(c *gin.Context, action string, destination any) bool {
	if err := common.DecodeJson(c.Request.Body, destination); err != nil {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidRequestBody", "invalid request body: "+err.Error(), nil)
		return false
	}
	return true
}

func validateAssetLibraryPagination(c *gin.Context, action string, pageNumberValue *int64, pageSizeValue *int64) (int64, int64, bool) {
	pageNumber := int64(1)
	pageSize := int64(10)
	if pageNumberValue != nil {
		pageNumber = *pageNumberValue
	}
	if pageSizeValue != nil {
		pageSize = *pageSizeValue
	}
	if pageNumber < 1 {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.PageNumber", "PageNumber must be at least 1", nil)
		return 0, 0, false
	}
	if pageNumber > 1_000_000 {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.PageNumber", "PageNumber is too large", nil)
		return 0, 0, false
	}
	if pageSize < 1 || pageSize > 100 {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.PageSize", "PageSize must be between 1 and 100", nil)
		return 0, 0, false
	}
	return pageNumber, pageSize, true
}

func validateAssetLibrarySort(c *gin.Context, action string, sortBy string, sortOrder string, allowed map[string]struct{}) bool {
	if _, ok := allowed[sortBy]; !ok {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.SortBy", "unsupported SortBy value", nil)
		return false
	}
	if sortOrder != "" && sortOrder != "Asc" && sortOrder != "Desc" {
		writeAssetLibraryError(c, action, http.StatusBadRequest, "InvalidParameter.SortOrder", "SortOrder must be Asc or Desc", nil)
		return false
	}
	return true
}

func assetLibraryOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func assetLibraryLogicalProject(value *string) string {
	projectName := assetLibraryOptionalString(value)
	if projectName == "" {
		return service.DefaultAssetLibraryProject
	}
	return projectName
}

func validateAssetLibrarySourceURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 8192 {
		return "", errors.New("URL must not exceed 8192 bytes")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return "", errors.New("URL must be a publicly accessible http or https URL without embedded credentials")
	}
	return value, nil
}

func isLogicalAssetLibraryId(value string, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+32 {
		return false
	}
	for _, char := range value[len(prefix):] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return false
		}
	}
	return true
}

func assetLibraryFormatTime(timestamp int64) string {
	if timestamp <= 0 {
		return ""
	}
	return time.Unix(timestamp, 0).UTC().Format(time.RFC3339)
}

func writeAssetLibrarySuccess(c *gin.Context, action string, result any) {
	c.JSON(http.StatusOK, assetLibraryResponse{
		ResponseMetadata: newAssetLibraryResponseMetadata(c, action),
		Result:           result,
	})
}

func writeAssetLibraryError(c *gin.Context, action string, status int, code string, message string, result any) {
	metadata := newAssetLibraryResponseMetadata(c, action)
	metadata.Error = &dto.AssetLibraryError{Code: code, Message: message}
	c.JSON(status, assetLibraryResponse{ResponseMetadata: metadata, Result: result})
}

func writeAssetLibraryLookupError(c *gin.Context, action string, code string, message string, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeAssetLibraryError(c, action, http.StatusNotFound, code, message, nil)
		return
	}
	writeAssetLibraryInternalError(c, action, err)
}

func writeAssetLibraryInternalError(c *gin.Context, action string, err error) {
	common.SysError("asset library " + action + " failed: " + common.MaskSensitiveInfo(common.LocalLogPreview(err.Error())))
	writeAssetLibraryError(c, action, http.StatusInternalServerError, "InternalError", "asset library operation failed", nil)
}

func newAssetLibraryResponseMetadata(c *gin.Context, action string) assetLibraryResponseMetadata {
	requestId := c.GetString(common.RequestIdKey)
	if requestId == "" {
		requestId = common.NewRequestId()
	}
	return assetLibraryResponseMetadata{
		RequestId: requestId,
		Action:    action,
		Version:   service.AssetLibraryVersion,
		Service:   service.AssetLibraryService,
		Region:    service.DefaultAssetLibraryRegion,
	}
}

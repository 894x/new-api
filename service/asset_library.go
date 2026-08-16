package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type assetLibraryChannelError struct {
	ChannelId int
	Message   string
}

type AssetLibraryReplicationReport struct {
	Summary *dto.AssetReplicaSummary
	Errors  []assetLibraryChannelError
}

type AssetLibrarySyncResult struct {
	ChannelId     int                        `json:"channel_id"`
	GroupsCreated int                        `json:"groups_created"`
	GroupsSkipped int                        `json:"groups_skipped"`
	GroupsFailed  int                        `json:"groups_failed"`
	AssetsCreated int                        `json:"assets_created"`
	AssetsSkipped int                        `json:"assets_skipped"`
	AssetsFailed  int                        `json:"assets_failed"`
	Errors        []assetLibraryChannelError `json:"-"`
}

type AssetLibraryAssetDetails struct {
	Id                string                 `json:"Id"`
	Name              string                 `json:"Name"`
	URL               string                 `json:"URL"`
	GroupId           string                 `json:"GroupId"`
	AssetType         string                 `json:"AssetType"`
	Status            string                 `json:"Status"`
	Error             *dto.AssetLibraryError `json:"Error,omitempty"`
	ProjectName       string                 `json:"ProjectName"`
	CreateTime        string                 `json:"CreateTime"`
	UpdateTime        string                 `json:"UpdateTime"`
	LastInferenceTime string                 `json:"LastInferenceTime,omitempty"`
}

var assetLibraryChannelLocks sync.Map

func getAssetLibraryChannelLock(channelId int) *sync.Mutex {
	lock, _ := assetLibraryChannelLocks.LoadOrStore(channelId, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func SaveAssetLibraryChannelConfig(config *model.ChannelAssetConfig) ([]string, error) {
	if config == nil {
		return nil, errors.New("asset library channel config is nil")
	}
	lock := getAssetLibraryChannelLock(config.ChannelId)
	lock.Lock()
	defer lock.Unlock()

	changedFields := make([]string, 0)
	existing, err := model.GetChannelAssetConfig(config.ChannelId)
	newConfig := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !newConfig {
		return nil, err
	}
	identityChanged := newConfig
	if newConfig || existing.Enabled != config.Enabled {
		changedFields = append(changedFields, "enabled")
	}
	if newConfig || existing.BaseURL != config.BaseURL {
		changedFields = append(changedFields, "base_url")
		identityChanged = true
	}
	if newConfig || existing.AuthType != config.AuthType {
		changedFields = append(changedFields, "auth_type")
		identityChanged = true
	}
	if newConfig || existing.Region != config.Region {
		changedFields = append(changedFields, "region")
		identityChanged = true
	}
	if newConfig || existing.ProjectName != config.ProjectName {
		changedFields = append(changedFields, "project_name")
		identityChanged = true
	}
	if newConfig || existing.AccessKey != config.AccessKey || existing.SecretKey != config.SecretKey || existing.APIKey != config.APIKey {
		changedFields = append(changedFields, "credentials")
		identityChanged = true
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if identityChanged && !newConfig {
			if err := model.DeleteChannelAssetLibraryData(tx, []int{config.ChannelId}); err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "channel_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"enabled", "base_url", "auth_type", "access_key", "secret_key", "api_key",
				"region", "project_name", "updated_time",
			}),
		}).Create(config).Error
	}); err != nil {
		return nil, err
	}
	return changedFields, nil
}

func DeleteAssetLibraryChannelConfig(channelId int) error {
	lock := getAssetLibraryChannelLock(channelId)
	lock.Lock()
	defer lock.Unlock()
	return model.DeleteChannelAssetLibraryData(model.DB, []int{channelId})
}

func assetLibraryProject(config *model.ChannelAssetConfig) string {
	projectName := strings.TrimSpace(config.ProjectName)
	if projectName == "" {
		return DefaultAssetLibraryProject
	}
	return projectName
}

func ReplicateAssetGroup(ctx context.Context, group *model.UserAssetGroup) (*AssetLibraryReplicationReport, error) {
	configs, err := model.GetEnabledChannelAssetConfigs()
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range configs {
		channelId := configs[i].ChannelId
		lock := getAssetLibraryChannelLock(channelId)
		lock.Lock()
		config, configErr := model.GetChannelAssetConfig(channelId)
		if configErr != nil || !config.Enabled {
			lock.Unlock()
			if configErr != nil {
				errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: channelId, Message: configErr.Error()})
			}
			continue
		}
		_, replicateErr := replicateAssetGroupToChannelLocked(ctx, group, config)
		lock.Unlock()
		if replicateErr != nil {
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: config.ChannelId, Message: replicateErr.Error()})
		}
	}
	summary, summaryErr := GetAssetGroupReplicationSummary(group.Id)
	if summaryErr != nil {
		return nil, summaryErr
	}
	return &AssetLibraryReplicationReport{Summary: summary, Errors: errorsByChannel}, nil
}

func ReplicateAsset(ctx context.Context, asset *model.UserAsset) (*AssetLibraryReplicationReport, error) {
	configs, err := model.GetEnabledChannelAssetConfigs()
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range configs {
		channelId := configs[i].ChannelId
		lock := getAssetLibraryChannelLock(channelId)
		lock.Lock()
		config, configErr := model.GetChannelAssetConfig(channelId)
		if configErr != nil || !config.Enabled {
			lock.Unlock()
			if configErr != nil {
				errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: channelId, Message: configErr.Error()})
			}
			continue
		}
		_, replicateErr := replicateAssetToChannelLocked(ctx, asset, config)
		lock.Unlock()
		if replicateErr != nil {
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: config.ChannelId, Message: replicateErr.Error()})
		}
	}
	summary, summaryErr := GetAssetReplicationSummary(asset.Id)
	if summaryErr != nil {
		return nil, summaryErr
	}
	return &AssetLibraryReplicationReport{Summary: summary, Errors: errorsByChannel}, nil
}

func replicateAssetGroupToChannelLocked(ctx context.Context, group *model.UserAssetGroup, config *model.ChannelAssetConfig) (bool, error) {
	existing, err := model.GetUserAssetGroupReplica(group.Id, config.ChannelId)
	if err == nil && existing.UpstreamGroupId != "" {
		return false, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	replica := &model.UserAssetGroupReplica{
		GroupId:   group.Id,
		ChannelId: config.ChannelId,
		State:     model.AssetReplicaStateProcessing,
	}
	if existing != nil {
		replica.Id = existing.Id
		replica.CreatedTime = existing.CreatedTime
	}
	if err := model.SaveUserAssetGroupReplica(replica); err != nil {
		return false, err
	}
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
		replica.State = model.AssetReplicaStateFailed
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetGroupReplica(replica)
		return false, err
	}
	if strings.TrimSpace(result.Id) == "" {
		err := errors.New("asset library upstream returned an empty group id")
		replica.State = model.AssetReplicaStateFailed
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetGroupReplica(replica)
		return false, err
	}
	replica.UpstreamGroupId = result.Id
	replica.State = model.AssetReplicaStateReady
	replica.LastError = ""
	if err := model.SaveUserAssetGroupReplica(replica); err != nil {
		return false, err
	}
	return true, nil
}

func replicateAssetToChannelLocked(ctx context.Context, asset *model.UserAsset, config *model.ChannelAssetConfig) (bool, error) {
	existing, err := model.GetUserAssetReplica(asset.Id, config.ChannelId)
	if err == nil && existing.UpstreamAssetId != "" {
		return false, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	group, err := model.GetUserAssetGroup(asset.UserId, asset.GroupId)
	if err != nil {
		return false, err
	}
	if _, err := replicateAssetGroupToChannelLocked(ctx, group, config); err != nil {
		return false, fmt.Errorf("replicate asset group: %w", err)
	}
	groupReplica, err := model.GetUserAssetGroupReplica(asset.GroupId, config.ChannelId)
	if err != nil {
		return false, err
	}
	replica := &model.UserAssetReplica{
		AssetId:        asset.Id,
		ChannelId:      config.ChannelId,
		State:          model.AssetReplicaStateProcessing,
		UpstreamStatus: "Processing",
	}
	if existing != nil {
		replica.Id = existing.Id
		replica.CreatedTime = existing.CreatedTime
	}
	if err := model.SaveUserAssetReplica(replica); err != nil {
		return false, err
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
		replica.State = model.AssetReplicaStateFailed
		replica.UpstreamStatus = "Failed"
		replica.LastError = assetLibraryStoredError(err)
		if upstreamErr, ok := err.(*AssetLibraryUpstreamError); ok {
			replica.LastErrorCode = upstreamErr.Code
		}
		_ = model.SaveUserAssetReplica(replica)
		return false, err
	}
	if strings.TrimSpace(result.Id) == "" {
		err := errors.New("asset library upstream returned an empty asset id")
		replica.State = model.AssetReplicaStateFailed
		replica.UpstreamStatus = "Failed"
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetReplica(replica)
		return false, err
	}
	replica.UpstreamAssetId = result.Id
	replica.State = model.AssetReplicaStateReady
	replica.UpstreamStatus = "Processing"
	replica.LastErrorCode = ""
	replica.LastError = ""
	if err := model.SaveUserAssetReplica(replica); err != nil {
		return false, err
	}
	return true, nil
}

func SyncAssetLibraryChannel(ctx context.Context, channelId int) (*AssetLibrarySyncResult, error) {
	lock := getAssetLibraryChannelLock(channelId)
	lock.Lock()
	defer lock.Unlock()

	config, err := model.GetChannelAssetConfig(channelId)
	if err != nil {
		return nil, err
	}
	if !config.Enabled {
		return nil, errors.New("asset library is not enabled for channel")
	}
	groups, err := model.GetUserAssetGroupsForSync()
	if err != nil {
		return nil, err
	}
	result := &AssetLibrarySyncResult{ChannelId: channelId, Errors: make([]assetLibraryChannelError, 0)}
	for i := range groups {
		group := &groups[i]
		created, replicateErr := replicateAssetGroupToChannelLocked(ctx, group, config)
		if replicateErr != nil {
			result.GroupsFailed++
			result.Errors = appendSyncError(result.Errors, channelId, "group "+group.Id+": "+replicateErr.Error())
			assets, listErr := model.GetGroupAssetsForSync(group.Id)
			if listErr == nil {
				result.AssetsFailed += len(assets)
			}
			continue
		}
		if created {
			result.GroupsCreated++
		} else {
			result.GroupsSkipped++
		}
		assets, listErr := model.GetGroupAssetsForSync(group.Id)
		if listErr != nil {
			result.Errors = appendSyncError(result.Errors, channelId, "list assets for group "+group.Id+": "+listErr.Error())
			continue
		}
		for j := range assets {
			created, replicateErr := replicateAssetToChannelLocked(ctx, &assets[j], config)
			if replicateErr != nil {
				result.AssetsFailed++
				result.Errors = appendSyncError(result.Errors, channelId, "asset "+assets[j].Id+": "+replicateErr.Error())
				continue
			}
			if created {
				result.AssetsCreated++
			} else {
				result.AssetsSkipped++
			}
		}
	}
	return result, nil
}

func appendSyncError(items []assetLibraryChannelError, channelId int, message string) []assetLibraryChannelError {
	const maxSyncErrors = 100
	if len(items) >= maxSyncErrors {
		return items
	}
	return append(items, assetLibraryChannelError{ChannelId: channelId, Message: message})
}

func UpdateAssetGroupReplicas(ctx context.Context, group *model.UserAssetGroup) (*AssetLibraryReplicationReport, error) {
	replicas, err := model.ListUserAssetGroupReplicas(group.Id)
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range replicas {
		replica := &replicas[i]
		if replica.UpstreamGroupId == "" {
			continue
		}
		lock := getAssetLibraryChannelLock(replica.ChannelId)
		lock.Lock()
		currentReplica, replicaErr := model.GetUserAssetGroupReplica(group.Id, replica.ChannelId)
		if replicaErr != nil {
			lock.Unlock()
			if !errors.Is(replicaErr, gorm.ErrRecordNotFound) {
				errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: replicaErr.Error()})
			}
			continue
		}
		replica = currentReplica
		config, configErr := model.GetChannelAssetConfig(replica.ChannelId)
		if configErr == nil {
			upstreamConfig := *config
			upstreamConfig.Enabled = true
			projectName := assetLibraryProject(&upstreamConfig)
			request := dto.UpdateAssetGroupRequest{
				Id:          replica.UpstreamGroupId,
				Name:        &group.Name,
				Description: &group.Description,
				ProjectName: &projectName,
			}
			configErr = CallAssetLibraryUpstream(ctx, &upstreamConfig, "UpdateAssetGroup", request, nil)
		}
		if configErr != nil {
			replica.LastError = assetLibraryStoredError(configErr)
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: configErr.Error()})
		} else {
			replica.LastError = ""
			replica.State = model.AssetReplicaStateReady
		}
		_ = model.SaveUserAssetGroupReplica(replica)
		lock.Unlock()
	}
	summary, summaryErr := GetAssetGroupReplicationSummary(group.Id)
	if summaryErr != nil {
		return nil, summaryErr
	}
	return &AssetLibraryReplicationReport{Summary: summary, Errors: errorsByChannel}, nil
}

func UpdateAssetReplicas(ctx context.Context, asset *model.UserAsset) (*AssetLibraryReplicationReport, error) {
	replicas, err := model.ListUserAssetReplicas(asset.Id)
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range replicas {
		replica := &replicas[i]
		if replica.UpstreamAssetId == "" {
			continue
		}
		lock := getAssetLibraryChannelLock(replica.ChannelId)
		lock.Lock()
		currentReplica, replicaErr := model.GetUserAssetReplica(asset.Id, replica.ChannelId)
		if replicaErr != nil {
			lock.Unlock()
			if !errors.Is(replicaErr, gorm.ErrRecordNotFound) {
				errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: replicaErr.Error()})
			}
			continue
		}
		replica = currentReplica
		config, configErr := model.GetChannelAssetConfig(replica.ChannelId)
		if configErr == nil {
			upstreamConfig := *config
			upstreamConfig.Enabled = true
			projectName := assetLibraryProject(&upstreamConfig)
			request := dto.UpdateAssetRequest{Id: replica.UpstreamAssetId, Name: &asset.Name, ProjectName: &projectName}
			configErr = CallAssetLibraryUpstream(ctx, &upstreamConfig, "UpdateAsset", request, nil)
		}
		if configErr != nil {
			replica.LastError = assetLibraryStoredError(configErr)
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: configErr.Error()})
		} else {
			replica.LastErrorCode = ""
			replica.LastError = ""
			replica.State = model.AssetReplicaStateReady
		}
		_ = model.SaveUserAssetReplica(replica)
		lock.Unlock()
	}
	summary, summaryErr := GetAssetReplicationSummary(asset.Id)
	if summaryErr != nil {
		return nil, summaryErr
	}
	return &AssetLibraryReplicationReport{Summary: summary, Errors: errorsByChannel}, nil
}

func DeleteAssetReplicas(ctx context.Context, assetId string) ([]assetLibraryChannelError, error) {
	replicas, err := model.ListUserAssetReplicas(assetId)
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range replicas {
		replica := &replicas[i]
		if replica.UpstreamAssetId == "" {
			continue
		}
		lock := getAssetLibraryChannelLock(replica.ChannelId)
		lock.Lock()
		currentReplica, replicaErr := model.GetUserAssetReplica(assetId, replica.ChannelId)
		if replicaErr != nil {
			lock.Unlock()
			if !errors.Is(replicaErr, gorm.ErrRecordNotFound) {
				errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: replicaErr.Error()})
			}
			continue
		}
		replica = currentReplica
		config, deleteErr := model.GetChannelAssetConfig(replica.ChannelId)
		if deleteErr == nil {
			upstreamConfig := *config
			upstreamConfig.Enabled = true
			projectName := assetLibraryProject(&upstreamConfig)
			request := dto.DeleteAssetRequest{Id: replica.UpstreamAssetId, ProjectName: &projectName}
			deleteErr = CallAssetLibraryUpstream(ctx, &upstreamConfig, "DeleteAsset", request, nil)
			if isAssetLibraryNotFound(deleteErr) {
				deleteErr = nil
			}
		}
		lock.Unlock()
		if deleteErr != nil {
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: deleteErr.Error()})
		}
	}
	return errorsByChannel, nil
}

func DeleteAssetGroupReplicas(ctx context.Context, groupId string) ([]assetLibraryChannelError, error) {
	replicas, err := model.ListUserAssetGroupReplicas(groupId)
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range replicas {
		replica := &replicas[i]
		if replica.UpstreamGroupId == "" {
			continue
		}
		lock := getAssetLibraryChannelLock(replica.ChannelId)
		lock.Lock()
		currentReplica, replicaErr := model.GetUserAssetGroupReplica(groupId, replica.ChannelId)
		if replicaErr != nil {
			lock.Unlock()
			if !errors.Is(replicaErr, gorm.ErrRecordNotFound) {
				errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: replicaErr.Error()})
			}
			continue
		}
		replica = currentReplica
		config, deleteErr := model.GetChannelAssetConfig(replica.ChannelId)
		if deleteErr == nil {
			upstreamConfig := *config
			upstreamConfig.Enabled = true
			projectName := assetLibraryProject(&upstreamConfig)
			request := dto.DeleteAssetGroupRequest{Id: replica.UpstreamGroupId, ProjectName: &projectName}
			deleteErr = CallAssetLibraryUpstream(ctx, &upstreamConfig, "DeleteAssetGroup", request, nil)
			if isAssetLibraryNotFound(deleteErr) {
				deleteErr = nil
			}
		}
		lock.Unlock()
		if deleteErr != nil {
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: deleteErr.Error()})
		}
	}
	return errorsByChannel, nil
}

func isAssetLibraryNotFound(err error) bool {
	if err == nil {
		return false
	}
	var upstreamErr *AssetLibraryUpstreamError
	return errors.As(err, &upstreamErr) && strings.Contains(strings.ToLower(upstreamErr.Code), "notfound")
}

func RefreshAssetLibraryAsset(ctx context.Context, assetId string) (*AssetLibraryAssetDetails, error) {
	replicas, err := model.ListUserAssetReplicas(assetId)
	if err != nil {
		return nil, err
	}
	var refreshErrors []error
	for i := range replicas {
		replica := &replicas[i]
		if replica.UpstreamAssetId == "" {
			continue
		}
		lock := getAssetLibraryChannelLock(replica.ChannelId)
		lock.Lock()
		currentReplica, replicaErr := model.GetUserAssetReplica(assetId, replica.ChannelId)
		if replicaErr != nil {
			lock.Unlock()
			continue
		}
		replica = currentReplica
		config, err := model.GetChannelAssetConfig(replica.ChannelId)
		if err != nil || !config.Enabled {
			lock.Unlock()
			continue
		}
		projectName := assetLibraryProject(config)
		request := dto.GetAssetRequest{Id: replica.UpstreamAssetId, ProjectName: &projectName}
		var details AssetLibraryAssetDetails
		err = CallAssetLibraryUpstream(ctx, config, "GetAsset", request, &details)
		if err != nil {
			replica.LastError = assetLibraryStoredError(err)
			if upstreamErr, ok := err.(*AssetLibraryUpstreamError); ok {
				replica.LastErrorCode = upstreamErr.Code
			}
			_ = model.SaveUserAssetReplica(replica)
			lock.Unlock()
			refreshErrors = append(refreshErrors, err)
			continue
		}
		replica.UpstreamStatus = details.Status
		replica.LastInferenceTime = details.LastInferenceTime
		replica.LastErrorCode = ""
		replica.LastError = ""
		if details.Error != nil {
			replica.LastErrorCode = details.Error.Code
			replica.LastError = common.MaskSensitiveInfo(common.LocalLogPreview(details.Error.Message))
		}
		_ = model.SaveUserAssetReplica(replica)
		lock.Unlock()
		return &details, nil
	}
	if len(refreshErrors) > 0 {
		return nil, errors.Join(refreshErrors...)
	}
	return nil, errors.New("asset has no available upstream replica")
}

func assetLibraryStoredError(err error) string {
	if err == nil {
		return ""
	}
	return common.MaskSensitiveInfo(common.LocalLogPreview(err.Error()))
}

func GetAssetGroupReplicationSummary(groupId string) (*dto.AssetReplicaSummary, error) {
	replicas, err := model.ListUserAssetGroupReplicas(groupId)
	if err != nil {
		return nil, err
	}
	configs, err := model.GetEnabledChannelAssetConfigs()
	if err != nil {
		return nil, err
	}
	enabled := make(map[int]struct{}, len(configs))
	for _, config := range configs {
		enabled[config.ChannelId] = struct{}{}
	}
	summary := &dto.AssetReplicaSummary{Total: len(configs)}
	for _, replica := range replicas {
		if _, ok := enabled[replica.ChannelId]; !ok {
			continue
		}
		if replica.UpstreamGroupId != "" {
			summary.Ready++
		} else if replica.State == model.AssetReplicaStateFailed {
			summary.Failed++
		} else {
			summary.Processing++
		}
	}
	summary.Processing += summary.Total - summary.Ready - summary.Failed - summary.Processing
	summary.Status = assetReplicationStatus(summary)
	return summary, nil
}

func GetAssetReplicationSummary(assetId string) (*dto.AssetReplicaSummary, error) {
	replicas, err := model.ListUserAssetReplicas(assetId)
	if err != nil {
		return nil, err
	}
	configs, err := model.GetEnabledChannelAssetConfigs()
	if err != nil {
		return nil, err
	}
	enabled := make(map[int]struct{}, len(configs))
	for _, config := range configs {
		enabled[config.ChannelId] = struct{}{}
	}
	summary := &dto.AssetReplicaSummary{Total: len(configs)}
	for _, replica := range replicas {
		if _, ok := enabled[replica.ChannelId]; !ok {
			continue
		}
		if replica.UpstreamAssetId != "" {
			summary.Ready++
		} else if replica.State == model.AssetReplicaStateFailed {
			summary.Failed++
		} else {
			summary.Processing++
		}
	}
	summary.Processing += summary.Total - summary.Ready - summary.Failed - summary.Processing
	summary.Status = assetReplicationStatus(summary)
	return summary, nil
}

func GetAssetLibraryAggregateState(assetId string) (string, *dto.AssetLibraryError, string, error) {
	replicas, err := model.ListUserAssetReplicas(assetId)
	if err != nil {
		return "", nil, "", err
	}
	status := "Processing"
	lastInferenceTime := ""
	failed := 0
	var assetError *dto.AssetLibraryError
	configs, err := model.GetEnabledChannelAssetConfigs()
	if err != nil {
		return "", nil, "", err
	}
	enabled := make(map[int]struct{}, len(configs))
	for _, config := range configs {
		enabled[config.ChannelId] = struct{}{}
	}
	considered := 0
	for _, replica := range replicas {
		if _, ok := enabled[replica.ChannelId]; !ok {
			continue
		}
		considered++
		if strings.EqualFold(replica.UpstreamStatus, "Active") {
			status = "Active"
		}
		if strings.EqualFold(replica.UpstreamStatus, "Failed") {
			failed++
			if assetError == nil && (replica.LastErrorCode != "" || replica.LastError != "") {
				assetError = &dto.AssetLibraryError{Code: "AssetProcessingFailed", Message: "Asset processing failed"}
			}
		}
		if replica.LastInferenceTime > lastInferenceTime {
			lastInferenceTime = replica.LastInferenceTime
		}
	}
	if considered > 0 && failed == considered {
		status = "Failed"
	}
	return status, assetError, lastInferenceTime, nil
}

func assetReplicationStatus(summary *dto.AssetReplicaSummary) string {
	if summary.Total == 0 {
		return "unavailable"
	}
	if summary.Ready == summary.Total {
		return "ready"
	}
	if summary.Ready > 0 {
		return "partial"
	}
	if summary.Failed == summary.Total {
		return "failed"
	}
	return "processing"
}

func RewriteAssetReferences(userId int, channelId int, payload map[string]any) (map[string]any, error) {
	assetIds := make(map[string]struct{})
	if err := collectAssetLibraryReferences(payload, assetIds); err != nil {
		return nil, err
	}
	orderedIds := make([]string, 0, len(assetIds))
	for assetId := range assetIds {
		orderedIds = append(orderedIds, assetId)
	}
	sort.Strings(orderedIds)
	mappings := make(map[string]string)
	if len(orderedIds) > 0 {
		var err error
		mappings, err = model.GetAssetReplicaMappings(userId, channelId, orderedIds)
		if err != nil {
			return nil, err
		}
		missing := make([]string, 0)
		for _, assetId := range orderedIds {
			if mappings[assetId] == "" {
				missing = append(missing, assetId)
			}
		}
		if len(missing) > 0 {
			return nil, fmt.Errorf("asset replica is unavailable for channel: %s", strings.Join(missing, ", "))
		}
	}
	rewritten, _ := rewriteAssetLibraryValue(payload, mappings).(map[string]any)
	return rewritten, nil
}

// RejectAssetReferences rejects provider asset URIs on request formats that do
// not support New API logical asset routing. This prevents raw upstream IDs
// from bypassing account ownership through a shared channel credential.
func RejectAssetReferences(payload any) error {
	if containsAssetLibraryReference(payload) {
		return errors.New("invalid asset URI; use the native endpoint with an account asset ID")
	}
	return nil
}

func containsAssetLibraryReference(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if containsAssetLibraryReference(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsAssetLibraryReference(child) {
				return true
			}
		}
	case string:
		return hasAssetLibraryURIScheme(typed)
	}
	return false
}

func collectAssetLibraryReferences(value any, assetIds map[string]struct{}) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, child := range typed {
			if err := collectAssetLibraryReferences(child, assetIds); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := collectAssetLibraryReferences(child, assetIds); err != nil {
				return err
			}
		}
	case string:
		if assetId, ok := parseLocalAssetReference(typed); ok {
			assetIds[assetId] = struct{}{}
		} else if hasAssetLibraryURIScheme(typed) {
			return errors.New("invalid asset URI; use an account asset ID")
		}
	}
	return nil
}

func hasAssetLibraryURIScheme(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= len("asset://") && strings.EqualFold(value[:len("asset://")], "asset://")
}

func rewriteAssetLibraryValue(value any, mappings map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = rewriteAssetLibraryValue(child, mappings)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			result[i] = rewriteAssetLibraryValue(child, mappings)
		}
		return result
	case string:
		if assetId, ok := parseLocalAssetReference(typed); ok {
			return "asset://" + mappings[assetId]
		}
		return typed
	default:
		return typed
	}
}

func parseLocalAssetReference(value string) (string, bool) {
	const prefix = "asset://asset-na-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+32 {
		return "", false
	}
	for _, char := range value[len(prefix):] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return "", false
		}
	}
	return strings.TrimPrefix(value, "asset://"), true
}

package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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
	ChannelId int    `json:"channel_id"`
	AssetId   string `json:"asset_id,omitempty"`
	Message   string `json:"message"`
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

const AssetReplicaStateNotSynced = "not_synced"

type AssetChannelReplicaDetails struct {
	ChannelId         int    `json:"channel_id"`
	ChannelName       string `json:"channel_name"`
	Backend           string `json:"backend"`
	Enabled           bool   `json:"enabled"`
	State             string `json:"state"`
	UpstreamAssetId   string `json:"upstream_asset_id"`
	UpstreamStatus    string `json:"upstream_status"`
	LastErrorCode     string `json:"last_error_code,omitempty"`
	LastError         string `json:"last_error,omitempty"`
	LastInferenceTime string `json:"last_inference_time,omitempty"`
	UpdatedTime       int64  `json:"updated_time,omitempty"`
}

type AssetReplicaDetailsResult struct {
	Summary  *dto.AssetReplicaSummary     `json:"summary"`
	Replicas []AssetChannelReplicaDetails `json:"replicas"`
}

type AssetGroupChannelReplicaDetails struct {
	ChannelId       int    `json:"channel_id"`
	ChannelName     string `json:"channel_name"`
	Backend         string `json:"backend"`
	Enabled         bool   `json:"enabled"`
	State           string `json:"state"`
	UpstreamGroupId string `json:"upstream_group_id"`
	LastError       string `json:"last_error,omitempty"`
	UpdatedTime     int64  `json:"updated_time,omitempty"`
}

type AssetGroupReplicaDetailsResult struct {
	Summary  *dto.AssetReplicaSummary          `json:"summary"`
	Replicas []AssetGroupChannelReplicaDetails `json:"replicas"`
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
	backend, err := effectiveAssetLibraryBackend(config.Backend, config.ChannelId)
	if err != nil {
		return nil, err
	}
	config.Backend = backend
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
	if !newConfig {
		existingBackend, backendErr := effectiveAssetLibraryBackend(existing.Backend, existing.ChannelId)
		if backendErr != nil {
			return nil, backendErr
		}
		if existingBackend != config.Backend {
			changedFields = append(changedFields, "backend")
			identityChanged = true
		}
	} else {
		changedFields = append(changedFields, "backend")
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
				"enabled", "backend", "base_url", "auth_type", "access_key", "secret_key", "api_key",
				"region", "project_name", "updated_time",
			}),
		}).Create(config).Error
	}); err != nil {
		return nil, err
	}
	return changedFields, nil
}

func effectiveAssetLibraryBackend(backend string, channelId int) (string, error) {
	backend = strings.TrimSpace(backend)
	if backend != "" {
		if !IsSupportedAssetLibraryBackend(backend) {
			return "", fmt.Errorf("unsupported asset library backend %q", backend)
		}
		return backend, nil
	}
	channel, err := model.GetChannelById(channelId, false)
	if err == nil {
		return DefaultAssetLibraryBackend(channel.Type), nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return AssetLibraryBackendAction, nil
	}
	return "", err
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

func GetAssetReplicaDetails(assetId string) (*AssetReplicaDetailsResult, error) {
	configs, err := model.ListChannelAssetConfigs()
	if err != nil {
		return nil, err
	}
	replicas, err := model.ListUserAssetReplicas(assetId)
	if err != nil {
		return nil, err
	}
	channelNames, err := assetLibraryChannelNames(configs)
	if err != nil {
		return nil, err
	}
	replicasByChannel := make(map[int]model.UserAssetReplica, len(replicas))
	for _, replica := range replicas {
		replicasByChannel[replica.ChannelId] = replica
	}
	items := make([]AssetChannelReplicaDetails, 0, len(configs))
	for _, config := range configs {
		backend, err := effectiveAssetLibraryBackend(config.Backend, config.ChannelId)
		if err != nil {
			return nil, err
		}
		item := AssetChannelReplicaDetails{
			ChannelId:   config.ChannelId,
			ChannelName: channelNames[config.ChannelId],
			Backend:     backend,
			Enabled:     config.Enabled,
			State:       AssetReplicaStateNotSynced,
		}
		if replica, ok := replicasByChannel[config.ChannelId]; ok {
			item.State = replica.State
			item.UpstreamAssetId = replica.UpstreamAssetId
			item.UpstreamStatus = replica.UpstreamStatus
			item.LastErrorCode = replica.LastErrorCode
			item.LastError = replica.LastError
			item.LastInferenceTime = replica.LastInferenceTime
			item.UpdatedTime = replica.UpdatedTime
		}
		items = append(items, item)
	}
	summary, err := GetAssetReplicationSummary(assetId)
	if err != nil {
		return nil, err
	}
	return &AssetReplicaDetailsResult{Summary: summary, Replicas: items}, nil
}

func GetAssetGroupReplicaDetails(groupId string) (*AssetGroupReplicaDetailsResult, error) {
	configs, err := model.ListChannelAssetConfigs()
	if err != nil {
		return nil, err
	}
	replicas, err := model.ListUserAssetGroupReplicas(groupId)
	if err != nil {
		return nil, err
	}
	channelNames, err := assetLibraryChannelNames(configs)
	if err != nil {
		return nil, err
	}
	replicasByChannel := make(map[int]model.UserAssetGroupReplica, len(replicas))
	for _, replica := range replicas {
		replicasByChannel[replica.ChannelId] = replica
	}
	items := make([]AssetGroupChannelReplicaDetails, 0, len(configs))
	for _, config := range configs {
		backend, err := effectiveAssetLibraryBackend(config.Backend, config.ChannelId)
		if err != nil {
			return nil, err
		}
		item := AssetGroupChannelReplicaDetails{
			ChannelId:   config.ChannelId,
			ChannelName: channelNames[config.ChannelId],
			Backend:     backend,
			Enabled:     config.Enabled,
			State:       AssetReplicaStateNotSynced,
		}
		if replica, ok := replicasByChannel[config.ChannelId]; ok {
			item.State = replica.State
			item.UpstreamGroupId = replica.UpstreamGroupId
			item.LastError = replica.LastError
			item.UpdatedTime = replica.UpdatedTime
		}
		items = append(items, item)
	}
	summary, err := GetAssetGroupReplicationSummary(groupId)
	if err != nil {
		return nil, err
	}
	return &AssetGroupReplicaDetailsResult{Summary: summary, Replicas: items}, nil
}

func assetLibraryChannelNames(configs []model.ChannelAssetConfig) (map[int]string, error) {
	ids := make([]int, 0, len(configs))
	for _, config := range configs {
		ids = append(ids, config.ChannelId)
	}
	names := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	channels, err := model.GetChannelsByIds(ids)
	if err != nil {
		return nil, err
	}
	for _, channel := range channels {
		names[channel.Id] = channel.Name
	}
	return names, nil
}

func SyncAssetReplicas(ctx context.Context, asset *model.UserAsset, channelIds []int) (*AssetLibraryReplicationReport, error) {
	configs, err := selectedAssetLibraryConfigs(channelIds)
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range configs {
		config := &configs[i]
		lock := getAssetLibraryChannelLock(config.ChannelId)
		lock.Lock()
		currentConfig, configErr := model.GetChannelAssetConfig(config.ChannelId)
		if configErr == nil && !currentConfig.Enabled {
			configErr = errors.New("asset library is not enabled for channel")
		}
		if configErr == nil {
			replica, replicaErr := model.GetUserAssetReplica(asset.Id, config.ChannelId)
			switch {
			case replicaErr == nil && replica.UpstreamAssetId != "":
				_, configErr = refreshAssetReplicaToChannelLocked(ctx, currentConfig, replica)
			case replicaErr == nil || errors.Is(replicaErr, gorm.ErrRecordNotFound):
				_, configErr = replicateAssetToChannelLocked(ctx, asset, currentConfig)
			default:
				configErr = replicaErr
			}
		}
		lock.Unlock()
		if configErr != nil {
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: config.ChannelId, Message: assetLibraryStoredError(configErr)})
		}
	}
	summary, err := GetAssetReplicationSummary(asset.Id)
	if err != nil {
		return nil, err
	}
	return &AssetLibraryReplicationReport{Summary: summary, Errors: errorsByChannel}, nil
}

func SyncAssetGroupReplicas(ctx context.Context, group *model.UserAssetGroup, channelIds []int) (*AssetLibraryReplicationReport, error) {
	configs, err := selectedAssetLibraryConfigs(channelIds)
	if err != nil {
		return nil, err
	}
	assets, err := model.GetGroupAssetsForSync(group.Id)
	if err != nil {
		return nil, err
	}
	errorsByChannel := make([]assetLibraryChannelError, 0)
	for i := range configs {
		config := &configs[i]
		lock := getAssetLibraryChannelLock(config.ChannelId)
		lock.Lock()
		currentConfig, configErr := model.GetChannelAssetConfig(config.ChannelId)
		if configErr == nil && !currentConfig.Enabled {
			configErr = errors.New("asset library is not enabled for channel")
		}
		if configErr == nil {
			_, configErr = replicateAssetGroupToChannelLocked(ctx, group, currentConfig)
		}
		if configErr == nil {
			for j := range assets {
				asset := &assets[j]
				replica, replicaErr := model.GetUserAssetReplica(asset.Id, config.ChannelId)
				switch {
				case replicaErr == nil && replica.UpstreamAssetId != "":
					_, replicaErr = refreshAssetReplicaToChannelLocked(ctx, currentConfig, replica)
				case replicaErr == nil || errors.Is(replicaErr, gorm.ErrRecordNotFound):
					_, replicaErr = replicateAssetToChannelLocked(ctx, asset, currentConfig)
				}
				if replicaErr != nil {
					errorsByChannel = appendAssetSyncError(
						errorsByChannel,
						config.ChannelId,
						asset.Id,
						assetLibraryStoredError(replicaErr),
					)
				}
			}
		}
		lock.Unlock()
		if configErr != nil {
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: config.ChannelId, Message: assetLibraryStoredError(configErr)})
		}
	}
	summary, err := GetAssetGroupReplicationSummary(group.Id)
	if err != nil {
		return nil, err
	}
	return &AssetLibraryReplicationReport{Summary: summary, Errors: errorsByChannel}, nil
}

func appendAssetSyncError(items []assetLibraryChannelError, channelId int, assetId string, message string) []assetLibraryChannelError {
	const maxSyncErrors = 100
	if len(items) >= maxSyncErrors {
		return items
	}
	return append(items, assetLibraryChannelError{ChannelId: channelId, AssetId: assetId, Message: message})
}

func selectedAssetLibraryConfigs(channelIds []int) ([]model.ChannelAssetConfig, error) {
	if len(channelIds) == 0 {
		configs, err := model.GetEnabledChannelAssetConfigs()
		if err != nil {
			return nil, err
		}
		if len(configs) == 0 {
			return nil, errors.New("no enabled asset library channels are available")
		}
		return configs, nil
	}
	seen := make(map[int]struct{}, len(channelIds))
	configs := make([]model.ChannelAssetConfig, 0, len(channelIds))
	for _, channelId := range channelIds {
		if channelId <= 0 {
			return nil, errors.New("channel id must be positive")
		}
		if _, ok := seen[channelId]; ok {
			continue
		}
		seen[channelId] = struct{}{}
		config, err := model.GetChannelAssetConfig(channelId)
		if err != nil {
			return nil, err
		}
		if !config.Enabled {
			return nil, fmt.Errorf("asset library is not enabled for channel %d", channelId)
		}
		configs = append(configs, *config)
	}
	return configs, nil
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
	backend, err := assetLibraryBackendForChannel(config.ChannelId)
	if err != nil {
		replica.State = model.AssetReplicaStateFailed
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetGroupReplica(replica)
		return false, err
	}
	result, err := backend.CreateGroup(ctx, config, group)
	if err != nil {
		replica.State = model.AssetReplicaStateFailed
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetGroupReplica(replica)
		return false, err
	}
	if result.Deferred {
		replica.State = model.AssetReplicaStateProcessing
		replica.LastError = ""
		if err := model.SaveUserAssetGroupReplica(replica); err != nil {
			return false, err
		}
		return false, nil
	}
	if strings.TrimSpace(result.GroupID) == "" {
		err := errors.New("asset library upstream returned an empty group id")
		replica.State = model.AssetReplicaStateFailed
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetGroupReplica(replica)
		return false, err
	}
	replica.UpstreamGroupId = result.GroupID
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
	backend, err := assetLibraryBackendForChannel(config.ChannelId)
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
	result, err := backend.CreateAsset(ctx, config, group, groupReplica, asset)
	if err != nil {
		replica.State = model.AssetReplicaStateFailed
		replica.UpstreamStatus = "Failed"
		replica.LastError = assetLibraryStoredError(err)
		if upstreamErr, ok := err.(*AssetLibraryUpstreamError); ok {
			replica.LastErrorCode = upstreamErr.Code
		}
		_ = model.SaveUserAssetReplica(replica)
		return false, err
	}
	if strings.TrimSpace(result.AssetID) == "" {
		err := errors.New("asset library upstream returned an empty asset id")
		replica.State = model.AssetReplicaStateFailed
		replica.UpstreamStatus = "Failed"
		replica.LastError = assetLibraryStoredError(err)
		_ = model.SaveUserAssetReplica(replica)
		return false, err
	}
	if strings.TrimSpace(result.GroupID) != "" && groupReplica.UpstreamGroupId == "" {
		groupReplica.UpstreamGroupId = result.GroupID
		groupReplica.State = model.AssetReplicaStateReady
		groupReplica.LastError = ""
		if err := model.SaveUserAssetGroupReplica(groupReplica); err != nil {
			return false, err
		}
	}
	replica.UpstreamAssetId = result.AssetID
	replica.UpstreamStatus = strings.TrimSpace(result.Status)
	if replica.UpstreamStatus == "" {
		replica.UpstreamStatus = "Processing"
	}
	replica.State = assetReplicaStateForStatus(replica.UpstreamStatus)
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
			backend, backendErr := assetLibraryBackendForChannel(replica.ChannelId)
			if backendErr != nil {
				configErr = backendErr
			} else {
				configErr = backend.UpdateGroup(ctx, &upstreamConfig, group, replica.UpstreamGroupId)
			}
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
			backend, backendErr := assetLibraryBackendForChannel(replica.ChannelId)
			if backendErr != nil {
				configErr = backendErr
			} else {
				configErr = backend.UpdateAsset(ctx, &upstreamConfig, asset, replica.UpstreamAssetId)
			}
		}
		if configErr != nil {
			replica.LastError = assetLibraryStoredError(configErr)
			errorsByChannel = append(errorsByChannel, assetLibraryChannelError{ChannelId: replica.ChannelId, Message: configErr.Error()})
		} else {
			replica.LastErrorCode = ""
			replica.LastError = ""
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
			backend, backendErr := assetLibraryBackendForChannel(replica.ChannelId)
			if backendErr != nil {
				deleteErr = backendErr
			} else {
				deleteErr = backend.DeleteAsset(ctx, &upstreamConfig, replica.UpstreamAssetId)
			}
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
			backend, backendErr := assetLibraryBackendForChannel(replica.ChannelId)
			if backendErr != nil {
				deleteErr = backendErr
			} else {
				deleteErr = backend.DeleteGroup(ctx, &upstreamConfig, replica.UpstreamGroupId)
			}
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
	if !errors.As(err, &upstreamErr) {
		return false
	}
	if upstreamErr.StatusCode == http.StatusNotFound || upstreamErr.Code == "3001" || upstreamErr.Code == "3002" {
		return true
	}
	return strings.Contains(strings.ToLower(upstreamErr.Code), "notfound")
}

func RefreshAssetLibraryAsset(ctx context.Context, assetId string) (*AssetLibraryAssetDetails, error) {
	return refreshAssetLibraryAsset(ctx, assetId, false)
}

func RefreshAdminAssetLibraryAsset(ctx context.Context, assetId string) (*AssetLibraryAssetDetails, error) {
	return refreshAssetLibraryAsset(ctx, assetId, true)
}

func refreshAssetLibraryAsset(ctx context.Context, assetId string, includeDisabled bool) (*AssetLibraryAssetDetails, error) {
	replicas, err := model.ListUserAssetReplicas(assetId)
	if err != nil {
		return nil, err
	}
	var refreshErrors []error
	var selectedDetails *AssetLibraryAssetDetails
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
		if err != nil || (!includeDisabled && !config.Enabled) {
			lock.Unlock()
			continue
		}
		requestConfig := config
		if includeDisabled && !config.Enabled {
			inspectionConfig := *config
			inspectionConfig.Enabled = true
			requestConfig = &inspectionConfig
		}
		details, err := refreshAssetReplicaToChannelLocked(ctx, requestConfig, replica)
		lock.Unlock()
		if err != nil {
			refreshErrors = append(refreshErrors, err)
			continue
		}
		if selectedDetails == nil {
			selectedDetails = details
			continue
		}
		selectedIsActive := strings.EqualFold(selectedDetails.Status, "Active")
		detailsIsActive := strings.EqualFold(details.Status, "Active")
		if (!selectedIsActive && detailsIsActive) ||
			(selectedIsActive == detailsIsActive && strings.TrimSpace(selectedDetails.URL) == "" && strings.TrimSpace(details.URL) != "") {
			selectedDetails = details
		}
	}
	if selectedDetails != nil {
		return selectedDetails, nil
	}
	if len(refreshErrors) > 0 {
		return nil, errors.Join(refreshErrors...)
	}
	return nil, errors.New("asset has no available upstream replica")
}

func refreshAssetReplicaToChannelLocked(ctx context.Context, config *model.ChannelAssetConfig, replica *model.UserAssetReplica) (*AssetLibraryAssetDetails, error) {
	backend, err := assetLibraryBackendForChannel(replica.ChannelId)
	if err != nil {
		return nil, err
	}
	details, err := backend.GetAsset(ctx, config, replica.UpstreamAssetId)
	if err != nil {
		replica.LastError = assetLibraryStoredError(err)
		var upstreamErr *AssetLibraryUpstreamError
		if errors.As(err, &upstreamErr) {
			replica.LastErrorCode = upstreamErr.Code
		}
		if saveErr := model.SaveUserAssetReplica(replica); saveErr != nil {
			return nil, errors.Join(err, saveErr)
		}
		return nil, err
	}
	replica.UpstreamStatus = details.Status
	replica.State = assetReplicaStateForStatus(details.Status)
	replica.LastInferenceTime = details.LastInferenceTime
	replica.LastErrorCode = ""
	replica.LastError = ""
	if details.Error != nil {
		replica.LastErrorCode = details.Error.Code
		replica.LastError = common.MaskSensitiveInfo(common.LocalLogPreview(details.Error.Message))
	}
	if err := model.SaveUserAssetReplica(replica); err != nil {
		return nil, err
	}
	return details, nil
}

func assetReplicaStateForStatus(status string) string {
	switch {
	case strings.EqualFold(strings.TrimSpace(status), "Active"):
		return model.AssetReplicaStateReady
	case strings.EqualFold(strings.TrimSpace(status), "Failed"):
		return model.AssetReplicaStateFailed
	default:
		return model.AssetReplicaStateProcessing
	}
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
		if replica.State == model.AssetReplicaStateReady && replica.UpstreamGroupId != "" {
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
		if replica.State == model.AssetReplicaStateReady && replica.UpstreamAssetId != "" {
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
		backend, err := assetLibraryBackendForChannel(channelId)
		if err != nil {
			return nil, err
		}
		for assetId, upstreamAssetId := range mappings {
			mappings[assetId] = backend.FormatAssetReference(upstreamAssetId)
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
			return mappings[assetId]
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

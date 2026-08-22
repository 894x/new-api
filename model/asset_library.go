package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AssetReplicaStateReady      = "ready"
	AssetReplicaStateProcessing = "processing"
	AssetReplicaStateFailed     = "failed"
)

type ChannelAssetConfig struct {
	ChannelId   int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false"`
	Enabled     bool   `json:"enabled" gorm:"not null"`
	Backend     string `json:"backend" gorm:"type:varchar(32)"`
	BaseURL     string `json:"base_url" gorm:"type:varchar(2048);not null"`
	AuthType    string `json:"auth_type" gorm:"type:varchar(16);not null"`
	AccessKey   string `json:"-" gorm:"type:text"`
	SecretKey   string `json:"-" gorm:"type:text"`
	APIKey      string `json:"-" gorm:"type:text"`
	Region      string `json:"region" gorm:"type:varchar(64);not null"`
	ProjectName string `json:"project_name" gorm:"type:varchar(128);not null"`
	CreatedTime int64  `json:"created_time" gorm:"autoCreateTime"`
	UpdatedTime int64  `json:"updated_time" gorm:"autoUpdateTime"`
}

func (ChannelAssetConfig) TableName() string {
	return "channel_asset_configs"
}

type UserAssetGroup struct {
	Id          string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserId      int    `json:"user_id" gorm:"not null;index:idx_user_asset_groups_user_created,priority:1"`
	Name        string `json:"name" gorm:"type:varchar(64);not null"`
	Description string `json:"description" gorm:"type:varchar(300)"`
	GroupType   string `json:"group_type" gorm:"type:varchar(32);not null"`
	ProjectName string `json:"project_name" gorm:"type:varchar(128);not null;index"`
	CreatedTime int64  `json:"created_time" gorm:"autoCreateTime;index:idx_user_asset_groups_user_created,priority:2"`
	UpdatedTime int64  `json:"updated_time" gorm:"autoUpdateTime"`
}

func (UserAssetGroup) TableName() string {
	return "user_asset_groups"
}

type UserAsset struct {
	Id          string `json:"id" gorm:"type:varchar(64);primaryKey"`
	UserId      int    `json:"user_id" gorm:"not null;index:idx_user_assets_user_created,priority:1"`
	GroupId     string `json:"group_id" gorm:"type:varchar(64);not null;index"`
	Name        string `json:"name" gorm:"type:varchar(64)"`
	SourceURL   string `json:"-" gorm:"type:text;not null"`
	AssetType   string `json:"asset_type" gorm:"type:varchar(32);not null;index"`
	ProjectName string `json:"project_name" gorm:"type:varchar(128);not null;index"`
	CreatedTime int64  `json:"created_time" gorm:"autoCreateTime;index:idx_user_assets_user_created,priority:2"`
	UpdatedTime int64  `json:"updated_time" gorm:"autoUpdateTime"`
}

func (UserAsset) TableName() string {
	return "user_assets"
}

type UserAssetGroupReplica struct {
	Id              int    `json:"id" gorm:"primaryKey"`
	GroupId         string `json:"group_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_asset_group_replica,priority:1"`
	ChannelId       int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_asset_group_replica,priority:2;index"`
	UpstreamGroupId string `json:"upstream_group_id" gorm:"type:varchar(256)"`
	State           string `json:"state" gorm:"type:varchar(16);not null;index"`
	LastError       string `json:"last_error,omitempty" gorm:"type:text"`
	CreatedTime     int64  `json:"created_time" gorm:"autoCreateTime"`
	UpdatedTime     int64  `json:"updated_time" gorm:"autoUpdateTime"`
}

func (UserAssetGroupReplica) TableName() string {
	return "user_asset_group_replicas"
}

type UserAssetReplica struct {
	Id                int    `json:"id" gorm:"primaryKey"`
	AssetId           string `json:"asset_id" gorm:"type:varchar(64);not null;uniqueIndex:idx_asset_replica,priority:1"`
	ChannelId         int    `json:"channel_id" gorm:"not null;uniqueIndex:idx_asset_replica,priority:2;index"`
	UpstreamAssetId   string `json:"upstream_asset_id" gorm:"type:varchar(256)"`
	State             string `json:"state" gorm:"type:varchar(16);not null;index"`
	UpstreamStatus    string `json:"upstream_status,omitempty" gorm:"type:varchar(64)"`
	LastErrorCode     string `json:"last_error_code,omitempty" gorm:"type:varchar(128)"`
	LastError         string `json:"last_error,omitempty" gorm:"type:text"`
	LastInferenceTime string `json:"last_inference_time,omitempty" gorm:"type:varchar(64)"`
	CreatedTime       int64  `json:"created_time" gorm:"autoCreateTime"`
	UpdatedTime       int64  `json:"updated_time" gorm:"autoUpdateTime"`
}

func (UserAssetReplica) TableName() string {
	return "user_asset_replicas"
}

type AssetGroupListParams struct {
	GroupIds    []string
	GroupType   string
	Name        string
	ProjectName string
	PageNumber  int64
	PageSize    int64
	SortBy      string
	SortOrder   string
}

type AssetListParams struct {
	GroupIds    []string
	GroupType   string
	Statuses    []string
	Name        string
	AssetType   string
	ProjectName string
	PageNumber  int64
	PageSize    int64
	SortBy      string
	SortOrder   string
}

func GetChannelAssetConfig(channelId int) (*ChannelAssetConfig, error) {
	var config ChannelAssetConfig
	if err := DB.First(&config, "channel_id = ?", channelId).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

func GetEnabledChannelAssetConfigs() ([]ChannelAssetConfig, error) {
	var configs []ChannelAssetConfig
	err := DB.Where("enabled = ?", true).Order("channel_id ASC").Find(&configs).Error
	return configs, err
}

func CountChannelAssetReplicas(channelId int) (int64, error) {
	var groupCount int64
	if err := DB.Model(&UserAssetGroupReplica{}).Where("channel_id = ?", channelId).Count(&groupCount).Error; err != nil {
		return 0, err
	}
	var assetCount int64
	if err := DB.Model(&UserAssetReplica{}).Where("channel_id = ?", channelId).Count(&assetCount).Error; err != nil {
		return 0, err
	}
	return groupCount + assetCount, nil
}

func CreateUserAssetGroup(group *UserAssetGroup) error {
	return DB.Create(group).Error
}

func GetUserAssetGroup(userId int, groupId string) (*UserAssetGroup, error) {
	var group UserAssetGroup
	if err := DB.Where("id = ? AND user_id = ?", groupId, userId).First(&group).Error; err != nil {
		return nil, err
	}
	return &group, nil
}

func ListUserAssetGroups(userId int, params AssetGroupListParams) ([]UserAssetGroup, int64, error) {
	query := DB.Model(&UserAssetGroup{}).Where("user_id = ?", userId)
	if len(params.GroupIds) > 0 {
		query = query.Where("id IN ?", params.GroupIds)
	}
	if params.GroupType != "" {
		query = query.Where("group_type = ?", params.GroupType)
	}
	if params.Name != "" {
		query = query.Where("name LIKE ?", "%"+params.Name+"%")
	}
	if params.ProjectName != "" {
		query = query.Where("project_name = ?", params.ProjectName)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	orderColumn := "created_time"
	if params.SortBy == "UpdateTime" {
		orderColumn = "updated_time"
	}
	orderDirection := "DESC"
	if strings.EqualFold(params.SortOrder, "Asc") {
		orderDirection = "ASC"
	}
	var groups []UserAssetGroup
	err := query.Order(orderColumn + " " + orderDirection).
		Limit(int(params.PageSize)).
		Offset(int((params.PageNumber - 1) * params.PageSize)).
		Find(&groups).Error
	return groups, total, err
}

func UpdateUserAssetGroup(group *UserAssetGroup) error {
	return DB.Model(&UserAssetGroup{}).
		Where("id = ? AND user_id = ?", group.Id, group.UserId).
		Select("name", "description", "updated_time").
		Updates(group).Error
}

func CreateUserAsset(asset *UserAsset) error {
	return DB.Create(asset).Error
}

func GetUserAsset(userId int, assetId string) (*UserAsset, error) {
	var asset UserAsset
	if err := DB.Where("id = ? AND user_id = ?", assetId, userId).First(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func ListUserAssets(userId int, params AssetListParams) ([]UserAsset, int64, error) {
	query := DB.Model(&UserAsset{}).Where("user_id = ?", userId)
	if len(params.GroupIds) > 0 {
		query = query.Where("group_id IN ?", params.GroupIds)
	}
	if params.Name != "" {
		query = query.Where("name LIKE ?", "%"+params.Name+"%")
	}
	if params.AssetType != "" {
		query = query.Where("asset_type = ?", params.AssetType)
	}
	if params.ProjectName != "" {
		query = query.Where("project_name = ?", params.ProjectName)
	}
	if params.GroupType != "" {
		query = query.Where("group_id IN (?)", DB.Model(&UserAssetGroup{}).
			Select("id").Where("user_id = ? AND group_type = ?", userId, params.GroupType))
	}
	if len(params.Statuses) > 0 {
		replicaQuery := DB.Model(&UserAssetReplica{}).
			Select("1").
			Joins("JOIN channel_asset_configs AS configs ON configs.channel_id = user_asset_replicas.channel_id AND configs.enabled = ?", true).
			Where("user_asset_replicas.asset_id = user_assets.id").
			Where("user_asset_replicas.upstream_status IN ?", params.Statuses)
		query = query.Where("EXISTS (?)", replicaQuery)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	orderColumn := "created_time"
	switch params.SortBy {
	case "UpdateTime":
		orderColumn = "updated_time"
	case "GroupId":
		orderColumn = "group_id"
	}
	orderDirection := "DESC"
	if strings.EqualFold(params.SortOrder, "Asc") {
		orderDirection = "ASC"
	}
	var assets []UserAsset
	err := query.Order(orderColumn + " " + orderDirection).
		Limit(int(params.PageSize)).
		Offset(int((params.PageNumber - 1) * params.PageSize)).
		Find(&assets).Error
	return assets, total, err
}

func UpdateUserAsset(asset *UserAsset) error {
	return DB.Model(&UserAsset{}).
		Where("id = ? AND user_id = ?", asset.Id, asset.UserId).
		Select("name", "updated_time").
		Updates(asset).Error
}

func GetUserAssetGroupsForSync() ([]UserAssetGroup, error) {
	var groups []UserAssetGroup
	err := DB.Order("created_time ASC").Find(&groups).Error
	return groups, err
}

func GetGroupAssetsForSync(groupId string) ([]UserAsset, error) {
	var assets []UserAsset
	err := DB.Where("group_id = ?", groupId).Order("created_time ASC").Find(&assets).Error
	return assets, err
}

func CountUserAssetsInGroup(userId int, groupId string) (int64, error) {
	var count int64
	err := DB.Model(&UserAsset{}).Where("user_id = ? AND group_id = ?", userId, groupId).Count(&count).Error
	return count, err
}

func GetUserAssetGroupReplica(groupId string, channelId int) (*UserAssetGroupReplica, error) {
	var replica UserAssetGroupReplica
	if err := DB.Where("group_id = ? AND channel_id = ?", groupId, channelId).First(&replica).Error; err != nil {
		return nil, err
	}
	return &replica, nil
}

func SaveUserAssetGroupReplica(replica *UserAssetGroupReplica) error {
	if replica == nil || replica.GroupId == "" || replica.ChannelId <= 0 {
		return errors.New("invalid asset group replica")
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "group_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"upstream_group_id", "state", "last_error", "updated_time"}),
	}).Create(replica).Error
}

func ListUserAssetGroupReplicas(groupId string) ([]UserAssetGroupReplica, error) {
	var replicas []UserAssetGroupReplica
	err := DB.Where("group_id = ?", groupId).Order("channel_id ASC").Find(&replicas).Error
	return replicas, err
}

func GetUserAssetReplica(assetId string, channelId int) (*UserAssetReplica, error) {
	var replica UserAssetReplica
	if err := DB.Where("asset_id = ? AND channel_id = ?", assetId, channelId).First(&replica).Error; err != nil {
		return nil, err
	}
	return &replica, nil
}

func SaveUserAssetReplica(replica *UserAssetReplica) error {
	if replica == nil || replica.AssetId == "" || replica.ChannelId <= 0 {
		return errors.New("invalid asset replica")
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "asset_id"}, {Name: "channel_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"upstream_asset_id", "state", "upstream_status", "last_error_code", "last_error",
			"last_inference_time", "updated_time",
		}),
	}).Create(replica).Error
}

func ListUserAssetReplicas(assetId string) ([]UserAssetReplica, error) {
	var replicas []UserAssetReplica
	err := DB.Where("asset_id = ?", assetId).Order("channel_id ASC").Find(&replicas).Error
	return replicas, err
}

func GetAssetReplicaMappings(userId int, channelId int, assetIds []string) (map[string]string, error) {
	if len(assetIds) == 0 {
		return map[string]string{}, nil
	}
	type mapping struct {
		AssetId         string
		UpstreamAssetId string
	}
	var rows []mapping
	err := DB.Table("user_asset_replicas AS replicas").
		Select("replicas.asset_id, replicas.upstream_asset_id").
		Joins("JOIN user_assets AS assets ON assets.id = replicas.asset_id").
		Where("assets.user_id = ? AND replicas.channel_id = ? AND replicas.asset_id IN ? AND replicas.upstream_asset_id <> '' AND replicas.state = ?", userId, channelId, assetIds, AssetReplicaStateReady).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	mappings := make(map[string]string, len(rows))
	for _, row := range rows {
		mappings[row.AssetId] = row.UpstreamAssetId
	}
	return mappings, nil
}

func GetAssetReplicaChannelIntersection(userId int, assetIds []string) (map[int]struct{}, error) {
	assetIds = uniqueAssetIds(assetIds)
	if len(assetIds) == 0 {
		return nil, nil
	}
	var ownedCount int64
	if err := DB.Model(&UserAsset{}).Where("user_id = ? AND id IN ?", userId, assetIds).Count(&ownedCount).Error; err != nil {
		return nil, err
	}
	if ownedCount != int64(len(assetIds)) {
		return nil, gorm.ErrRecordNotFound
	}
	var channelIds []int
	err := DB.Table("user_asset_replicas AS replicas").
		Select("replicas.channel_id").
		Joins("JOIN channel_asset_configs AS configs ON configs.channel_id = replicas.channel_id AND configs.enabled = ?", true).
		Where("replicas.asset_id IN ? AND replicas.upstream_asset_id <> '' AND replicas.state = ?", assetIds, AssetReplicaStateReady).
		Group("replicas.channel_id").
		Having("COUNT(DISTINCT replicas.asset_id) = ?", len(assetIds)).
		Pluck("replicas.channel_id", &channelIds).Error
	if err != nil {
		return nil, err
	}
	allowed := make(map[int]struct{}, len(channelIds))
	for _, channelId := range channelIds {
		allowed[channelId] = struct{}{}
	}
	return allowed, nil
}

func uniqueAssetIds(assetIds []string) []string {
	unique := make(map[string]struct{}, len(assetIds))
	result := make([]string, 0, len(assetIds))
	for _, assetId := range assetIds {
		if _, ok := unique[assetId]; ok {
			continue
		}
		unique[assetId] = struct{}{}
		result = append(result, assetId)
	}
	return result
}

func DeleteUserAssetReplica(assetId string, channelId int) error {
	return DB.Where("asset_id = ? AND channel_id = ?", assetId, channelId).Delete(&UserAssetReplica{}).Error
}

func DeleteUserAssetGroupReplica(groupId string, channelId int) error {
	return DB.Where("group_id = ? AND channel_id = ?", groupId, channelId).Delete(&UserAssetGroupReplica{}).Error
}

func DeleteUserAsset(userId int, assetId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("asset_id = ?", assetId).Delete(&UserAssetReplica{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ?", assetId, userId).Delete(&UserAsset{}).Error
	})
}

func DeleteUserAssetGroup(userId int, groupId string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var assetIds []string
		if err := tx.Model(&UserAsset{}).Where("user_id = ? AND group_id = ?", userId, groupId).Pluck("id", &assetIds).Error; err != nil {
			return err
		}
		if len(assetIds) > 0 {
			if err := tx.Where("asset_id IN ?", assetIds).Delete(&UserAssetReplica{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("group_id = ?", groupId).Delete(&UserAssetGroupReplica{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND group_id = ?", userId, groupId).Delete(&UserAsset{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ? AND user_id = ?", groupId, userId).Delete(&UserAssetGroup{}).Error
	})
}

func DeleteChannelAssetLibraryData(tx *gorm.DB, channelIds []int) error {
	if len(channelIds) == 0 {
		return nil
	}
	if err := tx.Where("channel_id IN ?", channelIds).Delete(&UserAssetReplica{}).Error; err != nil {
		return err
	}
	if err := tx.Where("channel_id IN ?", channelIds).Delete(&UserAssetGroupReplica{}).Error; err != nil {
		return err
	}
	return tx.Where("channel_id IN ?", channelIds).Delete(&ChannelAssetConfig{}).Error
}

func DeleteUserAssetLibraryData(tx *gorm.DB, userId int) error {
	var assetIds []string
	if err := tx.Model(&UserAsset{}).Where("user_id = ?", userId).Pluck("id", &assetIds).Error; err != nil {
		return err
	}
	if len(assetIds) > 0 {
		if err := tx.Where("asset_id IN ?", assetIds).Delete(&UserAssetReplica{}).Error; err != nil {
			return err
		}
	}
	var groupIds []string
	if err := tx.Model(&UserAssetGroup{}).Where("user_id = ?", userId).Pluck("id", &groupIds).Error; err != nil {
		return err
	}
	if len(groupIds) > 0 {
		if err := tx.Where("group_id IN ?", groupIds).Delete(&UserAssetGroupReplica{}).Error; err != nil {
			return err
		}
	}
	if err := tx.Where("user_id = ?", userId).Delete(&UserAsset{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ?", userId).Delete(&UserAssetGroup{}).Error
}

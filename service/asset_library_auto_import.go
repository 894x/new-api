package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"gorm.io/gorm"
)

const (
	autoImportAssetGroupName   = "Seedance Auto Imports"
	autoImportAssetProjectName = "seedance-auto-import"
	maxAutoImportAssetCount    = 8
	autoImportReadyTimeout     = 60 * time.Second
	autoImportPollInterval     = time.Second
)

type directAssetReference struct {
	SourceURL string
	AssetType string
}

// PrepareAssetReferences imports direct media URLs into the selected channel's
// asset library, waits for the replicas to become ready, and then rewrites both
// direct and logical references into the upstream format.
func PrepareAssetReferences(ctx context.Context, userId int, channelId int, payload map[string]any) (preparedResult map[string]any, err error) {
	if userId <= 0 || channelId <= 0 {
		return RewriteAssetReferences(userId, channelId, payload)
	}
	config, err := model.GetChannelAssetConfig(channelId)
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !config.Enabled {
		return RewriteAssetReferences(userId, channelId, payload)
	}
	if err != nil {
		return nil, err
	}

	references, err := collectDirectAssetReferences(payload)
	if err != nil {
		return nil, err
	}
	if len(references) == 0 {
		return RewriteAssetReferences(userId, channelId, payload)
	}
	if len(references) > maxAutoImportAssetCount {
		return nil, fmt.Errorf("at most %d direct asset references may be imported per request", maxAutoImportAssetCount)
	}

	ctx, finish := BeginAssetLibraryOperation(ctx, userId, "AutoImport", "")
	defer func() { finish(err) }()

	groups, _, err := model.ListUserAssetGroups(userId, model.AssetGroupListParams{
		ProjectName: autoImportAssetProjectName,
		PageNumber:  1,
		PageSize:    1,
	})
	if err != nil {
		return nil, err
	}
	var group *model.UserAssetGroup
	if len(groups) > 0 {
		group = &groups[0]
	}

	logicalReferences := make(map[string]string, len(references))
	for _, reference := range references {
		assetCtx := ctx
		var assets []model.UserAsset
		if group != nil {
			assets, _, err = model.ListUserAssets(userId, model.AssetListParams{
				GroupIds:   []string{group.Id},
				SourceURL:  reference.SourceURL,
				AssetType:  reference.AssetType,
				PageNumber: 1,
				PageSize:   1,
			})
			if err != nil {
				return nil, err
			}
		}

		var asset *model.UserAsset
		if len(assets) > 0 {
			asset = &assets[0]
		} else {
			assetID := "asset-na-" + common.GetUUID()
			assetCtx = BeginAssetLibraryUpload(ctx, assetID)
			metadata, validateErr := ValidateAssetLibraryMedia(assetCtx, reference.SourceURL, reference.AssetType)
			if validateErr != nil {
				return nil, fmt.Errorf("validate direct %s asset: %w", strings.ToLower(reference.AssetType), validateErr)
			}
			if group == nil {
				group = &model.UserAssetGroup{
					Id:          "group-na-" + common.GetUUID(),
					UserId:      userId,
					Name:        autoImportAssetGroupName,
					Description: "Assets imported automatically from Seedance requests",
					GroupType:   "AIGC",
					ProjectName: autoImportAssetProjectName,
				}
				if err := model.CreateUserAssetGroup(group); err != nil {
					return nil, err
				}
			}
			asset = &model.UserAsset{
				Id:          assetID,
				UserId:      userId,
				GroupId:     group.Id,
				Name:        autoImportedAssetName(reference.SourceURL),
				SourceURL:   reference.SourceURL,
				AssetType:   reference.AssetType,
				MediaFormat: metadata.Format,
				FileSize:    metadata.FileSize,
				Width:       metadata.Width,
				Height:      metadata.Height,
				Duration:    metadata.Duration,
				FPS:         metadata.FPS,
				ProjectName: group.ProjectName,
			}
			if err := CreateAssetLibraryRecord(assetCtx, asset); err != nil {
				return nil, err
			}
		}

		if err := ensureAssetReplicaReady(assetCtx, asset, channelId); err != nil {
			return nil, fmt.Errorf("synchronize direct asset %s: %w", asset.Id, err)
		}
		logicalReferences[directAssetReferenceKey(reference)] = "asset://" + asset.Id
	}

	prepared, _ := rewriteDirectAssetReferences(payload, logicalReferences).(map[string]any)
	return RewriteAssetReferences(userId, channelId, prepared)
}

func NormalizeAssetLibrarySourceURL(value string) (string, error) {
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

func collectDirectAssetReferences(value any) ([]directAssetReference, error) {
	unique := make(map[string]directAssetReference)
	var visit func(any) error
	visit = func(current any) error {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				assetType := assetTypeForReferenceField(key)
				if assetType != "" {
					if sourceURL := directAssetURL(child); sourceURL != "" {
						normalized, err := NormalizeAssetLibrarySourceURL(sourceURL)
						if err != nil {
							return err
						}
						reference := directAssetReference{SourceURL: normalized, AssetType: assetType}
						unique[directAssetReferenceKey(reference)] = reference
					}
				}
				if err := visit(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range typed {
				if err := visit(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := visit(value); err != nil {
		return nil, err
	}
	references := make([]directAssetReference, 0, len(unique))
	for _, reference := range unique {
		references = append(references, reference)
	}
	sort.Slice(references, func(i, j int) bool {
		return directAssetReferenceKey(references[i]) < directAssetReferenceKey(references[j])
	})
	return references, nil
}

func rewriteDirectAssetReferences(value any, mappings map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			assetType := assetTypeForReferenceField(key)
			if assetType != "" {
				if sourceURL := directAssetURL(child); sourceURL != "" {
					reference := directAssetReference{SourceURL: strings.TrimSpace(sourceURL), AssetType: assetType}
					if logicalReference := mappings[directAssetReferenceKey(reference)]; logicalReference != "" {
						switch nested := child.(type) {
						case map[string]any:
							copied := make(map[string]any, len(nested))
							for nestedKey, nestedValue := range nested {
								copied[nestedKey] = rewriteDirectAssetReferences(nestedValue, mappings)
							}
							copied["url"] = logicalReference
							result[key] = copied
						default:
							result[key] = logicalReference
						}
						continue
					}
				}
			}
			result[key] = rewriteDirectAssetReferences(child, mappings)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = rewriteDirectAssetReferences(child, mappings)
		}
		return result
	default:
		return value
	}
}

func ensureAssetReplicaReady(ctx context.Context, asset *model.UserAsset, channelId int) error {
	replica, err := model.GetUserAssetReplica(asset.Id, channelId)
	if err == nil && replica.State == model.AssetReplicaStateReady && replica.UpstreamAssetId != "" {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	report, err := SyncAssetReplicas(ctx, asset, []int{channelId})
	if err != nil {
		return err
	}
	if len(report.Errors) > 0 {
		return errors.New(report.Errors[0].Message)
	}

	waitCtx, cancel := context.WithTimeout(ctx, autoImportReadyTimeout)
	defer cancel()
	for {
		replica, err = model.GetUserAssetReplica(asset.Id, channelId)
		if err != nil {
			return err
		}
		switch replica.State {
		case model.AssetReplicaStateReady:
			if replica.UpstreamAssetId == "" {
				return errors.New("asset replica is ready without an upstream asset id")
			}
			return nil
		case model.AssetReplicaStateFailed:
			if replica.LastError != "" {
				return errors.New(replica.LastError)
			}
			return errors.New("asset replica synchronization failed")
		}

		lock := acquireAssetLibraryChannel(ctx, channelId)
		config, configErr := model.GetChannelAssetConfig(channelId)
		if configErr == nil && !config.Enabled {
			configErr = errors.New("asset library is not enabled for channel")
		}
		if configErr == nil {
			currentReplica, replicaErr := model.GetUserAssetReplica(asset.Id, channelId)
			if replicaErr != nil {
				configErr = replicaErr
			} else {
				_, configErr = refreshAssetReplicaToChannelLocked(waitCtx, config, currentReplica)
			}
		}
		lock.Unlock()
		if configErr != nil {
			return configErr
		}

		replica, err = model.GetUserAssetReplica(asset.Id, channelId)
		if err != nil {
			return err
		}
		if replica.State == model.AssetReplicaStateReady {
			return nil
		}
		if replica.State == model.AssetReplicaStateFailed {
			return errors.New("asset replica synchronization failed")
		}

		timer := time.NewTimer(autoImportPollInterval)
		select {
		case <-waitCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("asset replica did not become ready: %w", waitCtx.Err())
		case <-timer.C:
		}
	}
}

func assetTypeForReferenceField(field string) string {
	switch field {
	case "image_url":
		return "Image"
	case "video_url":
		return "Video"
	case "audio_url":
		return "Audio"
	default:
		return ""
	}
}

func directAssetURL(value any) string {
	var candidate string
	switch typed := value.(type) {
	case string:
		candidate = typed
	case map[string]any:
		candidate, _ = typed["url"].(string)
	}
	candidate = strings.TrimSpace(candidate)
	if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
		return candidate
	}
	return ""
}

func directAssetReferenceKey(reference directAssetReference) string {
	return reference.AssetType + "\x00" + reference.SourceURL
}

func autoImportedAssetName(sourceURL string) string {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return "Imported reference"
	}
	name := path.Base(parsed.Path)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		return "Imported reference"
	}
	if utf8.RuneCountInString(name) <= 64 {
		return name
	}
	return string([]rune(name)[:64])
}

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ManagedAssetReferences is a request-scoped snapshot. Retries sign its owned
// assets again rather than re-fetching mutable external URLs.
type ManagedAssetReferences map[string]*model.UserAsset

func StoreVideoAssetReferences(ctx context.Context, userID int, payload map[string]any, snapshot ManagedAssetReferences) (map[string]any, error) {
	resolved, err := walkVideoAssetReferences(payload, "", func(source, assetType string) (string, error) {
		key := assetType + "\x00" + source
		asset := snapshot[key]
		if asset == nil {
			if len(snapshot) >= 32 {
				return "", errors.New("at most 32 reference assets are supported")
			}
			var err error
			asset, err = importVideoReference(ctx, userID, source, assetType)
			if err != nil {
				return "", err
			}
			snapshot[key] = asset
		}
		return AssetContentURL(ctx, asset)
	})
	if err != nil {
		return nil, err
	}
	return resolved.(map[string]any), nil
}

func importVideoReference(ctx context.Context, userID int, source, assetType string) (*model.UserAsset, error) {
	if strings.HasPrefix(source, "asset://") {
		id, valid := parseLocalAssetReference(source)
		if !valid {
			return nil, errors.New("use an account asset ID")
		}
		asset, err := model.GetUserAsset(userID, id)
		if err != nil {
			return nil, errors.New("asset does not exist or does not belong to the current account")
		}
		if asset.AssetType != assetType {
			return nil, errors.New("asset type does not match reference field")
		}
		if err := EnsureAssetContentStored(ctx, asset); err != nil {
			return nil, err
		}
		return asset, nil
	}
	asset, err := FindManagedAssetURL(userID, source)
	if err == nil {
		if asset.AssetType != assetType {
			return nil, errors.New("asset type does not match reference field")
		}
		return asset, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	asset = &model.UserAsset{Id: "asset-na-" + common.GetUUID(), UserId: userID, SourceURL: source, AssetType: assetType, Name: "Imported reference", ProjectName: "platform-auto-import"}
	if err := ImportAssetContent(ctx, asset); err != nil {
		return nil, err
	}
	return createAutomaticStoredAsset(asset)
}

func ImportVideoFileReference(ctx context.Context, userID int, assetType string, file io.Reader) (*model.UserAsset, error) {
	asset := &model.UserAsset{Id: "asset-na-" + common.GetUUID(), UserId: userID, AssetType: assetType, Name: "Imported reference", ProjectName: "platform-auto-import"}
	if err := ImportAssetFile(ctx, asset, file); err != nil {
		return nil, err
	}
	return createAutomaticStoredAsset(asset)
}

func createAutomaticStoredAsset(asset *model.UserAsset) (*model.UserAsset, error) {
	userID := asset.UserId
	var existing model.UserAsset
	err := model.DB.Where("user_id = ? AND stored_object_id = ?", userID, asset.StoredObjectId).Order("created_time ASC").First(&existing).Error
	if err == nil {
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	groupHash := sha256.Sum256([]byte(asset.StoredObjectId + "/" + "reference"))
	asset.Id = "asset-na-" + hex.EncodeToString(groupHash[:16])
	// An account has one automatic-import group, independent of provider.
	ownerHash := sha256.Sum256([]byte(strings.Join([]string{"platform-auto-import", common.Interface2String(userID)}, ":")))
	asset.GroupId = "group-na-" + hex.EncodeToString(ownerHash[:16])
	group := &model.UserAssetGroup{Id: asset.GroupId, UserId: userID, Name: "Imported references", GroupType: "AIGC", ProjectName: asset.ProjectName}
	if err := model.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(group).Error; err != nil {
		return nil, err
	}
	if strings.HasPrefix(asset.SourceURL, "data:") {
		asset.SourceURL = ""
	}
	if err := model.CreateUserAsset(asset); err != nil {
		if existing, lookupErr := model.GetUserAsset(userID, asset.Id); lookupErr == nil {
			return existing, nil
		}
		return nil, err
	}
	return model.GetUserAsset(userID, asset.Id)
}

// Only media fields are rewritten. Prompts, callback URLs and arbitrary string
// values are never treated as downloadable assets.
func walkVideoAssetReferences(value any, mediaType string, resolve func(string, string) (string, error)) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		itemType, _ := typed["type"].(string)
		for key, child := range typed {
			if key == "bytesBase64Encoded" {
				if encoded, ok := child.(string); ok && encoded != "" {
					kind := mediaType
					if kind == "" {
						kind = "Image"
					}
					if _, err := resolve("data:application/octet-stream;base64,"+encoded, kind); err != nil {
						return nil, err
					}
					result[key] = child
					continue
				}
			}
			childType := videoReferenceMediaType(key)
			if key == "url" || key == "uri" {
				childType = mediaType
				if childType == "" {
					childType = videoReferenceMediaType(itemType)
				}
			}
			if key == "metadata" {
				if encoded, ok := child.(string); ok {
					var metadata map[string]any
					if err := common.UnmarshalJsonStr(encoded, &metadata); err == nil {
						child = metadata
					}
				}
			}
			next, err := walkVideoAssetReferences(child, childType, resolve)
			if err != nil {
				return nil, err
			}
			result[key] = next
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for i, child := range typed {
			next, err := walkVideoAssetReferences(child, mediaType, resolve)
			if err != nil {
				return nil, err
			}
			result[i] = next
		}
		return result, nil
	case string:
		trimmed := strings.TrimSpace(typed)
		if mediaType != "" && (strings.HasPrefix(trimmed, "https://") || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "asset://") || strings.HasPrefix(trimmed, "data:")) {
			return resolve(trimmed, mediaType)
		}
		if mediaType != "" && trimmed != "" {
			if strings.Contains(trimmed, ":") {
				return nil, errors.New("reference media must use HTTP, HTTPS, base64, or an account asset URI")
			}
			return resolve("data:application/octet-stream;base64,"+trimmed, mediaType)
		}
	}
	return value, nil
}

func videoReferenceMediaType(field string) string {
	switch field {
	case "image", "images", "image_url", "image_urls", "img_url", "image_tail", "first_frame", "last_frame", "first_frame_image", "last_frame_image", "first_frame_url", "last_frame_url", "start_image_url", "end_image_url", "input_reference", "reference_image", "reference_images":
		return "Image"
	case "video", "videos", "video_url", "video_urls", "reference_video", "reference_videos":
		return "Video"
	case "audio", "audio_url", "audio_urls", "reference_audio", "driving_audio":
		return "Audio"
	default:
		return ""
	}
}

func prepareStoredLibraryReferences(ctx context.Context, userID, channelID int, payload map[string]any) (map[string]any, error) {
	config, err := model.GetChannelAssetConfig(channelID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	useReplicas := err == nil && config.Enabled
	result, err := walkVideoAssetReferences(payload, "", func(source, assetType string) (string, error) {
		asset, err := importVideoReference(ctx, userID, source, assetType)
		if err != nil {
			return "", err
		}
		if !useReplicas {
			return AssetContentURL(ctx, asset)
		}
		metadata := AssetMediaMetadata{Format: asset.MediaFormat, FileSize: asset.FileSize, Width: asset.Width, Height: asset.Height, Duration: asset.Duration, FPS: asset.FPS}
		if err := validateAssetLibraryMediaMetadata(asset.AssetType, metadata); err != nil {
			return "", err
		}
		if err := ensureAssetReplicaReady(ctx, asset, channelID); err != nil {
			return "", err
		}
		replica, err := model.GetUserAssetReplica(asset.Id, channelID)
		if err != nil {
			return "", err
		}
		backend, err := assetLibraryBackendForChannel(channelID)
		if err != nil {
			return "", err
		}
		return backend.FormatAssetReference(replica.UpstreamAssetId), nil
	})
	if err != nil {
		return nil, err
	}
	return result.(map[string]any), nil
}

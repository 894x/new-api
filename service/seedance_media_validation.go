package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

type seedanceMediaCacheKey struct{}

type seedanceMediaCache struct {
	userID              int
	maxDirectReferences int
	model               string
	metadata            map[directAssetReference]AssetMediaMetadata
}

// ValidateSeedanceMedia checks reference media before billing or upstream writes.
// The returned context carries verified metadata for subsequent asset import.
func ValidateSeedanceMedia(ctx context.Context, userID int, upstreamModel string, payload map[string]any) (context.Context, error) {
	modelName := strings.ReplaceAll(strings.ToLower(upstreamModel), "_", "-")
	modelName = strings.ReplaceAll(modelName, ".", "-")
	maxDuration, maxCount := 15.0, 3
	switch {
	case modelName == "doubao-seedance-2-5" || strings.HasPrefix(modelName, "doubao-seedance-2-5-"):
		maxDuration, maxCount = 30, 10
	case modelName == "doubao-seedance-2-0" || strings.HasPrefix(modelName, "doubao-seedance-2-0-"):
	default:
		return ctx, nil
	}
	content, ok := payload["content"].([]any)
	if !ok {
		return ctx, fmt.Errorf("content must be an array")
	}
	references := make([]directAssetReference, 0)
	indices := make([]int, 0)
	counts := map[string]int{}
	firstFrames, lastFrames := 0, 0
	maxImages := 9
	if maxCount == 10 {
		maxImages = 30
	}
	for index, value := range content {
		item, ok := value.(map[string]any)
		if !ok {
			return ctx, fmt.Errorf("content[%d] must be an object", index)
		}
		field, _ := item["type"].(string)
		assetType := assetTypeForReferenceField(field)
		if assetType == "" {
			continue
		}
		media, ok := item[field].(map[string]any)
		if !ok {
			return ctx, fmt.Errorf("content[%d].%s must contain a url", index, field)
		}
		source, ok := media["url"].(string)
		if !ok || strings.TrimSpace(source) == "" {
			return ctx, fmt.Errorf("content[%d].%s.url must be a non-empty string", index, field)
		}
		role, _ := item["role"].(string)
		if role == "first_frame" || role == "last_frame" {
			if assetType != "Image" {
				return ctx, fmt.Errorf("content[%d].role: %s requires an image", index, role)
			}
			if role == "first_frame" {
				firstFrames++
			} else {
				lastFrames++
			}
		}
		counts[assetType]++
		if assetType == "Image" {
			continue
		}
		if assetType == "Video" && strings.HasPrefix(strings.TrimSpace(source), "data:") {
			return ctx, fmt.Errorf("content[%d].video_url: video requires an HTTP URL or account asset ID", index)
		}
		references = append(references, directAssetReference{AssetType: assetType, SourceURL: strings.TrimSpace(source)})
		indices = append(indices, index)
	}
	for _, assetType := range []string{"Audio", "Video", "Image"} {
		limit := maxCount
		if assetType == "Image" {
			limit = maxImages
		}
		if counts[assetType] > limit {
			return ctx, fmt.Errorf("content: reference %s count %d exceeds the limit of %d for %s", strings.ToLower(assetType), counts[assetType], limit, upstreamModel)
		}
	}
	if firstFrames > 1 || lastFrames > 1 || lastFrames > firstFrames {
		return ctx, fmt.Errorf("content.role: use one first_frame image, optionally followed by one last_frame image")
	}
	if firstFrames > 0 {
		if counts["Image"] != firstFrames+lastFrames || counts["Video"] > 0 || counts["Audio"] > 0 {
			return ctx, fmt.Errorf("content.role: first/last frame generation cannot be mixed with reference media")
		}
		if maxCount == 10 {
			if ratio, exists := payload["ratio"]; exists && ratio != "adaptive" {
				return ctx, fmt.Errorf("ratio must be adaptive for Seedance 2.5 first/last frame generation")
			}
		}
	}
	if maxCount == 3 && counts["Audio"] > 0 && counts["Image"] == 0 && counts["Video"] == 0 {
		return ctx, fmt.Errorf("content: Seedance 2.0 reference audio requires an image or video")
	}
	cache := &seedanceMediaCache{userID: userID, metadata: make(map[directAssetReference]AssetMediaMetadata)}
	if previous, ok := ctx.Value(seedanceMediaCacheKey{}).(*seedanceMediaCache); ok && previous.userID == userID {
		for reference, metadata := range previous.metadata {
			cache.metadata[reference] = metadata
		}
	}
	ctx = context.WithValue(ctx, seedanceMediaCacheKey{}, cache)
	totalDurations := map[string]float64{}
	for index, reference := range references {
		kind := strings.ToLower(reference.AssetType)
		metadata, err := seedanceReferenceMetadata(ctx, userID, reference)
		if err != nil {
			return ctx, fmt.Errorf("content[%d].%s_url: %w", indices[index], kind, err)
		}
		if !isFiniteAssetLibraryNumber(metadata.Duration) || metadata.Duration < 2 || metadata.Duration > maxDuration {
			return ctx, fmt.Errorf("content[%d].%s_url: %s duration %.3f seconds must be between 2 and %g seconds for %s", indices[index], kind, kind, metadata.Duration, maxDuration, upstreamModel)
		}
		totalDurations[reference.AssetType] += metadata.Duration
		totalDuration := totalDurations[reference.AssetType]
		if totalDuration > maxDuration {
			return ctx, fmt.Errorf("content[%d].%s_url: total reference %s duration %.3f seconds exceeds %g seconds for %s", indices[index], kind, kind, totalDuration, maxDuration, upstreamModel)
		}
	}
	cache.maxDirectReferences = maxImages + 2*maxCount
	cache.model = upstreamModel
	return ctx, nil
}

func seedanceReferenceMetadata(ctx context.Context, userID int, reference directAssetReference) (AssetMediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return AssetMediaMetadata{}, err
	}
	cache, _ := ctx.Value(seedanceMediaCacheKey{}).(*seedanceMediaCache)
	if cache != nil && cache.userID == userID {
		if metadata, ok := cache.metadata[reference]; ok {
			return metadata, nil
		}
	}
	source := reference.SourceURL
	if assetID, ok := parseLocalAssetReference(source); ok {
		asset, err := model.GetUserAsset(userID, assetID)
		if err != nil {
			return AssetMediaMetadata{}, fmt.Errorf("asset is unavailable for this account")
		}
		if asset.AssetType != reference.AssetType {
			return AssetMediaMetadata{}, fmt.Errorf("asset type must be %s", strings.ToLower(reference.AssetType))
		}
		metadata := AssetMediaMetadata{Format: asset.MediaFormat, FileSize: asset.FileSize, Width: asset.Width, Height: asset.Height, Duration: asset.Duration, FPS: asset.FPS}
		complete := metadata.Format != "" && metadata.FileSize > 0 && isFiniteAssetLibraryNumber(metadata.Duration) && metadata.Duration > 0
		if reference.AssetType == "Video" {
			complete = complete && metadata.Width > 0 && metadata.Height > 0 && isFiniteAssetLibraryNumber(metadata.FPS) && metadata.FPS > 0
		}
		if complete {
			if err := validateSeedanceReferenceMetadata(reference.AssetType, metadata); err != nil {
				return AssetMediaMetadata{}, err
			}
			if cache != nil {
				cache.metadata[reference] = metadata
			}
			return metadata, nil
		}
		// The source URL may have changed since a replica was created, so its
		// current bytes cannot establish the duration of that existing replica.
		return AssetMediaMetadata{}, fmt.Errorf("asset metadata is incomplete; re-import the asset before using it as a reference")
	} else if hasAssetLibraryURIScheme(source) {
		return AssetMediaMetadata{}, fmt.Errorf("invalid asset URI; use an account asset ID")
	}
	var metadata AssetMediaMetadata
	var err error
	if strings.HasPrefix(source, "data:") {
		header, encoded, found := strings.Cut(source, ",")
		if !found || !strings.HasSuffix(header, ";base64") {
			return AssetMediaMetadata{}, fmt.Errorf("media must use a base64 data URI")
		}
		limit, _, sizeErr := assetLibraryMediaSizeLimit(reference.AssetType)
		if sizeErr != nil {
			return AssetMediaMetadata{}, sizeErr
		}
		if int64(len(encoded)) > (limit+2)/3*4 {
			return AssetMediaMetadata{}, assetLibraryMediaSizeError(reference.AssetType)
		}
		response := &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: io.NopCloser(base64.NewDecoder(base64.StdEncoding, strings.NewReader(encoded)))}
		metadata, err = inspectDownloadedAssetLibraryMedia(ctx, reference.AssetType, response, nil, validateSeedanceReferenceMetadata)
	} else {
		normalized, normalizeErr := NormalizeAssetLibrarySourceURL(source)
		if normalizeErr != nil {
			return AssetMediaMetadata{}, normalizeErr
		}
		metadata, err = validateRemoteAssetMedia(ctx, normalized, reference.AssetType, validateSeedanceReferenceMetadata)
	}
	if err != nil {
		return AssetMediaMetadata{}, err
	}
	if cache != nil {
		cache.metadata[reference] = metadata
	}
	return metadata, nil
}

// Generation accepts a larger video pixel range than asset-library admission.
// Keep those two contracts separate while sharing protected download and inspection.
func validateSeedanceReferenceMetadata(assetType string, metadata AssetMediaMetadata) error {
	if assetType != "Video" {
		return validateAssetLibraryMediaMetadata(assetType, metadata)
	}
	if metadata.Format != "mp4" && metadata.Format != "mov" {
		return fmt.Errorf("video format must be mp4 or mov")
	}
	if metadata.FileSize <= 0 || metadata.FileSize > assetLibraryVideoMaxBytes {
		return assetLibraryMediaSizeError(assetType)
	}
	if metadata.Width < 300 || metadata.Width > 6000 || metadata.Height < 300 || metadata.Height > 6000 {
		return fmt.Errorf("video width and height must each be between 300 px and 6000 px")
	}
	ratio := float64(metadata.Width) / float64(metadata.Height)
	if ratio < 0.4 || ratio > 2.5 {
		return fmt.Errorf("video aspect ratio must be between 0.4 and 2.5")
	}
	pixels := int64(metadata.Width) * int64(metadata.Height)
	if pixels < 407696 || pixels > 8295044 {
		return fmt.Errorf("video pixel count must be between 407696 and 8295044")
	}
	if !isFiniteAssetLibraryNumber(metadata.FPS) || metadata.FPS < 24 || metadata.FPS > 60 {
		return fmt.Errorf("video frame rate must be between 24 and 60 FPS")
	}
	return nil
}

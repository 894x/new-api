package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
)

type managedVideoRequest struct {
	original   map[string]any
	references service.ManagedAssetReferences
}

func ManagedVideoTaskReferences(c *gin.Context) *model.TaskAssetReferences {
	value, exists := c.Get("managed_video_request")
	if !exists {
		return nil
	}
	prepared := value.(*managedVideoRequest)
	items := make(map[string]string)
	for _, asset := range prepared.references {
		items[asset.Id] = asset.StoredObjectId
	}
	if len(items) == 0 {
		return nil
	}
	result := &model.TaskAssetReferences{}
	for assetID, objectID := range items {
		result.Items = append(result.Items, model.TaskAssetReference{AssetID: assetID, StoredObjectID: objectID})
	}
	sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].AssetID < result.Items[j].AssetID })
	return result
}

func prepareManagedVideoRequest(c *gin.Context, userID int) error {
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}
	if c.Request == nil {
		return errors.New("missing video request")
	}
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		return prepareManagedVideoMultipart(c, userID)
	}
	if !strings.Contains(c.ContentType(), "json") {
		return errors.New("unsupported video request content type")
	}
	var prepared *managedVideoRequest
	if value, exists := c.Get("managed_video_request"); exists {
		prepared = value.(*managedVideoRequest)
	}
	if prepared == nil {
		var payload map[string]any
		if err := common.UnmarshalBodyReusable(c, &payload); err != nil {
			return err
		}
		prepared = &managedVideoRequest{original: payload, references: service.ManagedAssetReferences{}}
		c.Set("managed_video_request", prepared)
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	payload, err := service.StoreVideoAssetReferences(ctx, userID, prepared.original, prepared.references)
	if err != nil {
		return err
	}
	if err := service.RejectAssetReferences(payload); err != nil {
		return err
	}
	body, err := common.Marshal(payload)
	if err != nil {
		return err
	}
	common.CleanupBodyStorage(c)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	c.Request.ContentLength = int64(len(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.KeyRequestBody, body)
	ids := make(map[string]struct{})
	for _, asset := range prepared.references {
		ids[asset.Id] = struct{}{}
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	if len(ordered) > 0 {
		c.Header("X-New-Api-Asset-Ids", strings.Join(ordered, ","))
	}
	c.Set("managed_asset_ids", ordered)
	return nil
}

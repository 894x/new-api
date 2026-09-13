package relay

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func prepareManagedVideoMultipart(c *gin.Context, userID int) error {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Minute)
	defer cancel()
	if _, done := c.Get("managed_video_multipart"); done {
		return nil
	}
	form, err := common.ParseMultipartFormReusable(c)
	if err != nil {
		return err
	}
	defer form.RemoveAll()
	prepared := &managedVideoRequest{references: service.ManagedAssetReferences{}}
	payload := make(map[string]any, len(form.Value))
	for key, values := range form.Value {
		if len(values) == 1 {
			payload[key] = values[0]
		} else {
			list := make([]any, len(values))
			for i, value := range values {
				list[i] = value
			}
			payload[key] = list
		}
	}
	resolved, err := service.StoreVideoAssetReferences(ctx, userID, payload, prepared.references)
	if err != nil {
		return err
	}
	if err := service.RejectAssetReferences(resolved); err != nil {
		return err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range resolved {
		values, ok := value.([]any)
		if !ok {
			values = []any{value}
		}
		for _, item := range values {
			text, ok := item.(string)
			if !ok {
				data, err := common.Marshal(item)
				if err != nil {
					return err
				}
				text = string(data)
			}
			if err := writer.WriteField(key, text); err != nil {
				return err
			}
		}
	}
	for field, files := range form.File {
		assetType := "Image"
		if strings.Contains(field, "video") {
			assetType = "Video"
		} else if strings.Contains(field, "audio") {
			assetType = "Audio"
		}
		for _, header := range files {
			if len(prepared.references) >= 32 {
				return errors.New("at most 32 reference assets are supported")
			}
			file, err := header.Open()
			if err != nil {
				return err
			}
			asset, err := service.ImportVideoFileReference(ctx, userID, assetType, file)
			file.Close()
			if err != nil {
				return err
			}
			prepared.references[asset.Id] = asset
			part, err := writer.CreatePart(header.Header)
			if err != nil {
				return err
			}
			file, err = header.Open()
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(part, file)
			file.Close()
			if copyErr != nil {
				return copyErr
			}
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	common.CleanupBodyStorage(c)
	c.Request.Body = io.NopCloser(bytes.NewReader(body.Bytes()))
	c.Request.ContentLength = int64(body.Len())
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	c.Set("_original_multipart_ct", writer.FormDataContentType())
	c.Set(common.KeyRequestBody, body.Bytes())
	c.Set("managed_video_request", prepared)
	c.Set("managed_video_multipart", true)
	ids := make([]string, 0, len(prepared.references))
	for _, asset := range prepared.references {
		ids = append(ids, asset.Id)
	}
	sort.Strings(ids)
	c.Set("managed_asset_ids", ids)
	if len(ids) > 0 {
		c.Header("X-New-Api-Asset-Ids", strings.Join(ids, ","))
	}
	return nil
}

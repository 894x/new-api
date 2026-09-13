package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	cos "github.com/tencentyun/cos-go-sdk-v5"
	"gorm.io/gorm"
)

type assetObjectStore interface {
	Put(context.Context, *model.AssetStoredObject, io.Reader) error
	Sign(context.Context, *model.AssetStoredObject) (string, error)
	Get(context.Context, *model.AssetStoredObject, string) (*http.Response, error)
	Delete(context.Context, *model.AssetStoredObject) error
}

type cosAssetObjectStore struct {
	config    system_setting.AssetStorageConfig
	transport http.RoundTripper
}

var assetObjectStoreFactory = func(config system_setting.AssetStorageConfig) assetObjectStore {
	return &cosAssetObjectStore{config: config}
}

func (s *cosAssetObjectStore) client(object *model.AssetStoredObject) *cos.Client {
	location := s.config
	location.Bucket, location.Region = object.Bucket, object.Region
	return cos.NewClient(&cos.BaseURL{BucketURL: location.Endpoint()}, &http.Client{
		Timeout:       10 * time.Minute,
		Transport:     &cos.AuthorizationTransport{SecretID: s.config.SecretID, SecretKey: s.config.SecretKey, SessionToken: s.config.SessionToken, Transport: s.transport},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	})
}

func (s *cosAssetObjectStore) Put(ctx context.Context, object *model.AssetStoredObject, body io.Reader) error {
	client := s.client(object)
	// A retry may follow a successful PUT whose response or DB commit was lost.
	response, err := client.Object.Head(ctx, object.ObjectKey, nil)
	if err == nil {
		response.Body.Close()
		if response.ContentLength != object.FileSize || response.Header.Get("x-cos-meta-sha256") != object.SHA256 {
			return errors.New("stored object content does not match import")
		}
		return nil
	}
	if response != nil {
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			return errors.New("COS object verification failed")
		}
	} else {
		return errors.New("COS object verification failed")
	}
	headers := &http.Header{}
	headers.Set("x-cos-meta-sha256", object.SHA256)
	response, err = client.Object.Put(ctx, object.ObjectKey, body, &cos.ObjectPutOptions{ObjectPutHeaderOptions: &cos.ObjectPutHeaderOptions{ContentType: object.ContentType, ContentLength: object.FileSize, XCosMetaXXX: headers}})
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return errors.New("COS upload failed")
	}
	response, err = client.Object.Head(ctx, object.ObjectKey, nil)
	if response != nil {
		defer response.Body.Close()
	}
	if err != nil || response.ContentLength != object.FileSize || response.Header.Get("x-cos-meta-sha256") != object.SHA256 {
		return errors.New("COS uploaded object verification failed")
	}
	return nil
}

func (s *cosAssetObjectStore) Sign(ctx context.Context, object *model.AssetStoredObject) (string, error) {
	query := &url.Values{}
	if s.config.SessionToken != "" {
		query.Set("x-cos-security-token", s.config.SessionToken)
	}
	result, err := s.client(object).Object.GetPresignedURL(ctx, http.MethodGet, object.ObjectKey, s.config.SecretID, s.config.SecretKey, s.config.URLLifetime, &cos.PresignedURLOptions{Query: query})
	if err != nil {
		return "", errors.New("COS download authorization failed")
	}
	return result.String(), nil
}

func (s *cosAssetObjectStore) Get(ctx context.Context, object *model.AssetStoredObject, byteRange string) (*http.Response, error) {
	response, err := s.client(object).Object.Get(ctx, object.ObjectKey, &cos.ObjectGetOptions{Range: byteRange})
	if err != nil {
		if response != nil {
			response.Body.Close()
			if response.StatusCode == http.StatusRequestedRangeNotSatisfiable {
				// The SDK consumes error bodies. Preserve the range response without
				// forwarding that consumed stream or its original content length.
				response.Body = http.NoBody
				response.ContentLength = 0
				response.Header.Set("Content-Length", "0")
				return response.Response, nil
			}
		}
		return nil, errors.New("COS download failed")
	}
	return response.Response, nil
}

func (s *cosAssetObjectStore) Delete(ctx context.Context, object *model.AssetStoredObject) error {
	response, err := s.client(object).Object.Delete(ctx, object.ObjectKey)
	if response != nil {
		response.Body.Close()
	}
	if err != nil && (response == nil || response.StatusCode != http.StatusNotFound) {
		return errors.New("COS object deletion failed")
	}
	return nil
}

// ImportAssetContent retains the exact bytes inspected, not a second download
// of a potentially mutable URL. The caller persists the logical asset only once
// the original is durable. Upload failures are retained in the object ledger.
func ImportAssetContent(ctx context.Context, asset *model.UserAsset) error {
	return importAssetContent(ctx, asset, nil)
}

func ImportAssetFile(ctx context.Context, asset *model.UserAsset, file io.Reader) error {
	return importAssetContent(ctx, asset, file)
}

func importAssetContent(ctx context.Context, asset *model.UserAsset, uploaded io.Reader) (err error) {
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil {
		return err
	}
	if !config.Enabled {
		return errors.New("platform asset storage is not enabled")
	}
	if asset.UserId <= 0 {
		return errors.New("asset owner is required")
	}
	if asset.StoredObjectId != "" {
		object, err := model.GetAssetStoredObject(asset.UserId, asset.StoredObjectId)
		if err != nil {
			return err
		}
		if object.State != "ready" {
			return errors.New("asset original is not ready")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	ctx, finish := BeginAssetLibraryOperation(ctx, asset.UserId, "StoreOriginal", asset.Id)
	defer func() { finish(err) }()
	maxBytes, strict, err := assetLibraryMediaSizeLimit(asset.AssetType)
	if err != nil {
		return err
	}
	stage := startAssetLibraryStage(ctx, "source_download", "StoreOriginal", asset.Id, 0)
	defer func() {
		if stage.stage.Outcome == "running" {
			stage.finish(err)
		}
	}()
	var source io.ReadCloser
	if uploaded != nil {
		source = io.NopCloser(uploaded)
	} else if strings.HasPrefix(asset.SourceURL, "data:") {
		header, data, ok := strings.Cut(asset.SourceURL, ",")
		if !ok || !strings.HasSuffix(header, ";base64") {
			return errors.New("invalid base64 asset")
		}
		source = io.NopCloser(base64.NewDecoder(base64.StdEncoding, strings.NewReader(data)))
	} else {
		if err := ValidateSSRFProtectedFetchURL(asset.SourceURL); err != nil {
			return errors.New("asset source URL is not allowed")
		}
		response, err := DoDownloadRequestWithContext(ctx, asset.SourceURL, "asset_original")
		if err != nil {
			return errors.New("asset source download failed")
		}
		if response.StatusCode != http.StatusOK || response.ContentLength > maxBytes {
			response.Body.Close()
			return errors.New("asset source is unavailable or exceeds the size limit")
		}
		source = response.Body
	}
	defer source.Close()
	file, err := os.CreateTemp("", "new-api-original-*")
	if err != nil {
		return errors.New("could not stage asset original")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(source, maxBytes+1))
	if err != nil {
		return errors.New("asset download could not be completed")
	}
	if size == 0 || assetLibraryMediaSizeExceeded(size, maxBytes, strict) {
		return assetLibraryMediaSizeError(asset.AssetType)
	}
	stage.stage.Bytes = size
	stage.finish(nil)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	inspection := startAssetLibraryStage(ctx, "media_inspection", "StoreOriginal", asset.Id, 0)
	metadata, err := inspectAssetLibraryMedia(ctx, asset.AssetType, file, size)
	inspection.finish(err)
	if err != nil {
		return err
	}
	if metadata.Width < 0 || metadata.Height < 0 || !isFiniteAssetLibraryNumber(metadata.Duration) {
		return errors.New("invalid asset metadata")
	}
	contentType := mime.TypeByExtension("." + metadata.Format)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	sha := hex.EncodeToString(digest.Sum(nil))
	candidate := &model.AssetStoredObject{Id: common.GetUUID(), UserId: asset.UserId, SHA256: sha, AssetType: asset.AssetType, Bucket: config.Bucket, Region: config.Region, ObjectKey: fmt.Sprintf("assets/%d/%s/%s", asset.UserId, strings.ToLower(asset.AssetType), sha), FileSize: size, ContentType: contentType, State: "pending"}
	leaseToken := common.GetUUID()
	var object *model.AssetStoredObject
	for {
		var claimed bool
		object, claimed, err = model.ClaimAssetStoredObject(candidate, leaseToken)
		if err != nil {
			return err
		}
		if object.State == "ready" {
			break
		}
		if claimed {
			if _, err = file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			upload := startAssetLibraryStage(ctx, "storage_upload", "PutObject", asset.Id, 0)
			err = assetObjectStoreFactory(config).Put(ctx, object, file)
			upload.stage.Bytes = size
			upload.finish(err)
			commitErr := model.CompleteAssetStoredObject(object.Id, leaseToken, err)
			if err != nil {
				return err
			}
			if commitErr != nil {
				return commitErr
			}
			break
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	asset.StoredObjectId = object.Id
	asset.MediaFormat, asset.FileSize = metadata.Format, metadata.FileSize
	asset.Width, asset.Height, asset.Duration, asset.FPS = metadata.Width, metadata.Height, metadata.Duration, metadata.FPS
	return nil
}

func EnsureAssetContentStored(ctx context.Context, asset *model.UserAsset) error {
	if asset.StoredObjectId != "" {
		return nil
	}
	if err := ImportAssetContent(ctx, asset); err != nil {
		return err
	}
	return model.AttachAssetStoredObject(asset, asset.StoredObjectId)
}

func AssetContentURL(ctx context.Context, asset *model.UserAsset) (string, error) {
	if asset.StoredObjectId == "" {
		return asset.SourceURL, nil
	}
	object, err := model.GetAssetStoredObject(asset.UserId, asset.StoredObjectId)
	if err != nil {
		return "", err
	}
	if object.State != "ready" {
		return "", errors.New("asset original is not ready")
	}
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil {
		return "", err
	}
	if config.SecretID == "" || config.SecretKey == "" {
		return "", errors.New("COS credentials are unavailable")
	}
	return assetObjectStoreFactory(config).Sign(ctx, object)
}

func ReadAssetContent(ctx context.Context, asset *model.UserAsset, byteRange string) (*http.Response, error) {
	object, err := model.GetAssetStoredObject(asset.UserId, asset.StoredObjectId)
	if err != nil {
		return nil, err
	}
	if object.State != "ready" {
		return nil, errors.New("asset original is not ready")
	}
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil {
		return nil, err
	}
	return assetObjectStoreFactory(config).Get(ctx, object, byteRange)
}

// FindManagedAssetURL recognizes only an actual object owned by the caller.
// Matching a URL prefix is never sufficient to grant asset access.
func FindManagedAssetURL(userID int, source string) (*model.UserAsset, error) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "https" {
		return nil, gorm.ErrRecordNotFound
	}
	parts := strings.Split(parsed.Hostname(), ".")
	if len(parts) != 5 || parts[1] != "cos" || parts[3] != "myqcloud" || parts[4] != "com" {
		return nil, gorm.ErrRecordNotFound
	}
	var object model.AssetStoredObject
	err = model.DB.Where("bucket = ? AND region = ? AND object_key = ?", parts[0], parts[2], strings.TrimPrefix(parsed.Path, "/")).First(&object).Error
	if err != nil {
		return nil, err
	}
	if object.UserId != userID {
		return nil, errors.New("asset does not belong to the current account")
	}
	var asset model.UserAsset
	err = model.DB.Where("user_id = ? AND stored_object_id = ?", userID, object.Id).Order("created_time ASC").First(&asset).Error
	return &asset, err
}

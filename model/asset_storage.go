package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AssetStoredObject is shared only by assets owned by the same account. Its
// location and content identity are immutable, including after config changes.
type AssetStoredObject struct {
	QuotaReserved bool
	Id            string `gorm:"type:varchar(64);primaryKey"`
	UserId        int    `gorm:"not null;uniqueIndex:idx_asset_content,priority:1"`
	SHA256        string `gorm:"type:varchar(64);not null;uniqueIndex:idx_asset_content,priority:2"`
	AssetType     string `gorm:"type:varchar(16);not null;uniqueIndex:idx_asset_content,priority:3"`
	Bucket        string `gorm:"type:varchar(128);not null"`
	Region        string `gorm:"type:varchar(64);not null"`
	ObjectKey     string `gorm:"type:varchar(512);not null"`
	FileSize      int64
	ContentType   string `gorm:"type:varchar(128)"`
	State         string `gorm:"type:varchar(16);not null"`
	LeaseToken    string `gorm:"type:varchar(64)"`
	LeaseUntil    int64  `gorm:"type:bigint"`
	CreatedTime   int64  `gorm:"autoCreateTime"`
	UpdatedTime   int64  `gorm:"autoUpdateTime"`
}

type AssetStorageAccount struct {
	UserId          int    `gorm:"primaryKey;autoIncrement:false"`
	UsedBytes       int64  `gorm:"type:bigint;not null"`
	QuotaOverrideMB *int64 `gorm:"type:bigint"`
}

var ErrAssetQuotaExceeded = errors.New("free asset storage quota exceeded; remove unused assets or ask an administrator to increase the quota")

func reserveAssetObjectQuota(tx *gorm.DB, object *AssetStoredObject) error {
	if object.QuotaReserved {
		return nil
	}
	limit, err := system_setting.AssetQuotaBytes()
	if err != nil {
		return err
	}
	if object.FileSize <= 0 || object.FileSize > system_setting.MaxAssetQuotaMB*system_setting.AssetQuotaMB {
		return ErrAssetQuotaExceeded
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&AssetStorageAccount{UserId: object.UserId}).Error; err != nil {
		return err
	}
	// Resolve the override inside the conditional update so concurrent quota
	// changes and reservations serialize on the same account row.
	result := tx.Model(&AssetStorageAccount{}).Where("user_id = ? AND used_bytes <= COALESCE(quota_override_mb, ?) * ? - ?", object.UserId, limit/system_setting.AssetQuotaMB, system_setting.AssetQuotaMB, object.FileSize).UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", object.FileSize))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAssetQuotaExceeded
	}
	if err := tx.Model(&AssetStoredObject{}).Where("id = ?", object.Id).Update("quota_reserved", true).Error; err != nil {
		return err
	}
	object.QuotaReserved = true
	return nil
}

func releaseAssetObjectQuota(tx *gorm.DB, object *AssetStoredObject) error {
	if !object.QuotaReserved {
		return nil
	}
	result := tx.Model(&AssetStorageAccount{}).Where("user_id = ? AND used_bytes >= ?", object.UserId, object.FileSize).UpdateColumn("used_bytes", gorm.Expr("used_bytes - ?", object.FileSize))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("asset quota accounting is inconsistent")
	}
	return tx.Model(&AssetStoredObject{}).Where("id = ?", object.Id).Update("quota_reserved", false).Error
}

func GetAssetStorageUsedBytes(userID int) (int64, error) {
	account, err := GetAssetStorageAccount(userID)
	return account.UsedBytes, err
}

func GetAssetStorageAccount(userID int) (*AssetStorageAccount, error) {
	account := AssetStorageAccount{UserId: userID}
	err := DB.Where("user_id = ?", userID).First(&account).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &account, nil
	}
	return &account, err
}

func (account *AssetStorageAccount) QuotaBytes() (int64, error) {
	if account.QuotaOverrideMB == nil {
		return system_setting.AssetQuotaBytes()
	}
	if *account.QuotaOverrideMB < 0 || *account.QuotaOverrideMB > system_setting.MaxAssetQuotaMB {
		return 0, errors.New("asset quota must be between 0 and 1000000 MB")
	}
	return *account.QuotaOverrideMB * system_setting.AssetQuotaMB, nil
}

func SetAssetStorageQuotaOverride(userID int, quotaMB *int64) error {
	if userID <= 0 {
		return errors.New("asset owner is required")
	}
	if quotaMB != nil && (*quotaMB < 0 || *quotaMB > system_setting.MaxAssetQuotaMB) {
		return errors.New("asset quota must be between 0 and 1000000 MB")
	}
	return DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"quota_override_mb"}),
	}).Create(&AssetStorageAccount{UserId: userID, QuotaOverrideMB: quotaMB}).Error
}

func GetAssetStoredObject(userID int, id string) (*AssetStoredObject, error) {
	var object AssetStoredObject
	err := DB.Where("id = ? AND user_id = ?", id, userID).First(&object).Error
	return &object, err
}

// ClaimAssetStoredObject coordinates identical uploads across processes. No
// database transaction is held while talking to COS.
func ClaimAssetStoredObject(candidate *AssetStoredObject, leaseToken string) (*AssetStoredObject, bool, error) {
	var object AssetStoredObject
	claimed := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(candidate).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("user_id = ? AND sha256 = ? AND asset_type = ?", candidate.UserId, candidate.SHA256, candidate.AssetType).First(&object).Error; err != nil {
			return err
		}
		if object.State == "deleting" {
			return nil
		}
		if err := reserveAssetObjectQuota(tx, &object); err != nil {
			return err
		}
		if object.State == "ready" {
			return nil
		}
		now := time.Now().Unix()
		result := tx.Model(&AssetStoredObject{}).Where("id = ? AND state <> ? AND lease_until <= ?", object.Id, "ready", now).Updates(map[string]any{"state": "uploading", "lease_token": leaseToken, "lease_until": now + 900})
		claimed = result.RowsAffected == 1
		object.LeaseToken = leaseToken
		return result.Error
	})
	if err != nil {
		return nil, false, err
	}
	return &object, claimed, nil
}

// RecoverAssetStorageReservations releases abandoned uploads and originals
// whose logical creation never committed. COS bytes are retained for recovery.
func RecoverAssetStorageReservations() error {
	var objects []AssetStoredObject
	now := time.Now().Unix()
	references := DB.Model(&UserAsset{}).Select("1").Where("user_assets.stored_object_id = asset_stored_objects.id")
	if err := DB.Where("quota_reserved = ? AND updated_time < ? AND lease_until < ?", true, now-900, now).Where("NOT EXISTS (?)", references).Order("updated_time ASC").Limit(100).Find(&objects).Error; err != nil {
		return err
	}
	for _, candidate := range objects {
		if err := DB.Transaction(func(tx *gorm.DB) error {
			var object AssetStoredObject
			if err := lockForUpdate(tx).Where("id = ?", candidate.Id).First(&object).Error; err != nil {
				return err
			}
			if object.LeaseUntil >= now || object.UpdatedTime >= now-900 {
				return nil
			}
			if object.State == "deleting" {
				return nil
			}
			var references int64
			if err := tx.Model(&UserAsset{}).Where("stored_object_id = ?", object.Id).Count(&references).Error; err != nil {
				return err
			}
			if references > 0 {
				return nil
			}
			if err := releaseAssetObjectQuota(tx, &object); err != nil {
				return err
			}
			if object.State != "ready" {
				return tx.Model(&AssetStoredObject{}).Where("id = ?", object.Id).Updates(map[string]any{"state": "failed", "lease_token": "", "lease_until": 0}).Error
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// ClaimUnusedAssetStoredObjects leaves a seven-day grace period after the last
// library reference is removed, covering the maximum issued URL lifetime.
func ClaimUnusedAssetStoredObjects(leaseToken string) ([]AssetStoredObject, error) {
	now := time.Now().Unix()
	var candidates, claimed []AssetStoredObject
	references := DB.Model(&UserAsset{}).Select("1").Where("user_assets.stored_object_id = asset_stored_objects.id")
	err := DB.Where("state <> ? AND (state = ? OR updated_time < ?) AND lease_until < ?", "deleted", "deleting", now-7*86400, now).Where("NOT EXISTS (?)", references).Order("updated_time ASC").Limit(1).Find(&candidates).Error
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		err = DB.Transaction(func(tx *gorm.DB) error {
			var object AssetStoredObject
			if err := lockForUpdate(tx).Where("id = ?", candidate.Id).First(&object).Error; err != nil {
				return err
			}
			if object.State == "deleted" || object.LeaseUntil >= now || (object.State != "deleting" && object.UpdatedTime >= now-7*86400) {
				return nil
			}
			var count int64
			if err := tx.Model(&UserAsset{}).Where("stored_object_id = ?", object.Id).Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return nil
			}
			if err := tx.Model(&AssetStoredObject{}).Where("id = ?", object.Id).Updates(map[string]any{"state": "deleting", "lease_token": leaseToken, "lease_until": now + 900}).Error; err != nil {
				return err
			}
			claimed = append(claimed, object)
			return nil
		})
		if err != nil {
			return claimed, err
		}
	}
	return claimed, nil
}

func CompleteUnusedAssetStoredObject(id, leaseToken string, deleted bool) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var object AssetStoredObject
		if err := lockForUpdate(tx).Where("id = ? AND state = ? AND lease_token = ?", id, "deleting", leaseToken).First(&object).Error; err != nil {
			return err
		}
		if !deleted {
			// Keep imports waiting until deletion can be retried. Never publish a
			// ready object after an ambiguous DELETE response.
			return tx.Model(&AssetStoredObject{}).Where("id = ?", id).UpdateColumns(map[string]any{"lease_until": time.Now().Unix() + 900, "updated_time": time.Now().Unix() - 7*86400 - 1}).Error
		}
		if err := releaseAssetObjectQuota(tx, &object); err != nil {
			return err
		}
		return tx.Model(&AssetStoredObject{}).Where("id = ?", id).Updates(map[string]any{"state": "deleted", "lease_until": 0, "lease_token": ""}).Error
	})
}

func CompleteAssetStoredObject(id, leaseToken string, uploadErr error) error {
	state := "ready"
	if uploadErr != nil {
		state = "failed"
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var object AssetStoredObject
		if err := lockForUpdate(tx).Where("id = ? AND lease_token = ? AND state = ?", id, leaseToken, "uploading").First(&object).Error; err != nil {
			return err
		}
		if uploadErr != nil {
			if err := releaseAssetObjectQuota(tx, &object); err != nil {
				return err
			}
		}
		return tx.Model(&AssetStoredObject{}).Where("id = ?", id).Updates(map[string]any{"state": state, "lease_until": 0, "lease_token": ""}).Error
	})
}

func AttachAssetStoredObject(asset *UserAsset, objectID string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var object AssetStoredObject
		if err := lockForUpdate(tx).Where("id = ? AND user_id = ? AND state = ?", objectID, asset.UserId, "ready").First(&object).Error; err != nil {
			return err
		}
		if err := reserveAssetObjectQuota(tx, &object); err != nil {
			return err
		}
		result := tx.Model(&UserAsset{}).Where("id = ? AND user_id = ? AND (stored_object_id = '' OR stored_object_id IS NULL)", asset.Id, asset.UserId).Updates(map[string]any{"stored_object_id": objectID, "media_format": asset.MediaFormat, "file_size": asset.FileSize, "width": asset.Width, "height": asset.Height, "duration": asset.Duration, "fps": asset.FPS})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			var existing UserAsset
			if err := tx.Where("id = ? AND user_id = ?", asset.Id, asset.UserId).First(&existing).Error; err != nil {
				return err
			}
			if existing.StoredObjectId != objectID {
				return errors.New("asset content changed during import")
			}
		}
		asset.StoredObjectId = objectID
		return nil
	})
}

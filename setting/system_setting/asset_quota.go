package system_setting

import (
	"errors"
	"github.com/QuantumNous/new-api/setting/config"
)

const AssetQuotaMB = int64(1_000_000)
const MaxAssetQuotaMB = int64(1_000_000)

type AssetStorageSetting struct {
	DefaultQuotaMB int64 `json:"default_quota_mb"`
}

var assetStorageSetting = AssetStorageSetting{DefaultQuotaMB: 1024}

func init() { config.GlobalConfig.Register("asset_storage_setting", &assetStorageSetting) }

func GetAssetStorageSetting() *AssetStorageSetting { return &assetStorageSetting }

func AssetQuotaBytes() (int64, error) {
	mb := assetStorageSetting.DefaultQuotaMB
	if mb < 0 || mb > MaxAssetQuotaMB {
		return 0, errors.New("asset quota must be between 0 and 1000000 MB")
	}
	return mb * AssetQuotaMB, nil
}

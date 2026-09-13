package service

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"time"
)

func StartAssetStorageMaintenance() {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := model.RecoverAssetStorageReservations(); err != nil {
				common.SysError("asset storage reservation recovery failed: " + err.Error())
			}
			if err := DeleteUnusedAssetContent(context.Background()); err != nil {
				common.SysError("asset storage cleanup failed: " + err.Error())
			}
		}
	}()
}

func DeleteUnusedAssetContent(ctx context.Context) error {
	config, err := system_setting.LoadAssetStorageConfig()
	if err != nil {
		return err
	}
	if !config.Enabled {
		return nil
	}
	leaseToken := common.GetUUID()
	objects, err := model.ClaimUnusedAssetStoredObjects(leaseToken)
	if err != nil {
		return err
	}
	for i := range objects {
		deleteCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		deleteErr := assetObjectStoreFactory(config).Delete(deleteCtx, &objects[i])
		cancel()
		if err := model.CompleteUnusedAssetStoredObject(objects[i].Id, leaseToken, deleteErr == nil); err != nil {
			return err
		}
		if deleteErr != nil {
			return deleteErr
		}
	}
	return nil
}

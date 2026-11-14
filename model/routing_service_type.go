package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

var (
	ErrInvalidRoutingServiceType    = errors.New("invalid routing service type")
	ErrRoutingServiceTypeInUse      = errors.New("routing service type is assigned to users")
	ErrRoutingServiceTypeNameExists = errors.New("a routing service type with this name already exists")
)

// A service type shares routing configuration, independently of its name and
// scheduling modes. Existing user-specific pools retain precedence.
type RoutingServiceType struct {
	ID       int    `json:"id" gorm:"primaryKey"`
	Name     string `json:"name" gorm:"type:varchar(128);not null;uniqueIndex"`
	Revision string `json:"revision" gorm:"type:varchar(64);not null"`
}

type RoutingServiceTypePool struct {
	ServiceTypeID int    `gorm:"primaryKey;autoIncrement:false"`
	RoutingGroup  string `gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model         string `gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	Revision      string `gorm:"type:varchar(64);not null"`
	ConfigJSON    string `gorm:"type:text;not null"`
}

func (row *RoutingServiceTypePool) Decode() (*ModelRoutingPool, error) {
	var pool ModelRoutingPool
	if err := common.UnmarshalJsonStr(row.ConfigJSON, &pool); err != nil {
		return nil, fmt.Errorf("decode service type pool: %w", err)
	}
	pool.ServiceTypeID, pool.RoutingGroup, pool.Model, pool.Revision = row.ServiceTypeID, row.RoutingGroup, row.Model, row.Revision
	if pool.Tuning != nil {
		encoded, err := common.Marshal(pool.Tuning)
		if err != nil {
			return nil, err
		}
		pool.TuningJSON = string(encoded)
	}
	return &pool, nil
}

// Empty model/group is the account default. More specific bindings take
// precedence: model+group, model, group, account. Type zero explicitly uses
// default pools instead of the account's service type.
type UserRoutingServiceType struct {
	UserID        int    `json:"-" gorm:"primaryKey;autoIncrement:false"`
	Model         string `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	RoutingGroup  string `json:"routing_group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	ServiceTypeID int    `json:"service_type_id"`
	Revision      string `json:"revision" gorm:"type:varchar(64);not null"`
}

func ListRoutingServiceTypes() ([]RoutingServiceType, error) {
	rows := make([]RoutingServiceType, 0)
	err := DB.Order("name, id").Find(&rows).Error
	return rows, err
}

func SaveRoutingServiceType(row *RoutingServiceType, revision string) error {
	if row == nil || row.ID < 0 || row.Name == "" || !utf8.ValidString(row.Name) || utf8.RuneCountInString(row.Name) > 128 || strings.TrimSpace(row.Name) != row.Name || strings.ContainsAny(row.Name, "\x00\r\n") {
		return ErrInvalidRoutingServiceType
	}
	var duplicate int64
	if err := DB.Model(&RoutingServiceType{}).Where("name = ? AND id <> ?", row.Name, row.ID).Count(&duplicate).Error; err != nil {
		return err
	}
	if duplicate > 0 {
		return ErrRoutingServiceTypeNameExists
	}
	if row.ID == 0 {
		if revision != "" {
			return ErrRoutingPoolConflict
		}
		row.Revision = common.GetUUID()
		return DB.Create(row).Error
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current RoutingServiceType
		if err := lockForUpdate(tx).First(&current, row.ID).Error; err != nil {
			return err
		}
		if current.Revision != revision {
			return ErrRoutingPoolConflict
		}
		row.Revision = common.GetUUID()
		return tx.Model(&current).Updates(map[string]any{"name": row.Name, "revision": row.Revision}).Error
	})
}

func DeleteRoutingServiceType(id int, revision string) error {
	if id <= 0 || revision == "" {
		return ErrInvalidRoutingServiceType
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current RoutingServiceType
		if err := lockForUpdate(tx).First(&current, id).Error; err != nil {
			return err
		}
		if current.Revision != revision {
			return ErrRoutingPoolConflict
		}
		var count int64
		if err := tx.Model(&UserRoutingServiceType{}).Where("service_type_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRoutingServiceTypeInUse
		}
		if err := tx.Where("service_type_id = ?", id).Delete(&RoutingServiceTypePool{}).Error; err != nil {
			return err
		}
		return tx.Delete(&current).Error
	})
}

func saveRoutingServiceTypePool(pool *ModelRoutingPool, revision string) error {
	encoded, err := common.Marshal(pool)
	if err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var serviceType RoutingServiceType
		if err := lockForUpdate(tx).First(&serviceType, pool.ServiceTypeID).Error; err != nil {
			return err
		}
		query := tx.Where("service_type_id = ? AND routing_group = ? AND model = ?", pool.ServiceTypeID, pool.RoutingGroup, pool.Model)
		var current RoutingServiceTypePool
		err := query.First(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if current.Revision != revision {
			return ErrRoutingPoolConflict
		}
		row := RoutingServiceTypePool{ServiceTypeID: pool.ServiceTypeID, RoutingGroup: pool.RoutingGroup, Model: pool.Model, Revision: pool.Revision, ConfigJSON: string(encoded)}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = tx.Create(&row).Error
		} else {
			err = tx.Model(&current).Updates(map[string]any{"revision": row.Revision, "config_json": row.ConfigJSON}).Error
		}
		if err != nil {
			return err
		}
		return tx.Model(&serviceType).Update("revision", common.GetUUID()).Error
	})
}

func deleteRoutingServiceTypePool(group, name, revision string, id int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var serviceType RoutingServiceType
		if err := lockForUpdate(tx).First(&serviceType, id).Error; err != nil {
			return err
		}
		result := tx.Where("service_type_id = ? AND routing_group = ? AND model = ? AND revision = ?", id, group, name, revision).Delete(&RoutingServiceTypePool{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrRoutingPoolConflict
		}
		return tx.Model(&serviceType).Update("revision", common.GetUUID()).Error
	})
}

func ListUserRoutingServiceTypes(userID int) ([]UserRoutingServiceType, error) {
	rows := make([]UserRoutingServiceType, 0)
	if userID <= 0 {
		return rows, nil
	}
	err := DB.Where("user_id = ?", userID).Order("model, routing_group").Find(&rows).Error
	return rows, err
}

func SaveUserRoutingServiceType(row *UserRoutingServiceType, revision string, reset bool) error {
	if row == nil || row.UserID <= 0 || row.ServiceTypeID < 0 || len(row.Model) > 255 || strings.TrimSpace(row.Model) != row.Model || strings.ContainsAny(row.Model, "\x00\r\n") || len(row.RoutingGroup) > 64 || strings.TrimSpace(row.RoutingGroup) != row.RoutingGroup || row.RoutingGroup == "auto" || strings.ContainsAny(row.RoutingGroup, ",\x00\r\n") {
		return ErrInvalidRoutingServiceType
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, row.UserID).Error; err != nil {
			return err
		}
		query := tx.Where("user_id = ? AND model = ? AND routing_group = ?", row.UserID, row.Model, row.RoutingGroup)
		var current UserRoutingServiceType
		err := query.First(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if current.Revision != revision {
			return ErrRoutingPoolConflict
		}
		if reset {
			return tx.Where("user_id = ? AND model = ? AND routing_group = ?", row.UserID, row.Model, row.RoutingGroup).Delete(&UserRoutingServiceType{}).Error
		}
		if row.ServiceTypeID > 0 {
			var serviceType RoutingServiceType
			if err := lockForUpdate(tx).First(&serviceType, row.ServiceTypeID).Error; err != nil {
				return err
			}
		}
		row.Revision = common.GetUUID()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(row).Error
		}
		return tx.Model(&current).Where("user_id = ? AND model = ? AND routing_group = ?", row.UserID, row.Model, row.RoutingGroup).Updates(map[string]any{"service_type_id": row.ServiceTypeID, "revision": row.Revision}).Error
	})
}

// ResolveUserRoutingServiceType selects one binding without merging independent scopes.
func ResolveUserRoutingServiceType(rows []UserRoutingServiceType, modelName, group string) *UserRoutingServiceType {
	models := []string{modelName}
	if base := ratio_setting.RoutingMatchModelName(modelName); base != "" && base != modelName {
		models = append(models, base)
	}
	for _, scopedGroup := range []string{group, ""} {
		for _, name := range models {
			for i := range rows {
				row := &rows[i]
				if row.Model == name && row.RoutingGroup == scopedGroup {
					return row
				}
			}
		}
	}
	for _, scopedGroup := range []string{group, ""} {
		for i := range rows {
			row := &rows[i]
			if row.Model == "" && row.RoutingGroup == scopedGroup {
				return row
			}
		}
	}
	return nil
}

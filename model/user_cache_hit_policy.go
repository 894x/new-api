package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const UserCacheHitPolicyOptionPrefix = "UserCacheHitPolicies:"

var ErrCacheHitPolicyConflict = errors.New("cache hit policy changed; reload before saving")

// Percentages use basis points: 4000 means 40%. The lower bound is the start
// of the random target range, never a guaranteed minimum cache discount.
type UserCacheHitPolicy struct {
	Enabled bool `json:"enabled"`
	MinBPS  int  `json:"min_bps"`
	MaxBPS  int  `json:"max_bps"`
}

func (p UserCacheHitPolicy) Validate() error {
	if p.MinBPS < 0 || p.MaxBPS > 10000 || p.MinBPS > p.MaxBPS {
		return errors.New("cache hit target must satisfy 0 <= minimum <= maximum <= 100%")
	}
	return nil
}

func ParseUserCacheHitPolicies(raw string) (map[string]UserCacheHitPolicy, error) {
	policies := make(map[string]UserCacheHitPolicy)
	if raw != "" {
		if err := common.UnmarshalJsonStr(raw, &policies); err != nil {
			return nil, err
		}
	}
	if policies == nil || len(policies) > 128 {
		return nil, errors.New("cache hit policies must be an object with at most 128 models")
	}
	for modelName, policy := range policies {
		if modelName == "" || modelName != strings.TrimSpace(modelName) || len(modelName) > 255 {
			return nil, errors.New("invalid cache hit policy model")
		}
		if err := policy.Validate(); err != nil {
			return nil, err
		}
	}
	return policies, nil
}

func CacheHitPolicyRevision(raw string) string {
	if raw == "" {
		raw = "{}"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}

// Existing options storage avoids a schema migration. Read from the primary
// database so edits and disabling are immediately visible on every replica.
func GetUserCacheHitPolicies(userID int) (map[string]UserCacheHitPolicy, string, error) {
	var option Option
	err := DB.Where(&Option{Key: UserCacheHitPolicyOptionPrefix + strconv.Itoa(userID)}).Limit(1).Find(&option).Error
	if err != nil {
		return nil, "", err
	}
	policies, err := ParseUserCacheHitPolicies(option.Value)
	return policies, CacheHitPolicyRevision(option.Value), err
}

func UpdateUserCacheHitPolicy(userID int, modelName, revision string, policy UserCacheHitPolicy) error {
	if userID <= 0 || modelName == "" || modelName != strings.TrimSpace(modelName) || len(modelName) > 255 {
		return errors.New("invalid user or cache hit policy model")
	}
	if err := policy.Validate(); err != nil {
		return err
	}
	key := UserCacheHitPolicyOptionPrefix + strconv.Itoa(userID)
	err := DB.Transaction(func(tx *gorm.DB) error {
		option := Option{Key: key, Value: "{}"}
		if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where(&Option{Key: key}).First(&option).Error; err != nil {
			return err
		}
		if CacheHitPolicyRevision(option.Value) != revision {
			return ErrCacheHitPolicyConflict
		}
		policies, err := ParseUserCacheHitPolicies(option.Value)
		if err != nil {
			return err
		}
		policies[modelName] = policy
		if len(policies) > 128 {
			return errors.New("at most 128 cache hit policy models are allowed")
		}
		data, err := common.Marshal(policies)
		if err != nil {
			return err
		}
		encoded := string(data)
		if encoded == option.Value {
			return nil
		}
		result := tx.Model(&Option{}).Where(&Option{Key: key}).Where("value = ?", option.Value).Update("value", encoded)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrCacheHitPolicyConflict
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

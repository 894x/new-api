package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	ModelDocumentMaxHTMLBytes         = 1024 * 1024
	ModelDocumentDefaultInterfaceKey  = "default"
	ModelDocumentDefaultInterfaceName = "默认接口"
)

// ModelDocument is the legacy single-document table. It remains mapped so
// existing installations can migrate their document into the variant table.
type ModelDocument struct {
	Id                int    `json:"id"`
	ModelId           int    `json:"model_id" gorm:"not null;uniqueIndex"`
	Slug              string `json:"slug" gorm:"size:255;not null;uniqueIndex"`
	Title             string `json:"title" gorm:"size:255;not null"`
	Vendor            string `json:"vendor" gorm:"size:128;index"`
	Category          string `json:"category" gorm:"size:64;index"`
	Summary           string `json:"summary" gorm:"type:text"`
	DraftHTML         string `json:"draft_html" gorm:"type:text"`
	PublishedSlug     string `json:"-" gorm:"size:255;index"`
	PublishedTitle    string `json:"-" gorm:"size:255"`
	PublishedVendor   string `json:"-" gorm:"size:128;index"`
	PublishedCategory string `json:"-" gorm:"size:64;index"`
	PublishedSummary  string `json:"-" gorm:"type:text"`
	PublishedHTML     string `json:"-" gorm:"type:text"`
	Published         int    `json:"published"`
	CreatedTime       int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime       int64  `json:"updated_time" gorm:"bigint"`
	PublishedTime     int64  `json:"published_time" gorm:"bigint"`
}

type ModelDocumentVariant struct {
	Id                     int    `json:"id"`
	ModelId                int    `json:"model_id" gorm:"not null;uniqueIndex:idx_model_document_variant,priority:1"`
	InterfaceKey           string `json:"interface_key" gorm:"size:64;not null;uniqueIndex:idx_model_document_variant,priority:2"`
	InterfaceName          string `json:"interface_name" gorm:"size:128;not null"`
	Slug                   string `json:"slug" gorm:"size:255;not null;uniqueIndex"`
	Title                  string `json:"title" gorm:"size:255;not null"`
	Vendor                 string `json:"vendor" gorm:"size:128;index"`
	Category               string `json:"category" gorm:"size:64;index"`
	Summary                string `json:"summary" gorm:"type:text"`
	DraftHTML              string `json:"draft_html" gorm:"type:text"`
	PublishedInterfaceName string `json:"-" gorm:"size:128"`
	PublishedSlug          string `json:"-" gorm:"size:255;index"`
	PublishedTitle         string `json:"-" gorm:"size:255"`
	PublishedVendor        string `json:"-" gorm:"size:128;index"`
	PublishedCategory      string `json:"-" gorm:"size:64;index"`
	PublishedSummary       string `json:"-" gorm:"type:text"`
	PublishedHTML          string `json:"-" gorm:"type:text"`
	Published              int    `json:"published"`
	CreatedTime            int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime            int64  `json:"updated_time" gorm:"bigint"`
	PublishedTime          int64  `json:"published_time" gorm:"bigint"`
}

func MigrateLegacyModelDocuments() error {
	var documents []ModelDocument
	if err := DB.Find(&documents).Error; err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for _, document := range documents {
			variant := ModelDocumentVariant{
				ModelId:                document.ModelId,
				InterfaceKey:           ModelDocumentDefaultInterfaceKey,
				InterfaceName:          ModelDocumentDefaultInterfaceName,
				Slug:                   document.Slug,
				Title:                  document.Title,
				Vendor:                 document.Vendor,
				Category:               document.Category,
				Summary:                document.Summary,
				DraftHTML:              document.DraftHTML,
				PublishedInterfaceName: ModelDocumentDefaultInterfaceName,
				PublishedSlug:          document.PublishedSlug,
				PublishedTitle:         document.PublishedTitle,
				PublishedVendor:        document.PublishedVendor,
				PublishedCategory:      document.PublishedCategory,
				PublishedSummary:       document.PublishedSummary,
				PublishedHTML:          document.PublishedHTML,
				Published:              document.Published,
				CreatedTime:            document.CreatedTime,
				UpdatedTime:            document.UpdatedTime,
				PublishedTime:          document.PublishedTime,
			}
			if err := tx.Where("model_id = ? AND interface_key = ?", document.ModelId, ModelDocumentDefaultInterfaceKey).
				FirstOrCreate(&variant).Error; err != nil {
				return err
			}
			if err := tx.Delete(&document).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func GetModelDocumentVariant(modelID int, interfaceKey string) (*ModelDocumentVariant, bool, error) {
	var document ModelDocumentVariant
	err := DB.Where("model_id = ? AND interface_key = ?", modelID, interfaceKey).First(&document).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &document, true, nil
}

func GetModelDocumentVariants(modelID int) ([]ModelDocumentVariant, error) {
	var documents []ModelDocumentVariant
	err := DB.Where("model_id = ?", modelID).Order("id ASC").Find(&documents).Error
	return documents, err
}

func GetPublishedModelDocumentVariantBySlug(slug string) (*ModelDocumentVariant, bool, error) {
	var document ModelDocumentVariant
	err := DB.Where("published_slug = ? AND published = ?", slug, 1).First(&document).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &document, true, nil
}

func GetPublishedModelDocumentVariants(modelIDs []int) ([]ModelDocumentVariant, error) {
	if len(modelIDs) == 0 {
		return []ModelDocumentVariant{}, nil
	}
	var documents []ModelDocumentVariant
	err := DB.Where("model_id IN ? AND published = ?", modelIDs, 1).Find(&documents).Error
	return documents, err
}

func GetPublishedModelDocumentModelIDs(modelIDs []int) (map[int]struct{}, error) {
	result := make(map[int]struct{}, len(modelIDs))
	if len(modelIDs) == 0 {
		return result, nil
	}
	var publishedModelIDs []int
	err := DB.Model(&ModelDocumentVariant{}).
		Where("model_id IN ? AND published = ?", modelIDs, 1).
		Distinct("model_id").
		Pluck("model_id", &publishedModelIDs).Error
	if err != nil {
		return nil, err
	}
	for _, modelID := range publishedModelIDs {
		result[modelID] = struct{}{}
	}
	return result, nil
}

func IsModelDocumentSlugDuplicated(modelID int, interfaceKey string, slug string) (bool, error) {
	var count int64
	err := DB.Model(&ModelDocumentVariant{}).
		Where("(slug = ? OR published_slug = ?) AND NOT (model_id = ? AND interface_key = ?)", slug, slug, modelID, interfaceKey).
		Count(&count).Error
	return count > 0, err
}

func SaveModelDocumentVariantDraft(document *ModelDocumentVariant) error {
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing ModelDocumentVariant
		err := tx.Where("model_id = ? AND interface_key = ?", document.ModelId, document.InterfaceKey).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			document.CreatedTime = now
			document.UpdatedTime = now
			document.Published = 0
			return tx.Create(document).Error
		}
		if err != nil {
			return err
		}
		document.Id = existing.Id
		document.CreatedTime = existing.CreatedTime
		document.Published = existing.Published
		document.PublishedTime = existing.PublishedTime
		document.PublishedInterfaceName = existing.PublishedInterfaceName
		document.PublishedSlug = existing.PublishedSlug
		document.PublishedTitle = existing.PublishedTitle
		document.PublishedVendor = existing.PublishedVendor
		document.PublishedCategory = existing.PublishedCategory
		document.PublishedSummary = existing.PublishedSummary
		document.PublishedHTML = existing.PublishedHTML
		document.UpdatedTime = now
		return tx.Model(&existing).Updates(map[string]interface{}{
			"interface_name": document.InterfaceName,
			"slug":           document.Slug,
			"title":          document.Title,
			"vendor":         document.Vendor,
			"category":       document.Category,
			"summary":        document.Summary,
			"draft_html":     document.DraftHTML,
			"updated_time":   now,
		}).Error
	})
}

func PublishModelDocumentVariant(modelID int, interfaceKey string) (*ModelDocumentVariant, bool, error) {
	var document ModelDocumentVariant
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("model_id = ? AND interface_key = ?", modelID, interfaceKey).First(&document).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(&document).Updates(map[string]interface{}{
			"published_interface_name": document.InterfaceName,
			"published_slug":           document.Slug,
			"published_title":          document.Title,
			"published_vendor":         document.Vendor,
			"published_category":       document.Category,
			"published_summary":        document.Summary,
			"published_html":           document.DraftHTML,
			"published":                1,
			"published_time":           now,
			"updated_time":             now,
		}).Error; err != nil {
			return err
		}
		document.PublishedInterfaceName = document.InterfaceName
		document.PublishedHTML = document.DraftHTML
		document.PublishedSlug = document.Slug
		document.PublishedTitle = document.Title
		document.PublishedVendor = document.Vendor
		document.PublishedCategory = document.Category
		document.PublishedSummary = document.Summary
		document.Published = 1
		document.PublishedTime = now
		document.UpdatedTime = now
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &document, true, nil
}

func DeleteModelDocumentVariant(modelID int, interfaceKey string) error {
	return DB.Where("model_id = ? AND interface_key = ?", modelID, interfaceKey).Delete(&ModelDocumentVariant{}).Error
}

func DeleteModelDocuments(modelID int) error {
	return DB.Where("model_id = ?", modelID).Delete(&ModelDocumentVariant{}).Error
}

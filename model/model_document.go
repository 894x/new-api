package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const ModelDocumentMaxHTMLBytes = 1024 * 1024

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

func GetModelDocumentByModelID(modelID int) (*ModelDocument, bool, error) {
	var document ModelDocument
	err := DB.Where("model_id = ?", modelID).First(&document).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &document, true, nil
}

func GetPublishedModelDocumentBySlug(slug string) (*ModelDocument, bool, error) {
	var document ModelDocument
	err := DB.Where("published_slug = ? AND published = ?", slug, 1).First(&document).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &document, true, nil
}

func GetPublishedModelDocuments(modelIDs []int) ([]ModelDocument, error) {
	if len(modelIDs) == 0 {
		return []ModelDocument{}, nil
	}
	var documents []ModelDocument
	err := DB.Where("model_id IN ? AND published = ?", modelIDs, 1).Find(&documents).Error
	return documents, err
}

func GetPublishedModelDocumentModelIDs(modelIDs []int) (map[int]struct{}, error) {
	result := make(map[int]struct{}, len(modelIDs))
	if len(modelIDs) == 0 {
		return result, nil
	}
	var publishedModelIDs []int
	err := DB.Model(&ModelDocument{}).
		Where("model_id IN ? AND published = ?", modelIDs, 1).
		Pluck("model_id", &publishedModelIDs).Error
	if err != nil {
		return nil, err
	}
	for _, modelID := range publishedModelIDs {
		result[modelID] = struct{}{}
	}
	return result, nil
}

func IsModelDocumentSlugDuplicated(modelID int, slug string) (bool, error) {
	var count int64
	err := DB.Model(&ModelDocument{}).
		Where("(slug = ? OR published_slug = ?) AND model_id <> ?", slug, slug, modelID).
		Count(&count).Error
	return count > 0, err
}

func SaveModelDocumentDraft(document *ModelDocument) error {
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var existing ModelDocument
		err := tx.Where("model_id = ?", document.ModelId).First(&existing).Error
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
		document.PublishedSlug = existing.PublishedSlug
		document.PublishedTitle = existing.PublishedTitle
		document.PublishedVendor = existing.PublishedVendor
		document.PublishedCategory = existing.PublishedCategory
		document.PublishedSummary = existing.PublishedSummary
		document.PublishedHTML = existing.PublishedHTML
		document.UpdatedTime = now
		return tx.Model(&existing).Updates(map[string]interface{}{
			"slug":         document.Slug,
			"title":        document.Title,
			"vendor":       document.Vendor,
			"category":     document.Category,
			"summary":      document.Summary,
			"draft_html":   document.DraftHTML,
			"updated_time": now,
		}).Error
	})
}

func PublishModelDocument(modelID int) (*ModelDocument, bool, error) {
	var document ModelDocument
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("model_id = ?", modelID).First(&document).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		if err := tx.Model(&document).Updates(map[string]interface{}{
			"published_slug":     document.Slug,
			"published_title":    document.Title,
			"published_vendor":   document.Vendor,
			"published_category": document.Category,
			"published_summary":  document.Summary,
			"published_html":     document.DraftHTML,
			"published":          1,
			"published_time":     now,
			"updated_time":       now,
		}).Error; err != nil {
			return err
		}
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

func DeleteModelDocument(modelID int) error {
	return DB.Where("model_id = ?", modelID).Delete(&ModelDocument{}).Error
}

package controller

import (
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/modeldoc"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxModelDocumentRequestBytes = 2 * model.ModelDocumentMaxHTMLBytes

var modelDocumentSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)

type modelDocumentEditorRequest struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Vendor   string `json:"vendor"`
	Category string `json:"category"`
	Summary  string `json:"summary"`
	HTML     string `json:"html"`
}

type modelDocumentEditorData struct {
	ModelID         int    `json:"model_id"`
	Model           string `json:"model"`
	Source          string `json:"source"`
	EffectiveSource string `json:"effective_source"`
	HasBuiltin      bool   `json:"has_builtin"`
	HasCustom       bool   `json:"has_custom"`
	Published       bool   `json:"published"`
	Slug            string `json:"slug"`
	Title           string `json:"title"`
	Vendor          string `json:"vendor"`
	Category        string `json:"category"`
	Summary         string `json:"summary"`
	HTML            string `json:"html"`
	UpdatedTime     int64  `json:"updated_time"`
}

func GetModelDocsCatalog(c *gin.Context) {
	builtinDocuments, err := modeldoc.List()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var enabledModels []model.Model
	if err := model.DB.Select("id", "model_name").Where("doc_enabled = ?", 1).Find(&enabledModels).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	enabledByName := make(map[string]model.Model, len(enabledModels))
	enabledByID := make(map[int]model.Model, len(enabledModels))
	modelIDs := make([]int, 0, len(enabledModels))
	for _, modelMeta := range enabledModels {
		enabledByName[modelMeta.ModelName] = modelMeta
		enabledByID[modelMeta.Id] = modelMeta
		modelIDs = append(modelIDs, modelMeta.Id)
	}
	customDocuments, err := model.GetPublishedModelDocuments(modelIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	customByModelID := make(map[int]model.ModelDocument, len(customDocuments))
	for _, document := range customDocuments {
		customByModelID[document.ModelId] = document
	}

	result := make([]modeldoc.Document, 0, len(builtinDocuments)+len(customDocuments))
	includedModelIDs := make(map[int]struct{}, len(customDocuments))
	for _, builtin := range builtinDocuments {
		modelMeta, enabled := enabledByName[builtin.Model]
		if !enabled {
			continue
		}
		if custom, overridden := customByModelID[modelMeta.Id]; overridden {
			result = append(result, customDocumentCatalogItem(custom, modelMeta.ModelName))
			includedModelIDs[modelMeta.Id] = struct{}{}
			continue
		}
		result = append(result, builtin)
	}
	remainingCustom := make([]modeldoc.Document, 0)
	for _, custom := range customDocuments {
		if _, included := includedModelIDs[custom.ModelId]; included {
			continue
		}
		modelMeta, enabled := enabledByID[custom.ModelId]
		if !enabled {
			continue
		}
		remainingCustom = append(remainingCustom, customDocumentCatalogItem(custom, modelMeta.ModelName))
	}
	sort.Slice(remainingCustom, func(i, j int) bool {
		return remainingCustom[i].Title < remainingCustom[j].Title
	})
	result = append(result, remainingCustom...)
	common.ApiSuccess(c, result)
}

func GetModelDoc(c *gin.Context) {
	slug := c.Param("slug")
	custom, customExists, err := model.GetPublishedModelDocumentBySlug(slug)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if customExists {
		var modelMeta model.Model
		if err := model.DB.Select("id", "doc_enabled").First(&modelMeta, custom.ModelId).Error; errors.Is(err, gorm.ErrRecordNotFound) || modelMeta.DocEnabled != 1 {
			writeModelDocumentNotFound(c)
			return
		} else if err != nil {
			common.ApiError(c, err)
			return
		}
		writeModelDocumentHTML(c, []byte(custom.PublishedHTML))
		return
	}

	builtin, exists, err := modeldoc.Find(slug)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exists {
		writeModelDocumentNotFound(c)
		return
	}
	var modelMeta model.Model
	if err := model.DB.Select("id", "doc_enabled").Where("model_name = ?", builtin.Model).First(&modelMeta).Error; errors.Is(err, gorm.ErrRecordNotFound) || modelMeta.DocEnabled != 1 {
		writeModelDocumentNotFound(c)
		return
	} else if err != nil {
		common.ApiError(c, err)
		return
	}
	override, overrideExists, err := model.GetModelDocumentByModelID(modelMeta.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if overrideExists && override.Published == 1 {
		writeModelDocumentNotFound(c)
		return
	}
	document, _, err := modeldoc.Read(builtin.Slug)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeModelDocumentHTML(c, document)
}

func GetModelDocumentEditor(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	data, err := buildModelDocumentEditorData(*modelMeta)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func SaveModelDocumentEditorDraft(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxModelDocumentRequestBytes)
	var request modelDocumentEditorRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "文档内容无效或超过 1 MB")
		return
	}
	request.Slug = strings.TrimSpace(request.Slug)
	request.Title = strings.TrimSpace(request.Title)
	request.Vendor = strings.TrimSpace(request.Vendor)
	request.Category = strings.TrimSpace(request.Category)
	request.Summary = strings.TrimSpace(request.Summary)
	request.HTML = strings.TrimSpace(request.HTML)
	if request.Slug == "" || !modelDocumentSlugPattern.MatchString(request.Slug) {
		common.ApiErrorMsg(c, "文档 Slug 只能包含字母、数字、点、下划线和连字符")
		return
	}
	if request.Title == "" {
		common.ApiErrorMsg(c, "文档标题不能为空")
		return
	}
	if request.HTML == "" {
		common.ApiErrorMsg(c, "HTML 文档不能为空")
		return
	}
	if len([]byte(request.HTML)) > model.ModelDocumentMaxHTMLBytes {
		common.ApiErrorMsg(c, "HTML 文档不能超过 1 MB")
		return
	}
	if builtin, exists, err := modeldoc.Find(request.Slug); err != nil {
		common.ApiError(c, err)
		return
	} else if exists && builtin.Model != modelMeta.ModelName {
		common.ApiErrorMsg(c, "文档 Slug 已被内置文档使用")
		return
	}
	duplicated, err := model.IsModelDocumentSlugDuplicated(modelMeta.Id, request.Slug)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if duplicated {
		common.ApiErrorMsg(c, "文档 Slug 已被其他模型使用")
		return
	}
	document := &model.ModelDocument{
		ModelId:   modelMeta.Id,
		Slug:      request.Slug,
		Title:     request.Title,
		Vendor:    request.Vendor,
		Category:  request.Category,
		Summary:   request.Summary,
		DraftHTML: request.HTML,
	}
	if err := model.SaveModelDocumentDraft(document); err != nil {
		common.ApiError(c, err)
		return
	}
	data, err := buildModelDocumentEditorData(*modelMeta)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func PublishModelDocumentEditor(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	document, exists, err := model.GetModelDocumentByModelID(modelMeta.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !exists || strings.TrimSpace(document.DraftHTML) == "" {
		common.ApiErrorMsg(c, "请先保存 HTML 文档草稿")
		return
	}
	if len([]byte(document.DraftHTML)) > model.ModelDocumentMaxHTMLBytes {
		common.ApiErrorMsg(c, "HTML 文档不能超过 1 MB")
		return
	}
	if _, exists, err := model.PublishModelDocument(modelMeta.Id); err != nil {
		common.ApiError(c, err)
		return
	} else if !exists {
		common.ApiErrorMsg(c, "请先保存 HTML 文档草稿")
		return
	}
	data, err := buildModelDocumentEditorData(*modelMeta)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func DeleteModelDocumentEditor(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	if err := model.DeleteModelDocument(modelMeta.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	if !modeldoc.HasModel(modelMeta.ModelName) {
		if err := model.DB.Model(&model.Model{}).Where("id = ?", modelMeta.Id).Update("doc_enabled", 0).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	}
	data, err := buildModelDocumentEditorData(*modelMeta)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func PreviewModelDocumentEditor(c *gin.Context) {
	if _, ok := getModelDocumentModel(c); !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxModelDocumentRequestBytes)
	var request struct {
		HTML string `json:"html"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "文档内容无效或超过 1 MB")
		return
	}
	request.HTML = strings.TrimSpace(request.HTML)
	if request.HTML == "" {
		common.ApiErrorMsg(c, "HTML 文档不能为空")
		return
	}
	if len([]byte(request.HTML)) > model.ModelDocumentMaxHTMLBytes {
		common.ApiErrorMsg(c, "HTML 文档不能超过 1 MB")
		return
	}
	common.ApiSuccess(c, string(modeldoc.Render([]byte(request.HTML))))
}

func getModelDocumentModel(c *gin.Context) (*model.Model, bool) {
	modelID, err := strconv.Atoi(c.Param("id"))
	if err != nil || modelID <= 0 {
		common.ApiErrorMsg(c, "无效的模型 ID")
		return nil, false
	}
	var modelMeta model.Model
	if err := model.DB.First(&modelMeta, modelID).Error; err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	return &modelMeta, true
}

func buildModelDocumentEditorData(modelMeta model.Model) (*modelDocumentEditorData, error) {
	builtin, hasBuiltin, err := modeldoc.FindByModel(modelMeta.ModelName)
	if err != nil {
		return nil, err
	}
	data := &modelDocumentEditorData{
		ModelID:    modelMeta.Id,
		Model:      modelMeta.ModelName,
		Source:     "empty",
		HasBuiltin: hasBuiltin,
		Slug:       "model-" + strconv.Itoa(modelMeta.Id),
		Title:      modelMeta.ModelName,
	}
	if hasBuiltin {
		html, _, err := modeldoc.Read(builtin.Slug)
		if err != nil {
			return nil, err
		}
		data.Source = "builtin"
		data.EffectiveSource = "builtin"
		data.Slug = builtin.Slug
		data.Title = builtin.Title
		data.Vendor = builtin.Vendor
		data.Category = builtin.Category
		data.Summary = builtin.Summary
		data.HTML = string(html)
	}
	custom, hasCustom, err := model.GetModelDocumentByModelID(modelMeta.Id)
	if err != nil {
		return nil, err
	}
	if !hasCustom {
		if data.EffectiveSource == "" {
			data.EffectiveSource = "none"
		}
		return data, nil
	}
	data.Source = "custom"
	data.HasCustom = true
	data.Published = custom.Published == 1
	data.Slug = custom.Slug
	data.Title = custom.Title
	data.Vendor = custom.Vendor
	data.Category = custom.Category
	data.Summary = custom.Summary
	data.HTML = custom.DraftHTML
	data.UpdatedTime = custom.UpdatedTime
	if custom.Published == 1 {
		data.EffectiveSource = "custom"
	} else if hasBuiltin {
		data.EffectiveSource = "builtin"
	} else {
		data.EffectiveSource = "none"
	}
	return data, nil
}

func customDocumentCatalogItem(document model.ModelDocument, modelName string) modeldoc.Document {
	return modeldoc.Document{
		Slug:      document.PublishedSlug,
		Model:     modelName,
		Title:     document.PublishedTitle,
		Vendor:    document.PublishedVendor,
		Category:  document.PublishedCategory,
		Summary:   document.PublishedSummary,
		UpdatedAt: time.Unix(document.PublishedTime, 0).Format("2006-01-02"),
	}
}

func writeModelDocumentNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"success": false,
		"message": "model document not found",
	})
}

func writeModelDocumentHTML(c *gin.Context, document []byte) {
	c.Header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data: https:; base-uri 'none'; form-action 'none'; frame-ancestors 'self'")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "text/html; charset=utf-8", modeldoc.Render(document))
}

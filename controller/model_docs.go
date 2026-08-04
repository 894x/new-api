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

var (
	modelDocumentSlugPattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,254}$`)
	modelDocumentInterfacePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type modelDocumentEditorRequest struct {
	InterfaceName string `json:"interface_name"`
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	Vendor        string `json:"vendor"`
	Category      string `json:"category"`
	Summary       string `json:"summary"`
	HTML          string `json:"html"`
}

type modelDocumentEditorData struct {
	ModelID         int    `json:"model_id"`
	Model           string `json:"model"`
	InterfaceKey    string `json:"interface_key"`
	InterfaceName   string `json:"interface_name"`
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

type modelDocumentEditorCollection struct {
	ModelID  int                       `json:"model_id"`
	Model    string                    `json:"model"`
	Variants []modelDocumentEditorData `json:"variants"`
}

type modelDocumentVariantIdentity struct {
	ModelID      int
	InterfaceKey string
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
	customDocuments, err := model.GetPublishedModelDocumentVariants(modelIDs)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	customByVariant := make(map[modelDocumentVariantIdentity]model.ModelDocumentVariant, len(customDocuments))
	for _, document := range customDocuments {
		customByVariant[modelDocumentVariantIdentity{ModelID: document.ModelId, InterfaceKey: document.InterfaceKey}] = document
	}

	result := make([]modeldoc.Document, 0, len(builtinDocuments)+len(customDocuments))
	includedVariantIDs := make(map[int]struct{}, len(customDocuments))
	for _, builtin := range builtinDocuments {
		modelMeta, enabled := enabledByName[builtin.Model]
		if !enabled {
			continue
		}
		identity := modelDocumentVariantIdentity{ModelID: modelMeta.Id, InterfaceKey: builtin.InterfaceKey}
		if custom, overridden := customByVariant[identity]; overridden {
			result = append(result, customDocumentCatalogItem(custom, modelMeta.ModelName))
			includedVariantIDs[custom.Id] = struct{}{}
			continue
		}
		result = append(result, builtin)
	}
	remainingCustom := make([]modeldoc.Document, 0)
	for _, custom := range customDocuments {
		if _, included := includedVariantIDs[custom.Id]; included {
			continue
		}
		modelMeta, enabled := enabledByID[custom.ModelId]
		if !enabled {
			continue
		}
		remainingCustom = append(remainingCustom, customDocumentCatalogItem(custom, modelMeta.ModelName))
	}
	sort.Slice(remainingCustom, func(i, j int) bool {
		if remainingCustom[i].Title == remainingCustom[j].Title {
			return remainingCustom[i].InterfaceName < remainingCustom[j].InterfaceName
		}
		return remainingCustom[i].Title < remainingCustom[j].Title
	})
	result = append(result, remainingCustom...)
	common.ApiSuccess(c, result)
}

func GetModelDoc(c *gin.Context) {
	slug := c.Param("slug")
	custom, customExists, err := model.GetPublishedModelDocumentVariantBySlug(slug)
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
	override, overrideExists, err := model.GetModelDocumentVariant(modelMeta.Id, builtin.InterfaceKey)
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

func GetModelDocumentsEditor(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	data, err := buildModelDocumentEditorCollection(*modelMeta)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func SaveModelDocumentVariantDraft(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	interfaceKey := strings.TrimSpace(c.Param("interface_key"))
	if !modelDocumentInterfacePattern.MatchString(interfaceKey) {
		common.ApiErrorMsg(c, "接口标识只能包含字母、数字、点、下划线和连字符")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxModelDocumentRequestBytes)
	var request modelDocumentEditorRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "文档内容无效或超过 1 MB")
		return
	}
	request.InterfaceName = strings.TrimSpace(request.InterfaceName)
	request.Slug = strings.TrimSpace(request.Slug)
	request.Title = strings.TrimSpace(request.Title)
	request.Vendor = strings.TrimSpace(request.Vendor)
	request.Category = strings.TrimSpace(request.Category)
	request.Summary = strings.TrimSpace(request.Summary)
	request.HTML = strings.TrimSpace(request.HTML)
	if request.InterfaceName == "" {
		common.ApiErrorMsg(c, "接口名称不能为空")
		return
	}
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
	} else if exists && (builtin.Model != modelMeta.ModelName || builtin.InterfaceKey != interfaceKey) {
		common.ApiErrorMsg(c, "文档 Slug 已被内置文档使用")
		return
	}
	duplicated, err := model.IsModelDocumentSlugDuplicated(modelMeta.Id, interfaceKey, request.Slug)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if duplicated {
		common.ApiErrorMsg(c, "文档 Slug 已被其他接口文档使用")
		return
	}
	document := &model.ModelDocumentVariant{
		ModelId:       modelMeta.Id,
		InterfaceKey:  interfaceKey,
		InterfaceName: request.InterfaceName,
		Slug:          request.Slug,
		Title:         request.Title,
		Vendor:        request.Vendor,
		Category:      request.Category,
		Summary:       request.Summary,
		DraftHTML:     request.HTML,
	}
	if err := model.SaveModelDocumentVariantDraft(document); err != nil {
		common.ApiError(c, err)
		return
	}
	writeModelDocumentEditorCollection(c, *modelMeta)
}

func PublishModelDocumentVariant(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	interfaceKey := strings.TrimSpace(c.Param("interface_key"))
	document, exists, err := model.GetModelDocumentVariant(modelMeta.Id, interfaceKey)
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
	if _, exists, err := model.PublishModelDocumentVariant(modelMeta.Id, interfaceKey); err != nil {
		common.ApiError(c, err)
		return
	} else if !exists {
		common.ApiErrorMsg(c, "请先保存 HTML 文档草稿")
		return
	}
	writeModelDocumentEditorCollection(c, *modelMeta)
}

func DeleteModelDocumentVariant(c *gin.Context) {
	modelMeta, ok := getModelDocumentModel(c)
	if !ok {
		return
	}
	interfaceKey := strings.TrimSpace(c.Param("interface_key"))
	if err := model.DeleteModelDocumentVariant(modelMeta.Id, interfaceKey); err != nil {
		common.ApiError(c, err)
		return
	}
	builtinDocuments, err := modeldoc.ListByModel(modelMeta.ModelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	publishedDocuments, err := model.GetPublishedModelDocumentVariants([]int{modelMeta.Id})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if len(builtinDocuments) == 0 && len(publishedDocuments) == 0 {
		if err := model.DB.Model(&model.Model{}).Where("id = ?", modelMeta.Id).Update("doc_enabled", 0).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	}
	writeModelDocumentEditorCollection(c, *modelMeta)
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

func buildModelDocumentEditorCollection(modelMeta model.Model) (*modelDocumentEditorCollection, error) {
	builtinDocuments, err := modeldoc.ListByModel(modelMeta.ModelName)
	if err != nil {
		return nil, err
	}
	customDocuments, err := model.GetModelDocumentVariants(modelMeta.Id)
	if err != nil {
		return nil, err
	}
	customByInterface := make(map[string]model.ModelDocumentVariant, len(customDocuments))
	for _, document := range customDocuments {
		customByInterface[document.InterfaceKey] = document
	}
	result := &modelDocumentEditorCollection{
		ModelID:  modelMeta.Id,
		Model:    modelMeta.ModelName,
		Variants: make([]modelDocumentEditorData, 0, len(builtinDocuments)+len(customDocuments)),
	}
	for _, builtin := range builtinDocuments {
		custom, hasCustom := customByInterface[builtin.InterfaceKey]
		data, err := buildModelDocumentEditorData(modelMeta, builtin, true, custom, hasCustom)
		if err != nil {
			return nil, err
		}
		result.Variants = append(result.Variants, data)
		delete(customByInterface, builtin.InterfaceKey)
	}
	for _, custom := range customDocuments {
		if _, remaining := customByInterface[custom.InterfaceKey]; !remaining {
			continue
		}
		data, err := buildModelDocumentEditorData(modelMeta, modeldoc.Document{}, false, custom, true)
		if err != nil {
			return nil, err
		}
		result.Variants = append(result.Variants, data)
	}
	return result, nil
}

func buildModelDocumentEditorData(modelMeta model.Model, builtin modeldoc.Document, hasBuiltin bool, custom model.ModelDocumentVariant, hasCustom bool) (modelDocumentEditorData, error) {
	data := modelDocumentEditorData{
		ModelID:         modelMeta.Id,
		Model:           modelMeta.ModelName,
		Source:          "empty",
		EffectiveSource: "none",
		HasBuiltin:      hasBuiltin,
	}
	if hasBuiltin {
		html, _, err := modeldoc.Read(builtin.Slug)
		if err != nil {
			return modelDocumentEditorData{}, err
		}
		data.InterfaceKey = builtin.InterfaceKey
		data.InterfaceName = builtin.InterfaceName
		data.Source = "builtin"
		data.EffectiveSource = "builtin"
		data.Slug = builtin.Slug
		data.Title = builtin.Title
		data.Vendor = builtin.Vendor
		data.Category = builtin.Category
		data.Summary = builtin.Summary
		data.HTML = string(html)
	}
	if !hasCustom {
		return data, nil
	}
	data.InterfaceKey = custom.InterfaceKey
	data.InterfaceName = custom.InterfaceName
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
	}
	return data, nil
}

func writeModelDocumentEditorCollection(c *gin.Context, modelMeta model.Model) {
	data, err := buildModelDocumentEditorCollection(modelMeta)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func customDocumentCatalogItem(document model.ModelDocumentVariant, modelName string) modeldoc.Document {
	return modeldoc.Document{
		Slug:          document.PublishedSlug,
		Model:         modelName,
		InterfaceKey:  document.InterfaceKey,
		InterfaceName: document.PublishedInterfaceName,
		Title:         document.PublishedTitle,
		Vendor:        document.PublishedVendor,
		Category:      document.PublishedCategory,
		Summary:       document.PublishedSummary,
		UpdatedAt:     time.Unix(document.PublishedTime, 0).Format("2006-01-02"),
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

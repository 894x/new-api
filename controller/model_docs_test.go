package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/modeldoc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelDocsOnlyExposeIndividuallyEnabledDocuments(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:  "doubao-seedance-1-5-pro-251215",
		DocEnabled: 1,
	}).Error)
	require.NoError(t, db.Create(&model.Model{
		ModelName:  "doubao-seedance-2-0-260128",
		DocEnabled: 0,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs", nil)
	GetModelDocsCatalog(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                `json:"success"`
		Data    []modeldoc.Document `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	require.Len(t, response.Data, 1)
	assert.Equal(t, "doubao-seedance-1-5-pro-251215", response.Data[0].Model)
}

func TestModelDocRequiresIndividualEnablement(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:  "doubao-seedance-1-5-pro-251215",
		DocEnabled: 0,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs/doubao-seedance-1-5-pro-251215", nil)
	ctx.Params = gin.Params{{Key: "slug", Value: "doubao-seedance-1-5-pro-251215"}}
	GetModelDoc(ctx)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestModelDocUsesSharedSiteStyles(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Model{
		ModelName:  "doubao-seedance-1-5-pro-251215",
		DocEnabled: 1,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs/doubao-seedance-1-5-pro-251215", nil)
	ctx.Params = gin.Params{{Key: "slug", Value: "doubao-seedance-1-5-pro-251215"}}
	GetModelDoc(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "background: var(--background)")
	assert.Contains(t, recorder.Body.String(), ".back")
}

func TestCustomModelDocumentDraftPublishAndBuiltinFallback(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	modelMeta := &model.Model{
		ModelName:  "doubao-seedance-1-5-pro-251215",
		DocEnabled: 1,
	}
	require.NoError(t, db.Create(modelMeta).Error)

	saveModelDocumentDraftForTest(t, modelMeta.Id, model.ModelDocumentDefaultInterfaceKey, "doubao-seedance-1-5-pro-251215", "默认接口", "<main><h1>在线版本一</h1></main>")
	assertModelDocumentBodyContains(t, "doubao-seedance-1-5-pro-251215", "Seedance 1.5 Pro")
	publishModelDocumentForTest(t, modelMeta.Id, model.ModelDocumentDefaultInterfaceKey)
	assertModelDocumentBodyContains(t, "doubao-seedance-1-5-pro-251215", "在线版本一")

	saveModelDocumentDraftForTest(t, modelMeta.Id, model.ModelDocumentDefaultInterfaceKey, "doubao-seedance-1-5-pro-251215", "默认接口", "<main><h1>在线版本二</h1></main>")
	assertModelDocumentBodyContains(t, "doubao-seedance-1-5-pro-251215", "在线版本一")
	publishModelDocumentForTest(t, modelMeta.Id, model.ModelDocumentDefaultInterfaceKey)
	assertModelDocumentBodyContains(t, "doubao-seedance-1-5-pro-251215", "在线版本二")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/models/1/documents/default", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(modelMeta.Id)}, {Key: "interface_key", Value: model.ModelDocumentDefaultInterfaceKey}}
	DeleteModelDocumentVariant(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	assertModelDocumentBodyContains(t, "doubao-seedance-1-5-pro-251215", "Seedance 1.5 Pro")
}

func TestPublishedCustomDocumentAppearsInCatalogWithoutBuiltin(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	modelMeta := &model.Model{ModelName: "custom-video-model", DocEnabled: 1}
	require.NoError(t, db.Create(modelMeta).Error)

	payload, err := common.Marshal(modelDocumentEditorRequest{
		InterfaceName: "OpenAI 兼容接口",
		Slug:          "custom-video-model",
		Title:         "自定义视频模型",
		Vendor:        "Custom Vendor",
		Category:      "video",
		Summary:       "在线创建的模型文档",
		HTML:          "<main><h1>自定义模型文档</h1></main>",
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/models/1/documents/openai-compatible", bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(modelMeta.Id)}, {Key: "interface_key", Value: "openai-compatible"}}
	SaveModelDocumentVariantDraft(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	publishModelDocumentForTest(t, modelMeta.Id, "openai-compatible")

	recorder = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs", nil)
	GetModelDocsCatalog(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool                `json:"success"`
		Data    []modeldoc.Document `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	assert.Equal(t, "custom-video-model", response.Data[0].Slug)
	assert.Equal(t, "自定义视频模型", response.Data[0].Title)
	assert.Equal(t, "openai-compatible", response.Data[0].InterfaceKey)
	assert.Equal(t, "OpenAI 兼容接口", response.Data[0].InterfaceName)
}

func TestModelDocumentCatalogIncludesMultipleInterfacesForOneModel(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	modelMeta := &model.Model{ModelName: "multi-interface-model", DocEnabled: 1}
	require.NoError(t, db.Create(modelMeta).Error)

	saveModelDocumentDraftForTest(t, modelMeta.Id, "openai-compatible", "multi-interface-openai", "OpenAI 兼容接口", "<main><h1>OpenAI 文档</h1></main>")
	publishModelDocumentForTest(t, modelMeta.Id, "openai-compatible")
	saveModelDocumentDraftForTest(t, modelMeta.Id, "native", "multi-interface-native", "原生接口", "<main><h1>原生文档</h1></main>")
	publishModelDocumentForTest(t, modelMeta.Id, "native")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs", nil)
	GetModelDocsCatalog(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data []modeldoc.Document `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	assert.ElementsMatch(t, []string{"openai-compatible", "native"}, []string{response.Data[0].InterfaceKey, response.Data[1].InterfaceKey})
	assertModelDocumentBodyContains(t, "multi-interface-openai", "OpenAI 文档")
	assertModelDocumentBodyContains(t, "multi-interface-native", "原生文档")
}

func TestCustomInterfaceDoesNotReplaceAnotherBuiltinInterface(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	modelMeta := &model.Model{ModelName: "doubao-seedance-1-5-pro-251215", DocEnabled: 1}
	require.NoError(t, db.Create(modelMeta).Error)

	saveModelDocumentDraftForTest(t, modelMeta.Id, "openai-compatible", "seedance-openai-compatible", "OpenAI 兼容接口", "<main><h1>OpenAI 兼容文档</h1></main>")
	publishModelDocumentForTest(t, modelMeta.Id, "openai-compatible")

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs", nil)
	GetModelDocsCatalog(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data []modeldoc.Document `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
	assert.ElementsMatch(t, []string{model.ModelDocumentDefaultInterfaceKey, "openai-compatible"}, []string{response.Data[0].InterfaceKey, response.Data[1].InterfaceKey})
}

func TestLegacyModelDocumentMigratesToDefaultInterface(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	legacy := &model.ModelDocument{
		ModelId:       42,
		Slug:          "legacy-model-document",
		Title:         "旧文档",
		DraftHTML:     "<main>旧草稿</main>",
		PublishedSlug: "legacy-model-document",
		PublishedHTML: "<main>旧发布内容</main>",
		Published:     1,
	}
	require.NoError(t, db.Create(legacy).Error)
	require.NoError(t, model.MigrateLegacyModelDocuments())

	variant, exists, err := model.GetModelDocumentVariant(42, model.ModelDocumentDefaultInterfaceKey)
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, model.ModelDocumentDefaultInterfaceName, variant.InterfaceName)
	assert.Equal(t, "<main>旧发布内容</main>", variant.PublishedHTML)
	var legacyCount int64
	require.NoError(t, db.Model(&model.ModelDocument{}).Count(&legacyCount).Error)
	assert.Zero(t, legacyCount)
}

func saveModelDocumentDraftForTest(t *testing.T, modelID int, interfaceKey string, slug string, interfaceName string, html string) {
	t.Helper()
	payload, err := common.Marshal(modelDocumentEditorRequest{
		InterfaceName: interfaceName,
		Slug:          slug,
		Title:         "Seedance 在线文档",
		Vendor:        "VolcEngine",
		Category:      "video",
		Summary:       "在线编辑测试",
		HTML:          html,
	})
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/models/1/documents/"+interfaceKey, bytes.NewReader(payload))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(modelID)}, {Key: "interface_key", Value: interfaceKey}}
	SaveModelDocumentVariantDraft(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func publishModelDocumentForTest(t *testing.T, modelID int, interfaceKey string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/models/1/documents/"+interfaceKey+"/publish", nil)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(modelID)}, {Key: "interface_key", Value: interfaceKey}}
	PublishModelDocumentVariant(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

func assertModelDocumentBodyContains(t *testing.T, slug string, expected string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/model-docs/"+slug, nil)
	ctx.Params = gin.Params{{Key: "slug", Value: slug}}
	GetModelDoc(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), expected)
}

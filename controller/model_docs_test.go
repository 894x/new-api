package controller

import (
	"net/http"
	"net/http/httptest"
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

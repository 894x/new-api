package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRequestCaptureEndpointsRejectOrdinaryUsers(t *testing.T) {
	for _, handler := range []gin.HandlerFunc{GetRequestCapture, GetUserRequestCapture, UpdateUserRequestCapture} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", common.RoleCommonUser)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		handler(c)
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, w.Body.String())
	}
}

func TestRequestCaptureAdminPolicyAndMetadata(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.RequestCapture{}, &model.RequestCapturePolicy{}))
	previousDB, previousLog := model.DB, model.LOG_DB
	previousRedis := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = previousRedis })
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLog; _ = sqlDB.Close() })
	require.NoError(t, db.Create(&model.User{Id: 8, Username: "capture-user", Password: "unused", Role: common.RoleCommonUser}).Error)
	require.NoError(t, model.SetRequestCapturePolicy(context.Background(), model.RequestCapturePolicy{UserID: 8, Enabled: true}))
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{}`, http.StatusBadRequest}, {`{"enabled":null}`, http.StatusBadRequest}, {`{"enabled":"false"}`, http.StatusBadRequest}, {`{"enabled":false}`, http.StatusOK},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", common.RoleRootUser)
		c.Set("id", 8)
		c.Params = gin.Params{{Key: "id", Value: "8"}}
		c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(test.body))
		UpdateUserRequestCapture(c)
		assert.Equal(t, test.status, w.Code)
	}
	policy, err := model.GetRequestCapturePolicy(context.Background(), 8)
	require.NoError(t, err)
	assert.False(t, policy.Enabled)
	record := &model.RequestCapture{ID: common.GetUUID(), RequestID: "example", UserID: 8, Status: "ready", StoreID: "private-store", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.SaveRequestCapture(context.Background(), record))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleAdminUser)
	c.Params = gin.Params{{Key: "request_id", Value: "example"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	GetRequestCapture(c)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Contains(t, w.Body.String(), `"status":"ready"`)
	assert.NotContains(t, w.Body.String(), "private-store")
	assert.NotContains(t, w.Body.String(), `"parts"`)
}

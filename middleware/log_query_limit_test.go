package middleware

import (
	"net/http"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestLogQueryLimitUsesFreshUserOverrides(t *testing.T) {
	for _, redisEnabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(redisEnabled), func(t *testing.T) {
			previousDB, previousRedis := model.DB, common.RedisEnabled
			previousEnable, previousNum, previousDuration := common.LogQueryRateLimitEnable, common.LogQueryRateLimitNum, common.LogQueryRateLimitDuration
			db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{})
			require.NoError(t, err)
			connection, err := db.DB()
			require.NoError(t, err)
			model.DB, common.RedisEnabled = db, false
			common.LogQueryRateLimitEnable, common.LogQueryRateLimitNum, common.LogQueryRateLimitDuration = true, 2, 60
			t.Cleanup(func() {
				_ = connection.Close()
				model.DB, common.RedisEnabled = previousDB, previousRedis
				common.LogQueryRateLimitEnable, common.LogQueryRateLimitNum, common.LogQueryRateLimitDuration = previousEnable, previousNum, previousDuration
			})
			if redisEnabled {
				useRateLimitMiniRedis(t)
			}
			require.NoError(t, db.AutoMigrate(&model.User{}))
			for _, user := range []model.User{
				{Id: 931001, Username: "customer-a", AffCode: "a"},
				{Id: 931002, Username: "customer-b", AffCode: "b"},
			} {
				require.NoError(t, db.Create(&user).Error)
			}
			router := gin.New()
			router.GET("/query/:user", func(c *gin.Context) {
				id, _ := strconv.Atoi(c.Param("user"))
				c.Set("id", id)
			}, LogQueryRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/query/931001", "192.0.2.1:1234").Code)
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/query/931001", "192.0.2.2:1234").Code)
			limited := performRateLimitRequest(router, "/query/931001", "192.0.2.3:1234")
			assert.Equal(t, http.StatusTooManyRequests, limited.Code)
			assert.Equal(t, "60", limited.Header().Get("Retry-After"))
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/query/931002", "192.0.2.1:1234").Code)
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", 931001).Update("log_query_rate_limit", 10).Error)
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(router, "/query/931001", "192.0.2.1:1234").Code)
			require.NoError(t, db.Model(&model.User{}).Where("id = ?", 931001).Update("log_query_rate_limit", 0).Error)
			assert.Equal(t, http.StatusTooManyRequests, performRateLimitRequest(router, "/query/931001", "192.0.2.1:1234").Code)
			assert.Equal(t, http.StatusUnauthorized, performRateLimitRequest(router, "/query/0", "192.0.2.1:1234").Code)
			// Lookup failures must not grant an unlimited allowance.
			require.NoError(t, connection.Close())
			assert.Equal(t, http.StatusInternalServerError, performRateLimitRequest(router, "/query/931001", "192.0.2.1:1234").Code)
			common.LogQueryRateLimitEnable = false
			disabled := gin.New()
			disabled.GET("/query", LogQueryRateLimit(), func(c *gin.Context) { c.Status(http.StatusNoContent) })
			assert.Equal(t, http.StatusNoContent, performRateLimitRequest(disabled, "/query", "192.0.2.1:1234").Code)
		})
	}
}

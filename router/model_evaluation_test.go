package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluationRoutesRequireLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRedis, previousLimit := common.RedisEnabled, common.GlobalApiRateLimitEnable
	common.RedisEnabled, common.GlobalApiRateLimitEnable = false, false
	t.Cleanup(func() {
		common.RedisEnabled, common.GlobalApiRateLimitEnable = previousRedis, previousLimit
	})
	router := gin.New()
	SetApiRouter(router)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/evaluations/questions"},
		{"GET", "/api/evaluations/models?token_id=1"},
		{"GET", "/api/evaluations/runs"},
		{"GET", "/api/evaluations/runs/1"},
		{"POST", "/api/evaluations/runs"},
		{"POST", "/api/evaluations/runs/1/rating"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(route.method, route.path, nil))
			require.Equal(t, http.StatusUnauthorized, response.Code)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
			assert.False(t, result.Success)
		})
	}
}

package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func evaluationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	previousDB, previousLogs, previousHandler := model.DB, model.LOG_DB, evaluationRelay
	previousRedis, previousLogsEnabled := common.RedisEnabled, common.LogConsumeEnabled
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	initModelListColumnNames(t)
	common.RedisEnabled, common.LogConsumeEnabled = false, true
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.ModelEvaluation{}, &model.Log{}))
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB, evaluationRelay = previousDB, previousLogs, previousHandler
		common.RedisEnabled, common.LogConsumeEnabled = previousRedis, previousLogsEnabled
		common.SetDatabaseTypes(previousMainType, previousLogType)
		connection, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, connection.Close())
	})
	return db
}

func TestEvaluationPrivateHistoryAndRating(t *testing.T) {
	db := evaluationTestDB(t)
	run := model.ModelEvaluation{UserID: 7, TokenID: 3, TokenName: "removed key", Status: model.EvaluationPassed, Kind: "text", Prompt: "private question", Answer: "private answer", CreatedAt: time.Now().Unix()}
	require.NoError(t, db.Create(&run).Error)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 8) })
	router.GET("/runs/:id", GetModelEvaluationReport)
	router.GET("/runs", ListModelEvaluationHistory)
	router.POST("/runs/:id/rating", RateModelEvaluation)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/runs/%d", run.ID), nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.NotContains(t, response.Body.String(), "private")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/runs", nil))
	assert.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), "removed key")
	ok, err := model.RateModelEvaluation(run.ID, 8, model.EvaluationFailed)
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = model.RateModelEvaluation(run.ID, 7, model.EvaluationFailed)
	require.NoError(t, err)
	assert.False(t, ok, "text grades cannot be overwritten by manual ratings")
	owned, err := model.GetModelEvaluation(run.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, "removed key", owned.TokenName, "history does not depend on a live token")
}

func TestEvaluationRejectsForeignKeyAndUnconfirmedCharge(t *testing.T) {
	db := evaluationTestDB(t)
	foreign := model.Token{UserId: 8, Name: "other user", Key: "foreign-key", Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, db.Create(&foreign).Error)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 7) })
	router.POST("/runs", StartModelEvaluation)
	for _, test := range []struct {
		name      string
		confirmed bool
		status    int
	}{{"not confirmed", false, 400}, {"foreign key", true, 404}} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := common.Marshal(map[string]any{"token_id": foreign.Id, "model": "test-model", "question_id": "logic", "protocol": "chat", "confirm_charge": test.confirmed})
			require.NoError(t, err)
			response := httptest.NewRecorder()
			request := httptest.NewRequest("POST", "/runs", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			assert.Equal(t, test.status, response.Code)
			assert.NotContains(t, response.Body.String(), foreign.Key)
		})
	}
	var count int64
	require.NoError(t, db.Model(&model.ModelEvaluation{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEvaluationRelaysSelectedKeyAndMatchesItsCharge(t *testing.T) {
	db := evaluationTestDB(t)
	token := model.Token{UserId: 7, Key: "private-evaluation-key", Name: "My Key", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, db.Create(&token).Error)
	run := &model.ModelEvaluation{UserID: 7, TokenID: token.Id, ModelName: "test-model", Protocol: "chat", Kind: "text", Prompt: "question", Expected: `{"answer":22}`, MaxOutput: 4096, Status: model.EvaluationRunning}
	require.NoError(t, db.Create(run).Error)
	var calls int
	SetEvaluationRelay(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer sk-"+token.Key, r.Header.Get("Authorization"))
		assert.Equal(t, "203.0.113.10:0", r.RemoteAddr)
		assert.Empty(t, r.Header.Get("Cookie"))
		assert.Equal(t, true, r.Context().Value(evaluationRequestKey{}))
		var payload map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		assert.Equal(t, "test-model", payload["model"])
		assert.Equal(t, false, payload["stream"])
		assert.EqualValues(t, 4096, payload["max_completion_tokens"])
		w.Header().Set(common.RequestIdKey, "eval-owned-request")
		require.NoError(t, db.Create(&model.Log{UserId: 7, TokenId: token.Id, RequestId: "eval-owned-request", Type: model.LogTypeConsume, Quota: 321, PromptTokens: 10, CompletionTokens: 5}).Error)
		require.NoError(t, db.Create(&model.Log{UserId: 7, TokenId: token.Id, RequestId: "unrelated-request", Type: model.LogTypeConsume, Quota: 999}).Error)
		_, err := w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":22}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
		require.NoError(t, err)
	}))
	executeModelEvaluation(context.Background(), run, &token, "203.0.113.10")
	assert.Equal(t, 1, calls)
	saved, err := model.GetModelEvaluation(run.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, model.EvaluationPassed, saved.Status)
	require.NotNil(t, saved.Quota)
	assert.Equal(t, 321, *saved.Quota)
	encoded, err := common.Marshal(saved)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), token.Key)
}

func TestEvaluationResponseClassification(t *testing.T) {
	for _, test := range []struct {
		name, protocol, body string
		status               model.EvaluationStatus
		answer               string
		hasError             bool
	}{
		{"chat truncated", "chat", `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`, model.EvaluationTruncated, "partial", false},
		{"responses truncated", "responses", `{"status":"incomplete","output":[{"type":"message","content":[{"type":"output_text","text":"partial"}]}]}`, model.EvaluationTruncated, "partial", false},
		{"responses completed", "responses", `{"status":"completed","output":[{"type":"reasoning","content":[{"type":"output_text","text":"ignore"}]},{"type":"message","content":[{"type":"output_text","text":"answer"}]}],"usage":{"input_tokens":11,"output_tokens":7}}`, "", "answer", false},
		{"malformed", "chat", `not json`, "", "", true},
		{"refusal", "chat", `{"choices":[{"message":{"refusal":"no"},"finish_reason":"stop"}]}`, "", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			run := &model.ModelEvaluation{Protocol: test.protocol}
			err := readEvaluationAnswer(run, []byte(test.body))
			assert.Equal(t, test.hasError, err != nil)
			assert.Equal(t, test.status, run.Status)
			assert.Equal(t, test.answer, run.Answer)
		})
	}
}

func TestEvaluationNoRetryEvenForChannelErrors(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	err := types.NewError(errors.New("channel unavailable"), types.ErrorCode("channel:unavailable"))
	require.True(t, types.IsChannelError(err))
	assert.Equal(t, "retry", decideEvaluationAwareRelayRetry(c, err, 2).Action)
	c.Request = c.Request.WithContext(context.WithValue(context.Background(), evaluationRequestKey{}, true))
	// The same relay error must not initiate an extra paid attempt.
	assert.Equal(t, "stop", decideEvaluationAwareRelayRetry(c, err, 2).Action)
}

func TestEvaluationBoundedResponse(t *testing.T) {
	w := &evaluationResponse{header: make(http.Header)}
	input := []byte(strings.Repeat("x", 60*1024))
	n, err := w.Write(input)
	require.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.True(t, w.overflow)
	assert.Equal(t, 48*1024, w.body.Len())
}

func TestEvaluationUsesNormalTokenIPAndModelPermissions(t *testing.T) {
	db := evaluationTestDB(t)
	user := model.User{Id: 77, Username: "evaluation-owner", Group: "default", Status: common.UserStatusEnabled, Quota: 100000}
	require.NoError(t, db.Create(&user).Error)
	allowed := "203.0.113.10/32"
	token := model.Token{UserId: user.Id, Key: "evaluationipkey", Name: "restricted", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, AllowIps: &allowed, ModelLimitsEnabled: true, ModelLimits: "allowed-model"}
	require.NoError(t, db.Create(&token).Error)
	relay := gin.New()
	require.NoError(t, relay.SetTrustedProxies(nil))
	calls := 0
	relay.GET("/v1/models", middleware.TokenAuth(), func(c *gin.Context) {
		calls++
		assert.Equal(t, user.Id, c.GetInt("id"))
		assert.Equal(t, token.Id, c.GetInt("token_id"))
		assert.True(t, c.GetBool("token_model_limit_enabled"))
		limits, exists := c.Get("token_model_limit")
		require.True(t, exists)
		assert.Equal(t, token.GetModelLimitsMap(), limits)
		c.JSON(200, gin.H{"data": []gin.H{{"id": "allowed-model"}}})
	})
	SetEvaluationRelay(relay)
	response, err := callEvaluationRelay(context.Background(), &token, "203.0.113.10", "GET", "/v1/models", nil)
	require.NoError(t, err)
	assert.Equal(t, 200, response.status)
	response, err = callEvaluationRelay(context.Background(), &token, "203.0.113.11", "GET", "/v1/models", nil)
	require.NoError(t, err)
	assert.Equal(t, 403, response.status)
	assert.Equal(t, 1, calls, "IP restrictions reject the second request before model access")
}

func TestEvaluationActiveRunAndArtworkRating(t *testing.T) {
	db := evaluationTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "owner"}).Error)
	run := &model.ModelEvaluation{UserID: 7, Status: model.EvaluationRunning, Kind: "artwork", CreatedAt: time.Now().Unix()}
	require.NoError(t, model.CreateModelEvaluation(run))
	err := model.CreateModelEvaluation(&model.ModelEvaluation{UserID: 7, Status: model.EvaluationRunning})
	assert.ErrorIs(t, err, model.ErrEvaluationActive)
	ok, err := model.RateModelEvaluation(run.ID, 7, model.EvaluationPassed)
	require.NoError(t, err)
	assert.False(t, ok, "unfinished artwork cannot be rated")
	run.Status, run.Artwork = model.EvaluationReview, "<html><svg></svg></html>"
	require.NoError(t, model.CompleteModelEvaluation(run))
	ok, err = model.RateModelEvaluation(run.ID, 8, model.EvaluationPassed)
	require.NoError(t, err)
	assert.False(t, ok, "another user cannot rate this artwork")
	ok, err = model.RateModelEvaluation(run.ID, 7, model.EvaluationPartial)
	require.NoError(t, err)
	assert.True(t, ok)
	owned, err := model.GetModelEvaluation(run.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, model.EvaluationPartial, owned.Status)
	require.NoError(t, model.CreateModelEvaluation(&model.ModelEvaluation{UserID: 7, Status: model.EvaluationRunning, CreatedAt: time.Now().Add(-7 * time.Minute).Unix()}))
	require.NoError(t, model.ExpireUserEvaluations(7))
	require.NoError(t, model.CreateModelEvaluation(&model.ModelEvaluation{UserID: 7, Status: model.EvaluationRunning, CreatedAt: time.Now().Unix()}))
}

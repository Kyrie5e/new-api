package controller

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvaluationStartRunsOnceAndSurvivesRequestCancellation(t *testing.T) {
	db := evaluationTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "owner"}).Error)
	token := model.Token{UserId: 7, Key: "asyncownertoken", Name: "Owner key", Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, db.Create(&token).Error)
	resume := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
	})
	SetEvaluationRelay(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-resume
		assert.NoError(t, r.Context().Err(), "closing the start request must not cancel the background test")
		assert.Equal(t, "Bearer sk-"+token.Key, r.Header.Get("Authorization"))
		_, err := w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":0}"},"finish_reason":"stop"}]}`))
		assert.NoError(t, err)
	}))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 7) })
	router.POST("/runs", StartModelEvaluation)
	router.GET("/runs/:id", GetModelEvaluationReport)
	payload, err := common.Marshal(map[string]any{"token_id": token.Id, "model": "test-model", "question_id": "logic", "protocol": "chat", "confirm_charge": true})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest("POST", "/runs", bytes.NewReader(payload)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusAccepted, response.Code)
	var started struct {
		Success bool `json:"success"`
		Data    struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &started))
	require.True(t, started.Success)
	require.Positive(t, started.Data.ID)
	cancel()
	request = httptest.NewRequest("POST", "/runs", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	assert.Equal(t, http.StatusConflict, response.Code)
	close(resume)
	// Poll the public report contract because execution is asynchronous.
	require.EventuallyWithT(t, func(check *assert.CollectT) {
		response = httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/runs/%d", started.Data.ID), nil))
		var report struct {
			Success bool                  `json:"success"`
			Data    model.ModelEvaluation `json:"data"`
		}
		if assert.NoError(check, common.Unmarshal(response.Body.Bytes(), &report)) {
			assert.True(check, report.Success)
			assert.Equal(check, model.EvaluationFailed, report.Data.Status)
			assert.Equal(check, `{"answer":0}`, report.Data.Answer)
		}
	}, 3*time.Second, 10*time.Millisecond)
	assert.NotContains(t, response.Body.String(), token.Key)
	var count int64
	require.NoError(t, db.Model(&model.ModelEvaluation{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "duplicate starts must not create another run")
}

func TestEvaluationRejectsUnavailableKeysAndForeignRepeat(t *testing.T) {
	db := evaluationTestDB(t)
	foreign := model.ModelEvaluation{UserID: 8, QuestionID: "logic", Prompt: "foreign prompt", Expected: "foreign answer", Status: model.EvaluationPassed}
	require.NoError(t, db.Create(&foreign).Error)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 7) })
	router.POST("/runs", StartModelEvaluation)
	calls := 0
	SetEvaluationRelay(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	for i, test := range []struct {
		name    string
		status  int
		expiry  int64
		quota   int
		deleted bool
		source  int
		want    int
	}{
		{"disabled", common.TokenStatusDisabled, -1, 100, false, 0, 400},
		{"expired", common.TokenStatusEnabled, time.Now().Add(-time.Hour).Unix(), 100, false, 0, 400},
		{"exhausted", common.TokenStatusEnabled, -1, 0, false, 0, 400},
		{"deleted", common.TokenStatusEnabled, -1, 100, true, 0, 404},
		{"foreign repeat", common.TokenStatusEnabled, -1, 100, false, foreign.ID, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := model.Token{UserId: 7, Key: fmt.Sprintf("startkey%d", i), Status: test.status, ExpiredTime: test.expiry, RemainQuota: test.quota}
			require.NoError(t, db.Create(&token).Error)
			if test.deleted {
				require.NoError(t, db.Delete(&token).Error)
			}
			payload, err := common.Marshal(map[string]any{"token_id": token.Id, "model": "test-model", "question_id": "logic", "protocol": "chat", "confirm_charge": true, "source_id": test.source})
			require.NoError(t, err)
			request := httptest.NewRequest("POST", "/runs", bytes.NewReader(payload))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			assert.Equal(t, test.want, response.Code)
			assert.NotContains(t, response.Body.String(), token.Key)
			assert.NotContains(t, response.Body.String(), foreign.Prompt)
		})
	}
	assert.Zero(t, calls)
	var count int64
	require.NoError(t, db.Model(&model.ModelEvaluation{}).Where("user_id = ?", 7).Count(&count).Error)
	assert.Zero(t, count, "rejected starts must not create a run")
}

func TestEvaluationWorkerReloadsRevokedKey(t *testing.T) {
	for _, action := range []string{"deleted", "disabled", "expired", "exhausted"} {
		t.Run(action, func(t *testing.T) {
			db := evaluationTestDB(t)
			token := model.Token{UserId: 7, Key: "workertoken", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}
			require.NoError(t, db.Create(&token).Error)
			run := &model.ModelEvaluation{UserID: 7, TokenID: token.Id, Protocol: "chat", Status: model.EvaluationRunning}
			require.NoError(t, db.Create(run).Error)
			switch action {
			case "deleted":
				require.NoError(t, db.Delete(&token).Error)
			case "disabled":
				require.NoError(t, db.Model(&token).Update("status", common.TokenStatusDisabled).Error)
			case "expired":
				require.NoError(t, db.Model(&token).Update("expired_time", time.Now().Add(-time.Hour).Unix()).Error)
			case "exhausted":
				require.NoError(t, db.Model(&token).Update("remain_quota", 0).Error)
			}
			relay := gin.New()
			calls := 0
			relay.POST("/v1/chat/completions", middleware.TokenAuth(), func(c *gin.Context) { calls++ })
			SetEvaluationRelay(relay)
			executeModelEvaluation(context.Background(), run, &token, "203.0.113.10")
			saved, err := model.GetModelEvaluation(run.ID, 7)
			require.NoError(t, err)
			assert.Equal(t, model.EvaluationError, saved.Status)
			assert.NotEmpty(t, saved.Error)
			assert.Positive(t, saved.CompletedAt)
			assert.Zero(t, calls, "revocation before worker execution must prevent model access")
		})
	}
}

func TestEvaluationWorkerPersistsTerminalStates(t *testing.T) {
	for _, test := range []struct {
		name     string
		protocol string
		body     string
		status   model.EvaluationStatus
	}{
		{"upstream error", "chat", "secret upstream credentials", model.EvaluationError},
		{"panic", "chat", "", model.EvaluationError},
		{"deadline", "chat", "", model.EvaluationError},
		{"malformed", "chat", "not json", model.EvaluationError},
		{"empty answer", "chat", `{"choices":[{"message":{"content":""},"finish_reason":"stop"}]}`, model.EvaluationError},
		{"length limit", "chat", `{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`, model.EvaluationTruncated},
		{"size limit", "chat", strings.Repeat("x", 49*1024), model.EvaluationTruncated},
		{"responses complete", "responses", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"answer\":22}"}]}]}`, model.EvaluationPassed},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := evaluationTestDB(t)
			token := model.Token{UserId: 7, Key: "terminaltoken"}
			require.NoError(t, db.Create(&token).Error)
			run := &model.ModelEvaluation{UserID: 7, TokenID: token.Id, Protocol: test.protocol, Kind: "text", Expected: `{"answer":22}`, Status: model.EvaluationRunning}
			require.NoError(t, db.Create(run).Error)
			calls := 0
			SetEvaluationRelay(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch test.name {
				case "panic":
					panic("secret upstream credentials")
				case "deadline":
					assert.ErrorIs(t, r.Context().Err(), context.DeadlineExceeded)
					w.WriteHeader(http.StatusGatewayTimeout)
				case "upstream error":
					w.WriteHeader(http.StatusBadGateway)
				}
				_, err := w.Write([]byte(test.body))
				assert.NoError(t, err)
			}))
			ctx := context.Background()
			if test.name == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Unix(1, 0))
				defer cancel()
			}
			executeModelEvaluation(ctx, run, &token, "203.0.113.10")
			saved, err := model.GetModelEvaluation(run.ID, 7)
			require.NoError(t, err)
			assert.Equal(t, test.status, saved.Status)
			assert.Positive(t, saved.CompletedAt)
			assert.Equal(t, 1, calls, "one test must make at most one relay attempt")
			assert.NotContains(t, saved.Error, "secret upstream credentials")
			if test.name == "deadline" {
				assert.Contains(t, saved.Error, "timed out")
			}
		})
	}
}

func TestEvaluationHistoryExpiresOnlyOwnersStaleRuns(t *testing.T) {
	db := evaluationTestDB(t)
	now := time.Now()
	stale := model.ModelEvaluation{UserID: 7, Status: model.EvaluationRunning, CreatedAt: now.Add(-7 * time.Minute).Unix()}
	fresh := model.ModelEvaluation{UserID: 7, Status: model.EvaluationRunning, CreatedAt: now.Unix()}
	foreign := model.ModelEvaluation{UserID: 8, Status: model.EvaluationRunning, CreatedAt: stale.CreatedAt}
	for _, run := range []*model.ModelEvaluation{&stale, &fresh, &foreign} {
		require.NoError(t, db.Create(run).Error)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 7) })
	router.GET("/runs", ListModelEvaluationHistory)
	router.GET("/runs/:id", GetModelEvaluationReport)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/runs", nil))
	require.Equal(t, http.StatusOK, response.Code)
	var history struct {
		Success bool `json:"success"`
		Data    struct {
			Total int                     `json:"total"`
			Items []model.ModelEvaluation `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &history))
	require.True(t, history.Success)
	assert.Equal(t, 2, history.Data.Total)
	require.Len(t, history.Data.Items, 2)
	assert.Equal(t, fresh.ID, history.Data.Items[0].ID)
	assert.Equal(t, model.EvaluationRunning, history.Data.Items[0].Status)
	assert.Equal(t, stale.ID, history.Data.Items[1].ID)
	assert.Equal(t, model.EvaluationError, history.Data.Items[1].Status)
	other, err := model.GetModelEvaluation(foreign.ID, 8)
	require.NoError(t, err)
	assert.Equal(t, model.EvaluationRunning, other.Status)
	// A late worker cannot overwrite the interruption shown in history.
	stale.Status, stale.Answer = model.EvaluationPassed, "late answer"
	require.NoError(t, model.CompleteModelEvaluation(&stale))
	saved, err := model.GetModelEvaluation(stale.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, model.EvaluationError, saved.Status)
	assert.Empty(t, saved.Answer)
	// Detail views must also recover stale records without a history-list visit.
	require.NoError(t, db.Model(&fresh).Update("created_at", stale.CreatedAt).Error)
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/runs/%d", fresh.ID), nil))
	require.Equal(t, http.StatusOK, response.Code)
	saved, err = model.GetModelEvaluation(fresh.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, model.EvaluationError, saved.Status)
	_, err = model.GetModelEvaluation(fresh.ID, 8)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestEvaluationModelsRequireOwnedUsableKey(t *testing.T) {
	db := evaluationTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 7, Username: "owner", Group: "default", Status: common.UserStatusEnabled}).Error)
	relay := gin.New()
	calls := 0
	relay.GET("/v1/models", middleware.TokenAuth(), func(c *gin.Context) {
		calls++
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{{"id": "allowed-model"}}})
	})
	SetEvaluationRelay(relay)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("id", 7) })
	router.GET("/models", GetEvaluationModels)
	for i, test := range []struct {
		name   string
		userID int
		status int
		expiry int64
		quota  int
		want   int
	}{
		{"own key", 7, common.TokenStatusEnabled, -1, 100, 200},
		{"foreign key", 8, common.TokenStatusEnabled, -1, 100, 404},
		{"disabled", 7, common.TokenStatusDisabled, -1, 100, 400},
		{"expired", 7, common.TokenStatusEnabled, time.Now().Add(-time.Hour).Unix(), 100, 400},
		{"exhausted", 7, common.TokenStatusEnabled, -1, 0, 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			token := model.Token{UserId: test.userID, Key: fmt.Sprintf("modelkey%d", i), Name: test.name, Status: test.status, ExpiredTime: test.expiry, RemainQuota: test.quota}
			require.NoError(t, db.Create(&token).Error)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("GET", fmt.Sprintf("/models?token_id=%d", token.Id), nil))
			assert.Equal(t, test.want, response.Code)
			assert.NotContains(t, response.Body.String(), token.Key)
			if test.want == http.StatusOK {
				assert.JSONEq(t, `{"success":true,"message":"","data":["allowed-model"]}`, response.Body.String())
			} else {
				assert.NotContains(t, response.Body.String(), "allowed-model")
			}
		})
	}
	assert.Equal(t, 1, calls, "only the owner's usable key reaches the model handler")
}

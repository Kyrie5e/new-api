package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluationConcurrencyAllowsTwentyUsersAndOneRunPerUser(t *testing.T) {
	db := evaluationTestDB(t)
	connection, err := db.DB()
	require.NoError(t, err)
	connection.SetMaxOpenConns(1)
	tokens := make([]model.Token, 21)
	resume := make([]chan struct{}, 21)
	byKey := make(map[string]chan struct{}, 21)
	for i := range tokens {
		userID := i + 1
		require.NoError(t, db.Create(&model.User{Id: userID, Username: fmt.Sprintf("owner%d", userID), AffCode: fmt.Sprintf("aff%d", userID)}).Error)
		tokens[i] = model.Token{UserId: userID, Key: fmt.Sprintf("concurrencykey%d", userID), Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
		require.NoError(t, db.Create(&tokens[i]).Error)
		resume[i] = make(chan struct{})
		byKey["Bearer sk-"+tokens[i].Key] = resume[i]
	}
	// Unblock and drain workers before the temporary database is closed.
	t.Cleanup(func() {
		for _, gate := range resume {
			select {
			case <-gate:
			default:
				close(gate)
			}
		}
		assert.Eventually(t, func() bool { return len(evaluationSlots) == 0 }, 3*time.Second, 10*time.Millisecond)
	})
	SetEvaluationRelay(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gate, exists := byKey[r.Header.Get("Authorization")]
		if !exists {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		<-gate
		_, err := w.Write([]byte(`{"choices":[{"message":{"content":"{\"answer\":0}"},"finish_reason":"stop"}]}`))
		assert.NoError(t, err)
	}))
	start := func(token model.Token) *httptest.ResponseRecorder {
		t.Helper()
		payload, err := common.Marshal(map[string]any{"token_id": token.Id, "model": "test-model", "question_id": "logic", "protocol": "chat", "confirm_charge": true})
		require.NoError(t, err)
		response := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(response)
		c.Set("id", token.UserId)
		c.Request = httptest.NewRequest("POST", "/api/evaluations/runs", bytes.NewReader(payload))
		c.Request.Header.Set("Content-Type", "application/json")
		StartModelEvaluation(c)
		return response
	}
	for i := 0; i < 20; i++ {
		response := start(tokens[i])
		require.Equal(t, http.StatusAccepted, response.Code, "user %d: %s", i+1, response.Body.String())
	}
	// A second key must not let the same account start another active test.
	secondKey := model.Token{UserId: tokens[0].UserId, Key: "secondownerkey", Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, db.Create(&secondKey).Error)
	response := start(secondKey)
	assert.Equal(t, http.StatusConflict, response.Code)
	response = start(tokens[20])
	assert.Equal(t, http.StatusTooManyRequests, response.Code)
	var count int64
	require.NoError(t, db.Model(&model.ModelEvaluation{}).Count(&count).Error)
	assert.EqualValues(t, 20, count, "capacity rejection must not create a queued or billable run")
	// The same account can test again after completion: this is not a lifetime limit.
	close(resume[0])
	require.EventuallyWithT(t, func(check *assert.CollectT) {
		response = start(tokens[0])
		assert.Equal(check, http.StatusAccepted, response.Code)
	}, 3*time.Second, 10*time.Millisecond)
	// The restarted test finishes immediately, freeing the slot for another user.
	require.EventuallyWithT(t, func(check *assert.CollectT) {
		response = start(tokens[20])
		assert.Equal(check, http.StatusAccepted, response.Code)
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, db.Model(&model.ModelEvaluation{}).Count(&count).Error)
	assert.EqualValues(t, 22, count)
}

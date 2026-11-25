package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type evaluationRequestKey struct{}

var evaluationRelay http.Handler
var evaluationSlots = make(chan struct{}, 20)
var evaluationUsers sync.Map

// Dispatch only to this application's router. No user-supplied URL or key is
// accepted; TokenAuth and the normal relay perform all permission and billing checks.
func SetEvaluationRelay(handler http.Handler) { evaluationRelay = handler }

func decideEvaluationAwareRelayRetry(c *gin.Context, err *types.NewAPIError, remaining int) service.PolicyDecision {
	if c.Request.Context().Value(evaluationRequestKey{}) == true {
		return service.PolicyDecision{Action: "stop", Reason: "evaluation_single_attempt", Source: "system"}
	}
	return service.DecideRelayRetry(c, err, remaining)
}

type evaluationResponse struct {
	header   http.Header
	status   int
	body     bytes.Buffer
	overflow bool
}

func (w *evaluationResponse) Header() http.Header { return w.header }
func (w *evaluationResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *evaluationResponse) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	remaining := 48*1024 - w.body.Len()
	if len(p) > remaining {
		w.overflow = true
	}
	if remaining > 0 {
		_, _ = w.body.Write(p[:min(len(p), remaining)])
	}
	return len(p), nil
}
func (w *evaluationResponse) Flush() { w.WriteHeader(http.StatusOK) }

func callEvaluationRelay(ctx context.Context, token *model.Token, ip, method, path string, payload any) (*evaluationResponse, error) {
	if evaluationRelay == nil {
		return nil, errors.New("Evaluation service unavailable")
	}
	var body []byte
	var err error
	if payload != nil {
		body, err = common.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	request, err := http.NewRequestWithContext(context.WithValue(ctx, evaluationRequestKey{}, true), method, path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.RemoteAddr = net.JoinHostPort(ip, "0")
	request.Header.Set("Authorization", "Bearer sk-"+strings.TrimPrefix(token.Key, "sk-"))
	request.Header.Set("Content-Type", "application/json")
	response := &evaluationResponse{header: make(http.Header)}
	evaluationRelay.ServeHTTP(response, request)
	return response, nil
}

func GetEvaluationQuestions(c *gin.Context) { common.ApiSuccess(c, service.EvaluationQuestions) }

func GetEvaluationModels(c *gin.Context) {
	id, err := strconv.Atoi(c.Query("token_id"))
	if err != nil || id <= 0 {
		c.JSON(400, gin.H{"success": false, "message": "Select an API key"})
		return
	}
	token, err := model.GetTokenByIds(id, c.GetInt("id"))
	if err != nil {
		c.JSON(404, gin.H{"success": false, "message": "API key not found"})
		return
	}
	response, err := callEvaluationRelay(c.Request.Context(), token, c.ClientIP(), http.MethodGet, "/v1/models", nil)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if response.status != 200 || common.Unmarshal(response.body.Bytes(), &result) != nil || result.Data == nil {
		c.JSON(400, gin.H{"success": false, "message": "Cannot load models; check key status, group and IP restrictions"})
		return
	}
	models := make([]string, 0, len(result.Data))
	for _, item := range result.Data {
		models = append(models, item.ID)
	}
	common.ApiSuccess(c, models)
}

func StartModelEvaluation(c *gin.Context) {
	var input struct {
		TokenID       int    `json:"token_id"`
		Model         string `json:"model"`
		QuestionID    string `json:"question_id"`
		Protocol      string `json:"protocol"`
		Effort        string `json:"effort"`
		ConfirmCharge bool   `json:"confirm_charge"`
		SourceID      int    `json:"source_id"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if c.ShouldBindJSON(&input) != nil || !input.ConfirmCharge || input.TokenID <= 0 || input.Model == "" || len(input.Model) > 255 {
		c.JSON(400, gin.H{"success": false, "message": "Invalid evaluation request or charge not confirmed"})
		return
	}
	if input.Protocol != "chat" && input.Protocol != "responses" {
		c.JSON(400, gin.H{"success": false, "message": "Unsupported API protocol"})
		return
	}
	switch input.Effort {
	case "", "low", "medium", "high":
	default:
		c.JSON(400, gin.H{"success": false, "message": "Invalid reasoning effort"})
		return
	}
	question, prompt, expected, err := service.BuildEvaluationQuestion(input.QuestionID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userID := c.GetInt("id")
	token, err := model.GetTokenByIds(input.TokenID, userID)
	if err != nil {
		c.JSON(404, gin.H{"success": false, "message": "API key not found"})
		return
	}
	if token.Status != common.TokenStatusEnabled || (token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix()) || (!token.UnlimitedQuota && token.RemainQuota <= 0) {
		c.JSON(400, gin.H{"success": false, "message": "API key is disabled, expired or exhausted"})
		return
	}
	if _, loaded := evaluationUsers.LoadOrStore(userID, true); loaded {
		c.JSON(409, gin.H{"success": false, "message": model.ErrEvaluationActive.Error()})
		return
	}
	select {
	case evaluationSlots <- struct{}{}:
	default:
		evaluationUsers.Delete(userID)
		c.JSON(429, gin.H{"success": false, "message": "Evaluation capacity reached; try again later"})
		return
	}
	release := func() { <-evaluationSlots; evaluationUsers.Delete(userID) }
	if err = model.ExpireUserEvaluations(userID); err != nil {
		release()
		common.ApiError(c, err)
		return
	}
	run := &model.ModelEvaluation{UserID: userID, TokenID: token.Id, TokenName: token.Name, TokenMasked: token.GetMaskedKey(), TokenGroup: token.Group, ModelName: input.Model, Protocol: input.Protocol, Effort: input.Effort, QuestionID: question.ID, QuestionName: question.Name, QuestionVersion: question.Version, Kind: question.Kind, MaxOutput: question.MaxOutput, Prompt: prompt, Expected: expected, Status: model.EvaluationRunning, CreatedAt: time.Now().Unix()}
	if input.SourceID > 0 {
		source, sourceErr := model.GetModelEvaluation(input.SourceID, userID)
		if sourceErr != nil || source.QuestionID != input.QuestionID {
			release()
			c.JSON(400, gin.H{"success": false, "message": "Source evaluation unavailable"})
			return
		}
		run.Prompt, run.Expected, run.QuestionVersion = source.Prompt, source.Expected, source.QuestionVersion
	}
	if err = model.CreateModelEvaluation(run); err != nil {
		release()
		common.ApiError(c, err)
		return
	}
	ip := c.ClientIP()
	// Do not share the response object with the worker: it updates the run.
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{"id": run.ID}})
	go func() { defer release(); executeModelEvaluation(context.Background(), run, token, ip) }()
}

func executeModelEvaluation(ctx context.Context, run *model.ModelEvaluation, token *model.Token, ip string) {
	started := time.Now()
	defer func() {
		if recover() != nil {
			run.Status = model.EvaluationError
			run.Error = "Evaluation interrupted; review usage logs before retrying"
		}
		run.CompletedAt, run.DurationMs = time.Now().Unix(), time.Since(started).Milliseconds()
		model.AttachEvaluationUsage(run)
		if err := model.CompleteModelEvaluation(run); err != nil {
			common.SysError("failed to save model evaluation result: " + err.Error())
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	// Reload so deletion, revocation and changed permissions are respected.
	currentToken, err := model.GetTokenByIds(token.Id, run.UserID)
	if err != nil {
		run.Status = model.EvaluationError
		run.Error = "API key no longer available"
		return
	}
	path := "/v1/chat/completions"
	payload := map[string]any{"model": run.ModelName, "stream": false, "messages": []map[string]string{{"role": "user", "content": run.Prompt}}, "max_completion_tokens": run.MaxOutput}
	if run.Protocol == "responses" {
		path = "/v1/responses"
		payload = map[string]any{"model": run.ModelName, "stream": false, "input": run.Prompt, "max_output_tokens": run.MaxOutput, "store": false}
		if run.Effort != "" {
			payload["reasoning"] = map[string]string{"effort": run.Effort}
		}
	} else if run.Effort != "" {
		payload["reasoning_effort"] = run.Effort
	}
	response, err := callEvaluationRelay(ctx, currentToken, ip, http.MethodPost, path, payload)
	if err != nil {
		run.Status = model.EvaluationError
		run.Error = "Evaluation request failed"
		return
	}
	run.RequestID = response.Header().Get(common.RequestIdKey)
	if ctx.Err() != nil {
		run.Status = model.EvaluationError
		run.Error = "Evaluation timed out; review usage logs before retrying"
		return
	}
	if response.overflow {
		run.Status = model.EvaluationTruncated
		run.Error = "Response exceeded the evaluation size limit"
		return
	}
	if response.status < 200 || response.status >= 300 {
		run.Status = model.EvaluationError
		// Avoid persisting upstream error text which may contain channel credentials.
		run.Error = fmt.Sprintf("Model request failed (HTTP %d); see usage logs with the request ID", response.status)
		return
	}
	if err = readEvaluationAnswer(run, response.body.Bytes()); err != nil {
		run.Status = model.EvaluationError
		run.Error = err.Error()
		return
	}
	if run.Status == model.EvaluationTruncated {
		return
	}
	if strings.TrimSpace(run.Answer) == "" {
		run.Status = model.EvaluationError
		run.Error = "The model returned no text answer"
		return
	}
	if run.Kind == "artwork" {
		run.Artwork = service.EvaluationArtwork(run.Answer)
		if run.Artwork == "" {
			run.Status = model.EvaluationFailed
			return
		}
		run.Status = model.EvaluationReview
		return
	}
	run.Status = model.EvaluationFailed
	if service.GradeEvaluationAnswer(run.Answer, run.Expected) {
		run.Status = model.EvaluationPassed
	}
}

func readEvaluationAnswer(run *model.ModelEvaluation, body []byte) error {
	var result struct {
		Status  string `json:"status"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
			Input      int `json:"input_tokens"`
			Output     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if common.Unmarshal(body, &result) != nil {
		return errors.New("Invalid model response")
	}
	run.PromptTokens, run.CompletionTokens = result.Usage.Prompt, result.Usage.Completion
	if run.Protocol == "responses" {
		run.PromptTokens, run.CompletionTokens = result.Usage.Input, result.Usage.Output
		run.FinishReason = result.Status
		for _, item := range result.Output {
			if item.Type == "message" {
				for _, content := range item.Content {
					if content.Type == "output_text" {
						run.Answer += content.Text
					}
				}
			}
		}
		if result.Status == "incomplete" {
			run.Status = model.EvaluationTruncated
			return nil
		}
		if result.Status != "completed" {
			return errors.New("Model response did not complete")
		}
		return nil
	}
	if len(result.Choices) == 0 {
		return errors.New("The model returned no choices")
	}
	run.Answer, run.FinishReason = result.Choices[0].Message.Content, result.Choices[0].FinishReason
	if run.FinishReason == "length" {
		run.Status = model.EvaluationTruncated
	}
	if run.FinishReason == "content_filter" || result.Choices[0].Message.Refusal != "" {
		return errors.New("The model refused this test")
	}
	return nil
}

func ListModelEvaluationHistory(c *gin.Context) {
	userID := c.GetInt("id")
	if err := model.ExpireUserEvaluations(userID); err != nil {
		common.ApiError(c, err)
		return
	}
	page := common.GetPageQuery(c)
	runs, total, err := model.ListModelEvaluations(userID, page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range runs {
		model.AttachEvaluationUsage(&runs[i])
	}
	page.SetTotal(int(total))
	page.SetItems(runs)
	common.ApiSuccess(c, page)
}

func GetModelEvaluationReport(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(400, gin.H{"success": false, "message": "Invalid evaluation ID"})
		return
	}
	if err := model.ExpireUserEvaluations(c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	run, err := model.GetModelEvaluation(id, c.GetInt("id"))
	if err != nil {
		c.JSON(404, gin.H{"success": false, "message": "Evaluation not found"})
		return
	}
	model.AttachEvaluationUsage(run)
	common.ApiSuccess(c, run)
}

func RateModelEvaluation(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	var input struct {
		Status model.EvaluationStatus `json:"status"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	if err != nil || c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"success": false, "message": "Invalid evaluation rating"})
		return
	}
	if input.Status != model.EvaluationPassed && input.Status != model.EvaluationPartial && input.Status != model.EvaluationFailed {
		c.JSON(400, gin.H{"success": false, "message": "Invalid evaluation rating"})
		return
	}
	ok, err := model.RateModelEvaluation(id, c.GetInt("id"), input.Status)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !ok {
		c.JSON(404, gin.H{"success": false, "message": "No completed artwork available for rating"})
		return
	}
	common.ApiSuccess(c, nil)
}

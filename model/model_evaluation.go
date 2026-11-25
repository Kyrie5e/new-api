package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type EvaluationStatus string

const (
	EvaluationRunning   EvaluationStatus = "running"
	EvaluationPassed    EvaluationStatus = "passed"
	EvaluationFailed    EvaluationStatus = "failed"
	EvaluationError     EvaluationStatus = "error"
	EvaluationTruncated EvaluationStatus = "truncated"
	EvaluationReview    EvaluationStatus = "review"
	EvaluationPartial   EvaluationStatus = "partial"
)

type ModelEvaluation struct {
	ID               int              `json:"id" gorm:"primaryKey"`
	UserID           int              `json:"-" gorm:"index:idx_evaluation_user_time,priority:1"`
	TokenID          int              `json:"token_id"`
	TokenName        string           `json:"token_name"`
	TokenMasked      string           `json:"token_masked"`
	TokenGroup       string           `json:"token_group"`
	ModelName        string           `json:"model" gorm:"type:varchar(255)"`
	Protocol         string           `json:"protocol"`
	Effort           string           `json:"effort"`
	QuestionID       string           `json:"question_id"`
	QuestionName     string           `json:"question_name"`
	QuestionVersion  string           `json:"question_version"`
	Kind             string           `json:"kind"`
	MaxOutput        int              `json:"max_output"`
	Prompt           string           `json:"prompt" gorm:"type:text"`
	Expected         string           `json:"expected" gorm:"type:text"`
	Answer           string           `json:"answer" gorm:"type:text"`
	Artwork          string           `json:"artwork" gorm:"type:text"`
	Status           EvaluationStatus `json:"status" gorm:"type:varchar(24);index"`
	Error            string           `json:"error" gorm:"type:text"`
	FinishReason     string           `json:"finish_reason"`
	RequestID        string           `json:"request_id" gorm:"type:varchar(64);index"`
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	Quota            *int             `json:"quota"`
	CreatedAt        int64            `json:"created_at" gorm:"bigint;index:idx_evaluation_user_time,priority:2"`
	CompletedAt      int64            `json:"completed_at" gorm:"bigint"`
	DurationMs       int64            `json:"duration_ms" gorm:"bigint"`
}

var ErrEvaluationActive = errors.New("You already have a running evaluation")

func ExpireUserEvaluations(userID int) error {
	return DB.Model(&ModelEvaluation{}).Where("user_id = ? AND status = ? AND created_at < ?", userID, EvaluationRunning, time.Now().Add(-6*time.Minute).Unix()).Updates(map[string]any{
		"status": EvaluationError, "error": "Evaluation interrupted or timed out; review usage logs before retrying", "completed_at": time.Now().Unix(),
	}).Error
}

func CreateModelEvaluation(run *ModelEvaluation) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id").First(&user, run.UserID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&ModelEvaluation{}).Where("user_id = ? AND status = ?", run.UserID, EvaluationRunning).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrEvaluationActive
		}
		return tx.Create(run).Error
	})
}

func CompleteModelEvaluation(run *ModelEvaluation) error {
	return DB.Model(&ModelEvaluation{}).Where("id = ? AND user_id = ? AND status = ?", run.ID, run.UserID, EvaluationRunning).Select("status", "answer", "artwork", "error", "finish_reason", "request_id", "prompt_tokens", "completion_tokens", "quota", "completed_at", "duration_ms").Updates(run).Error
}

func GetModelEvaluation(id, userID int) (*ModelEvaluation, error) {
	var run ModelEvaluation
	err := DB.Where("id = ? AND user_id = ?", id, userID).First(&run).Error
	return &run, err
}

func ListModelEvaluations(userID, offset, limit int) ([]ModelEvaluation, int64, error) {
	var total int64
	query := DB.Model(&ModelEvaluation{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	runs := make([]ModelEvaluation, 0)
	err := query.Omit("prompt", "expected", "answer", "artwork").Order("id DESC").Offset(offset).Limit(limit).Find(&runs).Error
	return runs, total, err
}

func RateModelEvaluation(id, userID int, status EvaluationStatus) (bool, error) {
	result := DB.Model(&ModelEvaluation{}).Where("id = ? AND user_id = ? AND kind = ? AND artwork <> '' AND status IN ?", id, userID, "artwork", []EvaluationStatus{EvaluationReview, EvaluationPassed, EvaluationPartial, EvaluationFailed}).Update("status", status)
	return result.RowsAffected > 0, result.Error
}

// Match billing by request ID; balance differences include unrelated requests.
func AttachEvaluationUsage(run *ModelEvaluation) {
	if run.RequestID == "" || run.Quota != nil || !common.LogConsumeEnabled {
		return
	}
	var logs []Log
	if err := LOG_DB.Select("quota", "prompt_tokens", "completion_tokens").Where("user_id = ? AND token_id = ? AND request_id = ? AND type = ?", run.UserID, run.TokenID, run.RequestID, LogTypeConsume).Find(&logs).Error; err != nil || len(logs) == 0 {
		return
	}
	quota, prompt, completion := 0, 0, 0
	for _, log := range logs {
		quota += log.Quota
		prompt += log.PromptTokens
		completion += log.CompletionTokens
	}
	run.Quota, run.PromptTokens, run.CompletionTokens = &quota, prompt, completion
}

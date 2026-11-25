package model

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvaluationHistorySurvivesDatabaseReopenAndKeyDeletion(t *testing.T) {
	previousDB := DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	path := filepath.Join(t.TempDir(), "evaluation.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	connection, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB = previousDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		require.NoError(t, connection.Close())
	})
	DB = db
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &ModelEvaluation{}))
	require.NoError(t, db.Create(&User{Id: 7, Username: "owner"}).Error)
	token := Token{UserId: 7, Key: "temporarytestkey", Name: "previous key"}
	require.NoError(t, db.Create(&token).Error)
	completed := ModelEvaluation{UserID: 7, TokenID: token.Id, TokenName: token.Name, TokenMasked: token.GetMaskedKey(), Status: EvaluationPassed, Prompt: "saved prompt", Answer: "saved answer"}
	require.NoError(t, db.Create(&completed).Error)
	interrupted := ModelEvaluation{UserID: 7, TokenID: token.Id, Status: EvaluationRunning, CreatedAt: time.Now().Add(-7 * time.Minute).Unix()}
	require.NoError(t, CreateModelEvaluation(&interrupted))
	// All deletions in this test affect only its temporary SQLite database.
	require.NoError(t, db.Delete(&token).Error)
	require.NoError(t, connection.Close())
	DB, err = gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	connection, err = DB.DB()
	require.NoError(t, err)
	// Startup migration must preserve existing reports on subsequent opens.
	require.NoError(t, DB.AutoMigrate(&User{}, &Token{}, &ModelEvaluation{}))
	saved, err := GetModelEvaluation(completed.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, completed.Prompt, saved.Prompt)
	assert.Equal(t, completed.Answer, saved.Answer)
	assert.Equal(t, completed.TokenName, saved.TokenName)
	assert.Equal(t, EvaluationPassed, saved.Status)
	_, err = GetTokenByIds(token.Id, 7)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = GetModelEvaluation(completed.ID, 8)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.NoError(t, ExpireUserEvaluations(7))
	saved, err = GetModelEvaluation(interrupted.ID, 7)
	require.NoError(t, err)
	assert.Equal(t, EvaluationError, saved.Status)
	assert.NotEmpty(t, saved.Error)
	assert.Positive(t, saved.CompletedAt)
	// Recovery unblocks the next test without retrying the interrupted request.
	require.NoError(t, CreateModelEvaluation(&ModelEvaluation{UserID: 7, Status: EvaluationRunning, CreatedAt: time.Now().Unix()}))
}

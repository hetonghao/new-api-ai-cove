package controller

import (
	"context"
	"encoding/json"
	"fmt"
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

func setupRiskRecordControllerTest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalMainType := common.MainDatabaseType()
	originalLogType := common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, db.AutoMigrate(
		&model.RiskRecord{},
		&model.RiskRecordGovernance{},
		&model.Channel{},
		&model.User{},
		&model.Token{},
	))
	t.Cleanup(func() {
		model.DB = originalDB
		common.SetDatabaseTypes(originalMainType, originalLogType)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestListRiskRecords_distinguishesMissingProviderFilterFromExplicitZero(t *testing.T) {
	// Given
	setupRiskRecordControllerTest(t)
	observedAt := time.Date(2026, time.July, 26, 1, 0, 0, 0, time.UTC)
	require.NoError(t, model.RecordRiskObservation(context.Background(), model.RiskRecordInput{
		RequestID: "provider-zero", ChannelID: 1, UserID: 1,
		Result: model.RiskRecordResultNotReviewed, Source: model.RiskRecordSourceLocal,
		ObservedAt: observedAt,
	}))
	require.NoError(t, model.RecordRiskObservation(context.Background(), model.RiskRecordInput{
		RequestID: "provider-positive", ChannelID: 1, UserID: 1,
		ProviderID: 21, ProviderName: "provider", Result: model.RiskRecordResultSafe,
		Source: model.RiskRecordSourceProvider, ObservedAt: observedAt.Add(time.Second),
	}))
	tests := []struct {
		name      string
		target    string
		wantTotal int
		wantID    string
	}{
		{name: "missing provider filter", target: "/api/risk/records?p=1&page_size=20", wantTotal: 2, wantID: "provider-positive"},
		{name: "explicit zero provider", target: "/api/risk/records?p=1&page_size=20&provider_id=0", wantTotal: 1, wantID: "provider-zero"},
		{name: "positive provider", target: "/api/risk/records?p=1&page_size=20&provider_id=21", wantTotal: 1, wantID: "provider-positive"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// When
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodGet, test.target, nil)
			ListRiskRecords(ctx)

			// Then
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					Total int                 `json:"total"`
					Items []*model.RiskRecord `json:"items"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			require.True(t, response.Success)
			assert.Equal(t, test.wantTotal, response.Data.Total)
			require.NotEmpty(t, response.Data.Items)
			assert.Equal(t, test.wantID, response.Data.Items[0].RequestID)
		})
	}
}

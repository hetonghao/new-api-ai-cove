package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestModelQualityAPIRejectsMalformedBodiesAndFilters(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		handler                  gin.HandlerFunc
	}{
		{"trailing object", "POST", "/cases", `{} {}`, CreateQualityCase},
		{"oversize", "POST", "/cases", `{"name":"` + strings.Repeat("x", qualityCaseBodyLimit) + `"}`, CreateQualityCase},
		{"bad version", "GET", "/cases/1/samples?version=oops&channel_id=7", "", GetQualitySamples},
		{"missing export channel", "GET", "/cases/1/export", "", GetQualityExport},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := gin.New()
			route := "/cases"
			if tc.handler != nil && strings.Contains(tc.path, "/samples") {
				route = "/cases/:id/samples"
			}
			if strings.Contains(tc.path, "/export") {
				route = "/cases/:id/export"
			}
			router.Handle(tc.method, route, tc.handler)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			var response map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, false, response["success"])
		})
	}
}

func setupQualityQueueTest(t *testing.T, targets []int, samples int) (*model.ModelQualityRun, time.Time) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	old := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = old; require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.ModelQualitySettings{}, &model.ModelQualityCase{}, &model.ModelQualityRevision{}, &model.ModelQualityRun{}, &model.ModelQualitySample{}, &model.ModelQualityArtifact{}, &model.ModelQualityBudget{}, &model.Channel{}, &model.SystemTask{}, &model.SystemTaskLock{}))
	now := time.Now()
	settings := model.DefaultQualitySettings()
	settings.Enabled, settings.TokenIDs = true, []int{1}
	settingsJSON, err := common.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelQualitySettings{ID: 1, Version: 1, Config: string(settingsJSON)}).Error)
	cfg := model.QualityConfig{Model: "gpt-6-astra", OutputType: "text", Mode: "channel", Protocol: "responses", TokenID: 1, Group: "default", ChannelIDs: targets, Prompt: "test fixture", MaxOutputTokens: 100, SamplesPerTarget: samples, TimeoutSeconds: 10, DailyLimit: 20}
	cfgJSON, fingerprint, err := model.QualityConfigFingerprint(cfg)
	require.NoError(t, err)
	c := model.ModelQualityCase{Name: "fixture", Enabled: true, Version: 1, EditVersion: 1, CreatedAt: now.UnixMilli()}
	require.NoError(t, db.Create(&c).Error)
	require.NoError(t, db.Create(&model.ModelQualityRevision{CaseID: c.ID, Version: 1, ConfigJSON: cfgJSON, Fingerprint: fingerprint}).Error)
	require.NoError(t, db.Create(&model.SystemTask{TaskID: "executor", Type: model.ModelQualityExecuteTask, Status: model.SystemTaskStatusRunning, LockedBy: "runner"}).Error)
	require.NoError(t, db.Create(&model.SystemTaskLock{TaskID: "executor", Type: model.ModelQualityExecuteTask, LockedBy: "runner", LockedUntil: now.Add(time.Hour).Unix()}).Error)
	run, _, err := model.CreateQualityRun(c.ID, model.QualityRunRequest{}, 1, "fixture", "manual", 0, now)
	require.NoError(t, err)
	return run, now
}

func TestModelQualityQueueLimitsConcurrentChannelsAndPersistsResults(t *testing.T) {
	run, _ := setupQualityQueueTest(t, []int{7, 8}, 2)
	var mu sync.Mutex
	active, maximum, overlap := 0, 0, false
	channelActive := map[int]int{}
	secondStarted := make(chan struct{})
	var once sync.Once
	execute := func(ctx context.Context, _ model.QualityConfig, target int) model.QualityResult {
		mu.Lock()
		active++
		maximum = max(maximum, active)
		channelActive[target]++
		if channelActive[target] > 1 {
			overlap = true
		}
		if active == 2 {
			once.Do(func() { close(secondStarted) })
		}
		mu.Unlock()
		select {
		case <-secondStarted:
		case <-ctx.Done():
		}
		mu.Lock()
		active--
		channelActive[target]--
		mu.Unlock()
		return model.QualityResult{Status: "succeeded", RequestSuccess: true, Text: []byte("fixture")}
	}
	require.NoError(t, runModelQualityQueue(context.Background(), "executor", execute))
	assert.Equal(t, 2, maximum)
	assert.False(t, overlap)
	stored, samples, err := model.GetQualityRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", stored.Status)
	require.Len(t, samples, 4)
	for _, s := range samples {
		assert.Equal(t, "succeeded", s.Status)
		assert.Equal(t, s.TargetChannelID, s.ChannelID)
		_, err := model.GetQualityArtifact(s.ID)
		require.NoError(t, err)
	}
}

func TestModelQualityQueueCancellationDoesNotReplayInFlight(t *testing.T) {
	run, _ := setupQualityQueueTest(t, []int{7}, 2)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	execute := func(ctx context.Context, _ model.QualityConfig, _ int) model.QualityResult {
		calls++
		cancel()
		<-ctx.Done()
		return model.QualityResult{Status: "cancelled", ErrorCode: "cancelled"}
	}
	require.ErrorIs(t, runModelQualityQueue(ctx, "executor", execute), context.Canceled)
	assert.Equal(t, 1, calls)
	_, samples, err := model.GetQualityRun(run.ID)
	require.NoError(t, err)
	assert.Equal(t, "interrupted", samples[0].Status)
	assert.Equal(t, "pending", samples[1].Status)
}

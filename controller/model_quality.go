package controller

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/service/qualityinspect"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	qualityCaseBodyLimit  = 128 << 10 // case writes carry the prompt/config JSON
	qualityOtherBodyLimit = 16 << 10
)

// qualityBodyLimit bounds request bodies for the model-quality API so prompt
// payloads cannot grow unboundedly.
func qualityBodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}

func qualityError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "internal error"
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		status, message = http.StatusNotFound, "not found"
	case errors.Is(err, model.ErrQualityConflict):
		status, message = http.StatusConflict, "edit conflict, refresh and retry"
	case errors.Is(err, model.ErrQualityDisabled):
		status, message = http.StatusForbidden, "quality execution is disabled or the token is not approved"
	case errors.Is(err, model.ErrQualityBudget):
		status, message = http.StatusTooManyRequests, "daily sample budget exhausted"
	case errors.Is(err, model.ErrQualityOwnership):
		status, message = http.StatusConflict, "sample ownership changed"
	default:
		status, message = http.StatusUnprocessableEntity, "invalid quality request"
	}
	common.SysError("model quality api request failed")
	c.JSON(status, gin.H{"success": false, "message": message})
}

func qualityActor(c *gin.Context) int {
	return c.GetInt("id")
}

func qualityParseID(c *gin.Context, param string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid id"})
		return 0, false
	}
	return id, true
}

func qualityBindJSON(c *gin.Context, out any) bool {
	if c.Request.Body == nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return false
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, qualityCaseBodyLimit+1))
	if err != nil || len(body) > qualityCaseBodyLimit || common.Unmarshal(body, out) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request body"})
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Capabilities
// ---------------------------------------------------------------------------

type qualityChannelCapability struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Models []string `json:"models"`
	Groups []string `json:"groups"`
}

type qualityTokenCapability struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
}

func GetQualityCapabilities(c *gin.Context) {
	var channels []*model.Channel
	if err := model.DB.Select("id", "name", "models", "group").Order("id ASC").Find(&channels).Error; err != nil {
		qualityError(c, err)
		return
	}
	channelViews := make([]qualityChannelCapability, 0, len(channels))
	for _, channel := range channels {
		groups := []string{}
		for _, g := range strings.Split(channel.Group, ",") {
			if trimmed := strings.TrimSpace(g); trimmed != "" {
				groups = append(groups, trimmed)
			}
		}
		channelViews = append(channelViews, qualityChannelCapability{
			ID:     channel.Id,
			Name:   channel.Name,
			Models: channel.GetModels(),
			Groups: groups,
		})
	}

	settings, _, err := model.GetQualitySettings()
	if err != nil {
		qualityError(c, err)
		return
	}
	tokens := make([]qualityTokenCapability, 0, len(settings.TokenIDs))
	for _, id := range settings.TokenIDs {
		token, err := model.GetTokenById(id)
		if err != nil {
			continue // revoked tokens stay invisible; IDs never leak keys
		}
		tokens = append(tokens, qualityTokenCapability{ID: token.Id, Name: token.Name, Group: token.Group})
	}

	userID := c.GetInt("id")
	role := c.GetInt("role")
	common.ApiSuccess(c, gin.H{
		"channels":      channelViews,
		"tokens":        tokens,
		"can_operate":   authz.Can(userID, role, authz.ChannelOperate),
		"can_configure": role == common.RoleRootUser,
	})
}

// ---------------------------------------------------------------------------
// Cases
// ---------------------------------------------------------------------------

func GetQualityCases(c *gin.Context) {
	views, err := model.ListQualityCases()
	if err != nil {
		qualityError(c, err)
		return
	}
	if views == nil {
		views = []model.QualityCaseView{}
	}
	common.ApiSuccess(c, views)
}

func GetQualityCase(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	view, err := model.GetQualityCase(id)
	if err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

func CreateQualityCase(c *gin.Context) {
	var input model.QualityCaseWrite
	if !qualityBindJSON(c, &input) {
		return
	}
	input.ExpectedEditVersion = 0
	view, err := model.SaveQualityCase(0, input, qualityActor(c), time.Now())
	if err != nil {
		qualityError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "message": "", "data": view})
}

func UpdateQualityCase(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	var input model.QualityCaseWrite
	if !qualityBindJSON(c, &input) {
		return
	}
	view, err := model.SaveQualityCase(id, input, qualityActor(c), time.Now())
	if err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, view)
}

type qualityCaseStateRequest struct {
	Enabled             bool  `json:"enabled"`
	Archived            bool  `json:"archived"`
	ExpectedEditVersion int64 `json:"expected_edit_version"`
}

func SetQualityCaseState(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	var req qualityCaseStateRequest
	if !qualityBindJSON(c, &req) {
		return
	}
	if err := model.SetQualityCaseState(id, req.Enabled, req.Archived, req.ExpectedEditVersion, qualityActor(c), time.Now()); err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func DeleteQualityCase(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	var expectedEditVersion int64
	if raw := c.Query("expected_edit_version"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid expected_edit_version"})
			return
		}
		expectedEditVersion = v
	}
	if err := model.DeleteQualityCase(id, expectedEditVersion); err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

type qualityRevisionView struct {
	model.ModelQualityRevision
	Config model.QualityConfig `json:"config"`
}

func GetQualityCaseRevisions(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	revisions, err := model.ListQualityRevisions(id)
	if err != nil {
		qualityError(c, err)
		return
	}
	views := make([]qualityRevisionView, 0, len(revisions))
	for _, revision := range revisions {
		var cfg model.QualityConfig
		if err := common.UnmarshalJsonStr(revision.ConfigJSON, &cfg); err != nil {
			qualityError(c, err)
			return
		}
		views = append(views, qualityRevisionView{ModelQualityRevision: revision, Config: cfg})
	}
	common.ApiSuccess(c, views)
}

// ---------------------------------------------------------------------------
// Runs
// ---------------------------------------------------------------------------

func CreateQualityRun(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	var req model.QualityRunRequest
	if !qualityBindJSON(c, &req) {
		return
	}
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" || len(key) > 128 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Idempotency-Key header is required (1-128 chars)"})
		return
	}
	run, created, err := model.CreateQualityRun(id, req, qualityActor(c), key, "manual", 0, time.Now())
	if err != nil {
		qualityError(c, err)
		return
	}
	if created {
		if _, _, err := service.EnqueueSystemTask(model.ModelQualityExecuteTask, nil); err != nil {
			common.SysError("model quality execute enqueue failed: " + err.Error())
		}
	}
	status := http.StatusAccepted
	if !created {
		status = http.StatusOK
	}
	c.JSON(status, gin.H{"success": true, "message": "", "data": gin.H{"run": run, "newly_created": created}})
}

func GetQualityRuns(c *gin.Context) {
	filter := model.QualityRunFilter{}
	if raw := c.Query("case_id"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid case_id"})
			return
		}
		filter.CaseID = id
	}
	if raw := c.Query("source"); raw != "" {
		if raw != "manual" && raw != "scheduled" {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid source"})
			return
		}
		filter.Source = raw
	}
	if raw := c.Query("status"); raw != "" {
		switch raw {
		case "queued", "running", "completed", "cancelled", "interrupted":
			filter.Status = raw
		default:
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid status"})
			return
		}
	}
	for _, bound := range []struct {
		name string
		out  *int64
	}{{"start", &filter.Start}, {"end", &filter.End}} {
		raw := c.Query(bound.name)
		if raw == "" {
			continue
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid " + bound.name})
			return
		}
		*bound.out = v
	}
	pageInfo := common.GetPageQuery(c)
	runs, total, err := model.ListQualityRuns(filter, pageInfo.GetStartIdx(), pageInfo.PageSize)
	if err != nil {
		qualityError(c, err)
		return
	}
	if runs == nil {
		runs = []model.ModelQualityRun{}
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(runs)
	common.ApiSuccess(c, pageInfo)
}

func GetQualityRun(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid run id"})
		return
	}
	run, samples, err := model.GetQualityRun(id)
	if err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"run": run, "samples": samples})
}

func CancelQualityRun(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid run id"})
		return
	}
	if err := model.CancelQualityRun(id, time.Now()); err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// ---------------------------------------------------------------------------
// Dashboard / samples / artifact / annotation
// ---------------------------------------------------------------------------

func GetQualityDashboard(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	version := 0
	if raw := c.Query("version"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < -1 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid version"})
			return
		}
		version = v
	}
	channelID := -1 // server autoselects the default channel
	if raw := c.Query("channel_id"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel_id"})
			return
		}
		channelID = v
	}
	data, err := model.QualityDashboard(id, version, channelID, time.Now())
	if err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func GetQualitySamples(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	parseInt := func(name string, min int64) (int64, bool) {
		raw := c.Query(name)
		if raw == "" {
			return 0, true
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < min {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid " + name})
			return 0, false
		}
		return v, true
	}
	version, ok := parseInt("version", -1)
	if !ok {
		return
	}
	channelID := int64(-1)
	if raw := c.Query("channel_id"); raw != "" {
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid channel_id"})
			return
		}
		channelID = v
	}
	if channelID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel_id is required"})
		return
	}
	fromMS, ok := parseInt("from_ms", 0)
	if !ok {
		return
	}
	toMS, ok := parseInt("to_ms", 0)
	if !ok {
		return
	}
	beforeMS, ok := parseInt("before_ms", 1)
	if !ok {
		return
	}
	beforeID, ok := parseInt("before_id", 1)
	if !ok {
		return
	}
	limit, ok := parseInt("limit", 1)
	if !ok {
		return
	}
	samples, err := model.QualitySamples(id, int(version), int(channelID), fromMS, toMS, beforeMS, beforeID, int(limit))
	if err != nil {
		qualityError(c, err)
		return
	}
	if samples == nil {
		samples = []model.ModelQualitySample{}
	}
	common.ApiSuccess(c, samples)
}

func GetQualityArtifact(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	artifact, err := model.GetQualityArtifact(id)
	if err != nil {
		qualityError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	expired := artifact.ExpiresAt > 0 && artifact.ExpiresAt < time.Now().UnixMilli()
	common.ApiSuccess(c, gin.H{
		"sample_id":          artifact.SampleID,
		"text":               string(artifact.Text),
		"svg":                string(artifact.SVG),
		"sha256":             artifact.SHA256,
		"validation_version": artifact.ValidationVersion,
		"expired":            expired,
	})
}

type qualityAnnotationRequest struct {
	Tag    string `json:"tag"`
	Note   string `json:"note"`
	Pinned bool   `json:"pinned"`
}

func AnnotateQualitySample(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	var req qualityAnnotationRequest
	if !qualityBindJSON(c, &req) {
		return
	}
	if err := model.AnnotateQualitySample(id, req.Tag, req.Note, req.Pinned, qualityActor(c), time.Now()); err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// GetQualityExport returns a bounded, sanitized JSON export of terminal
// samples for a case. CSV export remains a TODO; JSON is the delivered format.
func GetQualityExport(c *gin.Context) {
	id, ok := qualityParseID(c, "id")
	if !ok {
		return
	}
	version := 0
	if raw := c.Query("version"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < -1 {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid version"})
			return
		}
		version = v
	}
	channelID, err := strconv.Atoi(c.Query("channel_id"))
	if err != nil || channelID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "channel_id is required"})
		return
	}
	samples, err := model.QualitySamples(id, version, channelID, 0, 0, 0, 0, 100)
	if err != nil {
		qualityError(c, err)
		return
	}
	type exportRow struct {
		ID            int64  `json:"id"`
		RunID         string `json:"run_id"`
		Status        string `json:"status"`
		RequestID     string `json:"request_id"`
		ChannelID     int    `json:"channel_id"`
		ChannelName   string `json:"channel_name"`
		Model         string `json:"model"`
		ResponseModel string `json:"response_model"`
		StartedAt     int64  `json:"started_at"`
		FinishedAt    int64  `json:"finished_at"`
		DurationMs    *int64 `json:"duration_ms"`
		ErrorCode     string `json:"error_code"`
		Validation    string `json:"validation"`
		Annotation    string `json:"annotation"`
	}
	rows := make([]exportRow, 0, len(samples))
	for _, s := range samples {
		rows = append(rows, exportRow{
			ID: s.ID, RunID: s.RunID, Status: s.Status, RequestID: s.RequestID,
			ChannelID: s.ChannelID, ChannelName: s.ChannelName, Model: s.Model,
			ResponseModel: s.ResponseModel, StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
			DurationMs: s.DurationMs, ErrorCode: s.ErrorCode, Validation: s.Validation,
			Annotation: s.Annotation,
		})
	}
	c.Header("Cache-Control", "private, no-store")
	common.ApiSuccess(c, rows)
}

// ---------------------------------------------------------------------------
// Settings (root only)
// ---------------------------------------------------------------------------

func GetQualitySettings(c *gin.Context) {
	cfg, version, err := model.GetQualitySettings()
	if err != nil {
		qualityError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"config": cfg, "version": version})
}

type qualitySettingsWrite struct {
	Config          model.QualitySettingsConfig `json:"config"`
	ExpectedVersion int64                       `json:"expected_version"`
}

func UpdateQualitySettings(c *gin.Context) {
	var req qualitySettingsWrite
	if !qualityBindJSON(c, &req) {
		return
	}
	if err := model.ValidateQualitySettings(req.Config); err != nil {
		qualityError(c, err)
		return
	}
	for _, id := range req.Config.TokenIDs {
		if _, err := qualityinspect.ValidateExecutionIdentity(id); err != nil {
			c.JSON(http.StatusUnprocessableEntity, gin.H{"success": false, "message": "execution identity must be an active, finite-quota administrator token"})
			return
		}
	}
	if err := model.SaveQualitySettings(req.Config, req.ExpectedVersion, time.Now()); err != nil {
		qualityError(c, err)
		return
	}
	model.RecordAuditLog(c, model.AuditLog{UserId: qualityActor(c), ActorRole: c.GetInt("role"), Category: model.AuditCategoryOperation, Action: "model_quality_settings", Content: "quality execution settings updated", Success: true})
	common.ApiSuccess(c, nil)
}

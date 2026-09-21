package model

import (
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
)

type QualityChannelView struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type QualityBucket struct {
	Start    int64 `json:"start"`
	End      int64 `json:"end"`
	Success  int   `json:"success"`
	Failure  int   `json:"failure"`
	LatestAt int64 `json:"latest_at"`
}

type QualitySummary struct {
	Success     int      `json:"success"`
	Failure     int      `json:"failure"`
	Pending     int      `json:"pending"`
	Running     int      `json:"running"`
	Cancelled   int      `json:"cancelled"`
	Interrupted int      `json:"interrupted"`
	Skipped     int      `json:"skipped"`
	Total       int      `json:"total"`
	SuccessRate *float64 `json:"success_rate"`
	P50Ms       *int64   `json:"p50_ms"`
	P95Ms       *int64   `json:"p95_ms"`
}

type QualityDashboardData struct {
	CaseID      int64                `json:"case_id"`
	Version     int                  `json:"version"`
	ChannelID   int                  `json:"channel_id"`
	Channels    []QualityChannelView `json:"channels"`
	AsOf        int64                `json:"as_of"`
	WindowStart int64                `json:"window_start"`
	WindowEnd   int64                `json:"window_end"`
	Summary     QualitySummary       `json:"summary"`
	Buckets     []QualityBucket      `json:"buckets"`
}

func BuildQualityTimeline(samples []ModelQualitySample, asOf time.Time) (QualitySummary, []QualityBucket) {
	end := asOf.UnixMilli() + 1
	start := end - QualityTimelineBuckets*QualityBucketMillis
	buckets := make([]QualityBucket, QualityTimelineBuckets)
	for i := range buckets {
		buckets[i] = QualityBucket{Start: start + int64(i)*QualityBucketMillis, End: start + int64(i+1)*QualityBucketMillis}
	}
	summary := QualitySummary{}
	durations := make([]int64, 0, len(samples))
	for _, sample := range samples {
		at := sample.StartedAt
		if at == 0 {
			at = sample.CreatedAt
		}
		if at < start || at >= end {
			continue
		}
		switch sample.Status {
		case "pending":
			summary.Pending++
		case "requesting":
			summary.Running++
		case "cancelled":
			summary.Cancelled++
		case "interrupted":
			summary.Interrupted++
		case "skipped":
			summary.Skipped++
		case "succeeded", "failed":
			if sample.StartedAt == 0 {
				continue
			}
			bucket := &buckets[(at-start)/QualityBucketMillis]
			bucket.LatestAt = max(bucket.LatestAt, at)
			if sample.Status == "succeeded" {
				summary.Success++
				bucket.Success++
				if sample.DurationMs != nil && *sample.DurationMs >= 0 {
					durations = append(durations, *sample.DurationMs)
				}
			} else {
				summary.Failure++
				bucket.Failure++
			}
		}
	}
	summary.Total = summary.Success + summary.Failure
	if summary.Total > 0 {
		rate := float64(summary.Success) / float64(summary.Total)
		summary.SuccessRate = &rate
	}
	slices.Sort(durations)
	if len(durations) > 0 {
		p50 := durations[(len(durations)+1)/2-1]
		summary.P50Ms = &p50
	}
	if len(durations) >= 20 {
		p95 := durations[(95*len(durations)+99)/100-1]
		summary.P95Ms = &p95
	}
	return summary, buckets
}

func QualityDashboard(caseID int64, version int, channelID int, asOf time.Time) (QualityDashboardData, error) {
	out := QualityDashboardData{CaseID: caseID, AsOf: asOf.UnixMilli(), WindowEnd: asOf.UnixMilli() + 1, Channels: []QualityChannelView{}}
	out.WindowStart = out.WindowEnd - QualityTimelineBuckets*QualityBucketMillis
	var c ModelQualityCase
	if err := DB.First(&c, caseID).Error; err != nil {
		return out, err
	}
	allVersions := version < 0
	if version <= 0 {
		version = c.Version
	}
	if !allVersions {
		out.Version = version
	}
	var revision ModelQualityRevision
	if err := DB.Where("case_id = ? AND version = ?", caseID, version).First(&revision).Error; err != nil {
		return out, err
	}
	var cfg QualityConfig
	if err := common.UnmarshalJsonStr(revision.ConfigJSON, &cfg); err != nil {
		return out, err
	}
	names := make(map[int]string)
	for _, id := range cfg.ChannelIDs {
		names[id] = "channel"
		if ch, err := GetChannelById(id, false); err == nil {
			names[id] = ch.Name
		}
	}
	var latest []ModelQualitySample
	sub := DB.Model(&ModelQualitySample{}).Select("MAX(id)").Where("case_id = ?", caseID)
	if !allVersions {
		sub = sub.Where("version = ?", version)
	}
	sub = sub.Group("channel_id")
	if err := DB.Select("channel_id", "channel_name").Where("id IN (?)", sub).Find(&latest).Error; err != nil {
		return out, err
	}
	for _, sample := range latest {
		if sample.ChannelName != "" {
			names[sample.ChannelID] = sample.ChannelName
		} else if _, ok := names[sample.ChannelID]; !ok {
			names[sample.ChannelID] = "channel"
		}
	}
	if len(names) == 0 && cfg.Mode == "route" {
		names[0] = "Unattributed channel"
	}
	for id, name := range names {
		if id == 0 {
			name = "Unattributed channel"
		}
		out.Channels = append(out.Channels, QualityChannelView{ID: id, Name: name})
	}
	sort.Slice(out.Channels, func(i, j int) bool { return out.Channels[i].ID < out.Channels[j].ID })
	if channelID == -1 && len(out.Channels) > 0 {
		channelID = out.Channels[0].ID
	}
	if channelID < -2 {
		return out, errors.New("invalid channel filter")
	}
	if channelID >= 0 {
		if _, ok := names[channelID]; !ok {
			return out, errors.New("channel does not belong to this case version")
		}
	}
	out.ChannelID = channelID
	var samples []ModelQualitySample
	query := DB.Where("case_id = ?", caseID)
	if channelID >= 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if !allVersions {
		query = query.Where("version = ?", version)
	}
	err := query.
		Where("(started_at >= ? AND started_at < ?) OR (started_at = 0 AND created_at >= ? AND created_at < ?)", out.WindowStart, out.WindowEnd, out.WindowStart, out.WindowEnd).
		Select("status", "started_at", "created_at", "duration_ms").Find(&samples).Error
	if err != nil {
		return out, err
	}
	out.Summary, out.Buckets = BuildQualityTimeline(samples, asOf)
	return out, nil
}

// channelID -2 aggregates every channel (the "all" panel view); -1 is invalid
// here because samples must be explicitly scoped.
func QualitySamples(caseID int64, version int, channelID int, fromMS, toMS, beforeMS, beforeID int64, limit int) ([]ModelQualitySample, error) {
	if caseID < 1 || channelID < -2 || fromMS < 0 || toMS < 0 || (toMS > 0 && toMS <= fromMS) {
		return nil, errors.New("invalid sample filters")
	}
	var c ModelQualityCase
	if err := DB.First(&c, caseID).Error; err != nil {
		return nil, err
	}
	if version == 0 {
		version = c.Version
	}
	query := DB.Where("case_id = ?", caseID)
	if channelID >= 0 {
		query = query.Where("channel_id = ?", channelID)
	}
	if version > 0 {
		query = query.Where("version = ?", version)
	}
	const effectiveTime = "(CASE WHEN started_at > 0 THEN started_at ELSE created_at END)"
	if fromMS > 0 {
		query = query.Where(effectiveTime+" >= ?", fromMS)
	}
	if toMS > 0 {
		query = query.Where(effectiveTime+" < ?", toMS)
	}
	if beforeMS > 0 {
		query = query.Where(effectiveTime+" < ? OR ("+effectiveTime+" = ? AND id < ?)", beforeMS, beforeMS, beforeID)
	}
	if limit <= 0 {
		limit = 24
	}
	rows := []ModelQualitySample{}
	err := query.Order(effectiveTime + " DESC, id DESC").Limit(min(limit, 100)).Find(&rows).Error
	return rows, err
}

func QualityRequestChannel(requestID string, tokenID int) (int, string, error) {
	if requestID == "" || tokenID < 1 || LOG_DB == nil {
		return 0, "", nil
	}
	var logs []Log
	err := LOG_DB.Where("request_id = ? AND token_id = ? AND type = ?", requestID, tokenID, LogTypeConsume).
		Select("channel_id").Limit(2).Find(&logs).Error
	if err != nil || len(logs) != 1 {
		return 0, "", err
	}
	if logs[0].ChannelId < 1 {
		return 0, "", nil
	}
	channel, err := GetChannelById(logs[0].ChannelId, false)
	if err != nil {
		return logs[0].ChannelId, "", nil
	}
	return channel.Id, channel.Name, nil
}

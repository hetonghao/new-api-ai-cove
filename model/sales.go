package model

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func ListSalesInvitedUsers(salesUserID int, keyword string, group string, startIdx int, num int) ([]*User, int64, error) {
	var users []*User
	var total int64

	query := DB.Model(&User{}).Where("inviter_id = ?", salesUserID)
	if group != "" {
		query = query.Where(&User{Group: group})
	}
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		if keywordInt, err := strconv.Atoi(keyword); err == nil {
			query = query.Where("(id = ? OR username LIKE ? OR email LIKE ? OR display_name LIKE ?)",
				keywordInt, likeKeyword, likeKeyword, likeKeyword)
		} else {
			query = query.Where("(username LIKE ? OR email LIKE ? OR display_name LIKE ?)",
				likeKeyword, likeKeyword, likeKeyword)
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Omit("password", "remark").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	if err := fillSalesUserTopUpAmounts(users); err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func fillSalesUserTopUpAmounts(users []*User) error {
	if len(users) == 0 {
		return nil
	}

	userIDs := make([]int, 0, len(users))
	for _, user := range users {
		userIDs = append(userIDs, user.Id)
	}

	var rows []struct {
		UserID int     `gorm:"column:user_id"`
		Amount float64 `gorm:"column:amount"`
	}
	if err := DB.Table("top_ups").
		Select("user_id, COALESCE(sum(money), 0) as amount").
		Where("user_id IN ? AND status = ?", userIDs, common.TopUpStatusSuccess).
		Group("user_id").
		Scan(&rows).Error; err != nil {
		return err
	}

	amountsByUserID := make(map[int]float64, len(rows))
	for _, row := range rows {
		amountsByUserID[row.UserID] = row.Amount
	}
	for _, user := range users {
		user.TopUpAmount = amountsByUserID[user.Id]
		user.Remark = ""
	}
	return nil
}

func UpdateSalesInvitedUserGroup(salesUserID int, targetUserID int, group string) error {
	group = strings.TrimSpace(group)
	if salesUserID == 0 || targetUserID == 0 || group == "" {
		return errors.New("invalid sales user group update")
	}

	result := DB.Model(&User{}).
		Where("id = ? AND inviter_id = ?", targetUserID, salesUserID).
		Update("group", group)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("user is not invited by current salesperson")
	}

	return invalidateUserCache(targetUserID)
}

func GetSalesQuotaData(salesUserID int, startTime int64, endTime int64) ([]*QuotaData, error) {
	var quotaDatas []*QuotaData
	err := salesQuotaDataScope(salesUserID, startTime, endTime).
		Select("quota_data.model_name, quota_data.created_at, sum(quota_data.count) as count, sum(quota_data.quota) as quota, sum(quota_data.token_used) as token_used").
		Group("quota_data.model_name, quota_data.created_at").
		Order("quota_data.created_at asc, quota_data.model_name asc").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetSalesQuotaDataGroupByUser(salesUserID int, startTime int64, endTime int64) ([]*QuotaData, error) {
	var quotaDatas []*QuotaData
	err := salesQuotaDataGroupByUserScope(salesUserID, startTime, endTime).
		Select("users.username as username, quota_data.created_at, sum(quota_data.count) as count, sum(quota_data.quota) as quota, sum(quota_data.token_used) as token_used").
		Group("users.username, quota_data.created_at").
		Order("quota_data.created_at asc, users.username asc").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func salesQuotaDataScope(salesUserID int, startTime int64, endTime int64) *gorm.DB {
	query := DB.Table("quota_data").
		Joins("JOIN users ON users.id = quota_data.user_id").
		Where("users.inviter_id = ? AND users.deleted_at IS NULL", salesUserID)
	if startTime > 0 {
		query = query.Where("quota_data.created_at >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("quota_data.created_at <= ?", endTime)
	}
	return query
}

func salesQuotaDataGroupByUserScope(salesUserID int, startTime int64, endTime int64) *gorm.DB {
	query := DB.Table("quota_data").
		Joins("JOIN users ON users.id = quota_data.user_id").
		Where("(users.id = ? OR users.inviter_id = ?) AND users.deleted_at IS NULL", salesUserID, salesUserID)
	if startTime > 0 {
		query = query.Where("quota_data.created_at >= ?", startTime)
	}
	if endTime > 0 {
		query = query.Where("quota_data.created_at <= ?", endTime)
	}
	return query
}

func GetSalesTopUpAmount(salesUserID int) (float64, error) {
	var total struct {
		Amount float64 `gorm:"column:amount"`
	}
	err := DB.Table("top_ups").
		Select("COALESCE(sum(top_ups.money), 0) as amount").
		Joins("JOIN users ON users.id = top_ups.user_id").
		Where("users.inviter_id = ? AND users.deleted_at IS NULL", salesUserID).
		Where("top_ups.status = ?", common.TopUpStatusSuccess).
		Scan(&total).Error
	return total.Amount, err
}

func GetSalesInvitedUser(salesUserID int, targetUserID int) (*User, error) {
	var user User
	if err := DB.Omit("password", "remark").
		Where("id = ? AND inviter_id = ? AND deleted_at IS NULL", targetUserID, salesUserID).
		First(&user).Error; err != nil {
		return nil, err
	}
	user.Remark = ""
	return &user, nil
}

func GetSalesLogs(salesUserID int, logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string, sourceFilters LogSourceFilters) (logs []*Log, total int64, err error) {
	tx, err := salesLogsScope(salesUserID, logType)
	if err != nil {
		return nil, 0, err
	}
	tx = applyLogSourceFilters(tx, sourceFilters)

	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return nil, 0, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return nil, 0, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
	}
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error; err != nil {
		return nil, 0, err
	}
	formatUserLogs(logs, startIdx, false)
	return logs, total, nil
}

func GetSalesLogsStat(salesUserID int, logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, requestId string, upstreamRequestId string, sourceFilters LogSourceFilters) (stat Stat, err error) {
	if logType != LogTypeUnknown && logType != LogTypeConsume {
		return stat, nil
	}

	userIDs, err := salesInvitedUserIDs(salesUserID)
	if err != nil {
		return stat, err
	}
	tx := salesLogsScopeForUserIDs(userIDs, LogTypeConsume).Select("COALESCE(sum(logs.quota), 0) quota")
	rpmTpmQuery := salesLogsScopeForUserIDs(userIDs, LogTypeConsume).Select("count(*) rpm, COALESCE(sum(logs.prompt_tokens), 0) + COALESCE(sum(logs.completion_tokens), 0) tpm")
	tx = applyLogSourceFilters(tx, sourceFilters)
	rpmTpmQuery = applyLogSourceFilters(rpmTpmQuery, sourceFilters)

	if tx, err = applyExplicitLogTextFilter(tx, "logs.username", username); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "logs.username", username); err != nil {
		return stat, err
	}
	if tx, err = applyExplicitLogTextFilter(tx, "logs.model_name", modelName); err != nil {
		return stat, err
	}
	if rpmTpmQuery, err = applyExplicitLogTextFilter(rpmTpmQuery, "logs.model_name", modelName); err != nil {
		return stat, err
	}
	if tokenName != "" {
		tx = tx.Where("logs.token_name = ?", tokenName)
		rpmTpmQuery = rpmTpmQuery.Where("logs.token_name = ?", tokenName)
	}
	if requestId != "" {
		tx = tx.Where("logs.request_id = ?", requestId)
		rpmTpmQuery = rpmTpmQuery.Where("logs.request_id = ?", requestId)
	}
	if upstreamRequestId != "" {
		tx = tx.Where("logs.upstream_request_id = ?", upstreamRequestId)
		rpmTpmQuery = rpmTpmQuery.Where("logs.upstream_request_id = ?", upstreamRequestId)
	}
	if startTimestamp != 0 {
		tx = tx.Where("logs.created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("logs.created_at <= ?", endTimestamp)
	}
	if channel != 0 {
		tx = tx.Where("logs.channel_id = ?", channel)
		rpmTpmQuery = rpmTpmQuery.Where("logs.channel_id = ?", channel)
	}
	if group != "" {
		tx = tx.Where("logs."+logGroupCol+" = ?", group)
		rpmTpmQuery = rpmTpmQuery.Where("logs."+logGroupCol+" = ?", group)
	}
	rpmTpmQuery = rpmTpmQuery.Where("logs.created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	if err = tx.Scan(&stat).Error; err != nil {
		common.SysError("failed to query sales log stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	if err = rpmTpmQuery.Scan(&stat).Error; err != nil {
		common.SysError("failed to query sales rpm/tpm stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}

	return stat, nil
}

func salesInvitedUserIDs(salesUserID int) ([]int, error) {
	var userIDs []int
	err := DB.Model(&User{}).Where("inviter_id = ?", salesUserID).Pluck("id", &userIDs).Error
	return userIDs, err
}

func salesLogsScope(salesUserID int, logType int) (*gorm.DB, error) {
	userIDs, err := salesInvitedUserIDs(salesUserID)
	if err != nil {
		return nil, err
	}
	return salesLogsScopeForUserIDs(userIDs, logType), nil
}

func salesLogsScopeForUserIDs(userIDs []int, logType int) *gorm.DB {
	tx := LOG_DB.Model(&Log{})
	if len(userIDs) == 0 {
		tx = tx.Where("1 = 0")
	} else {
		tx = tx.Where("logs.user_id IN ?", userIDs)
	}
	if logType != LogTypeUnknown {
		tx = tx.Where("logs.type = ?", logType)
	}
	return tx
}

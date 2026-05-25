package model

import (
	"errors"
	"strconv"
	"strings"

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
	if err := query.Omit("password").Order("id desc").Limit(num).Offset(startIdx).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
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
	err := salesQuotaDataScope(salesUserID, startTime, endTime).
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

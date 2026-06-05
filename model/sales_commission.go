package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type SalesCommissionSettlement struct {
	Id              int     `json:"id"`
	SalesUserID     int     `json:"sales_user_id" gorm:"column:sales_user_id;index"`
	OperatorUserID  int     `json:"operator_user_id" gorm:"column:operator_user_id;index"`
	Amount          float64 `json:"amount" gorm:"default:0"`
	CommissionRatio float64 `json:"commission_ratio" gorm:"default:0"`
	CoveredRevenue  float64 `json:"covered_revenue" gorm:"default:0"`
	Note            string  `json:"note" gorm:"type:varchar(255);default:''"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime;column:created_at"`
}

type SalesCommissionSummary struct {
	SalesUserID              int     `json:"sales_user_id"`
	TotalRevenue             float64 `json:"total_revenue"`
	CommissionRatio          float64 `json:"commission_ratio"`
	SettledCommissionAmount  float64 `json:"settled_commission_amount"`
	SettledCommissionRevenue float64 `json:"settled_commission_revenue"`
	PendingCommissionRevenue float64 `json:"pending_commission_revenue"`
	PendingCommissionAmount  float64 `json:"pending_commission_amount"`
	TotalCommissionAmount    float64 `json:"total_commission_amount"`
	LastSettlementCreatedAt  int64   `json:"last_settlement_created_at"`
}

type SalesCommissionAdminRow struct {
	SalesUserID              int     `json:"sales_user_id"`
	Username                 string  `json:"username"`
	Email                    string  `json:"email"`
	DisplayName              string  `json:"display_name"`
	CommissionRatio          float64 `json:"commission_ratio"`
	TotalRevenue             float64 `json:"total_revenue"`
	SettledCommissionAmount  float64 `json:"settled_commission_amount"`
	SettledCommissionRevenue float64 `json:"settled_commission_revenue"`
	PendingCommissionRevenue float64 `json:"pending_commission_revenue"`
	PendingCommissionAmount  float64 `json:"pending_commission_amount"`
	TotalCommissionAmount    float64 `json:"total_commission_amount"`
	LastSettlementCreatedAt  int64   `json:"last_settlement_created_at"`
}

var (
	ErrInvalidSalesCommissionRatio       = errors.New("invalid sales commission ratio")
	ErrInvalidSalesCommissionAmount      = errors.New("invalid sales commission amount")
	ErrSalesCommissionAmountTooLarge     = errors.New("sales commission amount exceeds pending amount")
	ErrSalesCommissionRatioNotConfigured = errors.New("sales commission ratio is not configured")
)

func GetSalesCommissionSummary(salesUserID int) (*SalesCommissionSummary, error) {
	return getSalesCommissionSummaryWithTx(DB, salesUserID)
}

func getSalesCommissionSummaryWithTx(tx *gorm.DB, salesUserID int) (*SalesCommissionSummary, error) {
	var user User
	if err := tx.Model(&User{}).Unscoped().Select("id", "commission_ratio").Where("id = ?", salesUserID).First(&user).Error; err != nil {
		return nil, err
	}

	totalRevenue, err := getSalesCommissionRevenueWithTx(tx, salesUserID)
	if err != nil {
		return nil, err
	}

	var settled struct {
		Amount         float64 `gorm:"column:amount"`
		CoveredRevenue float64 `gorm:"column:covered_revenue"`
		LastCreatedAt  int64   `gorm:"column:last_created_at"`
	}
	if err := tx.Model(&SalesCommissionSettlement{}).
		Select("COALESCE(sum(amount), 0) as amount, COALESCE(sum(covered_revenue), 0) as covered_revenue, COALESCE(max(created_at), 0) as last_created_at").
		Where("sales_user_id = ?", salesUserID).
		Scan(&settled).Error; err != nil {
		return nil, err
	}

	pendingRevenueDec := decimal.NewFromFloat(totalRevenue).Sub(decimal.NewFromFloat(settled.CoveredRevenue))
	if pendingRevenueDec.IsNegative() {
		pendingRevenueDec = decimal.Zero
	}
	pendingAmountDec := pendingRevenueDec.Mul(decimal.NewFromFloat(user.CommissionRatio)).Div(decimal.NewFromInt(100))
	totalAmountDec := decimal.NewFromFloat(settled.Amount).Add(pendingAmountDec)

	return &SalesCommissionSummary{
		SalesUserID:              salesUserID,
		TotalRevenue:             totalRevenue,
		CommissionRatio:          user.CommissionRatio,
		SettledCommissionAmount:  settled.Amount,
		SettledCommissionRevenue: settled.CoveredRevenue,
		PendingCommissionRevenue: pendingRevenueDec.InexactFloat64(),
		PendingCommissionAmount:  pendingAmountDec.InexactFloat64(),
		TotalCommissionAmount:    totalAmountDec.InexactFloat64(),
		LastSettlementCreatedAt:  settled.LastCreatedAt,
	}, nil
}

func getSalesCommissionRevenueWithTx(tx *gorm.DB, salesUserID int) (float64, error) {
	var total struct {
		Amount float64 `gorm:"column:amount"`
	}
	err := tx.Table("top_ups").
		Select("COALESCE(sum(top_ups.money), 0) as amount").
		Joins("JOIN users ON users.id = top_ups.user_id").
		Where("users.inviter_id = ? AND users.deleted_at IS NULL", salesUserID).
		Where("top_ups.status = ?", common.TopUpStatusSuccess).
		Scan(&total).Error
	return total.Amount, err
}

func ListSalesCommissionSettlements(salesUserID int, pageInfo *common.PageInfo) ([]*SalesCommissionSettlement, int64, error) {
	var settlements []*SalesCommissionSettlement
	var total int64
	query := DB.Model(&SalesCommissionSettlement{}).Where("sales_user_id = ?", salesUserID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&settlements).Error; err != nil {
		return nil, 0, err
	}
	return settlements, total, nil
}

func ListSalesCommissionAdminRows(keyword string, pageInfo *common.PageInfo) ([]*SalesCommissionAdminRow, int64, error) {
	var users []*User
	var total int64

	query := DB.Model(&User{}).Where("role = ? AND deleted_at IS NULL", common.RoleSalesUser)
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where("(username LIKE ? OR email LIKE ? OR display_name LIKE ?)", likeKeyword, likeKeyword, likeKeyword)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := query.Omit("password", "remark").Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	rows := make([]*SalesCommissionAdminRow, 0, len(users))
	for _, user := range users {
		summary, err := GetSalesCommissionSummary(user.Id)
		if err != nil {
			return nil, 0, err
		}
		rows = append(rows, &SalesCommissionAdminRow{
			SalesUserID:              user.Id,
			Username:                 user.Username,
			Email:                    user.Email,
			DisplayName:              user.DisplayName,
			CommissionRatio:          summary.CommissionRatio,
			TotalRevenue:             summary.TotalRevenue,
			SettledCommissionAmount:  summary.SettledCommissionAmount,
			SettledCommissionRevenue: summary.SettledCommissionRevenue,
			PendingCommissionRevenue: summary.PendingCommissionRevenue,
			PendingCommissionAmount:  summary.PendingCommissionAmount,
			TotalCommissionAmount:    summary.TotalCommissionAmount,
			LastSettlementCreatedAt:  summary.LastSettlementCreatedAt,
		})
	}
	return rows, total, nil
}

func CreateSalesCommissionSettlement(salesUserID int, operatorUserID int, amount float64, note string) (*SalesCommissionSettlement, error) {
	if salesUserID == 0 || operatorUserID == 0 {
		return nil, ErrInvalidSalesCommissionAmount
	}
	amountDec := decimal.NewFromFloat(amount)
	if !amountDec.IsPositive() {
		return nil, ErrInvalidSalesCommissionAmount
	}

	var settlement SalesCommissionSettlement
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Set("gorm:query_option", "FOR UPDATE").
			Where("id = ? AND role = ? AND deleted_at IS NULL", salesUserID, common.RoleSalesUser).
			First(&user).Error; err != nil {
			return err
		}
		if user.CommissionRatio <= 0 {
			return ErrSalesCommissionRatioNotConfigured
		}

		summary, err := getSalesCommissionSummaryWithTx(tx, salesUserID)
		if err != nil {
			return err
		}
		pendingDec := decimal.NewFromFloat(summary.PendingCommissionAmount)
		if amountDec.GreaterThan(pendingDec) {
			return ErrSalesCommissionAmountTooLarge
		}

		ratioDec := decimal.NewFromFloat(user.CommissionRatio).Div(decimal.NewFromInt(100))
		coveredRevenueDec := amountDec.Div(ratioDec)
		settlement = SalesCommissionSettlement{
			SalesUserID:     salesUserID,
			OperatorUserID:  operatorUserID,
			Amount:          amountDec.InexactFloat64(),
			CommissionRatio: user.CommissionRatio,
			CoveredRevenue:  coveredRevenueDec.InexactFloat64(),
			Note:            strings.TrimSpace(note),
			CreatedAt:       common.GetTimestamp(),
		}
		return tx.Create(&settlement).Error
	})
	if err != nil {
		return nil, err
	}
	return &settlement, nil
}

func UpdateSalesCommissionRatio(salesUserID int, ratio float64) error {
	if salesUserID == 0 || ratio < 0 || ratio > 100 {
		return ErrInvalidSalesCommissionRatio
	}
	result := DB.Model(&User{}).
		Where("id = ? AND role = ? AND deleted_at IS NULL", salesUserID, common.RoleSalesUser).
		Update("commission_ratio", ratio)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return invalidateUserCache(salesUserID)
}

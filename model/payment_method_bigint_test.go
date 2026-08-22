package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRechargeOtherProvidersCredit5000ToBigIntWallet(t *testing.T) {
	tests := []struct {
		name      string
		provider  string
		tradeNo   string
		settle    func(string) error
		amount    int64
		money     float64
		wantQuota int
	}{
		{
			name:     "stripe",
			provider: PaymentProviderStripe,
			tradeNo:  "TOPUPBIGINTSTRIPE",
			settle: func(tradeNo string) error {
				return Recharge(tradeNo, "cus_bigint", "127.0.0.1")
			},
			amount:    1,
			money:     5000,
			wantQuota: 7_486_109_506,
		},
		{
			name:     "creem",
			provider: PaymentProviderCreem,
			tradeNo:  "TOPUPBIGINTCREEM",
			settle: func(tradeNo string) error {
				return RechargeCreem(tradeNo, "", "", "127.0.0.1")
			},
			amount:    2_500_000_000,
			money:     5000,
			wantQuota: 7_486_109_506,
		},
		{
			name:     "waffo",
			provider: PaymentProviderWaffo,
			tradeNo:  "TOPUPBIGINTWAFFO",
			settle: func(tradeNo string) error {
				return RechargeWaffo(tradeNo, "127.0.0.1")
			},
			amount:    5000,
			money:     5000,
			wantQuota: 7_486_109_506,
		},
		{
			name:     "waffo pancake",
			provider: PaymentProviderWaffoPancake,
			tradeNo:  "TOPUPBIGINTPANCAKE",
			settle: func(tradeNo string) error {
				return RechargeWaffoPancake(tradeNo)
			},
			amount:    5000,
			money:     5000,
			wantQuota: 7_486_109_506,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			truncateTables(t)
			oldQuotaPerUnit := common.QuotaPerUnit
			common.QuotaPerUnit = 500000
			t.Cleanup(func() { common.QuotaPerUnit = oldQuotaPerUnit })

			user := insertUserForPaymentGuardTest(t, 507, 4_986_109_506)
			topUp := TopUp{
				UserId:          user.Id,
				Amount:          tc.amount,
				Money:           tc.money,
				TradeNo:         tc.tradeNo,
				PaymentMethod:   tc.provider,
				PaymentProvider: tc.provider,
				CreateTime:      common.GetTimestamp(),
				Status:          common.TopUpStatusPending,
			}
			require.NoError(t, DB.Create(&topUp).Error)

			require.NoError(t, tc.settle(topUp.TradeNo))
			assert.Equal(t, tc.wantQuota, getUserQuotaForPaymentGuardTest(t, user.Id))
			assert.Equal(t, common.TopUpStatusSuccess, getTopUpStatusForPaymentGuardTest(t, topUp.TradeNo))
		})
	}
}

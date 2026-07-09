package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveTopupAmountDiscount(t *testing.T) {
	originalDiscounts := make(map[int]float64, len(paymentSetting.AmountDiscount))
	for k, v := range paymentSetting.AmountDiscount {
		originalDiscounts[k] = v
	}

	t.Cleanup(func() {
		paymentSetting.AmountDiscount = originalDiscounts
	})

	paymentSetting.AmountDiscount = map[int]float64{
		100:  0.95,
		500:  0.92,
		1000: 0.91,
		1500: 0,
	}

	testCases := []struct {
		name     string
		amount   int64
		expected float64
	}{
		{name: "non-positive amount has no discount", amount: 0, expected: 1},
		{name: "below first tier has no discount", amount: 99, expected: 1},
		{name: "exact first tier uses first discount", amount: 100, expected: 0.95},
		{name: "between tiers uses previous tier", amount: 499, expected: 0.95},
		{name: "exact middle tier uses middle discount", amount: 500, expected: 0.92},
		{name: "above max tier keeps max discount", amount: 1001, expected: 0.91},
		{name: "invalid configured discount is ignored", amount: 1500, expected: 0.91},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, ResolveTopupAmountDiscount(tc.amount))
		})
	}
}

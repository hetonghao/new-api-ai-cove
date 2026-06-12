package operation_setting

import "testing"

func TestQuotaDisplayGettersAlwaysUseUSD(t *testing.T) {
	originalType := generalSetting.QuotaDisplayType
	originalSymbol := generalSetting.CustomCurrencySymbol
	originalRate := generalSetting.CustomCurrencyExchangeRate

	t.Cleanup(func() {
		generalSetting.QuotaDisplayType = originalType
		generalSetting.CustomCurrencySymbol = originalSymbol
		generalSetting.CustomCurrencyExchangeRate = originalRate
	})

	generalSetting.QuotaDisplayType = QuotaDisplayTypeCNY
	generalSetting.CustomCurrencySymbol = "€"
	generalSetting.CustomCurrencyExchangeRate = 0.9

	if got := GetQuotaDisplayType(); got != QuotaDisplayTypeUSD {
		t.Fatalf("GetQuotaDisplayType() = %q, want %q", got, QuotaDisplayTypeUSD)
	}
	if !IsCurrencyDisplay() {
		t.Fatal("IsCurrencyDisplay() = false, want true")
	}
	if IsCNYDisplay() {
		t.Fatal("IsCNYDisplay() = true, want false")
	}
	if got := GetCurrencySymbol(); got != "$" {
		t.Fatalf("GetCurrencySymbol() = %q, want $", got)
	}
	if got := GetUsdToCurrencyRate(7.2); got != 1 {
		t.Fatalf("GetUsdToCurrencyRate() = %v, want 1", got)
	}
}

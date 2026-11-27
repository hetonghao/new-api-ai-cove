package operation_setting

import (
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

// DeviMonOptionKey stores the Devin monitor configuration as one JSON option.
const DeviMonOptionKey = "DeviMonSettings"

// DeviMonSettings configures the custom Devin monitor: how often the collector
// reruns, and the egress proxy used by the ChatGPT sign-in channel. The proxy
// is shared by every account on that channel; CPA-channel accounts ignore it.
type DeviMonSettings struct {
	IntervalMinutes int    `json:"interval_minutes"`
	ChatGPTProxyURL string `json:"chatgpt_proxy_url"`
}

var deviMonDefaults = DeviMonSettings{IntervalMinutes: 180}
var deviMonSettings atomic.Pointer[DeviMonSettings]

func GetDeviMonSettings() DeviMonSettings {
	if settings := deviMonSettings.Load(); settings != nil {
		return *settings
	}
	return deviMonDefaults
}

func (settings DeviMonSettings) Validate() error {
	if settings.IntervalMinutes < 15 || settings.IntervalMinutes > 10080 {
		return common.NewMessage("Monitor interval must be between 15 and 10080 minutes")
	}
	if _, err := common.ParseProxyURLStrict(settings.ChatGPTProxyURL); err != nil {
		return common.NewMessage("Proxy URL must use http, https, socks5, or socks5h")
	}
	return nil
}

func ParseDeviMonSettings(value string) (DeviMonSettings, error) {
	var fields struct {
		IntervalMinutes *int   `json:"interval_minutes"`
		ChatGPTProxyURL string `json:"chatgpt_proxy_url"`
	}
	if err := common.UnmarshalJsonStr(value, &fields); err != nil || fields.IntervalMinutes == nil {
		return DeviMonSettings{}, common.NewMessage("Invalid monitor configuration")
	}
	settings := DeviMonSettings{
		IntervalMinutes: *fields.IntervalMinutes,
		ChatGPTProxyURL: strings.TrimSpace(fields.ChatGPTProxyURL),
	}
	return settings, settings.Validate()
}

func SetDeviMonSettings(settings DeviMonSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	deviMonSettings.Store(&settings)
	return nil
}

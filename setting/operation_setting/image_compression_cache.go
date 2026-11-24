package operation_setting

import (
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
)

const ImageCompressionCacheOptionKey = "ImageCompressionCacheSettings"

type ImageCompressionCacheSettings struct {
	Enabled    bool  `json:"enabled"`
	MaxBytes   int64 `json:"max_bytes"`
	TTLSeconds int   `json:"ttl_seconds"`
}

var imageCompressionCacheDefaults = ImageCompressionCacheSettings{MaxBytes: 64 << 20, TTLSeconds: 900}
var imageCompressionCacheSettings atomic.Pointer[ImageCompressionCacheSettings]
var imageCompressionCacheSettingsChange atomic.Pointer[func()]

func GetImageCompressionCacheSettings() ImageCompressionCacheSettings {
	if settings := imageCompressionCacheSettings.Load(); settings != nil {
		return *settings
	}
	return imageCompressionCacheDefaults
}

func (settings ImageCompressionCacheSettings) Validate() error {
	if settings.MaxBytes < 1<<20 || settings.MaxBytes > 256<<20 {
		return common.NewMessage("Image compression cache capacity must be between 1 and 256 MiB")
	}
	if settings.TTLSeconds < 60 || settings.TTLSeconds > 3600 {
		return common.NewMessage("Image compression cache lifetime must be between 60 and 3600 seconds")
	}
	return nil
}

func ParseImageCompressionCacheSettings(value string) (ImageCompressionCacheSettings, error) {
	var fields struct {
		Enabled    *bool `json:"enabled"`
		MaxBytes   int64 `json:"max_bytes"`
		TTLSeconds int   `json:"ttl_seconds"`
	}
	if err := common.UnmarshalJsonStr(value, &fields); err != nil || fields.Enabled == nil {
		return ImageCompressionCacheSettings{}, common.NewMessage("Invalid image compression cache configuration")
	}
	settings := ImageCompressionCacheSettings{Enabled: *fields.Enabled, MaxBytes: fields.MaxBytes, TTLSeconds: fields.TTLSeconds}
	return settings, settings.Validate()
}

func SetImageCompressionCacheSettings(settings ImageCompressionCacheSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	previous := imageCompressionCacheSettings.Swap(&settings)
	if callback := imageCompressionCacheSettingsChange.Load(); callback != nil && (previous == nil || *previous != settings) {
		(*callback)()
	}
	return nil
}

// RegisterImageCompressionCacheSettingsChange installs the process cache's change callback.
func RegisterImageCompressionCacheSettingsChange(callback func()) {
	imageCompressionCacheSettingsChange.Store(&callback)
}

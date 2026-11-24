package model

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

var imageCompressionCacheOptionMutex sync.Mutex

func UpdateImageCompressionCacheSettings(settings operation_setting.ImageCompressionCacheSettings) error {
	encoded, err := common.Marshal(settings)
	if err != nil {
		return err
	}
	return UpdateOptionsBulk(map[string]string{operation_setting.ImageCompressionCacheOptionKey: string(encoded)})
}

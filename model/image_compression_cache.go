package model

import (
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var imageCompressionCacheOptionMutex sync.Mutex

func UpdateImageCompressionCacheSettings(settings operation_setting.ImageCompressionCacheSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	imageCompressionCacheOptionMutex.Lock()
	defer imageCompressionCacheOptionMutex.Unlock()
	key := operation_setting.ImageCompressionCacheOptionKey
	settings.Revision = 0
	var encoded []byte
	err := DB.Transaction(func(tx *gorm.DB) error {
		row := Option{Key: key}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where(Option{Key: key}).Take(&row).Error; err != nil {
			return err
		}
		if row.Value != "" {
			prior, err := operation_setting.ParseImageCompressionCacheSettings(row.Value)
			if err != nil {
				return err
			}
			settings.Revision = prior.Revision
		}
		settings.Revision++
		var err error
		encoded, err = common.Marshal(struct {
			operation_setting.ImageCompressionCacheSettings
			Revision int64 `json:"revision"`
		}{ImageCompressionCacheSettings: settings, Revision: settings.Revision})
		if err != nil {
			return err
		}
		return tx.Model(&row).Update("value", string(encoded)).Error
	})
	if err != nil {
		return err
	}
	BumpSyncTopic(SyncTopicOptions)
	return updateOptionMap(key, string(encoded))
}

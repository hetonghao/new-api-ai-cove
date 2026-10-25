package model

import (
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Ability struct {
	Group     string  `json:"group" gorm:"type:varchar(64);primaryKey;autoIncrement:false"`
	Model     string  `json:"model" gorm:"type:varchar(255);primaryKey;autoIncrement:false"`
	ChannelId int     `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool    `json:"enabled"`
	Priority  *int64  `json:"priority" gorm:"bigint;default:0;index"`
	Weight    uint    `json:"weight" gorm:"default:0;index"`
	Tag       *string `json:"tag" gorm:"index"`
}

type AbilityWithChannel struct {
	Ability
	ChannelType int `json:"channel_type"`
}

func GetAllEnableAbilityWithChannels() ([]AbilityWithChannel, error) {
	var abilities []AbilityWithChannel
	err := DB.Table("abilities").
		Select("abilities.*, channels.type as channel_type").
		Joins("left join channels on abilities.channel_id = channels.id").
		Where("abilities.enabled = ?", true).
		Scan(&abilities).Error
	return abilities, err
}

func GetGroupEnabledModels(group string) []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where(commonGroupCol+" = ? and enabled = ?", group, true).Distinct("model").Pluck("model", &models)
	return models
}

func GetEnabledModels() []string {
	var models []string
	// Find distinct models
	DB.Table("abilities").Where("enabled = ?", true).Distinct("model").Pluck("model", &models)
	return models
}

func GetAllEnableAbilities() []Ability {
	var abilities []Ability
	DB.Find(&abilities, "enabled = ?", true)
	return abilities
}

func getChannelQuery(group string, model string, excludedChannelIDs map[int]bool) *gorm.DB {
	query := DB.Model(&Ability{}).Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true)
	if excludedIDs := channelIDsFromMap(excludedChannelIDs); len(excludedIDs) > 0 {
		query = query.Where("channel_id NOT IN ?", excludedIDs)
	}
	return query.Order("priority DESC, weight DESC")
}

func GetChannel(group string, model string, retry int, filters []dto.ChannelFilter) (*Channel, error) {
	return getChannelWithSelection(group, model, retry, "", false, nil, filters)
}

func GetChannelWithSelection(group string, model string, retry int, requestPath string, requireWebSockets bool, excludedChannelIDs map[int]bool, filters ...[]dto.ChannelFilter) (*Channel, error) {
	var channelFilters []dto.ChannelFilter
	if len(filters) > 0 {
		channelFilters = append(channelFilters, filters[0]...)
	}
	if requestPath != "" {
		channelFilters = append(channelFilters, dto.ChannelFilter{Kind: dto.FilterRequestPath, RequestPath: requestPath})
	}
	return getChannelWithSelection(group, model, retry, requestPath, requireWebSockets, excludedChannelIDs, channelFilters)
}

func getChannelWithSelection(group string, model string, retry int, requestPath string, requireWebSockets bool, excludedChannelIDs map[int]bool, filters []dto.ChannelFilter) (*Channel, error) {
	var abilities []Ability
	if err := getChannelQuery(group, model, excludedChannelIDs).Find(&abilities).Error; err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByConstraints(abilities, model, filters)
	// Like the memory cache, retry with the routing name (for example without
	// @ modifiers) when no channel serves the exact name.
	if normalized := ratio_setting.RoutingMatchModelName(model); len(abilities) == 0 && normalized != "" && normalized != model {
		if err := getChannelQuery(group, normalized, excludedChannelIDs).Find(&abilities).Error; err != nil {
			return nil, err
		}
		abilities = filterAbilitiesByConstraints(abilities, model, filters)
	}
	if requestPath != "" && len(filters) == 0 {
		abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, model)
	}
	if requireWebSockets {
		abilities = filterAbilitiesByWebSocketCapability(abilities, excludedChannelIDs)
	}
	abilities = filterAbilitiesByExcludedChannels(abilities, excludedChannelIDs)
	if len(abilities) > 0 {
		priorities := make([]int64, 0)
		seen := make(map[int64]bool)
		for _, ability := range abilities {
			priority := int64(0)
			if ability.Priority != nil {
				priority = *ability.Priority
			}
			if !seen[priority] {
				seen[priority] = true
				priorities = append(priorities, priority)
			}
		}
		sort.Slice(priorities, func(i, j int) bool { return priorities[i] > priorities[j] })
		if retry >= len(priorities) {
			retry = len(priorities) - 1
		}
		targetPriority := priorities[retry]
		abilities = lo.Filter(abilities, func(ability Ability, _ int) bool {
			return ability.Priority == nil && targetPriority == 0 || ability.Priority != nil && *ability.Priority == targetPriority
		})
	}
	channel := Channel{}
	if len(abilities) > 0 {
		// Randomly choose one
		weightSum := uint(0)
		for _, ability_ := range abilities {
			weightSum += ability_.Weight + 10
		}
		// Randomly choose one
		weight := common.GetRandomInt(int(weightSum))
		for _, ability_ := range abilities {
			weight -= int(ability_.Weight) + 10
			//log.Printf("weight: %d, ability weight: %d", weight, *ability_.Weight)
			if weight <= 0 {
				channel.Id = ability_.ChannelId
				break
			}
		}
	} else {
		return nil, nil
	}
	err := DB.First(&channel, "id = ?", channel.Id).Error
	return &channel, err
}

func channelIDsFromMap(channelIDs map[int]bool) []int {
	ids := make([]int, 0, len(channelIDs))
	for id, excluded := range channelIDs {
		if excluded {
			ids = append(ids, id)
		}
	}
	return ids
}

func filterAbilitiesByExcludedChannels(abilities []Ability, excludedChannelIDs map[int]bool) []Ability {
	if len(excludedChannelIDs) == 0 || len(abilities) == 0 {
		return abilities
	}
	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if !excludedChannelIDs[ability.ChannelId] {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

func getWebSocketChannel(group string, model string, retry int, requestPath string, excludedChannelIDs map[int]bool) (*Channel, error) {
	var abilities []Ability
	if err := DB.Where(commonGroupCol+" = ? and model = ? and enabled = ?", group, model, true).
		Order("priority DESC, weight DESC").Find(&abilities).Error; err != nil {
		return nil, err
	}
	abilities = filterAbilitiesByRequestPathAndModel(abilities, requestPath, model)
	abilities = filterAbilitiesByWebSocketCapability(abilities, excludedChannelIDs)
	if len(abilities) == 0 {
		return nil, nil
	}

	priorities := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, ability := range abilities {
		priority := lo.FromPtrOr(ability.Priority, int64(0))
		if _, ok := seen[priority]; ok {
			continue
		}
		seen[priority] = struct{}{}
		priorities = append(priorities, priority)
	}
	if retry >= len(priorities) {
		retry = len(priorities) - 1
	}
	targetPriority := priorities[retry]
	var eligible []Ability
	for _, ability := range abilities {
		if lo.FromPtrOr(ability.Priority, int64(0)) == targetPriority {
			eligible = append(eligible, ability)
		}
	}

	weightSum := 0
	for _, ability := range eligible {
		weightSum += int(ability.Weight) + 10
	}
	weight := common.GetRandomInt(weightSum)
	channelID := eligible[len(eligible)-1].ChannelId
	for _, ability := range eligible {
		weight -= int(ability.Weight) + 10
		if weight <= 0 {
			channelID = ability.ChannelId
			break
		}
	}
	var channel Channel
	if err := DB.First(&channel, "id = ?", channelID).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

func filterAbilitiesByWebSocketCapability(abilities []Ability, excludedChannelIDs map[int]bool) []Ability {
	if len(abilities) == 0 {
		return abilities
	}
	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		if excludedChannelIDs[ability.ChannelId] {
			continue
		}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	var channels []Channel
	if err := DB.Where("id IN ? AND status = ?", channelIDs, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil
	}
	enabled := make(map[int]struct{}, len(channels))
	for _, channel := range channels {
		if isResponsesWebSocketChannelType(channel.Type) && channel.GetSetting().ResponsesWebSocketEnabled {
			enabled[channel.Id] = struct{}{}
		}
	}
	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if excludedChannelIDs[ability.ChannelId] {
			continue
		}
		if _, ok := enabled[ability.ChannelId]; ok {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

// filterAbilitiesByRequestPathAndModel restricts candidates by request path and
// model for the DB (non-memory-cache) selection path. Only Advanced Custom
// (type 58) channels are path-checked; all other channel types always pass.
func filterAbilitiesByRequestPathAndModel(abilities []Ability, requestPath string, model string) []Ability {
	if requestPath == "" || len(abilities) == 0 {
		return abilities
	}

	channelIds := make([]int, 0, len(abilities))
	seen := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIds = append(channelIds, ability.ChannelId)
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
		return abilities
	}

	advancedConfigs := make(map[int]*kitdto.AdvancedCustomConfig)
	for _, channel := range channels {
		if channel.Type == constant.ChannelTypeAdvancedCustom {
			advancedConfigs[channel.Id] = channel.GetOtherSettings().AdvancedCustom
		}
	}

	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		config, isAdvancedCustom := advancedConfigs[ability.ChannelId]
		if !isAdvancedCustom || (config != nil && config.SupportsPathForModel(requestPath, model)) {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

// filterAbilitiesByConstraints applies the same ChannelSatisfiesFilters
// predicate used by the memory-cache path. A failed channel lookup fails
// closed when client restrictions or a task-plugin identity require it.
func filterAbilitiesByConstraints(abilities []Ability, modelName string, filters []dto.ChannelFilter) []Ability {
	if len(abilities) == 0 {
		return nil
	}

	channelIds := make([]int, 0, len(abilities))
	seen := make(map[int]struct{}, len(abilities))
	for _, ability := range abilities {
		if _, ok := seen[ability.ChannelId]; ok {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIds = append(channelIds, ability.ChannelId)
	}

	var channels []*Channel
	if err := DB.Where("id IN ?", channelIds).Find(&channels).Error; err != nil {
		for _, filter := range filters {
			if filter.Kind == dto.FilterCodexClient && !filter.CodexClient {
				return nil
			}
		}
		if identityFilterRequiresKey(filters) {
			return nil
		}
		return abilities
	}

	channelsByID := make(map[int]*Channel, len(channels))
	for _, channel := range channels {
		channelsByID[channel.Id] = channel
	}

	filtered := make([]Ability, 0, len(abilities))
	for _, ability := range abilities {
		if ok, _ := ChannelSatisfiesFilters(channelsByID[ability.ChannelId], modelName, filters); ok {
			filtered = append(filtered, ability)
		}
	}
	return filtered
}

func identityFilterRequiresKey(filters []dto.ChannelFilter) bool {
	for _, filter := range filters {
		if filter.Kind == dto.FilterTaskPluginIdentity && filter.TaskPluginKey != "" {
			return true
		}
	}
	return false
}

func (channel *Channel) AddAbilities(tx *gorm.DB) error {
	models_ := strings.Split(channel.Models, ",")
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	// choose DB or provided tx
	useDB := DB
	if tx != nil {
		useDB = tx
	}
	for _, chunk := range lo.Chunk(abilities, 50) {
		err := useDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities(tx *gorm.DB) error {
	isNewTx := false
	// 如果没有传入事务，创建新的事务
	if tx == nil {
		tx = DB.Begin()
		if tx.Error != nil {
			return tx.Error
		}
		isNewTx = true
		defer func() {
			if r := recover(); r != nil {
				tx.Rollback()
			}
		}()
	}

	// First delete all abilities of this channel
	err := tx.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
	if err != nil {
		if isNewTx {
			tx.Rollback()
		}
		return err
	}

	// Then add new abilities
	models_ := channel.GetModels()
	groups_ := strings.Split(channel.Group, ",")
	abilitySet := make(map[string]struct{})
	abilities := make([]Ability, 0, len(models_))
	for _, model := range models_ {
		for _, group := range groups_ {
			key := group + "|" + model
			if _, exists := abilitySet[key]; exists {
				continue
			}
			abilitySet[key] = struct{}{}
			ability := Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == common.ChannelStatusEnabled,
				Priority:  channel.Priority,
				Weight:    uint(channel.GetWeight()),
				Tag:       channel.Tag,
			}
			abilities = append(abilities, ability)
		}
	}

	if len(abilities) > 0 {
		for _, chunk := range lo.Chunk(abilities, 50) {
			err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error
			if err != nil {
				if isNewTx {
					tx.Rollback()
				}
				return err
			}
		}
	}

	// 如果是新创建的事务，需要提交
	if isNewTx {
		return tx.Commit().Error
	}

	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityStatusByTag(tag string, status bool) error {
	return DB.Model(&Ability{}).Where("tag = ?", tag).Select("enabled").Update("enabled", status).Error
}

func UpdateAbilityByTag(tag string, newTag *string, priority *int64, weight *uint) error {
	ability := Ability{}
	if newTag != nil {
		ability.Tag = newTag
	}
	if priority != nil {
		ability.Priority = priority
	}
	if weight != nil {
		ability.Weight = *weight
	}
	return DB.Model(&Ability{}).Where("tag = ?", tag).Updates(ability).Error
}

var fixLock = sync.Mutex{}

func FixAbility() (int, int, error) {
	lock := fixLock.TryLock()
	if !lock {
		return 0, 0, common.NewMessage("A repair task is already running. Please try again later")
	}
	defer fixLock.Unlock()

	// truncate abilities table
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		err := DB.Exec("DELETE FROM abilities").Error
		if err != nil {
			common.SysLog(common.LogText("Delete abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	} else {
		err := DB.Exec("TRUNCATE TABLE abilities").Error
		if err != nil {
			common.SysLog(common.LogText("Truncate abilities failed: %s", err.Error()))
			return 0, 0, err
		}
	}
	var channels []*Channel
	// Find all channels
	err := DB.Model(&Channel{}).Find(&channels).Error
	if err != nil {
		return 0, 0, err
	}
	if len(channels) == 0 {
		return 0, 0, nil
	}
	successCount := 0
	failCount := 0
	for _, chunk := range lo.Chunk(channels, 50) {
		ids := lo.Map(chunk, func(c *Channel, _ int) int { return c.Id })
		// Delete all abilities of this channel
		err = DB.Where("channel_id IN ?", ids).Delete(&Ability{}).Error
		if err != nil {
			common.SysLog(common.LogText("Delete abilities failed: %s", err.Error()))
			failCount += len(chunk)
			continue
		}
		// Then add new abilities
		for _, channel := range chunk {
			err = channel.AddAbilities(nil)
			if err != nil {
				common.SysLog(common.LogText("Add abilities for channel %d failed: %s", channel.Id, err.Error()))
				failCount++
			} else {
				successCount++
			}
		}
	}
	InitChannelCache()
	return successCount, failCount, nil
}

package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelSelectAutoGroupsTest(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalRetryTimes := common.RetryTimes
	originalAutoGroups := setting.AutoGroups2JsonString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalGroupRatios := ratio_setting.GroupRatio2JSONString()
	originalMaxTokenAutoGroups := setting.GetMaxTokenAutoGroups()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db
	common.MemoryCacheEnabled = true
	common.RetryTimes = 0

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":2}`))
	require.NoError(t, setting.UpdateMaxTokenAutoGroups("2"))

	t.Cleanup(func() {
		model.DB = originalDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.RetryTimes = originalRetryTimes
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatios))
		require.NoError(t, setting.UpdateMaxTokenAutoGroups(fmt.Sprintf("%d", originalMaxTokenAutoGroups)))

		if originalMemoryCacheEnabled && originalDB != nil &&
			originalDB.Migrator().HasTable(&model.Channel{}) && originalDB.Migrator().HasTable(&model.Ability{}) {
			model.InitChannelCache()
		}
		sqlDB, err := db.DB()
		if err == nil {
			require.NoError(t, sqlDB.Close())
		}
	})

	return db
}

func createChannelSelectAutoGroupsChannel(t *testing.T, db *gorm.DB, id int, group, modelName string) {
	t.Helper()
	priority := int64(0)
	weight := uint(100)
	require.NoError(t, db.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeOpenAI,
		Key:      fmt.Sprintf("key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("channel-%d", id),
		Weight:   &weight,
		Models:   modelName,
		Group:    group,
		Priority: &priority,
	}).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     modelName,
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelUsesTokenAutoGroupsWhenGlobalAutoIsEmpty(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "auto-groups-runtime-model"
	createChannelSelectAutoGroupsChannel(t, db, 2101, "vip", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2102, "default", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"vip", "default"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2101, first.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, "vip", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
	assert.Empty(t, setting.GetAutoGroups(), "the selection must not depend on the global Auto list")

	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2102, second.Id)
	assert.Equal(t, "default", selectedGroup)
	assert.Equal(t, "default", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelPreservesGlobalRetryBudgetOnGroupSwitch(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	common.RetryTimes = 1
	const modelName = "auto-group-budget-model"
	createChannelSelectAutoGroupsChannel(t, db, 2111, "vip", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 1
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "auto",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 2111, channel.Id)
	assert.Equal(t, "vip", selectedGroup)
	assert.Equal(t, 1, param.GetRetry(), "switching auto groups must not reset the logical retry budget")
}

func TestCacheGetRandomSatisfiedChannelCrossGroupPrefersNextGroupBeforeSameChannelFallback(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	common.RetryTimes = 2
	const modelName = "cross-group-fallback-model"
	createChannelSelectAutoGroupsChannel(t, db, 2121, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2122, "vip", modelName)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroups, []string{"default", "vip"})
	common.SetContextKey(ctx, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := 0
	param := &RetryParam{Ctx: ctx, TokenGroup: "auto", ModelName: modelName, RequestPath: "/v1/chat/completions", Retry: &retry}
	first, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, 2121, first.Id)
	assert.Equal(t, "default", selectedGroup)

	param.RecordChannel(first)
	param.IncreaseRetry()
	second, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, 2122, second.Id, "cross-group retry must advance before same-channel fallback")
	assert.Equal(t, "vip", selectedGroup)

	param.RecordChannel(second)
	param.IncreaseRetry()
	third, selectedGroup, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, 2122, third.Id, "same-channel fallback is allowed after all groups are exhausted")
	assert.Equal(t, "vip", selectedGroup)
}

func TestRetryCandidateOrderPrefersDifferentRouteBeforeSameRoute(t *testing.T) {
	baseA := "https://cpa-a.example"
	baseB := "https://cpa-b.example"
	candidates := []*model.Channel{
		{Id: 2201, Type: constant.ChannelTypeNewAPI, BaseURL: &baseA},
		{Id: 2202, Type: constant.ChannelTypeNewAPI, BaseURL: &baseA},
		{Id: 2203, Type: constant.ChannelTypeNewAPI, BaseURL: &baseB},
	}

	ordered := orderRetryCandidates(candidates, 2201, channelRetryRouteKey(candidates[0]))
	require.Len(t, ordered, 3)
	assert.Equal(t, 2203, ordered[0].Id)
	assert.Equal(t, 2202, ordered[1].Id)
	assert.Equal(t, 2201, ordered[2].Id)
}

func TestRetryCandidateOrderKeepsUnknownRouteLastChannelAsFinalFallback(t *testing.T) {
	candidates := []*model.Channel{{Id: 2211}, {Id: 2212}, {Id: 2213}}
	ordered := orderRetryCandidates(candidates, 2211, "")
	require.Len(t, ordered, 3)
	assert.Equal(t, 2212, ordered[0].Id)
	assert.Equal(t, 2213, ordered[1].Id)
	assert.Equal(t, 2211, ordered[2].Id)
}

func TestCacheGetRandomSatisfiedChannelRetryPrefersAnotherRouteAndAllowsLastFallback(t *testing.T) {
	db := setupChannelSelectAutoGroupsTest(t)
	const modelName = "retry-route-model"
	createChannelSelectAutoGroupsChannel(t, db, 2301, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2302, "default", modelName)
	createChannelSelectAutoGroupsChannel(t, db, 2303, "default", modelName)
	baseA := "https://cpa-a.example"
	baseB := "https://cpa-b.example"
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2301).Update("base_url", baseA).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2302).Update("base_url", baseA).Error)
	require.NoError(t, db.Model(&model.Channel{}).Where("id = ?", 2303).Update("base_url", baseB).Error)
	model.InitChannelCache()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	retry := 0
	param := &RetryParam{
		Ctx:         ctx,
		TokenGroup:  "default",
		ModelName:   modelName,
		RequestPath: "/v1/chat/completions",
		Retry:       &retry,
	}
	first, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, first)
	param.RecordChannel(first)
	param.IncreaseRetry()

	second, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.NotEqual(t, first.Id, second.Id)
	assert.NotEqual(t, channelRetryRouteKey(first), channelRetryRouteKey(second))

	param.RecordChannel(second)
	param.IncreaseRetry()
	third, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.NotEqual(t, second.Id, third.Id)

	param.RecordChannel(third)
	param.IncreaseRetry()
	fourth, _, err := CacheGetRandomSatisfiedChannel(param)
	require.NoError(t, err)
	require.NotNil(t, fourth)
	assert.Equal(t, third.Id, fourth.Id, "same-channel fallback must retry the most recent channel deterministically")
}

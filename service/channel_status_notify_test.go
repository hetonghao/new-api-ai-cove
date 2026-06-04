package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestShouldSendChannelStatusNotificationDefaultsOff(t *testing.T) {
	truncate(t)

	require.False(t, shouldSendChannelStatusNotification(404))

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:   101,
		Name: "default-off",
	}).Error)
	require.False(t, shouldSendChannelStatusNotification(101))

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:            102,
		Name:          "empty-settings",
		OtherSettings: "{}",
	}).Error)
	require.False(t, shouldSendChannelStatusNotification(102))

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:            103,
		Name:          "explicit-off",
		OtherSettings: `{"channel_status_notify_enabled":false}`,
	}).Error)
	require.False(t, shouldSendChannelStatusNotification(103))
}

func TestShouldSendChannelStatusNotificationAllowsExplicitOptIn(t *testing.T) {
	truncate(t)

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:            201,
		Name:          "status-mail-enabled",
		OtherSettings: `{"channel_status_notify_enabled":true}`,
	}).Error)

	require.True(t, shouldSendChannelStatusNotification(201))
}

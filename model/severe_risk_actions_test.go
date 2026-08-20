package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestSevereRiskActions_disableUserAndKeepChannelQuarantined(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	user := &User{Username: "severe-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "severe-user"}
	channel := &Channel{Name: "severe-channel", Key: "key", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(user).Error)
	require.NoError(t, db.Create(channel).Error)

	// When
	require.NoError(t, DisableUserForSevereRisk(context.Background(), user.Id))
	require.True(t, QuarantineChannel(channel.Id, "", "invalid_prompt"))
	require.False(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "health check"))

	// Then
	var storedUser User
	var storedChannel Channel
	require.NoError(t, db.First(&storedUser, user.Id).Error)
	require.NoError(t, db.First(&storedChannel, channel.Id).Error)
	require.Equal(t, common.UserStatusDisabled, storedUser.Status)
	require.EqualValues(t, 2, storedUser.AuthVersion)
	require.Equal(t, common.ChannelStatusSevereDisabled, storedChannel.Status)
	require.True(t, UpdateChannelStatus(channel.Id, "", common.ChannelStatusEnabled, "manual severe review"))
}

func TestSevereRiskActions_quarantineOnlyMatchedMultiKey(t *testing.T) {
	// Given
	db := setupRiskRecordModelTest(t)
	channel := &Channel{
		Name: "multi-key-severe", Key: "key-a\nkey-b", Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling},
	}
	require.NoError(t, db.Create(channel).Error)

	// When
	require.True(t, QuarantineChannel(channel.Id, "key-a", "invalid_prompt"))

	// Then
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	require.Equal(t, common.ChannelStatusEnabled, stored.Status)
	require.Equal(t, common.ChannelStatusSevereDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	require.NotEqual(t, common.ChannelStatusSevereDisabled, stored.ChannelInfo.MultiKeyStatusList[1])
	require.False(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, "health check"))
}

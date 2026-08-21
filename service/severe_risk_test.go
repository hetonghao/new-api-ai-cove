package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestHandleSevereRiskEvent_quarantinesUserAndChannelAndRedactsContext(t *testing.T) {
	// Given
	require.NoError(t, model.DB.AutoMigrate(&model.SevereRiskRecord{}, &model.UserSession{}, &model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))
	user := &model.User{Username: "severe-service-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "severe-service-user"}
	channel := &model.Channel{Name: "severe-service-channel", Key: "service-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(channel).Error)
	token := &model.Token{UserId: user.Id, Name: "service-token", Key: "service-key-token", Status: common.TokenStatusEnabled}
	require.NoError(t, model.DB.Create(token).Error)
	t.Cleanup(func() {
		model.DB.Delete(&model.SevereRiskRecord{}, "request_id = ?", "severe-service-request")
		model.DB.Delete(&model.Token{}, token.Id)
		model.DB.Delete(&model.Channel{}, channel.Id)
		model.DB.Delete(&model.User{}, user.Id)
	})
	req := &dto.GeneralOpenAIRequest{
		Messages: []dto.Message{
			{Role: "user", Content: "keep this context"},
			{Role: "user", Content: []dto.MediaContent{
				{Type: dto.ContentTypeImageURL, ImageUrl: map[string]any{"url": "data:image/png;base64,secret-image"}},
				{Type: dto.ContentTypeFile, File: map[string]any{"file_data": "secret-file-data", "file_name": "secret-file-name", "file_id": "secret-file-id"}},
			}},
		},
		Metadata: []byte(`{"Authorization":"secret","safe":"value"}`),
	}
	apiErr := types.WithOpenAIError(types.OpenAIError{Message: "Invalid prompt: your prompt was flagged", Code: "invalid_prompt"}, http.StatusBadRequest)

	// When
	require.NoError(t, HandleSevereRiskEvent(SevereRiskEventInput{
		Context: context.Background(), Request: req, RequestID: "severe-service-request", ChannelID: channel.Id,
		ChannelName: channel.Name, UserID: user.Id, Username: user.Username, TokenID: token.Id, TokenName: token.Name,
		Model: "gpt-5.6-sol", Path: "/v1/responses", UpstreamErr: apiErr,
	}))
	require.NoError(t, HandleSevereRiskEvent(SevereRiskEventInput{
		Context: context.Background(), Request: req, RequestID: "severe-service-request", ChannelID: channel.Id,
		ChannelName: channel.Name, UserID: user.Id, Username: user.Username, TokenID: token.Id, TokenName: token.Name,
		Model: "gpt-5.6-sol", Path: "/v1/responses", UpstreamErr: apiErr,
	}))

	// Then
	var storedUser model.User
	var storedChannel model.Channel
	var record model.SevereRiskRecord
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	require.NoError(t, model.DB.First(&storedChannel, channel.Id).Error)
	require.NoError(t, model.DB.Where("request_id = ?", "severe-service-request").First(&record).Error)
	require.Equal(t, common.UserStatusDisabled, storedUser.Status)
	require.Equal(t, common.ChannelStatusSevereDisabled, storedChannel.Status)
	require.Equal(t, model.SevereRiskActionSuccess, record.UserActionStatus)
	require.Equal(t, model.SevereRiskActionSuccess, record.ChannelActionStatus)
	contextSnapshot, err := common.DecryptCredential(record.ContextEncrypted)
	require.NoError(t, err)
	require.Contains(t, contextSnapshot, "keep this context")
	require.NotContains(t, contextSnapshot, "Authorization")
	require.NotContains(t, contextSnapshot, "secret")
	require.NotContains(t, contextSnapshot, "secret-image")
	require.NotContains(t, contextSnapshot, "secret-file")
	require.Contains(t, contextSnapshot, "safe")

}

func TestHandleSevereRiskEvent_ignoresNonTerminalErrors(t *testing.T) {
	// Given
	require.NoError(t, model.DB.AutoMigrate(&model.SevereRiskRecord{}))
	apiErr := types.WithOpenAIError(types.OpenAIError{Message: "Invalid prompt", Code: "invalid_prompt"}, http.StatusInternalServerError)

	// When
	require.NoError(t, HandleSevereRiskEvent(SevereRiskEventInput{
		Context: context.Background(), Request: &dto.GeneralOpenAIRequest{Prompt: "prompt"}, RequestID: "non-terminal-request",
		ChannelID: 1, UserID: 1, TokenID: 1, Model: "gpt-5.6-sol", Path: "/v1/responses", UpstreamErr: apiErr,
	}))

	// Then
	var count int64
	require.NoError(t, model.DB.Model(&model.SevereRiskRecord{}).Where("request_id = ?", "non-terminal-request").Count(&count).Error)
	require.Zero(t, count)
}

func TestHandleSevereRiskEvent_recordsButSkipsQuarantineWhenDisabled(t *testing.T) {
	// Given
	require.NoError(t, model.DB.AutoMigrate(&model.SevereRiskRecord{}, &model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}))
	previous := common.SevereRiskAutoQuarantineEnabled
	common.SevereRiskAutoQuarantineEnabled = false
	t.Cleanup(func() { common.SevereRiskAutoQuarantineEnabled = previous })
	user := &model.User{Username: "severe-disabled-user", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, AuthVersion: 1, AffCode: "severe-disabled-user"}
	channel := &model.Channel{Name: "severe-disabled-channel", Key: "disabled-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(user).Error)
	require.NoError(t, model.DB.Create(channel).Error)
	token := &model.Token{UserId: user.Id, Name: "disabled-token", Key: "disabled-token-key", Status: common.TokenStatusEnabled}
	require.NoError(t, model.DB.Create(token).Error)
	t.Cleanup(func() {
		model.DB.Delete(&model.SevereRiskRecord{}, "request_id = ?", "severe-disabled-request")
		model.DB.Delete(&model.Token{}, token.Id)
		model.DB.Delete(&model.Channel{}, channel.Id)
		model.DB.Delete(&model.User{}, user.Id)
	})
	apiErr := types.WithOpenAIError(types.OpenAIError{Message: "Invalid prompt", Code: "invalid_prompt"}, http.StatusBadRequest)

	// When
	require.NoError(t, HandleSevereRiskEvent(SevereRiskEventInput{
		Context: context.Background(), Request: &dto.GeneralOpenAIRequest{Prompt: "prompt"}, RequestID: "severe-disabled-request", ChannelID: channel.Id,
		ChannelName: channel.Name, UserID: user.Id, Username: user.Username, TokenID: token.Id, TokenName: token.Name,
		Model: "gpt-5.6-sol", Path: "/v1/responses", UpstreamErr: apiErr,
	}))

	// Then
	var record model.SevereRiskRecord
	require.NoError(t, model.DB.Where("request_id = ?", "severe-disabled-request").First(&record).Error)
	var storedUser model.User
	var storedChannel model.Channel
	require.NoError(t, model.DB.First(&storedUser, user.Id).Error)
	require.NoError(t, model.DB.First(&storedChannel, channel.Id).Error)
	require.Equal(t, model.SevereRiskActionDisabled, record.UserActionStatus)
	require.Equal(t, model.SevereRiskActionDisabled, record.ChannelActionStatus)
	require.Equal(t, common.UserStatusEnabled, storedUser.Status)
	require.Equal(t, common.ChannelStatusEnabled, storedChannel.Status)
}

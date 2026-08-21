package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

type SevereRiskEventInput struct {
	Context     context.Context
	Request     dto.Request
	RequestID   string
	ChannelID   int
	ChannelName string
	UsingKey    string
	IsMultiKey  bool
	UserID      int
	Username    string
	TokenID     int
	TokenName   string
	Model       string
	Path        string
	UpstreamErr *types.NewAPIError
	RootUser    bool
	SystemToken bool
	ChannelTest bool
}

type SevereRiskRelayInput struct {
	Context     *gin.Context
	Request     dto.Request
	Channel     types.ChannelError
	Model       string
	UpstreamErr *types.NewAPIError
	ChannelTest bool
}

func HandleSevereRiskEvent(input SevereRiskEventInput) error {
	if input.Context == nil {
		input.Context = context.Background()
	}
	if input.UpstreamErr == nil || !input.UpstreamErr.IsUpstreamInvalidPrompt() || input.RootUser || input.SystemToken || input.ChannelTest {
		return nil
	}
	if input.RequestID == "" || input.ChannelID <= 0 || input.UserID <= 0 || input.TokenID <= 0 || input.Model == "" || input.Path == "" || input.Request == nil {
		return nil
	}

	snapshot, hash, err := buildSevereRiskContext(input.Request)
	if err != nil {
		return fmt.Errorf("build severe risk context: %w", err)
	}
	ciphertext, err := common.EncryptCredential(snapshot)
	if err != nil {
		return fmt.Errorf("encrypt severe risk context: %w", err)
	}
	scope := model.SevereRiskChannelScopeAll
	fingerprint := ""
	if input.IsMultiKey && strings.TrimSpace(input.UsingKey) != "" {
		scope = model.SevereRiskChannelScopeKey
		fingerprint = common.GenerateHMAC(strings.TrimSpace(input.UsingKey))
	}
	recordInput := model.SevereRiskRecordInput{
		RequestID: input.RequestID, ChannelID: input.ChannelID, ChannelName: input.ChannelName,
		UserID: input.UserID, Username: input.Username, TokenID: input.TokenID, TokenName: input.TokenName,
		Model: input.Model, Path: input.Path, ErrorCode: "invalid_prompt", ErrorDetail: input.UpstreamErr.Error(),
		ContextHash: hash, ContextEncrypted: ciphertext, ChannelScope: scope, ChannelKeyFingerprint: fingerprint,
		TriggeredAt: time.Now().UTC(),
	}
	if err := model.RecordSevereRiskEvent(input.Context, recordInput); err != nil {
		return err
	}
	claimed, err := model.ClaimSevereRiskAction(input.Context, input.RequestID)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	if !common.SevereRiskAutoQuarantineEnabled {
		return model.UpdateSevereRiskActionStatus(input.Context, input.RequestID, model.SevereRiskActionDisabled, model.SevereRiskActionDisabled)
	}

	userStatus := model.SevereRiskActionFailed
	if err := model.DisableUserForSevereRisk(input.Context, input.UserID); err == nil {
		userStatus = model.SevereRiskActionSuccess
	} else {
		common.SysError(fmt.Sprintf("severe risk user quarantine failed for user %d: %v", input.UserID, err))
	}
	channelStatus := model.SevereRiskActionFailed
	if model.QuarantineChannel(input.ChannelID, input.UsingKey, input.UpstreamErr.Error()) {
		channelStatus = model.SevereRiskActionSuccess
	}
	if err := model.UpdateSevereRiskActionStatus(input.Context, input.RequestID, userStatus, channelStatus); err != nil {
		return err
	}
	return nil
}

func HandleSevereRiskFromRelay(input SevereRiskRelayInput) {
	c := input.Context
	if c == nil {
		return
	}
	channelError := input.Channel
	if severeErr := HandleSevereRiskEvent(SevereRiskEventInput{
		Context: c.Request.Context(), Request: input.Request, RequestID: c.GetString(common.RequestIdKey),
		ChannelID: channelError.ChannelId, ChannelName: channelError.ChannelName, UsingKey: channelError.UsingKey,
		IsMultiKey: channelError.IsMultiKey, UserID: c.GetInt("id"), Username: c.GetString("username"),
		TokenID: c.GetInt("token_id"), TokenName: c.GetString("token_name"), Model: input.Model,
		Path: c.Request.URL.Path, UpstreamErr: input.UpstreamErr, RootUser: c.GetInt("role") == common.RoleRootUser,
		SystemToken: c.GetBool("token_system_managed"), ChannelTest: input.ChannelTest,
	}); severeErr != nil {
		common.SysError(fmt.Sprintf("record severe risk event failed for request %s: %v", c.GetString(common.RequestIdKey), severeErr))
	}
}

func buildSevereRiskContext(request dto.Request) (string, string, error) {
	data, err := common.Marshal(request)
	if err != nil {
		return "", "", err
	}
	var value any
	if err := common.Unmarshal(data, &value); err != nil {
		return "", "", err
	}
	sanitizeSevereRiskValue(value)
	data, err = common.Marshal(value)
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(data)
	return string(data), fmt.Sprintf("%x", digest[:]), nil
}

func sanitizeSevereRiskValue(value any) {
	switch current := value.(type) {
	case map[string]any:
		for key, nested := range current {
			if severeRiskSensitiveKey(key) || severeRiskAttachmentKey(key) || isDataURI(nested) {
				delete(current, key)
				continue
			}
			sanitizeSevereRiskValue(nested)
		}
	case []any:
		for _, nested := range current {
			sanitizeSevereRiskValue(nested)
		}
	}
}

func severeRiskAttachmentKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), " ", "_"))
	if normalized == "file" || normalized == "image" || normalized == "filename" || normalized == "file_id" {
		return true
	}
	for _, marker := range []string{"image_url", "input_audio", "file_data", "attachment", "binary", "video", "audio"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func isDataURI(value any) bool {
	text, ok := value.(string)
	return ok && strings.HasPrefix(strings.ToLower(strings.TrimSpace(text)), "data:")
}

func severeRiskSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"), " ", "_"))
	for _, marker := range []string{"authorization", "cookie", "api_key", "apikey", "session", "access_token", "credential", "client_secret", "secret"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

package qualityinspect

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
)

func ValidateRuntime(cfg model.QualityConfig, target int) (*model.Token, error) {
	if err := model.ValidateQualityConfig(cfg); err != nil {
		return nil, err
	}
	settings, _, err := model.GetQualitySettings()
	if err != nil {
		return nil, err
	}
	if !settings.Enabled || !slices.Contains(settings.TokenIDs, cfg.TokenID) {
		return nil, errors.New("execution identity is not approved")
	}
	token, err := ValidateExecutionIdentity(cfg.TokenID)
	if err != nil {
		return nil, err
	}
	user, err := model.GetUserById(token.UserId, false)
	if err != nil {
		return nil, errors.New("execution identity is unavailable")
	}
	group := token.Group
	if group == "" {
		group = user.Group
	}
	if cfg.Group != group {
		return nil, errors.New("test group differs from execution token group")
	}
	if token.ModelLimitsEnabled && !middleware.TokenModelLimitAllows(token.GetModelLimitsMap(), cfg.Model) {
		return nil, errors.New("execution identity does not allow this model")
	}
	if cfg.Mode == "route" {
		if target != 0 {
			return nil, errors.New("normal routing cannot pin channels")
		}
		return token, nil
	}
	if !slices.Contains(cfg.ChannelIDs, target) {
		return nil, errors.New("channel is outside the saved test targets")
	}
	channel, err := model.GetChannelById(target, false)
	if err != nil || channel.Status != common.ChannelStatusEnabled {
		return nil, errors.New("target channel is unavailable")
	}
	if !slices.Contains(channel.GetModels(), cfg.Model) || (group != "auto" && !slices.Contains(strings.Split(channel.Group, ","), group)) {
		return nil, errors.New("target channel does not support the model or group")
	}
	return token, nil
}

func ValidateExecutionIdentity(id int) (*model.Token, error) {
	token, err := model.GetTokenById(id)
	if err != nil {
		return nil, errors.New("execution identity is unavailable")
	}
	if token.SystemManaged || token.UnlimitedQuota || token.Status != common.TokenStatusEnabled || (token.ExpiredTime != -1 && token.ExpiredTime <= time.Now().Unix()) || token.RemainQuota <= 0 {
		return nil, errors.New("execution identity is inactive or has no finite quota")
	}
	user, err := model.GetUserById(token.UserId, false)
	if err != nil || user.Status != common.UserStatusEnabled || user.Role < common.RoleAdminUser || !authz.Can(user.Id, user.Role, authz.ChannelOperate) {
		return nil, errors.New("execution identity lacks channel operation permission")
	}
	return token, nil
}

func BuildRequest(cfg model.QualityConfig) (string, []byte, error) {
	if err := model.ValidateQualityConfig(cfg); err != nil {
		return "", nil, err
	}
	messages := make([]map[string]string, 0, 2)
	if cfg.Instruction != "" {
		messages = append(messages, map[string]string{"role": cfg.InstructionRole, "content": cfg.Instruction})
	}
	messages = append(messages, map[string]string{"role": "user", "content": cfg.Prompt})
	body := map[string]any{"model": cfg.Model, "stream": true}
	path := "/v1/responses"
	if cfg.Protocol == "responses" {
		body["input"] = messages
		body["max_output_tokens"] = cfg.MaxOutputTokens
		body["store"] = false
		if cfg.ReasoningEffort != "" {
			body["reasoning"] = map[string]string{"effort": cfg.ReasoningEffort}
		}
	} else {
		path = "/v1/chat/completions"
		body["messages"] = messages
		body["stream_options"] = map[string]bool{"include_usage": true}
		limitKey := "max_tokens"
		for _, prefix := range []string{"gpt-5", "gpt-6", "o1", "o3", "o4"} {
			if strings.HasPrefix(cfg.Model, prefix) {
				limitKey = "max_completion_tokens"
				break
			}
		}
		body[limitKey] = cfg.MaxOutputTokens
		if cfg.ReasoningEffort != "" {
			body["reasoning_effort"] = cfg.ReasoningEffort
		}
	}
	if cfg.Temperature != nil {
		body["temperature"] = *cfg.Temperature
	}
	if cfg.TopP != nil {
		body["top_p"] = *cfg.TopP
	}
	data, err := common.Marshal(body)
	return path, data, err
}

func RunSample(ctx context.Context, cfg model.QualityConfig, target int) model.QualityResult {
	result := model.QualityResult{Status: "skipped", ErrorCode: "invalid_configuration"}
	token, err := ValidateRuntime(cfg, target)
	if err != nil {
		return result
	}
	path, body, err := BuildRequest(cfg)
	if err != nil {
		return result
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return result
	}
	base := "http://" + net.JoinHostPort("127.0.0.1", port)
	key := "sk-" + token.GetFullKey()
	if target > 0 {
		key += "-" + strconv.Itoa(target)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	result = execute(ctx, client, base+path, key, body, cfg)
	if target > 0 {
		result.ChannelID = target
		if channel, err := model.GetChannelById(target, false); err == nil {
			result.ChannelName = channel.Name
		}
	} else if id, name, err := model.QualityRequestChannel(result.RequestID, cfg.TokenID); err == nil {
		result.ChannelID, result.ChannelName = id, name
	}
	return result
}

func execute(parent context.Context, client *http.Client, url, key string, body []byte, cfg model.QualityConfig) model.QualityResult {
	result := model.QualityResult{Status: "failed", ErrorCode: "upstream_error"}
	ctx, cancel := context.WithTimeout(parent, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	start := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		result.ErrorCode = "invalid_configuration"
		return result
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.ErrorCode = "timeout"
		}
		if errors.Is(parent.Err(), context.Canceled) {
			result.Status, result.ErrorCode = "cancelled", "cancelled"
		}
		return result
	}
	defer response.Body.Close()
	result.RequestID = response.Header.Get(common.RequestIdKey)
	if len(result.RequestID) > 128 {
		result.RequestID = ""
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		result.ErrorCode = "http_" + strconv.Itoa(response.StatusCode)
		return result
	}
	parsed := ParseResponse(response.Body, response.Header.Get("Content-Type"), cfg.Protocol, start)
	parsed.RequestID = result.RequestID
	duration := time.Since(start).Milliseconds()
	parsed.DurationMs = &duration
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		parsed.Status, parsed.ErrorCode = "failed", "timeout"
	}
	if errors.Is(parent.Err(), context.Canceled) {
		parsed.Status, parsed.ErrorCode = "cancelled", "cancelled"
	}
	if parsed.Status != "succeeded" {
		return parsed
	}
	if cfg.OutputType == "svg" {
		svg, code := ExtractSafeSVG(string(parsed.Text))
		if code != "" {
			parsed.Status, parsed.ErrorCode, parsed.Validation = "failed", code, code
			return parsed
		}
		parsed.SVG = []byte(svg)
		parsed.Validation = "safe"
	} else {
		parsed.Validation = "not_applicable"
	}
	return parsed
}

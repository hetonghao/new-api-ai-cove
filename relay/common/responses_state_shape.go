package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	napicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// responsesStateShape 描述一类把会话状态交给上游的 Responses 请求形态。
// 无状态上游中转站会在 HTTP 上拒绝这些形态，因此网关把上游的拒绝事实记在
// (渠道, 模型, 形态) 上，之后直接在本地拒绝，不再白跑一次上游。
// 状态形态只描述请求形状，不代表上游的永久能力，因此带有效期。
type responsesStateShape string

const (
	// responsesStateShapeNone 表示未识别出状态形态。
	responsesStateShapeNone responsesStateShape = ""
	// responsesStateShapeContinuation 表示请求带 previous_response_id，期望上游保留会话状态。
	responsesStateShapeContinuation responsesStateShape = "http_continuation"
	// responsesStateShapeInput 表示请求的 input 为空，没有任何新的输入项。
	responsesStateShapeInput responsesStateShape = "non_empty_input"
)

// responsesStateShapeTTL 是学习结果的有效期。本地拒绝不续期，因此到期后会放一次请求到
// 上游重新确认；上游恢复支持该形态时能自动解除。
const responsesStateShapeTTL = 30 * time.Minute

const responsesStateShapeRedisPrefix = "responses_state_shape"

// detectResponsesStateShapeFromError 从上游 400 响应里识别“该上游不支持的状态形态”。
func detectResponsesStateShapeFromError(statusCode int, body []byte) responsesStateShape {
	if statusCode != http.StatusBadRequest {
		return responsesStateShapeNone
	}
	lowered := strings.ToLower(string(body))
	// 输入类文案：`One of "input" or "previous_response_id" ... must be provided.`
	// 和 `input is required`。第一条同时包含 previous_response_id，所以顺序不能反。
	if strings.Contains(lowered, "input") &&
		(strings.Contains(lowered, "must be provided") || strings.Contains(lowered, "is required")) {
		return responsesStateShapeInput
	}
	// 续传类文案只认能力措辞。`previous_response_not_found` 这类“这个 response id 失效了”
	// 的 400 是一次性可恢复错误，不能当成渠道能力。
	if strings.Contains(lowered, "previous_response_id") &&
		(strings.Contains(lowered, "not available") ||
			strings.Contains(lowered, "not supported") ||
			strings.Contains(lowered, "unsupported") ||
			strings.Contains(lowered, "only supported")) {
		return responsesStateShapeContinuation
	}
	return responsesStateShapeNone
}

// emptyResponsesInput 判断 input 是否为空：缺失、null、[]、{}、"" 和纯空白字符串都算空。
func emptyResponsesInput(input json.RawMessage) bool {
	trimmed := bytes.TrimSpace(input)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	last := trimmed[len(trimmed)-1]
	switch trimmed[0] {
	case '[':
		return len(trimmed) >= 2 && last == ']' && len(bytes.TrimSpace(trimmed[1:len(trimmed)-1])) == 0
	case '{':
		return len(trimmed) >= 2 && last == '}' && len(bytes.TrimSpace(trimmed[1:len(trimmed)-1])) == 0
	case '"':
		return len(trimmed) >= 2 && last == '"' && len(bytes.TrimSpace(trimmed[1:len(trimmed)-1])) == 0
	default:
		return false
	}
}

// ResponsesStateShapeRejection 返回本地拒绝错误；nil 表示可以继续请求上游。
func ResponsesStateShapeRejection(channelID int, model, previousResponseID string, input json.RawMessage) *types.NewAPIError {
	if channelID <= 0 {
		return nil
	}
	return responsesStateShapeRejection(func(shape responsesStateShape) bool {
		return responsesStateShapeUnsupported(channelID, model, shape)
	}, previousResponseID, input)
}

func responsesStateShapeRejection(unsupported func(responsesStateShape) bool, previousResponseID string, input json.RawMessage) *types.NewAPIError {
	if strings.TrimSpace(previousResponseID) != "" {
		if unsupported(responsesStateShapeContinuation) {
			return responsesStateShapeError("previous_response_id is not supported by this upstream; resend the full input without it")
		}
		return nil
	}
	if emptyResponsesInput(input) && unsupported(responsesStateShapeInput) {
		return responsesStateShapeError("input must contain at least one item on this upstream; resend the full input")
	}
	return nil
}

func responsesStateShapeError(message string) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		errors.New(message),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	)
}

// responsesStateShapeStore 是学习结果的进程内副本，按显式时间判断有效期，便于测试。
// 过期条目在下一次读取同一个键时删除；键的数量只受渠道数与模型数限制。
type responsesStateShapeStore struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func newResponsesStateShapeStore() *responsesStateShapeStore {
	return &responsesStateShapeStore{entries: make(map[string]time.Time)}
}

func (s *responsesStateShapeStore) remember(key string, ttl time.Duration, now time.Time) {
	if s == nil || key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = now.Add(ttl)
}

func (s *responsesStateShapeStore) remembered(key string, now time.Time) bool {
	if s == nil || key == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.entries[key]
	if !ok {
		return false
	}
	if !now.Before(expiresAt) {
		delete(s.entries, key)
		return false
	}
	return true
}

var responsesStateShapeMemory = newResponsesStateShapeStore()

func responsesStateShapeKey(channelID int, model string, shape responsesStateShape) string {
	return fmt.Sprintf("channel=%d:model=%s:shape=%s", channelID, strings.TrimSpace(model), shape)
}

// markResponsesStateShapeUnsupported 记录上游拒绝过该状态形态。进程内副本总是写入，
// Redis 可用时额外共享给其它节点；读取时先查 Redis，未命中再回落到进程内副本。
func markResponsesStateShapeUnsupported(channelID int, model string, shape responsesStateShape) {
	if channelID <= 0 || shape == responsesStateShapeNone {
		return
	}
	key := responsesStateShapeKey(channelID, model, shape)
	responsesStateShapeMemory.remember(key, responsesStateShapeTTL, time.Now())
	if napicommon.RedisEnabled && napicommon.RDB != nil {
		if err := napicommon.RedisSet(responsesStateShapeRedisPrefix+":"+key, "1", responsesStateShapeTTL); err != nil {
			napicommon.SysError("responses state shape redis set failed: " + err.Error())
		}
	}
}

// responsesStateShapeUnsupported 查询该渠道与模型是否被上游拒绝过该状态形态。
func responsesStateShapeUnsupported(channelID int, model string, shape responsesStateShape) bool {
	if channelID <= 0 || shape == responsesStateShapeNone {
		return false
	}
	key := responsesStateShapeKey(channelID, model, shape)
	if napicommon.RedisEnabled && napicommon.RDB != nil {
		if value, err := napicommon.RedisGet(responsesStateShapeRedisPrefix + ":" + key); err == nil && value != "" {
			return true
		}
	}
	return responsesStateShapeMemory.remembered(key, time.Now())
}

// ObserveResponsesUpstreamFailure 在上游返回错误时学习状态形态。响应体会被还原，
// 调用方随后仍可交给 RelayErrorHandler 读取。
func ObserveResponsesUpstreamFailure(c *gin.Context, channelID int, model string, resp *http.Response) {
	if resp == nil || channelID <= 0 {
		return
	}
	shape := detectResponsesStateShapeFromError(resp.StatusCode, ReadAndRestoreResponseBody(resp))
	if shape == responsesStateShapeNone {
		return
	}
	markResponsesStateShapeUnsupported(channelID, model, shape)
	if c != nil && c.Request != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf(
			"responses state shape learned: channel=%d model=%q shape=%s ttl=%s",
			channelID, model, shape, responsesStateShapeTTL,
		))
	}
}

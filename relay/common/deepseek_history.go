package common

import (
	"encoding/json"
	"strings"
	"time"

	napicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/gin-gonic/gin"
)

const (
	deepSeekHistoryTTL          = 2 * time.Hour
	deepSeekResponsesInputKey   = "deepseek_responses_input"
	deepSeekResponsesRequestKey = "deepseek_responses_request"
)

type deepSeekHistorySnapshot struct {
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
}

func StashDeepSeekResponsesInput(c *gin.Context, input json.RawMessage) {
	if c == nil || len(input) == 0 {
		return
	}
	c.Set(deepSeekResponsesInputKey, append(json.RawMessage(nil), input...))
}

// StashDeepSeekResponsesRequest keeps the converted /responses request of the
// current turn so a payload the upstream refuses can be rebuilt once without
// repeating the whole conversion.
func StashDeepSeekResponsesRequest(c *gin.Context, request dto.OpenAIResponsesRequest) {
	if c == nil {
		return
	}
	c.Set(deepSeekResponsesRequestKey, request)
}

func StashedDeepSeekResponsesRequest(c *gin.Context) (dto.OpenAIResponsesRequest, bool) {
	if c == nil {
		return dto.OpenAIResponsesRequest{}, false
	}
	value, ok := c.Get(deepSeekResponsesRequestKey)
	if !ok {
		return dto.OpenAIResponsesRequest{}, false
	}
	request, ok := value.(dto.OpenAIResponsesRequest)
	return request, ok
}

func SaveDeepSeekHistory(c *gin.Context, responseID string, output []dto.ResponsesOutput) {
	if !napicommon.RedisEnabled || napicommon.RDB == nil {
		return
	}
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return
	}
	outRaw, err := napicommon.Marshal(output)
	if err != nil {
		return
	}
	snap, err := napicommon.Marshal(deepSeekHistorySnapshot{
		Input:  stashedDeepSeekResponsesInput(c),
		Output: outRaw,
	})
	if err != nil {
		return
	}
	_ = napicommon.RedisSet(deepSeekHistoryIDKey(responseID), string(snap), deepSeekHistoryTTL)
}

func LoadDeepSeekHistory(previousResponseID string) (input, output json.RawMessage, ok bool) {
	if !napicommon.RedisEnabled || napicommon.RDB == nil {
		return nil, nil, false
	}
	previousResponseID = strings.TrimSpace(previousResponseID)
	if previousResponseID == "" {
		return nil, nil, false
	}
	text, err := napicommon.RedisGet(deepSeekHistoryIDKey(previousResponseID))
	if err != nil || text == "" {
		return nil, nil, false
	}
	var snap deepSeekHistorySnapshot
	if napicommon.Unmarshal([]byte(text), &snap) != nil {
		return nil, nil, false
	}
	return snap.Input, snap.Output, true
}

func stashedDeepSeekResponsesInput(c *gin.Context) json.RawMessage {
	if c == nil {
		return nil
	}
	v, ok := c.Get(deepSeekResponsesInputKey)
	if !ok {
		return nil
	}
	raw, _ := v.(json.RawMessage)
	return raw
}

func deepSeekHistoryIDKey(responseID string) string {
	return "ds_hist:id:" + responseID
}

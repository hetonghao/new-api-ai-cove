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
	deepSeekHistoryTTL        = 2 * time.Hour
	deepSeekResponsesInputKey = "deepseek_responses_input"
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

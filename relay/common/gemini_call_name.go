package common

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	napicommon "github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const geminiCallNameTTL = 30 * time.Minute

var geminiCallNameFallback sync.Map

func SaveGeminiCallName(callID, name string) {
	callID = strings.TrimSpace(callID)
	name = strings.TrimSpace(name)
	if callID == "" || name == "" {
		return
	}
	if napicommon.RedisEnabled && napicommon.RDB != nil {
		_ = napicommon.RedisSet(geminiCallNameKey(callID), name, geminiCallNameTTL)
		return
	}
	geminiCallNameFallback.Store(callID, name)
}

func LoadGeminiCallName(callID string) string {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return ""
	}
	if napicommon.RedisEnabled && napicommon.RDB != nil {
		text, err := napicommon.RedisGet(geminiCallNameKey(callID))
		if err == nil {
			return text
		}
		return ""
	}
	value, ok := geminiCallNameFallback.Load(callID)
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

func ObserveGeminiCallName(item *dto.ResponsesOutput) {
	if item == nil || item.Type != "function_call" {
		return
	}
	SaveGeminiCallName(item.CallId, item.Name)
}

func HydrateGeminiFunctionOutputNames(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []map[string]any
	if json.Unmarshal(input, &items) != nil {
		return input
	}
	local := make(map[string]string)
	for _, item := range items {
		if strings.TrimSpace(asString(item["type"])) != "function_call" {
			continue
		}
		callID := strings.TrimSpace(asString(item["call_id"]))
		name := strings.TrimSpace(asString(item["name"]))
		if callID == "" || name == "" {
			continue
		}
		local[callID] = name
		SaveGeminiCallName(callID, name)
	}
	changed := false
	for i, item := range items {
		if strings.TrimSpace(asString(item["type"])) != "function_call_output" {
			continue
		}
		if strings.TrimSpace(asString(item["name"])) != "" {
			continue
		}
		callID := strings.TrimSpace(asString(item["call_id"]))
		name := local[callID]
		if name == "" {
			name = LoadGeminiCallName(callID)
		}
		if name == "" {
			continue
		}
		items[i]["name"] = name
		changed = true
	}
	if !changed {
		return input
	}
	raw, err := json.Marshal(items)
	if err != nil {
		return input
	}
	return raw
}

func geminiCallNameKey(callID string) string {
	return "gemini_fc_name:" + callID
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

package common

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func IsOfficialOpenAIResponsesURL(rawURL string) bool {
	target, err := url.Parse(rawURL)
	return err == nil && (target.Scheme == "https" || target.Scheme == "wss") &&
		strings.EqualFold(target.Hostname(), "api.openai.com") &&
		(target.Port() == "" || target.Port() == "443") && target.Path == "/v1/responses"
}

// NormalizeOfficialOpenAIResponsesItemIDs repairs replayed item IDs without
// changing call_id links, opaque reasoning, or server-owned item references.
func NormalizeOfficialOpenAIResponsesItemIDs(rawURL string, body []byte) ([]byte, error) {
	if !IsOfficialOpenAIResponsesURL(rawURL) {
		return body, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, nil
	}
	items := input.Array()
	occupied := make(map[string]bool, len(items))
	referenced := make(map[string]bool)
	for _, item := range items {
		id := item.Get("id").String()
		occupied[id] = true
		if item.Get("type").String() == "item_reference" {
			referenced[id] = true
		}
	}
	mapped := make(map[string]string)
	rawItems := make([]string, len(items))
	changed := false
	for i, item := range items {
		rawItems[i] = item.Raw
		var prefix string
		switch item.Get("type").String() {
		case "":
			if item.Get("role").String() == "" {
				continue
			}
			prefix = "msg"
		case "message":
			prefix = "msg"
		case "function_call":
			prefix = "fc"
		case "custom_tool_call":
			prefix = "ctc"
		case "custom_tool_call_output":
			prefix = "ctco"
		default:
			continue
		}
		idValue := item.Get("id")
		id := idValue.String()
		if idValue.Type != gjson.String || id == "" || strings.HasPrefix(id, prefix) || referenced[id] {
			continue
		}
		key := prefix + "\x00" + id
		normalized, ok := mapped[key]
		if !ok {
			normalized = prefix + "_" + id
			for attempt := 0; occupied[normalized] || len(normalized) > 64; attempt++ {
				sum := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", key, attempt)))
				normalized = fmt.Sprintf("%s_%x", prefix, sum[:16])
			}
			mapped[key], occupied[normalized] = normalized, true
		}
		updated, err := sjson.Set(item.Raw, "id", normalized)
		if err != nil {
			return nil, fmt.Errorf("normalize Responses input item %d: %w", i, err)
		}
		rawItems[i], changed = updated, true
	}
	if !changed {
		return body, nil
	}
	return sjson.SetRawBytes(body, "input", []byte("["+strings.Join(rawItems, ",")+"]"))
}

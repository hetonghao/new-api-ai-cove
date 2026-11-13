package common

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func IsOfficialOpenAIResponsesURL(rawURL string) bool {
	return IsOfficialOpenAIURL(rawURL, "/v1/responses")
}

// NormalizeOfficialOpenAIResponsesItemIDs repairs replayed item identifiers and
// foreign reasoning metadata, preserving content and server-owned references.
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
	rawItems := make([]string, 0, len(items))
	changed := false
	for i, item := range items {
		rawItems = append(rawItems, item.Raw)
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
		case "reasoning":
			id := item.Get("id")
			if referenced[id.Str] {
				continue
			}
			encrypted := item.Get("encrypted_content")
			native := encrypted.Type == gjson.String && strings.HasPrefix(encrypted.Str, "gAAAA")
			foreign := encrypted.Type == gjson.String && strings.HasPrefix(encrypted.Str, "sealed.v1.")
			if !native && !foreign && encrypted.Type != gjson.Null && (encrypted.Type != gjson.String || encrypted.Str != "") {
				continue
			}
			if native || (!foreign && strings.HasPrefix(id.Str, "rs")) {
				updated, err := sjson.Delete(item.Raw, "status")
				if err == nil && id.Str != "" && !strings.HasPrefix(id.Str, "rs") {
					updated, err = sjson.Delete(updated, "id")
				}
				if err != nil {
					return nil, fmt.Errorf("normalize Responses reasoning item %d: %w", i, err)
				}
				rawItems[len(rawItems)-1] = updated
				changed = changed || updated != item.Raw
				continue
			}
			var texts []string
			for _, field := range []string{"summary", "content"} {
				for _, part := range item.Get(field).Array() {
					if text := part.Get("text"); text.Type == gjson.String && text.Str != "" {
						texts = append(texts, text.Str)
					}
				}
			}
			// Unsigned summaries are not rendered as reasoning by the official API.
			if len(texts) == 0 {
				rawItems = rawItems[:len(rawItems)-1]
			} else {
				text, err := json.Marshal("Previous reasoning summary:\n" + strings.Join(texts, "\n\n"))
				if err != nil {
					return nil, fmt.Errorf("encode Responses reasoning summary %d: %w", i, err)
				}
				rawItems[len(rawItems)-1] = `{"type":"message","role":"assistant","content":[{"type":"output_text","text":` + string(text) + `}]}`
			}
			changed = true
			continue
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
		rawItems[len(rawItems)-1], changed = updated, true
	}
	if !changed {
		return body, nil
	}
	return sjson.SetRawBytes(body, "input", []byte("["+strings.Join(rawItems, ",")+"]"))
}

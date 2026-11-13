package common

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func IsOfficialOpenAIURL(rawURL, path string) bool {
	target, err := url.Parse(rawURL)
	return err == nil && (target.Scheme == "https" || target.Scheme == "wss") &&
		strings.EqualFold(target.Hostname(), "api.openai.com") &&
		(target.Port() == "" || target.Port() == "443") && target.Path == path
}

// NormalizeOfficialOpenAIChatRequest disables reasoning only for models whose
// Chat Completions function calling requires none; Responses keeps reasoning.
// https://developers.openai.com/api/docs/guides/latest-model#migration-quickstart
func NormalizeOfficialOpenAIChatRequest(rawURL string, info *RelayInfo, body []byte) ([]byte, error) {
	if !IsOfficialOpenAIURL(rawURL, "/v1/chat/completions") {
		return body, nil
	}
	model := gjson.GetBytes(body, "model").String()
	compatible := false
	for _, base := range []string{"gpt-6-luna", "gpt-6-sol"} {
		if model == base {
			compatible = true
			break
		}
		if snapshot, ok := strings.CutPrefix(model, base+"-"); ok {
			_, err := time.Parse(time.DateOnly, snapshot)
			compatible = err == nil
			break
		}
	}
	functions := gjson.GetBytes(body, "functions")
	if !compatible || (!gjson.GetBytes(body, `tools.#(type=="function")`).Exists() &&
		(!functions.IsArray() || len(functions.Array()) == 0)) {
		return body, nil
	}
	updated, err := sjson.SetBytes(body, "reasoning_effort", "none")
	if err != nil {
		return nil, fmt.Errorf("normalize OpenAI Chat function reasoning: %w", err)
	}
	info.SetReasoningEffort("none")
	return updated, nil
}

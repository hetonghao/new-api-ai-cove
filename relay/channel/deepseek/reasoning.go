package deepseek

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// injectCachedDeepSeekReasoning keeps the upstream contract "thinking mode must
// receive reasoning_text" satisfied for the turn that is still being continued.
// It only ever inserts one item directly above the trailing tool run: rewriting
// items that already went upstream invalidates the prompt cache from that point
// on, which pinned a production conversation at 62k cached tokens of an 800k
// context because every turn changed the bytes the upstream had already cached.
func injectCachedDeepSeekReasoning(info *relaycommon.RelayInfo, request *dto.OpenAIResponsesRequest, previousResponseID string) {
	if request == nil || !relaycommon.IsDeepSeekReasoningRelay(info, request.Model) {
		return
	}
	cached := relaycommon.LoadDeepSeekReasoning(info, previousResponseID)
	request.Input = repairDeepSeekTrailingToolRun(request.Input, cached)
}

func repairDeepSeekTrailingToolRun(input json.RawMessage, cached string) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
	if regrouped, ok := regroupDeepSeekToolRuns(items); ok {
		items, changed = regrouped, true
	}
	if strings.TrimSpace(cached) != "" {
		if insertAt, ok := trailingToolRunMissingReasoning(items); ok {
			if filled := reasoningItemJSON("", cached); len(filled) > 0 {
				next := make([]json.RawMessage, 0, len(items)+1)
				next = append(next, items[:insertAt]...)
				next = append(next, filled)
				next = append(next, items[insertAt:]...)
				items, changed = next, true
			}
		}
	}
	if !changed {
		return input
	}
	raw, err := common.Marshal(items)
	if err != nil {
		return input
	}
	return raw
}

// forceRepairDeepSeekReasoning is the retry-only repair for a payload the
// upstream refused with the thinking-mode reasoning_text error: it fills every
// empty reasoning item and puts reasoning_text above every tool run. It rewrites
// the prefix the upstream already cached, so the happy path never calls it.
func forceRepairDeepSeekReasoning(input json.RawMessage, cached string) (json.RawMessage, bool) {
	if len(input) == 0 || strings.TrimSpace(cached) == "" {
		return input, false
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input, false
	}
	changed := false
	if regrouped, ok := regroupDeepSeekToolRuns(items); ok {
		items, changed = regrouped, true
	}
	for i, item := range items {
		if peekType(item) != "reasoning" || reasoningItemHasText(item) {
			continue
		}
		// Keep the original id: the upstream uses it to associate the reasoning
		// with the call it belongs to.
		if filled := reasoningItemJSON(reasoningItemID(item), cached); len(filled) > 0 {
			items[i], changed = filled, true
		}
	}
	filled := reasoningItemJSON("", cached)
	if len(filled) == 0 {
		return input, false
	}
	repaired := make([]json.RawMessage, 0, len(items)+4)
	runChecked := false
	for _, item := range items {
		if !isDeepSeekToolItem(item) {
			repaired = append(repaired, item)
			runChecked = false
			continue
		}
		if !runChecked {
			previous := len(repaired) - 1
			if previous < 0 || !reasoningItemHasText(repaired[previous]) {
				repaired = append(repaired, filled)
				changed = true
			}
			runChecked = true
		}
		repaired = append(repaired, item)
	}
	if !changed {
		return input, false
	}
	raw, err := common.Marshal(repaired)
	if err != nil {
		return input, false
	}
	return raw, true
}

func regroupDeepSeekToolRuns(items []json.RawMessage) ([]json.RawMessage, bool) {
	out := make([]json.RawMessage, 0, len(items))
	changed := false
	i := 0
	for i < len(items) {
		if !isDeepSeekToolCall(items[i]) && !isDeepSeekToolOutput(items[i]) {
			out = append(out, items[i])
			i++
			continue
		}
		j := i
		for j < len(items) && (isDeepSeekToolCall(items[j]) || isDeepSeekToolOutput(items[j])) {
			j++
		}
		run := items[i:j]
		calls := make([]json.RawMessage, 0, len(run))
		outs := make([]json.RawMessage, 0, len(run))
		seenOut := false
		grouped := true
		callIDs := make(map[string]struct{}, len(run))
		outputIDs := make(map[string]struct{}, len(run))
		var idlessCall bool
		for _, item := range run {
			if isDeepSeekToolOutput(item) {
				seenOut = true
				outs = append(outs, item)
				if id := toolCallID(item); id != "" {
					outputIDs[id] = struct{}{}
				}
				continue
			}
			if seenOut {
				grouped = false
			}
			calls = append(calls, item)
			if id := toolCallID(item); id == "" {
				idlessCall = true
			} else {
				callIDs[id] = struct{}{}
			}
		}
		// Hoisting a call above the outputs leaves it dangling when its own output
		// is missing, which the upstream rejects outright: in that case keep the
		// bytes the client sent.
		unpairedCall := idlessCall
		for id := range callIDs {
			if _, ok := outputIDs[id]; !ok {
				unpairedCall = true
				break
			}
		}
		if grouped || unpairedCall {
			out = append(out, run...)
		} else {
			out = append(out, calls...)
			out = append(out, outs...)
			changed = true
		}
		i = j
	}
	if !changed {
		return items, false
	}
	return out, true
}

// trailingToolRunMissingReasoning locates the tool run still sitting at the end
// of the payload, the turn the upstream is being asked to continue. Runs further
// back belong to the cached prefix and must stay byte-identical.
func trailingToolRunMissingReasoning(items []json.RawMessage) (int, bool) {
	lastOut := -1
	for i := len(items) - 1; i >= 0; i-- {
		if isDeepSeekToolOutput(items[i]) {
			lastOut = i
			break
		}
	}
	if lastOut < 0 || lastOut < len(items)-2 {
		return 0, false
	}
	startOut := lastOut
	for startOut > 0 && isDeepSeekToolOutput(items[startOut-1]) {
		startOut--
	}
	insertAt := startOut
	if startOut > 0 && isDeepSeekToolCall(items[startOut-1]) {
		insertAt = startOut - 1
		for insertAt > 0 && isDeepSeekToolCall(items[insertAt-1]) {
			insertAt--
		}
	}
	if insertAt > 0 && reasoningItemHasText(items[insertAt-1]) {
		return 0, false
	}
	return insertAt, true
}

func isDeepSeekToolCall(item json.RawMessage) bool {
	switch peekType(item) {
	case "function_call", "custom_tool_call":
		return true
	default:
		return false
	}
}

func isDeepSeekToolOutput(item json.RawMessage) bool {
	switch peekType(item) {
	case "function_call_output", "custom_tool_call_output":
		return true
	default:
		return false
	}
}

// Codex replays the previous provider's history verbatim. The DeepSeek upstream
// only accepts a contiguous tool run in which every call has its own output:
// an assistant message or reasoning item in between fails the whole turn with
// "No tool output found for tool call ..." (grok-4.6 interleaves both, including
// web_search_call items). So hoist such items above the call they interrupt.
// The upstream never resolves previous_response_id state, so it also rejects any
// output whose call is missing from the same payload with "No tool call found
// for tool output ...": drop those together with the foreign web_search_call
// items it cannot deserialize.
func canonicalizeDeepSeekToolRuns(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	callIDs := make(map[string]struct{}, len(items))
	for _, item := range items {
		if !isDeepSeekToolCall(item) {
			continue
		}
		if id := toolCallID(item); id != "" {
			callIDs[id] = struct{}{}
		}
	}
	keptInput := make([]json.RawMessage, 0, len(items))
	dropped := false
	for _, item := range items {
		if isDeepSeekForeignSearchCall(item) {
			dropped = true
			continue
		}
		if isDeepSeekToolOutput(item) {
			if id := toolCallID(item); id != "" {
				if _, ok := callIDs[id]; !ok {
					dropped = true
					continue
				}
			}
		}
		keptInput = append(keptInput, item)
	}
	items = keptInput
	outputAt := make(map[string]int, len(items))
	for i, item := range items {
		if !isDeepSeekToolOutput(item) {
			continue
		}
		if id := toolCallID(item); id != "" {
			outputAt[id] = i
		}
	}
	hoisted := make(map[int][]json.RawMessage)
	moved := make(map[int]struct{})
	for i, item := range items {
		if !isDeepSeekToolRunBreaker(item) {
			continue
		}
		target := -1
		for j := range i {
			if !isDeepSeekToolCall(items[j]) {
				continue
			}
			id := toolCallID(items[j])
			if id == "" {
				continue
			}
			at, ok := outputAt[id]
			if !ok || at < i {
				continue
			}
			target = j
			break
		}
		if target >= 0 {
			moved[i] = struct{}{}
			hoisted[target] = append(hoisted[target], item)
		}
	}
	if len(moved) == 0 && !dropped {
		return input
	}
	kept := make([]json.RawMessage, 0, len(items))
	for i, item := range items {
		if _, ok := moved[i]; ok {
			continue
		}
		kept = append(kept, hoisted[i]...)
		kept = append(kept, item)
	}
	raw, err := common.Marshal(kept)
	if err != nil {
		return input
	}
	return raw
}

func toolCallID(item json.RawMessage) string {
	var peek deepSeekToolPairItem
	if common.Unmarshal(item, &peek) != nil {
		return ""
	}
	return peek.CallID
}

// message 与 reasoning 都会终止上游的工具轮校验，必须移出工具轮。
func isDeepSeekToolRunBreaker(item json.RawMessage) bool {
	switch peekType(item) {
	case "message", "reasoning":
		return true
	default:
		return false
	}
}

// xAI 的 web_search_call（status/action）上游反序列化不过去，deepseek 自己也不会产出，
// 只可能来自被切过来的别家历史。
func isDeepSeekForeignSearchCall(item json.RawMessage) bool {
	return peekType(item) == "web_search_call"
}

func reasoningItemHasText(item json.RawMessage) bool {
	var parsed map[string]any
	if common.Unmarshal(item, &parsed) != nil || peekType(item) != "reasoning" {
		return false
	}
	content, _ := parsed["content"].([]any)
	for _, part := range content {
		fields, _ := part.(map[string]any)
		if strings.TrimSpace(asString(fields["type"])) != "reasoning_text" {
			continue
		}
		if strings.TrimSpace(asString(fields["text"])) != "" {
			return true
		}
	}
	return false
}

func reasoningItemJSON(id, text string) json.RawMessage {
	item := map[string]any{
		"type": "reasoning",
		"content": []map[string]string{{
			"type": "reasoning_text",
			"text": text,
		}},
	}
	if strings.TrimSpace(id) != "" {
		item["id"] = id
	}
	raw, err := common.Marshal(item)
	if err != nil {
		return nil
	}
	return raw
}

func reasoningItemID(item json.RawMessage) string {
	var peek struct {
		ID string `json:"id"`
	}
	if common.Unmarshal(item, &peek) != nil {
		return ""
	}
	return peek.ID
}

func isDeepSeekToolItem(item json.RawMessage) bool {
	return isDeepSeekToolCall(item) || isDeepSeekToolOutput(item)
}

func peekType(item json.RawMessage) string {
	var peek struct {
		Type string `json:"type"`
	}
	_ = common.Unmarshal(item, &peek)
	return peek.Type
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

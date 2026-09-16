package deepseek

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
)

// DeepSeek 上游对 Responses 工具轮的要求只有三条：轮内连续、调用在前结果在后、
// 每个 call 都在同一份 payload 里有自己的 output。轮内夹进别的 item 会报
// "No tool output found for tool call ..."（xAI 历史里的 web_search_call、
// message、reasoning 都会插进工具轮中间），出现没有对应 call 的 output 会报
// "No tool call found for tool output ..."（上游从不解析 previous_response_id
// 状态）。
//
// canonicalizeDeepSeekToolRuns 只做重排、提升和丢弃，永远不新增、不改写 item。
// 插入 item 会改写上游已经缓存过的前缀，把 prompt cache 打掉；更糟的是上游把
// `assistant message → reasoning` 这种相邻顺序直接判成 400
// "The `reasoning_text` in the thinking mode must be passed back to the API."，
// 所以「补 reasoning_text」是禁止动作，不是修复手段。
func canonicalizeDeepSeekToolRuns(input json.RawMessage) json.RawMessage {
	if len(input) == 0 {
		return input
	}
	var items []json.RawMessage
	if common.Unmarshal(input, &items) != nil {
		return input
	}
	changed := false
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
	for _, item := range items {
		if isDeepSeekForeignSearchCall(item) {
			changed = true
			continue
		}
		if isDeepSeekToolOutput(item) {
			if id := toolCallID(item); id != "" {
				if _, ok := callIDs[id]; !ok {
					changed = true
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
	if len(moved) > 0 {
		changed = true
	}
	kept := make([]json.RawMessage, 0, len(items))
	for i, item := range items {
		if _, ok := moved[i]; ok {
			continue
		}
		kept = append(kept, hoisted[i]...)
		kept = append(kept, item)
	}
	regrouped, regroupedChanged := regroupDeepSeekToolRuns(kept)
	if regroupedChanged {
		kept, changed = regrouped, true
	}
	if !changed {
		return input
	}
	raw, err := common.Marshal(kept)
	if err != nil {
		return input
	}
	return raw
}

// regroupDeepSeekToolRuns puts every call of one contiguous tool run above that
// run's outputs. A call hoisted above the outputs stays dangling when its own
// output is missing, which the upstream rejects outright, so such a run keeps
// the bytes the client sent.
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

func toolCallID(item json.RawMessage) string {
	var peek deepSeekToolPairItem
	if common.Unmarshal(item, &peek) != nil {
		return ""
	}
	return peek.CallID
}

func peekType(item json.RawMessage) string {
	var peek struct {
		Type string `json:"type"`
	}
	_ = common.Unmarshal(item, &peek)
	return peek.Type
}

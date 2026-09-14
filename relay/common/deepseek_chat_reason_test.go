package common

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeChatCompletionsReasoningCopiesReasoningField(t *testing.T) {
	in := []byte(`{"model":"deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"ok","reasoning":"think"}]}`)
	out := NormalizeChatCompletionsReasoning(in)
	if gjson.GetBytes(out, "messages.1.reasoning_content").String() != "think" {
		t.Fatalf("expected reasoning_content, got %s", out)
	}
	if gjson.GetBytes(out, "messages.1.reasoning").String() != "think" {
		t.Fatalf("kept reasoning: %s", out)
	}
}

func TestNormalizeChatCompletionsReasoningKeepsExisting(t *testing.T) {
	in := []byte(`{"messages":[{"role":"assistant","content":"ok","reasoning_content":"keep","reasoning":"other"}]}`)
	out := NormalizeChatCompletionsReasoning(in)
	if gjson.GetBytes(out, "messages.0.reasoning_content").String() != "keep" {
		t.Fatalf("got %s", out)
	}
}

func TestInjectChatCachedReasoningFillsLastAssistant(t *testing.T) {
	in := []byte(`{"messages":[{"role":"assistant","content":"a"},{"role":"user","content":"next"},{"role":"assistant","content":"b"}]}`)
	out := injectChatCachedReasoning(in, "cached-think")
	if gjson.GetBytes(out, "messages.0.reasoning_content").String() != "" {
		t.Fatalf("must not fill earlier assistant: %s", out)
	}
	if gjson.GetBytes(out, "messages.2.reasoning_content").String() != "cached-think" {
		t.Fatalf("got %s", out)
	}
}

func TestNormalizeChatStreamDeltaCopiesReasoning(t *testing.T) {
	info := &RelayInfo{OriginModelName: "deepseek-v4.1-flash"}
	in := `{"choices":[{"delta":{"reasoning":"step"}}]}`
	out := NormalizeChatStreamDelta(info, in)
	if gjson.Get(out, "choices.0.delta.reasoning_content").String() != "step" {
		t.Fatalf("got %s", out)
	}
}

func TestAppendDeepSeekChatReasoning(t *testing.T) {
	var buf strings.Builder
	AppendDeepSeekChatReasoning(&buf, `{"choices":[{"delta":{"reasoning_content":"ab"}}]}`)
	AppendDeepSeekChatReasoning(&buf, `{"choices":[{"delta":{"reasoning":"c"}}]}`)
	if buf.String() != "abc" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestPrepareChatCompletionsBodyDoesNotInjectCachedReasoning(t *testing.T) {
	info := &RelayInfo{UserId: 2, TokenId: 9, OriginModelName: "deepseek-v4.1-flash"}
	in := []byte(`{"messages":[{"role":"assistant","content":"a"},{"role":"user","content":"next"}]}`)
	out := PrepareChatCompletionsBody(info, in)
	if gjson.GetBytes(out, "messages.0.reasoning_content").String() != "" {
		t.Fatalf("must not fill from token cache: %s", out)
	}
	injected := injectChatCachedReasoning(in, "cached-think")
	if gjson.GetBytes(injected, "messages.0.reasoning_content").String() != "cached-think" {
		t.Fatalf("sanity inject helper: %s", injected)
	}
}

func TestPrepareChatCompletionsBodyStillCopiesThisRequestReasoning(t *testing.T) {
	info := &RelayInfo{OriginModelName: "deepseek-v4.1-flash"}
	in := []byte(`{"messages":[{"role":"assistant","content":"ok","reasoning":"think"}]}`)
	out := PrepareChatCompletionsBody(info, in)
	if gjson.GetBytes(out, "messages.0.reasoning_content").String() != "think" {
		t.Fatalf("got %s", out)
	}
}

func TestApplyChatCopiesEarlierReasoningToToolAssistant(t *testing.T) {
	in := []byte(`{"messages":[
		{"role":"assistant","content":"a","reasoning_content":"think1","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"ok"},
		{"role":"assistant","content":"","tool_calls":[{"id":"c2","type":"function","function":{"name":"pwd","arguments":"{}"}}]}
	]}`)
	out := ApplyChatOpenCodeThinking(in, "", OpenCodeSessionUnknown)
	if gjson.GetBytes(out, "messages.2.reasoning_content").String() != "think1" {
		t.Fatalf("expected earlier reasoning, got %s", out)
	}
	if gjson.GetBytes(out, "messages.2.tool_calls.0.id").String() != "c2" {
		t.Fatalf("kept tool calls: %s", out)
	}
}

func TestApplyChatFillsToolAssistantFromSessionCache(t *testing.T) {
	in := []byte(`{"messages":[
		{"role":"user","content":"go"},
		{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"ok"}
	]}`)
	out := ApplyChatOpenCodeThinking(in, "need pwd", OpenCodeSessionDeepSeek)
	if gjson.GetBytes(out, "messages.1.reasoning_content").String() != "need pwd" {
		t.Fatalf("expected cache fill, got %s", out)
	}
}

func TestApplyChatDoesNotFillWhenLastModelUnknown(t *testing.T) {
	in := []byte(`{"messages":[
		{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"ok"}
	]}`)
	out := ApplyChatOpenCodeThinking(in, "need pwd", OpenCodeSessionUnknown)
	if gjson.GetBytes(out, "messages.1.reasoning_content").String() != "" {
		t.Fatalf("unknown last model must not fill: %s", out)
	}
	if gjson.GetBytes(out, "messages.0.tool_calls.0.id").String() != "c1" {
		t.Fatalf("unknown last model must keep tools: %s", out)
	}
}

func TestApplyChatFlattensToolsWhenLastModelOther(t *testing.T) {
	in := []byte(`{"messages":[
		{"role":"user","content":"hi"},
		{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"ok"}
	]}`)
	out := ApplyChatOpenCodeThinking(in, "stale-deepseek", OpenCodeSessionOther)
	if gjson.GetBytes(out, "messages.#").Int() != 2 {
		t.Fatalf("expected user+flattened assistant, got %s", out)
	}
	if gjson.GetBytes(out, "messages.1.tool_calls").Exists() && gjson.GetBytes(out, "messages.1.tool_calls").Raw != "null" && gjson.GetBytes(out, "messages.1.tool_calls.#").Int() != 0 {
		t.Fatalf("tool_calls must be gone: %s", out)
	}
	content := gjson.GetBytes(out, "messages.1.content").String()
	if !strings.Contains(content, "ls") || !strings.Contains(content, "ok") {
		t.Fatalf("flatten should keep name and result, got %q", content)
	}
	if gjson.GetBytes(out, "messages.1.reasoning_content").String() != "" {
		t.Fatalf("must not paste deepseek cache on switch: %s", out)
	}
}

func TestApplyChatKeepsToolsWhenRequestAlreadyHasReasoning(t *testing.T) {
	in := []byte(`{"messages":[
		{"role":"assistant","content":"a","reasoning_content":"keep","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"ok"}
	]}`)
	out := ApplyChatOpenCodeThinking(in, "", OpenCodeSessionOther)
	if gjson.GetBytes(out, "messages.0.tool_calls.0.id").String() != "c1" {
		t.Fatalf("must not flatten when reasoning present: %s", out)
	}
}

func TestPrepareChatCompletionsBodyDoesNotReadTokenCacheOnToolContinue(t *testing.T) {
	info := &RelayInfo{UserId: 2, TokenId: 9, OriginModelName: "deepseek-v4.1-flash"}
	in := []byte(`{"messages":[{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]}]}`)
	out := PrepareChatCompletionsBody(info, in)
	if gjson.GetBytes(out, "messages.0.reasoning_content").String() != "" {
		t.Fatalf("must not fill from token cache: %s", out)
	}
}

func TestOpenCodeSessionKeyPrefersPromptCacheKeyAndRejectsCoveFallback(t *testing.T) {
	body := []byte(`{"prompt_cache_key":"sess-1"}`)
	if got := OpenCodeSessionKey(nil, body); got != "sess-1" {
		t.Fatalf("body key: %q", got)
	}
	info := &RelayInfo{RequestHeaders: map[string]string{"x-opencode-session": "AI-Cove-3", "session_id": "pi-1"}}
	if got := OpenCodeSessionKey(info, nil); got != "pi-1" {
		t.Fatalf("skip cove fallback, use session_id: %q", got)
	}
	info = &RelayInfo{RequestHeaders: map[string]string{"x-opencode-session": "AI-Cove"}}
	if got := OpenCodeSessionKey(info, nil); got != "" {
		t.Fatalf("cove-only must be empty: %q", got)
	}
}

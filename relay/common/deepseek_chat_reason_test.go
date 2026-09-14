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

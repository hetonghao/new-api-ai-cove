package openai

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

var openDesignToolCallID = regexp.MustCompile(`^call_[0-9]{2}_[A-Za-z0-9]{24}$`)

type openDesignToolStream struct {
	id        string
	index     int
	arguments strings.Builder
	repaired  bool
	disabled  bool
	ended     bool
}

func newOpenDesignToolStream(resp *http.Response, info *relaycommon.RelayInfo) *openDesignToolStream {
	if info.ChannelMeta == nil || info.UpstreamModelName != "deepseek-v4.1-flash" || resp.Request == nil || resp.Request.URL == nil {
		return nil
	}
	target := resp.Request.URL
	if target.Scheme != "https" || !strings.EqualFold(target.Hostname(), "amr-link.open-design.ai") ||
		(target.Port() != "" && target.Port() != "443") || target.Path != "/v1/chat/completions" {
		return nil
	}
	return &openDesignToolStream{}
}

func (s *openDesignToolStream) normalize(chunk *dto.ChatCompletionsStreamResponse) (bool, error) {
	if s == nil || s.disabled || len(chunk.Choices) == 0 {
		return false, nil
	}
	if len(chunk.Choices) != 1 || chunk.Choices[0].Index != 0 || len(chunk.Choices[0].Delta.ToolCalls) > 1 {
		if s.repaired {
			return false, fmt.Errorf("ambiguous Open Design tool-call fragments after index repair")
		}
		s.disabled = true
		return false, nil
	}
	choice := &chunk.Choices[0]
	if s.ended && len(choice.Delta.ToolCalls) != 0 {
		return false, fmt.Errorf("Open Design tool-call data arrived after completion")
	}
	repaired := false
	for i := range choice.Delta.ToolCalls {
		call := &choice.Delta.ToolCalls[i]
		if call.Index == nil || *call.Index != 0 {
			if s.repaired {
				return false, fmt.Errorf("ambiguous Open Design tool-call index after repair")
			}
			s.disabled = true
			return false, nil
		}
		id := strings.TrimSpace(call.ID)
		if s.id == "" {
			if !isOpenDesignToolCallID(id, 0) || strings.TrimSpace(call.Function.Name) == "" || call.Type != "function" {
				s.disabled = true
				return false, nil
			}
			s.id = id
		} else if id != "" && id != s.id {
			// shortcut: only sequential native IDs with complete JSON are repairable; reject other layouts.
			if !isOpenDesignToolCallID(id, s.index+1) || strings.TrimSpace(call.Function.Name) == "" ||
				call.Type != "function" || !s.completeArguments() {
				return false, fmt.Errorf("ambiguous Open Design tool-call index reuse")
			}
			s.index++
			s.id = id
			s.arguments.Reset()
			s.repaired = true
			repaired = true
		}
		s.arguments.WriteString(call.Function.Arguments)
		if s.repaired {
			call.SetIndex(s.index)
		}
	}
	if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
		s.ended = true
		return repaired, s.finish()
	}
	return repaired, nil
}

func (s *openDesignToolStream) finish() error {
	if s != nil && s.repaired && (!s.ended || !s.completeArguments()) {
		return fmt.Errorf("Open Design tool stream ended without a finish reason or complete arguments after index repair")
	}
	return nil
}

func (s *openDesignToolStream) completeArguments() bool {
	var arguments map[string]common.RawMessage
	return common.UnmarshalJsonStr(s.arguments.String(), &arguments) == nil && arguments != nil
}

func isOpenDesignToolCallID(id string, index int) bool {
	return openDesignToolCallID.MatchString(id) && strings.HasPrefix(id, fmt.Sprintf("call_%02d_", index))
}

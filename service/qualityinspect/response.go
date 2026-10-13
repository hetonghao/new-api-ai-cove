package qualityinspect

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type responseUsage struct {
	Input      *int64 `json:"input_tokens"`
	Output     *int64 `json:"output_tokens"`
	Prompt     *int64 `json:"prompt_tokens"`
	Completion *int64 `json:"completion_tokens"`
}

type responseContent struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Refusal string `json:"refusal"`
}

type responseObject struct {
	Status string         `json:"status"`
	Model  string         `json:"model"`
	Usage  *responseUsage `json:"usage"`
	Output []struct {
		Type      string            `json:"type"`
		Role      string            `json:"role"`
		Content   []responseContent `json:"content"`
		Name      string            `json:"name"`
		Arguments common.RawMessage `json:"arguments"`
		Input     common.RawMessage `json:"input"`
		Action    common.RawMessage `json:"action"`
	} `json:"output"`
	Incomplete struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type responseEvent struct {
	Type     string          `json:"type"`
	Model    string          `json:"model"`
	Delta    string          `json:"delta"`
	Response *responseObject `json:"response"`
	Usage    *responseUsage  `json:"usage"`
	Error    any             `json:"error"`
	Choices  []struct {
		Index        int     `json:"index"`
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Content   string         `json:"content"`
			Refusal   string         `json:"refusal"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"delta"`
		Message struct {
			Content   string         `json:"content"`
			Refusal   string         `json:"refusal"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
}

type chatToolCall struct {
	Index    int `json:"index"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolCall struct{ Name, Arguments string }

// toolCallTranscript keeps what a tool_call_turn model actually tried to do in
// the artifact text so operators can tell an agentic upstream wrapper from a
// genuinely empty reply. It stays within the artifact limit so the sample keeps
// its tool_call_turn code instead of becoming artifact_too_large.
func toolCallTranscript(text string, calls []toolCall) []byte {
	var transcript strings.Builder
	transcript.WriteString(text)
	for _, call := range calls {
		if transcript.Len() > 0 {
			transcript.WriteString("\n\n")
		}
		transcript.WriteString("[tool_call] " + call.Name)
		if call.Arguments != "" {
			transcript.WriteString("\n" + call.Arguments)
		}
	}
	out := []byte(transcript.String())
	if len(out) > model.QualityMaxResponseBytes {
		out = bytes.ToValidUTF8(out[:model.QualityMaxResponseBytes], nil)
	}
	return out
}

func readUsage(result *model.QualityResult, usage *responseUsage, protocol string) {
	if usage == nil {
		return
	}
	if protocol == "responses" {
		result.InputTokens, result.OutputTokens = usage.Input, usage.Output
	} else {
		result.InputTokens, result.OutputTokens = usage.Prompt, usage.Completion
	}
	if result.InputTokens != nil && *result.InputTokens < 0 {
		result.InputTokens = nil
	}
	if result.OutputTokens != nil && *result.OutputTokens < 0 {
		result.OutputTokens = nil
	}
}

func finishResponse(result *model.QualityResult, response *responseObject) {
	if response == nil {
		result.ErrorCode = "missing_terminal"
		return
	}
	result.ResponseModel = response.Model
	result.FinishReason = response.Status
	readUsage(result, response.Usage, "responses")
	var text strings.Builder
	sawMessage := false
	var calls []toolCall
	for _, item := range response.Output {
		if item.Type != "message" {
			// Agentic upstreams can end a turn with tool/function calls and no
			// assistant message at all; distinguish that from an empty reply.
			if strings.HasSuffix(item.Type, "_call") {
				call := toolCall{Name: cmp.Or(item.Name, item.Type)}
				for _, raw := range []common.RawMessage{item.Arguments, item.Input, item.Action} {
					if len(raw) == 0 || string(raw) == "null" {
						continue
					}
					if common.Unmarshal(raw, &call.Arguments) != nil {
						call.Arguments = string(raw)
					}
					break
				}
				calls = append(calls, call)
			}
			continue
		}
		if item.Role != "assistant" && item.Role != "" {
			continue
		}
		sawMessage = true
		for _, part := range item.Content {
			if part.Type == "refusal" || part.Refusal != "" {
				result.ErrorCode = "refusal"
				return
			}
			if part.Type == "output_text" {
				text.WriteString(part.Text)
			}
		}
	}
	result.Text = []byte(text.String())
	if response.Status != "completed" {
		result.ErrorCode = "incomplete_response"
		if response.Status == "incomplete" {
			result.ErrorCode = "truncated"
		}
		return
	}
	if strings.TrimSpace(text.String()) == "" {
		switch {
		case !sawMessage && len(calls) > 0:
			result.ErrorCode = "tool_call_turn"
			result.Text = toolCallTranscript("", calls)
		case !sawMessage:
			result.ErrorCode = "no_message"
		default:
			result.ErrorCode = "empty_output"
		}
		return
	}
	result.Status, result.ErrorCode, result.RequestSuccess = "succeeded", "", true
}

func ParseResponse(reader io.Reader, contentType, protocol string, start time.Time) model.QualityResult {
	result := model.QualityResult{Status: "failed", ErrorCode: "missing_terminal"}
	if !strings.Contains(strings.ToLower(contentType), "text/event-stream") {
		limited := &io.LimitedReader{R: reader, N: model.QualityMaxEventBytes + 1}
		body, err := io.ReadAll(limited)
		if err != nil {
			result.ErrorCode = "response_read_error"
			return result
		}
		if len(body) > model.QualityMaxEventBytes {
			result.ErrorCode = "response_too_large"
			return result
		}
		if protocol == "responses" {
			var response responseObject
			if common.Unmarshal(body, &response) != nil {
				result.ErrorCode = "invalid_response"
				return result
			}
			finishResponse(&result, &response)
		} else {
			var event responseEvent
			if common.Unmarshal(body, &event) != nil || event.Error != nil {
				result.ErrorCode = "invalid_response"
				return result
			}
			result.ResponseModel = event.Model
			readUsage(&result, event.Usage, protocol)
			for _, choice := range event.Choices {
				if choice.Index != 0 {
					continue
				}
				if choice.FinishReason != nil {
					result.FinishReason = *choice.FinishReason
				}
				result.Text = []byte(choice.Message.Content)
				if choice.Message.Refusal != "" {
					result.ErrorCode = "refusal"
					return result
				}
				if result.FinishReason == "length" {
					result.ErrorCode = "truncated"
					return result
				}
				if result.FinishReason == "tool_calls" {
					result.ErrorCode = "tool_call_turn"
					calls := make([]toolCall, 0, len(choice.Message.ToolCalls))
					for _, call := range choice.Message.ToolCalls {
						calls = append(calls, toolCall{call.Function.Name, call.Function.Arguments})
					}
					result.Text = toolCallTranscript(choice.Message.Content, calls)
					return result
				}
				if result.FinishReason != "stop" {
					result.ErrorCode = "incomplete_response"
					return result
				}
				if strings.TrimSpace(choice.Message.Content) == "" {
					result.ErrorCode = "empty_output"
					return result
				}
				result.Status, result.ErrorCode, result.RequestSuccess = "succeeded", "", true
				break
			}
		}
		if len(result.ResponseModel) > 256 {
			result.ResponseModel = ""
		}
		return result
	}
	limited := &io.LimitedReader{R: reader, N: model.QualityMaxStreamBytes + 1}
	scanner := bufio.NewScanner(limited)
	scanner.Buffer(make([]byte, 4096), model.QualityMaxEventBytes+1)
	var data bytes.Buffer
	var output strings.Builder
	// Streamed tool calls arrive as name/argument fragments keyed by index.
	var streamCalls []*[2]strings.Builder
	streamCallBytes := 0
	terminal, done := false, false
	seenFinish := false
	process := func() bool {
		if data.Len() == 0 {
			return true
		}
		chunk := bytes.TrimSpace(data.Bytes())
		defer data.Reset()
		if bytes.Equal(chunk, []byte("[DONE]")) {
			done = true
			return true
		}
		var event responseEvent
		if common.Unmarshal(chunk, &event) != nil {
			result.ErrorCode = "invalid_stream"
			return false
		}
		if event.Error != nil || event.Type == "error" || event.Type == "response.failed" {
			result.Status, result.ErrorCode, result.RequestSuccess = "failed", "stream_error", false
			return false
		}
		if event.Model != "" {
			result.ResponseModel = event.Model
		}
		readUsage(&result, event.Usage, protocol)
		if protocol == "responses" {
			if event.Type == "response.output_text.delta" && event.Delta != "" {
				output.WriteString(event.Delta)
				if output.Len() > model.QualityMaxResponseBytes {
					result.Status, result.ErrorCode, result.RequestSuccess = "failed", "response_too_large", false
					return false
				}
				if result.FirstTextMs == nil {
					latency := time.Since(start).Milliseconds()
					result.FirstTextMs = &latency
				}
			}
			if event.Type == "response.completed" || event.Type == "response.incomplete" {
				if terminal {
					result.Status, result.ErrorCode, result.RequestSuccess = "failed", "duplicate_terminal", false
					return false
				}
				terminal = true
				finishResponse(&result, event.Response)
			}
			return true
		}
		for _, choice := range event.Choices {
			if choice.Index != 0 {
				continue
			}
			if choice.Delta.Refusal != "" {
				result.ErrorCode = "refusal"
				return false
			}
			for _, delta := range choice.Delta.ToolCalls {
				if delta.Index < 0 || delta.Index > 64 || streamCallBytes > model.QualityMaxResponseBytes {
					continue
				}
				for len(streamCalls) <= delta.Index {
					streamCalls = append(streamCalls, &[2]strings.Builder{})
				}
				streamCalls[delta.Index][0].WriteString(delta.Function.Name)
				streamCalls[delta.Index][1].WriteString(delta.Function.Arguments)
				streamCallBytes += len(delta.Function.Name) + len(delta.Function.Arguments)
			}
			if choice.Delta.Content != "" {
				if seenFinish {
					result.ErrorCode = "content_after_terminal"
					return false
				}
				if result.FirstTextMs == nil {
					latency := time.Since(start).Milliseconds()
					result.FirstTextMs = &latency
				}
				output.WriteString(choice.Delta.Content)
				if output.Len() > model.QualityMaxResponseBytes {
					result.Status, result.ErrorCode, result.RequestSuccess = "failed", "response_too_large", false
					return false
				}
			}
			if choice.FinishReason != nil {
				seenFinish = true
				result.FinishReason = *choice.FinishReason
				if *choice.FinishReason == "length" {
					result.ErrorCode = "truncated"
					return false
				}
				if *choice.FinishReason == "tool_calls" {
					result.ErrorCode = "tool_call_turn"
					calls := make([]toolCall, 0, len(streamCalls))
					for _, call := range streamCalls {
						calls = append(calls, toolCall{call[0].String(), call[1].String()})
					}
					transcript := toolCallTranscript(output.String(), calls)
					output.Reset()
					output.Write(transcript)
					return false
				}
				if *choice.FinishReason != "stop" {
					result.ErrorCode = "incomplete_response"
					return false
				}
				terminal = true
			}
		}
		return true
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if !process() {
				result.Text = []byte(output.String())
				return result
			}
			if done || (protocol == "responses" && terminal) {
				break
			}
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(value, " "))
		}
	}
	if !terminal {
		result.Text = []byte(output.String())
	}
	if limited.N <= 0 {
		result.Status, result.ErrorCode, result.RequestSuccess = "failed", "stream_too_large", false
		return result
	}
	if err := scanner.Err(); err != nil {
		if protocol == "responses" && terminal && result.Status == "succeeded" {
			return result
		}
		code := "stream_interrupted"
		if errors.Is(err, bufio.ErrTooLong) {
			code = "stream_event_too_large"
		}
		result.Status, result.ErrorCode, result.RequestSuccess = "failed", code, false
		return result
	}
	if !process() {
		return result
	}
	if protocol == "chat" {
		result.Text = []byte(output.String())
		if !terminal || !done {
			result.ErrorCode = "missing_terminal"
			return result
		}
		if strings.TrimSpace(output.String()) == "" {
			result.ErrorCode = "empty_output"
			return result
		}
		result.Status, result.ErrorCode, result.RequestSuccess = "succeeded", "", true
	}
	if !terminal {
		result.Status, result.ErrorCode, result.RequestSuccess = "failed", "missing_terminal", false
	}
	if len(result.ResponseModel) > 256 {
		result.ResponseModel = ""
	}
	return result
}

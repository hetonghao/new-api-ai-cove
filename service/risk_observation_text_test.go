package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestExtractRiskObservationText_returns_only_latest_user_text(t *testing.T) {
	tests := []struct {
		name    string
		request dto.Request
		want    string
	}{
		{
			name: "openai chat",
			request: &dto.GeneralOpenAIRequest{Messages: []dto.Message{
				{Role: "system", Content: "system secret"},
				{Role: "user", Content: "old user turn"},
				{Role: "assistant", Content: "old answer"},
				{Role: "tool", Content: "tool result"},
				{Role: "user", Content: []any{
					map[string]any{"type": "text", "text": "  current  text  "},
					map[string]any{"type": "image_url", "image_url": "https://example.com/image.png"},
				}},
			}},
			want: "  current  text  ",
		},
		{
			name:    "openai completion",
			request: &dto.GeneralOpenAIRequest{Prompt: []any{"current", "completion"}},
			want:    "current\ncompletion",
		},
		{
			name: "claude messages",
			request: &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
				{Role: "user", Content: "old user turn"},
				{Role: "assistant", Content: "old answer"},
				{Role: "user", Content: []any{
					map[string]any{"type": "text", "text": "current claude text"},
					map[string]any{"type": "tool_result", "content": "tool result"},
				}},
			}},
			want: "current claude text",
		},
		{
			name: "gemini contents",
			request: &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{
				{Role: "user", Parts: []dto.GeminiPart{{Text: "old user turn"}}},
				{Role: "model", Parts: []dto.GeminiPart{{Text: "old answer"}}},
				{Role: "user", Parts: []dto.GeminiPart{{Text: "current gemini text"}, {FunctionResponse: &dto.GeminiFunctionResponse{Name: "lookup"}}}},
			}},
			want: "current gemini text",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Given
			request := test.request

			// When
			got := ExtractRiskObservationText(request)

			// Then
			require.Equal(t, test.want, got)
		})
	}
}

func TestBuildSelectiveRiskExcerpt_matches_regex_against_original_text(t *testing.T) {
	// Given
	text := "Calculate  and respond with ONLY the number, nothing else."
	rules := []*model.RiskRule{{
		Id: 1, RuleType: model.RiskRuleRegex,
		Pattern: `Calculate  and respond with ONLY`, Enabled: true,
	}}

	// When
	excerpt, ruleIDs := BuildSelectiveRiskExcerpt(text, rules)

	// Then
	require.Equal(t, []int{1}, ruleIDs)
	require.Equal(t, "calculate and respond with only the number, nothing else.", excerpt)
}

func TestExtractRiskObservationText_filters_responses_history_and_tools(t *testing.T) {
	// Given
	input, err := common.Marshal([]map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "old turn"}}},
		{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "old answer"}}},
		{"type": "function_call_output", "output": "tool result"},
		{"role": "user", "content": []map[string]any{
			{"type": "input_text", "text": "current responses text"},
			{"type": "input_image", "image_url": "https://example.com/image.png"},
		}},
	})
	require.NoError(t, err)
	request := &dto.OpenAIResponsesRequest{Input: input}

	// When
	got := ExtractRiskObservationText(request)

	// Then
	require.Equal(t, "current responses text", got)
}

func TestExtractRiskObservationText_accepts_responses_message_string(t *testing.T) {
	// Given
	input, err := common.Marshal([]map[string]any{{"role": "user", "content": "current string"}})
	require.NoError(t, err)

	// When
	got := ExtractRiskObservationText(&dto.OpenAIResponsesRequest{Input: input})

	// Then
	require.Equal(t, "current string", got)
}

func TestExtractRiskObservationText_ignores_history_when_request_only_continues_the_conversation(t *testing.T) {
	responsesInput, err := common.Marshal([]map[string]any{
		{"type": "message", "role": "user", "content": "old responses turn"},
		{"type": "function_call_output", "output": "tool result"},
	})
	require.NoError(t, err)

	tests := []struct {
		name    string
		request dto.Request
	}{
		{
			name: "openai tool continuation",
			request: &dto.GeneralOpenAIRequest{Messages: []dto.Message{
				{Role: "user", Content: "old openai turn"},
				{Role: "assistant", Content: "tool call"},
				{Role: "tool", Content: "tool result"},
			}},
		},
		{
			name:    "responses function output continuation",
			request: &dto.OpenAIResponsesRequest{Input: responsesInput},
		},
		{
			name: "claude assistant continuation",
			request: &dto.ClaudeRequest{Messages: []dto.ClaudeMessage{
				{Role: "user", Content: "old claude turn"},
				{Role: "assistant", Content: "tool call"},
			}},
		},
		{
			name: "gemini model continuation",
			request: &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{
				{Role: "user", Parts: []dto.GeminiPart{{Text: "old gemini turn"}}},
				{Role: "model", Parts: []dto.GeminiPart{{Text: "tool call"}}},
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Empty(t, ExtractRiskObservationText(test.request))
		})
	}
}

func TestBuildSelectiveRiskExcerpt_merges_windows_and_caps_runes(t *testing.T) {
	// Given
	text := "prefix " + string(make([]rune, 490)) + "alpha" + string(make([]rune, 300)) + "beta" + string(make([]rune, 5000))
	text = replaceZeroRunes(text, '中')
	rules := []*model.RiskRule{
		{Id: 1, RuleType: model.RiskRuleKeyword, Pattern: "ＡＬＰＨＡ", Enabled: true},
		{Id: 2, RuleType: model.RiskRuleRegex, Pattern: `beta`, Enabled: true},
		{Id: 3, RuleType: model.RiskRuleKeyword, Pattern: "prefix", Enabled: false},
	}

	// When
	excerpt, ruleIDs := BuildSelectiveRiskExcerpt(text, rules)

	// Then
	require.Equal(t, []int{1, 2}, ruleIDs)
	require.Contains(t, excerpt, "alpha")
	require.Contains(t, excerpt, "beta")
	require.LessOrEqual(t, len([]rune(excerpt)), 4000)
}

func replaceZeroRunes(text string, replacement rune) string {
	runes := []rune(text)
	for index, value := range runes {
		if value == 0 {
			runes[index] = replacement
		}
	}
	return string(runes)
}

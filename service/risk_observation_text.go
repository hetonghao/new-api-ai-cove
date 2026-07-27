package service

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

const (
	riskExcerptRadius = 500
	riskExcerptLimit  = 4000
)

type riskTextRange struct {
	start int
	end   int
}

type responsesRiskInput struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func ExtractRiskObservationText(request dto.Request) string {
	switch typed := request.(type) {
	case *dto.GeneralOpenAIRequest:
		return extractOpenAIRiskText(typed)
	case *dto.OpenAIResponsesRequest:
		return extractResponsesRiskText(typed)
	case *dto.ClaudeRequest:
		if len(typed.Messages) > 0 && typed.Messages[len(typed.Messages)-1].Role == "user" {
			return strings.TrimSpace(typed.Messages[len(typed.Messages)-1].GetStringContent())
		}
	case *dto.GeminiChatRequest:
		if len(typed.Contents) > 0 && typed.Contents[len(typed.Contents)-1].Role == "user" {
			content := typed.Contents[len(typed.Contents)-1]
			texts := make([]string, 0, len(content.Parts))
			for _, part := range content.Parts {
				if text := strings.TrimSpace(part.Text); text != "" {
					texts = append(texts, text)
				}
			}
			return strings.Join(texts, "\n")
		}
	}
	return ""
}

func extractOpenAIRiskText(request *dto.GeneralOpenAIRequest) string {
	if len(request.Messages) > 0 {
		message := request.Messages[len(request.Messages)-1]
		if message.Role != "user" {
			return ""
		}
		parts := message.ParseContent()
		texts := make([]string, 0, len(parts))
		for _, part := range parts {
			if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
				texts = append(texts, strings.TrimSpace(part.Text))
			}
		}
		return strings.Join(texts, "\n")
	}
	texts := stringValues(request.Prompt)
	texts = append(texts, stringValues(request.Input)...)
	if instruction := strings.TrimSpace(request.Instruction); instruction != "" {
		texts = append(texts, instruction)
	}
	return strings.Join(texts, "\n")
}

func extractResponsesRiskText(request *dto.OpenAIResponsesRequest) string {
	if common.GetJsonType(request.Input) == "string" {
		var text string
		if common.Unmarshal(request.Input, &text) == nil {
			return strings.TrimSpace(text)
		}
		return ""
	}
	var inputs []responsesRiskInput
	if common.Unmarshal(request.Input, &inputs) != nil {
		return ""
	}
	if len(inputs) == 0 {
		return ""
	}
	input := inputs[len(inputs)-1]
	if (input.Type != "" && input.Type != "message") || input.Role != "user" {
		return ""
	}
	if common.GetJsonType(input.Content) == "string" {
		var text string
		if common.Unmarshal(input.Content, &text) == nil {
			return strings.TrimSpace(text)
		}
		return ""
	}
	var parts []dto.MediaInput
	if common.Unmarshal(input.Content, &parts) != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "input_text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, strings.TrimSpace(part.Text))
		}
	}
	return strings.Join(texts, "\n")
}

func stringValues(value any) []string {
	switch typed := value.(type) {
	case string:
		if text := strings.TrimSpace(typed); text != "" {
			return []string{text}
		}
	case []any:
		texts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
		}
		return texts
	}
	return nil
}

func BuildSelectiveRiskExcerpt(text string, rules []*model.RiskRule) (string, []int) {
	normalized := NormalizeRiskText(text)
	if normalized == "" {
		return "", nil
	}
	var ranges []riskTextRange
	var ruleIDs []int
	for _, rule := range rules {
		if rule == nil || !rule.Enabled {
			continue
		}
		matches := riskRuleMatches(normalized, rule)
		if len(matches) == 0 {
			continue
		}
		ranges = append(ranges, matches...)
		ruleIDs = append(ruleIDs, rule.Id)
	}
	if len(ranges) == 0 {
		return "", nil
	}
	return mergeRiskExcerpt(normalized, ranges), ruleIDs
}

func riskRuleMatches(text string, rule *model.RiskRule) []riskTextRange {
	var byteRanges [][]int
	switch rule.RuleType {
	case model.RiskRuleKeyword, model.RiskRulePhrase:
		pattern := NormalizeRiskText(rule.Pattern)
		for offset := 0; pattern != "" && offset < len(text); {
			index := strings.Index(text[offset:], pattern)
			if index < 0 {
				break
			}
			start := offset + index
			byteRanges = append(byteRanges, []int{start, start + len(pattern)})
			offset = start + len(pattern)
		}
	case model.RiskRuleRegex:
		compiled, err := regexp.Compile(strings.TrimSpace(rule.Pattern))
		if err == nil {
			byteRanges = compiled.FindAllStringIndex(text, -1)
		}
	}
	ranges := make([]riskTextRange, 0, len(byteRanges))
	for _, match := range byteRanges {
		ranges = append(ranges, riskTextRange{
			start: utf8.RuneCountInString(text[:match[0]]),
			end:   utf8.RuneCountInString(text[:match[1]]),
		})
	}
	return ranges
}

func mergeRiskExcerpt(text string, matches []riskTextRange) string {
	runes := []rune(text)
	windows := make([]riskTextRange, 0, len(matches))
	for _, match := range matches {
		windows = append(windows, riskTextRange{
			start: max(0, match.start-riskExcerptRadius),
			end:   min(len(runes), match.end+riskExcerptRadius),
		})
	}
	sort.Slice(windows, func(left, right int) bool { return windows[left].start < windows[right].start })
	merged := windows[:1]
	for _, window := range windows[1:] {
		last := &merged[len(merged)-1]
		if window.start <= last.end {
			last.end = max(last.end, window.end)
			continue
		}
		merged = append(merged, window)
	}
	parts := make([]string, 0, len(merged))
	for _, window := range merged {
		parts = append(parts, string(runes[window.start:window.end]))
	}
	excerpt := []rune(strings.Join(parts, "\n...\n"))
	if len(excerpt) > riskExcerptLimit {
		excerpt = excerpt[:riskExcerptLimit]
	}
	return string(excerpt)
}

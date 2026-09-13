package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiRequestShapeOmitsUserText(t *testing.T) {
	sig, err := json.Marshal("abcdefghijklmnopqrstuvwxyz012345")
	require.NoError(t, err)
	req := &GeminiChatRequest{
		Tools:              json.RawMessage(`[{"functionDeclarations":[{"name":"exec_command"}]}]`),
		SystemInstructions: &GeminiChatContent{Parts: []GeminiPart{{Text: "secret system"}}},
		Contents: []GeminiChatContent{
			{Role: "user", Parts: []GeminiPart{{Text: "secret user prompt"}}},
			{Role: "model", Parts: []GeminiPart{
				{Thought: true, Text: "secret thought", ThoughtSignature: sig},
				{FunctionCall: &FunctionCall{FunctionName: "exec_command", Arguments: map[string]any{"cmd": "secret"}}},
			}},
			{Role: "user", Parts: []GeminiPart{
				{FunctionResponse: &GeminiFunctionResponse{Name: "exec_command", Response: map[string]any{"output": "secret"}}},
			}},
		},
	}

	got := GeminiRequestShape(req)
	require.NotContains(t, got, "secret")
	require.NotContains(t, got, "abcdefghijklmnopqrstuvwxyz012345")
	require.True(t, strings.HasPrefix(got, "contents=3 tools=true sys=true"))
	require.Contains(t, got, "c0=user(p=1 text=1 thought=0 fc=0 fr=0 inline=0)")
	require.Contains(t, got, "fc_names=exec_command")
	require.Contains(t, got, "fr_names=exec_command")
	require.Contains(t, got, "sig=32")
}

func TestGeminiRequestShapeNil(t *testing.T) {
	require.Equal(t, "nil", GeminiRequestShape(nil))
}

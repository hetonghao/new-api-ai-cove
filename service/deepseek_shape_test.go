package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeepSeekInputShapeOmitsText(t *testing.T) {
	input, err := json.Marshal([]map[string]any{
		{"type": "reasoning", "content": []map[string]string{{"type": "reasoning_text", "text": "secret thought"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command"},
		{"type": "function_call_output", "call_id": "call-1", "output": "secret output"},
	})
	require.NoError(t, err)
	got := deepSeekInputShape(input)
	require.NotContains(t, got, "secret")
	require.Equal(t, "items=3 reasoning=1 text=1 empty=0 fc=1 fco=1 last=reasoning,function_call,function_call_output", got)
}

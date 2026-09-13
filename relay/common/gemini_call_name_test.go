package common

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestHydrateGeminiFunctionOutputNamesFillsFromSavedCall(t *testing.T) {
	SaveGeminiCallName("call_unit_1", "exec_command")
	input, err := json.Marshal([]map[string]any{
		{"type": "function_call_output", "call_id": "call_unit_1", "output": "ok"},
	})
	require.NoError(t, err)
	got := HydrateGeminiFunctionOutputNames(input)
	require.Equal(t, "exec_command", gjson.GetBytes(got, "0.name").String())
	require.Equal(t, "call_unit_1", gjson.GetBytes(got, "0.call_id").String())
}

func TestHydrateGeminiFunctionOutputNamesFillsFromSameRequest(t *testing.T) {
	input, err := json.Marshal([]map[string]any{
		{"type": "function_call", "call_id": "call_unit_2", "name": "view_image", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call_unit_2", "output": "ok"},
	})
	require.NoError(t, err)
	got := HydrateGeminiFunctionOutputNames(input)
	require.Equal(t, "view_image", gjson.GetBytes(got, "1.name").String())
}

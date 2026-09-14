package deepseek

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestLastResponsesUserTextFromStringContent(t *testing.T) {
	in := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": "网关有离线兜底吗"},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
	})
	require.Equal(t, "网关有离线兜底吗", lastResponsesUserText(in))
}

func TestLastResponsesUserTextFromInputTextParts(t *testing.T) {
	in := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]string{
			{"type": "input_text", "text": "先不改代码"},
		}},
	})
	require.Equal(t, "先不改代码", lastResponsesUserText(in))
}

func TestAppendOpenCodeTaskReminderOnToolContinue(t *testing.T) {
	in := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})
	got := appendOpenCodeTaskReminder(in, "网关有离线兜底吗")
	require.Equal(t, []string{"function_call_output", "message"}, inputTypes(t, got))
	require.Equal(t, "developer", gjson.GetBytes(got, "1.role").String())
	require.Contains(t, gjson.GetBytes(got, "1.content.0.text").String(), "网关有离线兜底吗")
	require.Contains(t, gjson.GetBytes(got, "1.content.0.text").String(), openCodeTaskPrefix)
	require.NotContains(t, string(got), "根据以上工具结果，回答用户最初的问题")
}

func TestAppendOpenCodeTaskReminderDoesNotDuplicate(t *testing.T) {
	in := mustJSON(t, []map[string]any{
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "message", "role": "developer", "content": []map[string]string{
			{"type": "input_text", "text": openCodeTaskPrefix + "\n已有"},
		}},
	})
	got := appendOpenCodeTaskReminder(in, "网关有离线兜底吗")
	require.Equal(t, []string{"function_call_output", "message"}, inputTypes(t, got))
	require.Contains(t, string(got), "已有")
	require.NotContains(t, string(got), "网关有离线兜底吗")
}

func TestRememberOpenCodeUserTaskSkipsWhenUserPresent(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model:          "deepseek-v4.1-flash",
		PromptCacheKey: mustJSON(t, "sess-1"),
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "user", "content": "网关有离线兜底吗"},
		}),
	}
	RememberOpenCodeUserTask(nil, &req)
	require.Equal(t, []string{"message"}, inputTypes(t, req.Input))
	require.Equal(t, "user", gjson.GetBytes(req.Input, "0.role").String())
}

func TestOpenCodeSessionKeyFromPromptCacheKey(t *testing.T) {
	req := dto.OpenAIResponsesRequest{PromptCacheKey: mustJSON(t, "01a0-thread")}
	require.Equal(t, "01a0-thread", openCodeSessionKey(nil, &req))
}

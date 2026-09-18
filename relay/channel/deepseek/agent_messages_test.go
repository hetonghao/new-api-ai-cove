package deepseek

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// 生产现场形状（2026-09-18，request id 202609181247058130404008268d9d6d3f9gTSB 等 7 次重试）：
// 最后三项是 agent_message(NEW_TASK) → assistant message → agent_message(NEW_TASK)，
// 上游把两条 agent_message 全丢掉，剩下没有 reasoning 的 assistant message 收尾 → 400。
func TestNormalizeDeepSeekAgentMessagesRewritesProductionShape(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "<app-context>"}}},
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "# AGENTS.md instructions"}}},
		{"type": "agent_message", "author": "/root", "recipient": "/root/turnstate_patch_research", "id": "amsg_1",
			"content": []map[string]any{
				{"type": "input_text", "text": "Message Type: NEW_TASK\nTask name: /root/turnstate_patch_research\nSender: /root\nPayload:\n"},
				{"type": "encrypted_content", "encrypted_content": "Read-only research task. Do not modify production."},
			}},
		{"type": "message", "role": "assistant", "phase": "final_answer", "content": []map[string]any{{"type": "output_text", "text": "你还没给我任务"}}},
		{"type": "agent_message", "author": "/root", "recipient": "/root/turnstate_patch_research", "id": "amsg_2",
			"content": []map[string]any{
				{"type": "encrypted_content", "encrypted_content": "Research task (read-only, write exactly one Markdown file)."},
			}},
	})

	got := normalizeDeepSeekAgentMessages(input)

	require.NotContains(t, string(got), "agent_message")
	require.Equal(t, []string{"message", "message", "message", "message", "message"}, inputTypes(t, got))
	require.Equal(t, "user", gjson.GetBytes(got, "2.role").String())
	require.Equal(t, "Message Type: NEW_TASK\nTask name: /root/turnstate_patch_research\nSender: /root\nPayload:\n", gjson.GetBytes(got, "2.content.0.text").String())
	require.Equal(t, "Read-only research task. Do not modify production.", gjson.GetBytes(got, "2.content.1.text").String())
	// 尾项必须是 user：上游不再把没有 reasoning 的 assistant message 当成最后一条可见消息。
	require.Equal(t, "user", gjson.GetBytes(got, "4.role").String())
	require.Equal(t, "Research task (read-only, write exactly one Markdown file).", gjson.GetBytes(got, "4.content.0.text").String())
	// author / recipient / id 上游不认，不带到改写结果里。
	require.False(t, gjson.GetBytes(got, "2.author").Exists())
}

func TestNormalizeDeepSeekAgentMessagesLeavesOtherInputUntouched(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "hi"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
	})

	require.Equal(t, string(input), string(normalizeDeepSeekAgentMessages(input)))
}

func TestNormalizeDeepSeekAgentMessagesKeepsUnreadableItem(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "agent_message", "author": "/root", "recipient": "/root/x", "id": "amsg_1", "content": []map[string]any{}},
	})

	require.Equal(t, string(input), string(normalizeDeepSeekAgentMessages(input)))
}

func TestConvertOpenAIResponsesRequestRewritesAgentMessageToUserTurn(t *testing.T) {
	req := dto.OpenAIResponsesRequest{
		Model: "deepseek-v4.1-flash",
		Input: mustJSON(t, []map[string]any{
			{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "ok"}}},
			{"type": "agent_message", "author": "/root", "recipient": "/root/x", "id": "amsg_1",
				"content": []map[string]any{{"type": "input_text", "text": "Message Type: NEW_TASK\nPayload:\n"}}},
		}),
	}

	got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
	require.NoError(t, err)
	converted, ok := got.(dto.OpenAIResponsesRequest)
	require.True(t, ok)

	require.Equal(t, []string{"message", "message"}, inputTypes(t, converted.Input))
	require.Equal(t, "user", gjson.GetBytes(converted.Input, "1.role").String())
}

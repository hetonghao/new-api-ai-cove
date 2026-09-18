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

// 侧边会话投递（2026-09-18 23:01 生产 422 现场，渠道 Console Go → DeepSeek）：
// Codex 把"别的会话投递进来的消息"记成一条没有 call_id 的 function_call_output
// （主会话里不存在配对的 send_message_to_thread 调用）。上游对此是硬失败而不是静默忽略：
//
//	status_code=422 … input: missing field `call_id` at line 1 column 705395
//
// 这条 item 一旦进了历史就每轮重放，整条线程从此打不通。正文是投递内容，改写成 user 轮。
func TestNormalizeDeepSeekCallLessToolOutputRewritesToUserTurn(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "hi"}}},
		{"type": "function_call", "call_id": "call-1", "name": "exec_command", "arguments": "{}"},
		{"type": "function_call_output", "call_id": "call-1", "output": "ok"},
		{"type": "function_call_output", "id": "fco_01a0b509-aa9c-7b00-950e-9ec5ac930c44", "name": "send_message_to_thread",
			"namespace": "codex_app",
			"output":    "<codex_delegation>\n  <input>【侧边会话转来的收尾任务】</input>\n</codex_delegation>",
			"internal_chat_message_metadata_passthrough": map[string]any{"turn_id": "01a0b509"}},
	})

	got := normalizeDeepSeekAgentMessages(input)

	require.Equal(t, []string{"message", "function_call", "function_call_output", "message"}, inputTypes(t, got))
	// 有 call_id 的正常工具输出必须原样留着。
	require.Equal(t, "call-1", gjson.GetBytes(got, "2.call_id").String())
	require.Equal(t, "user", gjson.GetBytes(got, "3.role").String())
	require.Equal(t, "<codex_delegation>\n  <input>【侧边会话转来的收尾任务】</input>\n</codex_delegation>", gjson.GetBytes(got, "3.content.0.text").String())
	// 客户端字段不带到改写结果里，结果也不再是缺 call_id 的工具输出。
	require.False(t, gjson.GetBytes(got, "3.name").Exists())
	require.False(t, gjson.GetBytes(got, "3.call_id").Exists())
}

// 拿不到正文也必须处理：留着就是每轮 422，只能丢弃。
func TestNormalizeDeepSeekCallLessToolOutputWithoutTextIsDropped(t *testing.T) {
	input := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "hi"}}},
		{"type": "function_call_output", "id": "fco_blank", "name": "send_message_to_thread", "output": "   "},
		{"type": "custom_tool_call_output", "id": "ctco_1", "output": ""},
	})

	require.Equal(t, []string{"message"}, inputTypes(t, normalizeDeepSeekAgentMessages(input)))
}

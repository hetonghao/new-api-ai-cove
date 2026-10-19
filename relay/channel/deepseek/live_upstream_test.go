package deepseek

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	relayconvert "github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/stretchr/testify/require"
)

// 打真实上游的端到端检查，默认跳过；只有显式给出地址和密钥时才跑：
//
//	AI_COVE_DEEPSEEK_E2E_BASE_URL=https://opencode.ai/zen/go/v1/responses
//	AI_COVE_DEEPSEEK_E2E_API_KEY=sk-...
//	AI_COVE_DEEPSEEK_E2E_SESSION=ai-cove-live-e2e     （可选，zen 需要会话头）
//	AI_COVE_DEEPSEEK_E2E_MODEL=deepseek-v4.1-flash    （可选）
//
// 运行方式：make deepseek-live-e2e。每个用例一次短推理（max_output_tokens=16），
// 被上游当场拒绝的负例不产生推理。
//
// 复现要点（2026-09-16 用生产 dump 直连本上游逐条比对得到，改 fixture 前先读）：
//   - 请求必须声明 tools，否则上游不会把 function_call 映射成工具轮，整套校验都不触发；
//   - call_id 要用上游自己的形状（call_00_...）；外来 id 会让上游要求客户端回传
//     reasoning，和线上真实流量不是同一条路径；
//   - 上游按 x-opencode-session 维护服务端状态，同一个会话里连着发不同形状会互相影响，
//     所以每个用例都用自己的会话 id。
func TestDeepSeekLiveUpstreamContract(t *testing.T) {
	baseURL, apiKey, model, session := deepSeekLiveTarget(t)
	client := &http.Client{Timeout: 120 * time.Second}

	user := map[string]any{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "按顺序回复 ok"}}}
	commentary := map[string]any{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "我先跑一条命令。"}}, "phase": "commentary"}
	reasoning := map[string]any{"type": "reasoning", "id": "rs_1", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "先看目录"}}}
	emptyReasoning := map[string]any{"type": "reasoning", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": ""}}}
	call := func(index int) map[string]any {
		return map[string]any{"type": "function_call", "call_id": fmt.Sprintf("call_00_ET_aicove%dabcd", index), "name": "bash", "arguments": `{"command":"pwd"}`}
	}
	output := func(index int) map[string]any {
		return map[string]any{"type": "function_call_output", "call_id": fmt.Sprintf("call_00_ET_aicove%dabcd", index), "output": "/tmp\n"}
	}

	cases := []struct {
		name   string
		shape  []map[string]any
		reject bool
	}{
		{
			name:  "并行工具轮交错 call/out",
			shape: []map[string]any{user, call(1), call(2), output(1), output(2)},
		},
		{
			name:  "assistant message 直接带工具轮且没有 reasoning",
			shape: []map[string]any{user, commentary, call(1), output(1)},
		},
		{
			name:  "reasoning 在 assistant message 之前",
			shape: []map[string]any{user, reasoning, commentary, call(1), output(1)},
		},
		{
			name:  "compact 续轮：calls、outputs、末尾空 reasoning",
			shape: []map[string]any{user, call(1), output(1), emptyReasoning},
		},
		{
			// system 保留调用前位置；与 developer/user 的处理不同。
			name:  "system 消息后工具轮",
			shape: []map[string]any{user, {"type": "message", "role": "system", "content": "Image resized."}, call(1), output(1)},
		},
		{
			// 这条相邻顺序就是生产上反复 400 的形状，上游必须继续拒绝；一旦它开始
			// 接受，说明上游规则变了，"绝不补 reasoning" 的红线需要重新评估。
			name:   "assistant message 紧跟 reasoning（上游必须拒绝）",
			shape:  []map[string]any{user, commentary, reasoning, call(1), output(1)},
			reject: true,
		},
	}

	for index, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := dto.OpenAIResponsesRequest{Model: model, Input: mustJSON(t, tc.shape)}
			got, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, nil, req)
			require.NoError(t, err)
			converted, ok := got.(dto.OpenAIResponsesRequest)
			require.True(t, ok)

			caseSession := fmt.Sprintf("%s-%d-%d", session, time.Now().UnixNano(), index)
			status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey, caseSession, deepSeekLiveBody(t, model, converted.Input))

			if tc.reject {
				require.Equal(t, http.StatusBadRequest, status, "上游不再拒绝该历史形状，规则已变：%s", responseBody)
				require.Contains(t, responseBody, "reasoning_text")
				return
			}
			requireDeepSeekLiveSuccess(t, status, responseBody)
		})
	}
}

// 子代理形状（2026-09-18 生产 7 次 400 的现场）：上游把 agent_message 整条丢掉，
// 剩下没有 reasoning 的 assistant message 收尾就 400；改写成 user 轮后必须 200。
func TestDeepSeekLiveUpstreamAgentMessage(t *testing.T) {
	baseURL, apiKey, model, session := deepSeekLiveTarget(t)
	client := &http.Client{Timeout: 120 * time.Second}

	shape := []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "按顺序回复 ok"}}},
		{"type": "message", "role": "assistant", "phase": "final_answer", "content": []map[string]any{{"type": "output_text", "text": "你还没给我任务"}}},
		{"type": "agent_message", "author": "/root", "recipient": "/root/w", "id": "amsg_live_1", "content": []map[string]any{
			{"type": "input_text", "text": "Message Type: NEW_TASK\nTask name: /root/w\nSender: /root\nPayload:\n"},
			{"type": "encrypted_content", "encrypted_content": "Reply with exactly one word: BANANA"},
		}},
	}
	raw := mustJSON(t, shape)

	status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
		fmt.Sprintf("%s-agent-raw-%d", session, time.Now().UnixNano()), deepSeekLiveBody(t, model, raw))
	require.Equal(t, http.StatusBadRequest, status, "上游不再拒绝尾部 assistant message，规则已变：%s", responseBody)
	require.Contains(t, responseBody, "reasoning_text")

	normalized := normalizeDeepSeekAgentMessages(raw)
	status, responseBody = postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
		fmt.Sprintf("%s-agent-fix-%d", session, time.Now().UnixNano()), deepSeekLiveBody(t, model, normalized))
	requireDeepSeekLiveSuccess(t, status, responseBody)
}

// 2026-09-21 11:22 生产 400 现场（call_00_GLi6SIHQhqISBhtuwGpG3282，channel 59 D-6）：
// Codex 把 `<image_resize_notice>` 这条 developer message 发在 view_image 的 call 与它
// 自己的 output 之间，上游整轮回 "No tool output found for tool call ..."。
// 2026-09-26 D-3 实测：提到 calls 前又会触发 reasoning_text 400，必须移到整轮 outputs 后。
// 2026-10-19 起 call_id 会被规范化改写到外部命名空间（call_00_ET_+hash），上游不再把
// 外来签发的 id 当成需要回传 reasoning 的调用，原先 system+图片历史的 reasoning_text
// 限制也随之消失（已改成正例断言）。
func TestDeepSeekLiveUpstreamNoticeBetweenCallAndOutput(t *testing.T) {
	baseURL, apiKey, model, session := deepSeekLiveTarget(t)
	client := &http.Client{Timeout: 120 * time.Second}

	imageOutput := []map[string]any{{
		"type":      "input_image",
		"image_url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
		"detail":    "high",
	}}
	shape := []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "看这两张图"}}},
		{"type": "function_call", "call_id": "call_00_GLi6SIHQhqISBhtuwGpG3282", "name": "view_image", "arguments": `{"path":"/tmp/a.png"}`},
		{"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "<image_resize_notice>\nImage 1 of 1 was resized.\n</image_resize_notice>"}}},
		{"type": "function_call", "call_id": "call_01_0SUCSgUYWTDdcW0GlNmX3093", "name": "view_image", "arguments": `{"path":"/tmp/b.png"}`},
		{"type": "function_call_output", "call_id": "call_00_GLi6SIHQhqISBhtuwGpG3282", "output": imageOutput},
		{"type": "function_call_output", "call_id": "call_01_0SUCSgUYWTDdcW0GlNmX3093", "output": imageOutput},
	}
	shape = append(shape,
		map[string]any{"type": "reasoning", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "Inspect the images"}}},
		map[string]any{"type": "message", "role": "developer", "content": []map[string]any{{"type": "input_text", "text": "Second image resized."}}},
		map[string]any{"type": "function_call", "call_id": "call_02_ThirdImage0003", "name": "view_image", "arguments": `{"path":"/tmp/c.png"}`},
		map[string]any{"type": "function_call_output", "call_id": "call_02_ThirdImage0003", "output": "Third image inspected"},
	)
	for _, tc := range []struct {
		name       string
		order      []int
		role       string
		textOutput bool
		checkRaw   bool
		reject     bool
	}{
		{"between_calls", []int{0, 1, 2, 3, 4, 5}, "developer", false, true, true},
		{"between_outputs", []int{0, 1, 3, 4, 2, 5}, "developer", false, true, false},
		{"before_outputs", []int{0, 1, 3, 2, 4, 5}, "developer", false, true, true},
		{"after_outputs", []int{0, 1, 3, 4, 5, 2}, "developer", false, false, false},
		{"with_reasoning", []int{0, 6, 1, 3, 4, 2, 5}, "developer", false, false, false},
		{"three_calls_multiple_notices", []int{0, 6, 1, 3, 8, 4, 2, 5, 7, 9}, "developer", false, false, false},
		{"later_call_extends_run", []int{0, 1, 2, 3, 4, 8, 5, 9}, "developer", false, false, false},
		{"user_notice", []int{0, 1, 2, 3, 4, 5}, "user", false, false, false},
		{"system_image_history", []int{0, 1, 2, 3, 4, 5}, "system", false, false, false},
		{"text_outputs", []int{0, 1, 2, 3, 4, 5}, "developer", true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shape[2]["role"] = tc.role
			shape[4]["output"], shape[5]["output"] = imageOutput, imageOutput
			if tc.textOutput {
				shape[4]["output"], shape[5]["output"] = "image A", "image B"
			}
			items := make([]map[string]any, 0, len(tc.order))
			for _, index := range tc.order {
				items = append(items, shape[index])
			}
			raw := mustJSON(t, items)
			if tc.checkRaw {
				status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
					fmt.Sprintf("%s-%s-raw-%d", session, tc.name, time.Now().UnixNano()), deepSeekLiveViewImageBody(t, model, raw))
				if tc.reject {
					require.Equal(t, http.StatusBadRequest, status, "调用间或首个结果前的通知应被拒绝：%s", responseBody)
					require.Contains(t, responseBody, "No tool output found for tool call")
				} else {
					// 原始结果间通知可被接受，旧规范化才引入 400。
					requireDeepSeekLiveSuccess(t, status, responseBody)
				}
				t.Logf("raw status=%d", status)
			}
			status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
				fmt.Sprintf("%s-%s-fix-%d", session, tc.name, time.Now().UnixNano()), deepSeekLiveViewImageBody(t, model, NormalizeResponsesInput(raw)))
			requireDeepSeekLiveSuccess(t, status, responseBody)
			t.Logf("normalized status=%d: Responses stream finished", status)
		})
	}
}

// view_image 形状必须声明同名工具，否则上游不会把 function_call 映射成工具轮。
func deepSeekLiveViewImageBody(t *testing.T, model string, input json.RawMessage) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"model":             model,
		"input":             input,
		"max_output_tokens": 16,
		"stream":            true,
		"include":           []string{"reasoning.encrypted_content"},
		"store":             false,
		"reasoning":         map[string]any{"effort": "high", "summary": "auto"},
		"tools": []map[string]any{{
			"type":        "function",
			"name":        "view_image",
			"description": "view an image file",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
				"required":   []string{"path"},
			},
		}},
	})
}

func deepSeekLiveTarget(t *testing.T) (baseURL, apiKey, model, session string) {
	t.Helper()
	baseURL = strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_BASE_URL"))
	apiKey = strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_API_KEY"))
	if baseURL == "" || apiKey == "" {
		t.Skip("set AI_COVE_DEEPSEEK_E2E_BASE_URL and AI_COVE_DEEPSEEK_E2E_API_KEY to replay the DeepSeek contract against the real upstream")
	}
	model = strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_MODEL"))
	if model == "" {
		model = "deepseek-v4.1-flash"
	}
	session = strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_SESSION"))
	if session == "" {
		session = "ai-cove-live-e2e"
	}
	return baseURL, apiKey, model, session
}

// 按线上真实请求的形状发：声明 tools、带 include 与 reasoning，只有这样才能触发上游的校验。
func deepSeekLiveBody(t *testing.T, model string, input json.RawMessage) []byte {
	t.Helper()
	return mustJSON(t, map[string]any{
		"model":             model,
		"input":             input,
		"max_output_tokens": 16,
		"stream":            true,
		"include":           []string{"reasoning.encrypted_content"},
		"store":             false,
		"reasoning":         map[string]any{"effort": "high", "summary": "auto"},
		"tools": []map[string]any{{
			"type":        "function",
			"name":        "bash",
			"description": "run a shell command",
			"parameters":  map[string]any{"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}},
		}},
	})
}

func postDeepSeekLiveUpstream(t *testing.T, client *http.Client, url, apiKey, session string, body []byte) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	if session != "" {
		request.Header.Set("x-opencode-session", session)
	}
	response, err := client.Do(request)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	payload, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(payload)
}

// 侧边会话投递形状（2026-09-18 23:01 生产 422 现场）：Codex 会发出没有 call_id 的
// function_call_output，上游曾对这种缺字段硬拒；改写成 user 轮后必须放行。
// 2026-10-19 实测上游已不再硬拒（见下方分支），测试保留原始状态观测 + 规范化后必须成功。
func TestDeepSeekLiveUpstreamCallLessToolOutput(t *testing.T) {
	baseURL, apiKey, model, session := deepSeekLiveTarget(t)
	client := &http.Client{Timeout: 120 * time.Second}

	raw := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "按顺序回复 ok"}}},
		{"type": "function_call_output", "id": "fco_live_1", "name": "send_message_to_thread", "namespace": "codex_app",
			"output": "<codex_delegation><input>Reply with exactly one word: BANANA</input></codex_delegation>"},
	})

	status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
		fmt.Sprintf("%s-idless-raw-%d", session, time.Now().UnixNano()), deepSeekLiveBody(t, model, raw))
	if status >= http.StatusBadRequest {
		require.Contains(t, responseBody, "call_id", "拒绝理由不再是 call_id：%s", responseBody)
	} else {
		// 2026-10-19 用线上 key 实测：上游已接受缺 call_id 的 tool output（不再硬拒）。
		// 改写 user 轮仍保留：既保住投递正文，也不依赖这条上游行为的稳定性。
		requireDeepSeekLiveSuccess(t, status, responseBody)
		t.Logf("上游已接受缺 call_id 的 tool output（status=%d）", status)
	}

	status, responseBody = postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
		fmt.Sprintf("%s-idless-fix-%d", session, time.Now().UnixNano()), deepSeekLiveBody(t, model, normalizeDeepSeekAgentMessages(raw)))
	requireDeepSeekLiveSuccess(t, status, responseBody)
}

// 短输出预算允许 response.incomplete，但不能把 HTTP 200 或 response.created 当成成功。
func requireDeepSeekLiveSuccess(t *testing.T, status int, body string) {
	t.Helper()
	require.Equal(t, http.StatusOK, status, "上游拒绝请求：%s", body)
	created, terminal := false, false
	for line := range strings.SplitSeq(body, "\n") {
		data, ok := strings.CutPrefix(line, "data:")
		if !ok || strings.TrimSpace(data) == "[DONE]" {
			continue
		}
		var event struct {
			Type string `json:"type"`
		}
		require.NoError(t, common.Unmarshal([]byte(data), &event))
		require.NotEqual(t, "error", event.Type, "流内错误：%s", data)
		require.NotEqual(t, "response.failed", event.Type, "流内失败：%s", data)
		created = created || event.Type == "response.created"
		terminal = terminal || event.Type == "response.completed" || event.Type == "response.incomplete"
	}
	require.True(t, created, "缺少 response.created：%s", body)
	require.True(t, terminal, "缺少 Responses 终态：%s", body)
}

// 2026-10-19 16:33 生产现场：chat completions → responses 转换产出的 reasoning item
// 只有 summary、content 为空、没有 encrypted_content；这种历史 fallback 到原生
// responses 渠道时，上游要求 thinking 模式回传 reasoning_text。丢弃不可回放的
// reasoning 后，同一份历史必须能在原生类渠道继续跑。
//
// 合成小形状可能不会触发现有账号的拒绝（账号/历史规模差异），完整的“原始必须 400、
// 修复后必须 200”契约由 TestDeepSeekLiveUpstreamProductionPayloadReplay 覆盖。
func TestDeepSeekLiveUpstreamChatClassHistoryFallback(t *testing.T) {
	baseURL, apiKey, model, session := deepSeekLiveTarget(t)
	client := &http.Client{Timeout: 120 * time.Second}

	chatReasoning := func(id, text string) map[string]any {
		return map[string]any{
			"type":    "reasoning",
			"id":      id,
			"status":  "completed",
			"role":    "",
			"content": nil,
			"summary": []map[string]any{{"type": "summary_text", "text": text}},
			"quality": "",
			"size":    "",
		}
	}
	raw := mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "按顺序回复 ok"}}},
		chatReasoning("chatcmpl-live-a_reasoning_0", "先看目录"),
		{"type": "message", "role": "assistant", "phase": "commentary", "content": []map[string]any{{"type": "output_text", "text": "我先跑一条命令。"}}},
		{"type": "function_call", "call_id": "call_00_ET_chatfallback000000000001", "name": "bash", "arguments": `{"command":"pwd"}`},
		{"type": "function_call_output", "call_id": "call_00_ET_chatfallback000000000001", "output": "/tmp\n"},
		chatReasoning("chatcmpl-live-b_reasoning_0", "再看结果"),
		{"type": "function_call", "call_id": "call_00_ET_chatfallback000000000002", "name": "bash", "arguments": `{"command":"pwd"}`},
		{"type": "function_call_output", "call_id": "call_00_ET_chatfallback000000000002", "output": "/tmp\n"},
	})

	normalized := NormalizeResponsesInput(raw)
	require.NotContains(t, string(normalized), "_reasoning_0", "不可回放的 reasoning 必须被丢弃")
	status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
		fmt.Sprintf("%s-chat-history-fix-%d", session, time.Now().UnixNano()), deepSeekLiveBody(t, model, normalized))
	requireDeepSeekLiveSuccess(t, status, responseBody)
}

// 生产 dump 回放：把 `deepseek_upstream_failure payload` 分片还原出的完整出站 body
// 通过 AI_COVE_DEEPSEEK_E2E_PAYLOAD_FILE 传入。用同一份 payload 做单变量对照：
// 原始 body 必须复现 400 reasoning_text；只丢不可回放 reasoning 仍应 400（形状不是
// 唯一根因）；只把 call_id 外部化即 200；两项都做后 200——完整复现并验收 2026-10-19
// 16:33 事故（chat 类渠道历史 fallback 到原生类渠道被整轮拒绝）。
func TestDeepSeekLiveUpstreamProductionPayloadReplay(t *testing.T) {
	baseURL, apiKey, _, session := deepSeekLiveTarget(t)
	payloadPath := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_PAYLOAD_FILE"))
	if payloadPath == "" {
		t.Skip("set AI_COVE_DEEPSEEK_E2E_PAYLOAD_FILE to a reassembled deepseek_upstream_failure payload")
	}
	payload, err := os.ReadFile(payloadPath)
	require.NoError(t, err)

	var body map[string]json.RawMessage
	require.NoError(t, common.Unmarshal(payload, &body))
	// 生产 body 的 max_output_tokens 是 32768；回放只需要验证输入形状，限制输出成本。
	body["max_output_tokens"] = json.RawMessage(`16`)
	delete(body, "prompt_cache_key")
	originalInput := body["input"]
	client := &http.Client{Timeout: 120 * time.Second}

	post := func(name string) (int, string) {
		raw, err := common.Marshal(body)
		require.NoError(t, err)
		return postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
			fmt.Sprintf("%s-production-%s-%d", session, name, time.Now().UnixNano()), raw)
	}
	requireRejected := func(name string) {
		t.Helper()
		status, responseBody := post(name)
		require.Equal(t, http.StatusBadRequest, status, "生产 payload 未复现上游拒绝：%s", responseBody)
		require.Contains(t, responseBody, "reasoning_text")
	}

	requireRejected("raw")

	body["input"] = dropUnreplayableDeepSeekReasoning(originalInput)
	requireRejected("drop_reasoning_only")

	body["input"] = makeDeepSeekCallIDsPortable(originalInput)
	status, responseBody := post("portable_call_ids_only")
	require.Equal(t, http.StatusOK, status, "只做 call_id 外部化应已被上游接受：%s", responseBody)

	body["input"] = NormalizeResponsesInput(originalInput)
	fixed, err := common.Marshal(body)
	require.NoError(t, err)
	require.NotContains(t, string(fixed), "_reasoning_0", "不可回放的 reasoning 必须被丢弃")
	status, responseBody = postDeepSeekLiveUpstream(t, client, baseURL, apiKey,
		fmt.Sprintf("%s-production-fixed-%d", session, time.Now().UnixNano()), fixed)
	requireDeepSeekLiveSuccess(t, status, responseBody)
}

// 反向 fallback：原生 responses 历史落到 chat 类渠道时，转换后的 chat body 不能再带
// reasoning_text content part（DeepSeek 兼容 chat 上游 422 unknown variant reasoning_text）。
// 需要显式提供 chat completions 地址：AI_COVE_DEEPSEEK_E2E_CHAT_URL。
func TestDeepSeekLiveUpstreamNativeHistoryToChat(t *testing.T) {
	_, _, model, session := deepSeekLiveTarget(t)
	chatURL := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_CHAT_URL"))
	chatAPIKey := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_CHAT_API_KEY"))
	if chatURL == "" || chatAPIKey == "" {
		t.Skip("set AI_COVE_DEEPSEEK_E2E_CHAT_URL and AI_COVE_DEEPSEEK_E2E_CHAT_API_KEY for the chat-class fallback direction")
	}
	client := &http.Client{Timeout: 120 * time.Second}
	stream := true
	maxOutputTokens := uint(16)
	// 生产链路先过 host 侧规范化（call_id 转外部命名空间、丢不可回放 reasoning），
	// 再做 responses → chat 转换；fixture 用上游签发形态的 call_id 覆盖改写路径。
	input := NormalizeResponsesInput(mustJSON(t, []map[string]any{
		{"type": "message", "role": "user", "content": []map[string]any{{"type": "input_text", "text": "按顺序回复 ok"}}},
		{"type": "reasoning", "id": "rs_live_native_1", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "先看目录"}}, "encrypted_content": "opaque-live-1"},
		{"type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "我先跑一条命令。"}}},
		{"type": "function_call", "call_id": "call_00_cwBm5ITHjfMIIw3Drstn9324", "name": "bash", "arguments": `{"command":"pwd"}`},
		{"type": "function_call_output", "call_id": "call_00_cwBm5ITHjfMIIw3Drstn9324", "output": "/tmp\n"},
		{"type": "reasoning", "id": "rs_live_native_2", "summary": []any{}, "content": []map[string]any{{"type": "reasoning_text", "text": "再看结果"}}, "encrypted_content": "opaque-live-2"},
		{"type": "function_call", "call_id": "call_01_xlz65RztUGxjV9glpllb7847", "name": "bash", "arguments": `{"command":"pwd"}`},
		{"type": "function_call_output", "call_id": "call_01_xlz65RztUGxjV9glpllb7847", "output": "/tmp\n"},
	}))
	require.NotContains(t, string(input), "call_00_cwBm5ITHjfMIIw3Drstn9324",
		"上游签发形态的 call_id 必须被改写成外部命名空间")
	converted, err := relayconvert.ResponsesRequestToChatCompletionsRequest(&dto.OpenAIResponsesRequest{
		Model:           model,
		Stream:          &stream,
		MaxOutputTokens: &maxOutputTokens,
		Reasoning:       &dto.Reasoning{Effort: "high", Summary: "auto"},
		Tools: mustJSON(t, []map[string]any{{
			"type":        "function",
			"name":        "bash",
			"description": "run a shell command",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"command": map[string]any{"type": "string"}},
				"required":   []string{"command"},
			},
		}}),
		Input: input,
	})
	require.NoError(t, err)

	body, err := common.Marshal(converted)
	require.NoError(t, err)
	require.NotContains(t, string(body), "reasoning_text")
	require.Contains(t, string(body), "call_00_ET_")
	status, responseBody := postDeepSeekLiveUpstream(t, client, chatURL, chatAPIKey,
		fmt.Sprintf("%s-native-to-chat-%d", session, time.Now().UnixNano()), body)
	require.Equal(t, http.StatusOK, status, "chat 类上游拒绝原生历史转换结果：%s", responseBody)
}

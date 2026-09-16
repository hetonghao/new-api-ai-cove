package deepseek

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
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
	baseURL := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_BASE_URL"))
	apiKey := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_API_KEY"))
	if baseURL == "" || apiKey == "" {
		t.Skip("set AI_COVE_DEEPSEEK_E2E_BASE_URL and AI_COVE_DEEPSEEK_E2E_API_KEY to replay the DeepSeek contract against the real upstream")
	}
	model := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_MODEL"))
	if model == "" {
		model = "deepseek-v4.1-flash"
	}
	session := strings.TrimSpace(os.Getenv("AI_COVE_DEEPSEEK_E2E_SESSION"))
	if session == "" {
		session = "ai-cove-live-e2e"
	}
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

			// 按线上真实请求的形状发：声明 tools、带 include 与 reasoning，
			// 只有这样才能触发上游的工具轮校验。
			body := mustJSON(t, map[string]any{
				"model":             model,
				"input":             converted.Input,
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
			caseSession := fmt.Sprintf("%s-%d-%d", session, time.Now().UnixNano(), index)
			status, responseBody := postDeepSeekLiveUpstream(t, client, baseURL, apiKey, caseSession, body)

			if tc.reject {
				require.Equal(t, http.StatusBadRequest, status, "上游不再拒绝 assistant message → reasoning，规则已变：%s", responseBody)
				require.Contains(t, responseBody, "reasoning_text")
				return
			}
			require.NotEqual(t, http.StatusBadRequest, status, "上游拒绝了转换后的 payload：%s", responseBody)
			require.True(t, status >= 200 && status < 300, "上游返回 %d：%s", status, responseBody)
		})
	}
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

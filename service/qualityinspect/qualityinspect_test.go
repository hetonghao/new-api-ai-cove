package qualityinspect

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configFixture() model.QualityConfig {
	return model.QualityConfig{Model: "gpt-6-astra", OutputType: "svg", Mode: "channel", Protocol: "responses", TokenID: 1, Group: "default", ChannelIDs: []int{2}, Prompt: "Generate an SVG image of a pelican riding a bicycle by the seaside.", MaxOutputTokens: 16384, SamplesPerTarget: 3, TimeoutSeconds: 10, DailyLimit: 20}
}

func TestQualityRequestHasExactPromptAndNoHiddenInstructions(t *testing.T) {
	cfg := configFixture()
	path, body, err := BuildRequest(cfg)
	require.NoError(t, err)
	assert.Equal(t, "/v1/responses", path)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, []any{map[string]any{"role": "user", "content": cfg.Prompt}}, payload["input"])
	assert.Equal(t, float64(16384), payload["max_output_tokens"])
	assert.Equal(t, false, payload["store"])
	assert.NotContains(t, payload, "reasoning")
	assert.NotContains(t, payload, "tools")
	assert.NotContains(t, payload, "temperature")
	zero := 0.0
	cfg.Temperature = &zero
	cfg.Protocol = "chat"
	path, body, err = BuildRequest(cfg)
	require.NoError(t, err)
	assert.Equal(t, "/v1/chat/completions", path)
	require.NoError(t, common.Unmarshal(body, &payload))
	assert.Equal(t, float64(0), payload["temperature"])
	assert.Equal(t, float64(16384), payload["max_completion_tokens"])
	cfg.MaxOutputTokens = model.QualityMaxOutputTokens + 1
	_, _, err = BuildRequest(cfg)
	assert.Error(t, err)
}

var capturedPelicanSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 600" width="100%" height="100%">
  <defs>
    <!-- Wheel Spoke Pattern -->
    <pattern id="spokes" width="20" height="20" patternUnits="userSpaceOnUse">
      <line x1="10" y1="0" x2="10" y2="20" stroke="#BDC3C7" stroke-width="2" opacity="0.5"/>
      <line x1="0" y1="10" x2="20" y2="10" stroke="#BDC3C7" stroke-width="2" opacity="0.5"/>
    </pattern>
  </defs>
  <rect width="800" height="600" fill="#A9D0F5" />
  <circle cx="120" cy="120" r="60" fill="#F7DC6F" />
  <g fill="#FFFFFF" opacity="0.8">
    <circle cx="220" cy="120" r="25" />
    <rect x="220" y="120" width="60" height="25" rx="10" />
  </g>
  <g id="bicycle">
    <circle cx="250" cy="500" r="70" fill="none" stroke="#2C3E50" stroke-width="14" />
    <circle cx="250" cy="500" r="50" fill="url(#spokes)" />
    <path d="M250,500 L400,500 L360,380 L250,500 M400,500 L540,380 L550,500 M360,380 L540,380" fill="none" stroke="#E74C3C" stroke-width="12" stroke-linecap="round" stroke-linejoin="round" />
    <path d="M250,495 L400,485 M250,505 L400,515" stroke="#2C3E50" stroke-width="4" fill="none" stroke-dasharray="3 3" />
    <ellipse cx="350" cy="375" rx="30" ry="12" fill="#2C3E50" />
    <line x1="360" y1="380" x2="355" y2="400" stroke="#2C3E50" stroke-width="8" stroke-linecap="round" />
  </g>
</svg>`

func TestQualitySVGAllowlistAndNoRepair(t *testing.T) {
	valid := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100"><rect width="100" height="100" fill="#fff"/></svg>`
	styled := `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="sea"><stop offset="0%" stop-color="#abc"/></linearGradient></defs><style>.water { fill: url(#sea); stroke-width: 2; }</style><rect class="water" width="100" height="100"/></svg>`
	for _, tc := range []struct{ name, content, want string }{
		{"valid", valid, ""},
		{"fenced", "Here is the SVG:\n```svg\n" + valid + "\n```", ""},
		{"gradient CSS", styled, ""},
		{"captured pattern sample", "Here’s a clean SVG illustration of a pelican riding a bicycle along the seaside.\n```svg\n" + capturedPelicanSVG + "\n```\nThe scene combines a bright beach sky with a whimsical pelican on a red bicycle.", ""},
		{"pattern fill", `<svg xmlns="http://www.w3.org/2000/svg"><defs><pattern id="p" width="10" height="10" patternUnits="userSpaceOnUse"><rect width="5" height="5"/></pattern></defs><rect width="50" height="50" fill="url(#p)"/></svg>`, ""},
		{"use local ref", `<svg xmlns="http://www.w3.org/2000/svg"><defs><symbol id="dot"><circle r="4"/></symbol></defs><use href="#dot" x="10" y="10"/></svg>`, ""},
		{"clipPath and mask", `<svg xmlns="http://www.w3.org/2000/svg"><defs><clipPath id="c"><circle cx="10" cy="10" r="10"/></clipPath><mask id="m" maskUnits="userSpaceOnUse"><rect width="20" height="20" fill="#fff"/></mask></defs><rect width="20" height="20" clip-path="url(#c)" mask="url(#m)"/></svg>`, ""},
		{"filter primitives", `<svg xmlns="http://www.w3.org/2000/svg"><defs><filter id="f" filterUnits="userSpaceOnUse"><feGaussianBlur in="SourceGraphic" stdDeviation="2"/><feDropShadow dx="1" dy="1" flood-color="#000" flood-opacity="0.3"/></filter></defs><rect width="20" height="20" filter="url(#f)"/></svg>`, ""},
		{"marker", `<svg xmlns="http://www.w3.org/2000/svg"><defs><marker id="arrow" markerWidth="10" markerHeight="10" refX="5" refY="5" orient="auto" markerUnits="strokeWidth"><path d="M0,0L10,5L0,10z"/></marker></defs><line x1="0" y1="0" x2="20" y2="20" stroke="#000" marker-end="url(#arrow)"/></svg>`, ""},
		{"smil animation", `<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"><animate attributeName="x" from="0" to="10" dur="1s" begin="0s" repeatCount="indefinite" fill="freeze"/><animateTransform attributeName="transform" type="rotate" from="0 5 5" to="360 5 5" dur="2s" additive="sum" repeatCount="indefinite"/></rect></svg>`, ""},
		{"xlink href", `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink"><defs><circle id="c" r="5"/></defs><use xlink:href="#c"/></svg>`, ""},
		{"gradient href", `<svg xmlns="http://www.w3.org/2000/svg"><defs><linearGradient id="a"><stop offset="0" stop-color="#fff"/></linearGradient><linearGradient id="b" href="#a"/></defs><rect width="10" height="10" fill="url(#b)"/></svg>`, ""},
		{"textPath", `<svg xmlns="http://www.w3.org/2000/svg"><defs><path id="curve" d="M0,50 Q50,0 100,50"/></defs><text><textPath href="#curve" startOffset="10%">hello</textPath></text></svg>`, ""},
		{"xml space", `<svg xmlns="http://www.w3.org/2000/svg"><text xml:space="preserve"> hi </text></svg>`, ""},
		{"style transform", `<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10" style="transform: rotate(45deg); fill: #abc"/></svg>`, ""},
		{"transform attr", `<svg xmlns="http://www.w3.org/2000/svg"><g transform="translate(10 20) rotate(45)"><rect width="10" height="10"/></g></svg>`, ""},
		{"multiple", valid + valid, "ambiguous_svg"},
		{"truncated", strings.TrimSuffix(valid, "</svg>"), "invalid_xml"},
		{"missing", "I cannot create this image.", "missing_svg"},
		{"script", `<svg><script>alert(1)</script><rect/></svg>`, "unsafe_svg"},
		{"event", `<svg><rect onload="alert(1)"/></svg>`, "unsafe_svg"},
		{"foreignobject", `<svg><foreignObject><div/></foreignObject></svg>`, "unsafe_svg"},
		{"image", `<svg><image href="https://example.test/track"/></svg>`, "unsafe_svg"},
		{"feimage", `<svg><filter><feImage href="#x"/></filter><rect/></svg>`, "unsafe_svg"},
		{"anchor", `<svg><a href="#x"><rect/></a></svg>`, "unsafe_svg"},
		{"nested svg", `<svg><svg><rect/></svg></svg>`, "ambiguous_svg"},
		{"use external", `<svg><use href="https://example.test/x"/><rect/></svg>`, "unsafe_svg"},
		{"xlink javascript", `<svg xmlns:xlink="http://www.w3.org/1999/xlink"><use xlink:href="javascript:alert(1)"/><rect/></svg>`, "unsafe_svg"},
		{"xlink undeclared", `<svg><use xlink:href="https://example.test/x"/><rect/></svg>`, "unsafe_svg"},
		{"bad xlink decl", `<svg xmlns:xlink="https://example.test/"><use xlink:href="#a"/><rect/></svg>`, "unsafe_svg"},
		{"foreign attr ns", `<svg xmlns:evil="https://example.test/"><rect evil:href="#a"/></svg>`, "unsafe_svg"},
		{"externalstyle", `<svg><rect style="fill:url(https://example.test/x)"/></svg>`, "unsafe_svg"},
		{"cssescape", `<svg><rect style="fill:u\72l(https://example.test/x)"/></svg>`, "unsafe_svg"},
		{"cssimport", `<svg><style>@import 'https://example.test/x';</style><rect/></svg>`, "unsafe_svg"},
		{"cssnested", `<svg><style>@media all {.x{fill:red}}</style><rect/></svg>`, "unsafe_svg"},
		{"entity", `<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]><svg><text>&x;</text></svg>`, "unsafe_svg"},
		{"localrecursion", `<svg><g id="loop"><use href="#loop"/></g></svg>`, ""},
		{"danglingref", `<svg><rect fill="url(#missing)"/></svg>`, "unsafe_svg"},
		{"danglinguse", `<svg><use href="#missing"/><rect/></svg>`, "unsafe_svg"},
		{"duplicateid", `<svg><rect id="a"/><circle id="a"/></svg>`, "invalid_xml"},
		{"baddimension", `<svg width="1e99"><rect/></svg>`, "svg_too_complex"},
		{"empty", `<svg></svg>`, "empty_svg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svg, code := ExtractSafeSVG(tc.content)
			assert.Equal(t, tc.want, code)
			if tc.want == "" {
				assert.NotEmpty(t, svg)
			} else {
				assert.Empty(t, svg)
			}
		})
	}
	got, code := ExtractSafeSVG(styled)
	require.Empty(t, code)
	assert.Equal(t, styled, got)
	_, code = ExtractSafeSVG(`<svg><path d="` + strings.Repeat("M0 0 ", 15000) + `"/></svg>`)
	assert.Equal(t, "svg_too_complex", code)
}

func TestQualityProtocolTerminalCases(t *testing.T) {
	for _, tc := range []struct{ name, protocol, contentType, body, status, code, text string }{
		{"responses complete", "responses", "text/event-stream", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"model\":\"reported\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello\"}]}],\"usage\":{\"input_tokens\":0,\"output_tokens\":2}}}\n\n", "succeeded", "", "hello"},
		{"responses missing terminal", "responses", "text/event-stream", "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n", "failed", "missing_terminal", "hello"},
		{"responses incomplete", "responses", "text/event-stream", "data: {\"type\":\"response.incomplete\",\"response\":{\"status\":\"incomplete\"}}\n\n", "failed", "truncated", ""},
		{"HTTP200 stream error", "responses", "text/event-stream", "data: {\"type\":\"error\",\"error\":{\"message\":\"secret should not persist\"}}\n\n", "failed", "stream_error", ""},
		{"chat complete", "chat", "text/event-stream", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"choices\":[{\"index\":0,\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", "succeeded", "", "hello"},
		{"chat missing DONE", "chat", "text/event-stream", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}]}\n\n", "failed", "missing_terminal", "hello"},
		{"chat length", "chat", "text/event-stream", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":\"length\"}]}\n\n", "failed", "truncated", "partial"},
		{"chat refusal", "chat", "application/json", `{"choices":[{"index":0,"message":{"refusal":"No"},"finish_reason":"stop"}]}`, "failed", "refusal", ""},
		{"JSON empty", "responses", "application/json", `{"status":"completed","output":[]}`, "failed", "empty_output", ""},
		{"JSON text", "responses", "application/json", `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"ok"}]}]}`, "succeeded", "", "ok"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseResponse(strings.NewReader(tc.body), tc.contentType, tc.protocol, time.Now())
			assert.Equal(t, tc.status, result.Status)
			assert.Equal(t, tc.code, result.ErrorCode)
			assert.Equal(t, tc.text, string(result.Text))
			assert.Equal(t, tc.status == "succeeded", result.RequestSuccess)
			if tc.name == "responses complete" {
				require.NotNil(t, result.InputTokens)
				assert.Zero(t, *result.InputTokens)
				require.NotNil(t, result.OutputTokens)
				assert.Equal(t, int64(2), *result.OutputTokens)
				assert.NotNil(t, result.FirstTextMs)
			}
			if tc.name == "JSON text" {
				assert.Nil(t, result.InputTokens)
				assert.Nil(t, result.OutputTokens)
			}
		})
	}
	result := ParseResponse(strings.NewReader(strings.Repeat("x", model.QualityMaxEventBytes+1)), "application/json", "responses", time.Now())
	assert.Equal(t, "response_too_large", result.ErrorCode)
}

type streamChunkReader struct {
	chunk []byte
	left  int64
	head  []byte
}

func (r *streamChunkReader) Read(p []byte) (int, error) {
	if r.left <= 0 && len(r.head) == 0 {
		return 0, io.EOF
	}
	n := 0
	for n < len(p) && (len(r.head) > 0 || r.left > 0) {
		if len(r.head) == 0 {
			r.head = r.chunk
		}
		take := min(len(r.head), len(p)-n)
		copy(p[n:], r.head[:take])
		r.head = r.head[take:]
		r.left -= int64(take)
		n += take
	}
	return n, nil
}

func TestQualityStreamSizeLimits(t *testing.T) {
	delta := func(text string) string {
		return "data: {\"type\":\"response.output_text.delta\",\"delta\":\"" + text + "\"}\n\n"
	}
	t.Run("accumulated output over 2 MiB", func(t *testing.T) {
		var body strings.Builder
		for range 33 {
			body.WriteString(delta(strings.Repeat("x", 64<<10)))
		}
		result := ParseResponse(strings.NewReader(body.String()), "text/event-stream", "responses", time.Now())
		assert.Equal(t, "failed", result.Status)
		assert.Equal(t, "response_too_large", result.ErrorCode)
	})
	t.Run("raw stream over 64 MiB", func(t *testing.T) {
		reader := &streamChunkReader{
			chunk: []byte(delta("a")),
			left:  model.QualityMaxStreamBytes + 1<<20,
		}
		result := ParseResponse(reader, "text/event-stream", "responses", time.Now())
		assert.Equal(t, "failed", result.Status)
		assert.Equal(t, "stream_too_large", result.ErrorCode)
	})
	t.Run("single event over 8 MiB", func(t *testing.T) {
		body := delta(strings.Repeat("x", model.QualityMaxEventBytes+1))
		result := ParseResponse(strings.NewReader(body), "text/event-stream", "responses", time.Now())
		assert.Equal(t, "failed", result.Status)
		assert.Equal(t, "stream_event_too_large", result.ErrorCode)
	})
	t.Run("3 MiB wrapped stream with small text succeeds", func(t *testing.T) {
		var body strings.Builder
		for body.Len() < 3<<20 {
			body.WriteString(delta("abc"))
		}
		body.WriteString("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n")
		result := ParseResponse(strings.NewReader(body.String()), "text/event-stream", "responses", time.Now())
		assert.Equal(t, "succeeded", result.Status)
	})
}

func TestQualityHTTPExecutorDistinguishesRequestAndArtifactSuccess(t *testing.T) {
	for _, tc := range []struct{ name, text, status, validation string }{
		{"safe", `<svg xmlns="http://www.w3.org/2000/svg"><rect width="5" height="5"/></svg>`, "succeeded", "safe"},
		{"unsafe", `<svg><script>alert(1)</script><rect/></svg>`, "failed", "unsafe_svg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configFixture()
			_, payload, err := BuildRequest(cfg)
			require.NoError(t, err)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				assert.Equal(t, string(payload), string(body))
				assert.Equal(t, "Bearer local-test-token", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set(common.RequestIdKey, "quality-test-request")
				data, err := common.Marshal(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed", "output": []any{map[string]any{"type": "message", "content": []any{map[string]any{"type": "output_text", "text": tc.text}}}}}})
				require.NoError(t, err)
				_, err = w.Write(append(append([]byte("data: "), data...), []byte("\n\n")...))
				require.NoError(t, err)
			}))
			defer server.Close()
			result := execute(context.Background(), server.Client(), server.URL, "local-test-token", payload, cfg)
			assert.Equal(t, tc.status, result.Status)
			assert.Equal(t, tc.validation, result.Validation)
			assert.True(t, result.RequestSuccess)
			assert.Equal(t, tc.text, string(result.Text))
			assert.Equal(t, "quality-test-request", result.RequestID)
		})
	}
}

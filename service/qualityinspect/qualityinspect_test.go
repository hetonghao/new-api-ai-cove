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

var prodLabelledSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="960" height="720" viewBox="0 0 960 720"
     role="img" aria-labelledby="title desc">
  <title id="title">A pelican riding a bicycle by the seaside</title>
  <desc id="desc">
    A cheerful white pelican with a golden bill and a coral scarf rides a teal
    bicycle along a sunny seaside promenade, with waves and a sailboat behind it.
  </desc>

  <defs>
    <linearGradient id="sky" x2="0" y2="1">
      <stop stop-color="#bce5ec"/>
      <stop offset="1" stop-color="#edf5e8"/>
    </linearGradient>
    <linearGradient id="sea" x2="0" y2="1">
      <stop stop-color="#5ebbbb"/>
      <stop offset="1" stop-color="#a0d9cb"/>
    </linearGradient>
    <linearGradient id="bill" x2="0" y2="1">
      <stop stop-color="#ffd779"/>
      <stop offset="1" stop-color="#edaa4c"/>
    </linearGradient>
    <pattern id="sand" width="55" height="37" patternUnits="userSpaceOnUse">
      <circle cx="12" cy="12" r="1.3" fill="#cda779" opacity=".4"/>
      <circle cx="40" cy="28" r="1" fill="#cda779" opacity=".35"/>
    </pattern>
  </defs>

  <!-- Sunlit coast -->
  <path fill="url(#sky)" d="M0 0h960v720H0z"/>
  <circle cx="765" cy="131" r="62" fill="#ffe5a0"/>
  <circle cx="765" cy="131" r="79" fill="#ffe5a0" opacity=".2"/>

  <g fill="#fffdf2" opacity=".85">
    <path d="M92 132c-5-22 24-39 43-26 8-31 58-33 70-3 27-9 48 8 47 29Z"/>
    <path d="M359 82c0-18 22-28 37-17 10-24 44-21 50 3 18-5 35 4 36 14Z"/>
  </g>

  <path d="M0 314Q89 264 187 298T375 316L375 366H0Z" fill="#7fbab1"/>
  <path d="M0 338Q141 307 269 333T535 337T960 322V489H0Z" fill="url(#sea)"/>

  <g fill="none" stroke="#e2f4e7" stroke-linecap="round">
    <path d="M28 366h97m37 10h66m391-15h85m94 18h122" stroke-width="4"/>
    <path d="M69 412h125m41-13h57m308 12h126m47 22h111" stroke-width="3"/>
    <path d="M17 456q90-17 173-1t173-1 173 1 173-1 251-2" stroke-width="6"/>
  </g>

  <!-- Small sailboat -->
  <g stroke="#456b70" stroke-linejoin="round">
    <path d="M811 261v100" fill="none" stroke-width="4"/>
    <path d="M803 273l-53 74h53Z" fill="#fff9e9" stroke="none"/>
    <path d="M821 293l33 54h-33Z" fill="#f3bc87" stroke="none"/>
    <path d="M744 357h120l-19 17h-79Z" fill="#527c83" stroke="none"/>
  </g>

  <!-- Distant seabirds -->
  <g fill="none" stroke="#628b91" stroke-width="3" stroke-linecap="round">
    <path d="M591 164q12-13 24 0 12-13 24 0"/>
    <path d="M672 219q9-10 18 0 9-10 18 0"/>
    <path d="M288 210q10-11 20 0 10-11 20 0"/>
  </g>

  <path d="M0 472q177-20 355 3t355 0 250-4v249H0Z" fill="#f2dcaf"/>
  <path d="M0 472q177-20 355 3t355 0 250-4" fill="none" stroke="#fff8e5" stroke-width="12"/>
  <path d="M0 499h960v221H0z" fill="url(#sand)"/>
  <path d="M0 566Q465 538 960 562v158H0Z" fill="#e9c99c"/>
  <path d="M0 566Q465 538 960 562" fill="none" stroke="#f9e8c7" stroke-width="7"/>

  <!-- Bicycle shadow -->
  <ellipse cx="479" cy="620" rx="282" ry="23" fill="#ac916f" opacity=".23"/>

  <!-- Wheels and spokes -->
  <g fill="#e9dbbd" fill-opacity=".28" stroke="#304e58" stroke-width="13">
    <circle cx="291" cy="519" r="101"/>
    <circle cx="677" cy="519" r="101"/>
  </g>
  <g fill="none" stroke="#fff4d6" stroke-width="3">
    <circle cx="291" cy="519" r="91"/>
    <circle cx="677" cy="519" r="91"/>
  </g>
  <g stroke="#718a86" stroke-width="2" opacity=".8">
    <path d="M291 428v182m-91-91h182m-155-64 128 128m-128 0 128-128
             M256 435l70 168m-119-119 168 70m-168 0 168-70m-119 119 70-168"/>
    <path d="M677 428v182m-91-91h182m-155-64 128 128m-128 0 128-128
             M642 435l70 168m-119-119 168 70m-168 0 168-70m-119 119 70-168"/>
  </g>

  <!-- Frame -->
  <g fill="none" stroke-linecap="round" stroke-linejoin="round">
    <path d="M291 519l106-151 77 151H291l157-126h167L474 519
             M615 393l62 126" stroke="#286a70" stroke-width="12"/>
    <path d="M397 368l-13-37m231 62-16-57 27-34h30" stroke="#286a70" stroke-width="10"/>
    <path d="M615 393l62 126" stroke="#56a29c" stroke-width="5"/>
    <path d="M636 302h24" stroke="#304e58" stroke-width="12"/>
    <path d="M627 310q39 54 44 137" stroke="#496971" stroke-width="2.5"/>
    <path d="M218 437a111 111 0 0 1 148 0M604 437a111 111 0 0 1 148 0"
          stroke="#d9a459" stroke-width="7"/>
  </g>
  <path d="M357 320q28-10 65-1 8 3 4 11h-67q-11-3-2-10Z" fill="#795b4e"/>
  <g fill="#d6ded0" stroke="#345e63" stroke-width="4">
    <circle cx="291" cy="519" r="8"/>
    <circle cx="677" cy="519" r="8"/>
    <circle cx="474" cy="519" r="24"/>
  </g>
  <g fill="none" stroke="#46676a" stroke-width="5" stroke-linecap="round">
    <path d="M474 519l-34-32m34 32 37 31"/>
    <path d="M425 487h30m42 63h31"/>
  </g>

  <!-- Far leg -->
  <path d="M415 340l-15 63 43 80" fill="none" stroke="#d69542"
        stroke-width="13" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M438 476l-19 10q15 10 43 1l-12-11Z" fill="#eab352" stroke="#806745" stroke-width="3"/>

  <!-- Pelican body and tail -->
  <g stroke="#3d5960" stroke-width="4" stroke-linejoin="round">
    <path d="M365 285l-74-15 41 38-51-7 64 34Z" fill="#e5e9de"/>
    <path d="M335 281c22-39 73-59 121-35 38 19 55 60 32 92
             -25 35-113 41-147 4-17-19-20-39-6-61Z" fill="#fffdf0"/>
    <path d="M354 282c28-14 73-9 107 30-24 23-68 28-97 9
             19 0 36-3 47-10-26 1-42-9-57-29Z" fill="#dbe4dc"/>
    <path d="M365 290q29 14 56 16" fill="none" stroke="#b1c5bf" stroke-width="3"/>

    <!-- Long neck -->
    <path d="M460 306c42-12 64-40 61-75-2-24-19-42-12-70
             6-27 29-43 52-32 19 9 23 33 10 51
             -16 21-13 35-8 58 12 58-21 98-72 106"
          fill="#fffdf0"/>
    <path d="M535 193c-10 26 11 53-3 81" fill="none" stroke="#dbe4dc" stroke-width="9"/>

    <!-- Head and enormous bill -->
    <path d="M518 145c-2-27 18-46 43-43 25 3 41 21 36 45
             -3 22-23 38-48 30-18-6-28-17-31-32Z" fill="#fffdf0"/>
    <path d="M580 148l165 8c-14 39-58 65-103 57-34-6-53-29-62-65Z" fill="url(#bill)"/>
    <path d="M581 142q86-2 166 14l-167 8Z" fill="#ffdc7d"/>
    <path d="M595 165q43 33 92 24" fill="none" stroke="#d49442" stroke-width="2.5"/>
  </g>

  <!-- Face and feather tuft -->
  <path d="M530 112l-8-18 19 13-1-20 15 17" fill="#fffdf0"
        stroke="#3d5960" stroke-width="3" stroke-linejoin="round"/>
  <circle cx="566" cy="136" r="10" fill="#efcb7d"/>
  <circle cx="568" cy="135" r="5.5" fill="#263f49"/>
  <circle cx="570" cy="133" r="1.8" fill="white"/>
  <path d="M553 119q9-5 16-1" fill="none" stroke="#3d5960" stroke-width="3" stroke-linecap="round"/>

  <!-- Windblown scarf -->
  <path d="M514 198q18 12 41 5l5 16q-27 10-48-7Z" fill="#e57761" stroke="#a65149" stroke-width="3"/>
  <path d="M518 207c-29-17-49-5-75-22l9 23-13 12c33 9 52-7 79 1Z"
        fill="#ec856d" stroke="#a65149" stroke-width="3"/>
  <path d="M449 201q29 12 58 11" fill="none" stroke="#ffd0a5" stroke-width="3"/>

  <!-- Wing reaching the handlebars -->
  <path d="M464 283q42-8 80 14l72 6q13 6 7 14-9 8-25 1
           l-68 4q-37-3-66-20"
        fill="#fffdf0" stroke="#3d5960" stroke-width="4" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M581 308l27 3m-34 4 26 2" fill="none" stroke="#b1c5bf" stroke-width="2.5" stroke-linecap="round"/>

  <!-- Near leg and webbed foot on the pedal -->
  <path d="M462 350l-20 68 65 118" fill="none" stroke="#f0b657"
        stroke-width="14" stroke-linecap="round" stroke-linejoin="round"/>
  <path d="M502 527l-17 21q24 8 55-1l-15-8-11-12Z"
        fill="#f4c168" stroke="#806745" stroke-width="3" stroke-linejoin="round"/>
  <path d="M507 535l4 10m7-8 8 9" stroke="#ce9445" stroke-width="2" stroke-linecap="round"/>

  <!-- Seaside grasses and shells -->
  <g fill="none" stroke="#849e79" stroke-width="4" stroke-linecap="round">
    <path d="M56 638q-4-29-20-43m20 43q6-40 24-56m-24 56-2-46"/>
    <path d="M889 609q-2-27-17-42m17 42q8-37 24-45m-24 45 3-45"/>
  </g>
  <g fill="#fff0d2" stroke="#c79e7c" stroke-width="2">
    <path d="M791 662q16-30 33 0Z"/>
    <path d="M140 673q12-22 25 0Z"/>
  </g>
  <path d="M807 660v-12m-6 13-3-10m15 10 3-10" stroke="#d4b08e" stroke-width="2"/>
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
		{"prod aria-labelledby sample", prodLabelledSVG, ""},
		{"aria idref attrs", `<svg xmlns="http://www.w3.org/2000/svg" role="img" aria-labelledby="t" aria-hidden="false"><title id="t">x</title><rect width="10" height="10" focusable="true"/></svg>`, ""},
		{"aria-labelledby bad id", `<svg><rect aria-labelledby="a b!"/><rect/></svg>`, "unsafe_svg"},
		{"aria-labelledby dangling", `<svg><rect aria-labelledby="missing"/><rect/></svg>`, "unsafe_svg"},
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
		{"JSON empty", "responses", "application/json", `{"status":"completed","output":[]}`, "failed", "no_message", ""},
		{"JSON empty message", "responses", "application/json", `{"status":"completed","output":[{"type":"reasoning"},{"type":"message","role":"assistant","content":[]},{"type":"function_call"}]}`, "failed", "empty_output", ""},
		{"JSON reasoning only", "responses", "application/json", `{"status":"completed","output":[{"type":"reasoning"}]}`, "failed", "no_message", ""},
		{"JSON tool call turn", "responses", "application/json", `{"status":"completed","output":[{"type":"reasoning"},{"type":"function_call","name":"exec"}]}`, "failed", "tool_call_turn", ""},
		{"chat tool calls", "chat", "application/json", `{"choices":[{"index":0,"message":{"content":""},"finish_reason":"tool_calls"}]}`, "failed", "tool_call_turn", ""},
		{"chat tool calls stream", "chat", "text/event-stream", "data: {\"choices\":[{\"index\":0,\"finish_reason\":\"tool_calls\"}]}\n\n", "failed", "tool_call_turn", ""},
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

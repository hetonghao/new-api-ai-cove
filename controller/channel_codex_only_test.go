package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	filterdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestCodexClientRecognition(t *testing.T) {
	for _, tc := range []struct {
		name, ua, originator string
		fingerprint, allowed bool
	}{
		{"cli", "codex_cli_rs/0.141.0 (Mac OS; arm64)", "", true, true},
		{"tui", "codex-tui/0.141.0", "", true, true},
		{"desktop", "Codex Desktop/1.2.3", "", true, true},
		{"vscode", "codex_vscode/0.141.0-alpha.1", "", true, true},
		{"copilot", "codex_vscode_copilot/0.141.0", "", true, true},
		{"app", "codex_app/0.141.0", "", true, true},
		{"chatgpt", "codex_chatgpt_desktop/0.141.0", "", true, true},
		{"atlas", "codex_atlas/0.141.0", "", true, true},
		{"exec", "codex_exec/0.141.0", "", true, true},
		{"sdk", "codex_sdk_ts/0.141.0", "", true, true},
		{"case insensitive", " CODEX_CLI_RS/0.141.0 ", "", true, true},
		{"official originator", "custom/0.141.0", "codex_cli_rs", true, true},
		{"overridden originator", "custom/0.141.0 (Mac OS; arm64) term (codex-tui; 0.141.0)", "custom", true, true},
		{"no fingerprint", "codex_cli_rs/0.141.0", "codex_cli_rs", false, false},
		{"no version", "codex_cli_rs/", "codex_cli_rs", true, false},
		{"empty version", "codex_cli_rs/ (Mac OS)", "codex_cli_rs", true, false},
		{"short version", "codex_cli_rs/0.141", "codex_cli_rs", true, false},
		{"missing ua", "", "codex_cli_rs", true, false},
		{"curl", "curl/8.0.0", "", true, false},
		{"ua substring", "browser/1.2.3 codex_app/0.141.0", "", true, false},
		{"spoofed prefix", "evil-codex_app/0.141.0", "evil-codex_app", true, false},
		{"loose codex prefix", "codex-evil/0.141.0", "codex-evil", true, false},
		{"space before slash", "codex_app /0.141.0", "", true, false},
		{"unclosed trailer", "custom/0.141.0 (codex-tui; 0.141.0", "", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			request.Header.Set("User-Agent", tc.ua)
			request.Header.Set("originator", tc.originator)
			if tc.fingerprint {
				request.Header.Set("X-Codex-Window-Id", "")
			}
			require.Equal(t, tc.allowed, common.IsCodexClientRequest(request))
		})
	}
	require.False(t, common.IsCodexClientRequest(nil))
}

func setTestChannelCodexRestriction(t *testing.T, db *gorm.DB, id int, forbidden bool) {
	t.Helper()
	require.NoError(t, i18n.Init())
	channel, err := model.GetChannelById(id, true)
	require.NoError(t, err)
	settings := channel.GetSetting()
	settings.CodexOnly = !forbidden
	settings.CodexForbidden = forbidden
	channel.SetSetting(settings)
	require.NoError(t, db.Save(channel).Error)
}

// runCodexRoutingCase routes one request through the distributor against a
// restricted channel 801 and an unrestricted channel 802, so the Codex-only
// and Codex-forbidden tables share one fixture.
func runCodexRoutingCase(t *testing.T, cached bool, forbidden bool, tc codexRoutingCase) {
	t.Helper()
	db := setupResponsesWebSocketHandlerTest(t)
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 801, priority: 10})
	setTestChannelCodexRestriction(t, db, 801, forbidden)
	if !tc.onlyRestricted {
		insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 802})
	}
	common.MemoryCacheEnabled = cached
	if cached {
		model.InitChannelCache()
	}
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		if tc.pinned {
			service.GetChannelConstraints(c).AddPin(filterdto.ChannelPin{ChannelId: 801, Source: filterdto.PinSourceToken})
		}
	}, middleware.Distribute(), func(c *gin.Context) { c.JSON(200, gin.H{"channel": c.GetInt("channel_id")}) })
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-4o-mini","input":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	if tc.codex {
		request.Header.Set("User-Agent", "codex_cli_rs/0.141.0")
		request.Header.Set("X-Codex-Window-Id", "test")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
	require.Equal(t, int64(tc.channel), gjson.GetBytes(recorder.Body.Bytes(), "channel").Int())
	if tc.deniedCode != "" {
		require.Equal(t, tc.deniedCode, gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
	}
}

type codexRoutingCase struct {
	name                          string
	codex, pinned, onlyRestricted bool
	status, channel               int
	deniedCode                    string
}

func TestCodexRestrictionHTTPRouting(t *testing.T) {
	tests := []struct {
		name      string
		forbidden bool
		cases     []codexRoutingCase
	}{
		{
			name:      "codex only",
			forbidden: false,
			cases: []codexRoutingCase{
				{name: "ordinary skips restricted", codex: false, status: 200, channel: 802},
				{name: "codex uses restricted", codex: true, status: 200, channel: 801},
				{name: "ordinary pinned denied", codex: false, pinned: true, status: 403, deniedCode: "codex_client"},
				{name: "codex pinned allowed", codex: true, pinned: true, status: 200, channel: 801},
				{name: "no unrestricted channel", codex: false, onlyRestricted: true, status: 503},
			},
		},
		{
			name:      "codex forbidden",
			forbidden: true,
			cases: []codexRoutingCase{
				{name: "codex skips forbidden", codex: true, status: 200, channel: 802},
				{name: "ordinary uses forbidden", codex: false, status: 200, channel: 801},
				{name: "codex pinned denied", codex: true, pinned: true, status: 403, deniedCode: "codex_forbidden"},
				{name: "ordinary pinned allowed", codex: false, pinned: true, status: 200, channel: 801},
				{name: "no unrestricted channel", codex: true, onlyRestricted: true, status: 503},
			},
		},
	}
	for _, cached := range []bool{false, true} {
		for _, restriction := range tests {
			for _, tc := range restriction.cases {
				t.Run(fmt.Sprintf("cache=%t/%s/%s", cached, restriction.name, tc.name), func(t *testing.T) {
					runCodexRoutingCase(t, cached, restriction.forbidden, tc)
				})
			}
		}
	}
}

func TestCodexOnlyRetryExcludesRestrictedChannels(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", cached), func(t *testing.T) {
			db := setupResponsesWebSocketHandlerTest(t)
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 801, priority: 10})
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 802})
			setTestChannelCodexRestriction(t, db, 801, false)
			common.MemoryCacheEnabled = cached
			if cached {
				model.InitChannelCache()
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			param := &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: responsesWebSocketTestModel, RequestPath: "/v1/responses", RequireWebSockets: true, Retry: common.GetPointer(1), TriedChannelIDs: map[int]bool{802: true}, ExcludedChannelIDs: map[int]bool{802: true}}
			selected, _, err := service.CacheGetRandomSatisfiedChannel(param)
			require.NoError(t, err)
			require.Nil(t, selected, "retry must not use a restricted channel after the ordinary channel fails")
		})
	}
}

func TestCodexOnlyWebSocketRouting(t *testing.T) {
	for _, codex := range []bool{false, true} {
		t.Run(fmt.Sprintf("codex=%t", codex), func(t *testing.T) {
			db := setupResponsesWebSocketHandlerTest(t)
			serve := func(conn *websocket.Conn) {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
				if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`)); err != nil {
					t.Errorf("write upstream response: %v", err)
				}
			}
			restricted := newResponsesWebSocketTestUpstream(t, serve)
			ordinary := newResponsesWebSocketTestUpstream(t, serve)
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 801, baseURL: restricted.server.URL, priority: 10})
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 802, baseURL: ordinary.server.URL})
			setTestChannelCodexRestriction(t, db, 801, false)
			client := dialResponsesWebSocketTestClientWithContext(t, func(c *gin.Context) {
				if codex {
					c.Request.Header.Set("User-Agent", "codex_cli_rs/0.141.0")
					c.Request.Header.Set("X-Codex-Window-Id", "test")
				}
			})
			require.NoError(t, client.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.create","model":"gpt-4o-mini","input":"hello"}`)))
			require.Equal(t, "response.completed", gjson.GetBytes(readResponsesWebSocketTestEvent(t, client), "type").String())
			if codex {
				require.EqualValues(t, 1, restricted.connections.Load())
				require.Zero(t, ordinary.connections.Load())
			} else {
				require.Zero(t, restricted.connections.Load())
				require.EqualValues(t, 1, ordinary.connections.Load())
			}
			closeResponsesWebSocketTestClient(client)
		})
	}
}

func TestCodexOnlyWebSocketPinnedRejectedBeforeUpgrade(t *testing.T) {
	db := setupResponsesWebSocketHandlerTest(t)
	insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 801})
	setTestChannelCodexRestriction(t, db, 801, false)
	router := gin.New()
	router.GET("/v1/responses", func(c *gin.Context) { common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, "801") }, middleware.ResponsesWebSocketPreflight(), func(c *gin.Context) { t.Error("restricted request reached upgrade") })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/responses", nil))
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Equal(t, "codex_client", gjson.GetBytes(recorder.Body.Bytes(), "error.code").String())
}

func TestCodexForbiddenOverridesSessionAffinity(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		t.Run(transport, func(t *testing.T) {
			db := setupResponsesWebSocketHandlerTest(t)
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 801})
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 802})
			setTestChannelCodexRestriction(t, db, 801, true)
			affinity := operation_setting.GetChannelAffinitySetting()
			previous := *affinity
			t.Cleanup(func() { *affinity = previous })
			*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, SessionMode: "prefer", Rules: []operation_setting.ChannelAffinityRule{{Name: t.Name(), ModelRegex: []string{".*"}, IncludeRuleName: true, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}}}}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("X-Session", t.Name())
			c.Request.Header.Set("User-Agent", "codex_cli_rs/0.141.0")
			c.Request.Header.Set("X-Codex-Window-Id", "test")
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			service.GetPreferredChannelByAffinity(c, responsesWebSocketTestModel, "default")
			service.RecordChannelAffinity(c, 801)
			t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(c) })
			preferred, found := service.GetPreferredChannelByAffinity(c, responsesWebSocketTestModel, "default")
			require.True(t, found)
			require.Equal(t, 801, preferred)
			if transport == "http" {
				selected, _, err := service.SelectChannelForRequest(c, responsesWebSocketTestModel, &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: responsesWebSocketTestModel})
				require.Nil(t, err)
				require.Equal(t, 802, selected.Id)
			} else {
				selected, err := selectResponsesWebSocketChannel(c, &relaycommon.RelayInfo{OriginModelName: responsesWebSocketTestModel}, nil)
				require.Nil(t, err)
				require.Equal(t, 802, selected.Id)
			}
		})
	}
}

func TestCodexOnlyOverridesSessionAffinity(t *testing.T) {
	for _, transport := range []string{"http", "websocket"} {
		t.Run(transport, func(t *testing.T) {
			db := setupResponsesWebSocketHandlerTest(t)
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 801})
			insertResponsesWebSocketTestChannel(t, db, responsesWebSocketTestChannel{id: 802})
			setTestChannelCodexRestriction(t, db, 801, false)
			affinity := operation_setting.GetChannelAffinitySetting()
			previous := *affinity
			t.Cleanup(func() { *affinity = previous })
			*affinity = operation_setting.ChannelAffinitySetting{Enabled: true, SessionMode: "prefer", Rules: []operation_setting.ChannelAffinityRule{{Name: t.Name(), ModelRegex: []string{".*"}, IncludeRuleName: true, KeySources: []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}}}}}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("X-Session", t.Name())
			common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
			service.GetPreferredChannelByAffinity(c, responsesWebSocketTestModel, "default")
			service.RecordChannelAffinity(c, 801)
			t.Cleanup(func() { service.ClearCurrentChannelAffinityCache(c) })
			preferred, found := service.GetPreferredChannelByAffinity(c, responsesWebSocketTestModel, "default")
			require.True(t, found)
			require.Equal(t, 801, preferred)
			if transport == "http" {
				selected, _, err := service.SelectChannelForRequest(c, responsesWebSocketTestModel, &service.RetryParam{Ctx: c, TokenGroup: "default", ModelName: responsesWebSocketTestModel})
				require.Nil(t, err)
				require.Equal(t, 802, selected.Id)
			} else {
				selected, err := selectResponsesWebSocketChannel(c, &relaycommon.RelayInfo{OriginModelName: responsesWebSocketTestModel}, nil)
				require.Nil(t, err)
				require.Equal(t, 802, selected.Id)
			}
		})
	}
}

package qualityinspect

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupQualityAttributionTest(t *testing.T) (*gorm.DB, *model.Channel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	oldDB, oldLogDB := model.DB, model.LOG_DB
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = oldDB, oldLogDB
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, db.AutoMigrate(&model.ModelQualitySettings{}, &model.Token{}, &model.User{}, &model.Channel{}, &model.Log{}))
	settings := model.DefaultQualitySettings()
	settings.Enabled, settings.TokenIDs = true, []int{1}
	settingsJSON, err := common.Marshal(settings)
	require.NoError(t, err)
	require.NoError(t, db.Create(&model.ModelQualitySettings{ID: 1, Version: 1, Config: string(settingsJSON)}).Error)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "quality-test", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default"}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 1, UserId: 1, Key: "quality-test-key", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100}).Error)
	channel := &model.Channel{Id: 7, Name: "actual-route", Key: "upstream-test-key"}
	require.NoError(t, db.Create(channel).Error)
	return db, channel
}

func TestRunSampleAttributesChannelBeforeConsumeLog(t *testing.T) {
	db, channel := setupQualityAttributionTest(t)
	for _, protocol := range []string{"responses", "chat"} {
		t.Run(protocol, func(t *testing.T) {
			logWritten := make(chan struct{})
			releaseLog := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(logWritten)
				c, _ := gin.CreateTestContext(w)
				c.Request = r
				c.Set("id", 1)
				c.Set("token_id", 1)
				if err := middleware.SetupContextForSelectedChannel(c, channel, "gpt-6-astra"); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set(common.RequestIdKey, "quality-attribution-"+protocol)
				w.Header().Set("Content-Type", "text/event-stream")
				if protocol == "responses" {
					_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n"))
				} else {
					_, _ = w.Write([]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
				}
				w.(http.Flusher).Flush()
				<-releaseLog // Relay settlement writes the consume log after the terminal event.
				assert.NoError(t, db.Create(&model.Log{RequestId: "quality-attribution-" + protocol, TokenId: 1, Type: model.LogTypeConsume, ChannelId: channel.Id}).Error)
			}))
			defer server.Close()
			defer func() { close(releaseLog); <-logWritten }()
			_, port, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			t.Setenv("PORT", port)
			cfg := configFixture()
			cfg.Mode, cfg.ChannelIDs, cfg.OutputType, cfg.Protocol = "route", nil, "text", protocol
			result := RunSample(context.Background(), cfg, 0)
			require.Equal(t, "succeeded", result.Status)
			assert.Equal(t, channel.Id, result.ChannelID, "successful routed samples must not remain unattributed while billing is pending")
			assert.Equal(t, channel.Name, result.ChannelName)
			select {
			case <-logWritten:
				t.Fatal("test must exercise attribution before the log is written")
			default:
			}
		})
	}
}

func TestQualityChannelHeaderOnlyForAuthorizedLocalInspection(t *testing.T) {
	db, channel := setupQualityAttributionTest(t)
	require.NoError(t, db.Create(&model.User{Id: 2, Username: "ordinary-test", AffCode: "ordinary-test", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}).Error)
	for _, tc := range []struct {
		name, remote, marker string
		userID, tokenID      int
		want                 string
	}{
		{"local inspection", "127.0.0.1:1234", "1", 1, 1, "7"},
		{"IPv6 local inspection", "[::1]:1234", "1", 1, 1, "7"},
		{"ordinary request", "127.0.0.1:1234", "", 1, 1, ""},
		{"invalid marker", "127.0.0.1:1234", "true", 1, 1, ""},
		{"nonlocal inspection", "192.0.2.1:1234", "1", 1, 1, ""},
		{"missing permission", "127.0.0.1:1234", "1", 2, 1, ""},
		{"missing token", "127.0.0.1:1234", "1", 1, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.RemoteAddr = tc.remote
			c.Request.Header.Set(common.QualityInspectionHeader, tc.marker)
			c.Request.Header.Set("X-Forwarded-For", "127.0.0.1")
			c.Set("id", tc.userID)
			c.Set("token_id", tc.tokenID)
			require.Nil(t, middleware.SetupContextForSelectedChannel(c, channel, "gpt-6-astra"))
			assert.Equal(t, tc.want, w.Header().Get(common.QualityInspectionChannelHeader))
			if tc.want != "" {
				retried := *channel
				retried.Id = 8
				require.Nil(t, middleware.SetupContextForSelectedChannel(c, &retried, "gpt-6-astra"))
				assert.Equal(t, "8", w.Header().Get(common.QualityInspectionChannelHeader), "gateway retry must replace the first channel")
			}
		})
	}
}

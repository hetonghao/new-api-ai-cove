package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestUpstreamCannotOverrideQualityChannel(t *testing.T) {
	for _, localID := range []string{"", "7"} {
		t.Run(localID, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			if localID != "" {
				c.Header(common.QualityInspectionChannelHeader, localID)
			}
			upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{}}
			upstream.Header.Set(common.QualityInspectionChannelHeader, "99")
			IOCopyBytesGracefully(c, upstream, []byte("ok"))
			assert.Equal(t, localID, w.Header().Get(common.QualityInspectionChannelHeader))
		})
	}
}

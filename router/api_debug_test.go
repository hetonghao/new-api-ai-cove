package router

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/textproto"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProcessingProbeSends102BeforeFinalJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	SetApiRouter(engine)
	server := httptest.NewServer(engine)
	defer server.Close()

	var sawProcessing bool
	trace := &httptrace.ClientTrace{
		Got1xxResponse: func(code int, _ textproto.MIMEHeader) error {
			if code == http.StatusProcessing {
				sawProcessing = true
			}
			return nil
		},
	}

	request, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), trace),
		http.MethodGet,
		server.URL+"/api/debug/processing?delay_ms=1",
		nil,
	)
	if err != nil {
		t.Fatalf("NewRequestWithContext returned error: %v", err)
	}

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}

	if !sawProcessing {
		t.Fatal("did not observe 102 Processing interim response")
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d; body=%s", response.StatusCode, http.StatusOK, body)
	}
	if !strings.Contains(string(body), `"sent_interim_status":102`) {
		t.Fatalf("body = %s, want sent_interim_status 102", body)
	}
}

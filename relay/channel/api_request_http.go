package channel

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func DoApiRequest(a Adaptor, c *gin.Context, info *common.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	fullRequestURL, err := a.GetRequestURL(info)
	if err != nil {
		return nil, fmt.Errorf("get request url failed: %w", err)
	}
	logger.LogDebug(c, "fullRequestURL: %s", common.SanitizeURLForLog(fullRequestURL))
	if common.IsOfficialOpenAIURL(fullRequestURL, "/v1/chat/completions") {
		payload, err := io.ReadAll(requestBody)
		if err != nil {
			return nil, fmt.Errorf("read OpenAI Chat request body: %w", err)
		}
		payload, err = common.NormalizeOfficialOpenAIChatRequest(fullRequestURL, info, payload)
		if err != nil {
			return nil, err
		}
		body, closer, err := common.NewOutboundJSONBody(payload)
		if err != nil {
			return nil, fmt.Errorf("prepare OpenAI Chat request body: %w", err)
		}
		defer closer.Close()
		requestBody = body
	}
	req, err := http.NewRequest(c.Request.Method, fullRequestURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("new request failed: %w", err)
	}
	ApplyUpstreamBodyMetadata(req, requestBody)
	headers := req.Header
	err = a.SetupRequestHeader(c, &headers, info)
	if err != nil {
		return nil, fmt.Errorf("setup request header failed: %w", err)
	}
	// Header overrides retain their priority over adaptor defaults.
	if err = ApplyHeaderOverride(info, c, req); err != nil {
		return nil, err
	}
	if err = signUpstreamRequest(a, c, info, req.Method, req.URL, req.Header); err != nil {
		return nil, err
	}
	resp, err := doRequest(c, req, info)
	if err != nil {
		return nil, fmt.Errorf("do request failed: %w", err)
	}
	return resp, nil
}

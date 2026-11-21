package service

import (
	"errors"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

// PublicRelayError keeps provider details on the internal error for retry and diagnosis.
func PublicRelayError(c *gin.Context, err *types.NewAPIError) *types.NewAPIError {
	if !err.IsUpstreamHTTP429() {
		return err
	}
	return types.NewErrorWithStatusCode(errors.New(i18n.T(c, i18n.MsgRelayServiceBusy)), types.ErrorCodeBadResponseStatusCode, err.StatusCode)
}

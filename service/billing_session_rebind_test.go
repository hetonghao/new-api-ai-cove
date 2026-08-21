package service

import (
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type rebindCountingFunding struct {
	settles    atomic.Int32
	refunds    atomic.Int32
	refundDone chan struct{}
}

func (*rebindCountingFunding) Source() string       { return BillingSourceWallet }
func (*rebindCountingFunding) PreConsume(int) error { return nil }
func (f *rebindCountingFunding) Settle(int) error   { f.settles.Add(1); return nil }
func (f *rebindCountingFunding) Refund() error {
	f.refunds.Add(1)
	if f.refundDone != nil {
		select {
		case f.refundDone <- struct{}{}:
		default:
		}
	}
	return nil
}

func TestBillingSessionRebindKeepsOneTerminalOperation(t *testing.T) {
	oldInfo := &relaycommon.RelayInfo{IsPlayground: true}
	newInfo := &relaycommon.RelayInfo{IsPlayground: true}
	funding := &rebindCountingFunding{}
	session := &BillingSession{
		relayInfo:        oldInfo,
		funding:          funding,
		preConsumedQuota: 10,
	}
	oldInfo.Billing = session

	session.RebindRelayInfo(newInfo)
	session.RebindRelayInfo(newInfo)
	require.Same(t, session, newInfo.Billing)
	require.NoError(t, session.Settle(15))
	require.NoError(t, session.Settle(20))
	require.Equal(t, int32(1), funding.settles.Load())
}

func TestBillingSessionRebindRefundsOnce(t *testing.T) {
	oldInfo := &relaycommon.RelayInfo{IsPlayground: true}
	newInfo := &relaycommon.RelayInfo{IsPlayground: true}
	funding := &rebindCountingFunding{refundDone: make(chan struct{}, 1)}
	session := &BillingSession{
		relayInfo:     oldInfo,
		funding:       funding,
		tokenConsumed: 10,
	}
	oldInfo.Billing = session
	session.RebindRelayInfo(newInfo)
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session.Refund(ctx)
	session.Refund(ctx)
	select {
	case <-funding.refundDone:
	case <-time.After(time.Second):
		require.FailNow(t, "timeout waiting for asynchronous funding refund")
	}
	require.Equal(t, int32(1), funding.refunds.Load())
}

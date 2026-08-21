package service

import (
	"net/http/httptest"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type rebindCountingFunding struct {
	settles atomic.Int32
	refunds atomic.Int32
}

func (*rebindCountingFunding) Source() string       { return BillingSourceWallet }
func (*rebindCountingFunding) PreConsume(int) error { return nil }
func (f *rebindCountingFunding) Settle(int) error   { f.settles.Add(1); return nil }
func (f *rebindCountingFunding) Refund() error      { f.refunds.Add(1); return nil }

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
	funding := &rebindCountingFunding{}
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
	deadline := time.Now().Add(time.Second)
	for funding.refunds.Load() == 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	for gopool.WorkerCount() > 0 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	require.Equal(t, int32(1), funding.refunds.Load())
}

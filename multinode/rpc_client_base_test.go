package multinode

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	common "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	frameworkmetrics "github.com/smartcontractkit/chainlink-framework/metrics"
	"github.com/smartcontractkit/chainlink-framework/multinode/config"
)

type testRPC struct {
	*RPCClientBase[*testHead]
	latestBlockNum int64
}

// latestBlock simulates a chain-specific latestBlock function
func (rpc *testRPC) latestBlock(ctx context.Context) (*testHead, error) {
	rpc.latestBlockNum++
	return &testHead{rpc.latestBlockNum}, nil
}

func (rpc *testRPC) Close() {
	rpc.RPCClientBase.Close()
}

type testHead struct {
	blockNumber int64
}

func (t *testHead) BlockNumber() int64           { return t.blockNumber }
func (t *testHead) BlockDifficulty() *big.Int    { return nil }
func (t *testHead) GetTotalDifficulty() *big.Int { return nil }
func (t *testHead) IsValid() bool                { return t != nil && t.blockNumber > 0 }

func ptr[T any](t T) *T {
	return &t
}

func newTestRPC(t *testing.T) *testRPC {
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := &config.MultiNodeConfig{
		MultiNode: config.MultiNode{
			Enabled:                      ptr(true),
			PollFailureThreshold:         ptr(uint32(5)),
			PollInterval:                 common.MustNewDuration(15 * time.Second),
			SelectionMode:                ptr(NodeSelectionModePriorityLevel),
			SyncThreshold:                ptr(uint32(10)),
			LeaseDuration:                common.MustNewDuration(time.Minute),
			NodeIsSyncingEnabled:         ptr(false),
			NewHeadsPollInterval:         common.MustNewDuration(5 * time.Second),
			FinalizedBlockPollInterval:   common.MustNewDuration(5 * time.Second),
			EnforceRepeatableRead:        ptr(true),
			DeathDeclarationDelay:        common.MustNewDuration(20 * time.Second),
			NodeNoNewHeadsThreshold:      common.MustNewDuration(20 * time.Second),
			NoNewFinalizedHeadsThreshold: common.MustNewDuration(20 * time.Second),
			FinalityTagEnabled:           ptr(true),
			FinalityDepth:                ptr(uint32(0)),
			FinalizedBlockOffset:         ptr(uint32(50)),
		},
	}

	rpc := &testRPC{}
	rpc.RPCClientBase = NewRPCClientBase[*testHead](cfg, requestTimeout, lggr, rpc.latestBlock, rpc.latestBlock, nil)
	t.Cleanup(rpc.Close)
	return rpc
}

type recordedRPCRequest struct {
	callName   string
	latency    time.Duration
	err        error
}

type spyRPCClientMetrics struct {
	requests []recordedRPCRequest
}

var _ frameworkmetrics.RPCClientMetrics = (*spyRPCClientMetrics)(nil)

func (s *spyRPCClientMetrics) RecordRequest(_ context.Context, callName string, latency time.Duration, err error) {
	s.requests = append(s.requests, recordedRPCRequest{
		callName:   callName,
		latency:    latency,
		err:        err,
	})
}

func TestAdapter_LatestBlock(t *testing.T) {
	t.Run("LatestBlock", func(t *testing.T) {
		rpc := newTestRPC(t)
		latestChainInfo, highestChainInfo := rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.BlockNumber)
		require.Equal(t, int64(0), highestChainInfo.BlockNumber)
		head, err := rpc.LatestBlock(t.Context())
		require.NoError(t, err)
		require.True(t, head.IsValid())
		latestChainInfo, highestChainInfo = rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(1), latestChainInfo.BlockNumber)
		require.Equal(t, int64(1), highestChainInfo.BlockNumber)
	})

	t.Run("LatestFinalizedBlock", func(t *testing.T) {
		rpc := newTestRPC(t)
		latestChainInfo, highestChainInfo := rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(0), highestChainInfo.FinalizedBlockNumber)
		finalizedHead, err := rpc.LatestFinalizedBlock(t.Context())
		require.NoError(t, err)
		require.True(t, finalizedHead.IsValid())
		latestChainInfo, highestChainInfo = rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(1), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(1), highestChainInfo.FinalizedBlockNumber)
	})
}

func TestRPCClientBase_RecordsRPCMetrics(t *testing.T) {
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	cfg := &config.MultiNodeConfig{
		MultiNode: config.MultiNode{
			Enabled:                      ptr(true),
			PollFailureThreshold:         ptr(uint32(5)),
			PollInterval:                 common.MustNewDuration(15 * time.Second),
			SelectionMode:                ptr(NodeSelectionModePriorityLevel),
			SyncThreshold:                ptr(uint32(10)),
			LeaseDuration:                common.MustNewDuration(time.Minute),
			NodeIsSyncingEnabled:         ptr(false),
			NewHeadsPollInterval:         common.MustNewDuration(5 * time.Second),
			FinalizedBlockPollInterval:   common.MustNewDuration(5 * time.Second),
			EnforceRepeatableRead:        ptr(true),
			DeathDeclarationDelay:        common.MustNewDuration(20 * time.Second),
			NodeNoNewHeadsThreshold:      common.MustNewDuration(20 * time.Second),
			NoNewFinalizedHeadsThreshold: common.MustNewDuration(20 * time.Second),
			FinalityTagEnabled:           ptr(true),
			FinalityDepth:                ptr(uint32(0)),
			FinalizedBlockOffset:         ptr(uint32(50)),
		},
	}

	t.Run("records successful latest block requests", func(t *testing.T) {
		spy := &spyRPCClientMetrics{}
		rpc := NewRPCClientBase[*testHead](
			cfg,
			requestTimeout,
			lggr,
			func(context.Context) (*testHead, error) {
				return &testHead{blockNumber: 7}, nil
			},
			func(context.Context) (*testHead, error) {
				return &testHead{blockNumber: 8}, nil
			},
			spy,
		)

		head, err := rpc.LatestBlock(t.Context())
		require.NoError(t, err)
		require.Equal(t, int64(7), head.BlockNumber())
		require.Len(t, spy.requests, 1)
		require.Equal(t, rpcCallNameLatestBlock, spy.requests[0].callName)
		require.NoError(t, spy.requests[0].err)
		require.Positive(t, spy.requests[0].latency)
	})

	t.Run("records failed finalized block requests", func(t *testing.T) {
		spy := &spyRPCClientMetrics{}
		expectedErr := errors.New("boom")
		rpc := NewRPCClientBase[*testHead](
			cfg,
			requestTimeout,
			lggr,
			func(context.Context) (*testHead, error) {
				return &testHead{blockNumber: 7}, nil
			},
			func(context.Context) (*testHead, error) {
				return nil, expectedErr
			},
			spy,
		)

		_, err := rpc.LatestFinalizedBlock(t.Context())
		require.ErrorIs(t, err, expectedErr)
		require.Len(t, spy.requests, 1)
		require.Equal(t, rpcCallNameLatestFinalizedBlock, spy.requests[0].callName)
		require.ErrorIs(t, spy.requests[0].err, expectedErr)
		require.Positive(t, spy.requests[0].latency)
	})

	t.Run("records invalid heads as failed requests", func(t *testing.T) {
		spy := &spyRPCClientMetrics{}
		rpc := NewRPCClientBase[*testHead](
			cfg,
			requestTimeout,
			lggr,
			func(context.Context) (*testHead, error) {
				return &testHead{}, nil
			},
			func(context.Context) (*testHead, error) {
				return &testHead{blockNumber: 8}, nil
			},
			spy,
		)

		_, err := rpc.LatestBlock(t.Context())
		require.ErrorIs(t, err, errInvalidHead)
		require.Len(t, spy.requests, 1)
		require.Equal(t, rpcCallNameLatestBlock, spy.requests[0].callName)
		require.ErrorIs(t, spy.requests[0].err, errInvalidHead)
	})
}

func TestAdapter_OnNewHeadFunctions(t *testing.T) {
	timeout := 10 * time.Second
	t.Run("OnNewHead and OnNewFinalizedHead updates chain info", func(t *testing.T) {
		rpc := newTestRPC(t)
		latestChainInfo, highestChainInfo := rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.BlockNumber)
		require.Equal(t, int64(0), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(0), highestChainInfo.BlockNumber)
		require.Equal(t, int64(0), highestChainInfo.FinalizedBlockNumber)

		ctx, cancel, lifeCycleCh := rpc.AcquireQueryCtx(t.Context(), timeout)
		defer cancel()
		rpc.OnNewHead(ctx, lifeCycleCh, &testHead{blockNumber: 10})
		rpc.OnNewFinalizedHead(ctx, lifeCycleCh, &testHead{blockNumber: 3})
		rpc.OnNewHead(ctx, lifeCycleCh, &testHead{blockNumber: 5})
		rpc.OnNewFinalizedHead(ctx, lifeCycleCh, &testHead{blockNumber: 1})

		latestChainInfo, highestChainInfo = rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(5), latestChainInfo.BlockNumber)
		require.Equal(t, int64(1), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(10), highestChainInfo.BlockNumber)
		require.Equal(t, int64(3), highestChainInfo.FinalizedBlockNumber)
	})

	t.Run("OnNewHead respects HealthCheckCtx", func(t *testing.T) {
		rpc := newTestRPC(t)
		latestChainInfo, highestChainInfo := rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.BlockNumber)
		require.Equal(t, int64(0), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(0), highestChainInfo.BlockNumber)
		require.Equal(t, int64(0), highestChainInfo.FinalizedBlockNumber)

		healthCheckCtx := CtxAddHealthCheckFlag(t.Context())

		ctx, cancel, lifeCycleCh := rpc.AcquireQueryCtx(healthCheckCtx, timeout)
		defer cancel()
		rpc.OnNewHead(ctx, lifeCycleCh, &testHead{blockNumber: 10})
		rpc.OnNewFinalizedHead(ctx, lifeCycleCh, &testHead{blockNumber: 3})
		rpc.OnNewHead(ctx, lifeCycleCh, &testHead{blockNumber: 5})
		rpc.OnNewFinalizedHead(ctx, lifeCycleCh, &testHead{blockNumber: 1})

		latestChainInfo, highestChainInfo = rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(5), latestChainInfo.BlockNumber)
		require.Equal(t, int64(1), latestChainInfo.FinalizedBlockNumber)

		// Highest chain info should not be set on health check requests
		require.Equal(t, int64(0), highestChainInfo.BlockNumber)
		require.Equal(t, int64(0), highestChainInfo.FinalizedBlockNumber)
	})

	t.Run("OnNewHead and OnNewFinalizedHead respects closure of requestCh", func(t *testing.T) {
		rpc := newTestRPC(t)
		latestChainInfo, highestChainInfo := rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.BlockNumber)
		require.Equal(t, int64(0), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(0), highestChainInfo.BlockNumber)
		require.Equal(t, int64(0), highestChainInfo.FinalizedBlockNumber)

		ctx, cancel, lifeCycleCh := rpc.AcquireQueryCtx(t.Context(), timeout)
		defer cancel()
		rpc.CancelLifeCycle()

		rpc.OnNewHead(ctx, lifeCycleCh, &testHead{blockNumber: 10})
		rpc.OnNewFinalizedHead(ctx, lifeCycleCh, &testHead{blockNumber: 3})
		rpc.OnNewHead(ctx, lifeCycleCh, &testHead{blockNumber: 5})
		rpc.OnNewFinalizedHead(ctx, lifeCycleCh, &testHead{blockNumber: 1})

		// Latest chain info should not be set if life cycle is cancelled
		latestChainInfo, highestChainInfo = rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.BlockNumber)
		require.Equal(t, int64(0), latestChainInfo.FinalizedBlockNumber)

		require.Equal(t, int64(10), highestChainInfo.BlockNumber)
		require.Equal(t, int64(3), highestChainInfo.FinalizedBlockNumber)
	})
}

func TestAdapter_HeadSubscriptions(t *testing.T) {
	t.Run("SubscribeToHeads", func(t *testing.T) {
		rpc := newTestRPC(t)
		ch, sub, err := rpc.SubscribeToHeads(t.Context())
		require.NoError(t, err)
		defer sub.Unsubscribe()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		select {
		case head := <-ch:
			latest, _ := rpc.GetInterceptedChainInfo()
			require.Equal(t, head.BlockNumber(), latest.BlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive head: ", ctx.Err())
		}
	})

	t.Run("SubscribeToFinalizedHeads", func(t *testing.T) {
		rpc := newTestRPC(t)
		finalizedCh, finalizedSub, err := rpc.SubscribeToFinalizedHeads(t.Context())
		require.NoError(t, err)
		defer finalizedSub.Unsubscribe()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		select {
		case finalizedHead := <-finalizedCh:
			latest, _ := rpc.GetInterceptedChainInfo()
			require.Equal(t, finalizedHead.BlockNumber(), latest.FinalizedBlockNumber)
		case <-ctx.Done():
			t.Fatal("failed to receive finalized head: ", ctx.Err())
		}
	})

	t.Run("Remove Subscription on Unsubscribe", func(t *testing.T) {
		rpc := newTestRPC(t)
		_, sub1, err := rpc.SubscribeToHeads(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, rpc.lenSubs())
		_, sub2, err := rpc.SubscribeToFinalizedHeads(t.Context())
		require.NoError(t, err)
		require.Equal(t, 2, rpc.lenSubs())

		sub1.Unsubscribe()
		require.Equal(t, 1, rpc.lenSubs())
		sub2.Unsubscribe()
		require.Equal(t, 0, rpc.lenSubs())
	})

	t.Run("Ensure no deadlock on UnsubscribeAll", func(t *testing.T) {
		rpc := newTestRPC(t)
		_, _, err := rpc.SubscribeToHeads(t.Context())
		require.NoError(t, err)
		require.Equal(t, 1, rpc.lenSubs())
		_, _, err = rpc.SubscribeToFinalizedHeads(t.Context())
		require.NoError(t, err)
		require.Equal(t, 2, rpc.lenSubs())
		rpc.UnsubscribeAllExcept()
		require.Equal(t, 0, rpc.lenSubs())
	})
}

type mockSub struct {
	unsubscribed bool
}

func newMockSub() *mockSub {
	return &mockSub{unsubscribed: false}
}

func (s *mockSub) Unsubscribe() {
	s.unsubscribed = true
}
func (s *mockSub) Err() <-chan error {
	return nil
}

func TestMultiNodeClient_RegisterSubs(t *testing.T) {
	t.Run("RegisterSub", func(t *testing.T) {
		rpc := newTestRPC(t)
		mockSub := newMockSub()
		sub, err := rpc.RegisterSub(mockSub, make(chan struct{}))
		require.NoError(t, err)
		require.NotNil(t, sub)
		require.Equal(t, 1, rpc.lenSubs())
		rpc.UnsubscribeAllExcept()
	})

	t.Run("lifeCycleCh returns error and unsubscribes", func(t *testing.T) {
		rpc := newTestRPC(t)
		chStopInFlight := make(chan struct{})
		close(chStopInFlight)
		mockSub := newMockSub()
		_, err := rpc.RegisterSub(mockSub, chStopInFlight)
		require.Error(t, err)
		require.True(t, mockSub.unsubscribed)
	})

	t.Run("UnsubscribeAllExcept", func(t *testing.T) {
		rpc := newTestRPC(t)
		chStopInFlight := make(chan struct{})
		mockSub1 := newMockSub()
		mockSub2 := newMockSub()
		sub1, err := rpc.RegisterSub(mockSub1, chStopInFlight)
		require.NoError(t, err)
		_, err = rpc.RegisterSub(mockSub2, chStopInFlight)
		require.NoError(t, err)
		require.Equal(t, 2, rpc.lenSubs())

		// Ensure passed sub is not removed
		rpc.UnsubscribeAllExcept(sub1)
		require.Equal(t, 1, rpc.lenSubs())
		require.True(t, mockSub2.unsubscribed)
		require.False(t, mockSub1.unsubscribed)

		rpc.UnsubscribeAllExcept()
		require.Equal(t, 0, rpc.lenSubs())
		require.True(t, mockSub1.unsubscribed)
	})
}

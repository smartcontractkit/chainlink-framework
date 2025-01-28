package multinode

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	common "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-framework/multinode/config"
)

type testRPC struct {
	*Adapter[*testHead]
	latestBlockNum int64
}

// latestBlock simulates a chain-specific latestBlock function
func (rpc *testRPC) latestBlock(ctx context.Context) (*testHead, error) {
	rpc.latestBlockNum++
	return &testHead{rpc.latestBlockNum}, nil
}

func (rpc *testRPC) Close() {
	rpc.Adapter.Close()
}

type testHead struct {
	blockNumber int64
}

func (t *testHead) BlockNumber() int64           { return t.blockNumber }
func (t *testHead) BlockDifficulty() *big.Int    { return nil }
func (t *testHead) GetTotalDifficulty() *big.Int { return nil }
func (t *testHead) IsValid() bool                { return true }

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
	rpc.Adapter = NewAdapter[*testHead](cfg, requestTimeout, lggr, rpc.latestBlock, rpc.latestBlock)
	t.Cleanup(rpc.Close)
	return rpc
}

func TestAdapter_LatestBlock(t *testing.T) {
	t.Run("LatestBlock", func(t *testing.T) {
		rpc := newTestRPC(t)
		latestChainInfo, highestChainInfo := rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(0), latestChainInfo.BlockNumber)
		require.Equal(t, int64(0), highestChainInfo.BlockNumber)
		head, err := rpc.LatestBlock(tests.Context(t))
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
		finalizedHead, err := rpc.LatestFinalizedBlock(tests.Context(t))
		require.NoError(t, err)
		require.True(t, finalizedHead.IsValid())
		latestChainInfo, highestChainInfo = rpc.GetInterceptedChainInfo()
		require.Equal(t, int64(1), latestChainInfo.FinalizedBlockNumber)
		require.Equal(t, int64(1), highestChainInfo.FinalizedBlockNumber)
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

		ctx, cancel, lifeCycleCh := rpc.AcquireQueryCtx(tests.Context(t), timeout)
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

		healthCheckCtx := CtxAddHealthCheckFlag(tests.Context(t))

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

		ctx, cancel, lifeCycleCh := rpc.AcquireQueryCtx(tests.Context(t), timeout)
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
		ch, sub, err := rpc.SubscribeToHeads(tests.Context(t))
		require.NoError(t, err)
		defer sub.Unsubscribe()

		ctx, cancel := context.WithTimeout(tests.Context(t), time.Minute)
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
		finalizedCh, finalizedSub, err := rpc.SubscribeToFinalizedHeads(tests.Context(t))
		require.NoError(t, err)
		defer finalizedSub.Unsubscribe()

		ctx, cancel := context.WithTimeout(tests.Context(t), time.Minute)
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
		_, sub1, err := rpc.SubscribeToHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 1, rpc.LenSubs())
		_, sub2, err := rpc.SubscribeToFinalizedHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 2, rpc.LenSubs())

		sub1.Unsubscribe()
		require.Equal(t, 1, rpc.LenSubs())
		sub2.Unsubscribe()
		require.Equal(t, 0, rpc.LenSubs())
	})

	t.Run("Ensure no deadlock on UnsubscribeAll", func(t *testing.T) {
		rpc := newTestRPC(t)
		_, _, err := rpc.SubscribeToHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 1, rpc.LenSubs())
		_, _, err = rpc.SubscribeToFinalizedHeads(tests.Context(t))
		require.NoError(t, err)
		require.Equal(t, 2, rpc.LenSubs())
		rpc.UnsubscribeAllExcept()
		require.Equal(t, 0, rpc.LenSubs())
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
		require.Equal(t, 1, rpc.LenSubs())
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
		require.Equal(t, 2, rpc.LenSubs())

		// Ensure passed sub is not removed
		rpc.UnsubscribeAllExcept(sub1)
		require.Equal(t, 1, rpc.LenSubs())
		require.True(t, mockSub2.unsubscribed)
		require.False(t, mockSub1.unsubscribed)

		rpc.UnsubscribeAllExcept()
		require.Equal(t, 0, rpc.LenSubs())
		require.True(t, mockSub1.unsubscribed)
	})
}

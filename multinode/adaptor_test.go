package multinode

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type testRPC struct {
	latestBlock int64
}

type testHead struct {
	blockNumber int64
}

func (t *testHead) BlockNumber() int64        { return t.blockNumber }
func (t *testHead) BlockDifficulty() *big.Int { return nil }
func (t *testHead) IsValid() bool             { return true }

func LatestBlock(ctx context.Context, rpc *testRPC) (*testHead, error) {
	rpc.latestBlock++
	return &testHead{rpc.latestBlock}, nil
}

func newTestClient(t *testing.T) *Adapter[testRPC, *testHead] {
	requestTimeout := 5 * time.Second
	lggr := logger.Test(t)
	c := NewAdapter[testRPC, *testHead](&testRPC{}, requestTimeout, lggr, LatestBlock, LatestBlock)
	t.Cleanup(c.Close)
	return c
}

func TestMultiNodeClient_LatestBlock(t *testing.T) {
	t.Run("LatestBlock", func(t *testing.T) {
		c := newTestClient(t)
		head, err := c.LatestBlock(tests.Context(t))
		require.NoError(t, err)
		require.True(t, head.IsValid())
	})

	t.Run("LatestFinalizedBlock", func(t *testing.T) {
		c := newTestClient(t)
		finalizedHead, err := c.LatestFinalizedBlock(tests.Context(t))
		require.NoError(t, err)
		require.True(t, finalizedHead.IsValid())
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
		c := newTestClient(t)
		mockSub := newMockSub()
		sub, err := c.RegisterSub(mockSub, make(chan struct{}))
		require.NoError(t, err)
		require.NotNil(t, sub)
		require.Equal(t, 1, c.LenSubs())
		c.UnsubscribeAllExcept()
	})

	t.Run("chStopInFlight returns error and unsubscribes", func(t *testing.T) {
		c := newTestClient(t)
		chStopInFlight := make(chan struct{})
		close(chStopInFlight)
		mockSub := newMockSub()
		_, err := c.RegisterSub(mockSub, chStopInFlight)
		require.Error(t, err)
		require.True(t, mockSub.unsubscribed)
	})

	t.Run("UnsubscribeAllExcept", func(t *testing.T) {
		c := newTestClient(t)
		chStopInFlight := make(chan struct{})
		mockSub1 := newMockSub()
		mockSub2 := newMockSub()
		sub1, err := c.RegisterSub(mockSub1, chStopInFlight)
		require.NoError(t, err)
		_, err = c.RegisterSub(mockSub2, chStopInFlight)
		require.NoError(t, err)
		require.Equal(t, 2, c.LenSubs())

		// Ensure passed sub is not removed
		c.UnsubscribeAllExcept(sub1)
		require.Equal(t, 1, c.LenSubs())
		require.True(t, mockSub2.unsubscribed)
		require.False(t, mockSub1.unsubscribed)

		c.UnsubscribeAllExcept()
		require.Equal(t, 0, c.LenSubs())
		require.True(t, mockSub1.unsubscribed)
	})
}

package multinode

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

func Test_Poller(t *testing.T) {
	lggr := logger.Test(t)

	t.Run("Test multiple start", func(t *testing.T) {
		ctx := tests.Context(t)
		pollFunc := func(ctx context.Context) (Head, error) {
			return nil, nil
		}

		poller, _ := NewPoller[Head](time.Millisecond, pollFunc, time.Second, lggr)
		err := poller.Start(ctx)
		require.NoError(t, err)

		err = poller.Start(ctx)
		require.Error(t, err)
		poller.Unsubscribe()
	})

	t.Run("Test polling for heads", func(t *testing.T) {
		ctx := tests.Context(t)
		// Mock polling function that returns a new value every time it's called
		var pollNumber int
		pollLock := sync.Mutex{}
		pollFunc := func(ctx context.Context) (Head, error) {
			pollLock.Lock()
			defer pollLock.Unlock()
			pollNumber++
			h := head{
				BlockNumber:     int64(pollNumber),
				BlockDifficulty: big.NewInt(int64(pollNumber)),
			}
			return h.ToMockHead(t), nil
		}

		// Create poller and start to receive data
		poller, channel := NewPoller[Head](time.Millisecond, pollFunc, time.Second, lggr)
		require.NoError(t, poller.Start(ctx))
		defer poller.Unsubscribe()

		// Receive updates from the poller
		pollCount := 0
		pollMax := 50
		for ; pollCount < pollMax; pollCount++ {
			h := <-channel
			assert.Equal(t, int64(pollCount+1), h.BlockNumber())
		}
	})

	t.Run("Test polling errors", func(t *testing.T) {
		ctx := tests.Context(t)
		// Mock polling function that returns an error
		var pollNumber int
		pollLock := sync.Mutex{}
		pollFunc := func(ctx context.Context) (Head, error) {
			pollLock.Lock()
			defer pollLock.Unlock()
			pollNumber++
			return nil, fmt.Errorf("polling error %d", pollNumber)
		}

		olggr, observedLogs := logger.TestObserved(t, zap.WarnLevel)

		// Create poller and subscribe to receive data
		poller, _ := NewPoller[Head](time.Millisecond, pollFunc, time.Second, olggr)
		require.NoError(t, poller.Start(ctx))
		defer poller.Unsubscribe()

		// Ensure that all errors were logged as expected
		logsSeen := func() bool {
			for pollCount := 0; pollCount < 50; pollCount++ {
				numLogs := observedLogs.FilterMessage(fmt.Sprintf("polling error: polling error %d", pollCount+1)).Len()
				if numLogs != 1 {
					return false
				}
			}
			return true
		}
		require.Eventually(t, logsSeen, tests.WaitTimeout(t), 100*time.Millisecond)
	})

	t.Run("Test polling timeout", func(t *testing.T) {
		ctx := tests.Context(t)
		pollFunc := func(ctx context.Context) (Head, error) {
			if <-ctx.Done(); true {
				return nil, ctx.Err()
			}
			return nil, nil
		}

		// Set instant timeout
		pollingTimeout := time.Duration(0)

		olggr, observedLogs := logger.TestObserved(t, zap.WarnLevel)

		// Create poller and subscribe to receive data
		poller, _ := NewPoller[Head](time.Millisecond, pollFunc, pollingTimeout, olggr)
		require.NoError(t, poller.Start(ctx))
		defer poller.Unsubscribe()

		// Ensure that timeout errors were logged as expected
		logsSeen := func() bool {
			return observedLogs.FilterMessage("polling error: context deadline exceeded").Len() >= 1
		}
		require.Eventually(t, logsSeen, tests.WaitTimeout(t), 100*time.Millisecond)
	})

	t.Run("Test unsubscribe during polling", func(t *testing.T) {
		ctx := tests.Context(t)
		wait := make(chan struct{})
		closeOnce := sync.OnceFunc(func() { close(wait) })
		pollFunc := func(ctx context.Context) (Head, error) {
			closeOnce()
			// Block in polling function until context is cancelled
			if <-ctx.Done(); true {
				return nil, ctx.Err()
			}
			return nil, nil
		}

		// Set long timeout
		pollingTimeout := time.Minute

		olggr, observedLogs := logger.TestObserved(t, zap.WarnLevel)

		// Create poller and subscribe to receive data
		poller, _ := NewPoller[Head](time.Millisecond, pollFunc, pollingTimeout, olggr)
		require.NoError(t, poller.Start(ctx))

		// Unsubscribe while blocked in polling function
		<-wait
		poller.Unsubscribe()

		// Ensure error was logged
		logsSeen := func() bool {
			return observedLogs.FilterMessage("polling error: context canceled").Len() >= 1
		}
		require.Eventually(t, logsSeen, tests.WaitTimeout(t), 100*time.Millisecond)
	})
}

func Test_Poller_Unsubscribe(t *testing.T) {
	lggr := logger.Test(t)
	pollFunc := func(ctx context.Context) (Head, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			h := head{
				BlockNumber:     0,
				BlockDifficulty: big.NewInt(0),
			}
			return h.ToMockHead(t), nil
		}
	}

	t.Run("Test multiple unsubscribe", func(t *testing.T) {
		ctx := tests.Context(t)
		poller, channel := NewPoller[Head](time.Millisecond, pollFunc, time.Second, lggr)
		err := poller.Start(ctx)
		require.NoError(t, err)

		<-channel
		poller.Unsubscribe()
		poller.Unsubscribe()
	})

	t.Run("Read channel after unsubscribe", func(t *testing.T) {
		ctx := tests.Context(t)
		poller, channel := NewPoller[Head](time.Millisecond, pollFunc, time.Second, lggr)
		err := poller.Start(ctx)
		require.NoError(t, err)

		poller.Unsubscribe()
		require.Nil(t, <-channel)
	})
}

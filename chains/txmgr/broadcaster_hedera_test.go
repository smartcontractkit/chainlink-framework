package txmgr

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
)

func TestPollSequenceAtAfterBroadcast(t *testing.T) {
	t.Parallel()

	lgr := logger.Sugared(logger.Test(t))
	pollInterval := 5 * time.Millisecond

	t.Run("returns immediately when sequence already advanced", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		start := time.Now()

		got, err := pollSequenceAtAfterBroadcast(
			t.Context(), lgr, 85, pollInterval, 3,
			func(context.Context) (int64, error) {
				calls.Add(1)
				return 86, nil
			},
		)

		require.NoError(t, err)
		assert.Equal(t, int64(86), got)
		assert.Equal(t, int32(1), calls.Load())
		assert.Less(t, time.Since(start), pollInterval)
	})

	t.Run("waits and polls until sequence advances", func(t *testing.T) {
		t.Parallel()

		sequences := []int64{85, 85, 86}
		var calls atomic.Int32
		start := time.Now()

		got, err := pollSequenceAtAfterBroadcast(
			t.Context(), lgr, 85, pollInterval, 3,
			func(context.Context) (int64, error) {
				i := int(calls.Add(1)) - 1
				return sequences[i], nil
			},
		)

		require.NoError(t, err)
		assert.Equal(t, int64(86), got)
		assert.Equal(t, int32(3), calls.Load())
		assert.GreaterOrEqual(t, time.Since(start), 2*pollInterval)
	})

	t.Run("returns last sequence when it never advances", func(t *testing.T) {
		t.Parallel()

		var calls atomic.Int32
		start := time.Now()

		got, err := pollSequenceAtAfterBroadcast(
			t.Context(), lgr, 85, pollInterval, 3,
			func(context.Context) (int64, error) {
				calls.Add(1)
				return 85, nil
			},
		)

		require.NoError(t, err)
		assert.Equal(t, int64(85), got)
		assert.Equal(t, int32(4), calls.Load())
		assert.GreaterOrEqual(t, time.Since(start), 3*pollInterval)
	})

	t.Run("returns context error while waiting", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		_, err := pollSequenceAtAfterBroadcast(
			ctx, lgr, 85, pollInterval, 3,
			func(context.Context) (int64, error) {
				return 85, nil
			},
		)

		require.ErrorIs(t, err, context.Canceled)
	})
}

package metrics

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestLogPollerMetrics(t *testing.T) GenericLogPollerMetrics {
	m, err := NewGenericLogPollerMetrics("1", "test-network")
	require.NoError(t, err)
	return m
}

func TestLogPollerMetrics_RecordQueryDuration(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.RecordQueryDuration(ctx, "PollLogs", Read, 0.005) // 5ms

	// Collect and make sure at least one sample was recorded
	require.Greater(t, testutil.CollectAndCount(PromLpQueryDuration), 0)
}

func TestLogPollerMetrics_RecordQueryDatasetSize(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.RecordQueryDatasetSize(ctx, "PollLogs", Read, 10)

	require.Equal(t,
		float64(10),
		testutil.ToFloat64(PromLpQueryDataSets.WithLabelValues("test-network", "1", "PollLogs", "read")),
	)
}

func TestLogPollerMetrics_IncrementLogsInserted(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.IncrementLogsInserted(ctx, 5)

	require.Equal(t,
		float64(5),
		testutil.ToFloat64(PromLpLogsInserted.WithLabelValues("test-network", "1")),
	)
}

func TestLogPollerMetrics_IncrementBlocksInserted(t *testing.T) {
	m := setupTestLogPollerMetrics(t)
	ctx := context.Background()

	m.IncrementBlocksInserted(ctx, 3)

	require.Equal(t,
		float64(3),
		testutil.ToFloat64(PromLpBlocksInserted.WithLabelValues("test-network", "1")),
	)
}

package metrics

import (
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

	m.RecordQueryDuration(t.Context(), "PollLogs", Read, 0.005)
	require.Positive(t, testutil.CollectAndCount(PromLpQueryDuration))
}

func TestLogPollerMetrics_RecordQueryDatasetSize(t *testing.T) {
	m := setupTestLogPollerMetrics(t)

	m.RecordQueryDatasetSize(t.Context(), "PollLogs", Read, 10)

	require.InEpsilon(t,
		10.0,
		testutil.ToFloat64(PromLpQueryDataSets.WithLabelValues("test-network", "1", "PollLogs", "read")),
		0.001,
	)
}

func TestLogPollerMetrics_IncrementLogsInserted(t *testing.T) {
	m := setupTestLogPollerMetrics(t)

	m.IncrementLogsInserted(t.Context(), 5)

	require.InEpsilon(t,
		5.0,
		testutil.ToFloat64(PromLpLogsInserted.WithLabelValues("test-network", "1")),
		0.001,
	)
}

func TestLogPollerMetrics_IncrementBlocksInserted(t *testing.T) {
	m := setupTestLogPollerMetrics(t)

	m.IncrementBlocksInserted(t.Context(), 3)

	require.InEpsilon(t,
		3.0,
		testutil.ToFloat64(PromLpBlocksInserted.WithLabelValues("test-network", "1")),
		0.001,
	)
}

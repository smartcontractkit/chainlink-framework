package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestMultiNodeMetrics(t *testing.T) GenericMultiNodeMetrics {
	m, err := NewGenericMultiNodeMetrics("test-network", "1")
	require.NoError(t, err)
	return m
}

func TestMultiNodeMetrics_RecordNodeStates(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)

	m.RecordNodeStates(t.Context(), "Alive", 5)

	require.InEpsilon(t,
		5.0,
		testutil.ToFloat64(promMultiNodeRPCNodeStates.WithLabelValues("test-network", "1", "Alive")),
		0.001,
	)
}

func TestMultiNodeMetrics_RecordNodeClientVersion(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)

	m.RecordNodeClientVersion(t.Context(), "test-node-1", "rpc-1.2.3")

	value := testutil.ToFloat64(promNodeClientVersion.WithLabelValues(
		"test-network",
		"1",
		"test-node-1",
		"rpc-1.2.3",
	))

	require.InEpsilon(t,
		1.0,
		value,
		0.001,
	)
}

func TestMultiNodeMetrics_Verifies(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)

	m.IncrementNodeVerifies(t.Context(), "node-1")
	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promPoolRPCNodeVerifies.WithLabelValues("test-network", "1", "node-1")),
		0.001,
	)

	m.IncrementNodeVerifiesFailed(t.Context(), "node-1")
	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promPoolRPCNodeVerifiesFailed.WithLabelValues("test-network", "1", "node-1")),
		0.001,
	)

	m.IncrementNodeVerifiesSuccess(t.Context(), "node-1")
	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promPoolRPCNodeVerifiesSuccess.WithLabelValues("test-network", "1", "node-1")),
		0.001,
	)
}

func TestMultiNodeMetrics_NodeTransitions(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)
	nodeName := "node-1"

	tests := []struct {
		name       string
		increment  func()
		promMetric *prometheus.CounterVec
	}{
		{
			name: "Alive",
			increment: func() {
				m.IncrementNodeTransitionsToAlive(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToAlive,
		},
		{
			name: "InSync",
			increment: func() {
				m.IncrementNodeTransitionsToInSync(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToInSync,
		},
		{
			name: "OutOfSync",
			increment: func() {
				m.IncrementNodeTransitionsToOutOfSync(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToOutOfSync,
		},
		{
			name: "Unreachable",
			increment: func() {
				m.IncrementNodeTransitionsToUnreachable(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToUnreachable,
		},
		{
			name: "InvalidChainID",
			increment: func() {
				m.IncrementNodeTransitionsToInvalidChainID(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToInvalidChainID,
		},
		{
			name: "Unusable",
			increment: func() {
				m.IncrementNodeTransitionsToUnusable(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToUnusable,
		},
		{
			name: "Syncing",
			increment: func() {
				m.IncrementNodeTransitionsToSyncing(t.Context(), nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToSyncing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.increment()
			require.InEpsilon(t,
				1.0,
				testutil.ToFloat64(tt.promMetric.WithLabelValues("test-network", "1", nodeName)),
				0.001,
			)
		})
	}
}

func TestMultiNodeMetrics_LifecycleMetrics(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)
	nodeName := "node-1"

	t.Run("SetHighestSeenBlock", func(t *testing.T) {
		m.SetHighestSeenBlock(t.Context(), nodeName, 123)
		require.InEpsilon(t,
			123.0,
			testutil.ToFloat64(promPoolRPCNodeHighestSeenBlock.WithLabelValues("test-network", "1", nodeName)),
			0.001,
		)
	})

	t.Run("SetHighestFinalizedBlock", func(t *testing.T) {
		m.SetHighestFinalizedBlock(t.Context(), nodeName, 456)
		require.InEpsilon(t,
			456.0,
			testutil.ToFloat64(PromPoolRPCNodeHighestFinalizedBlock.WithLabelValues("test-network", "1", nodeName)),
			0.001,
		)
	})

	t.Run("IncrementSeenBlocks", func(t *testing.T) {
		m.IncrementSeenBlocks(t.Context(), nodeName)
		require.InEpsilon(t,
			1.0,
			testutil.ToFloat64(promPoolRPCNodeNumSeenBlocks.WithLabelValues("test-network", "1", nodeName)),
			0.001,
		)
	})

	t.Run("IncrementPolls", func(t *testing.T) {
		m.IncrementPolls(t.Context(), nodeName)
		require.InEpsilon(t,
			1.0,
			testutil.ToFloat64(promPoolRPCNodePolls.WithLabelValues("test-network", "1", nodeName)),
			0.001,
		)
	})

	t.Run("IncrementPollsFailed", func(t *testing.T) {
		m.IncrementPollsFailed(t.Context(), nodeName)
		require.InEpsilon(t,
			1.0,
			testutil.ToFloat64(promPoolRPCNodePollsFailed.WithLabelValues("test-network", "1", nodeName)),
			0.001,
		)
	})

	t.Run("IncrementPollsSuccess", func(t *testing.T) {
		m.IncrementPollsSuccess(t.Context(), nodeName)
		require.InEpsilon(t,
			1.0,
			testutil.ToFloat64(promPoolRPCNodePollsSuccess.WithLabelValues("test-network", "1", nodeName)),
			0.001,
		)
	})
}

func TestMultiNodeMetrics_IncrementInvariantViolations(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)

	m.IncrementInvariantViolations(t.Context(), "wrong_nonce")

	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promMultiNodeInvariantViolations.WithLabelValues("test-network", "1", "wrong_nonce")),
		0.001,
	)
}

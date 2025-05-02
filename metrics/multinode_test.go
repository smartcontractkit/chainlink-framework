package metrics

import (
	"context"
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
	ctx := context.Background()

	m.RecordNodeStates(ctx, "Alive", 5)

	require.InEpsilon(t,
		5.0,
		testutil.ToFloat64(promMultiNodeRPCNodeStates.WithLabelValues("test-network", "1", "Alive")),
		0.001,
	)
}

func TestMultiNodeMetrics_RecordNodeClientVersion(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)
	ctx := context.Background()

	m.RecordNodeClientVersion(ctx, "test-node-1", "rpc-1.2.3")

	value := testutil.ToFloat64(promNodeClientVersion.WithLabelValues(
		"test-network",
		"1",
		"test-node-1",
		"rpc-1.2.3",
	))

	require.Equal(t, float64(1), value)
}

func TestMultiNodeMetrics_Verifies(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)
	ctx := context.Background()

	m.IncrementNodeVerifies(ctx, "node-1")
	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promPoolRPCNodeVerifies.WithLabelValues("test-network", "1", "node-1")),
		0.001,
	)

	m.IncrementNodeVerifiesFailed(ctx, "node-1")
	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promPoolRPCNodeVerifiesFailed.WithLabelValues("test-network", "1", "node-1")),
		0.001,
	)

	m.IncrementNodeVerifiesSuccess(ctx, "node-1")
	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promPoolRPCNodeVerifiesSuccess.WithLabelValues("test-network", "1", "node-1")),
		0.001,
	)
}

func TestMultiNodeMetrics_NodeTransitions(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)
	ctx := context.Background()
	nodeName := "node-1"

	tests := []struct {
		name       string
		increment  func()
		promMetric *prometheus.CounterVec
	}{
		{
			name: "Alive",
			increment: func() {
				m.IncrementNodeTransitionsToAlive(ctx, nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToAlive,
		},
		{
			name: "InSync",
			increment: func() {
				m.IncrementNodeTransitionsToInSync(ctx, nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToInSync,
		},
		{
			name: "OutOfSync",
			increment: func() {
				m.IncrementNodeTransitionsToOutOfSync(ctx, nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToOutOfSync,
		},
		{
			name: "Unreachable",
			increment: func() {
				m.IncrementNodeTransitionsToUnreachable(ctx, nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToUnreachable,
		},
		{
			name: "InvalidChainID",
			increment: func() {
				m.IncrementNodeTransitionsToInvalidChainID(ctx, nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToInvalidChainID,
		},
		{
			name: "Unusable",
			increment: func() {
				m.IncrementNodeTransitionsToUnusable(ctx, nodeName)
			},
			promMetric: promPoolRPCNodeTransitionsToUnusable,
		},
		{
			name: "Syncing",
			increment: func() {
				m.IncrementNodeTransitionsToSyncing(ctx, nodeName)
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

func TestMultiNodeMetrics_IncrementInvariantViolations(t *testing.T) {
	m := setupTestMultiNodeMetrics(t)
	ctx := context.Background()

	m.IncrementInvariantViolations(ctx, "wrong_nonce")

	require.InEpsilon(t,
		1.0,
		testutil.ToFloat64(promMultiNodeInvariantViolations.WithLabelValues("test-network", "1", "wrong_nonce")),
		0.001,
	)
}

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestBalanceMetrics(t *testing.T) GenericBalanceMetrics {
	m, err := NewGenericBalanceMetrics("test-network", "1")
	require.NoError(t, err)
	return m
}

func TestBalanceMetrics_RecordNodeBalance(t *testing.T) {
	m := setupTestBalanceMetrics(t)
	m.RecordNodeBalance(t.Context(), "account", 5.5)
	require.InEpsilon(t,
		5.5,
		testutil.ToFloat64(NodeBalance.WithLabelValues("account", "1", "test-network")),
		0.001,
	)
}

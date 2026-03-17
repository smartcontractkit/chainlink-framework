//nolint:revive
package metrics

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRPCClientMetrics(t *testing.T) {
	m, err := NewRPCClientMetrics(RPCClientMetricsConfig{
		Env:         "staging",
		Network:     "ethereum",
		ChainID:     "1",
		RPCProvider: "primary",
	})
	require.NoError(t, err)
	require.NotNil(t, m)

	ctx := context.Background()
	m.RecordRequest(ctx, "latest_block", 100.0, nil)
	m.RecordRequest(ctx, "latest_block", 50.0, errors.New("rpc error"))
}

func TestNoopRPCClientMetrics_RecordRequest(t *testing.T) {
	var m NoopRPCClientMetrics
	ctx := context.Background()
	m.RecordRequest(ctx, "latest_block", 100.0, nil)
	m.RecordRequest(ctx, "latest_block", 50.0, errors.New("rpc error"))
	// Noop should not panic
}

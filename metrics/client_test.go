package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewRPCClientMetrics(t *testing.T) {
	m, err := NewRPCClientMetrics(RPCClientMetricsConfig{
		ChainFamily: "evm",
		ChainID:     "1",
	})
	require.NoError(t, err)
	require.NotNil(t, m)

	ctx := context.Background()
	const url = "http://localhost:8545"
	m.RecordRequest(ctx, url, false, "latest_block", 100*time.Millisecond, nil)
	m.RecordRequest(ctx, url, true, "latest_block", 50*time.Millisecond, errors.New("rpc error"))
}

func TestNoopRPCClientMetrics_RecordRequest(t *testing.T) {
	var m NoopRPCClientMetrics
	ctx := context.Background()
	m.RecordRequest(ctx, "http://localhost:8545", false, "latest_block", 100*time.Millisecond, nil)
	m.RecordRequest(ctx, "http://localhost:8545", false, "latest_block", 50*time.Millisecond, errors.New("rpc error"))
}

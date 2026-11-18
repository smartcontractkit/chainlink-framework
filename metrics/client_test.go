package metrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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
	m.RecordRequest(ctx, "http://localhost:8545", false, "latest_block", 100*time.Millisecond, nil)
	m.RecordRequest(ctx, "http://localhost:8545", false, "latest_block", 50*time.Millisecond, errors.New("rpc error"))
}

func TestNoopRPCClientMetrics_RecordRequest(t *testing.T) {
	var m NoopRPCClientMetrics
	ctx := context.Background()
	m.RecordRequest(ctx, "http://localhost:8545", false, "latest_block", 100*time.Millisecond, nil)
	m.RecordRequest(ctx, "http://localhost:8545", false, "latest_block", 50*time.Millisecond, errors.New("rpc error"))
}

func TestRPCClientMetrics_SuccessLabel(t *testing.T) {
	m, err := NewRPCClientMetrics(RPCClientMetricsConfig{ChainFamily: "evm", ChainID: "success-label-test"})
	require.NoError(t, err)

	ctx := context.Background()
	m.RecordRequest(ctx, "localhost:8545", false, "latest_block", time.Millisecond, nil)
	m.RecordRequest(ctx, "localhost:8545", false, "latest_block", time.Millisecond, nil)
	m.RecordRequest(ctx, "localhost:8545", false, "latest_block", time.Millisecond, errors.New("rpc error"))

	count := func(success string) uint64 {
		var pb dto.Metric
		obs := RPCCallLatency.WithLabelValues("evm", "success-label-test", "localhost:8545", "false", success, "latest_block")
		require.NoError(t, obs.(prometheus.Metric).Write(&pb))
		return pb.GetHistogram().GetSampleCount()
	}
	require.Equal(t, uint64(2), count("true"), "successful calls must be success=true")
	require.Equal(t, uint64(1), count("false"), "failed calls must be success=false")
}

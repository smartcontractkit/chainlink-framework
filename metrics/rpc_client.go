// RPC client observability using Beholder.
//
// This file defines rpc_request_latency_ms and rpc_request_errors_total, emitted
// from the RPC client when RPCClientBase is constructed with a non-nil RPCClientMetrics.
// Metrics are queryable in Prometheus/Grafana by env, network, chain_id, and rpc_provider.
//
// Example Prometheus/Grafana queries:
//
//   - Latency over time (e.g. p99 by env and chain):
//     histogram_quantile(0.99, sum(rate(rpc_request_latency_ms_bucket[5m])) by (le, env, network, chain_id))
//
//   - Error rate over time (errors per second by env and chain):
//     sum(rate(rpc_request_errors_total[5m])) by (env, network, chain_id, rpc_provider)
//
//   - Request rate by call type:
//     sum(rate(rpc_request_latency_ms_count[5m])) by (call, env, network)
package metrics

import (
	"context"
	"fmt"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
)

const (
	// RPCRequestLatencyMs is the Beholder/Prometheus metric name for RPC call latency in milliseconds.
	RPCRequestLatencyMs = "rpc_request_latency_ms"
	// RPCRequestErrorsTotal is the Beholder/Prometheus metric name for total RPC call errors.
	RPCRequestErrorsTotal = "rpc_request_errors_total"
)

var (
	rpcRequestLatencyBuckets = []float64{
		5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000,
	}
	promRPCRequestLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    RPCRequestLatencyMs,
		Help:    "RPC request latency in milliseconds (per call)",
		Buckets: rpcRequestLatencyBuckets,
	}, []string{"env", "network", "chain_id", "rpc_provider", "call"})
	promRPCRequestErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: RPCRequestErrorsTotal,
		Help: "Total number of failed RPC requests",
	}, []string{"env", "network", "chain_id", "rpc_provider", "call"})
)

// RPCClientMetrics records RPC latency and error metrics for observability via Beholder/Prometheus.
// Metrics are queryable by environment, network, chain_id, and rpc_provider in Grafana.
type RPCClientMetrics interface {
	// RecordRequest records latency for an RPC call. If err is non-nil, also increments the error counter.
	// callName identifies the operation (e.g. "latest_block", "latest_finalized_block").
	RecordRequest(ctx context.Context, callName string, latencyMs float64, err error)
}

var _ RPCClientMetrics = (*rpcClientMetrics)(nil)

type rpcClientMetrics struct {
	env         string
	network     string
	chainID     string
	rpcProvider string
	latency     metric.Float64Histogram
	errorsTotal metric.Int64Counter
}

// RPCClientMetricsConfig holds labels for RPC client metrics.
// Empty strings are allowed; they will still be emitted as labels for filtering.
type RPCClientMetricsConfig struct {
	Env         string // e.g. "staging", "production"
	Network     string // chain/network name
	ChainID     string // chain ID
	RPCProvider string // RPC provider or node name (optional)
}

// NewRPCClientMetrics creates RPC client metrics that publish to Beholder and Prometheus.
// Callers (e.g. chain-specific RPC clients or multinode) should pass env, network, chainID, and optionally rpcProvider
// so metrics can be queried in Grafana by environment, chain/network, and RPC provider.
func NewRPCClientMetrics(cfg RPCClientMetricsConfig) (RPCClientMetrics, error) {
	latency, err := beholder.GetMeter().Float64Histogram(RPCRequestLatencyMs)
	if err != nil {
		return nil, fmt.Errorf("failed to register RPC request latency metric: %w", err)
	}
	errorsTotal, err := beholder.GetMeter().Int64Counter(RPCRequestErrorsTotal)
	if err != nil {
		return nil, fmt.Errorf("failed to register RPC request errors metric: %w", err)
	}
	return &rpcClientMetrics{
		env:         cfg.Env,
		network:     cfg.Network,
		chainID:     cfg.ChainID,
		rpcProvider: cfg.RPCProvider,
		latency:     latency,
		errorsTotal: errorsTotal,
	}, nil
}

func (m *rpcClientMetrics) RecordRequest(ctx context.Context, callName string, latencyMs float64, err error) {
	attrs := metric.WithAttributes(
		attribute.String("env", m.env),
		attribute.String("network", m.network),
		attribute.String("chain_id", m.chainID),
		attribute.String("rpc_provider", m.rpcProvider),
		attribute.String("call", callName),
	)
	promRPCRequestLatency.WithLabelValues(m.env, m.network, m.chainID, m.rpcProvider, callName).Observe(latencyMs)
	m.latency.Record(ctx, latencyMs, attrs)
	if err != nil {
		promRPCRequestErrors.WithLabelValues(m.env, m.network, m.chainID, m.rpcProvider, callName).Inc()
		m.errorsTotal.Add(ctx, 1, attrs)
	}
}

// NoopRPCClientMetrics is a no-op implementation for when metrics are disabled.
type NoopRPCClientMetrics struct{}

func (NoopRPCClientMetrics) RecordRequest(context.Context, string, float64, error) {}

// Ensure NoopRPCClientMetrics implements RPCClientMetrics.
var _ RPCClientMetrics = NoopRPCClientMetrics{}

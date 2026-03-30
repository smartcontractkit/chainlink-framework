package metrics

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
)

var (
	RPCCallLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: rpcCallLatencyBeholder,
		Help: "The duration of an RPC call in nanoseconds",
		Buckets: []float64{
			float64(50 * time.Millisecond),
			float64(100 * time.Millisecond),
			float64(200 * time.Millisecond),
			float64(500 * time.Millisecond),
			float64(1 * time.Second),
			float64(2 * time.Second),
			float64(4 * time.Second),
			float64(8 * time.Second),
		},
	}, []string{"chainFamily", "chainID", "rpcUrl", "isSendOnly", "success", "rpcCallName"})
)

const rpcCallLatencyBeholder = "rpc_call_latency"

// RPCClientMetrics records RPC call latency to Prometheus and Beholder (failures: success="false"; same pattern as multinode metrics).
// Construct once per chain (or process) with ChainFamily and ChainID; pass rpcUrl and isSendOnly on each call
// when they vary by node or request.
type RPCClientMetrics interface {
	// RecordRequest records latency for an RPC call (observed in nanoseconds for Prometheus and Beholder).
	// Failures use success="false"; derive error rate from rpc_call_latency_count{success="false"} (or equivalent).
	RecordRequest(ctx context.Context, rpcURL string, isSendOnly bool, callName string, latency time.Duration, err error)
}

var _ RPCClientMetrics = (*rpcClientMetrics)(nil)

type rpcClientMetrics struct {
	chainFamily string
	chainID     string
	latencyHis  metric.Float64Histogram
}

// RPCClientMetricsConfig holds labels that are fixed for the lifetime of the metrics handle (e.g. one per chain).
type RPCClientMetricsConfig struct {
	ChainFamily string
	ChainID     string
}

// NewRPCClientMetrics creates RPC client metrics that publish to Prometheus and Beholder.
func NewRPCClientMetrics(cfg RPCClientMetricsConfig) (RPCClientMetrics, error) {
	latency, err := beholder.GetMeter().Float64Histogram(rpcCallLatencyBeholder)
	if err != nil {
		return nil, fmt.Errorf("failed to register RPC call latency metric: %w", err)
	}
	return &rpcClientMetrics{
		chainFamily: cfg.ChainFamily,
		chainID:     cfg.ChainID,
		latencyHis:  latency,
	}, nil
}

func (m *rpcClientMetrics) RecordRequest(ctx context.Context, rpcURL string, isSendOnly bool, callName string, latency time.Duration, err error) {
	successStr := "true"
	if err != nil {
		successStr = "false"
	}
	sendStr := strconv.FormatBool(isSendOnly)
	latencyNs := float64(latency)

	RPCCallLatency.WithLabelValues(m.chainFamily, m.chainID, rpcURL, sendStr, successStr, callName).Observe(latencyNs)

	latAttrs := metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID),
		attribute.String("rpcUrl", rpcURL),
		attribute.String("isSendOnly", sendStr),
		attribute.String("success", successStr),
		attribute.String("rpcCallName", callName),
	)
	m.latencyHis.Record(ctx, latencyNs, latAttrs)
}

// NoopRPCClientMetrics is a no-op implementation for when metrics are disabled.
type NoopRPCClientMetrics struct{}

func (NoopRPCClientMetrics) RecordRequest(context.Context, string, bool, string, time.Duration, error) {
}

var _ RPCClientMetrics = NoopRPCClientMetrics{}

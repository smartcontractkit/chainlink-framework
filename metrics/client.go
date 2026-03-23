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
	// RPCCallLatency measures RPC duration in milliseconds (bucket upper bounds from 50 ms to 8 s).
	// Values are latency.Seconds()*1000, not float64(duration) — the latter is nanoseconds and will skew quantiles.
	RPCCallLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: rpcCallLatencyBeholder,
		Help: "The duration of an RPC call in milliseconds",
		Buckets: []float64{
			50, 100, 200, 500,
			1000, 2000, 4000, 8000,
		},
	}, []string{"chainFamily", "chainID", "rpcUrl", "isSendOnly", "success", "rpcCallName"})

	RPCCallErrorsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rpc_call_errors_total",
		Help: "The total number of failed RPC calls",
	}, []string{"chainFamily", "chainID", "rpcUrl", "isSendOnly", "rpcCallName"})
)

const (
	rpcCallLatencyBeholder     = "rpc_call_latency"
	rpcCallErrorsTotalBeholder = "rpc_call_errors_total"
)

// RPCClientMetrics records RPC latency and errors to Prometheus and Beholder (same pattern as multinode metrics).
// Construct once per chain (or process) with ChainFamily and ChainID; pass rpcUrl and isSendOnly on each call
// when they vary by node or request.
type RPCClientMetrics interface {
	// RecordRequest records latency for an RPC call (observed in milliseconds for Prometheus and Beholder).
	// If err is non-nil, increments rpc_call_errors_total.
	RecordRequest(ctx context.Context, rpcURL string, isSendOnly bool, callName string, latency time.Duration, err error)
}

var _ RPCClientMetrics = (*rpcClientMetrics)(nil)

type rpcClientMetrics struct {
	chainFamily   string
	chainID       string
	latencyHis    metric.Float64Histogram
	errorsCounter metric.Int64Counter
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
	errorsTotal, err := beholder.GetMeter().Int64Counter(rpcCallErrorsTotalBeholder)
	if err != nil {
		return nil, fmt.Errorf("failed to register RPC call errors metric: %w", err)
	}
	return &rpcClientMetrics{
		chainFamily:   cfg.ChainFamily,
		chainID:       cfg.ChainID,
		latencyHis:    latency,
		errorsCounter: errorsTotal,
	}, nil
}

func (m *rpcClientMetrics) RecordRequest(ctx context.Context, rpcURL string, isSendOnly bool, callName string, latency time.Duration, err error) {
	successStr := "true"
	if err != nil {
		successStr = "false"
	}
	sendStr := strconv.FormatBool(isSendOnly)
	ms := latency.Seconds() * 1000

	RPCCallLatency.WithLabelValues(m.chainFamily, m.chainID, rpcURL, sendStr, successStr, callName).Observe(ms)

	latAttrs := metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID),
		attribute.String("rpcUrl", rpcURL),
		attribute.String("isSendOnly", sendStr),
		attribute.String("success", successStr),
		attribute.String("rpcCallName", callName),
	)
	m.latencyHis.Record(ctx, ms, latAttrs)

	if err != nil {
		RPCCallErrorsTotal.WithLabelValues(m.chainFamily, m.chainID, rpcURL, sendStr, callName).Inc()
		errAttrs := metric.WithAttributes(
			attribute.String("chainFamily", m.chainFamily),
			attribute.String("chainID", m.chainID),
			attribute.String("rpcUrl", rpcURL),
			attribute.String("isSendOnly", sendStr),
			attribute.String("rpcCallName", callName),
		)
		m.errorsCounter.Add(ctx, 1, errAttrs)
	}
}

// NoopRPCClientMetrics is a no-op implementation for when metrics are disabled.
type NoopRPCClientMetrics struct{}

func (NoopRPCClientMetrics) RecordRequest(context.Context, string, bool, string, time.Duration, error) {
}

var _ RPCClientMetrics = NoopRPCClientMetrics{}

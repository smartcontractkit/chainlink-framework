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
		Name: "rpc_call_latency",
		Help: "The duration of an RPC call in seconds",
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
type RPCClientMetrics interface {
	// RecordRequest records latency for an RPC call (observed in seconds for Prometheus).
	// If err is non-nil, increments rpc_call_errors_total.
	RecordRequest(ctx context.Context, callName string, latency time.Duration, err error)
}

var _ RPCClientMetrics = (*rpcClientMetrics)(nil)

type rpcClientMetrics struct {
	chainFamily string
	chainID     string
	rpcURL      string
	isSendOnly  bool
	latency     metric.Float64Histogram
	errorsTotal metric.Int64Counter
}

// RPCClientMetricsConfig holds fixed labels for an RPC client instance.
type RPCClientMetricsConfig struct {
	ChainFamily string
	ChainID     string
	RPCURL      string
	IsSendOnly  bool
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
		chainFamily: cfg.ChainFamily,
		chainID:     cfg.ChainID,
		rpcURL:      cfg.RPCURL,
		isSendOnly:  cfg.IsSendOnly,
		latency:     latency,
		errorsTotal: errorsTotal,
	}, nil
}

func (m *rpcClientMetrics) RecordRequest(ctx context.Context, callName string, latency time.Duration, err error) {
	successStr := "true"
	if err != nil {
		successStr = "false"
	}
	sendStr := strconv.FormatBool(m.isSendOnly)
	sec := latency.Seconds()

	RPCCallLatency.WithLabelValues(m.chainFamily, m.chainID, m.rpcURL, sendStr, successStr, callName).Observe(sec)

	latAttrs := metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID),
		attribute.String("rpcUrl", m.rpcURL),
		attribute.String("isSendOnly", sendStr),
		attribute.String("success", successStr),
		attribute.String("rpcCallName", callName),
	)
	m.latency.Record(ctx, sec, latAttrs)

	if err != nil {
		RPCCallErrorsTotal.WithLabelValues(m.chainFamily, m.chainID, m.rpcURL, sendStr, callName).Inc()
		errAttrs := metric.WithAttributes(
			attribute.String("chainFamily", m.chainFamily),
			attribute.String("chainID", m.chainID),
			attribute.String("rpcUrl", m.rpcURL),
			attribute.String("isSendOnly", sendStr),
			attribute.String("rpcCallName", callName),
		)
		m.errorsTotal.Add(ctx, 1, errAttrs)
	}
}

// NoopRPCClientMetrics is a no-op implementation for when metrics are disabled.
type NoopRPCClientMetrics struct{}

func (NoopRPCClientMetrics) RecordRequest(context.Context, string, time.Duration, error) {}

var _ RPCClientMetrics = NoopRPCClientMetrics{}

package metrics

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
)

type QueryType string

const (
	Create QueryType = "create"
	Read   QueryType = "read"
	Del    QueryType = "delete"
)

var (
	sqlLatencyBuckets = []float64{
		float64(1 * time.Millisecond),
		float64(5 * time.Millisecond),
		float64(10 * time.Millisecond),
		float64(20 * time.Millisecond),
		float64(30 * time.Millisecond),
		float64(40 * time.Millisecond),
		float64(50 * time.Millisecond),
		float64(60 * time.Millisecond),
		float64(70 * time.Millisecond),
		float64(80 * time.Millisecond),
		float64(90 * time.Millisecond),
		float64(100 * time.Millisecond),
		float64(200 * time.Millisecond),
		float64(300 * time.Millisecond),
		float64(400 * time.Millisecond),
		float64(500 * time.Millisecond),
		float64(750 * time.Millisecond),
		float64(1 * time.Second),
		float64(2 * time.Second),
		float64(5 * time.Second),
	}
	PromLpQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_poller_query_duration",
		Help:    "Measures duration of Log Poller's queries fetching logs",
		Buckets: sqlLatencyBuckets,
	}, []string{"chainFamily", "chainID", "query", "type"})
	PromLpQueryDataSets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "log_poller_query_dataset_size",
		Help: "Measures size of the datasets returned by Log Poller's queries",
	}, []string{"chainFamily", "chainID", "query", "type"})
	PromLpLogsInserted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_poller_logs_inserted",
		Help: "Counter to track number of logs inserted by Log Poller",
	}, []string{"chainFamily", "chainID"})
	PromLpBlocksInserted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "log_poller_blocks_inserted",
		Help: "Counter to track number of blocks inserted by Log Poller",
	}, []string{"chainFamily", "chainID"})
)

type GenericLogPollerMetrics interface {
	RecordQueryDuration(ctx context.Context, queryName string, queryType QueryType, duration float64)
	RecordQueryDatasetSize(ctx context.Context, queryName string, queryType QueryType, size int64)
	IncrementLogsInserted(ctx context.Context, numLogs int64)
	IncrementBlocksInserted(ctx context.Context, numBlocks int64)
}

var _ GenericLogPollerMetrics = &logPollerMetrics{}

type logPollerMetrics struct {
	chainID           string
	chainFamily       string
	queryDuration     metric.Float64Histogram
	queryDatasetsSize metric.Int64Gauge
	logsInserted      metric.Int64Counter
	blocksInserted    metric.Int64Counter
}

func NewGenericLogPollerMetrics(chainID string, chainFamily string) (GenericLogPollerMetrics, error) {
	queryDuration, err := beholder.GetMeter().Float64Histogram("beholder_log_poller_query_duration")
	if err != nil {
		return nil, fmt.Errorf("failed to register logpoller query duration metric: %w", err)
	}

	queryDatasetSize, err := beholder.GetMeter().Int64Gauge("beholder_log_poller_query_dataset_size")
	if err != nil {
		return nil, fmt.Errorf("failed to register query dataset size metric: %w", err)
	}

	logsInserted, err := beholder.GetMeter().Int64Counter("beholder_log_poller_logs_inserted")
	if err != nil {
		return nil, fmt.Errorf("failed to register logs inserted metric: %w", err)
	}

	blocksInserted, err := beholder.GetMeter().Int64Counter("beholder_log_poller_blocks_inserted")
	if err != nil {
		return nil, fmt.Errorf("failed to register blocks inserted metric: %w", err)
	}

	return &logPollerMetrics{
		chainID:           chainID,
		chainFamily:       chainFamily,
		queryDuration:     queryDuration,
		queryDatasetsSize: queryDatasetSize,
		logsInserted:      logsInserted,
		blocksInserted:    blocksInserted,
	}, nil
}

func (m *logPollerMetrics) RecordQueryDuration(ctx context.Context, queryName string, queryType QueryType, duration float64) {
	PromLpQueryDuration.WithLabelValues(m.chainFamily, m.chainID, queryName, string(queryType)).Observe(duration)
	m.queryDuration.Record(ctx, duration, metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID),
		attribute.String("query", queryName),
		attribute.String("type", string(queryType))))
}

func (m *logPollerMetrics) RecordQueryDatasetSize(ctx context.Context, queryName string, queryType QueryType, size int64) {
	PromLpQueryDataSets.WithLabelValues(m.chainFamily, m.chainID, queryName, string(queryType)).Set(float64(size))
	m.queryDatasetsSize.Record(ctx, size, metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID),
		attribute.String("query", queryName),
		attribute.String("type", string(queryType))))
}

func (m *logPollerMetrics) IncrementLogsInserted(ctx context.Context, numLogs int64) {
	PromLpLogsInserted.WithLabelValues(m.chainFamily, m.chainID).Add(float64(numLogs))
	m.logsInserted.Add(ctx, numLogs, metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID)))
}

func (m *logPollerMetrics) IncrementBlocksInserted(ctx context.Context, numBlocks int64) {
	PromLpBlocksInserted.WithLabelValues(m.chainFamily, m.chainID).Add(float64(numBlocks))
	m.blocksInserted.Add(ctx, numBlocks, metric.WithAttributes(
		attribute.String("chainFamily", m.chainFamily),
		attribute.String("chainID", m.chainID)))
}

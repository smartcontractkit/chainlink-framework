package logpoller

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type QueryType string

const (
	QueryCreate QueryType = "create"
	QueryRead   QueryType = "read"
	QueryDel    QueryType = "delete"
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
	lpQueryDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "log_poller_query_duration",
		Help:    "Measures duration of Log Poller's queries fetching logs",
		Buckets: sqlLatencyBuckets,
	}, []string{"evmChainID", "query", "type", "chain_id"})
	lpQueryDataSets = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "log_poller_query_dataset_size",
		Help: "Measures size of the datasets returned by Log Poller's queries",
	}, []string{"evmChainID", "query", "type", "chain_id"})
)

func WithObservedQueryAndResults[T any](chainID, queryName string, query func() ([]T, error)) ([]T, error) {
	results, err := WithObservedQuery(chainID, queryName, query)
	if err == nil {
		lpQueryDataSets.
			WithLabelValues(chainID, queryName, string(QueryRead), chainID).
			Set(float64(len(results)))
	}
	return results, err
}

func WithObservedExecAndRowsAffected(chainID, queryName string, queryType QueryType, exec func() (int64, error)) (int64, error) {
	queryStarted := time.Now()
	rowsAffected, err := exec()
	lpQueryDuration.
		WithLabelValues(chainID, queryName, string(queryType)).
		Observe(float64(time.Since(queryStarted)))

	if err == nil {
		lpQueryDataSets.
			WithLabelValues(chainID, queryName, string(queryType), chainID).
			Set(float64(rowsAffected))
	}

	return rowsAffected, err
}

func WithObservedQuery[T any](chainID, queryName string, query func() (T, error)) (T, error) {
	queryStarted := time.Now()
	defer func() {
		lpQueryDuration.
			WithLabelValues(chainID, queryName, string(QueryRead), chainID).
			Observe(float64(time.Since(queryStarted)))
	}()
	return query()
}

func WithObservedExec(chainID, query string, queryType QueryType, exec func() error) error {
	queryStarted := time.Now()
	defer func() {
		lpQueryDuration.
			WithLabelValues(chainID, query, string(queryType), chainID).
			Observe(float64(time.Since(queryStarted)))
	}()
	return exec()
}

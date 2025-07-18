//nolint:gosec // disable G115
package registry

import (
	"context"
	"fmt"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	beholdercommon "github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder"
)

// ns returns a namespaced metric name
func ns(name string) string {
	return "data_feeds_on_chain_registry_" + name
}

// Define a new struct for metrics
type Metrics struct {
	// Define on FeedUpdated metrics
	feedUpdated struct {
		basic beholder.MetricsCapBasic
		// specific to FeedUpdated
		observationsTimestamp metric.Int64Gauge
		duration              metric.Int64Gauge // ts.emit - ts.observation
		benchmark             metric.Float64Gauge
		blockTimestamp        metric.Int64Gauge
		blockNumber           metric.Int64Gauge
	}
}

func NewMetrics() (*Metrics, error) {
	// Define new metrics
	m := &Metrics{}

	meter := beholdercommon.GetMeter()

	// Create new metrics
	var err error

	// Define metrics configuration
	feedUpdated := struct {
		basic beholder.MetricsInfoCapBasic
		// specific to FeedUpdated
		observationsTimestamp beholdercommon.MetricInfo
		duration              beholdercommon.MetricInfo // ts.emit - ts.observation
		benchmark             beholdercommon.MetricInfo
		blockTimestamp        beholdercommon.MetricInfo
		blockNumber           beholdercommon.MetricInfo
	}{
		basic: beholder.NewMetricsInfoCapBasic(ns("feed_updated"), beholdercommon.ToSchemaFullName(&FeedUpdated{})),
		observationsTimestamp: beholdercommon.MetricInfo{
			Name:        ns("feed_updated_observations_timestamp"),
			Unit:        "ms",
			Description: "The observations timestamp for the latest confirmed update (as reported)",
		},
		duration: beholdercommon.MetricInfo{
			Name:        ns("feed_updated_duration"),
			Unit:        "ms",
			Description: "The duration (local) since observation to message: 'datafeeds.on-chain.registry.FeedUpdated' emit",
		},
		benchmark: beholdercommon.MetricInfo{
			Name:        ns("feed_updated_benchmark"),
			Unit:        "",
			Description: "The benchmark value for the latest confirmed update (as reported)",
		},
		blockTimestamp: beholdercommon.MetricInfo{
			Name:        ns("feed_updated_block_timestamp"),
			Unit:        "ms",
			Description: "The block timestamp at the latest confirmed update (as observed)",
		},
		blockNumber: beholdercommon.MetricInfo{
			Name:        ns("feed_updated_block_number"),
			Unit:        "",
			Description: "The block number at the latest confirmed update (as observed)",
		},
	}

	m.feedUpdated.basic, err = beholder.NewMetricsCapBasic(feedUpdated.basic)
	if err != nil {
		return nil, fmt.Errorf("failed to create new basic metrics: %w", err)
	}

	m.feedUpdated.observationsTimestamp, err = feedUpdated.observationsTimestamp.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.feedUpdated.duration, err = feedUpdated.duration.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.feedUpdated.benchmark, err = feedUpdated.benchmark.NewFloat64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.feedUpdated.blockTimestamp, err = feedUpdated.blockTimestamp.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.feedUpdated.blockNumber, err = feedUpdated.blockNumber.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	return m, nil
}

func (m *Metrics) OnFeedUpdated(ctx context.Context, msg *FeedUpdated, attrKVs ...any) error {
	// Define attributes
	attrs := metric.WithAttributes(msg.Attributes()...)

	// Emit basic metrics (count, timestamps)
	start, emit := msg.ExecutionContext.MetaCapabilityTimestampStart, msg.ExecutionContext.MetaCapabilityTimestampEmit
	m.feedUpdated.basic.RecordEmit(ctx, start, emit, msg.Attributes()...)

	// Timestamp e2e observation update
	m.feedUpdated.observationsTimestamp.Record(ctx, int64(msg.ObservationsTimestamp), attrs)
	observation := uint64(msg.ObservationsTimestamp) * 1000 // convert to milliseconds
	m.feedUpdated.duration.Record(ctx, int64(emit-observation), attrs)

	// Benchmark
	m.feedUpdated.benchmark.Record(ctx, msg.BenchmarkVal, attrs)

	// Block timestamp
	m.feedUpdated.blockTimestamp.Record(ctx, int64(msg.BlockData.BlockTimestamp), attrs)

	// Block number
	blockHeightVal, err := strconv.ParseInt(msg.BlockData.BlockHeight, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse block height: %w", err)
	}
	m.feedUpdated.blockNumber.Record(ctx, blockHeightVal, attrs)

	return nil
}

// Attributes returns the attributes for the FeedUpdated message to be used in metrics
func (m *FeedUpdated) Attributes() []attribute.KeyValue {
	context := beholder.ExecutionMetadata{
		// Execution Context - Source
		SourceID: m.ExecutionContext.MetaSourceId,
		// Execution Context - Chain
		ChainFamilyName: m.ExecutionContext.MetaChainFamilyName,
		ChainID:         m.ExecutionContext.MetaChainId,
		NetworkName:     m.ExecutionContext.MetaNetworkName,
		NetworkNameFull: m.ExecutionContext.MetaNetworkNameFull,
		// Execution Context - Workflow (capabilities.RequestMetadata)
		WorkflowID:               m.ExecutionContext.MetaWorkflowId,
		WorkflowOwner:            m.ExecutionContext.MetaWorkflowOwner,
		WorkflowExecutionID:      m.ExecutionContext.MetaWorkflowExecutionId,
		WorkflowName:             m.ExecutionContext.MetaWorkflowName,
		WorkflowDonID:            m.ExecutionContext.MetaWorkflowDonId,
		WorkflowDonConfigVersion: m.ExecutionContext.MetaWorkflowDonConfigVersion,
		ReferenceID:              m.ExecutionContext.MetaReferenceId,
		// Execution Context - Capability
		CapabilityType: m.ExecutionContext.MetaCapabilityType,
		CapabilityID:   m.ExecutionContext.MetaCapabilityId,
	}

	attrs := []attribute.KeyValue{
		// Transaction Data
		attribute.String("tx_sender", m.TransactionData.TxSender),
		attribute.String("tx_receiver", m.TransactionData.TxReceiver),

		// Event Data
		attribute.String("feed_id", m.FeedId),
		// TODO: do we need these attributes? (available in WriteConfirmed)
		// attribute.Int64("report_id", int64(m.ReportId)), // uint32 -> int64

		// We mark confrmations by transmitter so we can query for only initial (fast) confirmations
		// with PromQL, and ignore the slower confirmations by other signers for SLA measurements.
		attribute.Bool("observed_by_transmitter", m.TransactionData.TxSender == m.ExecutionContext.MetaSourceId), // source_id == node account
	}

	return append(attrs, context.Attributes()...)
}

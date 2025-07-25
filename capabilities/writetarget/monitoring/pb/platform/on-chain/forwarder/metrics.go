//nolint:gosec // disable G115
package forwarder

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
	return fmt.Sprintf("platform_on_chain_forwarder_%s", name)
}

// Define a new struct for metrics
type Metrics struct {
	// Define on ReportProcessed metrics
	reportProcessed struct {
		basic beholder.MetricsCapBasic
		// specific to ReportProcessed
		blockTimestamp metric.Int64Gauge
		blockNumber    metric.Int64Gauge
	}
}

func NewMetrics() (*Metrics, error) {
	// Define new metrics
	m := &Metrics{}

	meter := beholdercommon.GetMeter()

	// Create new metrics
	var err error

	reportProcessed := struct {
		basic beholder.MetricsInfoCapBasic
		// specific to ReportProcessed
		blockTimestamp beholdercommon.MetricInfo
		blockNumber    beholdercommon.MetricInfo
	}{
		basic: beholder.NewMetricsInfoCapBasic(ns("report_processed"), beholdercommon.ToSchemaFullName(&ReportProcessed{})),
		blockTimestamp: beholdercommon.MetricInfo{
			Name:        ns("report_processed_block_timestamp"),
			Unit:        "ms",
			Description: "The block timestamp at the latest confirmed write (as observed)",
		},
		blockNumber: beholdercommon.MetricInfo{
			Name:        ns("report_processed_block_number"),
			Unit:        "",
			Description: "The block number at the latest confirmed write (as observed)",
		},
	}

	fmt.Println("report processed name:", beholdercommon.ToSchemaFullName(&ReportProcessed{}))

	m.reportProcessed.basic, err = beholder.NewMetricsCapBasic(reportProcessed.basic)
	if err != nil {
		return nil, fmt.Errorf("failed to create new basic metrics: %w", err)
	}

	m.reportProcessed.blockTimestamp, err = reportProcessed.blockTimestamp.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.reportProcessed.blockNumber, err = reportProcessed.blockNumber.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	return m, nil
}

func (m *Metrics) OnReportProcessed(ctx context.Context, msg *ReportProcessed, attrKVs ...any) error {
	// Define attributes
	attrs := metric.WithAttributes(msg.Attributes()...)

	// Emit basic metrics (count, timestamps)
	start, emit := msg.ExecutionContext.MetaCapabilityTimestampStart, msg.ExecutionContext.MetaCapabilityTimestampEmit
	m.reportProcessed.basic.RecordEmit(ctx, start, emit, msg.Attributes()...)

	// Block timestamp
	m.reportProcessed.blockTimestamp.Record(ctx, int64(msg.BlockData.BlockTimestamp), attrs)

	// Block number
	blockHeightVal, err := strconv.ParseInt(msg.BlockData.BlockHeight, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse block height: %w", err)
	}
	m.reportProcessed.blockNumber.Record(ctx, blockHeightVal, attrs)

	return nil
}

// Attributes returns the attributes for the ReportProcessed message to be used in metrics
func (m *ReportProcessed) Attributes() []attribute.KeyValue {
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
		attribute.String("receiver", m.Receiver),
		attribute.Int64("report_id", int64(m.ReportId)), // uint32 -> int64
		attribute.Bool("success", m.Success),

		// We mark confrmations by transmitter so we can query for only initial (fast) confirmations
		// with PromQL, and ignore the slower confirmations by other signers for SLA measurements.
		attribute.Bool("observed_by_transmitter", m.TransactionData.TxSender == m.ExecutionContext.MetaSourceId), // source_id == node account
	}

	return append(attrs, context.Attributes()...)
}

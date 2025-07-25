//nolint:gosec // disable G115
package writetarget

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
	return fmt.Sprintf("platform_write_target_%s", name)
}

// Define a new struct for metrics
type Metrics struct {
	// Define on WriteInitiated metrics
	writeInitiated struct {
		basic beholder.MetricsCapBasic
	}
	// Define on WriteError metrics
	writeError struct {
		basic beholder.MetricsCapBasic
	}
	// Define on WriteSent metrics
	writeSent struct {
		basic beholder.MetricsCapBasic
		// specific to WriteSent
		blockTimestamp metric.Int64Gauge
		blockNumber    metric.Int64Gauge
	}
	// Define on WriteConfirmed metrics
	writeConfirmed struct {
		basic beholder.MetricsCapBasic
		// specific to WriteConfirmed
		blockTimestamp metric.Int64Gauge
		blockNumber    metric.Int64Gauge
		signersNumber  metric.Int64Gauge
	}
}

func NewMetrics() (*Metrics, error) {
	// Define new metrics
	m := &Metrics{}

	meter := beholdercommon.GetMeter()

	// Create new metrics
	var err error

	// Define metrics configuration
	writeInitiated := struct {
		basic beholder.MetricsInfoCapBasic
	}{
		basic: beholder.NewMetricsInfoCapBasic(ns("write_initiated"), beholdercommon.ToSchemaFullName(&WriteInitiated{})),
	}
	writeError := struct {
		basic beholder.MetricsInfoCapBasic
	}{
		basic: beholder.NewMetricsInfoCapBasic(ns("write_error"), beholdercommon.ToSchemaFullName(&WriteError{})),
	}
	writeSent := struct {
		basic beholder.MetricsInfoCapBasic
		// specific to WriteSent
		blockTimestamp beholdercommon.MetricInfo
		blockNumber    beholdercommon.MetricInfo
	}{
		basic: beholder.NewMetricsInfoCapBasic(ns("write_sent"), beholdercommon.ToSchemaFullName(&WriteSent{})),
		blockTimestamp: beholdercommon.MetricInfo{
			Name:        ns("write_sent_block_timestamp"),
			Unit:        "ms",
			Description: "The block timestamp at the latest sent write (as observed)",
		},
		blockNumber: beholdercommon.MetricInfo{
			Name:        ns("write_sent_block_number"),
			Unit:        "",
			Description: "The block number at the latest sent write (as observed)",
		},
	}
	writeConfirmed := struct {
		basic beholder.MetricsInfoCapBasic
		// specific to WriteSent
		blockTimestamp beholdercommon.MetricInfo
		blockNumber    beholdercommon.MetricInfo
		signersNumber  beholdercommon.MetricInfo
	}{
		basic: beholder.NewMetricsInfoCapBasic(ns("write_confirmed"), beholdercommon.ToSchemaFullName(&WriteConfirmed{})),
		blockTimestamp: beholdercommon.MetricInfo{
			Name:        ns("write_confirmed_block_timestamp"),
			Unit:        "ms",
			Description: "The block timestamp for latest confirmed write (as observed)",
		},
		blockNumber: beholdercommon.MetricInfo{
			Name:        ns("write_confirmed_block_number"),
			Unit:        "",
			Description: "The block number for latest confirmed write (as observed)",
		},
		signersNumber: beholdercommon.MetricInfo{
			Name:        ns("write_confirmed_signers_number"),
			Unit:        "",
			Description: "The number of signers attached to the processed and confirmed write request",
		},
	}

	// WriteInitiated
	m.writeInitiated.basic, err = beholder.NewMetricsCapBasic(writeInitiated.basic)
	if err != nil {
		return nil, fmt.Errorf("failed to create new basic metrics: %w", err)
	}

	// WriteError
	m.writeError.basic, err = beholder.NewMetricsCapBasic(writeError.basic)
	if err != nil {
		return nil, fmt.Errorf("failed to create new basic metrics: %w", err)
	}

	// WriteSent
	m.writeSent.basic, err = beholder.NewMetricsCapBasic(writeSent.basic)
	if err != nil {
		return nil, fmt.Errorf("failed to create new basic metrics: %w", err)
	}

	m.writeSent.blockTimestamp, err = writeSent.blockTimestamp.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.writeSent.blockNumber, err = writeSent.blockNumber.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	// WriteConfirmed
	m.writeConfirmed.basic, err = beholder.NewMetricsCapBasic(writeConfirmed.basic)
	if err != nil {
		return nil, fmt.Errorf("failed to create new basic metrics: %w", err)
	}

	m.writeConfirmed.blockTimestamp, err = writeConfirmed.blockTimestamp.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.writeConfirmed.blockNumber, err = writeConfirmed.blockNumber.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	m.writeConfirmed.signersNumber, err = writeConfirmed.signersNumber.NewInt64Gauge(meter)
	if err != nil {
		return nil, fmt.Errorf("failed to create new gauge: %w", err)
	}

	return m, nil
}

func (m *Metrics) OnWriteInitiated(ctx context.Context, msg *WriteInitiated, attrKVs ...any) error {
	// Emit basic metrics (count, timestamps)
	start, emit := msg.ExecutionContext.MetaCapabilityTimestampStart, msg.ExecutionContext.MetaCapabilityTimestampEmit
	m.writeInitiated.basic.RecordEmit(ctx, start, emit, msg.Attributes()...)
	return nil
}

func (m *Metrics) OnWriteError(ctx context.Context, msg *WriteError, attrKVs ...any) error {
	// Emit basic metrics (count, timestamps)
	start, emit := msg.ExecutionContext.MetaCapabilityTimestampStart, msg.ExecutionContext.MetaCapabilityTimestampEmit
	m.writeError.basic.RecordEmit(ctx, start, emit, msg.Attributes()...)
	return nil
}

func (m *Metrics) OnWriteSent(ctx context.Context, msg *WriteSent, attrKVs ...any) error {
	// Define attributes
	attrs := metric.WithAttributes(msg.Attributes()...)

	// Emit basic metrics (count, timestamps)
	start, emit := msg.ExecutionContext.MetaCapabilityTimestampStart, msg.ExecutionContext.MetaCapabilityTimestampEmit
	m.writeSent.basic.RecordEmit(ctx, start, emit, msg.Attributes()...)

	// Block timestamp
	m.writeSent.blockTimestamp.Record(ctx, int64(msg.BlockData.BlockTimestamp), attrs)

	// Block number
	blockHeightVal, err := strconv.ParseInt(msg.BlockData.BlockHeight, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse block height: %w", err)
	}
	m.writeSent.blockNumber.Record(ctx, blockHeightVal, attrs)
	return nil
}

func (m *Metrics) OnWriteConfirmed(ctx context.Context, msg *WriteConfirmed, attrKVs ...any) error {
	// Define attributes
	attrs := metric.WithAttributes(msg.Attributes()...)

	// Emit basic metrics (count, timestamps)
	start, emit := msg.ExecutionContext.MetaCapabilityTimestampStart, msg.ExecutionContext.MetaCapabilityTimestampEmit
	m.writeConfirmed.basic.RecordEmit(ctx, start, emit, msg.Attributes()...)

	// Signers number
	m.writeConfirmed.signersNumber.Record(ctx, int64(msg.SignersNum), attrs)

	// Block timestamp
	m.writeConfirmed.blockTimestamp.Record(ctx, int64(msg.BlockData.BlockTimestamp), attrs)

	// Block number
	blockHeightVal, err := strconv.ParseInt(msg.BlockData.BlockHeight, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse block height: %w", err)
	}
	m.writeConfirmed.blockNumber.Record(ctx, blockHeightVal, attrs)
	return nil
}

// Attributes returns the attributes for the WriteInitiated message to be used in metrics
func (m *WriteInitiated) Attributes() []attribute.KeyValue {
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
		attribute.String("node", m.Node),
		attribute.String("forwarder", m.Forwarder),
		attribute.String("receiver", m.Receiver),
		attribute.Int64("report_id", int64(m.ReportId)), // uint32 -> int64
	}

	return append(attrs, context.Attributes()...)
}

// Attributes returns the attributes for the WriteError message to be used in metrics
func (m *WriteError) Attributes() []attribute.KeyValue {
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
		attribute.String("node", m.Node),
		attribute.String("forwarder", m.Forwarder),
		attribute.String("receiver", m.Receiver),
		attribute.Int64("report_id", int64(m.ReportId)), // uint32 -> int64
		// Error information
		attribute.Int64("code", int64(m.Code)), // uint32 -> int64
		attribute.String("summary", m.Summary),
	}

	return append(attrs, context.Attributes()...)
}

// Attributes returns the attributes for the WriteSent message to be used in metrics
func (m *WriteSent) Attributes() []attribute.KeyValue {
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
		attribute.String("node", m.Node),
		attribute.String("forwarder", m.Forwarder),
		attribute.String("receiver", m.Receiver),
		attribute.Int64("report_id", int64(m.ReportId)), // uint32 -> int64
	}

	return append(attrs, context.Attributes()...)
}

// Attributes returns the attributes for the WriteConfirmed message to be used in metrics
func (m *WriteConfirmed) Attributes() []attribute.KeyValue {
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
		attribute.String("node", m.Node),
		attribute.String("forwarder", m.Forwarder),
		attribute.String("receiver", m.Receiver),
		attribute.Int64("report_id", int64(m.ReportId)), // uint32 -> int64
		attribute.String("transmitter", m.Transmitter),
		attribute.Bool("success", m.Success),
		// We mark confrmations by transmitter so we can query for only initial (fast) confirmations
		// with PromQL, and ignore the slower confirmations by other signers for SLA measurements.
		attribute.Bool("observed_by_transmitter", m.Transmitter == m.Node),
	}

	return append(attrs, context.Attributes()...)
}

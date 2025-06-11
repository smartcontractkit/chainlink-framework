//nolint:gosec,errcheck // disable G115, errcheck
package writetarget

import (
	"encoding/hex"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/types"
	monitor "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder"

	commonpb "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/common"
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
)

// messageBuilder is a helper component to build monitoring messages
type messageBuilder struct {
	ChainInfo monitor.ChainInfo
	CapInfo   capabilities.CapabilityInfo
}

// NewMessageBuilder creates a new message builder
func NewMessageBuilder(chainInfo monitor.ChainInfo, capInfo capabilities.CapabilityInfo) *messageBuilder {
	return &messageBuilder{
		ChainInfo: chainInfo,
		CapInfo:   capInfo,
	}
}

// reportInfo contains the report data for the request
type reportInfo struct {
	reportContext []byte
	report        []byte
	signersNum    uint32

	// Decoded report fields
	reportID uint16
}

// requestInfo contains the request data for the capability triggered
type requestInfo struct {
	tsStart int64

	node      string
	forwarder string
	receiver  string

	request                 capabilities.CapabilityRequest
	reportInfo              *reportInfo
	reportTransmissionState *TransmissionState
}

func (m *messageBuilder) buildWriteError(i requestInfo, code uint32, summary, cause string) *wt.WriteError {
	return &wt.WriteError{
		Code:    code,
		Summary: summary,
		Cause:   cause,

		Node:      i.node,
		Forwarder: i.forwarder,
		Receiver:  i.receiver,
		ReportId:  uint32(i.reportInfo.reportID),

		// Execution Context - Source
		ExecutionContext: &commonpb.ExecutionContext{
			MetaSourceId: i.node,

			// Execution Context - Chain
			MetaChainFamilyName: m.ChainInfo.FamilyName,
			MetaChainId:         m.ChainInfo.ChainID,
			MetaNetworkName:     m.ChainInfo.NetworkName,
			MetaNetworkNameFull: m.ChainInfo.NetworkNameFull,

			// Execution Context - Workflow (capabilities.RequestMetadata)
			MetaWorkflowId:               i.request.Metadata.WorkflowID,
			MetaWorkflowOwner:            i.request.Metadata.WorkflowOwner,
			MetaWorkflowExecutionId:      i.request.Metadata.WorkflowExecutionID,
			MetaWorkflowName:             i.request.Metadata.WorkflowName,
			MetaWorkflowDonId:            i.request.Metadata.WorkflowDonID,
			MetaWorkflowDonConfigVersion: i.request.Metadata.WorkflowDonConfigVersion,
			MetaReferenceId:              i.request.Metadata.ReferenceID,

			// Execution Context - Capability
			MetaCapabilityType:           string(m.CapInfo.CapabilityType),
			MetaCapabilityId:             m.CapInfo.ID,
			MetaCapabilityTimestampStart: uint64(i.tsStart),
			MetaCapabilityTimestampEmit:  uint64(time.Now().UnixMilli()),
		},
	}
}

func (m *messageBuilder) buildWriteInitiated(i requestInfo) *wt.WriteInitiated {
	return &wt.WriteInitiated{
		Node:      i.node,
		Forwarder: i.forwarder,
		Receiver:  i.receiver,
		ReportId:  uint32(i.reportInfo.reportID),

		ExecutionContext: &commonpb.ExecutionContext{
			// Execution Context - Source
			MetaSourceId: i.node,

			// Execution Context - Chain
			MetaChainFamilyName: m.ChainInfo.FamilyName,
			MetaChainId:         m.ChainInfo.ChainID,
			MetaNetworkName:     m.ChainInfo.NetworkName,
			MetaNetworkNameFull: m.ChainInfo.NetworkNameFull,

			// Execution Context - Workflow (capabilities.RequestMetadata)
			MetaWorkflowId:               i.request.Metadata.WorkflowID,
			MetaWorkflowOwner:            i.request.Metadata.WorkflowOwner,
			MetaWorkflowExecutionId:      i.request.Metadata.WorkflowExecutionID,
			MetaWorkflowName:             i.request.Metadata.WorkflowName,
			MetaWorkflowDonId:            i.request.Metadata.WorkflowDonID,
			MetaWorkflowDonConfigVersion: i.request.Metadata.WorkflowDonConfigVersion,
			MetaReferenceId:              i.request.Metadata.ReferenceID,

			// Execution Context - Capability
			MetaCapabilityType:           string(m.CapInfo.CapabilityType),
			MetaCapabilityId:             m.CapInfo.ID,
			MetaCapabilityTimestampStart: uint64(i.tsStart),
			MetaCapabilityTimestampEmit:  uint64(time.Now().UnixMilli()),
		},
	}
}

func (m *messageBuilder) buildWriteSkipped(i requestInfo, reason string) *wt.WriteSkipped {
	return &wt.WriteSkipped{
		Node:      i.node,
		Forwarder: i.forwarder,
		Receiver:  i.receiver,
		ReportId:  uint32(i.reportInfo.reportID),
		Reason:    reason,

		ExecutionContext: &commonpb.ExecutionContext{
			// Execution Context - Source
			MetaSourceId: i.node,

			// Execution Context - Chain
			MetaChainFamilyName: m.ChainInfo.FamilyName,
			MetaChainId:         m.ChainInfo.ChainID,
			MetaNetworkName:     m.ChainInfo.NetworkName,
			MetaNetworkNameFull: m.ChainInfo.NetworkNameFull,

			// Execution Context - Workflow (capabilities.RequestMetadata)
			MetaWorkflowId:               i.request.Metadata.WorkflowID,
			MetaWorkflowOwner:            i.request.Metadata.WorkflowOwner,
			MetaWorkflowExecutionId:      i.request.Metadata.WorkflowExecutionID,
			MetaWorkflowName:             i.request.Metadata.WorkflowName,
			MetaWorkflowDonId:            i.request.Metadata.WorkflowDonID,
			MetaWorkflowDonConfigVersion: i.request.Metadata.WorkflowDonConfigVersion,
			MetaReferenceId:              i.request.Metadata.ReferenceID,

			// Execution Context - Capability
			MetaCapabilityType:           string(m.CapInfo.CapabilityType),
			MetaCapabilityId:             m.CapInfo.ID,
			MetaCapabilityTimestampStart: uint64(i.tsStart),
			MetaCapabilityTimestampEmit:  uint64(time.Now().UnixMilli()),
		},
	}
}

func (m *messageBuilder) buildWriteSent(i requestInfo, head types.Head, txID string) *wt.WriteSent {
	return &wt.WriteSent{
		Node:      i.node,
		Forwarder: i.forwarder,
		Receiver:  i.receiver,
		ReportId:  uint32(i.reportInfo.reportID),

		TxId: txID,

		BlockData: &commonpb.BlockData{
			BlockHash:      hex.EncodeToString(head.Hash),
			BlockHeight:    head.Height,
			BlockTimestamp: head.Timestamp * 1000, // convert to milliseconds
		},

		ExecutionContext: &commonpb.ExecutionContext{

			// Execution Context - Source
			MetaSourceId: i.node,

			// Execution Context - Chain
			MetaChainFamilyName: m.ChainInfo.FamilyName,
			MetaChainId:         m.ChainInfo.ChainID,
			MetaNetworkName:     m.ChainInfo.NetworkName,
			MetaNetworkNameFull: m.ChainInfo.NetworkNameFull,

			// Execution Context - Workflow (capabilities.RequestMetadata)
			MetaWorkflowId:               i.request.Metadata.WorkflowID,
			MetaWorkflowOwner:            i.request.Metadata.WorkflowOwner,
			MetaWorkflowExecutionId:      i.request.Metadata.WorkflowExecutionID,
			MetaWorkflowName:             i.request.Metadata.WorkflowName,
			MetaWorkflowDonId:            i.request.Metadata.WorkflowDonID,
			MetaWorkflowDonConfigVersion: i.request.Metadata.WorkflowDonConfigVersion,
			MetaReferenceId:              i.request.Metadata.ReferenceID,

			// Execution Context - Capability
			MetaCapabilityType:           string(m.CapInfo.CapabilityType),
			MetaCapabilityId:             m.CapInfo.ID,
			MetaCapabilityTimestampStart: uint64(i.tsStart),
			MetaCapabilityTimestampEmit:  uint64(time.Now().UnixMilli()),
		},
	}
}

func (m *messageBuilder) buildWriteConfirmed(i requestInfo, head types.Head) *wt.WriteConfirmed {
	rcfg := ReqConfig{}
	processor := ""
	reqConfig := i.request.Config
	if reqConfig != nil {
		// ignore errors and use default values
		reqConfig.UnwrapTo(&rcfg)
		processor = rcfg.Processor
	}

	return &wt.WriteConfirmed{
		Node:      i.node,
		Forwarder: i.forwarder,
		Receiver:  i.receiver,

		ReportId:      uint32(i.reportInfo.reportID),
		ReportContext: i.reportInfo.reportContext,
		Report:        i.reportInfo.report,
		SignersNum:    i.reportInfo.signersNum,

		BlockData: &commonpb.BlockData{
			BlockHash:      hex.EncodeToString(head.Hash),
			BlockHeight:    head.Height,
			BlockTimestamp: head.Timestamp * 1000, // convert to milliseconds
		},

		// Transmission Info
		Transmitter: i.reportTransmissionState.Transmitter,
		Success:     i.reportTransmissionState.Status == TransmissionStateSucceeded,

		ExecutionContext: &commonpb.ExecutionContext{
			// Execution Context - Source
			MetaSourceId: i.node,

			// Execution Context - Chain
			MetaChainFamilyName: m.ChainInfo.FamilyName,
			MetaChainId:         m.ChainInfo.ChainID,
			MetaNetworkName:     m.ChainInfo.NetworkName,
			MetaNetworkNameFull: m.ChainInfo.NetworkNameFull,

			// Execution Context - Workflow (capabilities.RequestMetadata)
			MetaWorkflowId:               i.request.Metadata.WorkflowID,
			MetaWorkflowOwner:            i.request.Metadata.WorkflowOwner,
			MetaWorkflowExecutionId:      i.request.Metadata.WorkflowExecutionID,
			MetaWorkflowName:             i.request.Metadata.WorkflowName,
			MetaWorkflowDonId:            i.request.Metadata.WorkflowDonID,
			MetaWorkflowDonConfigVersion: i.request.Metadata.WorkflowDonConfigVersion,
			MetaReferenceId:              i.request.Metadata.ReferenceID,

			// Execution Context - Capability
			MetaCapabilityType:           string(m.CapInfo.CapabilityType),
			MetaCapabilityId:             m.CapInfo.ID,
			MetaCapabilityTimestampStart: uint64(i.tsStart),
			MetaCapabilityTimestampEmit:  uint64(time.Now().UnixMilli()),
		},
		MetaCapabilityProcessor: processor,
	}
}

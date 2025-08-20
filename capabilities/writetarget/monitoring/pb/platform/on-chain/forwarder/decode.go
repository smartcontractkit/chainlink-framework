package forwarder

import (
	"fmt"

	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform"

	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/common"
	wt_msg "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
)

// DecodeAsReportProcessed decodes a 'platform.write-target.WriteConfirmed' message
// as a 'keytone.forwarder.ReportProcessed' message
func DecodeAsReportProcessed(m *wt_msg.WriteConfirmed) (*ReportProcessed, error) {
	// Decode the confirmed report (WT -> platform forwarder contract event akka. Keystone)
	r, err := platform.Decode(m.Report)
	if err != nil {
		return nil, fmt.Errorf("failed to decode report: %w", err)
	}

	return &ReportProcessed{
		// Event data
		Receiver:            m.Receiver,
		WorkflowExecutionId: r.ExecutionID,
		ReportId:            m.ReportId,
		Success:             m.Success,

		BlockData: m.BlockData,

		// Transaction data - info about the tx that mained the event (optional)
		// Notice: we skip SOME head/tx data here (unknown), as we map from 'platform.write-target.WriteConfirmed'
		// and not from tx/event data (e.g., 'platform.write-target.WriteTxConfirmed')
		TransactionData: &common.TransactionData{
			TxSender:   m.Transmitter,
			TxReceiver: m.Forwarder,
		},

		ExecutionContext: m.ExecutionContext,
	}, nil
}

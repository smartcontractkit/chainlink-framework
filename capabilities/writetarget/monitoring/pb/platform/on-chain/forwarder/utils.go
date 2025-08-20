package forwarder

import (
	"testing"

	ocr3types "github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform"
)

func NewTestReport(t *testing.T, data []byte) ([]byte, error) {
	metadata := ocr3types.Metadata{
		Version:          1,
		ExecutionID:      "0102030405060708090a0b0c0d0e0f1000000000000000000000000000000000",
		Timestamp:        1620000000,
		DONID:            1,
		DONConfigVersion: 1,
		WorkflowID:       "1234567890123456789012345678901234567890123456789012345678901234",
		WorkflowName:     "12",
		WorkflowOwner:    "1234567890123456789012345678901234567890",
		ReportID:         "1234",
	}
	report := platform.Report{
		Metadata: metadata,
		Data:     data,
	}
	encoded, err := report.Encode()
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

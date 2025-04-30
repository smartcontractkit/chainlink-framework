package monitor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"

	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform/on-chain/forwarder"
)

func TestToSchemaPath(t *testing.T) {
	tests := []struct {
		input    proto.Message
		expected string
	}{
		{
			input:    &wt.WriteInitiated{},
			expected: "/<base-path>/platform/write-target/write_initiated.proto",
		},
		{
			input:    &wt.WriteError{},
			expected: "/<base-path>/platform/write-target/write_error.proto",
		},
		{
			input:    &wt.WriteSent{},
			expected: "/<base-path>/platform/write-target/write_sent.proto",
		},
		{
			input:    &wt.WriteConfirmed{},
			expected: "/<base-path>/platform/write-target/write_confirmed.proto",
		},
		{
			input:    &forwarder.ReportProcessed{},
			expected: "/<base-path>/platform/on-chain/forwarder/report_processed.proto",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := toSchemaPath(tt.input, "/<base-path>")
			assert.Equal(t, tt.expected, result)
		})
	}
}

package writetarget_test

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"

	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder/mocks"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/common"
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
)

func TestWriteTargetMonitor(t *testing.T) {
	processor := mocks.NewProtoProcessor(t)
	lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)

	m, err := writetarget.NewMonitor(writetarget.MonitorOpts{
		lggr,
		map[string]beholder.ProtoProcessor{"test": processor},
		[]string{},
		writetarget.NewMonitorEmitter(lggr),
	})
	require.NoError(t, err)

	encoded := []byte{}

	msg := &wt.WriteConfirmed{
		BlockData: &common.BlockData{
			BlockHeight: "10",
		},
		MetaCapabilityProcessor: "test",
		Report:                  encoded,
	}

	processor.On("Process", t.Context(), msg, mock.Anything).Return(nil).Once()
	err = m.ProtoEmitter.EmitWithLog(t.Context(), msg)
	require.NoError(t, err)

	m, err = writetarget.NewMonitor(writetarget.MonitorOpts{lggr, map[string]beholder.ProtoProcessor{"other": processor}, []string{}, writetarget.NewMonitorEmitter(lggr)})

	err = m.ProtoEmitter.EmitWithLog(t.Context(), msg)
	require.NoError(t, err)

	tests.RequireLogMessage(t, observed, "no matching processor for MetaCapabilityProcessor=test")

	// get new processor
	processor = mocks.NewProtoProcessor(t)
	msg.MetaCapabilityProcessor = ""
	processor.AssertNotCalled(t, "Process", mock.Anything, mock.Anything, mock.Anything)

	err = m.ProtoEmitter.EmitWithLog(t.Context(), msg)
	require.NoError(t, err)

	tests.RequireLogMessage(t, observed, "No product specific processor specified; skipping.")

	m, err = writetarget.NewMonitor(writetarget.MonitorOpts{lggr, map[string]beholder.ProtoProcessor{}, []string{"other"}, writetarget.NewMonitorEmitter(lggr)})

	err = m.ProtoEmitter.EmitWithLog(t.Context(), msg)
	require.NoError(t, err)

	tests.RequireLogMessage(t, observed, "no required processor with name other")

}

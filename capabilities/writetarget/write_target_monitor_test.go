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
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
)

func TestWriteTargetMonitor(t *testing.T) {
	processor := mocks.NewProtoProcessor(t)
	lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)

	m, err := writetarget.NewMonitor(writetarget.MonitorOpts{
		lggr,
		[]beholder.ProtoProcessor{},
		map[string]beholder.ProtoProcessor{"test": processor},
		writetarget.NewMonitorEmitter(lggr),
	})
	require.NoError(t, err)

	encoded := []byte{}

	msg := &wt.WriteConfirmed{
		BlockHeight:             "10",
		MetaCapabilityProcessor: "test",
		Report:                  encoded,
	}

	t.Run("Uses processor when name equals config", func(t *testing.T) {
		processor.On("Process", t.Context(), msg, mock.Anything).Return(nil).Once()
		err = m.ProtoEmitter.EmitWithLog(t.Context(), msg)
		require.NoError(t, err)
	})

	t.Run("Logs when config name is not found", func(t *testing.T) {
		m, err = writetarget.NewMonitor(writetarget.MonitorOpts{lggr, []beholder.ProtoProcessor{}, map[string]beholder.ProtoProcessor{"other": processor}, writetarget.NewMonitorEmitter(lggr)})

		err = m.ProtoEmitter.EmitWithLog(t.Context(), msg)
		require.NoError(t, err)

		tests.RequireLogMessage(t, observed, "no matching processor for MetaCapabilityProcessor=test")
	})

	t.Run("Does not use processor when none is configured", func(t *testing.T) {
		// get new processor
		processor = mocks.NewProtoProcessor(t)
		msg.MetaCapabilityProcessor = ""
		processor.AssertNotCalled(t, "Process", mock.Anything, mock.Anything, mock.Anything)

		err := m.ProtoEmitter.EmitWithLog(t.Context(), msg)
		require.NoError(t, err)

		tests.RequireLogMessage(t, observed, "No product specific processor specified; skipping.")
	})
}

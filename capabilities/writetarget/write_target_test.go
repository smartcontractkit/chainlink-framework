package writetarget_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	commonconfig "github.com/smartcontractkit/chainlink-common/pkg/config"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/values"

	writetarget "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	monitor "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder/monitor"
	utils "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform/on-chain/forwarder"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform/processor"

	monmocks "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder/monitor/mocks"
	wtmocks "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/mocks"
)

// setupWriteTarget builds the WriteTarget and a matching CapabilityRequest with a valid signed report
func setupWriteTarget(
	t *testing.T,
	lggr logger.Logger,
	strategy *wtmocks.TargetStrategy,
	chainSvc *wtmocks.ChainService,
	productSpecificProcessor bool,
	emitter monitor.ProtoEmitter,
) (capabilities.ExecutableCapability, capabilities.CapabilityRequest) {
	platformProcessors, err := processor.NewPlatformProcessors(emitter)
	require.NoError(t, err)

	var psp []writetarget.ProductSpecificProcessor
	if productSpecificProcessor {
		psp = []writetarget.ProductSpecificProcessor{newMockProductSpecificProcessor(t)}
	}
	monClient, err := writetarget.NewMonitor(lggr, platformProcessors, psp, emitter)
	require.NoError(t, err)

	pollPeriod, err := commonconfig.NewDuration(2 * time.Second)
	require.NoError(t, err)

	timeout, err := commonconfig.NewDuration(30 * time.Second)
	require.NoError(t, err)

	opts := writetarget.WriteTargetOpts{
		ID: "write_generic-testnet@1.0.0",
		Config: writetarget.Config{
			ConfirmerPollPeriod: pollPeriod,
			ConfirmerTimeout:    timeout,
		},
		ChainInfo:        monitor.ChainInfo{},
		Logger:           lggr,
		Beholder:         monClient,
		ChainService:     chainSvc,
		ContractReader:   nil,
		EVMService:       nil,
		ConfigValidateFn: func(_ capabilities.CapabilityRequest) (string, error) { return "0xreceiver", nil },
		NodeAddress:      "0xnode",
		ForwarderAddress: "0xforwarder",
		TargetStrategy:   strategy,
	}
	wt := writetarget.NewWriteTarget(opts)

	// generate a valid on-chain report and metadata
	report, err := utils.NewTestReport(t, []byte{})
	require.NoError(t, err)
	reportCtx := []byte{42}
	sigs := [][]byte{{1, 2, 3}}

	inputs, err := values.NewMap(map[string]any{
		"signed_report": map[string]any{
			"id":         report[:2],
			"report":     report,
			"context":    reportCtx,
			"signatures": sigs,
		},
	})
	require.NoError(t, err)

	req := capabilities.CapabilityRequest{Inputs: inputs}

	// match decoded report to request metadata
	repDecoded, err := platform.Decode(report)
	require.NoError(t, err)
	req.Metadata = capabilities.RequestMetadata{
		WorkflowID:          repDecoded.WorkflowID,
		WorkflowOwner:       repDecoded.WorkflowOwner,
		WorkflowName:        repDecoded.WorkflowName,
		WorkflowExecutionID: repDecoded.ExecutionID,
	}

	cfg, err := values.NewMap(map[string]any{"address": "0x1", "processor": "test"})
	require.NoError(t, err)
	req.Config = cfg

	return wt, req
}

func findLogMatch(t *testing.T, observed *observer.ObservedLogs, msg string, key string, value string) {
	require.Eventually(t, func() bool {
		filteredByMsg := observed.FilterMessage(msg)
		matches := filteredByMsg.
			Filter(func(le observer.LoggedEntry) bool {
				for _, field := range le.Context {
					if field.Key == key &&
						strings.Contains(fmt.Sprint(field.Interface),
							value) {
						return true
					}
				}
				return false
			}).
			All() // => []observer.LoggedEntry
		return len(matches) > 0
	}, 30*time.Second, 1*time.Second)
}

func newMockProductSpecificProcessor(t *testing.T) writetarget.ProductSpecificProcessor {
	processor := wtmocks.NewProductSpecificProcessor(t)
	processor.EXPECT().Name().Return("test").Once()
	processor.EXPECT().Process(mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
	return processor
}

func TestWriteTarget_Execute(t *testing.T) {
	cases := []struct {
		name                     string
		headHeight               string
		state                    writetarget.TransmissionState
		expectTransmit           bool
		simulateTxError          bool
		expectError              bool
		errorContains            string
		productSpecificProcessor bool
	}{
		{
			name:           "not attempted",
			headHeight:     "100",
			state:          writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			expectTransmit: true,
			expectError:    false,
		},
		{
			name:           "already succeeded",
			headHeight:     "200",
			state:          writetarget.TransmissionState{Status: writetarget.TransmissionStateSucceeded},
			expectTransmit: false,
			expectError:    false,
		},
		{
			name:                     "already succeeded with product specific processor",
			headHeight:               "200",
			state:                    writetarget.TransmissionState{Status: writetarget.TransmissionStateSucceeded},
			expectTransmit:           false,
			expectError:              false,
			productSpecificProcessor: true,
		},
		{
			name:           "fatal state",
			headHeight:     "300",
			state:          writetarget.TransmissionState{Status: writetarget.TransmissionStateFatal, Err: errors.New("fatal")},
			expectTransmit: false,
			expectError:    true,
			errorContains:  "Transmission attempt fatal",
		},
		{
			name:            "transmit error",
			headHeight:      "400",
			state:           writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			expectTransmit:  true,
			simulateTxError: true,
			expectError:     true,
			errorContains:   "failed to transmit the report",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)

			// TODO: replace with real emitter
			emitter := monmocks.NewProtoEmitter(t)

			strategy := wtmocks.NewTargetStrategy(t)
			strategy.EXPECT().QueryTransmissionState(mock.Anything, mock.Anything, mock.Anything).
				Return(&tc.state, nil)

			// Ensure the correct beholder messages are emitted for each case
			emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteInitiated"), mock.Anything).Return(nil).Once()
			if tc.expectError {
				emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteError"), mock.Anything).Return(nil).Once()
			} else {
				if tc.expectTransmit {
					emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteSent"), mock.Anything).Return(nil).Once()
					strategy.EXPECT().GetTransactionStatus(mock.Anything, mock.Anything).Return(commontypes.Finalized, nil)
				}
				emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteConfirmed"), mock.Anything).Return(nil).Once()
				emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*forwarder.ReportProcessed"), mock.Anything).Return(nil).Once()
			}

			chainSvc := wtmocks.NewChainService(t)
			chainSvc.EXPECT().LatestHead(mock.Anything).
				Return(commontypes.Head{Height: tc.headHeight}, nil)

			if tc.expectTransmit {
				ex := strategy.EXPECT().TransmitReport(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				if tc.simulateTxError {
					ex.Return("", errors.New("transmit fail"))
				} else {
					ex.Return("tx123", nil)
				}
			}

			target, req := setupWriteTarget(t, lggr, strategy, chainSvc, tc.productSpecificProcessor, emitter)

			resp, err := target.Execute(context.Background(), req)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)

				// TODO: remove next 2 lines and enable once synchronous waiting is implemented
				_ = observed
				time.Sleep(1 * time.Second)
				// if tc.productSpecificProcessor == nil {
				// 	findLogMatch(t, observed, "failed to emit write confirmed", "err", "no matching processor for MetaCapabilityProcessor")
				// }
			}
		})
	}

	// additional edge cases
	t.Run("missing signed report", func(t *testing.T) {
		strategy := wtmocks.NewTargetStrategy(t)
		chainSvc := wtmocks.NewChainService(t)
		emitter := monmocks.NewProtoEmitter(t)
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.Anything, mock.Anything).Return(nil)

		target, _ := setupWriteTarget(t, logger.Test(t), strategy, chainSvc, false, emitter)

		inputs, _ := values.NewMap(map[string]any{})
		config, _ := values.NewMap(map[string]any{"address": "x", "processor": "y"})
		req := capabilities.CapabilityRequest{Inputs: inputs, Config: config}

		_, err := target.Execute(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "input missing required field")
	})

	t.Run("invalid config", func(t *testing.T) {
		// override ConfigValidateFn
		lggr := logger.Test(t)
		emitter := monmocks.NewProtoEmitter(t)
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.Anything, mock.Anything).Return(nil)
		monClient, _ := writetarget.NewMonitor(lggr, nil, nil, emitter)
		chainSvc := wtmocks.NewChainService(t)
		strategy := wtmocks.NewTargetStrategy(t)

		opts := writetarget.WriteTargetOpts{
			ID:               "test@1.0.0",
			Config:           writetarget.Config{},
			ChainInfo:        monitor.ChainInfo{},
			Logger:           lggr,
			Beholder:         monClient,
			ChainService:     chainSvc,
			ContractReader:   nil,
			EVMService:       nil,
			ConfigValidateFn: func(_ capabilities.CapabilityRequest) (string, error) { return "", errors.New("bad cfg") },
			NodeAddress:      "x",
			ForwarderAddress: "y",
			TargetStrategy:   strategy,
		}
		target := writetarget.NewWriteTarget(opts)

		inputs, _ := values.NewMap(map[string]any{"signed_report": map[string]any{"id": []byte{0}, "report": []byte{0}, "context": []byte{}, "signatures": [][]byte{}}})
		config, _ := values.NewMap(map[string]any{"address": "x", "processor": "y"})
		req := capabilities.CapabilityRequest{Inputs: inputs, Config: config}

		_, err := target.Execute(context.Background(), req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to validate config")
	})
}

func TestNewWriteTargetID(t *testing.T) {
	tests := []struct {
		name            string
		chainFamilyName string
		networkName     string
		chainID         string
		version         string
		expected        string
		expectError     bool
	}{
		{
			name:            "Valid input with network name",
			chainFamilyName: "aptos",
			networkName:     "mainnet",
			chainID:         "1",
			version:         "1.0.0",
			expected:        "write_aptos-mainnet@1.0.0",
			expectError:     false,
		},
		{
			name:            "Valid input without network name",
			chainFamilyName: "aptos",
			networkName:     "",
			chainID:         "1",
			version:         "1.0.0",
			expected:        "write_aptos-1@1.0.0",
			expectError:     false,
		},
		{
			name:            "Valid input with empty chainFamilyName",
			chainFamilyName: "",
			networkName:     "ethereum-mainnet",
			chainID:         "1",
			version:         "1.0.0",
			expected:        "write_ethereum-mainnet@1.0.0",
			expectError:     false,
		},
		{
			name:            "Invalid input with empty version",
			chainFamilyName: "aptos",
			networkName:     "mainnet",
			chainID:         "1",
			version:         "",
			expected:        "",
			expectError:     true,
		},
		{
			name:            "Invalid input with empty networkName and chainID",
			chainFamilyName: "aptos",
			networkName:     "",
			chainID:         "",
			version:         "2.0.0",
			expected:        "",
			expectError:     true,
		},
		{
			name:            "Valid input with unknown network name",
			chainFamilyName: "aptos",
			networkName:     "unknown",
			chainID:         "1",
			version:         "2.0.1",
			expected:        "write_aptos-1@2.0.1",
			expectError:     false,
		},
		{
			name:            "Valid input with network name (testnet)",
			chainFamilyName: "aptos",
			networkName:     "testnet",
			chainID:         "2",
			version:         "1.0.3",
			expected:        "write_aptos-testnet@1.0.3",
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := writetarget.NewWriteTargetID(tt.chainFamilyName, tt.networkName, tt.chainID, tt.version)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

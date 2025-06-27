package writetarget_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-common/pkg/values"

	writetarget "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
	monitor "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder"
	utils "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform/on-chain/forwarder"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform/processor"

	monmocks "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder/mocks"
	wtmocks "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/mocks"
)

// setupWriteTarget builds the WriteTarget and a matching CapabilityRequest with a valid signed report
func setupWriteTarget(
	t *testing.T,
	lggr logger.Logger,
	strategy *wtmocks.TargetStrategy,
	chainSvc *wtmocks.ChainService,
	productSpecificProcessor bool,
	emitter beholder.ProtoEmitter,
	spendLimits []capabilities.SpendLimit,
) (capabilities.ExecutableCapability, capabilities.CapabilityRequest) {
	platformProcessors, err := processor.NewPlatformProcessors(emitter)
	require.NoError(t, err)

	if productSpecificProcessor {
		platformProcessors["test"] = newMockProductSpecificProcessor(t)
	}
	// Always add a mock for the default processor (writetarget) to prevent nil pointer dereference
	platformProcessors["writetarget"] = newMockProductSpecificProcessor(t)
	monClient, err := writetarget.NewMonitor(writetarget.MonitorOpts{lggr, platformProcessors, processor.PlatformDefaultProcessors, emitter})
	require.NoError(t, err)

	pollPeriod := 100 * time.Millisecond

	timeout := 500 * time.Millisecond

	opts := writetarget.WriteTargetOpts{
		ID: "write_generic-testnet@1.0.0",
		Config: writetarget.Config{
			PollPeriod:        pollPeriod,
			AcceptanceTimeout: timeout,
		},
		ChainInfo:            monitor.ChainInfo{ChainID: "1"},
		Logger:               lggr,
		Beholder:             monClient,
		ChainService:         chainSvc,
		ConfigValidateFn:     func(_ capabilities.CapabilityRequest) (string, error) { return "0xreceiver", nil },
		NodeAddress:          "0xnode",
		ForwarderAddress:     "0xforwarder",
		TargetStrategy:       strategy,
		WriteAcceptanceState: commontypes.Finalized,
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
		SpendLimits:         spendLimits,
	}

	cfg, err := values.NewMap(map[string]any{"address": "0x1", "processor": "test"})
	require.NoError(t, err)
	req.Config = cfg

	return wt, req
}

func newMockProductSpecificProcessor(t *testing.T) beholder.ProtoProcessor {
	processor := monmocks.NewProtoProcessor(t)
	// Handle both 3-arg and 4-arg Process calls
	processor.EXPECT().Process(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	processor.EXPECT().Process(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return processor
}

type testCase struct {
	name                     string
	initialTransmissionState writetarget.TransmissionState
	txState                  commontypes.TransactionStatus
	simulateTxError          bool
	expectError              bool
	errorContains            string
	productSpecificProcessor bool
	requiredLogMessage       string
	// Gas estimation and transaction fee fields
	spendLimits          []capabilities.SpendLimit
	gasEstimateError     error
	gasEstimateFee       *commontypes.EstimateFee
	transactionFeeError  error
	transactionFee       decimal.Decimal
	expectTransactionFee bool
}

func TestWriteTarget_Execute(t *testing.T) {
	cases := []testCase{
		{
			name:                     "succeeds transmission state is not attempted",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Finalized,
			expectError:              false,
			requiredLogMessage:       "no matching processor for MetaCapabilityProcessor=test",
			transactionFee:           decimal.NewFromFloat(0.0001),
			expectTransactionFee:     true,
		},
		{
			name:                     "succeeds transmission state is already succeeded",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateSucceeded},
			txState:                  commontypes.Unknown,
			expectError:              false,
		},
		{
			name:                     "succeeds transmission state is not attempted with product specific processor",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Finalized,
			expectError:              false,
			productSpecificProcessor: true,
			requiredLogMessage:       "confirmed - transmission state visible",
			transactionFee:           decimal.NewFromFloat(0.0001),
			expectTransactionFee:     true,
		},
		{
			name:                     "already succeeded with product specific processor",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateSucceeded},
			txState:                  commontypes.Unknown,
			expectError:              false,
			productSpecificProcessor: true,
		},
		{
			name:                     "fatal initialTransmissionState",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateFatal, Err: errors.New("fatal")},
			txState:                  commontypes.Unknown,
			expectError:              true,
			errorContains:            "Transmission attempt fatal",
		},
		{
			name:                     "transmit error",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Pending,
			simulateTxError:          true,
			expectError:              true,
			errorContains:            "failed to transmit the report",
		},
		{
			name:                     "Returns error if tx is not finalized before timeout",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Pending,
			simulateTxError:          false,
			expectError:              true,
			errorContains:            "context deadline exceeded",
		},
		{
			name:                     "Returns error if tx is finalized but no report is on-chain",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Finalized,
			simulateTxError:          false,
			expectError:              true,
			errorContains:            "platform.write_target.WriteError [ERR-0] - write confirmation - failed: transaction was finalized, but report was not observed on chain before timeout",
		},
		{
			name:                     "Returns error if tx is fatal but no report is on-chain",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Fatal,
			simulateTxError:          false,
			expectError:              true,
			errorContains:            "platform.write_target.WriteError [ERR-0] - write confirmation - failed: transaction failed and no other node managed to get report on chain before timeout",
		},
		{
			name:                     "Returns error if tx is failed but no report is on-chain",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Failed,
			simulateTxError:          false,
			expectError:              true,
			errorContains:            "platform.write_target.WriteError [ERR-0] - write confirmation - failed: transaction failed and no other node managed to get report on chain before timeout",
		},
		{
			name:                     "Returns success if report is on-chain but tx is fatal",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Fatal,
			simulateTxError:          false,
			expectError:              false,
			requiredLogMessage:       "confirmed - transmission state visible but submitted by another node. This node's tx failed",
			transactionFee:           decimal.NewFromFloat(0.0001),
			expectTransactionFee:     true,
		},
		{
			name:                     "Returns success if report is on-chain but tx is failed",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Failed,
			simulateTxError:          false,
			expectError:              false,
			requiredLogMessage:       "confirmed - transmission state visible but submitted by another node. This node's tx failed",
			transactionFee:           decimal.NewFromFloat(0.0001),
			expectTransactionFee:     true,
		},
		// Gas estimation and transaction fee test cases
		{
			name:                     "succeeds when no spend limit is specified",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Finalized,
			expectError:              false,
			spendLimits:              []capabilities.SpendLimit{},
			transactionFee:           decimal.NewFromFloat(0.0005),
			expectTransactionFee:     true,
			requiredLogMessage:       "No gas spend limit found, skipping gas estimation",
		},
		{
			name:                     "fails when gas estimate exceeds spend limit",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Unknown,
			expectError:              true,
			errorContains:            "InsufficientFunds",
			spendLimits: []capabilities.SpendLimit{
				{SpendType: "GAS.1", Limit: "0.001"},
			},
			gasEstimateFee: &commontypes.EstimateFee{
				Fee:      big.NewInt(2000000000000000), // 0.002 ETH in wei (exceeds limit)
				Decimals: 18,
			},
		},
		{
			name:                     "succeeds when gas estimate is within spend limit and includes transaction fee",
			initialTransmissionState: writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted},
			txState:                  commontypes.Finalized,
			expectError:              false,
			spendLimits: []capabilities.SpendLimit{
				{SpendType: "GAS.1", Limit: "0.001"},
			},
			gasEstimateFee: &commontypes.EstimateFee{
				Fee:      big.NewInt(500000000000000), // 0.0005 ETH in wei (within limit)
				Decimals: 18,
			},
			transactionFee:       decimal.NewFromFloat(0.0005),
			expectTransactionFee: true,
			requiredLogMessage:   "confirmed - transmission state visible",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			lggr, observed := logger.TestObserved(t, zapcore.DebugLevel)
			emitter := monmocks.NewProtoEmitter(t)
			strategy := wtmocks.NewTargetStrategy(t)

			mockTransmissionState(tc, strategy)
			mockBeholderMessages(tc, emitter)
			mockTransmit(tc, strategy, emitter)
			mockGasEstimation(tc, strategy)
			mockTransactionFee(tc, strategy)

			chainSvc := wtmocks.NewChainService(t)
			// Only set up LatestHead mock if gas estimation doesn't fail
			if !(tc.expectError && len(tc.spendLimits) > 0 && tc.gasEstimateFee != nil && tc.gasEstimateFee.Fee.Cmp(big.NewInt(1000000000000000)) > 0) {
				chainSvc.EXPECT().LatestHead(mock.Anything).
					Return(commontypes.Head{Height: "100"}, nil)
			}

			target, req := setupWriteTarget(t, lggr, strategy, chainSvc, tc.productSpecificProcessor, emitter, tc.spendLimits)

			resp, err := target.Execute(t.Context(), req)
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errorContains)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, resp)

				// Verify transaction fee in response metadata for successful cases
				if tc.expectTransactionFee && tc.transactionFeeError == nil {
					require.NotEmpty(t, resp.Metadata.Metering)
					require.Equal(t, "ignored_by_engine", resp.Metadata.Metering[0].Peer2PeerID)
					require.Equal(t, "GAS.1", resp.Metadata.Metering[0].SpendUnit)
					require.Equal(t, tc.transactionFee.String(), resp.Metadata.Metering[0].SpendValue)
				}
			}

			if tc.requiredLogMessage != "" {
				tests.RequireLogMessage(t, observed, tc.requiredLogMessage)
				// return len(observed.FilterMessage("no matching processor for MetaCapabilityProcessor=test").All()) > 0
			}
		})
	}

	// additional edge cases
	t.Run("missing signed report", func(t *testing.T) {
		strategy := wtmocks.NewTargetStrategy(t)
		chainSvc := wtmocks.NewChainService(t)
		emitter := monmocks.NewProtoEmitter(t)
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.Anything, mock.Anything).Return(nil)

		target, _ := setupWriteTarget(t, logger.Test(t), strategy, chainSvc, false, emitter, nil)

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
		monClient, _ := writetarget.NewMonitor(writetarget.MonitorOpts{lggr, nil, nil, emitter})
		chainSvc := wtmocks.NewChainService(t)
		strategy := wtmocks.NewTargetStrategy(t)

		opts := writetarget.WriteTargetOpts{
			ID:               "test@1.0.0",
			Config:           writetarget.Config{},
			ChainInfo:        monitor.ChainInfo{},
			Logger:           lggr,
			Beholder:         monClient,
			ChainService:     chainSvc,
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

func mockTransmissionState(tc testCase, strategy *wtmocks.TargetStrategy) {
	// Skip transmission state mocks if gas estimation fails
	if tc.expectError && len(tc.spendLimits) > 0 && tc.gasEstimateFee != nil && tc.gasEstimateFee.Fee.Cmp(big.NewInt(1000000000000000)) > 0 {
		return
	}

	// initial query for transmission state
	strategy.EXPECT().QueryTransmissionState(mock.Anything, mock.Anything, mock.Anything).
		Return(&tc.initialTransmissionState, nil).Once()

	// subsequent queries for transaction state (if applicable)
	if tc.expectError {
		strategy.EXPECT().QueryTransmissionState(mock.Anything, mock.Anything, mock.Anything).
			Return(&writetarget.TransmissionState{Status: writetarget.TransmissionStateNotAttempted}, nil).Maybe()
	} else {
		strategy.EXPECT().QueryTransmissionState(mock.Anything, mock.Anything, mock.Anything).
			Return(&writetarget.TransmissionState{Status: writetarget.TransmissionStateSucceeded}, nil).Maybe()
	}
}

func mockBeholderMessages(tc testCase, emitter *monmocks.ProtoEmitter) {
	// For gas estimation errors, only expect WriteError (no WriteInitiated)
	if tc.expectError && len(tc.spendLimits) > 0 && tc.gasEstimateFee != nil && tc.gasEstimateFee.Fee.Cmp(big.NewInt(1000000000000000)) > 0 {
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteError"), mock.Anything, mock.Anything).Return(nil).Once()
		return
	}

	// Ensure the correct beholder messages are emitted for each case
	emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteInitiated"), mock.Anything).Return(nil).Once()
	if tc.expectError {
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteError"), mock.Anything).Return(nil).Once()
	} else {
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteConfirmed"), mock.Anything).Return(nil).Once()
		emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*forwarder.ReportProcessed"), mock.Anything).Return(nil).Once()
	}
}

func mockTransmit(tc testCase, strategy *wtmocks.TargetStrategy, emitter *monmocks.ProtoEmitter) {
	// Skip transmission mocks if gas estimation fails
	if tc.expectError && len(tc.spendLimits) > 0 && tc.gasEstimateFee != nil && tc.gasEstimateFee.Fee.Cmp(big.NewInt(1000000000000000)) > 0 {
		return
	}

	if tc.txState != commontypes.Unknown {
		ex := strategy.EXPECT().TransmitReport(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		if tc.simulateTxError {
			ex.Return("", errors.New("transmit fail"))
		} else {
			ex.Return("tx123", nil)
			emitter.EXPECT().EmitWithLog(mock.Anything, mock.AnythingOfType("*writetarget.WriteSent"), mock.Anything).Return(nil).Once()
			strategy.EXPECT().GetTransactionStatus(mock.Anything, mock.Anything).Return(tc.txState, nil)
		}
	}
}

func mockGasEstimation(tc testCase, strategy *wtmocks.TargetStrategy) {
	// Only set up gas estimation mock if we have spend limits
	if len(tc.spendLimits) > 0 {
		ex := strategy.EXPECT().GetEstimateFee(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
		if tc.gasEstimateError != nil {
			ex.Return(commontypes.EstimateFee{}, tc.gasEstimateError)
		} else if tc.gasEstimateFee != nil {
			ex.Return(*tc.gasEstimateFee, nil)
		}
	}
}

func mockTransactionFee(tc testCase, strategy *wtmocks.TargetStrategy) {
	// Only set up transaction fee mock if we expect the execution to reach that point
	if !tc.expectError && tc.expectTransactionFee {
		ex := strategy.EXPECT().GetTransactionFee(mock.Anything, mock.Anything)
		if tc.transactionFeeError != nil {
			ex.Return(decimal.Decimal{}, tc.transactionFeeError)
		} else {
			ex.Return(tc.transactionFee, nil)
		}
	}
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

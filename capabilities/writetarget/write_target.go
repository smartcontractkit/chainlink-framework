//nolint:gosec,revive // disable G115,revive
package writetarget

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/shopspring/decimal"
	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities"
	"github.com/smartcontractkit/chainlink-common/pkg/capabilities/consensus/ocr3/types"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"

	monitor "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/beholder"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/report/platform"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/retry"

	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
)

var (
	_ capabilities.ExecutableCapability = &writeTarget{}
)

type TransactionStatus uint8

// new chain agnostic transmission state types
const (
	TransmissionStateNotAttempted TransactionStatus = iota
	TransmissionStateSucceeded
	TransmissionStateFailed // retry
	TransmissionStateFatal  // don't retry
)

// alter TransmissionState to reference specific types rather than just
// success bool
type TransmissionState struct {
	Status      TransactionStatus
	Transmitter string
	Err         error
}

type TargetStrategy interface {
	// QueryTransmissionState defines how the report should be queried
	// via ChainReader, and how resulting errors should be classified.
	QueryTransmissionState(ctx context.Context, reportID uint16, request capabilities.CapabilityRequest) (*TransmissionState, error)
	// TransmitReport constructs the tx to transmit the report, and defines
	// any specific handling for sending the report via ChainWriter.
	TransmitReport(ctx context.Context, report []byte, reportContext []byte, signatures [][]byte, request capabilities.CapabilityRequest) (string, error)
	// Wrapper around the ChainWriter to get the transaction status
	GetTransactionStatus(ctx context.Context, transactionID string) (commontypes.TransactionStatus, error)
	// Wrapper around the ChainWriter to get the fee esimate
	GetEstimateFee(ctx context.Context, report []byte, reportContext []byte, signatures [][]byte, request capabilities.CapabilityRequest) (commontypes.EstimateFee, error)
	// GetTransactionFee retrieves the actual transaction fee in native currency from the transaction receipt.
	// This method should be implemented by chain-specific services and handle the conversion of gas units to native currency.
	GetTransactionFee(ctx context.Context, transactionID string) (decimal.Decimal, error)
}

var (
	_ capabilities.ExecutableCapability = &writeTarget{}
)

// chain-agnostic consts
const (
	CapabilityName = "write"

	// Input keys
	// Is this key chain agnostic?
	KeySignedReport = "signed_report"
)

type chainService interface {
	LatestHead(ctx context.Context) (commontypes.Head, error)
}

type writeTarget struct {
	capabilities.CapabilityInfo

	config    Config
	chainInfo monitor.ChainInfo

	lggr logger.Logger
	// Local beholder client, also hosting the protobuf emitter
	beholder *beholder.BeholderClient

	cs               chainService
	configValidateFn func(request capabilities.CapabilityRequest) (string, error)

	nodeAddress      string
	forwarderAddress string

	targetStrategy       TargetStrategy
	writeAcceptanceState commontypes.TransactionStatus
}
type WriteTargetOpts struct {
	ID string

	// toml: [<CHAIN>.WriteTargetCap]
	Config Config
	// ChainInfo contains the chain information (used as execution context)
	// TODO: simplify by passing via ChainService.GetChainStatus fn
	ChainInfo monitor.ChainInfo

	Logger   logger.Logger
	Beholder *beholder.BeholderClient

	ChainService     chainService
	ConfigValidateFn func(request capabilities.CapabilityRequest) (string, error)

	NodeAddress      string
	ForwarderAddress string

	TargetStrategy       TargetStrategy
	WriteAcceptanceState commontypes.TransactionStatus
}

// Capability-specific configuration
type ReqConfig struct {
	Address   string
	Processor string
}

// NewWriteTargetID returns the capability ID for the write target
func NewWriteTargetID(chainFamilyName, networkName, chainID, version string) (string, error) {
	// Input args should not be empty
	if version == "" {
		return "", fmt.Errorf("version must not be empty")
	}

	// Network ID: network name is optional, if not provided, use the chain ID
	networkID := networkName
	if networkID == "" && chainID == "" {
		return "", fmt.Errorf("invalid input: networkName or chainID must not be empty")
	}
	if networkID == "" || networkID == "unknown" {
		networkID = chainID
	}

	// allow for chain family to be empty
	if chainFamilyName == "" {
		return fmt.Sprintf("%s_%s@%s", CapabilityName, networkID, version), nil
	}

	return fmt.Sprintf("%s_%s-%s@%s", CapabilityName, chainFamilyName, networkID, version), nil
}

// TODO: opts.Config input is not validated for sanity
func NewWriteTarget(opts WriteTargetOpts) capabilities.ExecutableCapability {
	capInfo := capabilities.MustNewCapabilityInfo(opts.ID, capabilities.CapabilityTypeTarget, CapabilityName)

	// override Unknown to ensure Finalized is default
	if opts.WriteAcceptanceState == 0 {
		opts.WriteAcceptanceState = commontypes.Finalized
	}

	return &writeTarget{
		capInfo,
		opts.Config,
		opts.ChainInfo,
		opts.Logger,
		opts.Beholder,
		opts.ChainService,
		opts.ConfigValidateFn,
		opts.NodeAddress,
		opts.ForwarderAddress,
		opts.TargetStrategy,
		opts.WriteAcceptanceState,
	}
}

func success() capabilities.CapabilityResponse {
	return capabilities.CapabilityResponse{}
}

// getGasSpendLimit returns the gas spend limit for the given chain ID from the request metadata
func (c *writeTarget) getGasSpendLimit(request capabilities.CapabilityRequest) (string, error) {
	spendType := "GAS." + c.chainInfo.ChainID

	for _, limit := range request.Metadata.SpendLimits {
		if spendType == string(limit.SpendType) {
			return limit.Limit, nil
		}
	}
	return "", fmt.Errorf("no gas spend limit found for chain %s", c.chainInfo.ChainID)
}

// checkGasEstimate verifies if the estimated gas fee is within the spend limit and returns the fee
func (c *writeTarget) checkGasEstimate(ctx context.Context, spendLimit string, report []byte, reportContext []byte, signatures [][]byte, request capabilities.CapabilityRequest) (*big.Int, uint32, error) {
	// Get gas estimate from ContractWriter
	fee, err := c.targetStrategy.GetEstimateFee(ctx, report, reportContext, signatures, request)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get gas estimate: %w", err)
	}

	// Convert spend limit from ETH to wei
	limitFloat, ok := new(big.Float).SetString(spendLimit)
	if !ok {
		return nil, 0, fmt.Errorf("invalid gas spend limit format: %s", spendLimit)
	}

	// Multiply by 10^decimals to convert from ETH to wei
	multiplier := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fee.Decimals)), nil))
	limitFloat.Mul(limitFloat, multiplier)

	// Convert to big.Int for comparison
	limit := new(big.Int)
	limitFloat.Int(limit)

	// Compare estimate with limit
	if fee.Fee.Cmp(limit) > 0 {
		return nil, 0, fmt.Errorf("estimated gas fee %s exceeds spend limit %s", fee.Fee.String(), limit.String())
	}

	return fee.Fee, fee.Decimals, nil
}

func (c *writeTarget) Execute(ctx context.Context, request capabilities.CapabilityRequest) (capabilities.CapabilityResponse, error) {
	// Take the local timestamp
	tsStart := time.Now().UnixMilli()

	// Trace the execution
	attrs := c.traceAttributes(request.Metadata.WorkflowExecutionID)
	ctx, span := c.beholder.Tracer.Start(ctx, "Execute", trace.WithAttributes(attrs...))
	defer span.End()

	// Notice: error skipped as implementation always returns nil
	capInfo, _ := c.Info(ctx)

	c.lggr.Debugw("Execute", "request", request, "capInfo", capInfo)

	// Helper to keep track of the request info
	info := requestInfo{
		tsStart:   tsStart,
		node:      c.nodeAddress,
		forwarder: c.forwarderAddress,
		receiver:  "N/A",
		request:   request,
		reportInfo: &reportInfo{
			reportContext: nil,
			report:        nil,
			signersNum:    0, // N/A
			reportID:      0, // N/A
		},
		reportTransmissionState: nil,
	}
	// Helper to build monitoring (Beholder) messages
	builder := NewMessageBuilder(c.chainInfo, capInfo)

	// Validate the config
	receiver, err := c.configValidateFn(request)
	if err != nil {
		msg := builder.buildWriteError(info, 0, "failed to validate config", err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	// Source the receiver address from the config
	info.receiver = receiver

	// Source the signed report from the request
	signedReport, ok := request.Inputs.Underlying[KeySignedReport]
	if !ok {
		cause := fmt.Sprintf("input missing required field: '%s'", KeySignedReport)
		msg := builder.buildWriteError(info, 0, "failed to source the signed report", cause)
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	// Decode the signed report
	inputs := types.SignedReport{}
	if err = signedReport.UnwrapTo(&inputs); err != nil {
		msg := builder.buildWriteError(info, 0, "failed to parse signed report", err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	// Source the report ID from the input
	info.reportInfo.reportID = binary.BigEndian.Uint16(inputs.ID)

	// Get gas spend limit first
	spendLimit, err := c.getGasSpendLimit(request)
	if err != nil {
		// No spend limit provided, skip gas estimation and continue with execution
		c.lggr.Debugw("No gas spend limit found, skipping gas estimation", "err", err)
	} else {
		// Check gas estimate before proceeding
		// TODO: discuss if we should release this in a separate PR
		_, _, err := c.checkGasEstimate(ctx, spendLimit, inputs.Report, inputs.Context, inputs.Signatures, request)
		if err != nil {
			// Build error message
			info := &requestInfo{
				tsStart: tsStart,
				node:    c.nodeAddress,
				request: request,
			}
			errMsg := c.asEmittedError(ctx, &wt.WriteError{
				Code:    uint32(TransmissionStateFatal),
				Summary: "InsufficientFunds",
				Cause:   err.Error(),
			}, "info", info)
			return capabilities.CapabilityResponse{}, errMsg
		}
	}

	err = c.beholder.ProtoEmitter.EmitWithLog(ctx, builder.buildWriteInitiated(info))
	if err != nil {
		c.lggr.Errorw("failed to emit write initiated", "err", err)
	}

	// Check whether the report is valid (e.g., not empty)
	if len(inputs.Report) == 0 {
		// We received any empty report -- this means we should skip transmission.
		err = c.beholder.ProtoEmitter.EmitWithLog(ctx, builder.buildWriteSkipped(info, "empty report"))
		if err != nil {
			c.lggr.Errorw("failed to emit write skipped", "err", err)
		}
		return success(), nil
	}

	// Update the info with the report info
	info.reportInfo = &reportInfo{
		reportID:      info.reportInfo.reportID,
		reportContext: inputs.Context,
		report:        inputs.Report,
		signersNum:    uint32(len(inputs.Signatures)),
	}

	// Decode the report
	reportDecoded, err := platform.Decode(inputs.Report)
	if err != nil {
		msg := builder.buildWriteError(info, 0, "failed to decode the report", err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	// Validate encoded report is prefixed with workflowID and executionID that match the request meta
	if reportDecoded.ExecutionID != request.Metadata.WorkflowExecutionID {
		msg := builder.buildWriteError(info, 0, "decoded report execution ID does not match the request", "")
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	} else if reportDecoded.WorkflowID != request.Metadata.WorkflowID {
		msg := builder.buildWriteError(info, 0, "decoded report workflow ID does not match the request", "")
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	// Fetch the latest head from the chain (timestamp), retry with a default backoff strategy
	ctx = retry.CtxWithID(ctx, info.request.Metadata.WorkflowExecutionID)
	head, err := retry.With(ctx, c.lggr, c.cs.LatestHead)
	if err != nil {
		msg := builder.buildWriteError(info, 0, "failed to fetch the latest head", err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	c.lggr.Debugw("non-empty valid report",
		"reportID", info.reportInfo.reportID,
		"report", "0x"+hex.EncodeToString(inputs.Report),
		"reportLen", len(inputs.Report),
		"reportDecoded", reportDecoded,
		"reportContext", "0x"+hex.EncodeToString(inputs.Context),
		"reportContextLen", len(inputs.Context),
		"signaturesLen", len(inputs.Signatures),
		"executionID", request.Metadata.WorkflowExecutionID,
	)

	state, err := c.targetStrategy.QueryTransmissionState(ctx, info.reportInfo.reportID, request)

	if err != nil {
		msg := builder.buildWriteError(info, 0, "failed to fetch [TransmissionState]", err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}

	switch state.Status {
	case TransmissionStateNotAttempted:
		c.lggr.Debugw("Report is not on chain yet, pushing tx", "reportID", info.reportInfo.reportID)
	case TransmissionStateFailed:
		c.lggr.Debugw("Tranmissions previously failed, retrying", "reportID", info.reportInfo.reportID)
	case TransmissionStateFatal:
		msg := builder.buildWriteError(info, 0, "Transmission attempt fatal", state.Err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	case TransmissionStateSucceeded:
		// Source the transmitter address from the on-chain state
		info.reportTransmissionState = state

		err = c.beholder.ProtoEmitter.EmitWithLog(ctx, builder.buildWriteConfirmed(info, head))
		if err != nil {
			c.lggr.Errorw("failed to emit write confirmed", "err", err)
		}
		return success(), nil
	}

	c.lggr.Infow("on-chain report check done - attempting to push to txmgr",
		"reportID", info.reportInfo.reportID,
		"reportLen", len(inputs.Report),
		"reportContextLen", len(inputs.Context),
		"signaturesLen", len(inputs.Signatures),
		"executionID", request.Metadata.WorkflowExecutionID,
	)

	txID, err := c.targetStrategy.TransmitReport(ctx, inputs.Report, inputs.Context, inputs.Signatures, request)
	c.lggr.Debugw("Transaction submitted", "request", request, "transaction-id", txID)
	if err != nil {
		msg := builder.buildWriteError(info, 0, "failed to transmit the report", err.Error())
		return capabilities.CapabilityResponse{}, c.asEmittedError(ctx, msg)
	}
	err = c.beholder.ProtoEmitter.EmitWithLog(ctx, builder.buildWriteSent(info, head, txID))
	if err != nil {
		c.lggr.Errorw("failed to emit write sent", "err", err)
	}

	err = c.acceptAndConfirmWrite(ctx, info, txID)
	if err != nil {
		return capabilities.CapabilityResponse{}, err
	}

	// Get the transaction fee
	fee, err := c.targetStrategy.GetTransactionFee(ctx, txID)
	if err != nil {
		return capabilities.CapabilityResponse{}, fmt.Errorf("failed to get transaction fee: %w", err)
	}

	return capabilities.CapabilityResponse{
		Metadata: capabilities.ResponseMetadata{
			Metering: []capabilities.MeteringNodeDetail{
				{
					Peer2PeerID: "ignored_by_engine",
					SpendUnit:   "GAS." + c.chainInfo.ChainID,
					SpendValue:  fee.String(),
				},
			},
		},
	}, nil
}

func (c *writeTarget) RegisterToWorkflow(ctx context.Context, request capabilities.RegisterToWorkflowRequest) error {
	// TODO: notify the background WriteTxConfirmer (workflow registered)
	return nil
}

func (c *writeTarget) UnregisterFromWorkflow(ctx context.Context, request capabilities.UnregisterFromWorkflowRequest) error {
	// TODO: notify the background WriteTxConfirmer (workflow unregistered)
	return nil
}

// acceptAndConfirmWrite waits (until timeout) for the report to be accepted and (optionally) confirmed on-chain
// Emits Beholder messages:
//   - 'platform.write-target.WriteError'     if not accepted
//   - 'platform.write-target.WriteAccepted'  if accepted (with or without an error)
//   - 'platform.write-target.WriteError'     if accepted (with an error)
//   - 'platform.write-target.WriteConfirmed' if confirmed (until timeout)
func (c *writeTarget) acceptAndConfirmWrite(ctx context.Context, info requestInfo, txID string) error {
	attrs := c.traceAttributes(info.request.Metadata.WorkflowExecutionID)
	_, span := c.beholder.Tracer.Start(ctx, "Execute.acceptAndConfirmWrite", trace.WithAttributes(attrs...))
	defer span.End()

	lggr := logger.Named(c.lggr, "write-confirmer")

	// Timeout for the confirmation process
	timeout := c.config.AcceptanceTimeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Retry interval for the confirmation process
	interval := c.config.PollPeriod
	ticker := services.NewTicker(interval)
	defer ticker.Stop()

	// Helper to build monitoring (Beholder) messages
	// Notice: error skipped as implementation always returns nil
	capInfo, _ := c.Info(ctx)
	builder := NewMessageBuilder(c.chainInfo, capInfo)

	txAccepted, err := c.waitTxReachesTerminalStatus(ctx, lggr, txID)

	if err != nil {
		// We (eventually) failed to confirm the report was transmitted
		msg := builder.buildWriteError(info, 0, "failed to wait until tx gets finalized", err.Error())
		lggr.Errorw("failed to wait until tx gets finalized", "txID", txID, "error", err)
		return c.asEmittedError(ctx, msg)
	}

	for {
		select {
		case <-ctx.Done():
			// We (eventually) failed to confirm the report was transmitted
			cause := "transaction was finalized, but report was not observed on chain before timeout"
			if !txAccepted {
				cause = "transaction failed and no other node managed to get report on chain before timeout"
			}
			msg := builder.buildWriteError(info, 0, "write confirmation - failed", cause)
			return c.asEmittedError(ctx, msg)
		case <-ticker.C:
			// Fetch the latest head from the chain (timestamp)
			head, err := c.cs.LatestHead(ctx)
			if err != nil {
				lggr.Errorw("failed to fetch the latest head", "txID", txID, "err", err)
				continue
			}

			// Check confirmation status (transmission state)
			state, err := c.targetStrategy.QueryTransmissionState(ctx, info.reportInfo.reportID, info.request)
			if err != nil {
				lggr.Errorw("failed to check confirmed status", "txID", txID, "err", err)
				continue
			}

			if state == nil || state.Status == TransmissionStateNotAttempted {
				lggr.Infow("not confirmed yet - transmission state NOT visible", "txID", txID)
				continue
			}

			// We (eventually) confirmed the report was transmitted
			// Emit the confirmation message and return
			if !txAccepted {
				lggr.Infow("confirmed - transmission state visible but submitted by another node. This node's tx failed", "txID", txID)
			} else {
				lggr.Infow("confirmed - transmission state visible", "txID", txID)
			}

			// Source the transmitter address from the on-chain state
			info.reportTransmissionState = state

			_ = c.beholder.ProtoEmitter.EmitWithLog(ctx, builder.buildWriteConfirmed(info, head))

			return nil
		}
	}
}

// Polls transaction status until it reaches one of terminal states [Finalized, Failed, Fatal]
func (c *writeTarget) waitTxReachesTerminalStatus(ctx context.Context, lggr logger.Logger, txID string) (finalized bool, err error) {
	// Retry interval for the confirmation process
	interval := c.config.PollPeriod
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
			// Check TXM for status
			status, err := c.targetStrategy.GetTransactionStatus(ctx, txID)
			if err != nil {
				lggr.Errorw("failed to fetch the transaction status", "txID", txID, "err", err)
				continue
			}

			lggr.Debugw("txm - tx status", "txID", txID, "status", status)

			switch status {
			case commontypes.Failed, commontypes.Fatal:
				// TODO: [Beholder] Emit 'platform.write-target.WriteError' if accepted with an error (surface specific on-chain error)
				lggr.Infow("transaction failed", "txID", txID, "status", status)
				return false, nil
			default:
				// Notice: On slower chains we can accept a non-finalized state, but on faster chains we should always wait for finalization.
				if status >= c.writeAcceptanceState {
					// Notice: report write confirmation is only possible after a tx is accepted without an error
					// TODO: [Beholder] Emit 'platform.write-target.WriteAccepted' (useful to source tx hash, block number, and tx status/error)
					lggr.Infow("accepted", "txID", txID, "status", status)
					return true, nil
				} else {
					lggr.Infow("not accepted yet", "txID", txID, "status", status)
					continue
				}
			}
		}
	}
}

// traceAttributes returns the attributes to be used for tracing
func (c *writeTarget) traceAttributes(workflowExecutionID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("capability_id", c.ID),
		attribute.String("capability_type", string(c.CapabilityType)),
		attribute.String("workflow_execution_id", workflowExecutionID),
	}
}

// asEmittedError returns the WriteError message as an (Go) error, after emitting it first
func (c *writeTarget) asEmittedError(ctx context.Context, e *wt.WriteError, attrKVs ...any) error {
	// Notice: we always want to log the error
	err := c.beholder.ProtoEmitter.EmitWithLog(ctx, e, attrKVs...)
	if err != nil {
		return errors.Join(fmt.Errorf("failed to emit error: %+w", err), e)
	}
	return e
}

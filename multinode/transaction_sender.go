package multinode

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

var (
	// PromMultiNodeInvariantViolations reports violation of our assumptions
	PromMultiNodeInvariantViolations = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "multi_node_invariant_violations",
		Help: "The number of invariant violations",
	}, []string{"network", "chainId", "invariant"})
)

// TxErrorClassifier - defines interface of a function that transforms raw RPC error into the SendTxReturnCode enum
// (e.g. Successful, Fatal, Retryable, etc.)
type TxErrorClassifier[T any] func(tx T, err error) SendTxReturnCode

type sendTxResult[R any] struct {
	Err        error
	ResultCode SendTxReturnCode
	Result     *R
}

const sendTxQuorum = 0.7

// SendTxRPCClient - defines interface of an RPC used by TransactionSender to broadcast transaction
type SendTxRPCClient[T any, R any] interface {
	// SendTransaction errors returned should include name or other unique identifier of the RPC
	SendTransaction(ctx context.Context, tx T) (*R, error)
}

func NewTransactionSender[T any, R any, I ID, C SendTxRPCClient[T, R]](
	lggr logger.Logger,
	chainID I,
	chainFamily string,
	multiNode *MultiNode[I, C],
	txErrorClassifier TxErrorClassifier[T],
	sendTxSoftTimeout time.Duration,
) *TransactionSender[T, R, I, C] {
	if sendTxSoftTimeout == 0 {
		sendTxSoftTimeout = QueryTimeout / 2
	}
	return &TransactionSender[T, R, I, C]{
		chainID:           chainID,
		chainFamily:       chainFamily,
		lggr:              logger.Sugared(lggr).Named("TransactionSender").With("chainID", chainID.String()),
		multiNode:         multiNode,
		txErrorClassifier: txErrorClassifier,
		sendTxSoftTimeout: sendTxSoftTimeout,
		chStop:            make(services.StopChan),
	}
}

type TransactionSender[T any, R any, I ID, C SendTxRPCClient[T, R]] struct {
	services.StateMachine
	chainID           I
	chainFamily       string
	lggr              logger.SugaredLogger
	multiNode         *MultiNode[I, C]
	txErrorClassifier TxErrorClassifier[T]
	sendTxSoftTimeout time.Duration // defines max waiting time from first response til responses evaluation

	wg     sync.WaitGroup // waits for all reporting goroutines to finish
	chStop services.StopChan
}

// SendTransaction - broadcasts transaction to all the send-only and primary nodes in MultiNode.
// A returned nil or error does not guarantee that the transaction will or won't be included. Additional checks must be
// performed to determine the final state.
//
// Send-only nodes' results are ignored as they tend to return false-positive responses. Broadcast to them is necessary
// to speed up the propagation of TX in the network.
//
// Handling of primary nodes' results consists of collection and aggregation.
// In the collection step, we gather as many results as possible while minimizing waiting time. This operation succeeds
// on one of the following conditions:
// * Received at least one success
// * Received at least one result and `sendTxSoftTimeout` expired
// * Received results from the sufficient number of nodes defined by sendTxQuorum.
// The aggregation is based on the following conditions:
// * If there is at least one success - returns success
// * If there is at least one terminal error - returns terminal error
// * If there is both success and terminal error - returns success and reports invariant violation
// * Otherwise, returns any (effectively random) of the errors.
func (txSender *TransactionSender[T, R, I, C]) SendTransaction(ctx context.Context, tx T) (*R, SendTxReturnCode, error) {
	txResults := make(chan sendTxResult[R])
	txResultsToReport := make(chan sendTxResult[R])
	primaryNodeWg := sync.WaitGroup{}

	if txSender.State() != "Started" {
		return nil, Retryable, errors.New("TransactionSender not started")
	}

	ctx, cancel := txSender.chStop.Ctx(ctx)
	defer cancel()

	healthyNodesNum := 0
	err := txSender.multiNode.DoAll(ctx, func(ctx context.Context, rpc C, isSendOnly bool) {
		if isSendOnly {
			txSender.wg.Add(1)
			go func() {
				defer txSender.wg.Done()
				// Send-only nodes' results are ignored as they tend to return false-positive responses.
				// Broadcast to them is necessary to speed up the propagation of TX in the network.
				_ = txSender.broadcastTxAsync(ctx, rpc, tx)
			}()
			return
		}

		// Primary Nodes
		healthyNodesNum++
		primaryNodeWg.Add(1)
		go func() {
			defer primaryNodeWg.Done()
			result := txSender.broadcastTxAsync(ctx, rpc, tx)
			select {
			case <-ctx.Done():
				return
			case txResults <- result:
			}

			select {
			case <-ctx.Done():
				return
			case txResultsToReport <- result:
			}
		}()
	})

	// This needs to be done in parallel so the reporting knows when it's done (when the channel is closed)
	txSender.wg.Add(1)
	go func() {
		defer txSender.wg.Done()
		primaryNodeWg.Wait()
		close(txResultsToReport)
		close(txResults)
	}()

	if err != nil {
		return nil, Retryable, err
	}

	txSender.wg.Add(1)
	go txSender.reportSendTxAnomalies(tx, txResultsToReport)

	return txSender.collectTxResults(ctx, tx, healthyNodesNum, txResults)
}

func (txSender *TransactionSender[T, R, I, C]) broadcastTxAsync(ctx context.Context, rpc C, tx T) sendTxResult[R] {
	result, txErr := rpc.SendTransaction(ctx, tx)
	txSender.lggr.Debugw("Node sent transaction", "tx", tx, "err", txErr)
	resultCode := txSender.txErrorClassifier(tx, txErr)
	if !slices.Contains(sendTxSuccessfulCodes, resultCode) {
		txSender.lggr.Warnw("RPC returned error", "tx", tx, "err", txErr)
	}
	return sendTxResult[R]{Err: txErr, ResultCode: resultCode, Result: result}
}

func (txSender *TransactionSender[T, R, I, C]) reportSendTxAnomalies(tx T, txResults <-chan sendTxResult[R]) {
	defer txSender.wg.Done()
	resultsByCode := sendTxResults[R]{}
	// txResults eventually will be closed
	for txResult := range txResults {
		resultsByCode[txResult.ResultCode] = append(resultsByCode[txResult.ResultCode], txResult)
	}

	_, _, _, criticalErr := aggregateTxResults[R](resultsByCode)
	if criticalErr != nil {
		txSender.lggr.Criticalw("observed invariant violation on SendTransaction", "tx", tx, "resultsByCode", resultsByCode, "err", criticalErr)
		PromMultiNodeInvariantViolations.WithLabelValues(txSender.chainFamily, txSender.chainID.String(), criticalErr.Error()).Inc()
	}
}

type sendTxResults[R any] map[SendTxReturnCode][]sendTxResult[R]

func aggregateTxResults[R any](resultsByCode sendTxResults[R]) (result *R, returnCode SendTxReturnCode, txResult error, err error) {
	severeCode, severeErrors, hasSevereErrors := findFirstIn(resultsByCode, sendTxSevereErrors)
	successCode, successResults, hasSuccess := findFirstIn(resultsByCode, sendTxSuccessfulCodes)
	if hasSuccess {
		// We assume that primary node would never report false positive txResult for a transaction.
		// Thus, if such case occurs it's probably due to misconfiguration or a bug and requires manual intervention.
		if hasSevereErrors {
			const errMsg = "found contradictions in nodes replies on SendTransaction: got success and severe error"
			// return success, since at least 1 node has accepted our broadcasted Tx, and thus it can now be included onchain
			return successResults[0].Result, successCode, successResults[0].Err, errors.New(errMsg)
		}

		// other errors are temporary - we are safe to return success
		return successResults[0].Result, successCode, successResults[0].Err, nil
	}

	if hasSevereErrors {
		return nil, severeCode, severeErrors[0].Err, nil
	}

	// return temporary error
	for code, result := range resultsByCode {
		return nil, code, result[0].Err, nil
	}

	err = fmt.Errorf("expected at least one response on SendTransaction")
	return nil, Retryable, err, err
}

func (txSender *TransactionSender[T, R, I, C]) collectTxResults(ctx context.Context, tx T, healthyNodesNum int, txResults <-chan sendTxResult[R]) (*R, SendTxReturnCode, error) {
	if healthyNodesNum == 0 {
		return nil, Retryable, ErroringNodeError
	}
	requiredResults := int(math.Ceil(float64(healthyNodesNum) * sendTxQuorum))
	errorsByCode := sendTxResults[R]{}
	var softTimeoutChan <-chan time.Time
	var resultsCount int
loop:
	for {
		select {
		case <-ctx.Done():
			txSender.lggr.Debugw("Failed to collect of the results before context was done", "tx", tx, "errorsByCode", errorsByCode)
			return nil, Retryable, ctx.Err()
		case result := <-txResults:
			errorsByCode[result.ResultCode] = append(errorsByCode[result.ResultCode], result)
			resultsCount++
			if slices.Contains(sendTxSuccessfulCodes, result.ResultCode) || resultsCount >= requiredResults {
				break loop
			}
		case <-softTimeoutChan:
			txSender.lggr.Debugw("Send Tx soft timeout expired - returning responses we've collected so far", "tx", tx, "resultsCount", resultsCount, "requiredResults", requiredResults)
			break loop
		}

		if softTimeoutChan == nil {
			tm := time.NewTimer(txSender.sendTxSoftTimeout)
			softTimeoutChan = tm.C
			// we are fine with stopping timer at the end of function
			//nolint
			defer tm.Stop()
		}
	}

	// ignore critical error as it's reported in reportSendTxAnomalies
	result, returnCode, resultErr, _ := aggregateTxResults(errorsByCode)
	return result, returnCode, resultErr
}

func (txSender *TransactionSender[T, R, I, C]) Start(ctx context.Context) error {
	return txSender.StartOnce("TransactionSender", func() error {
		return nil
	})
}

func (txSender *TransactionSender[T, R, I, C]) Close() error {
	return txSender.StopOnce("TransactionSender", func() error {
		close(txSender.chStop)
		txSender.wg.Wait()
		return nil
	})
}

// findFirstIn - returns the first existing key and value for the slice of keys
func findFirstIn[K comparable, V any](set map[K]V, keys []K) (K, V, bool) {
	for _, k := range keys {
		if v, ok := set[k]; ok {
			return k, v, true
		}
	}
	var zeroK K
	var zeroV V
	return zeroK, zeroV, false
}

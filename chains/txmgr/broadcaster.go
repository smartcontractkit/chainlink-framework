package txmgr

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jpillora/backoff"
	"go.uber.org/multierr"
	"gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-common/pkg/chains/label"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"
	"github.com/smartcontractkit/chainlink-framework/multinode"

	"github.com/smartcontractkit/chainlink-framework/chains"
	"github.com/smartcontractkit/chainlink-framework/chains/fees"
	"github.com/smartcontractkit/chainlink-framework/chains/txmgr/types"
)

const (
	// InFlightTransactionRecheckInterval controls how often the Broadcaster
	// will poll the unconfirmed queue to see if it is allowed to send another
	// transaction
	InFlightTransactionRecheckInterval = 1 * time.Second

	// TransmitCheckTimeout controls the maximum amount of time that will be
	// spent on the transmit check.
	TransmitCheckTimeout = 2 * time.Second

	// maxBroadcastRetries is the number of times a transaction broadcast is retried when the sequence fails to increment on Hedera
	maxHederaBroadcastRetries = 3

	// hederaChainType is the string representation of the Hedera chain type
	// Temporary solution until the Broadcaster is moved to the EVM code base
	hederaChainType = "hedera"
)

var ErrTxRemoved = errors.New("tx removed")

type ProcessUnstartedTxs[ADDR chains.Hashable] func(ctx context.Context, fromAddress ADDR) (retryable bool, err error)

// TransmitCheckerFactory creates a transmit checker based on a spec.
type TransmitCheckerFactory[CID chains.ID, ADDR chains.Hashable, THASH, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee] interface {
	// BuildChecker builds a new TransmitChecker based on the given spec.
	BuildChecker(spec types.TransmitCheckerSpec[ADDR]) (TransmitChecker[CID, ADDR, THASH, BHASH, SEQ, FEE], error)
}

// TransmitChecker determines whether a transaction should be submitted on-chain.
type TransmitChecker[CID chains.ID, ADDR chains.Hashable, THASH, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee] interface {

	// Check the given transaction. If the transaction should not be sent, an error indicating why
	// is returned. Errors should only be returned if the checker can confirm that a transaction
	// should not be sent, other errors (for example connection or other unexpected errors) should
	// be logged and swallowed.
	Check(ctx context.Context, l logger.SugaredLogger, tx types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], a types.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE]) error
}

type broadcasterMetrics interface {
	IncrementNumBroadcastedTxs(ctx context.Context)
	RecordTimeUntilTxBroadcast(ctx context.Context, duration float64)
}

// Broadcaster monitors txes for transactions that need to
// be broadcast, assigns sequences and ensures that at least one node
// somewhere has received the transaction successfully.
//
// This does not guarantee delivery! A whole host of other things can
// subsequently go wrong such as transactions being evicted from the mempool,
// nodes going offline etc. Responsibility for ensuring eventual inclusion
// into the chain falls on the shoulders of the confirmer.
//
// What Broadcaster does guarantee is:
// - a monotonic series of increasing sequences for txes that can all eventually be confirmed if you retry enough times
// - transition of txes out of unstarted into either fatal_error or unconfirmed
// - existence of a saved tx_attempt
type Broadcaster[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee] struct {
	services.StateMachine
	lggr    logger.SugaredLogger
	txStore types.TransactionStore[ADDR, CID, THASH, BHASH, SEQ, FEE]
	client  types.TransactionClient[CID, ADDR, THASH, BHASH, SEQ, FEE]
	types.TxAttemptBuilder[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]
	sequenceTracker types.SequenceTracker[ADDR, SEQ]
	resumeCallback  ResumeCallback
	chainID         CID
	chainType       string
	config          types.BroadcasterChainConfig
	feeConfig       types.BroadcasterFeeConfig
	txConfig        types.BroadcasterTransactionsConfig
	listenerConfig  types.BroadcasterListenerConfig
	metrics         broadcasterMetrics

	// autoSyncSequence, if set, will cause Broadcaster to fast-forward the sequence
	// when Start is called
	autoSyncSequence bool

	processUnstartedTxsImpl ProcessUnstartedTxs[ADDR]

	ks               types.KeyStore[ADDR]
	enabledAddresses []ADDR

	checkerFactory TransmitCheckerFactory[CID, ADDR, THASH, BHASH, SEQ, FEE]

	// triggers allow other goroutines to force Broadcaster to rescan the
	// database early (before the next poll interval)
	// Each key has its own trigger
	triggers map[ADDR]chan struct{}

	chStop services.StopChan
	wg     sync.WaitGroup

	initSync  sync.Mutex
	isStarted bool
}

func NewBroadcaster[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee](
	txStore types.TransactionStore[ADDR, CID, THASH, BHASH, SEQ, FEE],
	client types.TransactionClient[CID, ADDR, THASH, BHASH, SEQ, FEE],
	config types.BroadcasterChainConfig,
	feeConfig types.BroadcasterFeeConfig,
	txConfig types.BroadcasterTransactionsConfig,
	listenerConfig types.BroadcasterListenerConfig,
	keystore types.KeyStore[ADDR],
	txAttemptBuilder types.TxAttemptBuilder[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE],
	sequenceTracker types.SequenceTracker[ADDR, SEQ],
	lggr logger.Logger,
	checkerFactory TransmitCheckerFactory[CID, ADDR, THASH, BHASH, SEQ, FEE],
	autoSyncSequence bool,
	chainType string,
	metrics broadcasterMetrics,
) *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE] {
	lggr = logger.Named(lggr, "Broadcaster")
	b := &Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]{
		lggr:             logger.Sugared(lggr),
		txStore:          txStore,
		client:           client,
		TxAttemptBuilder: txAttemptBuilder,
		chainID:          client.ConfiguredChainID(),
		chainType:        chainType,
		config:           config,
		feeConfig:        feeConfig,
		txConfig:         txConfig,
		listenerConfig:   listenerConfig,
		ks:               keystore,
		checkerFactory:   checkerFactory,
		autoSyncSequence: autoSyncSequence,
		sequenceTracker:  sequenceTracker,
		metrics:          metrics,
	}

	b.processUnstartedTxsImpl = b.processUnstartedTxs
	return b
}

// Start starts Broadcaster service.
// The provided context can be used to terminate Start sequence.
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Start(ctx context.Context) error {
	return eb.StartOnce("Broadcaster", func() error {
		return eb.startInternal(ctx)
	})
}

// startInternal can be called multiple times, in conjunction with closeInternal. The TxMgr uses this functionality to reset broadcaster multiple times in its own lifetime.
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) startInternal(ctx context.Context) error {
	eb.initSync.Lock()
	defer eb.initSync.Unlock()
	if eb.isStarted {
		return errors.New("Broadcaster is already started")
	}
	if eb.enabledAddresses == nil {
		var err error
		eb.enabledAddresses, err = eb.ks.EnabledAddresses(ctx)
		if err != nil {
			return fmt.Errorf("Broadcaster: failed to load EnabledAddresses: %w", err)
		}
	}

	if len(eb.enabledAddresses) > 0 {
		eb.lggr.Debugw(fmt.Sprintf("Booting with %d keys", len(eb.enabledAddresses)), "keys", eb.enabledAddresses)
	} else {
		eb.lggr.Warnf("Chain %s does not have any keys, no transactions will be sent on this chain", eb.chainID.String())
	}
	eb.chStop = make(chan struct{})
	eb.wg = sync.WaitGroup{}
	eb.triggers = make(map[ADDR]chan struct{})
	eb.wg.Add(1)
	go eb.loadAndMonitor()

	eb.isStarted = true
	return nil
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) loadAndMonitor() {
	defer eb.wg.Done()
	ctx, cancel := eb.chStop.NewCtx()
	defer cancel()
	eb.sequenceTracker.LoadNextSequences(ctx, eb.enabledAddresses)
	eb.wg.Add(len(eb.enabledAddresses))
	for _, addr := range eb.enabledAddresses {
		triggerCh := make(chan struct{}, 1)
		eb.triggers[addr] = triggerCh
		go eb.monitorTxs(addr, triggerCh)
	}
}

// Close closes the Broadcaster
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Close() error {
	return eb.StopOnce("Broadcaster", func() error {
		return eb.closeInternal()
	})
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) closeInternal() error {
	eb.initSync.Lock()
	defer eb.initSync.Unlock()
	if !eb.isStarted {
		return fmt.Errorf("Broadcaster is not started: %w", services.ErrAlreadyStopped)
	}
	close(eb.chStop)
	eb.wg.Wait()
	eb.isStarted = false
	eb.enabledAddresses = nil
	return nil
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) SetResumeCallback(callback ResumeCallback) {
	eb.resumeCallback = callback
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Name() string {
	return eb.lggr.Name()
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) HealthReport() map[string]error {
	return map[string]error{eb.Name(): eb.Healthy()}
}

// Trigger forces the monitor for a particular address to recheck for new txes
// Logs error and does nothing if address was not registered on startup
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Trigger(addr ADDR) {
	eb.initSync.Lock()
	defer eb.initSync.Unlock()
	if !eb.isStarted {
		eb.lggr.Debugf("Unstarted; ignoring trigger for %s", addr)
	}
	triggerCh, exists := eb.triggers[addr]
	if !exists {
		// ignoring trigger for address which is not registered with this Broadcaster
		return
	}
	select {
	case triggerCh <- struct{}{}:
	default:
	}
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) newResendBackoff() backoff.Backoff {
	return backoff.Backoff{
		Min:    1 * time.Second,
		Max:    15 * time.Second,
		Jitter: true,
	}
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) monitorTxs(addr ADDR, triggerCh chan struct{}) {
	defer eb.wg.Done()

	ctx, cancel := eb.chStop.NewCtx()
	defer cancel()

	if eb.autoSyncSequence {
		eb.lggr.Debugw("Auto-syncing sequence", "address", addr.String())
		eb.sequenceTracker.SyncSequence(ctx, addr, eb.chStop)
		if ctx.Err() != nil {
			return
		}
	} else {
		eb.lggr.Debugw("Skipping sequence auto-sync", "address", addr.String())
	}

	// errorRetryCh allows retry on exponential backoff in case of timeout or
	// other unknown error
	var errorRetryCh <-chan time.Time
	bf := eb.newResendBackoff()

	for {
		pollDBTimer := time.NewTimer(utils.WithJitter(eb.listenerConfig.FallbackPollInterval()))

		retryable, err := eb.processUnstartedTxsImpl(ctx, addr)
		if err != nil {
			eb.lggr.Errorw("Error occurred while handling tx queue in ProcessUnstartedTxs", "err", err)
		}
		// On retryable errors we implement exponential backoff retries. This
		// handles intermittent connectivity, remote RPC races, timing issues etc
		if retryable {
			pollDBTimer.Reset(utils.WithJitter(eb.listenerConfig.FallbackPollInterval()))
			errorRetryCh = time.After(bf.Duration())
		} else {
			bf = eb.newResendBackoff()
			errorRetryCh = nil
		}

		select {
		case <-ctx.Done():
			// NOTE: See: https://godoc.org/time#Timer.Stop for an explanation of this pattern
			if !pollDBTimer.Stop() {
				<-pollDBTimer.C
			}
			return
		case <-triggerCh:
			// tx was inserted
			if !pollDBTimer.Stop() {
				<-pollDBTimer.C
			}
			continue
		case <-pollDBTimer.C:
			// DB poller timed out
			continue
		case <-errorRetryCh:
			// Error backoff period reached
			continue
		}
	}
}

// ProcessUnstartedTxs picks up and handles all txes in the queue
// revive:disable:error-return
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) ProcessUnstartedTxs(ctx context.Context, addr ADDR) (retryable bool, err error) {
	return eb.processUnstartedTxs(ctx, addr)
}

// NOTE: This MUST NOT be run concurrently for the same address or it could
// result in undefined state or deadlocks.
// First handle any in_progress transactions left over from last time.
// Then keep looking up unstarted transactions and processing them until there are none remaining.
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) processUnstartedTxs(ctx context.Context, fromAddress ADDR) (retryable bool, err error) {
	var n uint
	mark := time.Now()
	defer func() {
		if n > 0 {
			eb.lggr.Debugw("Finished processUnstartedTxs", "address", fromAddress, "time", time.Since(mark), "n", n, "id", "broadcaster")
		}
	}()

	err, retryable = eb.handleAnyInProgressTx(ctx, fromAddress)
	if err != nil {
		return retryable, fmt.Errorf("processUnstartedTxs failed on handleAnyInProgressTx: %w", err)
	}
	for {
		maxInFlightTransactions := eb.txConfig.MaxInFlight()
		if maxInFlightTransactions > 0 {
			nUnconfirmed, err := eb.txStore.CountUnconfirmedTransactions(ctx, fromAddress, eb.chainID)
			if err != nil {
				return true, fmt.Errorf("CountUnconfirmedTransactions failed: %w", err)
			}
			if nUnconfirmed >= maxInFlightTransactions {
				nUnstarted, err := eb.txStore.CountUnstartedTransactions(ctx, fromAddress, eb.chainID)
				if err != nil {
					return true, fmt.Errorf("CountUnstartedTransactions failed: %w", err)
				}
				eb.lggr.Warnw(fmt.Sprintf(`Transaction throttling; %d transactions in-flight and %d unstarted transactions pending (maximum number of in-flight transactions is %d per key). %s`, nUnconfirmed, nUnstarted, maxInFlightTransactions, label.MaxInFlightTransactionsWarning), "maxInFlightTransactions", maxInFlightTransactions, "nUnconfirmed", nUnconfirmed, "nUnstarted", nUnstarted)
				select {
				case <-time.After(InFlightTransactionRecheckInterval):
				case <-ctx.Done():
					return false, context.Cause(ctx)
				}
				continue
			}
		}
		etx, err := eb.nextUnstartedTransactionWithSequence(fromAddress)
		if err != nil {
			return true, fmt.Errorf("processUnstartedTxs failed on nextUnstartedTransactionWithSequence: %w", err)
		}
		if etx == nil {
			return false, nil
		}
		n++

		if err, retryable := eb.handleUnstartedTx(ctx, etx); err != nil {
			return retryable, fmt.Errorf("processUnstartedTxs failed on handleUnstartedTx: %w", err)
		}
	}
}

// handleInProgressTx checks if there is any transaction
// in_progress and if so, finishes the job
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) handleAnyInProgressTx(ctx context.Context, fromAddress ADDR) (err error, retryable bool) {
	etx, err := eb.txStore.GetTxInProgress(ctx, fromAddress)
	if err != nil {
		return fmt.Errorf("handleAnyInProgressTx failed: %w", err), true
	}
	if etx != nil {
		if err, retryable := eb.handleInProgressTx(ctx, *etx, etx.TxAttempts[0], etx.CreatedAt, 0); err != nil {
			return fmt.Errorf("handleAnyInProgressTx failed: %w", err), retryable
		}
	}
	return nil, false
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) handleUnstartedTx(ctx context.Context, etx *types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE]) (error, bool) {
	if etx.State != TxUnstarted {
		return fmt.Errorf("invariant violation: expected transaction %v to be unstarted, it was %s", etx.ID, etx.State), false
	}

	attempt, _, _, retryable, err := eb.NewTxAttempt(ctx, *etx, eb.lggr)
	// Mark transaction as fatal if provided gas limit is set too low
	if errors.Is(err, fees.ErrFeeLimitTooLow) {
		etx.Error = null.StringFrom(fees.ErrFeeLimitTooLow.Error())
		return eb.saveFatallyErroredTransaction(eb.lggr, etx), false
	} else if err != nil {
		return fmt.Errorf("processUnstartedTxs failed on NewAttempt: %w", err), retryable
	}

	checkerSpec, err := etx.GetChecker()
	if err != nil {
		return fmt.Errorf("parsing transmit checker: %w", err), false
	}

	checker, err := eb.checkerFactory.BuildChecker(checkerSpec)
	if err != nil {
		return fmt.Errorf("building transmit checker: %w", err), false
	}

	lgr := etx.GetLogger(eb.lggr.With("fee", attempt.TxFee))

	// If the transmit check does not complete within the timeout, the transaction will be sent
	// anyway.
	// It's intentional that we only run `Check` for unstarted transactions.
	// Running it on other states might lead to nonce duplication, as we might mark applied transactions as fatally errored.

	checkCtx, cancel := context.WithTimeout(ctx, TransmitCheckTimeout)
	defer cancel()
	err = checker.Check(checkCtx, lgr, *etx, attempt)
	if errors.Is(err, context.Canceled) {
		lgr.Warn("Transmission checker timed out, sending anyway")
	} else if err != nil {
		etx.Error = null.StringFrom(err.Error())
		lgr.Warnw("Transmission checker failed, fatally erroring transaction.", "err", err)
		return eb.saveFatallyErroredTransaction(lgr, etx), true
	}
	cancel()

	if err = eb.txStore.UpdateTxUnstartedToInProgress(ctx, etx, &attempt); errors.Is(err, ErrTxRemoved) {
		eb.lggr.Debugw("tx removed", "txID", etx.ID, "subject", etx.Subject)
		return nil, false
	} else if err != nil {
		return fmt.Errorf("processUnstartedTxs failed on UpdateTxUnstartedToInProgress: %w", err), true
	}

	return eb.handleInProgressTx(ctx, *etx, attempt, time.Now(), 0)
}

// There can be at most one in_progress transaction per address.
// Here we complete the job that we didn't finish last time.
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) handleInProgressTx(ctx context.Context, etx types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], attempt types.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE], initialBroadcastAt time.Time, retryCount int) (error, bool) {
	if etx.State != TxInProgress {
		return fmt.Errorf("invariant violation: expected transaction %v to be in_progress, it was %s", etx.ID, etx.State), false
	}

	lgr := etx.GetLogger(logger.With(eb.lggr, "fee", attempt.TxFee))
	lgr.Infow("Sending transaction", "txAttemptID", attempt.ID, "txHash", attempt.Hash, "meta", etx.Meta, "feeLimit", attempt.ChainSpecificFeeLimit, "callerProvidedFeeLimit", etx.FeeLimit, "attempt", attempt, "etx", etx)
	errType, err := eb.client.SendTransactionReturnCode(ctx, etx, attempt, lgr)

	// The validation below is only applicable to Hedera because it has instant finality and a unique sequence behavior
	if eb.chainType == hederaChainType {
		errType, err = eb.validateOnChainSequence(ctx, lgr, errType, err, etx, retryCount)
	}

	if errType == multinode.Fatal || errType == multinode.TerminallyStuck {
		eb.SvcErrBuffer.Append(err)
		etx.Error = null.StringFrom(err.Error())
		return eb.saveFatallyErroredTransaction(lgr, &etx), true
	}

	etx.InitialBroadcastAt = &initialBroadcastAt
	etx.BroadcastAt = &initialBroadcastAt

	switch errType {
	case multinode.TransactionAlreadyKnown:
		fallthrough
	case multinode.Successful:
		// Either the transaction was successful or one of the following four scenarios happened:
		//
		// SCENARIO 1
		//
		// This is resuming a previous crashed run. In this scenario, it is
		// likely that our previous transaction was the one who was confirmed,
		// in which case we hand it off to the confirmer to get the
		// receipt.
		//
		// SCENARIO 2
		//
		// It is also possible that an external wallet can have messed with the
		// account and sent a transaction on this sequence.
		//
		// In this case, the onus is on the node operator since this is
		// explicitly unsupported.
		//
		// If it turns out to have been an external wallet, we will never get a
		// receipt for this transaction and it will eventually be marked as
		// errored.
		//
		// The end result is that we will NOT SEND a transaction for this
		// sequence.
		//
		// SCENARIO 3
		//
		// The network client can be assumed to have at-least-once delivery
		// behavior. It is possible that the client could have already
		// sent this exact same transaction even if this is our first time
		// calling SendTransaction().
		//
		// SCENARIO 4 (most likely)
		//
		// A sendonly node got the transaction in first.
		//
		// In all scenarios, the correct thing to do is assume success for now
		// and hand off to the confirmer to get the receipt (or mark as
		// failed).
		observeTimeUntilBroadcast(ctx, eb.metrics, etx.CreatedAt, time.Now())
		err = eb.txStore.UpdateTxAttemptInProgressToBroadcast(ctx, &etx, attempt, types.TxAttemptBroadcast)
		if err != nil {
			return err, true
		}
		eb.metrics.IncrementNumBroadcastedTxs(ctx)
		// Increment sequence if successfully broadcasted
		eb.sequenceTracker.GenerateNextSequence(etx.FromAddress, *etx.Sequence)
		return err, true
	case multinode.Underpriced:
		bumpedAttempt, retryable, replaceErr := eb.replaceAttemptWithBumpedGas(ctx, lgr, err, etx, attempt)
		if replaceErr != nil {
			return replaceErr, retryable
		}

		return eb.handleInProgressTx(ctx, etx, bumpedAttempt, initialBroadcastAt, retryCount+1)
	case multinode.InsufficientFunds:
		// NOTE: This can occur due to either insufficient funds or a gas spike
		// combined with a high gas limit. Regardless of the cause, we need to obtain a new estimate,
		// replace the current attempt, and retry after the backoff duration.
		// The new attempt must be replaced immediately because of a database constraint.
		eb.SvcErrBuffer.Append(err)
		if _, _, replaceErr := eb.replaceAttemptWithNewEstimation(ctx, lgr, etx, attempt); replaceErr != nil {
			return replaceErr, true
		}
		return err, true
	case multinode.Retryable:
		return err, true
	case multinode.FeeOutOfValidRange:
		replacementAttempt, retryable, replaceErr := eb.replaceAttemptWithNewEstimation(ctx, lgr, etx, attempt)
		if replaceErr != nil {
			return replaceErr, retryable
		}

		lgr.Warnw("L2 rejected transaction due to incorrect fee, re-estimated and will try again",
			"etxID", etx.ID, "err", err, "newGasPrice", replacementAttempt.TxFee, "newGasLimit", replacementAttempt.ChainSpecificFeeLimit)
		return eb.handleInProgressTx(ctx, etx, *replacementAttempt, initialBroadcastAt, 0)
	case multinode.Unsupported:
		return err, false
	case multinode.ExceedsMaxFee:
		// Broadcaster: Note that we may have broadcast to multiple nodes and had it
		// accepted by one of them! It is not guaranteed that all nodes share
		// the same tx fee cap. That is why we must treat this as an unknown
		// error that may have been confirmed.
		// If there is only one RPC node, or all RPC nodes have the same
		// configured cap, this transaction will get stuck and keep repeating
		// forever until the issue is resolved.
		lgr.Criticalw(`RPC node rejected this tx as outside Fee Cap`, "attempt", attempt)
		fallthrough
	default:
		// Every error that doesn't fall under one of the above categories will be treated as Unknown.
		fallthrough
	case multinode.Unknown:
		eb.SvcErrBuffer.Append(err)
		lgr.Criticalw(`Unknown error occurred while handling tx queue in ProcessUnstartedTxs. This chain/RPC client may not be supported. `+
			`Urgent resolution required, Chainlink is currently operating in a degraded state and may miss transactions`, "attempt", attempt, "err", err)
		nextSequence, e := eb.client.PendingSequenceAt(ctx, etx.FromAddress)
		if e != nil {
			err = multierr.Combine(e, err)
			return fmt.Errorf("failed to fetch latest pending sequence after encountering unknown RPC error while sending transaction: %w", err), true
		}
		if nextSequence.Int64() > (*etx.Sequence).Int64() {
			// Despite the error, the RPC node considers the previously sent
			// transaction to have been accepted. In this case, the right thing to
			// do is assume success and hand off to Confirmer

			err = eb.txStore.UpdateTxAttemptInProgressToBroadcast(ctx, &etx, attempt, types.TxAttemptBroadcast)
			if err != nil {
				return err, true
			}
			eb.metrics.IncrementNumBroadcastedTxs(ctx)
			// Increment sequence if successfully broadcasted
			eb.sequenceTracker.GenerateNextSequence(etx.FromAddress, *etx.Sequence)
			return err, true
		}
		// Either the unknown error prevented the transaction from being mined, or
		// it has not yet propagated to the mempool, or there is some race on the
		// remote RPC.
		//
		// In all cases, the best thing we can do is go into a retry loop and keep
		// trying to send the transaction over again.
		return fmt.Errorf("retryable error while sending transaction %s (tx ID %d): %w", attempt.Hash.String(), etx.ID, err), true
	}
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) validateOnChainSequence(ctx context.Context, lgr logger.SugaredLogger, errType multinode.SendTxReturnCode, err error, etx types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], retryCount int) (multinode.SendTxReturnCode, error) {
	// Only check if sequence was incremented if broadcast was successful, otherwise return the existing err type
	if errType != multinode.Successful {
		return errType, err
	}
	// Transaction sequence cannot be nil here since a sequence is required to broadcast
	txSeq := *etx.Sequence
	// Retrieve the latest mined sequence from on-chain
	nextSeqOnChain, err := eb.client.SequenceAt(ctx, etx.FromAddress, nil)
	if err != nil {
		return errType, err
	}

	// Check that the transaction count has incremented on-chain to include the broadcasted transaction
	// Insufficient transaction fee is a common scenario in which the sequence is not incremented by the chain even though we got a successful response
	// If the sequence failed to increment and hasn't reached the max retries, return the Underpriced error to try again with a bumped attempt
	if nextSeqOnChain.Int64() == txSeq.Int64() && retryCount < maxHederaBroadcastRetries {
		return multinode.Underpriced, nil
	}

	// If the transaction reaches the retry limit and fails to get included, mark it as fatally errored
	// Some unknown error other than insufficient tx fee could be the cause
	if nextSeqOnChain.Int64() == txSeq.Int64() && retryCount >= maxHederaBroadcastRetries {
		err := fmt.Errorf("failed to broadcast transaction on %s after %d retries", hederaChainType, retryCount)
		lgr.Error(err.Error())
		return multinode.Fatal, err
	}

	// Belts and braces approach to detect and handle sqeuence gaps if the broadcast is considered successful
	if nextSeqOnChain.Int64() < txSeq.Int64() {
		err := fmt.Errorf("next expected sequence on-chain (%s) is less than the broadcasted transaction's sequence (%s)", nextSeqOnChain.String(), txSeq.String())
		lgr.Criticalw("Sequence gap has been detected and needs to be filled", "error", err)
		return multinode.Fatal, err
	}

	return multinode.Successful, nil
}

// Finds next transaction in the queue, assigns a sequence, and moves it to "in_progress" state ready for broadcast.
// Returns nil if no transactions are in queue
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) nextUnstartedTransactionWithSequence(fromAddress ADDR) (*types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], error) {
	ctx, cancel := eb.chStop.NewCtx()
	defer cancel()
	etx, err := eb.txStore.FindNextUnstartedTransactionFromAddress(ctx, fromAddress, eb.chainID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Finish. No more transactions left to process. Hoorah!
			return nil, nil
		}
		return nil, fmt.Errorf("findNextUnstartedTransactionFromAddress failed: %w", err)
	}

	sequence, err := eb.sequenceTracker.GetNextSequence(ctx, etx.FromAddress)
	if err != nil {
		return nil, err
	}
	etx.Sequence = &sequence
	return etx, nil
}

// replaceAttemptWithBumpedGas performs the replacement of the existing tx attempt with a new bumped fee attempt.
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) replaceAttemptWithBumpedGas(ctx context.Context, lgr logger.Logger, txError error, etx types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], attempt types.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE]) (replacedAttempt types.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE], retryable bool, err error) {
	// This log error is not applicable to Hedera since the action required would not be needed for its gas estimator
	if eb.chainType != hederaChainType {
		logger.With(lgr,
			"sendError", txError,
			"attemptFee", attempt.TxFee,
			"maxGasPriceConfig", eb.feeConfig.MaxFeePrice(),
		).Errorf("attempt fee %v was rejected by the node for being too low. "+
			"Node returned: '%s'. "+
			"Will bump and retry. ACTION REQUIRED: This is a configuration error. "+
			"Consider increasing FeeEstimator.PriceDefault (current value: %s)",
			attempt.TxFee, txError.Error(), eb.feeConfig.FeePriceDefault())
	}

	bumpedAttempt, bumpedFee, bumpedFeeLimit, retryable, err := eb.NewBumpTxAttempt(ctx, etx, attempt, nil, lgr)
	if err != nil {
		return bumpedAttempt, retryable, err
	}

	if err = eb.txStore.SaveReplacementInProgressAttempt(ctx, attempt, &bumpedAttempt); err != nil {
		return bumpedAttempt, true, err
	}

	lgr.Debugw("Bumped fee on initial send", "oldFee", attempt.TxFee.String(), "newFee", bumpedFee.String(), "newFeeLimit", bumpedFeeLimit)
	return bumpedAttempt, true, err
}

// replaceAttemptWithNewEstimation performs the replacement of the existing tx attempt with a new estimated fee attempt.
func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) replaceAttemptWithNewEstimation(ctx context.Context, lgr logger.Logger, etx types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], attempt types.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE]) (updatedAttempt *types.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE], retryable bool, err error) {
	newEstimatedAttempt, fee, feeLimit, retryable, err := eb.NewTxAttemptWithType(ctx, etx, lgr, attempt.TxType, fees.OptForceRefetch)
	if err != nil {
		return &newEstimatedAttempt, retryable, err
	}

	if err = eb.txStore.SaveReplacementInProgressAttempt(ctx, attempt, &newEstimatedAttempt); err != nil {
		return &newEstimatedAttempt, true, err
	}

	lgr.Debugw("new estimated fee on initial send", "oldFee", attempt.TxFee.String(), "newFee", fee.String(), "newFeeLimit", feeLimit)
	return &newEstimatedAttempt, true, err
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) saveFatallyErroredTransaction(lgr logger.Logger, etx *types.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE]) error {
	ctx, cancel := eb.chStop.NewCtx()
	defer cancel()
	if etx.State != TxInProgress && etx.State != TxUnstarted {
		return fmt.Errorf("can only transition to fatal_error from in_progress or unstarted, transaction is currently %s", etx.State)
	}
	if !etx.Error.Valid {
		return errors.New("expected error field to be set")
	}
	// NOTE: It's simpler to not do this transactionally for now (would require
	// refactoring pipeline runner resume to use postgres events)
	//
	// There is a very tiny possibility of the following:
	//
	// 1. We get a fatal error on the tx, resuming the pipeline with error
	// 2. Crash or failure during persist of fatal errored tx
	// 3. On the subsequent run the tx somehow succeeds and we save it as successful
	//
	// Now we have an errored pipeline even though the tx succeeded. This case
	// is relatively benign and probably nobody will ever run into it in
	// practice, but something to be aware of.
	if etx.PipelineTaskRunID.Valid && eb.resumeCallback != nil && etx.SignalCallback && !etx.CallbackCompleted {
		err := eb.resumeCallback(ctx, etx.PipelineTaskRunID.UUID, nil, fmt.Errorf("fatal error while sending transaction: %s", etx.Error.String))
		if errors.Is(err, sql.ErrNoRows) {
			lgr.Debugw("callback missing or already resumed", "etxID", etx.ID)
		} else if err != nil {
			return fmt.Errorf("failed to resume pipeline: %w", err)
		} else {
			// Mark tx as having completed callback
			if err := eb.txStore.UpdateTxCallbackCompleted(ctx, etx.PipelineTaskRunID.UUID, eb.chainID); err != nil {
				return err
			}
		}
	}
	return eb.txStore.UpdateTxFatalErrorAndDeleteAttempts(ctx, etx)
}

func observeTimeUntilBroadcast(ctx context.Context, metrics broadcasterMetrics, createdAt, broadcastAt time.Time) {
	duration := float64(broadcastAt.Sub(createdAt))
	metrics.RecordTimeUntilTxBroadcast(ctx, duration)
}

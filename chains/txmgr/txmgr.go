package txmgr

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jpillora/backoff"
	nullv4 "gopkg.in/guregu/null.v4"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-common/pkg/utils"

	"github.com/smartcontractkit/chainlink-framework/chains"
	"github.com/smartcontractkit/chainlink-framework/chains/fees"
	"github.com/smartcontractkit/chainlink-framework/chains/heads"
	txmgrtypes "github.com/smartcontractkit/chainlink-framework/chains/txmgr/types"
)

// For more information about the Txm architecture, see the design doc:
// https://www.notion.so/chainlink/Txm-Architecture-Overview-9dc62450cd7a443ba9e7dceffa1a8d6b

// ResumeCallback is assumed to be idempotent
type ResumeCallback func(ctx context.Context, id uuid.UUID, result interface{}, err error) error

type NewErrorClassifier func(err error) txmgrtypes.ErrorClassifier

// TxManager is the main component of the transaction manager.
// It is also the interface to external callers.
type TxManager[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee] interface {
	heads.Trackable[HEAD, BHASH]
	services.Service
	Trigger(addr ADDR)
	CreateTransaction(ctx context.Context, txRequest txmgrtypes.TxRequest[ADDR, THASH]) (etx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	GetForwarderForEOA(ctx context.Context, eoa ADDR) (forwarder ADDR, err error)
	GetForwarderForEOAOCR2Feeds(ctx context.Context, eoa, ocr2AggregatorID ADDR) (forwarder ADDR, err error)
	RegisterResumeCallback(fn ResumeCallback)
	SendNativeToken(ctx context.Context, chainID CID, from, to ADDR, value big.Int, gasLimit uint64) (etx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	Reset(addr ADDR, abandon bool) error
	// Find transactions by a field in the TxMeta blob and transaction states
	FindTxesByMetaFieldAndStates(ctx context.Context, metaField string, metaValue string, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	// Find transactions with a non-null TxMeta field that was provided by transaction states
	FindTxesWithMetaFieldByStates(ctx context.Context, metaField string, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	// Find transactions with a non-null TxMeta field that was provided and a receipt block number greater than or equal to the one provided
	FindTxesWithMetaFieldByReceiptBlockNum(ctx context.Context, metaField string, blockNum int64, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	// Find transactions loaded with transaction attempts and receipts by transaction IDs and states
	FindTxesWithAttemptsAndReceiptsByIdsAndState(ctx context.Context, ids []int64, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	FindEarliestUnconfirmedBroadcastTime(ctx context.Context) (nullv4.Time, error)
	FindEarliestUnconfirmedTxAttemptBlock(ctx context.Context) (nullv4.Int, error)
	CountTransactionsByState(ctx context.Context, state txmgrtypes.TxState) (count uint32, err error)
	GetTransactionStatus(ctx context.Context, transactionID string) (state commontypes.TransactionStatus, err error)
	GetTransactionFee(ctx context.Context, transactionID string) (fee *commontypes.TransactionFee, err error)
}

type TxmV2Wrapper[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee] interface {
	services.Service
	CreateTransaction(ctx context.Context, txRequest txmgrtypes.TxRequest[ADDR, THASH]) (etx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error)
	Reset(addr ADDR, abandon bool) error
}

type reset struct {
	// f is the function to execute between stopping/starting the
	// Broadcaster and Confirmer
	f func()
	// done is either closed after running f, or returns error if f could not
	// be run for some reason
	done chan error
}

type Txm[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, R txmgrtypes.ChainReceipt[THASH, BHASH], SEQ chains.Sequence, FEE fees.Fee] struct {
	services.StateMachine
	logger                  logger.SugaredLogger
	txStore                 txmgrtypes.TxStore[ADDR, CID, THASH, BHASH, R, SEQ, FEE]
	config                  txmgrtypes.TransactionManagerChainConfig
	txConfig                txmgrtypes.TransactionManagerTransactionsConfig
	keyStore                txmgrtypes.KeyStore[ADDR]
	chainID                 CID
	checkerFactory          TransmitCheckerFactory[CID, ADDR, THASH, BHASH, SEQ, FEE]
	pruneQueueAndCreateLock sync.Mutex

	chHeads        chan HEAD
	trigger        chan ADDR
	reset          chan reset
	resumeCallback ResumeCallback

	chStop   services.StopChan
	chSubbed chan struct{}
	wg       sync.WaitGroup

	reaper             *Reaper[CID]
	resender           *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]
	broadcaster        *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]
	confirmer          *Confirmer[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]
	tracker            *Tracker[CID, ADDR, THASH, BHASH, R, SEQ, FEE]
	finalizer          txmgrtypes.Finalizer[BHASH, HEAD]
	fwdMgr             txmgrtypes.ForwarderManager[ADDR]
	txAttemptBuilder   txmgrtypes.TxAttemptBuilder[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]
	newErrorClassifier NewErrorClassifier
	txmv2wrapper       TxmV2Wrapper[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]

	enabledAddrs []ADDR // sorted as strings
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) RegisterResumeCallback(fn ResumeCallback) {
	b.resumeCallback = fn
	b.broadcaster.SetResumeCallback(fn)
	b.confirmer.SetResumeCallback(fn)
	b.finalizer.SetResumeCallback(fn)
}

// NewTxm creates a new Txm with the given configuration.
func NewTxm[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, R txmgrtypes.ChainReceipt[THASH, BHASH], SEQ chains.Sequence, FEE fees.Fee](
	chainID CID,
	cfg txmgrtypes.TransactionManagerChainConfig,
	txCfg txmgrtypes.TransactionManagerTransactionsConfig,
	keyStore txmgrtypes.KeyStore[ADDR],
	lggr logger.Logger,
	checkerFactory TransmitCheckerFactory[CID, ADDR, THASH, BHASH, SEQ, FEE],
	fwdMgr txmgrtypes.ForwarderManager[ADDR],
	txAttemptBuilder txmgrtypes.TxAttemptBuilder[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE],
	txStore txmgrtypes.TxStore[ADDR, CID, THASH, BHASH, R, SEQ, FEE],
	broadcaster *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE],
	confirmer *Confirmer[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE],
	resender *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE],
	tracker *Tracker[CID, ADDR, THASH, BHASH, R, SEQ, FEE],
	finalizer txmgrtypes.Finalizer[BHASH, HEAD],
	newErrorClassifierFunc NewErrorClassifier,
	txmv2wrapper TxmV2Wrapper[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE],
) *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE] {
	b := Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]{
		logger:             logger.Sugared(lggr),
		txStore:            txStore,
		config:             cfg,
		txConfig:           txCfg,
		keyStore:           keyStore,
		chainID:            chainID,
		checkerFactory:     checkerFactory,
		chHeads:            make(chan HEAD),
		trigger:            make(chan ADDR),
		chStop:             make(chan struct{}),
		chSubbed:           make(chan struct{}),
		reset:              make(chan reset),
		fwdMgr:             fwdMgr,
		txAttemptBuilder:   txAttemptBuilder,
		broadcaster:        broadcaster,
		confirmer:          confirmer,
		resender:           resender,
		tracker:            tracker,
		newErrorClassifier: newErrorClassifierFunc,
		finalizer:          finalizer,
		txmv2wrapper:       txmv2wrapper,
	}

	if txCfg.ResendAfterThreshold() <= 0 {
		b.logger.Info("Resender: Disabled")
	}
	if txCfg.ReaperThreshold() > 0 && txCfg.ReaperInterval() > 0 {
		b.reaper = NewReaper[CID](lggr, b.txStore, txCfg, chainID)
	} else {
		b.logger.Info("TxReaper: Disabled")
	}

	return &b
}

// Start starts Txm service.
// The provided context can be used to terminate Start sequence.
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) Start(ctx context.Context) (merr error) {
	return b.StartOnce("Txm", func() error {
		var ms services.MultiStart
		if err := ms.Start(ctx, b.broadcaster); err != nil {
			return fmt.Errorf("Txm: Broadcaster failed to start: %w", err)
		}
		if err := ms.Start(ctx, b.confirmer); err != nil {
			return fmt.Errorf("Txm: Confirmer failed to start: %w", err)
		}

		if err := ms.Start(ctx, b.txAttemptBuilder); err != nil {
			return fmt.Errorf("Txm: Estimator failed to start: %w", err)
		}

		if err := ms.Start(ctx, b.tracker); err != nil {
			return fmt.Errorf("Txm: Tracker failed to start: %w", err)
		}
		b.enabledAddrs = b.tracker.getEnabledAddresses()
		slices.SortFunc(b.enabledAddrs, func(a, b ADDR) int { return strings.Compare(a.String(), b.String()) })

		if err := ms.Start(ctx, b.finalizer); err != nil {
			return fmt.Errorf("Txm: Finalizer failed to start: %w", err)
		}

		if b.txmv2wrapper != nil {
			if err := ms.Start(ctx, b.txmv2wrapper); err != nil {
				return fmt.Errorf("Txm: Txmv2 failed to start: %w", err)
			}
		}

		b.logger.Info("Txm starting runLoop")
		b.wg.Add(1)
		go b.runLoop()
		<-b.chSubbed

		if b.reaper != nil {
			b.reaper.Start()
		}

		if b.resender != nil {
			b.resender.Start(ctx)
		}

		if b.fwdMgr != nil {
			if err := ms.Start(ctx, b.fwdMgr); err != nil {
				return fmt.Errorf("Txm: ForwarderManager failed to start: %w", err)
			}
		}

		return nil
	})
}

// Reset stops Broadcaster/Confirmer, executes callback, then starts them again
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) Reset(addr ADDR, abandon bool) (err error) {
	ok := b.IfStarted(func() {
		done := make(chan error)
		f := func() {
			if abandon {
				err = b.abandon(addr)
				if b.txmv2wrapper != nil {
					if err2 := b.txmv2wrapper.Reset(addr, abandon); err2 != nil {
						b.logger.Error("failed to abandon transactions for dual broadcast", "err", err2)
					}
				}
			}
		}

		b.reset <- reset{f, done}
		err = <-done
	})
	if !ok {
		return errors.New("not started")
	}
	return err
}

// abandon, scoped to the key of this txm:
// - marks all pending and inflight transactions fatally errored (note: at this point all transactions are either confirmed or fatally errored)
// this must not be run while Broadcaster or Confirmer are running
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) abandon(addr ADDR) (err error) {
	ctx, cancel := b.chStop.NewCtx()
	defer cancel()
	if err = b.txStore.Abandon(ctx, b.chainID, addr); err != nil {
		return fmt.Errorf("abandon failed to update txes for key %s: %w", addr.String(), err)
	}
	return nil
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) Close() (merr error) {
	return b.StopOnce("Txm", func() error {
		close(b.chStop)

		b.txStore.Close()

		if b.reaper != nil {
			b.reaper.Stop()
		}
		if b.resender != nil {
			b.resender.Stop()
		}
		if b.fwdMgr != nil {
			if err := b.fwdMgr.Close(); err != nil {
				merr = errors.Join(merr, fmt.Errorf("Txm: failed to stop ForwarderManager: %w", err))
			}
		}

		b.wg.Wait()

		if err := b.txAttemptBuilder.Close(); err != nil {
			merr = errors.Join(merr, fmt.Errorf("Txm: failed to close TxAttemptBuilder: %w", err))
		}

		return nil
	})
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) Name() string {
	return b.logger.Name()
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) HealthReport() map[string]error {
	report := map[string]error{b.Name(): b.Healthy()}

	// only query if txm started properly
	b.IfStarted(func() {
		services.CopyHealth(report, b.broadcaster.HealthReport())
		services.CopyHealth(report, b.confirmer.HealthReport())
		services.CopyHealth(report, b.txAttemptBuilder.HealthReport())
		services.CopyHealth(report, b.finalizer.HealthReport())
	})

	if b.txConfig.ForwardersEnabled() {
		services.CopyHealth(report, b.fwdMgr.HealthReport())
	}
	return report
}

// setEnabled sets enabled addresses in the broadcaster, tracker, and confirmer.
// Must only be called before starting, or after closing (during a reset).
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) setEnabled(addrs []ADDR) {
	b.broadcaster.enabledAddresses = addrs
	b.tracker.setEnabledAddresses(addrs)
	b.confirmer.enabledAddresses = addrs
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) runLoop() {
	ctx, cancel := b.chStop.NewCtx()
	defer cancel()

	// eb, ec and keyStates can all be modified by the runloop.
	// This is concurrent-safe because the runloop ensures serial access.
	defer b.wg.Done()

	close(b.chSubbed)

	var stopped bool
	var stopOnce sync.Once

	// execReset is defined as an inline function here because it closes over
	// eb, ec and stopped
	execReset := func(ctx context.Context, f func()) {
		// These should always close successfully, since it should be logically
		// impossible to enter this code path with ec/eb in a state other than
		// "Started"
		if err := b.broadcaster.closeInternal(); err != nil {
			b.logger.Panicw(fmt.Sprintf("Failed to Close Broadcaster: %v", err), "err", err)
		}
		if err := b.tracker.closeInternal(); err != nil {
			b.logger.Panicw(fmt.Sprintf("Failed to Close Tracker: %v", err), "err", err)
		}
		if err := b.confirmer.closeInternal(); err != nil {
			b.logger.Panicw(fmt.Sprintf("Failed to Close Confirmer: %v", err), "err", err)
		}
		if f != nil {
			f()
		}
		var wg sync.WaitGroup
		// three goroutines to handle independent backoff retries starting:
		// - Broadcaster
		// - Confirmer
		// - Tracker
		// If chStop is closed, we mark stopped=true so that the main runloop
		// can check and exit early if necessary
		//
		// execReset will not return until either:
		// 1. Broadcaster, Confirmer, and Tracker all started successfully
		// 2. chStop was closed (txmgr exit)
		wg.Add(3)
		go func() {
			defer wg.Done()
			// Retry indefinitely on failure
			backoff := newRedialBackoff()
			for {
				select {
				case <-time.After(backoff.Duration()):
					if err := b.broadcaster.startInternal(ctx); err != nil {
						b.logger.Criticalw("Failed to start Broadcaster", "err", err)
						b.SvcErrBuffer.Append(err)
						continue
					}
					return
				case <-b.chStop:
					stopOnce.Do(func() { stopped = true })
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			// Retry indefinitely on failure
			backoff := newRedialBackoff()
			for {
				select {
				case <-time.After(backoff.Duration()):
					if err := b.tracker.startInternal(ctx); err != nil {
						b.logger.Criticalw("Failed to start Tracker", "err", err)
						b.SvcErrBuffer.Append(err)
						continue
					}
					return
				case <-b.chStop:
					stopOnce.Do(func() { stopped = true })
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			// Retry indefinitely on failure
			backoff := newRedialBackoff()
			for {
				select {
				case <-time.After(backoff.Duration()):
					if err := b.confirmer.startInternal(ctx); err != nil {
						b.logger.Criticalw("Failed to start Confirmer", "err", err)
						b.SvcErrBuffer.Append(err)
						continue
					}
					return
				case <-b.chStop:
					stopOnce.Do(func() { stopped = true })
					return
				}
			}
		}()

		wg.Wait()
	}

	pollEnabledAddresses := services.NewTicker(time.Minute)
	defer pollEnabledAddresses.Stop()

	for {
		select {
		case address := <-b.trigger:
			b.broadcaster.Trigger(address)
		case head := <-b.chHeads:
			b.confirmer.mb.Deliver(head)
			b.tracker.mb.Deliver(head.BlockNumber())
			b.finalizer.DeliverLatestHead(head)
		case reset := <-b.reset:
			// This check prevents the weird edge-case where you can select
			// into this block after chStop has already been closed and the
			// previous reset exited early.
			// In this case we do not want to reset again, we would rather go
			// around and hit the stop case.
			if stopped {
				reset.done <- errors.New("Txm was stopped")
				continue
			}
			execReset(ctx, func() {
				reset.f()
				close(reset.done)
			})
		case <-b.chStop:
			// close and exit
			//
			// Note that in some cases Broadcaster and/or Confirmer may
			// be in an Unstarted state here, if execReset exited early.
			//
			// In this case, we don't care about stopping them since they are
			// already "stopped".
			err := b.broadcaster.Close()
			if err != nil && (!errors.Is(err, services.ErrAlreadyStopped) || !errors.Is(err, services.ErrCannotStopUnstarted)) {
				b.logger.Errorw(fmt.Sprintf("Failed to Close Broadcaster: %v", err), "err", err)
			}
			err = b.confirmer.Close()
			if err != nil && (!errors.Is(err, services.ErrAlreadyStopped) || !errors.Is(err, services.ErrCannotStopUnstarted)) {
				b.logger.Errorw(fmt.Sprintf("Failed to Close Confirmer: %v", err), "err", err)
			}
			err = b.tracker.Close()
			if err != nil && (!errors.Is(err, services.ErrAlreadyStopped) || !errors.Is(err, services.ErrCannotStopUnstarted)) {
				b.logger.Errorw(fmt.Sprintf("Failed to Close Tracker: %v", err), "err", err)
			}
			err = b.finalizer.Close()
			if err != nil && (!errors.Is(err, services.ErrAlreadyStopped) || !errors.Is(err, services.ErrCannotStopUnstarted)) {
				b.logger.Errorw(fmt.Sprintf("Failed to Close Finalizer: %v", err), "err", err)
			}
			if b.txmv2wrapper != nil {
				err = b.txmv2wrapper.Close()
				if err != nil && (!errors.Is(err, services.ErrAlreadyStopped) || !errors.Is(err, services.ErrCannotStopUnstarted)) {
					b.logger.Errorw(fmt.Sprintf("Failed to Close Finalizer: %v", err), "err", err)
				}
			}
			return
		case <-pollEnabledAddresses.C:
			// This check prevents the weird edge-case where you can select
			// into this block after chStop has already been closed and the
			// previous reset exited early.
			// In this case we do not want to reset again, we would rather go
			// around and hit the stop case.
			if stopped {
				continue
			}
			enabledAddresses, err := b.keyStore.EnabledAddresses(ctx)
			if err != nil {
				b.logger.Critical("Failed to reload key states")
				b.SvcErrBuffer.Append(err)
				continue
			}
			slices.SortFunc(enabledAddresses, func(a, b ADDR) int {
				return strings.Compare(a.String(), b.String())
			})
			if slices.Equal(b.enabledAddrs, enabledAddresses) {
				continue // no change
			}
			b.logger.Debugw("Keys changed, reloading", "enabledAddresses", enabledAddresses)

			execReset(ctx, func() { b.setEnabled(enabledAddresses) })
		}
	}
}

// OnNewLongestChain conforms to HeadTrackable
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) OnNewLongestChain(ctx context.Context, head HEAD) {
	ok := b.IfStarted(func() {
		if b.reaper != nil {
			b.reaper.SetLatestBlockNum(head.BlockNumber())
		}
		b.txAttemptBuilder.OnNewLongestChain(ctx, head)
		select {
		case b.chHeads <- head:
		case <-ctx.Done():
			b.logger.Errorw("Timed out handling head", "blockNum", head.BlockNumber(), "ctxErr", ctx.Err())
		}
	})
	if !ok {
		b.logger.Debugw("Not started; ignoring head", "head", head, "state", b.State())
	}
}

// Trigger forces the Broadcaster to check early for the given address
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) Trigger(addr ADDR) {
	select {
	case b.trigger <- addr:
	default:
	}
}

// CreateTransaction inserts a new transaction
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) CreateTransaction(ctx context.Context, txRequest txmgrtypes.TxRequest[ADDR, THASH]) (tx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	// Check for existing Tx with IdempotencyKey. If found, return the Tx and do nothing
	// Skipping CreateTransaction to avoid double send
	if b.txmv2wrapper != nil && txRequest.Meta != nil && txRequest.Meta.DualBroadcast != nil && *txRequest.Meta.DualBroadcast {
		return b.txmv2wrapper.CreateTransaction(ctx, txRequest)
	}
	if txRequest.IdempotencyKey != nil {
		var existingTx *txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE]
		existingTx, err = b.txStore.FindTxWithIdempotencyKey(ctx, *txRequest.IdempotencyKey, b.chainID)
		if err != nil {
			return tx, fmt.Errorf("failed to search for transaction with IdempotencyKey: %w", err)
		}
		if existingTx != nil {
			b.logger.Infow("Found a Tx with IdempotencyKey. Returning existing Tx without creating a new one.", "IdempotencyKey", *txRequest.IdempotencyKey)
			return *existingTx, nil
		}
	}

	if err = b.checkEnabled(ctx, txRequest.FromAddress); err != nil {
		return tx, err
	}

	if b.txConfig.ForwardersEnabled() && (!utils.IsZero(txRequest.ForwarderAddress)) {
		fwdPayload, fwdErr := b.fwdMgr.ConvertPayload(txRequest.ToAddress, txRequest.EncodedPayload)
		if fwdErr == nil {
			// Handling meta not set at caller.
			toAddressCopy := txRequest.ToAddress
			if txRequest.Meta != nil {
				txRequest.Meta.FwdrDestAddress = &toAddressCopy
			} else {
				txRequest.Meta = &txmgrtypes.TxMeta[ADDR, THASH]{
					FwdrDestAddress: &toAddressCopy,
				}
			}
			txRequest.ToAddress = txRequest.ForwarderAddress
			txRequest.EncodedPayload = fwdPayload
		} else {
			b.logger.Errorf("Failed to use forwarder set upstream: %v", fwdErr.Error())
		}
	}

	err = b.txStore.CheckTxQueueCapacity(ctx, txRequest.FromAddress, b.txConfig.MaxQueued(), b.chainID)
	if err != nil {
		return tx, fmt.Errorf("Txm#CreateTransaction: %w", err)
	}

	tx, err = b.pruneQueueAndCreateTxn(ctx, txRequest, b.chainID)
	if err != nil {
		return tx, err
	}

	// Trigger the Broadcaster to check for new transaction
	b.broadcaster.Trigger(txRequest.FromAddress)

	return tx, nil
}

// Calls forwarderMgr to get a proper forwarder for a given EOA.
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) GetForwarderForEOA(ctx context.Context, eoa ADDR) (forwarder ADDR, err error) {
	if !b.txConfig.ForwardersEnabled() {
		return forwarder, fmt.Errorf("forwarding is not enabled, to enable set Transactions.ForwardersEnabled =true")
	}
	forwarder, err = b.fwdMgr.ForwarderFor(ctx, eoa)
	return
}

// GetForwarderForEOAOCR2Feeds calls forwarderMgr to get a proper forwarder for a given EOA and checks if its set as a transmitter on the OCR2Aggregator contract.
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) GetForwarderForEOAOCR2Feeds(ctx context.Context, eoa, ocr2Aggregator ADDR) (forwarder ADDR, err error) {
	if !b.txConfig.ForwardersEnabled() {
		return forwarder, fmt.Errorf("forwarding is not enabled, to enable set Transactions.ForwardersEnabled =true")
	}
	forwarder, err = b.fwdMgr.ForwarderForOCR2Feeds(ctx, eoa, ocr2Aggregator)
	return
}

type NotEnabledError[ADDR chains.Hashable] struct {
	FromAddress ADDR
	Err         error
}

func (e NotEnabledError[ADDR]) Is(err error) bool {
	if as, ok := err.(NotEnabledError[ADDR]); ok {
		var zero ADDR
		return as.FromAddress == zero || e.FromAddress.String() == as.FromAddress.String()
	}
	return false
}

func (e *NotEnabledError[ADDR]) As(err error) bool {
	if as, ok := err.(NotEnabledError[ADDR]); ok { //nolint:errorlint // implementing As
		*e = as
		return true
	}
	return false
}

func (e NotEnabledError[ADDR]) Unwrap() error { return e.Err }

func (e NotEnabledError[ADDR]) Error() string {
	return fmt.Sprintf("cannot send transaction from %s: %v", e.FromAddress, e.Err)
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) checkEnabled(ctx context.Context, addr ADDR) error {
	if err := b.keyStore.CheckEnabled(ctx, addr); err != nil {
		return NotEnabledError[ADDR]{FromAddress: addr, Err: err}
	}
	return nil
}

// SendNativeToken creates a transaction that transfers the given value of native tokens
func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) SendNativeToken(ctx context.Context, chainID CID, from, to ADDR, value big.Int, gasLimit uint64) (etx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	if utils.IsZero(to) {
		return etx, errors.New("cannot send native token to zero address")
	}
	txRequest := txmgrtypes.TxRequest[ADDR, THASH]{
		FromAddress:    from,
		ToAddress:      to,
		EncodedPayload: []byte{},
		Value:          value,
		FeeLimit:       gasLimit,
		Strategy:       NewSendEveryStrategy(),
	}
	etx, err = b.pruneQueueAndCreateTxn(ctx, txRequest, chainID)
	if err != nil {
		return etx, fmt.Errorf("SendNativeToken failed to insert tx: %w", err)
	}

	// Trigger the Broadcaster to check for new transaction
	b.broadcaster.Trigger(from)
	return etx, nil
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) FindTxesByMetaFieldAndStates(ctx context.Context, metaField string, metaValue string, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	txes, err = b.txStore.FindTxesByMetaFieldAndStates(ctx, metaField, metaValue, states, chainID)
	return
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) FindTxesWithMetaFieldByStates(ctx context.Context, metaField string, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	txes, err = b.txStore.FindTxesWithMetaFieldByStates(ctx, metaField, states, chainID)
	return
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) FindTxesWithMetaFieldByReceiptBlockNum(ctx context.Context, metaField string, blockNum int64, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	txes, err = b.txStore.FindTxesWithMetaFieldByReceiptBlockNum(ctx, metaField, blockNum, chainID)
	return
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) FindTxesWithAttemptsAndReceiptsByIdsAndState(ctx context.Context, ids []int64, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) { //nolint:revive // exported
	txes, err = b.txStore.FindTxesWithAttemptsAndReceiptsByIdsAndState(ctx, ids, states, chainID)
	return
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) FindEarliestUnconfirmedBroadcastTime(ctx context.Context) (nullv4.Time, error) {
	return b.txStore.FindEarliestUnconfirmedBroadcastTime(ctx, b.chainID)
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) FindEarliestUnconfirmedTxAttemptBlock(ctx context.Context) (nullv4.Int, error) {
	return b.txStore.FindEarliestUnconfirmedTxAttemptBlock(ctx, b.chainID)
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) CountTransactionsByState(ctx context.Context, state txmgrtypes.TxState) (count uint32, err error) {
	return b.txStore.CountTransactionsByState(ctx, state, b.chainID)
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) GetTransactionStatus(ctx context.Context, transactionID string) (status commontypes.TransactionStatus, err error) {
	// Loads attempts and receipts in the transaction
	tx, err := b.txStore.FindTxWithIdempotencyKey(ctx, transactionID, b.chainID)
	if err != nil {
		return status, fmt.Errorf("failed to find transaction with IdempotencyKey %s: %w", transactionID, err)
	}
	// This check is required since a no-rows error returns nil err
	if tx == nil {
		return status, fmt.Errorf("failed to find transaction with IdempotencyKey %s", transactionID)
	}
	switch tx.State {
	case TxUnconfirmed, TxConfirmedMissingReceipt:
		// Return pending for ConfirmedMissingReceipt since a receipt is required to consider it as unconfirmed
		return commontypes.Pending, nil
	case TxConfirmed:
		// Return unconfirmed for confirmed transactions because they are not yet finalized
		return commontypes.Unconfirmed, nil
	case TxFinalized:
		return commontypes.Finalized, nil
	case TxFatalError:
		// Use an ErrorClassifier to determine if the transaction is considered Fatal
		txErr := b.newErrorClassifier(tx.GetError())
		if txErr != nil && txErr.IsFatal() {
			return commontypes.Fatal, tx.GetError()
		}
		// Return failed for all other tx's marked as FatalError
		return commontypes.Failed, tx.GetError()
	default:
		// Unstarted and InProgress are classified as unknown since they are not supported by the ChainWriter interface
		return commontypes.Unknown, nil
	}
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) GetTransactionFee(ctx context.Context, transactionID string) (fee *commontypes.TransactionFee, err error) {
	receipt, err := b.txStore.FindReceiptWithIdempotencyKey(ctx, transactionID, b.chainID)
	if err != nil {
		return fee, fmt.Errorf("failed to find receipt with IdempotencyKey %s: %w", transactionID, err)
	}

	// This check is required since a no-rows error returns nil err
	if receipt == nil {
		return fee, fmt.Errorf("failed to find receipt with IdempotencyKey %s", transactionID)
	}

	gasUsed := new(big.Int).SetUint64(receipt.GetFeeUsed())
	price := receipt.GetEffectiveGasPrice()
	totalFee := new(big.Int).Mul(gasUsed, price)

	status, err := b.GetTransactionStatus(ctx, transactionID)
	if err != nil {
		return fee, fmt.Errorf("failed to find transaction with IdempotencyKey %s: %w", transactionID, err)
	}

	if status != commontypes.Finalized {
		return fee, fmt.Errorf("tx status is not finalized")
	}

	fee = &commontypes.TransactionFee{
		TransactionFee: totalFee,
	}

	return fee, nil
}

// Deprecated: use txmgrtest.ErrTxManager
type NullTxManager[CID chains.ID, HEAD chains.Head[BHASH], ADDR chains.Hashable, THASH, BHASH chains.Hashable, SEQ chains.Sequence, FEE fees.Fee] struct {
	ErrMsg string
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) OnNewLongestChain(context.Context, HEAD) {
}

// Start does noop for NullTxManager.
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Start(context.Context) error {
	return nil
}

// Close does noop for NullTxManager.
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Close() error {
	return nil
}

// Trigger does noop for NullTxManager.
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Trigger(ADDR) {
	panic(n.ErrMsg)
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) CreateTransaction(ctx context.Context, txRequest txmgrtypes.TxRequest[ADDR, THASH]) (etx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	return etx, errors.New(n.ErrMsg)
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) GetForwarderForEOA(ctx context.Context, addr ADDR) (fwdr ADDR, err error) {
	return fwdr, err
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) GetForwarderForEOAOCR2Feeds(ctx context.Context, _, _ ADDR) (fwdr ADDR, err error) {
	return fwdr, err
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Reset(addr ADDR, abandon bool) error {
	return nil
}

// SendNativeToken does nothing, null functionality
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) SendNativeToken(ctx context.Context, chainID CID, from, to ADDR, value big.Int, gasLimit uint64) (etx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	return etx, errors.New(n.ErrMsg)
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Ready() error {
	return nil
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) Name() string {
	return "NullTxManager"
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) HealthReport() map[string]error {
	return map[string]error{}
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) RegisterResumeCallback(fn ResumeCallback) {
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) FindTxesByMetaFieldAndStates(ctx context.Context, metaField string, metaValue string, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	return txes, errors.New(n.ErrMsg)
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) FindTxesWithMetaFieldByStates(ctx context.Context, metaField string, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	return txes, errors.New(n.ErrMsg)
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) FindTxesWithMetaFieldByReceiptBlockNum(ctx context.Context, metaField string, blockNum int64, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) {
	return txes, errors.New(n.ErrMsg)
}
func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) FindTxesWithAttemptsAndReceiptsByIdsAndState(ctx context.Context, ids []int64, states []txmgrtypes.TxState, chainID *big.Int) (txes []*txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE], err error) { //nolint:revive // exported
	return txes, errors.New(n.ErrMsg)
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) FindEarliestUnconfirmedBroadcastTime(ctx context.Context) (nullv4.Time, error) {
	return nullv4.Time{}, errors.New(n.ErrMsg)
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) FindEarliestUnconfirmedTxAttemptBlock(ctx context.Context) (nullv4.Int, error) {
	return nullv4.Int{}, errors.New(n.ErrMsg)
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) CountTransactionsByState(ctx context.Context, state txmgrtypes.TxState) (count uint32, err error) {
	return count, errors.New(n.ErrMsg)
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) GetTransactionStatus(ctx context.Context, transactionID string) (status commontypes.TransactionStatus, err error) {
	return
}

func (n *NullTxManager[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) GetTransactionFee(ctx context.Context, transactionID string) (fee *commontypes.TransactionFee, err error) {
	return
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) pruneQueueAndCreateTxn(
	ctx context.Context,
	txRequest txmgrtypes.TxRequest[ADDR, THASH],
	chainID CID,
) (
	tx txmgrtypes.Tx[CID, ADDR, THASH, BHASH, SEQ, FEE],
	err error,
) {
	b.pruneQueueAndCreateLock.Lock()
	defer b.pruneQueueAndCreateLock.Unlock()

	pruned, err := txRequest.Strategy.PruneQueue(ctx, b.txStore)
	if err != nil {
		return tx, err
	}
	if len(pruned) > 0 {
		b.logger.Warnw(fmt.Sprintf("Pruned %d old unstarted transactions", len(pruned)),
			"subject", txRequest.Strategy.Subject(),
			"pruned-tx-ids", pruned,
		)
	}

	tx, err = b.txStore.CreateTransaction(ctx, txRequest, chainID)
	if err != nil {
		return tx, err
	}
	b.logger.Debugw("Created transaction",
		"fromAddress", txRequest.FromAddress,
		"toAddress", txRequest.ToAddress,
		"meta", txRequest.Meta,
		"transactionID", tx.ID,
	)

	return tx, nil
}

// newRedialBackoff is a standard backoff to use for redialling or reconnecting to
// unreachable network endpoints
func newRedialBackoff() backoff.Backoff {
	return backoff.Backoff{
		Min:    1 * time.Second,
		Max:    15 * time.Second,
		Jitter: true,
	}
}

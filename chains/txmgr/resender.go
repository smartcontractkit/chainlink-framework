package txmgr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/chains/label"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-framework/multinode"

	"github.com/smartcontractkit/chainlink-framework/chains"
	"github.com/smartcontractkit/chainlink-framework/chains/fees"
	txmgrtypes "github.com/smartcontractkit/chainlink-framework/chains/txmgr/types"
)

const (
	// pollInterval is the maximum amount of time in addition to
	// TxResendAfterThreshold that we will wait before resending an attempt
	DefaultResenderPollInterval = 5 * time.Second

	// Alert interval for unconfirmed transaction attempts
	unconfirmedTxAlertLogFrequency = 2 * time.Minute

	// timeout value for batchSendTransactions
	batchSendTransactionTimeout = 30 * time.Second
)

// Resender periodically picks up transactions that have been languishing
// unconfirmed for a configured amount of time without being sent, and sends
// their highest priced attempt again. This helps to defend against geth/parity
// silently dropping txes, or txes being ejected from the mempool.
//
// Previously we relied on the bumper to do this for us implicitly but there
// can occasionally be problems with this (e.g. abnormally long block times, or
// if gas bumping is disabled)
type Resender[CID chains.ID, ADDR chains.Hashable, THASH chains.Hashable, BHASH chains.Hashable, R txmgrtypes.ChainReceipt[THASH, BHASH], SEQ chains.Sequence, FEE fees.Fee] struct {
	txStore             txmgrtypes.TransactionStore[ADDR, CID, THASH, BHASH, SEQ, FEE]
	client              txmgrtypes.TransactionClient[CID, ADDR, THASH, BHASH, SEQ, FEE]
	tracker             *Tracker[CID, ADDR, THASH, BHASH, R, SEQ, FEE]
	ks                  txmgrtypes.KeyStore[ADDR]
	chainID             CID
	interval            time.Duration
	config              txmgrtypes.ResenderChainConfig
	txConfig            txmgrtypes.ResenderTransactionsConfig
	logger              logger.SugaredLogger
	lastAlertTimestamps map[string]time.Time

	stopCh services.StopChan
	chDone chan struct{}
}

func NewResender[
	CHAIN_ID chains.ID,
	ADDR chains.Hashable,
	TX_HASH chains.Hashable,
	BLOCK_HASH chains.Hashable,
	R txmgrtypes.ChainReceipt[TX_HASH, BLOCK_HASH],
	SEQ chains.Sequence,
	FEE fees.Fee,
](
	lggr logger.Logger,
	txStore txmgrtypes.TransactionStore[ADDR, CHAIN_ID, TX_HASH, BLOCK_HASH, SEQ, FEE],
	client txmgrtypes.TransactionClient[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE],
	tracker *Tracker[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, R, SEQ, FEE],
	ks txmgrtypes.KeyStore[ADDR],
	pollInterval time.Duration,
	config txmgrtypes.ResenderChainConfig,
	txConfig txmgrtypes.ResenderTransactionsConfig,
) *Resender[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, R, SEQ, FEE] {
	if txConfig.ResendAfterThreshold() == 0 {
		panic("Resender requires a non-zero threshold")
	}
	// todo: add context to txStore https://smartcontract-it.atlassian.net/browse/BCI-1585
	return &Resender[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, R, SEQ, FEE]{
		txStore,
		client,
		tracker,
		ks,
		client.ConfiguredChainID(),
		pollInterval,
		config,
		txConfig,
		logger.Sugared(logger.Named(lggr, "Resender")),
		make(map[string]time.Time),
		make(chan struct{}),
		make(chan struct{}),
	}
}

// Start is a comment which satisfies the linter
func (er *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) Start(ctx context.Context) {
	er.logger.Debugf("Enabled with poll interval of %s and age threshold of %s", er.interval, er.txConfig.ResendAfterThreshold())
	go er.runLoop()
}

// Stop is a comment which satisfies the linter
func (er *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) Stop() {
	close(er.stopCh)
	<-er.chDone
}

func (er *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) runLoop() {
	defer close(er.chDone)
	ctx, cancel := er.stopCh.NewCtx()
	defer cancel()

	if err := er.resendUnconfirmed(ctx); err != nil {
		er.logger.Warnw("Failed to resend unconfirmed transactions", "err", err)
	}

	ticker := services.NewTicker(er.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := er.resendUnconfirmed(ctx); err != nil {
				er.logger.Warnw("Failed to resend unconfirmed transactions", "err", err)
			}
		}
	}
}

func (er *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) resendUnconfirmed(ctx context.Context) error {
	var cancel func()
	ctx, cancel = er.stopCh.Ctx(ctx)
	defer cancel()
	resendAddresses, err := er.ks.EnabledAddresses(ctx)
	if err != nil {
		return fmt.Errorf("resender failed getting enabled keys: %w", err)
	}

	ageThreshold := er.txConfig.ResendAfterThreshold()
	maxInFlightTransactions := er.txConfig.MaxInFlight()
	olderThan := time.Now().Add(-ageThreshold)
	var allAttempts []txmgrtypes.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE]

	for _, k := range resendAddresses {
		var attempts []txmgrtypes.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE]
		attempts, err = er.txStore.FindTxAttemptsRequiringResend(ctx, olderThan, maxInFlightTransactions, er.chainID, k)
		if err != nil {
			return fmt.Errorf("failed to FindTxAttemptsRequiringResend: %w", err)
		}
		er.logStuckAttempts(attempts, k)

		allAttempts = append(allAttempts, attempts...)
	}

	if len(allAttempts) == 0 {
		for k := range er.lastAlertTimestamps {
			er.lastAlertTimestamps[k] = time.Now()
		}
		return nil
	}
	er.logger.Infow(fmt.Sprintf("Re-sending %d unconfirmed transactions that were last sent over %s ago. These transactions are taking longer than usual to be mined. %s", len(allAttempts), ageThreshold, label.NodeConnectivityProblemWarning), "n", len(allAttempts))

	batchSize := int(er.config.RPCDefaultBatchSize())
	batchCtx, batchCancel := context.WithTimeout(ctx, batchSendTransactionTimeout)
	defer batchCancel()
	txErrTypes, _, broadcastTime, txIDs, err := er.client.BatchSendTransactions(batchCtx, allAttempts, batchSize, er.logger)

	// update broadcast times before checking additional errors
	if len(txIDs) > 0 {
		if updateErr := er.txStore.UpdateBroadcastAts(ctx, broadcastTime, txIDs); updateErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to update broadcast time: %w", updateErr))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to re-send transactions: %w", err)
	}
	logResendResult(er.logger, txErrTypes)

	return nil
}

func logResendResult(lggr logger.Logger, codes []multinode.SendTxReturnCode) {
	var nNew int
	var nFatal int
	for _, c := range codes {
		if c == multinode.Successful {
			nNew++
		} else if c == multinode.Fatal {
			nFatal++
		}
	}
	lggr.Debugw("Completed", "n", len(codes), "nNew", nNew, "nFatal", nFatal)
}

func (er *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) logStuckAttempts(attempts []txmgrtypes.TxAttempt[CID, ADDR, THASH, BHASH, SEQ, FEE], fromAddress ADDR) {
	if time.Since(er.lastAlertTimestamps[fromAddress.String()]) >= unconfirmedTxAlertLogFrequency {
		oldestAttempt, exists := findOldestUnconfirmedAttempt(attempts)
		if exists {
			// Wait at least 2 times the TxResendAfterThreshold to log critical with an unconfirmedTxAlertDelay
			if time.Since(oldestAttempt.CreatedAt) > er.txConfig.ResendAfterThreshold()*2 {
				er.lastAlertTimestamps[fromAddress.String()] = time.Now()
				er.logger.Errorw("TxAttempt has been unconfirmed for more than max duration", "maxDuration", er.txConfig.ResendAfterThreshold()*2,
					"txID", oldestAttempt.TxID, "txFee", oldestAttempt.TxFee,
					"BroadcastBeforeBlockNum", oldestAttempt.BroadcastBeforeBlockNum, "Hash", oldestAttempt.Hash, "fromAddress", fromAddress)
			}
		}
	}
}

func findOldestUnconfirmedAttempt[
	CHAIN_ID chains.ID,
	ADDR chains.Hashable,
	TX_HASH, BLOCK_HASH chains.Hashable,
	SEQ chains.Sequence,
	FEE fees.Fee,
](attempts []txmgrtypes.TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) (txmgrtypes.TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], bool) {
	var oldestAttempt txmgrtypes.TxAttempt[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]
	if len(attempts) < 1 {
		return oldestAttempt, false
	}
	oldestAttempt = attempts[0]
	for i := 1; i < len(attempts); i++ {
		if oldestAttempt.CreatedAt.Sub(attempts[i].CreatedAt) <= 0 {
			oldestAttempt = attempts[i]
		}
	}
	return oldestAttempt, true
}

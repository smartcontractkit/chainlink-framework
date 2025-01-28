package txmgrtest

import (
	"context"
	"math/big"

	"gopkg.in/guregu/null.v4"

	commontypes "github.com/smartcontractkit/chainlink-common/pkg/types"
	"github.com/smartcontractkit/chainlink-framework/chains"
	"github.com/smartcontractkit/chainlink-framework/chains/fees"
	"github.com/smartcontractkit/chainlink-framework/chains/txmgr"
	"github.com/smartcontractkit/chainlink-framework/chains/txmgr/types"
)

type ErrTxManager[
	CHAIN_ID chains.ID,
	HEAD chains.Head[BLOCK_HASH],
	ADDR chains.Hashable,
	TX_HASH, BLOCK_HASH chains.Hashable,
	SEQ chains.Sequence,
	FEE fees.Fee,
] struct {
	Err error
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) OnNewLongestChain(context.Context, HEAD) {
}

// Start does noop for ErrTxManager.
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) Start(context.Context) error {
	return nil
}

// Close does noop for ErrTxManager.
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) Close() error {
	return nil
}

// Trigger does noop for ErrTxManager.
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) Trigger(ADDR) {
	panic(n.Err)
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) CreateTransaction(ctx context.Context, txRequest types.TxRequest[ADDR, TX_HASH]) (etx types.Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error) {
	return etx, n.Err
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) GetForwarderForEOA(ctx context.Context, addr ADDR) (fwdr ADDR, err error) {
	return fwdr, err
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) GetForwarderForEOAOCR2Feeds(ctx context.Context, _, _ ADDR) (fwdr ADDR, err error) {
	return fwdr, err
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) Reset(addr ADDR, abandon bool) error {
	return nil
}

// SendNativeToken does nothing, null functionality
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) SendNativeToken(ctx context.Context, chainID CHAIN_ID, from, to ADDR, value big.Int, gasLimit uint64) (etx types.Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error) {
	return etx, n.Err
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) Ready() error {
	return nil
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) Name() string {
	return "ErrTxManager"
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) HealthReport() map[string]error {
	return map[string]error{}
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) RegisterResumeCallback(fn txmgr.ResumeCallback) {
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindTxesByMetaFieldAndStates(ctx context.Context, metaField string, metaValue string, states []types.TxState, chainID *big.Int) (txes []*types.Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error) {
	return txes, n.Err
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindTxesWithMetaFieldByStates(ctx context.Context, metaField string, states []types.TxState, chainID *big.Int) (txes []*types.Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error) {
	return txes, n.Err
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindTxesWithMetaFieldByReceiptBlockNum(ctx context.Context, metaField string, blockNum int64, chainID *big.Int) (txes []*types.Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error) {
	return txes, n.Err
}
func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindTxesWithAttemptsAndReceiptsByIdsAndState(ctx context.Context, ids []int64, states []types.TxState, chainID *big.Int) (txes []*types.Tx[CHAIN_ID, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE], err error) {
	return txes, n.Err
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindEarliestUnconfirmedBroadcastTime(ctx context.Context) (null.Time, error) {
	return null.Time{}, n.Err
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) FindEarliestUnconfirmedTxAttemptBlock(ctx context.Context) (null.Int, error) {
	return null.Int{}, n.Err
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) CountTransactionsByState(ctx context.Context, state types.TxState) (count uint32, err error) {
	return count, n.Err
}

func (n *ErrTxManager[CHAIN_ID, HEAD, ADDR, TX_HASH, BLOCK_HASH, SEQ, FEE]) GetTransactionStatus(ctx context.Context, transactionID string) (status commontypes.TransactionStatus, err error) {
	return
}

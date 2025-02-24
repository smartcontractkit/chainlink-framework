package txmgr

import (
	"context"
	"time"

	"github.com/smartcontractkit/chainlink-framework/chains/txmgr/types"
)

// TEST ONLY FUNCTIONS
// these need to be exported for the txmgr tests to continue to work

func (ec *Confirmer[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXTestSetClient(client types.TxmClient[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) {
	ec.client = client
}

func (tr *Tracker[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXTestSetTTL(ttl time.Duration) {
	tr.ttl = ttl
}

func (tr *Tracker[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXDeliverBlock(blockHeight int64) {
	tr.mb.Deliver(blockHeight)
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) XXXTestStartInternal(ctx context.Context) error {
	return eb.startInternal(ctx)
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) XXXTestCloseInternal() error {
	return eb.closeInternal()
}

func (eb *Broadcaster[CID, HEAD, ADDR, THASH, BHASH, SEQ, FEE]) XXXTestDisableUnstartedTxAutoProcessing() {
	eb.processUnstartedTxsImpl = func(ctx context.Context, fromAddress ADDR) (retryable bool, err error) { return false, nil }
}

func (ec *Confirmer[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXTestStartInternal() error {
	ctx, cancel := ec.stopCh.NewCtx()
	defer cancel()
	return ec.startInternal(ctx)
}

func (ec *Confirmer[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXTestCloseInternal() error {
	return ec.closeInternal()
}

func (er *Resender[CID, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXTestResendUnconfirmed() error {
	ctx, cancel := er.stopCh.NewCtx()
	defer cancel()
	return er.resendUnconfirmed(ctx)
}

func (b *Txm[CID, HEAD, ADDR, THASH, BHASH, R, SEQ, FEE]) XXXTestAbandon(addr ADDR) (err error) {
	return b.abandon(addr)
}

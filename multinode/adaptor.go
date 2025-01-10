package multinode

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

// Adapter is used to integrate multinode into chain-specific clients
type Adapter[RPC any, HEAD Head] struct {
	log         logger.Logger
	rpc         *RPC
	ctxTimeout  time.Duration
	stateMu     sync.RWMutex // protects state* fields
	subsSliceMu sync.RWMutex
	subs        map[Subscription]struct{}

	latestBlock          func(ctx context.Context, rpc *RPC) (HEAD, error)
	latestFinalizedBlock func(ctx context.Context, rpc *RPC) (HEAD, error)

	// chStopInFlight can be closed to immediately cancel all in-flight requests on
	// this RpcMultiNodeAdapter. Closing and replacing should be serialized through
	// stateMu since it can happen on state transitions as well as RpcMultiNodeAdapter Close.
	chStopInFlight chan struct{}

	chainInfoLock sync.RWMutex
	// intercepted values seen by callers of the rpcMultiNodeAdapter excluding health check calls. Need to ensure MultiNode provides repeatable read guarantee
	highestUserObservations ChainInfo
	// most recent chain info observed during current lifecycle (reseted on DisconnectAll)
	latestChainInfo ChainInfo
}

func NewAdapter[RPC any, HEAD Head](
	rpc *RPC, ctxTimeout time.Duration, log logger.Logger,
	latestBlock func(ctx context.Context, rpc *RPC) (HEAD, error),
	latestFinalizedBlock func(ctx context.Context, rpc *RPC) (HEAD, error),
) *Adapter[RPC, HEAD] {
	return &Adapter[RPC, HEAD]{
		rpc:                  rpc,
		log:                  log,
		ctxTimeout:           ctxTimeout,
		latestBlock:          latestBlock,
		latestFinalizedBlock: latestFinalizedBlock,
		subs:                 make(map[Subscription]struct{}),
		chStopInFlight:       make(chan struct{}),
	}
}

func (m *Adapter[RPC, HEAD]) LenSubs() int {
	m.subsSliceMu.RLock()
	defer m.subsSliceMu.RUnlock()
	return len(m.subs)
}

// RegisterSub adds the sub to the Adaptor list and returns a sub which is managed on unsubscribe
func (m *Adapter[RPC, HEAD]) RegisterSub(sub Subscription, stopInFLightCh chan struct{}) (*ManagedSubscription, error) {
	// ensure that the `sub` belongs to current life cycle of the `rpcMultiNodeAdapter` and it should not be killed due to
	// previous `DisconnectAll` call.
	select {
	case <-stopInFLightCh:
		sub.Unsubscribe()
		return nil, fmt.Errorf("failed to register subscription - all in-flight requests were canceled")
	default:
	}
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()
	managedSub := &ManagedSubscription{
		sub,
		m.removeSub,
	}
	m.subs[managedSub] = struct{}{}
	return managedSub, nil
}

func (m *Adapter[RPC, HEAD]) removeSub(sub Subscription) {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()
	delete(m.subs, sub)
}

func (m *Adapter[RPC, HEAD]) LatestBlock(ctx context.Context) (HEAD, error) {
	// capture chStopInFlight to ensure we are not updating chainInfo with observations related to previous life cycle
	ctx, cancel, chStopInFlight, rpc := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestBlock(ctx, rpc)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.onNewHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *Adapter[RPC, HEAD]) LatestFinalizedBlock(ctx context.Context) (HEAD, error) {
	ctx, cancel, chStopInFlight, rpc := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestFinalizedBlock(ctx, rpc)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.onNewFinalizedHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *Adapter[RPC, HEAD]) onNewHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
	if !head.IsValid() {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !CtxIsHeathCheckRequest(ctx) {
		m.highestUserObservations.BlockNumber = max(m.highestUserObservations.BlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcMultiNodeAdapter already started new life cycle
		return
	default:
		m.latestChainInfo.BlockNumber = head.BlockNumber()
	}
}

func (m *Adapter[RPC, HEAD]) onNewFinalizedHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
	if !head.IsValid() {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !CtxIsHeathCheckRequest(ctx) {
		m.highestUserObservations.FinalizedBlockNumber = max(m.highestUserObservations.FinalizedBlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update latestChainInfo, as rpcMultiNodeAdapter already started new life cycle
		return
	default:
		m.latestChainInfo.FinalizedBlockNumber = head.BlockNumber()
	}
}

// MakeQueryCtx returns a context that cancels if:
// 1. Passed in ctx cancels
// 2. Passed in channel is closed
// 3. Default timeout is reached (queryTimeout)
func MakeQueryCtx(ctx context.Context, ch services.StopChan, timeout time.Duration) (context.Context, context.CancelFunc) {
	var chCancel, timeoutCancel context.CancelFunc
	ctx, chCancel = ch.Ctx(ctx)
	ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
	cancel := func() {
		chCancel()
		timeoutCancel()
	}
	return ctx, cancel
}

func (m *Adapter[RPC, HEAD]) AcquireQueryCtx(parentCtx context.Context, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc,
	chStopInFlight chan struct{}, raw *RPC) {
	// Need to wrap in mutex because state transition can cancel and replace context
	m.stateMu.RLock()
	chStopInFlight = m.chStopInFlight
	cp := *m.rpc
	raw = &cp
	m.stateMu.RUnlock()
	ctx, cancel = MakeQueryCtx(parentCtx, chStopInFlight, timeout)
	return
}

func (m *Adapter[RPC, HEAD]) UnsubscribeAllExcept(subs ...Subscription) {
	m.subsSliceMu.Lock()
	keepSubs := map[Subscription]struct{}{}
	for _, sub := range subs {
		keepSubs[sub] = struct{}{}
	}

	var unsubs []Subscription
	for sub := range m.subs {
		if _, keep := keepSubs[sub]; !keep {
			unsubs = append(unsubs, sub)
		}
	}
	m.subsSliceMu.Unlock()

	for _, sub := range unsubs {
		sub.Unsubscribe()
	}
}

// cancelInflightRequests closes and replaces the chStopInFlight
func (m *Adapter[RPC, HEAD]) cancelInflightRequests() {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	close(m.chStopInFlight)
	m.chStopInFlight = make(chan struct{})
}

func (m *Adapter[RPC, HEAD]) Close() {
	m.cancelInflightRequests()
	m.UnsubscribeAllExcept()
	m.chainInfoLock.Lock()
	m.latestChainInfo = ChainInfo{}
	m.chainInfoLock.Unlock()
}

func (m *Adapter[RPC, HEAD]) GetInterceptedChainInfo() (latest, highestUserObservations ChainInfo) {
	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	return m.latestChainInfo, m.highestUserObservations
}

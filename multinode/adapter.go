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

type AdapterConfig interface {
	NewHeadsPollInterval() time.Duration
	FinalizedBlockPollInterval() time.Duration
}

// Adapter is used to integrate multinode into chain-specific clients.
// For new MultiNode integrations, we wrap the RPC client and inherit from the Adapter
// to get the required RPCClient methods and enable the use of MultiNode.
//
// The Adapter provides chain-agnostic functionality such as head and finalized head
// subscriptions, which are required in each Node lifecycle to execute various
// health checks.
type Adapter[HEAD Head] struct {
	cfg        AdapterConfig
	log        logger.Logger
	ctxTimeout time.Duration
	subsMu     sync.RWMutex
	subs       map[Subscription]struct{}

	latestBlock          func(ctx context.Context) (HEAD, error)
	latestFinalizedBlock func(ctx context.Context) (HEAD, error)

	// lifeCycleCh can be closed to immediately cancel all in-flight requests on
	// this RPC. Closing and replacing should be serialized through
	// lifeCycleMu since it can happen on state transitions as well as Adapter Close.
	// Also closed when RPC is declared unhealthy.
	lifeCycleMu sync.RWMutex
	lifeCycleCh chan struct{}

	chainInfoLock sync.RWMutex
	// intercepted values seen by callers of the Adapter excluding health check calls. Need to ensure MultiNode provides repeatable read guarantee
	chainInfoHighestUserObservations ChainInfo
	// most recent chain info observed during current lifecycle
	chainInfoLatest ChainInfo
}

func NewAdapter[HEAD Head](
	cfg AdapterConfig, ctxTimeout time.Duration, log logger.Logger,
	latestBlock func(ctx context.Context) (HEAD, error),
	latestFinalizedBlock func(ctx context.Context) (HEAD, error),
) *Adapter[HEAD] {
	return &Adapter[HEAD]{
		cfg:                  cfg,
		log:                  log,
		ctxTimeout:           ctxTimeout,
		latestBlock:          latestBlock,
		latestFinalizedBlock: latestFinalizedBlock,
		subs:                 make(map[Subscription]struct{}),
		lifeCycleCh:          make(chan struct{}),
	}
}

func (m *Adapter[HEAD]) LenSubs() int {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()
	return len(m.subs)
}

// RegisterSub adds the sub to the Adapter list and returns a managed sub which is removed on unsubscribe
func (m *Adapter[HEAD]) RegisterSub(sub Subscription, lifeCycleCh chan struct{}) (*ManagedSubscription, error) {
	// ensure that the `sub` belongs to current life cycle of the `rpcMultiNodeAdapter` and it should not be killed due to
	// previous `DisconnectAll` call.
	select {
	case <-lifeCycleCh:
		sub.Unsubscribe()
		return nil, fmt.Errorf("failed to register subscription - all in-flight requests were canceled")
	default:
	}
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	managedSub := &ManagedSubscription{
		sub,
		m.removeSub,
	}
	m.subs[managedSub] = struct{}{}
	return managedSub, nil
}

func (m *Adapter[HEAD]) removeSub(sub Subscription) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	delete(m.subs, sub)
}

func (m *Adapter[HEAD]) SubscribeToHeads(ctx context.Context) (<-chan HEAD, Subscription, error) {
	ctx, cancel, lifeCycleCh := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	pollInterval := m.cfg.NewHeadsPollInterval()
	if pollInterval == 0 {
		return nil, nil, errors.New("PollInterval is 0")
	}
	timeout := pollInterval
	poller, channel := NewPoller[HEAD](pollInterval, func(pollRequestCtx context.Context) (HEAD, error) {
		if CtxIsHealthCheckRequest(ctx) {
			pollRequestCtx = CtxAddHealthCheckFlag(pollRequestCtx)
		}
		return m.LatestBlock(pollRequestCtx)
	}, timeout, m.log)

	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	sub, err := m.RegisterSub(&poller, lifeCycleCh)
	if err != nil {
		sub.Unsubscribe()
		return nil, nil, err
	}
	return channel, sub, nil
}

func (m *Adapter[HEAD]) SubscribeToFinalizedHeads(ctx context.Context) (<-chan HEAD, Subscription, error) {
	ctx, cancel, lifeCycleCh := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	finalizedBlockPollInterval := m.cfg.FinalizedBlockPollInterval()
	if finalizedBlockPollInterval == 0 {
		return nil, nil, errors.New("FinalizedBlockPollInterval is 0")
	}
	timeout := finalizedBlockPollInterval
	poller, channel := NewPoller[HEAD](finalizedBlockPollInterval, func(pollRequestCtx context.Context) (HEAD, error) {
		if CtxIsHealthCheckRequest(ctx) {
			pollRequestCtx = CtxAddHealthCheckFlag(pollRequestCtx)
		}
		return m.LatestFinalizedBlock(pollRequestCtx)
	}, timeout, m.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	sub, err := m.RegisterSub(&poller, lifeCycleCh)
	if err != nil {
		poller.Unsubscribe()
		return nil, nil, err
	}
	return channel, sub, nil
}

func (m *Adapter[HEAD]) LatestBlock(ctx context.Context) (HEAD, error) {
	// capture lifeCycleCh to ensure we are not updating chainInfo with observations related to previous life cycle
	ctx, cancel, lifeCycleCh := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestBlock(ctx)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.OnNewHead(ctx, lifeCycleCh, head)
	return head, nil
}

func (m *Adapter[HEAD]) LatestFinalizedBlock(ctx context.Context) (HEAD, error) {
	ctx, cancel, lifeCycleCh := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestFinalizedBlock(ctx)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.OnNewFinalizedHead(ctx, lifeCycleCh, head)
	return head, nil
}

func (m *Adapter[HEAD]) OnNewHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
	if !head.IsValid() {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	blockNumber := head.BlockNumber()
	totalDifficulty := head.GetTotalDifficulty()
	if !CtxIsHealthCheckRequest(ctx) {
		m.chainInfoHighestUserObservations.BlockNumber = max(m.chainInfoHighestUserObservations.BlockNumber, blockNumber)
		m.chainInfoHighestUserObservations.TotalDifficulty = MaxTotalDifficulty(m.chainInfoHighestUserObservations.TotalDifficulty, totalDifficulty)
	}
	select {
	case <-requestCh: // no need to update chainInfoLatest, as rpcMultiNodeAdapter already started new life cycle
		return
	default:
		m.chainInfoLatest.BlockNumber = blockNumber
		m.chainInfoLatest.TotalDifficulty = totalDifficulty
	}
}

func (m *Adapter[HEAD]) OnNewFinalizedHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
	if !head.IsValid() {
		return
	}

	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	if !CtxIsHealthCheckRequest(ctx) {
		m.chainInfoHighestUserObservations.FinalizedBlockNumber = max(m.chainInfoHighestUserObservations.FinalizedBlockNumber, head.BlockNumber())
	}
	select {
	case <-requestCh: // no need to update chainInfoLatest, as rpcMultiNodeAdapter already started new life cycle
		return
	default:
		m.chainInfoLatest.FinalizedBlockNumber = head.BlockNumber()
	}
}

// makeQueryCtx returns a context that cancels if:
// 1. Passed in ctx cancels
// 2. Passed in channel is closed
// 3. Default timeout is reached (queryTimeout)
func makeQueryCtx(ctx context.Context, ch services.StopChan, timeout time.Duration) (context.Context, context.CancelFunc) {
	var chCancel, timeoutCancel context.CancelFunc
	ctx, chCancel = ch.Ctx(ctx)
	ctx, timeoutCancel = context.WithTimeout(ctx, timeout)
	cancel := func() {
		chCancel()
		timeoutCancel()
	}
	return ctx, cancel
}

func (m *Adapter[HEAD]) AcquireQueryCtx(parentCtx context.Context, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc,
	lifeCycleCh chan struct{}) {
	// Need to wrap in mutex because state transition can cancel and replace context
	m.lifeCycleMu.RLock()
	lifeCycleCh = m.lifeCycleCh
	m.lifeCycleMu.RUnlock()
	ctx, cancel = makeQueryCtx(parentCtx, lifeCycleCh, timeout)
	return
}

func (m *Adapter[HEAD]) UnsubscribeAllExcept(subs ...Subscription) {
	m.subsMu.Lock()
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
	m.subsMu.Unlock()

	for _, sub := range unsubs {
		sub.Unsubscribe()
	}
}

// CancelLifeCycle closes and replaces the lifeCycleCh
func (m *Adapter[HEAD]) CancelLifeCycle() {
	m.lifeCycleMu.Lock()
	defer m.lifeCycleMu.Unlock()
	close(m.lifeCycleCh)
	m.lifeCycleCh = make(chan struct{})
}

func (m *Adapter[HEAD]) resetLatestChainInfo() {
	m.chainInfoLock.Lock()
	m.chainInfoLatest = ChainInfo{}
	m.chainInfoLock.Unlock()
}

func (m *Adapter[HEAD]) Close() {
	m.CancelLifeCycle()
	m.UnsubscribeAllExcept()
	m.resetLatestChainInfo()
}

func (m *Adapter[HEAD]) GetInterceptedChainInfo() (latest, highestUserObservations ChainInfo) {
	m.chainInfoLock.RLock()
	defer m.chainInfoLock.RUnlock()
	return m.chainInfoLatest, m.chainInfoHighestUserObservations
}

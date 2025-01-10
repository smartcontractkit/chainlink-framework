package multinode

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
	"github.com/smartcontractkit/chainlink-framework/multinode/config"
)

// MultiNodeAdapter is used to integrate multinode into chain-specific clients
type MultiNodeAdapter[RPC any, HEAD Head] struct {
	cfg         *config.MultiNodeConfig
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

func NewMultiNodeAdapter[RPC any, HEAD Head](
	cfg *config.MultiNodeConfig, rpc *RPC, ctxTimeout time.Duration, log logger.Logger,
	latestBlock func(ctx context.Context, rpc *RPC) (HEAD, error),
	latestFinalizedBlock func(ctx context.Context, rpc *RPC) (HEAD, error),
) (*MultiNodeAdapter[RPC, HEAD], error) {
	return &MultiNodeAdapter[RPC, HEAD]{
		cfg:                  cfg,
		rpc:                  rpc,
		log:                  log,
		ctxTimeout:           ctxTimeout,
		latestBlock:          latestBlock,
		latestFinalizedBlock: latestFinalizedBlock,
		subs:                 make(map[Subscription]struct{}),
		chStopInFlight:       make(chan struct{}),
	}, nil
}

func (m *MultiNodeAdapter[RPC, HEAD]) LenSubs() int {
	m.subsSliceMu.RLock()
	defer m.subsSliceMu.RUnlock()
	return len(m.subs)
}

// registerSub adds the sub to the rpcMultiNodeAdapter list
func (m *MultiNodeAdapter[RPC, HEAD]) registerSub(sub Subscription, stopInFLightCh chan struct{}) error {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()
	// ensure that the `sub` belongs to current life cycle of the `rpcMultiNodeAdapter` and it should not be killed due to
	// previous `DisconnectAll` call.
	select {
	case <-stopInFLightCh:
		sub.Unsubscribe()
		return fmt.Errorf("failed to register subscription - all in-flight requests were canceled")
	default:
	}
	// TODO: BCI-3358 - delete sub when caller unsubscribes.
	m.subs[sub] = struct{}{}
	return nil
}

func (m *MultiNodeAdapter[RPC, HEAD]) LatestBlock(ctx context.Context) (HEAD, error) {
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

func (m *MultiNodeAdapter[RPC, HEAD]) LatestFinalizedBlock(ctx context.Context) (HEAD, error) {
	ctx, cancel, chStopInFlight, rpc := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	head, err := m.latestFinalizedBlock(ctx, rpc)
	if err != nil {
		return head, err
	}

	if !head.IsValid() {
		return head, errors.New("invalid head")
	}

	m.OnNewFinalizedHead(ctx, chStopInFlight, head)
	return head, nil
}

func (m *MultiNodeAdapter[RPC, HEAD]) SubscribeToHeads(ctx context.Context) (<-chan HEAD, Subscription, error) {
	ctx, cancel, chStopInFlight, _ := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	// TODO: BCFR-1070 - Add BlockPollInterval
	pollInterval := m.cfg.FinalizedBlockPollInterval() // Use same interval as finalized polling
	if pollInterval == 0 {
		return nil, nil, errors.New("PollInterval is 0")
	}
	timeout := pollInterval
	poller, channel := NewPoller[HEAD](pollInterval, func(pollRequestCtx context.Context) (HEAD, error) {
		if CtxIsHeathCheckRequest(ctx) {
			pollRequestCtx = CtxAddHealthCheckFlag(pollRequestCtx)
		}
		return m.LatestBlock(pollRequestCtx)
	}, timeout, m.log)

	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	err := m.registerSub(&poller, chStopInFlight)
	if err != nil {
		poller.Unsubscribe()
		return nil, nil, err
	}

	return channel, &poller, nil
}

func (m *MultiNodeAdapter[RPC, HEAD]) SubscribeToFinalizedHeads(ctx context.Context) (<-chan HEAD, Subscription, error) {
	ctx, cancel, chStopInFlight, _ := m.AcquireQueryCtx(ctx, m.ctxTimeout)
	defer cancel()

	finalizedBlockPollInterval := m.cfg.FinalizedBlockPollInterval()
	if finalizedBlockPollInterval == 0 {
		return nil, nil, errors.New("FinalizedBlockPollInterval is 0")
	}
	timeout := finalizedBlockPollInterval
	poller, channel := NewPoller[HEAD](finalizedBlockPollInterval, func(pollRequestCtx context.Context) (HEAD, error) {
		if CtxIsHeathCheckRequest(ctx) {
			pollRequestCtx = CtxAddHealthCheckFlag(pollRequestCtx)
		}
		return m.LatestFinalizedBlock(pollRequestCtx)
	}, timeout, m.log)
	if err := poller.Start(ctx); err != nil {
		return nil, nil, err
	}

	err := m.registerSub(&poller, chStopInFlight)
	if err != nil {
		poller.Unsubscribe()
		return nil, nil, err
	}

	return channel, &poller, nil
}

func (m *MultiNodeAdapter[RPC, HEAD]) onNewHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
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

func (m *MultiNodeAdapter[RPC, HEAD]) OnNewFinalizedHead(ctx context.Context, requestCh <-chan struct{}, head HEAD) {
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

func (m *MultiNodeAdapter[RPC, HEAD]) AcquireQueryCtx(parentCtx context.Context, timeout time.Duration) (ctx context.Context, cancel context.CancelFunc,
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

func (m *MultiNodeAdapter[RPC, HEAD]) UnsubscribeAllExcept(subs ...Subscription) {
	m.subsSliceMu.Lock()
	defer m.subsSliceMu.Unlock()

	keepSubs := map[Subscription]struct{}{}
	for _, sub := range subs {
		keepSubs[sub] = struct{}{}
	}

	for sub := range m.subs {
		if _, keep := keepSubs[sub]; !keep {
			sub.Unsubscribe()
			delete(m.subs, sub)
		}
	}
}

// cancelInflightRequests closes and replaces the chStopInFlight
func (m *MultiNodeAdapter[RPC, HEAD]) cancelInflightRequests() {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	close(m.chStopInFlight)
	m.chStopInFlight = make(chan struct{})
}

func (m *MultiNodeAdapter[RPC, HEAD]) Close() {
	m.cancelInflightRequests()
	m.UnsubscribeAllExcept()
	m.chainInfoLock.Lock()
	m.latestChainInfo = ChainInfo{}
	m.chainInfoLock.Unlock()
}

func (m *MultiNodeAdapter[RPC, HEAD]) GetInterceptedChainInfo() (latest, highestUserObservations ChainInfo) {
	m.chainInfoLock.Lock()
	defer m.chainInfoLock.Unlock()
	return m.latestChainInfo, m.highestUserObservations
}

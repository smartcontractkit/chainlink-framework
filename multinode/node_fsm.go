package multinode

import (
	"fmt"
)

// nodeState represents the current state of the node
// Node is a FSM (finite state machine)
type nodeState int

// isInitializing returns true if the node is still doing the initial dial & verification.
func (n nodeState) isInitializing() bool {
	return n == nodeStateUndialed || n == nodeStateDialed
}

func (n nodeState) String() string {
	switch n {
	case nodeStateUndialed:
		return "Undialed"
	case nodeStateDialed:
		return "Dialed"
	case nodeStateInvalidChainID:
		return "InvalidChainID"
	case nodeStateAlive:
		return "Alive"
	case nodeStateUnreachable:
		return "Unreachable"
	case nodeStateUnusable:
		return "Unusable"
	case nodeStateOutOfSync:
		return "OutOfSync"
	case nodeStateClosed:
		return "Closed"
	case nodeStateSyncing:
		return "Syncing"
	case nodeStateFinalizedBlockOutOfSync:
		return "FinalizedBlockOutOfSync"
	default:
		return fmt.Sprintf("nodeState(%d)", n)
	}
}

// GoString prints a prettier state
func (n nodeState) GoString() string {
	return fmt.Sprintf("nodeState%s(%d)", n.String(), n)
}

const (
	// nodeStateUndialed is the first state of a virgin node
	nodeStateUndialed = nodeState(iota)
	// nodeStateDialed is after a node has successfully dialed but before it has verified the correct chain ID
	nodeStateDialed
	// nodeStateInvalidChainID is after chain ID verification failed
	nodeStateInvalidChainID
	// nodeStateAlive is a healthy node after chain ID verification succeeded
	nodeStateAlive
	// nodeStateUnreachable is a node that cannot be dialed or has disconnected
	nodeStateUnreachable
	// nodeStateOutOfSync is a node that is accepting connections but exceeded
	// the failure threshold without sending any new heads. It will be
	// disconnected, then put into a revive loop and re-awakened after redial
	// if a new head arrives
	nodeStateOutOfSync
	// nodeStateUnusable is a sendonly node that has an invalid URL that can never be reached
	nodeStateUnusable
	// nodeStateClosed is after the connection has been closed and the node is at the end of its lifecycle
	nodeStateClosed
	// nodeStateSyncing is a node that is actively back-filling blockchain. Usually, it's a newly set up node that is
	// still syncing the chain. The main difference from `nodeStateOutOfSync` is that it represents state relative
	// to other primary nodes configured in the MultiNode. In contrast, `nodeStateSyncing` represents the internal state of
	// the node (RPC).
	nodeStateSyncing
	// nodeStateFinalizedBlockOutOfSync - node is lagging behind on latest finalized block
	nodeStateFinalizedBlockOutOfSync
	// nodeStateLen tracks the number of states
	nodeStateLen
)

// allNodeStates represents all possible states a node can be in
var allNodeStates []nodeState

func init() {
	for s := nodeState(0); s < nodeStateLen; s++ {
		allNodeStates = append(allNodeStates, s)
	}
}

// FSM methods

// State allows reading the current state of the node.
func (n *node[CHAIN_ID, HEAD, RPC]) State() nodeState {
	n.stateMu.RLock()
	defer n.stateMu.RUnlock()
	return n.recalculateState()
}

func (n *node[CHAIN_ID, HEAD, RPC]) getCachedState() nodeState {
	n.stateMu.RLock()
	defer n.stateMu.RUnlock()
	return n.state
}

func (n *node[CHAIN_ID, HEAD, RPC]) recalculateState() nodeState {
	if n.state != nodeStateAlive {
		return n.state
	}

	// double check that node is not lagging on finalized block
	if n.nodePoolCfg.EnforceRepeatableRead() && n.isFinalizedBlockOutOfSync() {
		return nodeStateFinalizedBlockOutOfSync
	}

	return nodeStateAlive
}

func (n *node[CHAIN_ID, HEAD, RPC]) isFinalizedBlockOutOfSync() bool {
	if n.poolInfoProvider == nil {
		return false
	}

	highestObservedByCaller := n.poolInfoProvider.HighestUserObservations()
	latest, rpcHighest := n.rpc.GetInterceptedChainInfo()
	isOutOfSync := false
	if n.chainCfg.FinalityTagEnabled() {
		isOutOfSync = latest.FinalizedBlockNumber < highestObservedByCaller.FinalizedBlockNumber-int64(n.chainCfg.FinalizedBlockOffset())
	} else {
		isOutOfSync = latest.BlockNumber < highestObservedByCaller.BlockNumber-int64(n.chainCfg.FinalizedBlockOffset())
	}

	if isOutOfSync {
		n.lfcLog.Debugw("finalized block is out of sync", "rpcLatestChainInfo", latest, "rpcHighest", rpcHighest, "highestObservedByCaller", highestObservedByCaller)
	}

	return isOutOfSync
}

// StateAndLatest returns nodeState with the latest ChainInfo observed by Node during current lifecycle.
func (n *node[CHAIN_ID, HEAD, RPC]) StateAndLatest() (nodeState, ChainInfo) {
	n.stateMu.RLock()
	defer n.stateMu.RUnlock()
	latest, _ := n.rpc.GetInterceptedChainInfo()
	return n.recalculateState(), latest
}

// HighestUserObservations - returns highest ChainInfo ever observed by external user of the Node
func (n *node[CHAIN_ID, HEAD, RPC]) HighestUserObservations() ChainInfo {
	_, highestUserObservations := n.rpc.GetInterceptedChainInfo()
	return highestUserObservations
}
func (n *node[CHAIN_ID, HEAD, RPC]) SetPoolChainInfoProvider(poolInfoProvider PoolChainInfoProvider) {
	n.poolInfoProvider = poolInfoProvider
}

// setState is only used by internal state management methods.
// This is low-level; care should be taken by the caller to ensure the new state is a valid transition.
// State changes should always be synchronous: only one goroutine at a time should change state.
// n.stateMu should not be locked for long periods of time because external clients expect a timely response from n.State()
func (n *node[CHAIN_ID, HEAD, RPC]) setState(s nodeState) {
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	n.state = s
}

// declareXXX methods change the state and pass conrol off the new state
// management goroutine

func (n *node[CHAIN_ID, HEAD, RPC]) declareAlive() {
	n.transitionToAlive(func() {
		n.lfcLog.Infow("RPC Node is online", "nodeState", n.state)
		n.wg.Add(1)
		go n.aliveLoop()
	})
}

func (n *node[CHAIN_ID, HEAD, RPC]) transitionToAlive(fn func()) {
	ctx, cancel := n.stopCh.NewCtx()
	defer cancel()
	n.metrics.IncrementNodeTransitionsToAlive(ctx, n.name)
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.state == nodeStateClosed {
		return
	}
	switch n.state {
	case nodeStateDialed, nodeStateInvalidChainID, nodeStateSyncing:
		n.state = nodeStateAlive
	default:
		panic(transitionFail(n.state, nodeStateAlive))
	}
	fn()
}

// declareInSync puts a node back into Alive state, allowing it to be used by
// pool consumers again
func (n *node[CHAIN_ID, HEAD, RPC]) declareInSync() {
	n.transitionToInSync(func() {
		n.lfcLog.Infow("RPC Node is back in sync", "nodeState", n.state)
		n.wg.Add(1)
		go n.aliveLoop()
	})
}

func (n *node[CHAIN_ID, HEAD, RPC]) transitionToInSync(fn func()) {
	ctx, cancel := n.stopCh.NewCtx()
	defer cancel()
	n.metrics.IncrementNodeTransitionsToAlive(ctx, n.name)
	n.metrics.IncrementNodeTransitionsToInSync(ctx, n.name)
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.state == nodeStateClosed {
		return
	}
	switch n.state {
	case nodeStateOutOfSync, nodeStateSyncing:
		n.state = nodeStateAlive
	default:
		panic(transitionFail(n.state, nodeStateAlive))
	}
	fn()
}

// declareOutOfSync puts a node into OutOfSync state, disconnecting all current
// clients and making it unavailable for use until back in-sync.
func (n *node[CHAIN_ID, HEAD, RPC]) declareOutOfSync(syncIssues syncStatus) {
	n.transitionToOutOfSync(func() {
		n.lfcLog.Errorw("RPC Node is out of sync", "nodeState", n.state, "syncIssues", syncIssues)
		n.wg.Add(1)
		go n.outOfSyncLoop(syncIssues)
	})
}

func (n *node[CHAIN_ID, HEAD, RPC]) transitionToOutOfSync(fn func()) {
	ctx, cancel := n.stopCh.NewCtx()
	defer cancel()
	n.metrics.IncrementNodeTransitionsToOutOfSync(ctx, n.name)
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.state == nodeStateClosed {
		return
	}
	switch n.state {
	case nodeStateAlive, nodeStateOutOfSync:
		n.rpc.Close()
		n.state = nodeStateOutOfSync
	default:
		panic(transitionFail(n.state, nodeStateOutOfSync))
	}
	fn()
}

func (n *node[CHAIN_ID, HEAD, RPC]) declareUnreachable() {
	n.transitionToUnreachable(func() {
		n.lfcLog.Errorw("RPC Node is unreachable", "nodeState", n.state)
		n.wg.Add(1)
		go n.unreachableLoop()
	})
}

func (n *node[CHAIN_ID, HEAD, RPC]) transitionToUnreachable(fn func()) {
	ctx, cancel := n.stopCh.NewCtx()
	defer cancel()
	n.metrics.IncrementNodeTransitionsToUnreachable(ctx, n.name)
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.state == nodeStateClosed {
		return
	}
	switch n.state {
	case nodeStateUndialed, nodeStateDialed, nodeStateAlive, nodeStateOutOfSync, nodeStateInvalidChainID, nodeStateSyncing:
		n.rpc.Close()
		n.state = nodeStateUnreachable
	default:
		panic(transitionFail(n.state, nodeStateUnreachable))
	}
	fn()
}

func (n *node[CHAIN_ID, HEAD, RPC]) declareState(state nodeState) {
	if n.getCachedState() == nodeStateClosed {
		return
	}
	switch state {
	case nodeStateInvalidChainID:
		n.declareInvalidChainID()
	case nodeStateUnreachable:
		n.declareUnreachable()
	case nodeStateSyncing:
		n.declareSyncing()
	case nodeStateAlive:
		n.declareAlive()
	default:
		panic(fmt.Sprintf("%#v state declaration is not implemented", state))
	}
}

func (n *node[CHAIN_ID, HEAD, RPC]) declareInvalidChainID() {
	n.transitionToInvalidChainID(func() {
		n.lfcLog.Errorw("RPC Node has the wrong chain ID", "nodeState", n.state)
		n.wg.Add(1)
		go n.invalidChainIDLoop()
	})
}

func (n *node[CHAIN_ID, HEAD, RPC]) transitionToInvalidChainID(fn func()) {
	ctx, cancel := n.stopCh.NewCtx()
	defer cancel()
	n.metrics.IncrementNodeTransitionsToInvalidChainID(ctx, n.name)
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.state == nodeStateClosed {
		return
	}
	switch n.state {
	case nodeStateDialed, nodeStateOutOfSync, nodeStateSyncing:
		n.rpc.Close()
		n.state = nodeStateInvalidChainID
	default:
		panic(transitionFail(n.state, nodeStateInvalidChainID))
	}
	fn()
}

func (n *node[CHAIN_ID, HEAD, RPC]) declareSyncing() {
	n.transitionToSyncing(func() {
		n.lfcLog.Errorw("RPC Node is syncing", "nodeState", n.state)
		n.wg.Add(1)
		go n.syncingLoop()
	})
}

func (n *node[CHAIN_ID, HEAD, RPC]) transitionToSyncing(fn func()) {
	ctx, cancel := n.stopCh.NewCtx()
	defer cancel()
	n.metrics.IncrementNodeTransitionsToSyncing(ctx, n.name)
	n.stateMu.Lock()
	defer n.stateMu.Unlock()
	if n.state == nodeStateClosed {
		return
	}
	switch n.state {
	case nodeStateDialed, nodeStateOutOfSync, nodeStateInvalidChainID:
		n.rpc.Close()
		n.state = nodeStateSyncing
	default:
		panic(transitionFail(n.state, nodeStateSyncing))
	}

	if !n.nodePoolCfg.NodeIsSyncingEnabled() {
		panic("unexpected transition to nodeStateSyncing, while it's disabled")
	}
	fn()
}

func transitionFail(from nodeState, to nodeState) string {
	return fmt.Sprintf("cannot transition from %#v to %#v", from, to)
}

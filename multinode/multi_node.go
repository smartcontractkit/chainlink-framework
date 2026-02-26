package multinode

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services"
)

var ErrNodeError = fmt.Errorf("no live nodes available")

type multiNodeMetrics interface {
	RecordNodeStates(ctx context.Context, state string, count int64)
}

// MultiNode is a generalized multi node client interface that includes methods to interact with different chains.
// It also handles multiple node RPC connections simultaneously.
type MultiNode[
	CHAIN_ID ID,
	RPC any,
] struct {
	services.Service
	eng *services.Engine

	primaryNodes          []Node[CHAIN_ID, RPC]
	sendOnlyNodes         []SendOnlyNode[CHAIN_ID, RPC]
	chainID               CHAIN_ID
	lggr                  logger.SugaredLogger
	metrics               multiNodeMetrics
	selectionMode         string
	nodeSelector          NodeSelector[CHAIN_ID, RPC]
	leaseDuration         time.Duration
	leaseTicker           *time.Ticker
	chainFamily           string
	reportInterval        time.Duration
	deathDeclarationDelay time.Duration

	activeMu   sync.RWMutex
	activeNode Node[CHAIN_ID, RPC]
}

func NewMultiNode[
	CHAIN_ID ID,
	RPC any,
](
	lggr logger.Logger,
	metrics multiNodeMetrics,
	selectionMode string, // type of the "best" RPC selector (e.g HighestHead, RoundRobin, etc.)
	leaseDuration time.Duration, // defines interval on which new "best" RPC should be selected
	primaryNodes []Node[CHAIN_ID, RPC],
	sendOnlyNodes []SendOnlyNode[CHAIN_ID, RPC],
	chainID CHAIN_ID, // configured chain ID (used to verify that passed primaryNodes belong to the same chain)
	chainFamily string, // name of the chain family - used in the metrics
	deathDeclarationDelay time.Duration,
) *MultiNode[CHAIN_ID, RPC] {
	nodeSelector := newNodeSelector(selectionMode, primaryNodes)
	// Prometheus' default interval is 15s, set this to under 7.5s to avoid
	// aliasing (see: https://en.wikipedia.org/wiki/Nyquist_frequency)
	const reportInterval = 6500 * time.Millisecond
	c := &MultiNode[CHAIN_ID, RPC]{
		metrics:               metrics,
		primaryNodes:          primaryNodes,
		sendOnlyNodes:         sendOnlyNodes,
		chainID:               chainID,
		selectionMode:         selectionMode,
		nodeSelector:          nodeSelector,
		leaseDuration:         leaseDuration,
		chainFamily:           chainFamily,
		reportInterval:        reportInterval,
		deathDeclarationDelay: deathDeclarationDelay,
	}
	c.Service, c.eng = services.Config{
		Name:  "MultiNode",
		Start: c.start,
		Close: c.close,
	}.NewServiceEngine(logger.With(lggr, "chainID", chainID.String()))
	c.lggr = c.eng.SugaredLogger

	c.lggr.Debugf("The MultiNode is configured to use NodeSelectionMode: %s", selectionMode)

	return c
}

func (c *MultiNode[CHAIN_ID, RPC]) ChainID() CHAIN_ID {
	return c.chainID
}

func (c *MultiNode[CHAIN_ID, RPC]) DoAll(ctx context.Context, do func(ctx context.Context, rpc RPC, isSendOnly bool)) error {
	return c.eng.IfNotStopped(func() error {
		callsCompleted := 0
		for _, n := range c.primaryNodes {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if n.State() != nodeStateAlive {
					continue
				}
				do(ctx, n.RPC(), false)
				callsCompleted++
			}
		}

		for _, n := range c.sendOnlyNodes {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				if n.State() != nodeStateAlive {
					continue
				}
				do(ctx, n.RPC(), true)
			}
		}
		if callsCompleted == 0 {
			return ErrNodeError
		}
		return nil
	})
}

func (c *MultiNode[CHAIN_ID, RPC]) NodeStates() map[string]string {
	states := map[string]string{}
	for _, n := range c.primaryNodes {
		states[n.Name()] = n.State().String()
	}
	for _, n := range c.sendOnlyNodes {
		states[n.Name()] = n.State().String()
	}
	return states
}

// Start starts every node in the pool
//
// Nodes handle their own redialing and runloops, so this function does not
// return any error if the nodes aren't available
func (c *MultiNode[CHAIN_ID, RPC]) start(ctx context.Context) error {
	if len(c.primaryNodes) == 0 {
		return fmt.Errorf("no available nodes for chain %s", c.chainID.String())
	}
	var ms services.MultiStart
	for _, n := range c.primaryNodes {
		if n.ConfiguredChainID().String() != c.chainID.String() {
			return ms.CloseBecause(fmt.Errorf("node %s has configured chain ID %s which does not match multinode configured chain ID of %s", n.String(), n.ConfiguredChainID().String(), c.chainID.String()))
		}
		n.SetPoolChainInfoProvider(c)
		// node will handle its own redialing and automatic recovery
		if err := ms.Start(ctx, n); err != nil {
			return err
		}
	}
	for _, s := range c.sendOnlyNodes {
		if s.ConfiguredChainID().String() != c.chainID.String() {
			return ms.CloseBecause(fmt.Errorf("sendonly node %s has configured chain ID %s which does not match multinode configured chain ID of %s", s.String(), s.ConfiguredChainID().String(), c.chainID.String()))
		}
		if err := ms.Start(ctx, s); err != nil {
			return err
		}
	}
	c.eng.Go(c.runLoop)

	if c.leaseDuration.Seconds() > 0 && c.selectionMode != NodeSelectionModeRoundRobin && c.selectionMode != NodeSelectionModeRandomRPC {
		c.lggr.Infof("The MultiNode will switch to best node every %s", c.leaseDuration.String())
		c.eng.Go(c.checkLeaseLoop)
	} else {
		c.lggr.Info("Best node switching is disabled")
	}

	return nil
}

// Close tears down the MultiNode and closes all nodes
func (c *MultiNode[CHAIN_ID, RPC]) close() error {
	return services.CloseAll(services.MultiCloser(c.primaryNodes), services.MultiCloser(c.sendOnlyNodes))
}

// SelectRPC returns an RPC of an active node. If there are no active nodes it returns an error, but tolerates undialed
// nodes by waiting for initial dial.
// Call this method from your chain-specific client implementation to access any chain-specific rpc calls.
func (c *MultiNode[CHAIN_ID, RPC]) SelectRPC(ctx context.Context) (rpc RPC, err error) {
	n, err := c.selectNode(ctx)
	if err != nil {
		return rpc, err
	}
	return n.RPC(), nil
}

// selectNode returns the active Node, if it is still nodeStateAlive, otherwise it selects a new one from the NodeSelector.
// If no alive node is available, it falls back to an out-of-sync node. If the current active node is out-of-sync,
// it will try to upgrade to an alive node when one becomes available.
func (c *MultiNode[CHAIN_ID, RPC]) selectNode(ctx context.Context) (node Node[CHAIN_ID, RPC], err error) {
	if c.selectionMode == NodeSelectionModeRandomRPC {
		return c.awaitNodeSelection(ctx)
	}

	c.activeMu.RLock()
	node = c.activeNode
	c.activeMu.RUnlock()
	if node != nil && node.State() == nodeStateAlive {
		return // still alive
	}

	// select a new one
	c.activeMu.Lock()
	defer c.activeMu.Unlock()
	node = c.activeNode
	if node != nil && node.State() == nodeStateAlive {
		return // another goroutine beat us here
	}

	// If the current active node is out-of-sync, try to find an alive one first
	if node != nil && isUsableState(node.State()) {
		if aliveNode := c.nodeSelector.Select(); aliveNode != nil {
			c.activeNode.UnsubscribeAllExceptAliveLoop()
			c.activeNode = aliveNode
			c.lggr.Debugw("Upgraded from out-of-sync to alive node", "prevNode", node.String(), "newNode", aliveNode.String())
			return c.activeNode, nil
		}
		return // keep using the out-of-sync node
	}

	var prevNodeName string
	if c.activeNode != nil {
		prevNodeName = c.activeNode.String()
		c.activeNode.UnsubscribeAllExceptAliveLoop()
	}

	c.activeNode, err = c.awaitNodeSelection(ctx)
	if err != nil {
		return nil, err
	}

	c.lggr.Debugw("Switched to a new active node due to prev node health issues", "prevNode", prevNodeName, "newNode", c.activeNode.String())
	return c.activeNode, err
}

// awaitNodeSelection blocks until nodeSelector returns a live node or all nodes
// finish initializing. If no alive nodes are available, falls back to an out-of-sync node.
// Returns ErrNodeError when no usable nodes are available.
func (c *MultiNode[CHAIN_ID, RPC]) awaitNodeSelection(ctx context.Context) (Node[CHAIN_ID, RPC], error) {
	for {
		node := c.nodeSelector.Select()
		if node != nil {
			return node, nil
		}
		if slices.ContainsFunc(c.primaryNodes, func(n Node[CHAIN_ID, RPC]) bool {
			return n.State().isInitializing()
		}) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		if fallback := c.selectOutOfSyncNode(); fallback != nil {
			c.lggr.Warnw("No alive RPC nodes available, falling back to out-of-sync node", "node", fallback.String())
			return fallback, nil
		}
		c.lggr.Criticalw("No live RPC nodes available", "NodeSelectionMode", c.nodeSelector.Name())
		c.eng.EmitHealthErr(fmt.Errorf("no live nodes available for chain %s", c.chainID.String()))
		return nil, ErrNodeError
	}
}

// selectOutOfSyncNode picks the best out-of-sync node by highest block number.
// Returns nil if no out-of-sync nodes are available.
func (c *MultiNode[CHAIN_ID, RPC]) selectOutOfSyncNode() Node[CHAIN_ID, RPC] {
	var bestNode Node[CHAIN_ID, RPC]
	var bestBlock int64 = math.MinInt64
	for _, n := range c.primaryNodes {
		if isUsableState(n.State()) {
			_, ci := n.StateAndLatest()
			if ci.BlockNumber > bestBlock {
				bestBlock = ci.BlockNumber
				bestNode = n
			}
		}
	}
	return bestNode
}

// isUsableState returns true for out-of-sync states that can still serve requests as a fallback.
func isUsableState(s nodeState) bool {
	return s == nodeStateOutOfSync || s == nodeStateFinalizedBlockOutOfSync
}

// LatestChainInfo returns the number of alive nodes in the pool (excluding the node identified by callerName)
// and the highest ChainInfo most recently received by those nodes.
// E.g. If Node A's the most recent block is 10 and highest 15 and for Node B it's - 12 and 14. This method will return 12.
func (c *MultiNode[CHAIN_ID, RPC]) LatestChainInfo(callerName string) (int, ChainInfo) {
	var nLiveNodes int
	ch := ChainInfo{
		TotalDifficulty: big.NewInt(0),
	}
	for _, n := range c.primaryNodes {
		if n.Name() == callerName {
			continue
		}
		if s, nodeChainInfo := n.StateAndLatest(); s == nodeStateAlive {
			nLiveNodes++
			ch.BlockNumber = max(ch.BlockNumber, nodeChainInfo.BlockNumber)
			ch.FinalizedBlockNumber = max(ch.FinalizedBlockNumber, nodeChainInfo.FinalizedBlockNumber)
			ch.TotalDifficulty = MaxTotalDifficulty(ch.TotalDifficulty, nodeChainInfo.TotalDifficulty)
		}
	}
	return nLiveNodes, ch
}

// HighestUserObservations - returns highest ChainInfo ever observed by any user of the MultiNode
func (c *MultiNode[CHAIN_ID, RPC]) HighestUserObservations() ChainInfo {
	ch := ChainInfo{
		TotalDifficulty: big.NewInt(0),
	}
	for _, n := range c.primaryNodes {
		nodeChainInfo := n.HighestUserObservations()
		ch.BlockNumber = max(ch.BlockNumber, nodeChainInfo.BlockNumber)
		ch.FinalizedBlockNumber = max(ch.FinalizedBlockNumber, nodeChainInfo.FinalizedBlockNumber)
		ch.TotalDifficulty = MaxTotalDifficulty(ch.TotalDifficulty, nodeChainInfo.TotalDifficulty)
	}
	return ch
}

func (c *MultiNode[CHAIN_ID, RPC]) checkLease() {
	bestNode := c.nodeSelector.Select()
	for _, n := range c.primaryNodes {
		// Terminate client subscriptions. Services are responsible for reconnecting, which will be routed to the new
		// best node. Only terminate connections with more than 1 subscription to account for the aliveLoop subscription
		if n.State() == nodeStateAlive && n != bestNode {
			c.lggr.Infof("Switching to best node from %q to %q", n.String(), bestNode.String())
			n.UnsubscribeAllExceptAliveLoop()
		}
	}

	c.activeMu.Lock()
	defer c.activeMu.Unlock()

	if bestNode == nil {
		// No alive node available; if the current active is still usable (out-of-sync), keep it
		if c.activeNode != nil && isUsableState(c.activeNode.State()) {
			return
		}
		// Try out-of-sync fallback
		if fallback := c.selectOutOfSyncNode(); fallback != nil && fallback != c.activeNode {
			if c.activeNode != nil {
				c.activeNode.UnsubscribeAllExceptAliveLoop()
			}
			c.activeNode = fallback
		}
		return
	}

	if bestNode != c.activeNode {
		if c.activeNode != nil {
			c.activeNode.UnsubscribeAllExceptAliveLoop()
		}
		c.activeNode = bestNode
	}
}

func (c *MultiNode[CHAIN_ID, RPC]) checkLeaseLoop(ctx context.Context) {
	c.leaseTicker = time.NewTicker(c.leaseDuration)
	defer c.leaseTicker.Stop()

	for {
		select {
		case <-c.leaseTicker.C:
			c.checkLease()
		case <-ctx.Done():
			return
		}
	}
}

func (c *MultiNode[CHAIN_ID, RPC]) runLoop(ctx context.Context) {
	nodeStates := make([]nodeWithState, len(c.primaryNodes))
	for i, n := range c.primaryNodes {
		nodeStates[i] = nodeWithState{
			Node:      n.String(),
			State:     n.State().String(),
			DeadSince: nil,
		}
	}

	c.report(nodeStates)

	monitor := services.NewTicker(c.reportInterval)
	defer monitor.Stop()

	for {
		select {
		case <-monitor.C:
			c.report(nodeStates)
		case <-ctx.Done():
			return
		}
	}
}

type nodeWithState struct {
	Node      string
	State     string
	DeadSince *time.Time
}

func (c *MultiNode[CHAIN_ID, RPC]) report(nodesStateInfo []nodeWithState) {
	start := time.Now()
	var dead int
	counts := make(map[nodeState]int)
	for i, n := range c.primaryNodes {
		state := n.State()
		counts[state]++
		nodesStateInfo[i].State = state.String()
		if state == nodeStateAlive {
			nodesStateInfo[i].DeadSince = nil
			continue
		}

		if nodesStateInfo[i].DeadSince == nil {
			nodesStateInfo[i].DeadSince = &start
		}

		if start.Sub(*nodesStateInfo[i].DeadSince) >= c.deathDeclarationDelay {
			dead++
		}
	}

	ctx, cancel := c.eng.NewCtx()
	defer cancel()
	for _, state := range allNodeStates {
		count := int64(counts[state])
		c.metrics.RecordNodeStates(ctx, state.String(), count)
	}

	total := len(c.primaryNodes)
	live := total - dead
	c.lggr.Tracew(fmt.Sprintf("MultiNode state: %d/%d nodes are alive", live, total), "nodeStates", nodesStateInfo)
	if total == dead {
		rerr := fmt.Errorf("no primary nodes available: 0/%d nodes are alive", total)
		c.lggr.Criticalw(rerr.Error(), "nodeStates", nodesStateInfo)
		c.eng.EmitHealthErr(rerr)
	} else if dead > 0 {
		c.lggr.Errorw(fmt.Sprintf("At least one primary node is dead: %d/%d nodes are alive", live, total), "nodeStates", nodesStateInfo)
	}
}

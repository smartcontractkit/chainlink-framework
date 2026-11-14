package multinode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	clientMocks "github.com/smartcontractkit/chainlink-framework/multinode/mocks"
)

type rpcView struct {
	mu      sync.Mutex
	latest  ChainInfo
	highest ChainInfo
}

func (v *rpcView) set(latestFinalized, highestUserFinalized int64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.latest = ChainInfo{BlockNumber: latestFinalized + 10, FinalizedBlockNumber: latestFinalized}
	v.highest = ChainInfo{FinalizedBlockNumber: highestUserFinalized}
}

func (v *rpcView) get() (ChainInfo, ChainInfo) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.latest, v.highest
}

// testHealthyPeersPool adds the optional healthyPeersProvider to a mock pool.
type testHealthyPeersPool struct {
	*mockPoolChainInfoProvider
	healthyPeers int
	peersBest    ChainInfo
	missing      []string
	calls        atomic.Int32
	lastCaller   atomic.Value
}

func newTestHealthyPeersPool(t *testing.T, healthyPeers int, peersBest ChainInfo) *testHealthyPeersPool {
	return &testHealthyPeersPool{mockPoolChainInfoProvider: newMockPoolChainInfoProvider(t), healthyPeers: healthyPeers, peersBest: peersBest}
}

func (p *testHealthyPeersPool) HealthyPeersChainInfo(callerName string) (int, ChainInfo) {
	p.calls.Add(1)
	p.lastCaller.Store(callerName)
	return p.healthyPeers, p.peersBest
}

func (p *testHealthyPeersPool) nodesWithoutHealthSnapshot() []string { return p.missing }

type poolNode struct {
	testNode
	view *rpcView
}

// wrappedNode decorates a Node by embedding it: it gets every Node method but not loadHealthSnapshot.
type wrappedNode struct {
	Node[ID, RPCClient[ID, Head]]
}

// newFinalizedPool wires real nodes (mock RPCs) to a MultiNode as their pool. Nothing is started.
func newFinalizedPool(t *testing.T, lggr logger.Logger, fallback bool, names ...string) (*MultiNode[ID, RPCClient[ID, Head]], map[string]poolNode) {
	return newFinalizedPoolWithWrapped(t, lggr, fallback, nil, names...)
}

// newFinalizedPoolWithWrapped is newFinalizedPool, but hands the MultiNode a wrappedNode for each name in wrapped.
// The returned map always holds the underlying real nodes, so tests can drive their state and views.
func newFinalizedPoolWithWrapped(t *testing.T, lggr logger.Logger, fallback bool, wrapped map[string]bool, names ...string) (*MultiNode[ID, RPCClient[ID, Head]], map[string]poolNode) {
	chainID := RandomID()
	nodes := make([]Node[ID, RPCClient[ID, Head]], 0, len(names))
	byName := make(map[string]poolNode, len(names))
	for i, name := range names {
		view := &rpcView{}
		rpc := newMockRPCClient[ID, Head](t)
		rpc.EXPECT().GetInterceptedChainInfo().RunAndReturn(view.get).Maybe()
		rpc.EXPECT().Close().Maybe()
		n := newTestNode(t, testNodeOpts{
			config:      testNodeConfig{enforceRepeatableRead: true, finalizedBlockOutOfSyncFallback: fallback},
			chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
			lggr:        lggr,
			name:        name,
			id:          i + 1,
			chainID:     chainID,
			nodeOrder:   1,
			rpc:         rpc,
		})
		if wrapped[name] {
			nodes = append(nodes, wrappedNode{Node: n})
		} else {
			nodes = append(nodes, n)
		}
		byName[name] = poolNode{testNode: n, view: view}
	}
	mn := NewMultiNode[ID, RPCClient[ID, Head]](lggr, makeMockMultiNodeMetrics(t), NodeSelectionModeHighestHead, 0,
		nodes, nil, chainID, "test", 0)
	for _, n := range nodes {
		n.SetPoolChainInfoProvider(mn)
	}
	return mn, byName
}

// Incident timeline: pods pin to the fastest RPC, which fails while the others trail by one finalization batch.
func TestFinalizedBlockOutOfSync_TnetOutage(t *testing.T) {
	t.Parallel()
	mn, p := newFinalizedPool(t, logger.Test(t), true, "alchemy", "simplystaking", "linkpool_1", "linkpool_2")
	alchemy, simplystaking, linkpool1, linkpool2 := p["alchemy"], p["simplystaking"], p["linkpool_1"], p["linkpool_2"]

	// alchemy served 1200; the others have not polled the new batch yet.
	alchemy.view.set(1200, 1200)
	simplystaking.view.set(1000, 0)
	linkpool1.view.set(1000, 0)
	linkpool2.view.set(1100, 0)
	for _, n := range p {
		n.setState(nodeStateAlive)
	}

	// t1: strict, as before.
	assert.Equal(t, nodeStateAlive, alchemy.State())
	assert.Equal(t, nodeStateFinalizedBlockOutOfSync, simplystaking.State())
	assert.Equal(t, nodeStateFinalizedBlockOutOfSync, linkpool1.State())
	assert.Equal(t, nodeStateFinalizedBlockOutOfSync, linkpool2.State())
	assert.Equal(t, alchemy.Name(), mn.nodeSelector.Select().Name())

	// t2: alchemy fails. Previously ErrNodeError; now the best laggard serves a delayed view.
	alchemy.setState(nodeStateUnreachable)
	assert.Equal(t, nodeStateAlive, linkpool2.State())
	assert.Equal(t, nodeStateFinalizedBlockOutOfSync, linkpool2.StateForMetrics(), "metrics must still expose the lag")
	assert.Equal(t, nodeStateFinalizedBlockOutOfSync, simplystaking.State(), "worse laggards stay excluded")
	assert.Equal(t, nodeStateFinalizedBlockOutOfSync, linkpool1.State())
	selected, err := mn.selectNode(t.Context())
	require.NoError(t, err)
	assert.Equal(t, linkpool2.Name(), selected.Name())

	// t3: the others catch up; strict again.
	simplystaking.view.set(1200, 0)
	linkpool1.view.set(1200, 0)
	linkpool2.view.set(1200, 0)
	for _, n := range []poolNode{simplystaking, linkpool1, linkpool2} {
		assert.Equal(t, nodeStateAlive, n.State())
		assert.Equal(t, nodeStateAlive, n.StateForMetrics())
	}
}

// Without the opt-in fallback, behaviour is unchanged: the same outage leaves no routable RPC.
func TestFinalizedBlockOutOfSync_FallbackDisabled(t *testing.T) {
	t.Parallel()

	t.Run("outage leaves no routable RPC", func(t *testing.T) {
		t.Parallel()
		mn, p := newFinalizedPool(t, logger.Test(t), false, "alchemy", "linkpool_1", "linkpool_2")
		p["alchemy"].view.set(1200, 1200)
		p["linkpool_1"].view.set(1000, 0)
		p["linkpool_2"].view.set(1100, 0)
		for _, n := range p {
			n.setState(nodeStateAlive)
		}
		p["alchemy"].setState(nodeStateUnreachable)

		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, p["linkpool_1"].State())
		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, p["linkpool_2"].State())
		_, err := mn.selectNode(t.Context())
		require.ErrorIs(t, err, ErrNodeError)
	})

	t.Run("lagging node does not scan peers", func(t *testing.T) {
		t.Parallel()
		rpc := newMockRPCClient[ID, Head](t)
		rpc.EXPECT().GetInterceptedChainInfo().Return(ChainInfo{FinalizedBlockNumber: 1000}, ChainInfo{}).Maybe()
		node := newTestNode(t, testNodeOpts{
			config:      testNodeConfig{enforceRepeatableRead: true},
			chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
			rpc:         rpc,
		})
		pool := newMockPoolChainInfoProvider(t) // HealthyPeersChainInfo is not expected
		pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Maybe()
		node.SetPoolChainInfoProvider(pool)
		node.setState(nodeStateAlive)
		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, node.State())
	})

	t.Run("config without the optional interface keeps it disabled", func(t *testing.T) {
		t.Parallel()
		enabled := newTestNode(t, testNodeOpts{config: testNodeConfig{finalizedBlockOutOfSyncFallback: true}})
		require.True(t, enabled.finalizedBlockOutOfSyncFallback)

		withoutInterface := struct{ NodeConfig }{enabled.nodePoolCfg}
		n := NewNode[ID, Head, RPCClient[ID, Head]](withoutInterface, enabled.chainCfg, logger.Test(t), enabled.metrics,
			nil, nil, "n", 1, RandomID(), 1, enabled.rpc, "family", false)
		assert.False(t, n.(*node[ID, Head, RPCClient[ID, Head]]).finalizedBlockOutOfSyncFallback)
	})
}

// With the fallback enabled, a pool that cannot report the health of every peer keeps strict behaviour.
func TestFinalizedBlockOutOfSync_FallbackFailsClosed(t *testing.T) {
	t.Parallel()

	newLaggingNode := func(t *testing.T, lggr logger.Logger) testNode {
		rpc := newMockRPCClient[ID, Head](t)
		rpc.EXPECT().GetInterceptedChainInfo().Return(ChainInfo{FinalizedBlockNumber: 1000}, ChainInfo{}).Maybe()
		return newTestNode(t, testNodeOpts{
			config:      testNodeConfig{enforceRepeatableRead: true, finalizedBlockOutOfSyncFallback: true},
			chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
			rpc:         rpc,
			lggr:        lggr,
		})
	}

	t.Run("pool without healthyPeersProvider", func(t *testing.T) {
		t.Parallel()
		lggr, observed := logger.TestObserved(t, zap.WarnLevel)
		node := newLaggingNode(t, lggr)
		pool := newMockPoolChainInfoProvider(t)
		pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Maybe()
		node.SetPoolChainInfoProvider(pool)
		node.setState(nodeStateAlive)

		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, node.State())
		assert.Equal(t, 1, observed.FilterMessageSnippet("cannot report healthy peers").Len())
	})

	t.Run("pool with nodes that do not report health", func(t *testing.T) {
		t.Parallel()
		lggr, observed := logger.TestObserved(t, zap.WarnLevel)
		node := newLaggingNode(t, lggr)
		pool := newTestHealthyPeersPool(t, 0, ChainInfo{})
		pool.missing = []string{"wrapped"}
		pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Maybe()
		node.SetPoolChainInfoProvider(pool)
		node.setState(nodeStateAlive)

		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, node.State())
		assert.Zero(t, pool.calls.Load(), "peers must not be consulted")
		assert.Zero(t, observed.Len(), "the pool logs this once, not each node")
	})

	t.Run("outage with a wrapped peer stays strict and warns once per pool", func(t *testing.T) {
		t.Parallel()
		lggr, observed := logger.TestObserved(t, zap.WarnLevel)
		mn, p := newFinalizedPoolWithWrapped(t, lggr, true, map[string]bool{"simplystaking": true},
			"alchemy", "simplystaking", "linkpool_1", "linkpool_2")
		p["alchemy"].view.set(1200, 1200)
		p["simplystaking"].view.set(1000, 0)
		p["linkpool_1"].view.set(1000, 0)
		p["linkpool_2"].view.set(1100, 0)
		for _, n := range p {
			n.setState(nodeStateAlive)
		}
		p["alchemy"].setState(nodeStateUnreachable)

		// With every peer reporting health, linkpool_2 would serve a delayed view (TestFinalizedBlockOutOfSync_TnetOutage).
		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, p["linkpool_2"].State())
		_, err := mn.selectNode(t.Context())
		require.ErrorIs(t, err, ErrNodeError)

		warns := observed.FilterMessageSnippet("do not report health")
		require.Equal(t, 1, warns.Len())
		assert.Equal(t, []any{"simplystaking"}, warns.All()[0].ContextMap()["nodes"])
	})

	t.Run("no warning when no node requested the fallback", func(t *testing.T) {
		t.Parallel()
		lggr, observed := logger.TestObserved(t, zap.WarnLevel)
		newFinalizedPoolWithWrapped(t, lggr, false, map[string]bool{"b": true}, "a", "b")
		assert.Zero(t, observed.FilterMessageSnippet("do not report health").Len())
	})

	t.Run("MultiNode lists nodes without a health snapshot", func(t *testing.T) {
		t.Parallel()
		realNode := newLaggingNode(t, logger.Test(t))
		wrapped := newMockNode[ID, RPCClient[ID, Head]](t)
		wrapped.On("Name").Return("wrapped").Maybe()
		mn := NewMultiNode[ID, RPCClient[ID, Head]](logger.Test(t), makeMockMultiNodeMetrics(t), NodeSelectionModeHighestHead, 0,
			[]Node[ID, RPCClient[ID, Head]]{realNode, wrapped}, nil, RandomID(), "test", 0)
		assert.Equal(t, []string{"wrapped"}, mn.nodesWithoutHealthSnapshot())

		_, fullySupported := newFinalizedPool(t, logger.Test(t), true, "a", "b")
		assert.Empty(t, fullySupported["a"].healthyPeers.nodesWithoutHealthSnapshot())
	})
}

func TestFinalizedBlockOutOfSync_PoolScenarios(t *testing.T) {
	t.Parallel()

	t.Run("a node kept alive only by forced revival does not block a lagging healthy node", func(t *testing.T) {
		t.Parallel()
		mn, p := newFinalizedPool(t, logger.Test(t), true, "alchemy", "linkpool_1", "linkpool_2")
		p["alchemy"].view.set(1200, 1200)
		p["linkpool_1"].view.set(1000, 0)
		p["linkpool_2"].view.set(1100, 0)
		for _, n := range p {
			n.setState(nodeStateAlive)
		}
		p["alchemy"].modifyHealthCheckFailure(healthCheckPoll, true)

		assert.Equal(t, nodeStateAlive, p["linkpool_2"].State())
		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, p["linkpool_1"].State())
		// Lets the degraded node's last-node-standing check eject it.
		live, _ := mn.LatestChainInfo("alchemy")
		assert.Equal(t, 1, live)
	})

	t.Run("a leading node whose finalized block regresses stays routable", func(t *testing.T) {
		t.Parallel()
		_, p := newFinalizedPool(t, logger.Test(t), true, "alchemy", "linkpool_1")
		p["alchemy"].view.set(1190, 1200)
		p["linkpool_1"].view.set(1000, 0)
		for _, n := range p {
			n.setState(nodeStateAlive)
		}
		assert.Equal(t, nodeStateAlive, p["alchemy"].State())
		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, p["linkpool_1"].State())
	})

	t.Run("a freshly reconnected peer with no finalized block does not lower the reference", func(t *testing.T) {
		t.Parallel()
		_, p := newFinalizedPool(t, logger.Test(t), true, "alchemy", "linkpool_1", "linkpool_2")
		p["alchemy"].view.set(1200, 1200)
		p["linkpool_1"].view.set(1000, 0)
		p["linkpool_2"].view.set(0, 0)
		for _, n := range p {
			n.setState(nodeStateAlive)
		}
		p["alchemy"].setState(nodeStateUnreachable)
		assert.Equal(t, nodeStateAlive, p["linkpool_1"].State())
		assert.Equal(t, nodeStateFinalizedBlockOutOfSync, p["linkpool_2"].State())
	})

	t.Run("last node standing stays routable", func(t *testing.T) {
		t.Parallel()
		mn, p := newFinalizedPool(t, logger.Test(t), true, "alchemy", "linkpool_1")
		p["alchemy"].view.set(1200, 1200)
		p["linkpool_1"].view.set(1000, 0)
		p["alchemy"].setState(nodeStateOutOfSync)
		p["linkpool_1"].setState(nodeStateAlive)
		assert.Equal(t, nodeStateAlive, p["linkpool_1"].State())
		selected, err := mn.selectNode(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "linkpool_1", selected.Name())
	})
}

func TestMultiNode_HealthyPeersChainInfo(t *testing.T) {
	t.Parallel()
	mn, p := newFinalizedPool(t, logger.Test(t), true, "self", "healthy_a", "healthy_b", "unreachable", "poll_failing", "sync_issue")
	for name, finalized := range map[string]int64{"self": 5000, "healthy_a": 900, "healthy_b": 1000,
		"unreachable": 3000, "poll_failing": 3000, "sync_issue": 3000} {
		p[name].view.set(finalized, 0)
		p[name].setState(nodeStateAlive)
	}
	p["unreachable"].setState(nodeStateUnreachable)
	p["poll_failing"].modifyHealthCheckFailure(healthCheckPoll, true)
	p["sync_issue"].modifySyncIssue(syncStatusNoNewHead, true)

	count, best := mn.HealthyPeersChainInfo("self")
	assert.Equal(t, 2, count, "excludes the caller and every node with a health issue")
	assert.Equal(t, int64(1000), best.FinalizedBlockNumber)
	assert.Equal(t, int64(1010), best.BlockNumber)

	t.Run("nodes not implementing healthSnapshotReporter are never healthy peers", func(t *testing.T) {
		mockNode := newMockNode[ID, RPCClient[ID, Head]](t) // no expectations: any call would fail the test
		mn := NewMultiNode[ID, RPCClient[ID, Head]](logger.Test(t), makeMockMultiNodeMetrics(t), NodeSelectionModeHighestHead, 0,
			[]Node[ID, RPCClient[ID, Head]]{mockNode}, nil, RandomID(), "test", 0)
		count, _ := mn.HealthyPeersChainInfo("self")
		assert.Equal(t, 0, count)
	})
}

func TestNode_FinalizedBlockOutOfSync_Reference(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name          string
		finalityTag   bool
		offset        uint32
		highMark      ChainInfo
		latest        ChainInfo
		healthyPeers  int
		peersBest     ChainInfo
		expectedState nodeState
	}{
		{
			name: "healthy peer at the high-water mark: strict", finalityTag: true,
			highMark: ChainInfo{FinalizedBlockNumber: 1200}, latest: ChainInfo{FinalizedBlockNumber: 1000},
			healthyPeers: 1, peersBest: ChainInfo{FinalizedBlockNumber: 1200}, expectedState: nodeStateFinalizedBlockOutOfSync,
		},
		{
			name: "best laggard: routable", finalityTag: true,
			highMark: ChainInfo{FinalizedBlockNumber: 1200}, latest: ChainInfo{FinalizedBlockNumber: 1100},
			healthyPeers: 3, peersBest: ChainInfo{FinalizedBlockNumber: 1000}, expectedState: nodeStateAlive,
		},
		{
			name: "tied laggards: all routable", finalityTag: true,
			highMark: ChainInfo{FinalizedBlockNumber: 1200}, latest: ChainInfo{FinalizedBlockNumber: 1000},
			healthyPeers: 2, peersBest: ChainInfo{FinalizedBlockNumber: 1000}, expectedState: nodeStateAlive,
		},
		{
			name: "healthy peer further ahead but below the mark: excluded", finalityTag: true,
			highMark: ChainInfo{FinalizedBlockNumber: 1200}, latest: ChainInfo{FinalizedBlockNumber: 1000},
			healthyPeers: 1, peersBest: ChainInfo{FinalizedBlockNumber: 1100}, expectedState: nodeStateFinalizedBlockOutOfSync,
		},
		{
			name: "no healthy peers: last node standing", finalityTag: true,
			highMark: ChainInfo{FinalizedBlockNumber: 1200}, latest: ChainInfo{FinalizedBlockNumber: 1000},
			healthyPeers: 0, expectedState: nodeStateAlive,
		},
		{
			name: "offset is applied to the reference", finalityTag: true, offset: 15,
			highMark: ChainInfo{FinalizedBlockNumber: 1200}, latest: ChainInfo{FinalizedBlockNumber: 1090},
			healthyPeers: 1, peersBest: ChainInfo{FinalizedBlockNumber: 1100}, expectedState: nodeStateAlive,
		},
		{
			name:     "finality depth: compares latest blocks",
			highMark: ChainInfo{BlockNumber: 1200}, latest: ChainInfo{BlockNumber: 1100},
			healthyPeers: 1, peersBest: ChainInfo{BlockNumber: 1050}, expectedState: nodeStateAlive,
		},
		{
			name:     "finality depth: healthy peer at the mark is strict",
			highMark: ChainInfo{BlockNumber: 1200}, latest: ChainInfo{BlockNumber: 1100},
			healthyPeers: 1, peersBest: ChainInfo{BlockNumber: 1200}, expectedState: nodeStateFinalizedBlockOutOfSync,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rpc := newMockRPCClient[ID, Head](t)
			rpc.EXPECT().GetInterceptedChainInfo().Return(tc.latest, ChainInfo{}).Maybe()
			node := newTestNode(t, testNodeOpts{
				config:      testNodeConfig{enforceRepeatableRead: true, finalizedBlockOutOfSyncFallback: true},
				chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: tc.finalityTag, FinalizedBlockOffsetVal: tc.offset},
				rpc:         rpc,
			})
			pool := newTestHealthyPeersPool(t, tc.healthyPeers, tc.peersBest)
			pool.EXPECT().HighestUserObservations().Return(tc.highMark).Maybe()
			node.SetPoolChainInfoProvider(pool)
			node.setState(nodeStateAlive)

			assert.Equal(t, tc.expectedState, node.State())
			assert.Equal(t, node.name, pool.lastCaller.Load())
			// Metrics always expose the lag.
			assert.Equal(t, nodeStateFinalizedBlockOutOfSync, node.StateForMetrics())
		})
	}

	t.Run("in-sync nodes skip the peer scan", func(t *testing.T) {
		t.Parallel()
		rpc := newMockRPCClient[ID, Head](t)
		rpc.EXPECT().GetInterceptedChainInfo().Return(ChainInfo{FinalizedBlockNumber: 1200}, ChainInfo{}).Maybe()
		node := newTestNode(t, testNodeOpts{
			config:      testNodeConfig{enforceRepeatableRead: true, finalizedBlockOutOfSyncFallback: true},
			chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
			rpc:         rpc,
		})
		pool := newTestHealthyPeersPool(t, 1, ChainInfo{FinalizedBlockNumber: 1200})
		pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Maybe()
		node.SetPoolChainInfoProvider(pool)
		node.setState(nodeStateAlive)
		assert.Equal(t, nodeStateAlive, node.State())
		assert.Equal(t, nodeStateAlive, node.StateForMetrics())
		assert.Zero(t, pool.calls.Load())
	})
}

func TestNode_FinalizedBlockOutOfSync_LogsOnlyOnTransitions(t *testing.T) {
	t.Parallel()
	lggr, observed := logger.TestObserved(t, zap.InfoLevel)
	view := &rpcView{}
	view.set(1000, 0)
	rpc := newMockRPCClient[ID, Head](t)
	rpc.EXPECT().GetInterceptedChainInfo().RunAndReturn(view.get).Maybe()
	node := newTestNode(t, testNodeOpts{
		config:      testNodeConfig{enforceRepeatableRead: true, finalizedBlockOutOfSyncFallback: true},
		chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
		rpc:         rpc,
		lggr:        lggr,
	})
	pool := newTestHealthyPeersPool(t, 0, ChainInfo{})
	pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Maybe()
	node.SetPoolChainInfoProvider(pool)
	node.setState(nodeStateAlive)

	for range 5 {
		assert.Equal(t, nodeStateAlive, node.State())
	}
	assert.Equal(t, 1, observed.FilterMessageSnippet("serving a delayed finalized view").Len())

	view.set(1200, 0)
	for range 5 {
		assert.Equal(t, nodeStateAlive, node.State())
	}
	assert.Equal(t, 1, observed.FilterMessageSnippet("no longer serving a delayed finalized view").Len())
}

func TestNode_FinalizedBlockOutOfSync_DelayedViewWarnsAgainAfterLeavingAlive(t *testing.T) {
	t.Parallel()
	lggr, observed := logger.TestObserved(t, zap.WarnLevel)
	rpc := newMockRPCClient[ID, Head](t)
	rpc.EXPECT().GetInterceptedChainInfo().Return(ChainInfo{FinalizedBlockNumber: 1000}, ChainInfo{}).Maybe()
	node := newTestNode(t, testNodeOpts{
		config:      testNodeConfig{enforceRepeatableRead: true, finalizedBlockOutOfSyncFallback: true},
		chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
		rpc:         rpc,
		lggr:        lggr,
	})
	pool := newTestHealthyPeersPool(t, 0, ChainInfo{})
	pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Maybe()
	node.SetPoolChainInfoProvider(pool)

	node.setState(nodeStateAlive)
	assert.Equal(t, nodeStateAlive, node.State())
	assert.Equal(t, 1, observed.FilterMessageSnippet("serving a delayed finalized view").Len())

	node.setState(nodeStateUnreachable)
	assert.False(t, node.servingDelayedFinalized.Load())
	node.setState(nodeStateAlive)
	assert.Equal(t, nodeStateAlive, node.State(), "still lagging with no peers")
	assert.Equal(t, 2, observed.FilterMessageSnippet("serving a delayed finalized view").Len(), "new Alive episode warns again")
}

func TestNode_StateForMetrics_StrictModeChecksFinalizedOnce(t *testing.T) {
	t.Parallel()
	rpc := newMockRPCClient[ID, Head](t)
	rpc.EXPECT().GetInterceptedChainInfo().Return(ChainInfo{FinalizedBlockNumber: 1200}, ChainInfo{}).Maybe()
	node := newTestNode(t, testNodeOpts{
		config:      testNodeConfig{enforceRepeatableRead: true},
		chainConfig: clientMocks.ChainConfig{IsFinalityTagEnabled: true},
		rpc:         rpc,
	})
	pool := newMockPoolChainInfoProvider(t)
	pool.EXPECT().HighestUserObservations().Return(ChainInfo{FinalizedBlockNumber: 1200}).Once()
	node.SetPoolChainInfoProvider(pool)
	node.setState(nodeStateAlive)

	assert.Equal(t, nodeStateAlive, node.StateForMetrics())
	assert.False(t, node.servingDelayedFinalized.Load(), "strict mode never touches the delayed-view flag")
}

func TestNode_HealthSnapshotConsistency(t *testing.T) {
	t.Parallel()
	rpc := newMockRPCClient[ID, Head](t)
	rpc.EXPECT().Close().Maybe()
	rpc.EXPECT().GetInterceptedChainInfo().Return(ChainInfo{FinalizedBlockNumber: 7}, ChainInfo{}).Maybe()
	node := newTestNode(t, testNodeOpts{rpc: rpc})

	assertConsistent := func(step string) {
		node.stateMu.RLock()
		want := node.state == nodeStateAlive && node.stateHealthFailures == 0 && node.stateSyncIssues == syncStatusSynced
		node.stateMu.RUnlock()
		healthy, latest := node.loadHealthSnapshot()
		require.Equal(t, want, healthy, step)
		if healthy {
			require.Equal(t, int64(7), latest.FinalizedBlockNumber, step)
		}
	}

	assertConsistent("initial")
	for _, s := range allNodeStates {
		node.setState(s)
		assertConsistent("setState " + s.String())
	}

	node.setState(nodeStateAlive)
	node.modifyHealthCheckFailure(healthCheckPoll, true)
	assertConsistent("poll failure")
	node.modifyHealthCheckFailure(healthCheckPoll, false)
	assertConsistent("poll recovered")
	node.modifySyncIssue(syncStatusNoNewHead, true)
	assertConsistent("sync issue")
	node.modifySyncIssue(syncStatusNoNewHead, false)
	assertConsistent("sync recovered")

	node.transitionToUnreachable(func() {})
	assertConsistent("transitionToUnreachable")

	node.setState(nodeStateDialed)
	node.modifyHealthCheckFailure(healthCheckFinalizedState, true)
	node.transitionToAlive(func() {})
	assertConsistent("transitionToAlive resets failures")

	node.setState(nodeStateOutOfSync)
	node.transitionToInSync(syncStatusNotInSyncWithPool, func() {})
	assertConsistent("forced transitionToInSync retains sync issue")

	require.NoError(t, node.close())
	assertConsistent("close")
}

// A direct write to node.state would leave healthSnapshot stale.
func TestNode_StateWritesGoThroughSetStateLocked(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(t, err)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !isNodeReceiver(fn) || fn.Name.Name == "setStateLocked" {
				continue
			}
			ast.Inspect(fn.Body, func(nd ast.Node) bool {
				assign, ok := nd.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range assign.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "state" {
						t.Errorf("%s: %s assigns node.state directly; use setStateLocked", fset.Position(assign.Pos()), fn.Name.Name)
					}
				}
				return true
			})
		}
	}
}

func isNodeReceiver(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return false
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if idx, ok := expr.(*ast.IndexListExpr); ok {
		expr = idx.X
	}
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "node"
}

// Any nested stateMu acquisition from within State() deadlocks under queued writers.
func TestFinalizedBlockOutOfSync_NoDeadlock(t *testing.T) {
	t.Parallel()
	mn, p := newFinalizedPool(t, logger.Nop(), true, "alchemy", "simplystaking", "linkpool_1", "linkpool_2")
	nodes := make([]poolNode, 0, len(p))
	for i, name := range []string{"alchemy", "simplystaking", "linkpool_1", "linkpool_2"} {
		p[name].view.set(int64(1000+100*i), 1200)
		p[name].setState(nodeStateAlive)
		nodes = append(nodes, p[name])
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	loop := func(fn func(i int)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
					fn(i)
				}
			}
		}()
	}
	for _, n := range nodes {
		loop(func(int) { n.State() })
		loop(func(int) { n.StateAndLatest() })
		loop(func(int) { n.StateForMetrics() })
		loop(func(i int) {
			if i%2 == 0 {
				n.setState(nodeStateOutOfSync)
			} else {
				n.setState(nodeStateAlive)
			}
		})
		loop(func(i int) { n.modifyHealthCheckFailure(healthCheckPoll, i%2 == 0) })
		loop(func(i int) { n.modifySyncIssue(syncStatusNoNewHead, i%3 == 0) })
		loop(func(i int) { n.view.set(int64(1000+i%300), 1200) })
	}
	loop(func(int) { mn.LatestChainInfo("alchemy") })
	loop(func(int) { mn.HealthyPeersChainInfo("alchemy") })
	loop(func(int) { mn.nodeSelector.Select() })

	time.Sleep(500 * time.Millisecond)
	close(stop)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("state readers did not finish: likely deadlock between nodes' state locks")
	}
}

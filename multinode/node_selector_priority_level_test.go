package multinode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPriorityLevelNodeSelectorName(t *testing.T) {
	selector := newNodeSelector[ID, RPCClient[ID, Head]](NodeSelectionModePriorityLevel, nil)
	assert.Equal(t, NodeSelectionModePriorityLevel, selector.Name())
}

func TestPriorityLevelNodeSelector(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	type testNode struct {
		order int32
		state nodeState
	}
	type testCase struct {
		name    string
		nodes   []testNode
		allowed []int // indexes of the nodes Select may return; every one must be returned at least once
	}

	testCases := []testCase{
		{
			name: "TwoNodesSameOrder: Highest Allowed Order",
			nodes: []testNode{
				{order: 1, state: nodeStateAlive},
				{order: 1, state: nodeStateAlive},
			},
			allowed: []int{0, 1},
		},
		{
			name: "TwoNodesSameOrder: Lowest Allowed Order",
			nodes: []testNode{
				{order: 100, state: nodeStateAlive},
				{order: 100, state: nodeStateAlive},
			},
			allowed: []int{0, 1},
		},
		{
			name: "NoneAvailable",
			nodes: []testNode{
				{order: 1, state: nodeStateOutOfSync},
				{order: 1, state: nodeStateUnreachable},
				{order: 1, state: nodeStateUnreachable},
			},
		},
		{
			name: "DifferentOrder",
			nodes: []testNode{
				{order: 1, state: nodeStateAlive},
				{order: 2, state: nodeStateAlive},
				{order: 3, state: nodeStateAlive},
			},
			allowed: []int{0}, // only the highest priority tier should be selected
		},
		{
			name: "UnsortedOrder",
			nodes: []testNode{
				{order: 3, state: nodeStateAlive},
				{order: 2, state: nodeStateAlive},
				{order: 1, state: nodeStateAlive},
				{order: 2, state: nodeStateAlive},
				{order: 1, state: nodeStateAlive},
			},
			allowed: []int{2, 4}, // a better tier found later must replace the one collected so far
		},
		{
			name: "HighestTierPartiallyAlive",
			nodes: []testNode{
				{order: 1, state: nodeStateUnreachable},
				{order: 2, state: nodeStateAlive},
				{order: 2, state: nodeStateOutOfSync},
				{order: 2, state: nodeStateAlive},
				{order: 2, state: nodeStateAlive},
				{order: 3, state: nodeStateAlive},
			},
			allowed: []int{1, 3, 4}, // alive nodes of the best tier that has any alive node
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var nodes []Node[ID, nodeClient]
			for _, tn := range tc.nodes {
				node := newMockNode[ID, nodeClient](t)
				node.On("State").Return(tn.state)
				node.On("Order").Return(tn.order).Maybe()
				nodes = append(nodes, node)
			}

			selector := newNodeSelector(NodeSelectionModePriorityLevel, nodes)
			if len(tc.allowed) == 0 {
				assert.Nil(t, selector.Select())
				return
			}

			// With 1000 draws, the chance of never picking a given allowed node is negligible (< 0.67^1000).
			picked := map[Node[ID, nodeClient]]int{}
			for range 1000 {
				picked[selector.Select()]++
			}
			for _, idx := range tc.allowed {
				assert.Positive(t, picked[nodes[idx]], "node %d was never selected", idx)
				delete(picked, nodes[idx])
			}
			assert.Empty(t, picked, "selected nodes outside the allowed set")
		})
	}
}

// Pods share the same RPC set; each builds its own selector. Their first picks must spread across the tier instead of
// all landing on the same node.
func TestPriorityLevelNodeSelector_FirstPickSpreadsAcrossSelectors(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	nodes := make([]Node[ID, nodeClient], 0, 3)
	for range 3 {
		node := newMockNode[ID, nodeClient](t)
		node.On("State").Return(nodeStateAlive)
		node.On("Order").Return(int32(1))
		nodes = append(nodes, node)
	}

	firstPicks := map[Node[ID, nodeClient]]int{}
	for range 300 {
		firstPicks[newNodeSelector(NodeSelectionModePriorityLevel, nodes).Select()]++
	}
	for i, n := range nodes {
		assert.Positive(t, firstPicks[n], "no selector picked node %d first", i)
	}
}

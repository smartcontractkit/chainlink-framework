package multinode

import (
	"math"
	"math/rand/v2"
	"sort"
)

type priorityLevelNodeSelector[
	CHAIN_ID ID,
	RPC any,
] struct {
	nodes []Node[CHAIN_ID, RPC]
}

type nodeWithPriority[
	CHAIN_ID ID,
	RPC any,
] struct {
	node     Node[CHAIN_ID, RPC]
	priority int32
}

func NewPriorityLevelNodeSelector[
	CHAIN_ID ID,
	RPC any,
](nodes []Node[CHAIN_ID, RPC]) NodeSelector[CHAIN_ID, RPC] {
	return &priorityLevelNodeSelector[CHAIN_ID, RPC]{
		nodes: nodes,
	}
}

// Select picks a random alive node from the highest priority tier. Random rather than round-robin, so that pods sharing
// the same RPC set spread their load instead of all starting from the same node.
func (s priorityLevelNodeSelector[CHAIN_ID, RPC]) Select() Node[CHAIN_ID, RPC] {
	nodes := s.getHighestPriorityAliveTier()
	if len(nodes) == 0 {
		return nil
	}
	// #nosec G404
	return nodes[rand.IntN(len(nodes))].node
}

func (s priorityLevelNodeSelector[CHAIN_ID, RPC]) Name() string {
	return NodeSelectionModePriorityLevel
}

// getHighestPriorityAliveTier filters nodes that are not in state nodeStateAlive and
// returns only the highest tier of alive nodes
func (s priorityLevelNodeSelector[CHAIN_ID, RPC]) getHighestPriorityAliveTier() []nodeWithPriority[CHAIN_ID, RPC] {
	var nodes []nodeWithPriority[CHAIN_ID, RPC]
	for _, n := range s.nodes {
		if n.State() == nodeStateAlive {
			nodes = append(nodes, nodeWithPriority[CHAIN_ID, RPC]{n, n.Order()})
		}
	}

	if len(nodes) == 0 {
		return nil
	}

	return removeLowerTiers(nodes)
}

// removeLowerTiers take a slice of nodeWithPriority[CHAIN_ID, BLOCK_HASH, HEAD, RPC] and keeps only the highest tier
func removeLowerTiers[
	CHAIN_ID ID,
	RPC any,
](nodes []nodeWithPriority[CHAIN_ID, RPC]) []nodeWithPriority[CHAIN_ID, RPC] {
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].priority > nodes[j].priority
	})

	var nodes2 []nodeWithPriority[CHAIN_ID, RPC]
	currentPriority := nodes[len(nodes)-1].priority

	for _, n := range nodes {
		if n.priority == currentPriority {
			nodes2 = append(nodes2, n)
		}
	}

	return nodes2
}

// firstOrHighestPriority takes a list of nodes and returns the first one with the highest priority
func firstOrHighestPriority[
	CHAIN_ID ID,
	RPC any,
](nodes []Node[CHAIN_ID, RPC]) Node[CHAIN_ID, RPC] {
	hp := int32(math.MaxInt32)
	var node Node[CHAIN_ID, RPC]
	for _, n := range nodes {
		if n.Order() < hp {
			hp = n.Order()
			node = n
		}
	}
	return node
}

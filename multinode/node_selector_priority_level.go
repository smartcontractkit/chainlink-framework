package multinode

import (
	"math"
	"math/rand/v2"
)

type priorityLevelNodeSelector[
	CHAIN_ID ID,
	RPC any,
] struct {
	nodes []Node[CHAIN_ID, RPC]
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
	return nodes[rand.IntN(len(nodes))]
}

func (s priorityLevelNodeSelector[CHAIN_ID, RPC]) Name() string {
	return NodeSelectionModePriorityLevel
}

// getHighestPriorityAliveTier returns the alive nodes with the lowest Order (highest priority).
func (s priorityLevelNodeSelector[CHAIN_ID, RPC]) getHighestPriorityAliveTier() []Node[CHAIN_ID, RPC] {
	var tier []Node[CHAIN_ID, RPC]
	bestOrder := int32(math.MaxInt32)
	for _, n := range s.nodes {
		if n.State() != nodeStateAlive {
			continue
		}
		switch order := n.Order(); {
		case order < bestOrder:
			bestOrder = order
			tier = append(tier[:0], n)
		case order == bestOrder:
			tier = append(tier, n)
		}
	}
	return tier
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

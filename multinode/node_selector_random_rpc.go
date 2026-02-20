package multinode

import (
	"math/rand/v2"
)

type randomRPCSelector[
	CHAIN_ID ID,
	RPC any,
] struct {
	nodes []Node[CHAIN_ID, RPC]
}

func NewRandomRPCSelector[
	CHAIN_ID ID,
	RPC any,
](nodes []Node[CHAIN_ID, RPC]) NodeSelector[CHAIN_ID, RPC] {
	return &randomRPCSelector[CHAIN_ID, RPC]{
		nodes: nodes,
	}
}

func (s *randomRPCSelector[CHAIN_ID, RPC]) Select() Node[CHAIN_ID, RPC] {
	var liveNodes []Node[CHAIN_ID, RPC]
	for _, n := range s.nodes {
		if n.State() == nodeStateAlive {
			liveNodes = append(liveNodes, n)
		} else {
			n.UnsubscribeAllExceptAliveLoop()
		}
	}

	if len(liveNodes) == 0 {
		return nil
	}

	// #nosec G404
	return liveNodes[rand.IntN(len(liveNodes))]
}

func (s *randomRPCSelector[CHAIN_ID, RPC]) Name() string {
	return NodeSelectionModeRandomRPC
}

package multinode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRandomRPCNodeSelectorName(t *testing.T) {
	selector := newNodeSelector[ID, RPCClient[ID, Head]](NodeSelectionModeRandomRPC, nil)
	assert.Equal(t, NodeSelectionModeRandomRPC, selector.Name())
}

func TestRandomRPCNodeSelector(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	for i := 0; i < 3; i++ {
		node := newMockNode[ID, nodeClient](t)
		if i == 0 {
			node.On("State").Return(nodeStateOutOfSync)
			node.On("UnsubscribeAllExceptAliveLoop")
		} else {
			node.On("State").Return(nodeStateAlive)
		}
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeRandomRPC, nodes)

	// All selections should be from alive nodes only
	for i := 0; i < 20; i++ {
		selected := selector.Select()
		assert.NotNil(t, selected)
		assert.Contains(t, []Node[ID, nodeClient]{nodes[1], nodes[2]}, selected)
	}
}

func TestRandomRPCNodeSelector_None(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	for i := 0; i < 3; i++ {
		node := newMockNode[ID, nodeClient](t)
		if i == 0 {
			node.On("State").Return(nodeStateOutOfSync)
		} else {
			node.On("State").Return(nodeStateUnreachable)
		}
		node.On("UnsubscribeAllExceptAliveLoop")
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeRandomRPC, nodes)
	assert.Nil(t, selector.Select())
}

func TestRandomRPCNodeSelector_Distribution(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	const nAlive = 3
	for i := 0; i < nAlive; i++ {
		node := newMockNode[ID, nodeClient](t)
		node.On("State").Return(nodeStateAlive)
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeRandomRPC, nodes)

	const iterations = 1000
	counts := make(map[Node[ID, nodeClient]]int, nAlive)
	for i := 0; i < iterations; i++ {
		selected := selector.Select()
		assert.NotNil(t, selected)
		counts[selected]++
	}

	// Each node should be selected at least once with overwhelming probability
	for _, n := range nodes {
		assert.Positive(t, counts[n], "expected every alive node to be selected at least once")
	}
}

func TestRandomRPCNodeSelector_SingleNode(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]

	node := newMockNode[ID, nodeClient](t)
	node.On("State").Return(nodeStateAlive)

	selector := newNodeSelector(NodeSelectionModeRandomRPC, []Node[ID, nodeClient]{node})

	for i := 0; i < 5; i++ {
		assert.Same(t, node, selector.Select())
	}
}

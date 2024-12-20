package multinode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRoundRobinNodeSelectorName(t *testing.T) {
	selector := newNodeSelector[ID, RPCClient[ID, Head]](NodeSelectionModeRoundRobin, nil)
	assert.Equal(t, NodeSelectionModeRoundRobin, selector.Name())
}

func TestRoundRobinNodeSelector(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	for i := 0; i < 3; i++ {
		node := newMockNode[ID, nodeClient](t)
		if i == 0 {
			// first node is out of sync
			node.On("State").Return(nodeStateOutOfSync)
		} else {
			// second & third nodes are alive
			node.On("State").Return(nodeStateAlive)
		}
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeRoundRobin, nodes)
	assert.Same(t, nodes[1], selector.Select())
	assert.Same(t, nodes[2], selector.Select())
	assert.Same(t, nodes[1], selector.Select())
	assert.Same(t, nodes[2], selector.Select())
}

func TestRoundRobinNodeSelector_None(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	for i := 0; i < 3; i++ {
		node := newMockNode[ID, nodeClient](t)
		if i == 0 {
			// first node is out of sync
			node.On("State").Return(nodeStateOutOfSync)
		} else {
			// others are unreachable
			node.On("State").Return(nodeStateUnreachable)
		}
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeRoundRobin, nodes)
	assert.Nil(t, selector.Select())
}

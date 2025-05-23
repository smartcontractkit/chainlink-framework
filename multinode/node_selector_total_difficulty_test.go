package multinode

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTotalDifficultyNodeSelectorName(t *testing.T) {
	selector := newNodeSelector[ID, RPCClient[ID, Head]](NodeSelectionModeTotalDifficulty, nil)
	assert.Equal(t, NodeSelectionModeTotalDifficulty, selector.Name())
}

func TestTotalDifficultyNodeSelector(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	for i := 0; i < 3; i++ {
		node := newMockNode[ID, nodeClient](t)
		switch i {
		case 0:
			// first node is out of sync
			node.On("StateAndLatest").Return(nodeStateOutOfSync, ChainInfo{BlockNumber: -1})
		case 1:
			// second node is alive
			node.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(7)})
		default:
			// third node is alive and best
			node.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 2, TotalDifficulty: big.NewInt(8)})
		}
		node.On("Order").Maybe().Return(int32(1))
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
	assert.Same(t, nodes[2], selector.Select())

	t.Run("stick to the same node", func(t *testing.T) {
		node := newMockNode[ID, nodeClient](t)
		// fourth node is alive (same as 3rd)
		node.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 2, TotalDifficulty: big.NewInt(8)})
		node.On("Order").Maybe().Return(int32(1))
		nodes = append(nodes, node)

		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		assert.Same(t, nodes[2], selector.Select())
	})

	t.Run("another best node", func(t *testing.T) {
		node := newMockNode[ID, nodeClient](t)
		// fifth node is alive (better than 3rd and 4th)
		node.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 3, TotalDifficulty: big.NewInt(11)})
		node.On("Order").Maybe().Return(int32(1))
		nodes = append(nodes, node)

		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		assert.Same(t, nodes[4], selector.Select())
	})

	t.Run("nodes never update latest block number", func(t *testing.T) {
		node1 := newMockNode[ID, nodeClient](t)
		node1.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: -1, TotalDifficulty: nil})
		node1.On("Order").Maybe().Return(int32(1))
		node2 := newMockNode[ID, nodeClient](t)
		node2.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: -1, TotalDifficulty: nil})
		node2.On("Order").Maybe().Return(int32(1))
		nodes := []Node[ID, nodeClient]{node1, node2}

		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		assert.Same(t, node1, selector.Select())
	})
}

func TestTotalDifficultyNodeSelector_None(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	for i := 0; i < 3; i++ {
		node := newMockNode[ID, nodeClient](t)
		if i == 0 {
			// first node is out of sync
			node.On("StateAndLatest").Return(nodeStateOutOfSync, ChainInfo{BlockNumber: -1, TotalDifficulty: nil})
		} else {
			// others are unreachable
			node.On("StateAndLatest").Return(nodeStateUnreachable, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(7)})
		}
		nodes = append(nodes, node)
	}

	selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
	assert.Nil(t, selector.Select())
}

func TestTotalDifficultyNodeSelectorWithOrder(t *testing.T) {
	t.Parallel()

	type nodeClient RPCClient[ID, Head]
	var nodes []Node[ID, nodeClient]

	t.Run("same td and order", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			node := newMockNode[ID, nodeClient](t)
			node.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(10)})
			node.On("Order").Return(int32(2))
			nodes = append(nodes, node)
		}
		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		// Should select the first node because all things are equal
		assert.Same(t, nodes[0], selector.Select())
	})

	t.Run("same td but different order", func(t *testing.T) {
		node1 := newMockNode[ID, nodeClient](t)
		node1.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 3, TotalDifficulty: big.NewInt(10)})
		node1.On("Order").Return(int32(3))

		node2 := newMockNode[ID, nodeClient](t)
		node2.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 3, TotalDifficulty: big.NewInt(10)})
		node2.On("Order").Return(int32(1))

		node3 := newMockNode[ID, nodeClient](t)
		node3.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 3, TotalDifficulty: big.NewInt(10)})
		node3.On("Order").Return(int32(2))

		nodes := []Node[ID, nodeClient]{node1, node2, node3}
		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		// Should select the second node as it has the highest priority
		assert.Same(t, nodes[1], selector.Select())
	})

	t.Run("different td but same order", func(t *testing.T) {
		node1 := newMockNode[ID, nodeClient](t)
		node1.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(10)})
		node1.On("Order").Maybe().Return(int32(3))

		node2 := newMockNode[ID, nodeClient](t)
		node2.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(11)})
		node2.On("Order").Maybe().Return(int32(3))

		node3 := newMockNode[ID, nodeClient](t)
		node3.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(12)})
		node3.On("Order").Return(int32(3))

		nodes := []Node[ID, nodeClient]{node1, node2, node3}
		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		// Should select the third node as it has the highest td
		assert.Same(t, nodes[2], selector.Select())
	})

	t.Run("different head and different order", func(t *testing.T) {
		node1 := newMockNode[ID, nodeClient](t)
		node1.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(100)})
		node1.On("Order").Maybe().Return(int32(4))

		node2 := newMockNode[ID, nodeClient](t)
		node2.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(110)})
		node2.On("Order").Maybe().Return(int32(5))

		node3 := newMockNode[ID, nodeClient](t)
		node3.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(110)})
		node3.On("Order").Maybe().Return(int32(1))

		node4 := newMockNode[ID, nodeClient](t)
		node4.On("StateAndLatest").Return(nodeStateAlive, ChainInfo{BlockNumber: 1, TotalDifficulty: big.NewInt(105)})
		node4.On("Order").Maybe().Return(int32(2))

		nodes := []Node[ID, nodeClient]{node1, node2, node3, node4}
		selector := newNodeSelector(NodeSelectionModeTotalDifficulty, nodes)
		// Should select the third node as it has the highest td and will win the priority tie-breaker
		assert.Same(t, nodes[2], selector.Select())
	})
}

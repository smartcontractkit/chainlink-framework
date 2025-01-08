package multinode

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"sync"
)

var (
	metricsInit sync.Once
)

func init() {
	metricsInit.Do(func() {
		// Node Metrics
		promPoolRPCNodeVerifies = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_verifies",
			Help: "The total number of chain ID verifications for the given RPC node",
		}, []string{"network", "chainID", "nodeName"})
		promPoolRPCNodeVerifiesFailed = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_verifies_failed",
			Help: "The total number of failed chain ID verifications for the given RPC node",
		}, []string{"network", "chainID", "nodeName"})
		promPoolRPCNodeVerifiesSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_verifies_success",
			Help: "The total number of successful chain ID verifications for the given RPC node",
		}, []string{"network", "chainID", "nodeName"})

		// Node FSM Metrics
		promPoolRPCNodeTransitionsToAlive = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_alive",
			Help: transitionString(nodeStateAlive),
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeTransitionsToInSync = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_in_sync",
			Help: fmt.Sprintf("%s to %s", transitionString(nodeStateOutOfSync), nodeStateAlive),
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeTransitionsToOutOfSync = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_out_of_sync",
			Help: transitionString(nodeStateOutOfSync),
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeTransitionsToUnreachable = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_unreachable",
			Help: transitionString(nodeStateUnreachable),
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeTransitionsToInvalidChainID = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_invalid_chain_id",
			Help: transitionString(nodeStateInvalidChainID),
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeTransitionsToUnusable = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_unusable",
			Help: transitionString(nodeStateUnusable),
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeTransitionsToSyncing = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_transitions_to_syncing",
			Help: transitionString(nodeStateSyncing),
		}, []string{"chainID", "nodeName"})

		// Node Lifecycle Metrics
		promPoolRPCNodeHighestSeenBlock = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pool_rpc_node_highest_seen_block",
			Help: "The highest seen block for the given RPC node",
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeHighestFinalizedBlock = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pool_rpc_node_highest_finalized_block",
			Help: "The highest seen finalized block for the given RPC node",
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodeNumSeenBlocks = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_num_seen_blocks",
			Help: "The total number of new blocks seen by the given RPC node",
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodePolls = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_polls_total",
			Help: "The total number of poll checks for the given RPC node",
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodePollsFailed = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_polls_failed",
			Help: "The total number of failed poll checks for the given RPC node",
		}, []string{"chainID", "nodeName"})
		promPoolRPCNodePollsSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_rpc_node_polls_success",
			Help: "The total number of successful poll checks for the given RPC node",
		}, []string{"chainID", "nodeName"})

		// MultiNode Metrics
		PromMultiNodeRPCNodeStates = promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "multi_node_states",
			Help: "The number of RPC nodes currently in the given state for the given chain",
		}, []string{"network", "chainId", "state"})

		// Transaction Sender Metrics
		PromMultiNodeInvariantViolations = promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "multi_node_invariant_violations",
			Help: "The number of invariant violations",
		}, []string{"network", "chainId", "invariant"})
	})
}

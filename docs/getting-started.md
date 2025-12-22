# Getting Started

This guide walks you through installing and using the Chainlink Framework modules in your Go project.

## Installation

Each module can be installed independently. Choose the modules you need:

```bash
# MultiNode - multi-RPC client with health checks and load balancing
go get github.com/smartcontractkit/chainlink-framework/multinode

# Chains - transaction manager and head tracker
go get github.com/smartcontractkit/chainlink-framework/chains

# Capabilities - WriteTarget implementation
go get github.com/smartcontractkit/chainlink-framework/capabilities

# Metrics - Prometheus metrics for observability
go get github.com/smartcontractkit/chainlink-framework/metrics
```

## Quick Start: MultiNode

The most common entry point is `MultiNode`, which manages connections to multiple RPC endpoints.

### 1. Implement the RPCClient Interface

First, wrap your chain's RPC client to implement the required interface:

```go
package mychain

import (
    "context"
    "math/big"

    "github.com/smartcontractkit/chainlink-framework/multinode"
)

// MyRPCClient wraps your chain-specific RPC client
type MyRPCClient struct {
    // your chain's client
}

// ChainID returns the chain ID from the RPC
func (c *MyRPCClient) ChainID(ctx context.Context) (*big.Int, error) {
    // Implement chain ID retrieval
}

// Dial establishes connection to the RPC endpoint
func (c *MyRPCClient) Dial(ctx context.Context) error {
    // Implement connection logic
}

// Close terminates the RPC connection
func (c *MyRPCClient) Close() {
    // Implement cleanup
}

// SubscribeToHeads creates a subscription for new block headers
func (c *MyRPCClient) SubscribeToHeads(ctx context.Context) (<-chan Head, multinode.Subscription, error) {
    // Implement head subscription
}

// IsSyncing checks if the node is still syncing
func (c *MyRPCClient) IsSyncing(ctx context.Context) (bool, error) {
    // Implement sync check
}
```

### 2. Create Nodes

Wrap your RPC clients in `Node` instances:

```go
package main

import (
    "net/url"
    "time"

    "github.com/smartcontractkit/chainlink-common/pkg/logger"
    "github.com/smartcontractkit/chainlink-framework/multinode"
)

func createNode(
    lggr logger.Logger,
    metrics multinode.NodeMetrics,
    rpcURL string,
    name string,
    chainID *big.Int,
) multinode.Node[*big.Int, *MyRPCClient] {
    wsURL, _ := url.Parse(rpcURL)

    rpcClient := &MyRPCClient{/* init */}

    return multinode.NewNode(
        nodeConfig,      // NodeConfig implementation
        chainConfig,     // ChainConfig implementation
        lggr,
        metrics,
        wsURL,           // WebSocket URL
        nil,             // HTTP URL (optional)
        name,
        0,               // node ID
        chainID,
        0,               // node order/priority
        rpcClient,
        "MyChain",       // chain family name
        false,           // isLoadBalancedRPC
    )
}
```

### 3. Initialize MultiNode

Create and start the MultiNode manager:

```go
package main

import (
    "context"
    "time"

    "github.com/smartcontractkit/chainlink-common/pkg/logger"
    "github.com/smartcontractkit/chainlink-framework/multinode"
)

func main() {
    ctx := context.Background()
    lggr := logger.NewLogger()

    // Create primary nodes
    nodes := []multinode.Node[*big.Int, *MyRPCClient]{
        createNode(lggr, metrics, "wss://rpc1.example.com", "node1", chainID),
        createNode(lggr, metrics, "wss://rpc2.example.com", "node2", chainID),
    }

    // Create MultiNode
    mn := multinode.NewMultiNode(
        lggr,
        multiNodeMetrics,                           // metrics implementation
        multinode.NodeSelectionModeHighestHead,     // selection strategy
        time.Minute,                                // lease duration
        nodes,
        nil,                                        // send-only nodes
        chainID,
        "MyChain",
        5*time.Minute,                              // death declaration delay
    )

    // Start MultiNode
    if err := mn.Start(ctx); err != nil {
        panic(err)
    }
    defer mn.Close()

    // Select an RPC and use it
    rpc, err := mn.SelectRPC(ctx)
    if err != nil {
        panic(err)
    }

    // Use rpc for chain operations...
}
```

## Node Selection Modes

MultiNode supports several selection strategies:

| Mode              | Description                                    |
| ----------------- | ---------------------------------------------- |
| `HighestHead`     | Selects the node with the highest block number |
| `RoundRobin`      | Cycles through healthy nodes                   |
| `TotalDifficulty` | Selects based on chain difficulty (PoW chains) |
| `PriorityLevel`   | Selects based on configured node priorities    |

## Next Steps

- [Architecture](architecture.md) — Understand how components interact
- [Usage Guide](usage-guide.md) — Detailed integration patterns
- [Contributing](contributing.md) — Set up your development environment

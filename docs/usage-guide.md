# Usage Guide

This guide provides detailed integration patterns and examples for each Chainlink Framework component.

## MultiNode

MultiNode enables resilient connections to multiple RPC endpoints with automatic failover.

### Implementing RPCClient

The core interface you must implement for your chain:

```go
// RPCClient wraps chain-specific RPC functionality
type RPCClient[CHAIN_ID ID, HEAD Head] interface {
    // Connection lifecycle
    Dial(ctx context.Context) error
    Close()

    // Chain information
    ChainID(ctx context.Context) (CHAIN_ID, error)
    IsSyncing(ctx context.Context) (bool, error)

    // Head tracking
    SubscribeToHeads(ctx context.Context) (<-chan HEAD, Subscription, error)
    SubscribeToFinalizedHeads(ctx context.Context) (<-chan HEAD, Subscription, error)

    // Subscription management
    UnsubscribeAllExcept(subs ...Subscription)

    // Observations (for node selection)
    GetInterceptedChainInfo() (latest, highest ChainInfo)
}
```

### Node Configuration

Configure node behavior via the `NodeConfig` interface:

```go
type NodeConfig interface {
    PollFailureThreshold() uint32     // Failures before marking unreachable
    PollInterval() time.Duration      // Health check interval
    SelectionMode() string            // Node selection strategy
    SyncThreshold() uint32            // Blocks behind before out-of-sync
    NodeIsSyncingEnabled() bool       // Check if node is syncing
    FinalizedBlockPollInterval() time.Duration
    EnforceRepeatableRead() bool
    DeathDeclarationDelay() time.Duration
    NewHeadsPollInterval() time.Duration
    VerifyChainID() bool              // Verify chain ID on connect
}
```

### Node Selection Strategies

#### HighestHead

Selects the node with the highest observed block number:

```go
mn := multinode.NewMultiNode(
    lggr,
    metrics,
    multinode.NodeSelectionModeHighestHead,
    // ...
)
```

#### RoundRobin

Cycles through healthy nodes evenly:

```go
mn := multinode.NewMultiNode(
    lggr,
    metrics,
    multinode.NodeSelectionModeRoundRobin,
    // ...
)
```

#### PriorityLevel

Selects based on configured node priority:

```go
// When creating nodes, set priority via nodeOrder parameter
node := multinode.NewNode(
    nodeCfg, chainCfg, lggr, metrics,
    wsURL, httpURL, "primary-node",
    1,          // node ID
    chainID,
    0,          // priority: lower = higher priority
    rpc,
    "EVM",
    false,
)
```

### Broadcasting to All Nodes

Use `DoAll` to execute operations across all healthy nodes:

```go
err := mn.DoAll(ctx, func(ctx context.Context, rpc *MyRPCClient, isSendOnly bool) {
    if isSendOnly {
        // Handle send-only nodes differently if needed
        return
    }
    // Execute operation on each node
    rpc.DoSomething(ctx)
})
```

### Monitoring Node Health

Check current node states:

```go
states := mn.NodeStates()
for nodeName, state := range states {
    fmt.Printf("Node %s: %s\n", nodeName, state)
}

// Get chain info from live nodes
nLive, chainInfo := mn.LatestChainInfo()
fmt.Printf("Live nodes: %d, Highest block: %d\n", nLive, chainInfo.BlockNumber)
```

---

## Transaction Manager

The TxManager handles transaction lifecycle with automatic retry and gas bumping.

### Creating Transactions

```go
import (
    "github.com/smartcontractkit/chainlink-framework/chains/txmgr"
    txmgrtypes "github.com/smartcontractkit/chainlink-framework/chains/txmgr/types"
)

// Create a transaction request
request := txmgrtypes.TxRequest[common.Address, common.Hash]{
    FromAddress:    fromAddr,
    ToAddress:      toAddr,
    EncodedPayload: calldata,
    Value:          big.NewInt(0),
    FeeLimit:       gasLimit,
    IdempotencyKey: &idempotencyKey,  // Prevents duplicate sends
    Strategy:       txmgr.NewSendEveryStrategy(),
}

// Submit transaction
tx, err := txm.CreateTransaction(ctx, request)
if err != nil {
    return err
}
```

### Transaction Strategies

Control transaction queueing behavior:

```go
// Send every transaction (default)
request.Strategy = txmgr.NewSendEveryStrategy()

// Drop old transactions for same key
request.Strategy = txmgr.NewDropOldestStrategy(subjectKey, maxQueued)

// Queue limit per subject
request.Strategy = txmgr.NewQueueingTxStrategy(subjectKey, maxQueued)
```

### Checking Transaction Status

```go
status, err := txm.GetTransactionStatus(ctx, idempotencyKey)
switch status {
case commontypes.Pending:
    // Transaction submitted, awaiting confirmation
case commontypes.Unconfirmed:
    // Transaction confirmed but not yet finalized
case commontypes.Finalized:
    // Transaction finalized
case commontypes.Failed:
    // Transaction failed (retryable)
case commontypes.Fatal:
    // Transaction failed (not retryable)
}
```

### Retrieving Transaction Fee

```go
fee, err := txm.GetTransactionFee(ctx, idempotencyKey)
if err != nil {
    return err
}
fmt.Printf("Transaction fee: %s wei\n", fee.TransactionFee.String())
```

### Using Forwarders

Enable meta-transactions via forwarder contracts:

```go
// Enable in config
// Transactions.ForwardersEnabled = true

// Get forwarder for an EOA
forwarder, err := txm.GetForwarderForEOA(ctx, eoaAddress)

// Use in transaction request
request := txmgrtypes.TxRequest{
    FromAddress:      eoaAddress,
    ForwarderAddress: forwarder,
    // ...
}
```

---

## Head Tracker

The HeadTracker monitors blockchain heads and maintains finality information.

### Subscribing to New Heads

Implement the `Trackable` interface:

```go
type MyService struct {
    // ...
}

func (s *MyService) OnNewLongestChain(ctx context.Context, head Head) {
    // React to new chain head
    fmt.Printf("New head: %d\n", head.BlockNumber())
}
```

Register with the HeadBroadcaster:

```go
unsubscribe := headBroadcaster.Subscribe(&myService)
defer unsubscribe()
```

### Getting Latest Chain Info

```go
// Get the latest chain head
latestHead := tracker.LatestChain()
if latestHead.IsValid() {
    fmt.Printf("Latest: %d\n", latestHead.BlockNumber())
}

// Get latest and finalized blocks
latest, finalized, err := tracker.LatestAndFinalizedBlock(ctx)
if err != nil {
    return err
}
fmt.Printf("Latest: %d, Finalized: %d\n",
    latest.BlockNumber(),
    finalized.BlockNumber())
```

### Safe Block Access

Get the latest "safe" block (useful for reorg-sensitive operations):

```go
safeHead, err := tracker.LatestSafeBlock(ctx)
if err != nil {
    return err
}
// Use safeHead for operations that shouldn't be affected by reorgs
```

---

## Write Target

The WriteTarget capability enables workflow-based transaction submission.

### Implementing TargetStrategy

Chain-specific implementations must implement the `TargetStrategy` interface:

```go
type TargetStrategy interface {
    // Check if report was already transmitted
    QueryTransmissionState(
        ctx context.Context,
        reportID uint16,
        request capabilities.CapabilityRequest,
    ) (*TransmissionState, error)

    // Submit the report transaction
    TransmitReport(
        ctx context.Context,
        report []byte,
        reportContext []byte,
        signatures [][]byte,
        request capabilities.CapabilityRequest,
    ) (string, error)

    // Get transaction status
    GetTransactionStatus(
        ctx context.Context,
        transactionID string,
    ) (commontypes.TransactionStatus, error)

    // Get fee estimate
    GetEstimateFee(
        ctx context.Context,
        report []byte,
        reportContext []byte,
        signatures [][]byte,
        request capabilities.CapabilityRequest,
    ) (commontypes.EstimateFee, error)

    // Get actual transaction fee
    GetTransactionFee(
        ctx context.Context,
        transactionID string,
    ) (decimal.Decimal, error)
}
```

### Creating a WriteTarget

```go
import (
    "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget"
)

// Generate capability ID
capID, err := writetarget.NewWriteTargetID(
    "evm",           // chain family
    "mainnet",       // network name
    "1",             // chain ID
    "1.0.0",         // version
)

// Create WriteTarget
wt := writetarget.NewWriteTarget(writetarget.WriteTargetOpts{
    ID:               capID,
    Config:           writeTargetConfig,
    ChainInfo:        chainInfo,
    Logger:           lggr,
    Beholder:         beholderClient,
    ChainService:     chainService,
    ConfigValidateFn: validateConfig,
    NodeAddress:      nodeAddr,
    ForwarderAddress: forwarderAddr,
    TargetStrategy:   myStrategy,
})
```

### Reference Implementations

- **EVM**: [chainlink/core/services/relay/evm/target_strategy.go](https://github.com/smartcontractkit/chainlink)
- **Aptos**: [chainlink-aptos/relayer/write_target/strategy.go](https://github.com/smartcontractkit/chainlink-aptos)

---

## Metrics

The metrics module provides Prometheus instrumentation for chain observability.

### Using Balance Metrics

```go
import "github.com/smartcontractkit/chainlink-framework/metrics"

balanceMetrics := metrics.NewBalance(lggr)

// Record balance
balanceMetrics.Set(
    ctx,
    balance,      // *big.Int
    address,      // string
    chainID,      // string
    chainFamily,  // e.g., "EVM"
)
```

### Using TxManager Metrics

```go
txmMetrics := metrics.NewTxm(lggr)

// Record transaction states
txmMetrics.RecordTxState(ctx, txState, chainID)

// Record gas bump
txmMetrics.RecordGasBump(ctx, amount, chainID)
```

### Using MultiNode Metrics

```go
multiNodeMetrics := metrics.NewMultiNode(lggr)

// Record node states
multiNodeMetrics.RecordNodeStates(ctx, "Alive", 3)
multiNodeMetrics.RecordNodeStates(ctx, "Unreachable", 1)
```

---

## Performance Tips

### Connection Pooling

Configure appropriate lease duration to balance load distribution and connection stability:

```go
mn := multinode.NewMultiNode(
    lggr,
    metrics,
    multinode.NodeSelectionModeHighestHead,
    5*time.Minute,  // Lease duration: higher = more stable, lower = better load distribution
    // ...
)
```

### Transaction Batching

For high-throughput scenarios, configure queue capacity:

```go
// In TxConfig
MaxQueued: 1000           // Max pending transactions per address
ResendAfterThreshold: 30s // Resend stuck transactions
```

### Head Sampling

For chains with fast block times, enable head sampling to reduce processing load:

```go
// In TrackerConfig
SamplingInterval: 100ms   // Process heads at most every 100ms
```

---

## Troubleshooting

### All Nodes Unreachable

Check node health and logs:

```go
states := mn.NodeStates()
for name, state := range states {
    if state == "Unreachable" {
        log.Warnf("Node %s is unreachable", name)
    }
}
```

Common causes:

- Network connectivity issues
- RPC endpoint rate limiting
- Chain ID mismatch

### Transactions Stuck

Check transaction status and consider manual intervention:

```go
status, err := txm.GetTransactionStatus(ctx, txID)
if status == commontypes.Pending {
    // Transaction may need gas bump or is stuck in mempool
}

// Reset and abandon stuck transactions for an address
err = txm.Reset(address, true)
```

### Head Tracker Falling Behind

Monitor finality violations:

```go
// Check if tracker detected finality issues
report := tracker.HealthReport()
if err, ok := report["HeadTracker"]; ok && err != nil {
    log.Errorf("HeadTracker unhealthy: %v", err)
}
```

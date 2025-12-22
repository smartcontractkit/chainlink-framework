# Architecture

This document describes the internal architecture of each Chainlink Framework module and how they interact.

## System Overview

```mermaid
flowchart TB
    subgraph "Chainlink Node"
        subgraph "chainlink-framework"
            MN[MultiNode]
            TXM[TxManager]
            HT[HeadTracker]
            WT[WriteTarget]
        end

        subgraph "Chain-Specific"
            RPC1[RPC Client 1]
            RPC2[RPC Client 2]
            RPCN[RPC Client N]
        end
    end

    subgraph "Blockchain Network"
        N1[Node 1]
        N2[Node 2]
        NN[Node N]
    end

    MN --> RPC1
    MN --> RPC2
    MN --> RPCN
    TXM --> MN
    HT --> MN
    WT --> TXM

    RPC1 --> N1
    RPC2 --> N2
    RPCN --> NN
```

## MultiNode Architecture

MultiNode is the core component that manages connections to multiple RPC endpoints, providing health monitoring, load balancing, and automatic failover.

### Component Diagram

```mermaid
flowchart TB
    subgraph MultiNode
        NS[NodeSelector]
        NM[Node Manager]
        SO[SendOnly Nodes]
        TS[Transaction Sender]

        subgraph "Node Pool"
            N1[Node 1]
            N2[Node 2]
            N3[Node N]
        end
    end

    subgraph "Per Node"
        subgraph "Node"
            FSM[State Machine]
            RPC[RPC Client]
            LC[Lifecycle Manager]
            HC[Health Checks]
        end
    end

    NS --> N1
    NS --> N2
    NS --> N3
    TS --> N1
    TS --> N2
    TS --> N3
    TS --> SO

    N1 --> FSM
    FSM --> RPC
    LC --> FSM
    HC --> FSM
```

### Node State Machine

Each node maintains a finite state machine tracking its health status:

```mermaid
stateDiagram-v2
    [*] --> Undialed: Created
    Undialed --> Dialed: Dial success
    Undialed --> Unreachable: Dial failed

    Dialed --> Alive: Verification passed
    Dialed --> InvalidChainID: Chain ID mismatch
    Dialed --> Syncing: Node is syncing
    Dialed --> Unreachable: Connection error

    Alive --> OutOfSync: Head too far behind
    Alive --> Unreachable: Connection lost

    OutOfSync --> Alive: Caught up
    OutOfSync --> Unreachable: Connection lost

    InvalidChainID --> Dialed: Retry verification
    Syncing --> Dialed: Retry verification
    Unreachable --> Undialed: Redial

    Alive --> Closed: Shutdown
    OutOfSync --> Closed: Shutdown
    Unreachable --> Closed: Shutdown
    Closed --> [*]
```

### Node Selection Flow

```mermaid
sequenceDiagram
    participant Client
    participant MultiNode
    participant NodeSelector
    participant Node1
    participant Node2

    Client->>MultiNode: SelectRPC()
    MultiNode->>MultiNode: Check activeNode
    alt Active node alive
        MultiNode-->>Client: Return activeNode.RPC()
    else Need new node
        MultiNode->>NodeSelector: Select()
        NodeSelector->>Node1: State()
        Node1-->>NodeSelector: Alive
        NodeSelector->>Node2: State()
        Node2-->>NodeSelector: OutOfSync
        NodeSelector-->>MultiNode: Node1 (best)
        MultiNode->>MultiNode: Set activeNode = Node1
        MultiNode-->>Client: Return Node1.RPC()
    end
```

## Transaction Manager (TxManager)

The TxManager handles transaction lifecycle from creation through finalization.

### Component Structure

```mermaid
flowchart TB
    subgraph TxManager
        BC[Broadcaster]
        CF[Confirmer]
        TK[Tracker]
        FN[Finalizer]
        RS[Resender]
        RP[Reaper]
        TS[TxStore]
    end

    subgraph External
        HT[HeadTracker]
        KS[KeyStore]
        MN[MultiNode]
    end

    HT -->|New heads| BC
    HT -->|New heads| CF
    HT -->|New heads| FN

    BC --> TS
    CF --> TS
    TK --> TS
    FN --> TS

    BC --> MN
    CF --> MN
    RS --> MN

    KS --> BC
    KS --> TK
```

### Transaction Lifecycle

```mermaid
stateDiagram-v2
    [*] --> Unstarted: CreateTransaction()

    Unstarted --> InProgress: Broadcaster picks up
    InProgress --> Unconfirmed: Broadcast success
    InProgress --> FatalError: Broadcast fatal error

    Unconfirmed --> Confirmed: Receipt found
    Unconfirmed --> Unconfirmed: Bump gas (rebroadcast)

    Confirmed --> ConfirmedMissingReceipt: Receipt lost
    Confirmed --> Finalized: Block finalized

    ConfirmedMissingReceipt --> Confirmed: Receipt refound
    ConfirmedMissingReceipt --> FatalError: Abandoned

    Finalized --> [*]
    FatalError --> [*]
```

### Transaction Flow

```mermaid
sequenceDiagram
    participant Client
    participant TxManager
    participant Broadcaster
    participant Confirmer
    participant Finalizer
    participant TxStore
    participant MultiNode

    Client->>TxManager: CreateTransaction(request)
    TxManager->>TxStore: Store transaction
    TxManager->>Broadcaster: Trigger(address)

    Broadcaster->>TxStore: Get unstarted txs
    Broadcaster->>Broadcaster: Build attempt
    Broadcaster->>MultiNode: SendTransaction
    Broadcaster->>TxStore: Update state -> Unconfirmed

    Note over Confirmer: On new block head
    Confirmer->>TxStore: Get unconfirmed txs
    Confirmer->>MultiNode: GetReceipt
    Confirmer->>TxStore: Update state -> Confirmed

    Note over Finalizer: On finalized block
    Finalizer->>TxStore: Get confirmed txs
    Finalizer->>TxStore: Update state -> Finalized
    Finalizer-->>Client: Resume callback
```

## Head Tracker

The HeadTracker monitors blockchain heads and maintains a local chain cache.

### Component Structure

```mermaid
flowchart TB
    subgraph HeadTracker
        HL[HeadListener]
        HS[HeadSaver]
        HB[HeadBroadcaster]
        BF[Backfiller]
    end

    subgraph External
        MN[MultiNode/RPC]
        DB[(Database)]
        TXM[TxManager]
        LP[LogPoller]
    end

    MN -->|Subscribe| HL
    HL -->|New head| HS
    HS --> DB
    HS --> HB
    HS --> BF
    BF --> MN
    HB --> TXM
    HB --> LP
```

### Head Processing Flow

```mermaid
sequenceDiagram
    participant RPC
    participant Listener
    participant Saver
    participant Backfiller
    participant Broadcaster
    participant Subscribers

    RPC->>Listener: New head (height N)
    Listener->>Saver: Save(head)
    Saver->>Saver: Update chain cache

    alt Head is new highest
        Saver->>Backfiller: Backfill(head, prevHead)
        Backfiller->>RPC: Fetch missing blocks
        Backfiller->>Saver: Save missing heads
        Saver->>Broadcaster: BroadcastNewLongestChain
        Broadcaster->>Subscribers: OnNewLongestChain(head)
    else Head is duplicate/old
        Note over Saver: Skip broadcast
    end
```

## Write Target (Capabilities)

The WriteTarget capability enables chain-agnostic transaction submission for Chainlink workflows.

### Component Structure

```mermaid
flowchart TB
    subgraph WriteTarget
        WT[WriteTarget]
        TS[TargetStrategy]
        MB[MessageBuilder]
        RT[Retry Logic]
    end

    subgraph External
        WE[Workflow Engine]
        CR[ChainReader]
        CW[ChainWriter]
        BH[Beholder/Telemetry]
    end

    WE -->|Execute| WT
    WT --> TS
    TS --> CR
    TS --> CW
    WT --> MB
    MB --> BH
    WT --> RT
```

### Write Execution Flow

```mermaid
sequenceDiagram
    participant Workflow
    participant WriteTarget
    participant Strategy
    participant ChainReader
    participant ChainWriter
    participant Beholder

    Workflow->>WriteTarget: Execute(request)
    WriteTarget->>WriteTarget: Validate config
    WriteTarget->>WriteTarget: Parse signed report
    WriteTarget->>Beholder: Emit WriteInitiated

    alt Empty report
        WriteTarget->>Beholder: Emit WriteSkipped
        WriteTarget-->>Workflow: Success (no-op)
    else Valid report
        WriteTarget->>Strategy: QueryTransmissionState
        Strategy->>ChainReader: Check if already transmitted

        alt Already transmitted
            WriteTarget->>Beholder: Emit WriteConfirmed
            WriteTarget-->>Workflow: Success
        else Not transmitted
            WriteTarget->>Strategy: TransmitReport
            Strategy->>ChainWriter: Submit transaction
            WriteTarget->>Beholder: Emit WriteSent

            loop Until confirmed or timeout
                WriteTarget->>Strategy: GetTransactionStatus
                WriteTarget->>Strategy: QueryTransmissionState
            end

            WriteTarget->>Beholder: Emit WriteConfirmed
            WriteTarget-->>Workflow: Success + fee metering
        end
    end
```

## Data Flow Summary

```mermaid
flowchart LR
    subgraph Input
        WF[Workflows]
        TX[Transactions]
    end

    subgraph Processing
        WT[WriteTarget]
        TXM[TxManager]
        HT[HeadTracker]
    end

    subgraph Infrastructure
        MN[MultiNode]
    end

    subgraph Output
        BC[Blockchain]
        MT[Metrics]
        TL[Telemetry]
    end

    WF --> WT
    TX --> TXM
    WT --> TXM
    TXM --> MN
    HT --> MN
    MN --> BC
    MN --> MT
    WT --> TL
    TXM --> MT
```

## Key Design Principles

1. **Generic Types**: All components use Go generics to remain chain-agnostic
2. **Interface-Based**: Chain-specific behavior is injected via interfaces
3. **Service Pattern**: Components implement `services.Service` for lifecycle management
4. **Observability**: Built-in metrics and structured logging throughout
5. **Resilience**: Automatic retries, reconnection, and graceful degradation

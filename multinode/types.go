package multinode

import (
	"context"
	"fmt"
	"math/big"
)

// ID represents the base type, for any chain's ID.
// It should be convertible to a string, that can uniquely identify this chain
type ID fmt.Stringer

// StringID enables using string directly as a ChainID
type StringID string

func (s StringID) String() string {
	return string(s)
}

// Subscription represents an event subscription where events are
// delivered on a data channel.
// This is a generic interface for Subscription to represent used by clients.
type Subscription interface {
	// Unsubscribe cancels the sending of events to the data channel
	// and closes the error channel. Unsubscribe should be callable multiple
	// times without causing an error.
	Unsubscribe()
	// Err returns the subscription error channel. The error channel receives
	// a value if there is an issue with the subscription (e.g. the network connection
	// delivering the events has been closed). Only one value will ever be sent.
	// The error channel is closed by Unsubscribe.
	Err() <-chan error
}

// ManagedSubscription is a Subscription which contains an onUnsubscribe callback for cleanup
type ManagedSubscription struct {
	Subscription
	onUnsubscribe func(sub Subscription)
}

func (w *ManagedSubscription) Unsubscribe() {
	w.Subscription.Unsubscribe()
	if w.onUnsubscribe != nil {
		w.onUnsubscribe(w)
	}
}

// RPCClient includes all the necessary generalized RPC methods used by Node to perform health checks
type RPCClient[
	CHAIN_ID ID,
	HEAD Head,
] interface {
	// ChainID - fetches ChainID from the RPC to verify that it matches config
	ChainID(ctx context.Context) (CHAIN_ID, error)
	// Dial - prepares the RPC for usage. Can be called on fresh or closed RPC
	Dial(ctx context.Context) error
	// SubscribeToHeads - returns channel and subscription for new heads.
	SubscribeToHeads(ctx context.Context) (<-chan HEAD, Subscription, error)
	// SubscribeToFinalizedHeads - returns channel and subscription for finalized heads.
	SubscribeToFinalizedHeads(ctx context.Context) (<-chan HEAD, Subscription, error)
	// ClientVersion - returns error if RPC is not reachable
	ClientVersion(context.Context) (string, error)
	// IsSyncing - returns true if the RPC is in Syncing state and can not process calls
	IsSyncing(ctx context.Context) (bool, error)
	// UnsubscribeAllExcept - close all subscriptions except `subs`
	UnsubscribeAllExcept(subs ...Subscription)
	// Close - closes all subscriptions and aborts all RPC calls
	Close()
	// GetInterceptedChainInfo - returns latest and highest observed by application layer ChainInfo.
	// latestChainInfo is the most recent value received within a NodeClient's current lifecycle between Dial and DisconnectAll.
	// highestUserObservations is the highest ChainInfo observed excluding health checks calls.
	// Its values must not be reset.
	// The results of corresponding calls, to get the most recent head and the latest finalized head, must be
	// intercepted and reflected in ChainInfo before being returned to a caller. Otherwise, MultiNode is not able to
	// provide repeatable read guarantee.
	// DisconnectAll must reset latest ChainInfo to default value.
	// Ensure implementation does not have a race condition when values are reset before request completion and as
	// a result latest ChainInfo contains information from the previous cycle.
	GetInterceptedChainInfo() (latest, highestUserObservations ChainInfo)
}

// Head is the interface required by the NodeClient
type Head interface {
	BlockNumber() int64
	BlockDifficulty() *big.Int
	GetTotalDifficulty() *big.Int
	IsValid() bool
}

// PoolChainInfoProvider - provides aggregation of nodes pool ChainInfo
type PoolChainInfoProvider interface {
	// LatestChainInfo - returns number of live nodes available in the pool, so we can prevent the last alive node in a pool from being
	// moved to out-of-sync state. It is better to have one out-of-sync node than no nodes at all.
	// Returns highest latest ChainInfo within the alive nodes. E.g. most recent block number and highest block number
	// observed by Node A are 10 and 15; Node B - 12 and 14. This method will return 12.
	LatestChainInfo() (int, ChainInfo)
	// HighestUserObservations - returns highest ChainInfo ever observed by any user of MultiNode.
	HighestUserObservations() ChainInfo
}

// ChainInfo - defines RPC's or MultiNode's view on the chain
type ChainInfo struct {
	BlockNumber          int64
	FinalizedBlockNumber int64
	TotalDifficulty      *big.Int
}

func MaxTotalDifficulty(a, b *big.Int) *big.Int {
	if a == nil {
		if b == nil {
			return nil
		}

		return big.NewInt(0).Set(b)
	}

	if b == nil || a.Cmp(b) >= 0 {
		return big.NewInt(0).Set(a)
	}

	return big.NewInt(0).Set(b)
}

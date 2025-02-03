package types

import (
	"context"

	"github.com/smartcontractkit/chainlink-framework/chains"
)

// KeyStore encompasses the subset of keystore used by txmgr
type KeyStore[
	// Account Address type.
	ADDR chains.Hashable,
	// Chain ID type
	CHAIN_ID chains.ID,
] interface {
	CheckEnabled(ctx context.Context, address ADDR, chainID CHAIN_ID) error
	EnabledAddressesForChain(ctx context.Context, chainID CHAIN_ID) ([]ADDR, error)
	SubscribeToKeyChanges(ctx context.Context) (ch chan struct{}, unsub func())
}

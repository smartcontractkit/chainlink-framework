package multinode

import (
	"context"
)

// CheckFinalizedStateAvailability extends mockRPCClient to also satisfy FinalizedStateChecker,
// allowing the type assertion any(n.rpc).(FinalizedStateChecker) to succeed in tests.
func (_m *mockRPCClient[CHAIN_ID, HEAD]) CheckFinalizedStateAvailability(ctx context.Context) error {
	ret := _m.Called(ctx)

	if len(ret) == 0 {
		panic("no return value specified for CheckFinalizedStateAvailability")
	}

	var r0 error
	if rf, ok := ret.Get(0).(func(context.Context) error); ok {
		r0 = rf(ctx)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

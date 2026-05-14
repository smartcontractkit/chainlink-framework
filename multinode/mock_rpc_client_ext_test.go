package multinode

import (
	"context"

	mock "github.com/stretchr/testify/mock"
)

// mockRPCClientCheckFinalizedStateAvailabilityCall mirrors mockery-generated *Call types for
// CheckFinalizedStateAvailability (added manually on mockRPCClient, not on RPCClient interface).
// Named without underscores to satisfy revive var-naming (mockery-generated code uses underscores).
type mockRPCClientCheckFinalizedStateAvailabilityCall[CHAIN_ID ID, HEAD Head] struct {
	*mock.Call
}

// CheckFinalizedStateAvailability is a helper to define EXPECT().CheckFinalizedStateAvailability(...).
func (_e *mockRPCClient_Expecter[CHAIN_ID, HEAD]) CheckFinalizedStateAvailability(ctx interface{}) *mockRPCClientCheckFinalizedStateAvailabilityCall[CHAIN_ID, HEAD] {
	return &mockRPCClientCheckFinalizedStateAvailabilityCall[CHAIN_ID, HEAD]{Call: _e.mock.On("CheckFinalizedStateAvailability", ctx)}
}

func (_c *mockRPCClientCheckFinalizedStateAvailabilityCall[CHAIN_ID, HEAD]) Return(err error) *mockRPCClientCheckFinalizedStateAvailabilityCall[CHAIN_ID, HEAD] {
	_c.Call.Return(err)
	return _c
}

// CheckFinalizedStateAvailability extends mockRPCClient to also satisfy FinalizedStateChecker,
// so NewNode populates finalizedStateChecker for tests that exercise finalized-state checks.
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

// Code generated by mockery v2.46.3. DO NOT EDIT.

package multinode

import mock "github.com/stretchr/testify/mock"

// mockPoolChainInfoProvider is an autogenerated mock type for the PoolChainInfoProvider type
type mockPoolChainInfoProvider struct {
	mock.Mock
}

type mockPoolChainInfoProvider_Expecter struct {
	mock *mock.Mock
}

func (_m *mockPoolChainInfoProvider) EXPECT() *mockPoolChainInfoProvider_Expecter {
	return &mockPoolChainInfoProvider_Expecter{mock: &_m.Mock}
}

// HighestUserObservations provides a mock function with given fields:
func (_m *mockPoolChainInfoProvider) HighestUserObservations() ChainInfo {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for HighestUserObservations")
	}

	var r0 ChainInfo
	if rf, ok := ret.Get(0).(func() ChainInfo); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(ChainInfo)
	}

	return r0
}

// mockPoolChainInfoProvider_HighestUserObservations_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'HighestUserObservations'
type mockPoolChainInfoProvider_HighestUserObservations_Call struct {
	*mock.Call
}

// HighestUserObservations is a helper method to define mock.On call
func (_e *mockPoolChainInfoProvider_Expecter) HighestUserObservations() *mockPoolChainInfoProvider_HighestUserObservations_Call {
	return &mockPoolChainInfoProvider_HighestUserObservations_Call{Call: _e.mock.On("HighestUserObservations")}
}

func (_c *mockPoolChainInfoProvider_HighestUserObservations_Call) Run(run func()) *mockPoolChainInfoProvider_HighestUserObservations_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *mockPoolChainInfoProvider_HighestUserObservations_Call) Return(_a0 ChainInfo) *mockPoolChainInfoProvider_HighestUserObservations_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *mockPoolChainInfoProvider_HighestUserObservations_Call) RunAndReturn(run func() ChainInfo) *mockPoolChainInfoProvider_HighestUserObservations_Call {
	_c.Call.Return(run)
	return _c
}

// LatestChainInfo provides a mock function with given fields:
func (_m *mockPoolChainInfoProvider) LatestChainInfo() (int, ChainInfo) {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for LatestChainInfo")
	}

	var r0 int
	var r1 ChainInfo
	if rf, ok := ret.Get(0).(func() (int, ChainInfo)); ok {
		return rf()
	}
	if rf, ok := ret.Get(0).(func() int); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(int)
	}

	if rf, ok := ret.Get(1).(func() ChainInfo); ok {
		r1 = rf()
	} else {
		r1 = ret.Get(1).(ChainInfo)
	}

	return r0, r1
}

// mockPoolChainInfoProvider_LatestChainInfo_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'LatestChainInfo'
type mockPoolChainInfoProvider_LatestChainInfo_Call struct {
	*mock.Call
}

// LatestChainInfo is a helper method to define mock.On call
func (_e *mockPoolChainInfoProvider_Expecter) LatestChainInfo() *mockPoolChainInfoProvider_LatestChainInfo_Call {
	return &mockPoolChainInfoProvider_LatestChainInfo_Call{Call: _e.mock.On("LatestChainInfo")}
}

func (_c *mockPoolChainInfoProvider_LatestChainInfo_Call) Run(run func()) *mockPoolChainInfoProvider_LatestChainInfo_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *mockPoolChainInfoProvider_LatestChainInfo_Call) Return(_a0 int, _a1 ChainInfo) *mockPoolChainInfoProvider_LatestChainInfo_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *mockPoolChainInfoProvider_LatestChainInfo_Call) RunAndReturn(run func() (int, ChainInfo)) *mockPoolChainInfoProvider_LatestChainInfo_Call {
	_c.Call.Return(run)
	return _c
}

// newMockPoolChainInfoProvider creates a new instance of mockPoolChainInfoProvider. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func newMockPoolChainInfoProvider(t interface {
	mock.TestingT
	Cleanup(func())
}) *mockPoolChainInfoProvider {
	mock := &mockPoolChainInfoProvider{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}

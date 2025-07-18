package multinode

import (
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	"github.com/smartcontractkit/chainlink-framework/metrics"
	"github.com/smartcontractkit/chainlink-framework/multinode/mocks"
)

type testNodeConfig struct {
	pollFailureThreshold       uint32
	pollInterval               time.Duration
	selectionMode              string
	syncThreshold              uint32
	nodeIsSyncingEnabled       bool
	enforceRepeatableRead      bool
	finalizedBlockPollInterval time.Duration
	deathDeclarationDelay      time.Duration
	newHeadsPollInterval       time.Duration
}

func (n testNodeConfig) NewHeadsPollInterval() time.Duration {
	return n.newHeadsPollInterval
}

func (n testNodeConfig) PollFailureThreshold() uint32 {
	return n.pollFailureThreshold
}

func (n testNodeConfig) PollInterval() time.Duration {
	return n.pollInterval
}

func (n testNodeConfig) SelectionMode() string {
	return n.selectionMode
}

func (n testNodeConfig) SyncThreshold() uint32 {
	return n.syncThreshold
}

func (n testNodeConfig) NodeIsSyncingEnabled() bool {
	return n.nodeIsSyncingEnabled
}

func (n testNodeConfig) FinalizedBlockPollInterval() time.Duration {
	return n.finalizedBlockPollInterval
}

func (n testNodeConfig) EnforceRepeatableRead() bool {
	return n.enforceRepeatableRead
}

func (n testNodeConfig) DeathDeclarationDelay() time.Duration {
	return n.deathDeclarationDelay
}

func (n testNodeConfig) VerifyChainID() bool {
	return true
}

type testNode struct {
	*node[ID, Head, RPCClient[ID, Head]]
}

type testNodeOpts struct {
	config      testNodeConfig
	chainConfig mocks.ChainConfig
	lggr        logger.Logger
	wsuri       *url.URL
	httpuri     *url.URL
	name        string
	id          int
	chainID     ID
	nodeOrder   int32
	rpc         *mockRPCClient[ID, Head]
	chainFamily string
	isRPCProxy  bool
}

func newTestNode(t *testing.T, opts testNodeOpts) testNode {
	if opts.lggr == nil {
		opts.lggr = logger.Test(t)
	}

	if opts.name == "" {
		opts.name = "tes node"
	}

	if opts.chainFamily == "" {
		opts.chainFamily = "test node chain family"
	}

	if opts.chainID == nil {
		opts.chainID = RandomID()
	}

	if opts.id == 0 {
		opts.id = 42
	}

	nodeMetrics, err := metrics.NewGenericMultiNodeMetrics("test-network", "1")
	require.NoError(t, err)

	nodeI := NewNode[ID, Head, RPCClient[ID, Head]](opts.config, opts.chainConfig, opts.lggr, nodeMetrics,
		opts.wsuri, opts.httpuri, opts.name, opts.id, opts.chainID, opts.nodeOrder, opts.rpc, opts.chainFamily, opts.isRPCProxy)

	return testNode{
		nodeI.(*node[ID, Head, RPCClient[ID, Head]]),
	}
}

func makeMockNodeMetrics(t *testing.T) *mockNodeMetrics {
	mockMetrics := newMockNodeMetrics(t)
	mockMetrics.On("IncrementNodeVerifies", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeVerifiesFailed", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeVerifiesSuccess", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToAlive", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToInSync", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToOutOfSync", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToUnreachable", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToInvalidChainID", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToUnusable", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementNodeTransitionsToSyncing", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("SetHighestSeenBlock", mock.Anything, mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("SetHighestFinalizedBlock", mock.Anything, mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementSeenBlocks", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementPolls", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementPollsFailed", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("IncrementPollsSuccess", mock.Anything, mock.Anything).Maybe()
	mockMetrics.On("RecordNodeClientVersion", mock.Anything, mock.Anything, mock.Anything).Maybe()
	return mockMetrics
}

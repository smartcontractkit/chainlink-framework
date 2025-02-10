package multinode

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/services/servicetest"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
)

type TestSendTxRPCClient SendTxRPCClient[any, any]

type sendTxMultiNode struct {
	*MultiNode[ID, TestSendTxRPCClient]
}

type sendTxRPC struct {
	sendTxRun func(args mock.Arguments)
	sendTxErr error
}

var _ TestSendTxRPCClient = (*sendTxRPC)(nil)

func newSendTxRPC(sendTxErr error, sendTxRun func(args mock.Arguments)) *sendTxRPC {
	return &sendTxRPC{sendTxErr: sendTxErr, sendTxRun: sendTxRun}
}

func (rpc *sendTxRPC) SendTransaction(ctx context.Context, _ any) (any, SendTxReturnCode, error) {
	if rpc.sendTxRun != nil {
		rpc.sendTxRun(mock.Arguments{ctx})
	}
	return nil, classifySendTxError(nil, rpc.sendTxErr), rpc.sendTxErr
}

// newTestTransactionSender returns a sendTxMultiNode and TransactionSender.
// Only the TransactionSender is run via Start/Close.
func newTestTransactionSender(t *testing.T, chainID ID, lggr logger.Logger,
	nodes []Node[ID, TestSendTxRPCClient],
	sendOnlyNodes []SendOnlyNode[ID, TestSendTxRPCClient],
) (*sendTxMultiNode, *TransactionSender[any, any, ID, TestSendTxRPCClient]) {
	mn := sendTxMultiNode{NewMultiNode[ID, TestSendTxRPCClient](
		lggr, NodeSelectionModeRoundRobin, 0, nodes, sendOnlyNodes, chainID, "chainFamily", 0)}

	txSender := NewTransactionSender[any, any, ID, TestSendTxRPCClient](lggr, chainID, mn.chainFamily, mn.MultiNode, func(err error) SendTxReturnCode { return 0 }, tests.TestInterval)
	servicetest.Run(t, txSender)
	return &mn, txSender
}

func classifySendTxError(_ any, err error) SendTxReturnCode {
	if err != nil {
		return Fatal
	}
	return Successful
}

func TestTransactionSender_SendTransaction(t *testing.T) {
	t.Parallel()

	newNodeWithState := func(t *testing.T, state nodeState, txErr error, sendTxRun func(args mock.Arguments)) *mockNode[ID, TestSendTxRPCClient] {
		rpc := newSendTxRPC(txErr, sendTxRun)
		node := newMockNode[ID, TestSendTxRPCClient](t)
		node.On("String").Return("node name").Maybe()
		node.On("RPC").Return(rpc).Maybe()
		node.On("State").Return(state).Maybe()
		node.On("Start", mock.Anything).Return(nil).Maybe()
		node.On("Close").Return(nil).Maybe()
		node.On("SetPoolChainInfoProvider", mock.Anything).Return(nil).Maybe()
		return node
	}

	newNode := func(t *testing.T, txErr error, sendTxRun func(args mock.Arguments)) *mockNode[ID, TestSendTxRPCClient] {
		return newNodeWithState(t, nodeStateAlive, txErr, sendTxRun)
	}

	t.Run("Fails if there is no nodes available", func(t *testing.T) {
		lggr := logger.Test(t)
		_, txSender := newTestTransactionSender(t, RandomID(), lggr, nil, nil)
		_, _, err := txSender.SendTransaction(tests.Context(t), nil)
		assert.EqualError(t, err, ErrNodeError.Error())
	})

	t.Run("Transaction failure happy path", func(t *testing.T) {
		expectedError := errors.New("transaction failed")
		mainNode := newNode(t, expectedError, nil)
		lggr, observedLogs := logger.TestObserved(t, zap.DebugLevel)

		_, txSender := newTestTransactionSender(t, RandomID(), lggr,
			[]Node[ID, TestSendTxRPCClient]{mainNode},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{newNode(t, errors.New("unexpected error"), nil)})

		_, code, err := txSender.SendTransaction(tests.Context(t), nil)
		require.ErrorIs(t, err, expectedError)
		require.Equal(t, Fatal, code)
		tests.AssertLogCountEventually(t, observedLogs, "Node sent transaction", 2)
		tests.AssertLogCountEventually(t, observedLogs, "RPC returned error", 2)
	})

	t.Run("Transaction success happy path", func(t *testing.T) {
		mainNode := newNode(t, nil, nil)

		lggr, observedLogs := logger.TestObserved(t, zap.DebugLevel)
		_, txSender := newTestTransactionSender(t, RandomID(), lggr,
			[]Node[ID, TestSendTxRPCClient]{mainNode},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{newNode(t, errors.New("unexpected error"), nil)})

		_, code, err := txSender.SendTransaction(tests.Context(t), nil)
		require.NoError(t, err)
		require.Equal(t, Successful, code)
		tests.AssertLogCountEventually(t, observedLogs, "Node sent transaction", 2)
		tests.AssertLogCountEventually(t, observedLogs, "RPC returned error", 1)
	})

	t.Run("Context expired before collecting sufficient results", func(t *testing.T) {
		testContext, testCancel := context.WithCancel(tests.Context(t))
		defer testCancel()

		mainNode := newNode(t, nil, func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})

		lggr := logger.Test(t)

		_, txSender := newTestTransactionSender(t, RandomID(), lggr,
			[]Node[ID, TestSendTxRPCClient]{mainNode}, nil)

		requestContext, cancel := context.WithCancel(tests.Context(t))
		cancel()
		_, _, err := txSender.SendTransaction(requestContext, nil)
		require.EqualError(t, err, "context canceled")
	})

	t.Run("Context cancelled while sending results does not cause invariant violation", func(t *testing.T) {
		requestContext, cancel := context.WithCancel(tests.Context(t))
		mainNode := newNode(t, nil, func(_ mock.Arguments) {
			cancel()
		})

		lggr, observedLogs := logger.TestObserved(t, zap.WarnLevel)

		_, txSender := newTestTransactionSender(t, RandomID(), lggr,
			[]Node[ID, TestSendTxRPCClient]{mainNode}, nil)

		_, _, err := txSender.SendTransaction(requestContext, nil)
		require.EqualError(t, err, "context canceled")
		require.Empty(t, observedLogs.FilterMessage("observed invariant violation on SendTransaction").Len())
	})

	t.Run("Soft timeout stops results collection", func(t *testing.T) {
		chainID := RandomID()
		expectedError := errors.New("transaction failed")
		fastNode := newNode(t, expectedError, nil)

		// hold reply from the node till end of the test
		testContext, testCancel := context.WithCancel(tests.Context(t))
		defer testCancel()
		slowNode := newNode(t, errors.New("transaction failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})

		lggr := logger.Test(t)

		_, txSender := newTestTransactionSender(t, chainID, lggr, []Node[ID, TestSendTxRPCClient]{fastNode, slowNode}, nil)
		_, _, err := txSender.SendTransaction(tests.Context(t), nil)
		require.EqualError(t, err, expectedError.Error())
	})
	t.Run("Returns success without waiting for the rest of the nodes", func(t *testing.T) {
		chainID := RandomID()
		fastNode := newNode(t, nil, nil)
		// hold reply from the node till end of the test
		testContext, testCancel := context.WithCancel(tests.Context(t))
		defer testCancel()
		slowNode := newNode(t, errors.New("transaction failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})
		slowSendOnly := newNode(t, errors.New("send only failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})
		lggr, _ := logger.TestObserved(t, zap.WarnLevel)
		_, txSender := newTestTransactionSender(t, chainID, lggr,
			[]Node[ID, TestSendTxRPCClient]{fastNode, slowNode},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{slowSendOnly})

		_, code, err := txSender.SendTransaction(tests.Context(t), nil)
		require.NoError(t, err)
		require.Equal(t, Successful, code)
	})
	t.Run("Fails when multinode is closed", func(t *testing.T) {
		chainID := RandomID()
		fastNode := newNode(t, nil, nil)
		fastNode.On("ConfiguredChainID").Return(chainID).Maybe()
		// hold reply from the node till end of the test
		testContext, testCancel := context.WithCancel(tests.Context(t))
		defer testCancel()
		slowNode := newNode(t, errors.New("transaction failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})
		slowNode.On("ConfiguredChainID").Return(chainID).Maybe()
		slowSendOnly := newNode(t, errors.New("send only failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})
		slowSendOnly.On("ConfiguredChainID").Return(chainID).Maybe()

		lggr, _ := logger.TestObserved(t, zap.DebugLevel)

		mn, txSender := newTestTransactionSender(t, chainID, lggr,
			[]Node[ID, TestSendTxRPCClient]{fastNode, slowNode},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{slowSendOnly})

		require.NoError(t, mn.Start(tests.Context(t)))
		require.NoError(t, mn.Close())
		_, _, err := txSender.SendTransaction(tests.Context(t), nil)
		require.EqualError(t, err, "service is stopped")
	})
	t.Run("Fails when closed", func(t *testing.T) {
		chainID := RandomID()
		fastNode := newNode(t, nil, nil)
		// hold reply from the node till end of the test
		testContext, testCancel := context.WithCancel(tests.Context(t))
		defer testCancel()
		slowNode := newNode(t, errors.New("transaction failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})
		slowSendOnly := newNode(t, errors.New("send only failed"), func(_ mock.Arguments) {
			// block caller til end of the test
			<-testContext.Done()
		})

		var txSender *TransactionSender[any, any, ID, TestSendTxRPCClient]

		t.Cleanup(func() { // after txSender.Close()
			_, _, err := txSender.SendTransaction(tests.Context(t), nil)
			assert.EqualError(t, err, "TransactionSender not started")
		})

		_, txSender = newTestTransactionSender(t, chainID, logger.Test(t),
			[]Node[ID, TestSendTxRPCClient]{fastNode, slowNode},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{slowSendOnly})
	})
	t.Run("Returns error if there is no healthy primary nodes", func(t *testing.T) {
		chainID := RandomID()
		primary := newNodeWithState(t, nodeStateUnreachable, nil, nil)
		sendOnly := newNodeWithState(t, nodeStateUnreachable, nil, nil)

		lggr := logger.Test(t)

		_, txSender := newTestTransactionSender(t, chainID, lggr,
			[]Node[ID, TestSendTxRPCClient]{primary},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{sendOnly})

		_, _, err := txSender.SendTransaction(tests.Context(t), nil)
		assert.EqualError(t, err, ErrNodeError.Error())
	})

	t.Run("Transaction success even if one of the nodes is unhealthy", func(t *testing.T) {
		chainID := RandomID()
		mainNode := newNode(t, nil, nil)
		unexpectedCall := func(args mock.Arguments) {
			panic("SendTx must not be called for unhealthy node")
		}
		unhealthyNode := newNodeWithState(t, nodeStateUnreachable, nil, unexpectedCall)
		unhealthySendOnlyNode := newNodeWithState(t, nodeStateUnreachable, nil, unexpectedCall)

		lggr := logger.Test(t)

		_, txSender := newTestTransactionSender(t, chainID, lggr,
			[]Node[ID, TestSendTxRPCClient]{mainNode, unhealthyNode},
			[]SendOnlyNode[ID, TestSendTxRPCClient]{unhealthySendOnlyNode})

		_, code, err := txSender.SendTransaction(tests.Context(t), nil)
		require.NoError(t, err)
		require.Equal(t, Successful, code)
	})
	t.Run("All background jobs stop even if RPC returns result after soft timeout", func(t *testing.T) {
		chainID := RandomID()
		expectedError := errors.New("transaction failed")
		fastNode := newNode(t, expectedError, nil)

		// hold reply from the node till SendTransaction returns result
		sendTxContext, sendTxCancel := context.WithCancel(tests.Context(t))
		slowNode := newNode(t, errors.New("transaction failed"), func(_ mock.Arguments) {
			<-sendTxContext.Done()
		})

		lggr := logger.Test(t)

		_, txSender := newTestTransactionSender(t, chainID, lggr, []Node[ID, TestSendTxRPCClient]{fastNode, slowNode}, nil)
		_, _, err := txSender.SendTransaction(sendTxContext, nil)
		sendTxCancel()
		require.EqualError(t, err, expectedError.Error())
		// TxSender should stop all background go routines after SendTransaction is done and before test is done.
		// Otherwise, it signals that we have a goroutine leak.
		txSender.wg.Wait()
	})
}

func TestTransactionSender_SendTransaction_aggregateTxResults(t *testing.T) {
	t.Parallel()
	// ensure failure on new SendTxReturnCode
	codesToCover := map[SendTxReturnCode]struct{}{}
	for code := Successful; code < sendTxReturnCodeLen; code++ {
		codesToCover[code] = struct{}{}
	}

	testCases := []struct {
		Name                string
		ExpectedTxResult    string
		ExpectedCriticalErr string
		ResultsByCode       sendTxResults[any]
	}{
		{
			Name:                "Returns success and logs critical error on success and Fatal",
			ExpectedTxResult:    "success",
			ExpectedCriticalErr: "found contradictions in nodes replies on SendTransaction: got success and severe error",
			ResultsByCode: sendTxResults[any]{
				Successful: {newSendTxResult(errors.New("success"))},
				Fatal:      {newSendTxResult(errors.New("fatal"))},
			},
		},
		{
			Name:                "Returns TransactionAlreadyKnown and logs critical error on TransactionAlreadyKnown and Fatal",
			ExpectedTxResult:    "tx_already_known",
			ExpectedCriticalErr: "found contradictions in nodes replies on SendTransaction: got success and severe error",
			ResultsByCode: sendTxResults[any]{
				TransactionAlreadyKnown: {newSendTxResult(errors.New("tx_already_known"))},
				Unsupported:             {newSendTxResult(errors.New("unsupported"))},
			},
		},
		{
			Name:                "Prefers sever error to temporary",
			ExpectedTxResult:    "underpriced",
			ExpectedCriticalErr: "",
			ResultsByCode: sendTxResults[any]{
				Retryable:   {newSendTxResult(errors.New("retryable"))},
				Underpriced: {newSendTxResult(errors.New("underpriced"))},
			},
		},
		{
			Name:                "Returns temporary error",
			ExpectedTxResult:    "retryable",
			ExpectedCriticalErr: "",
			ResultsByCode: sendTxResults[any]{
				Retryable: {newSendTxResult(errors.New("retryable"))},
			},
		},
		{
			Name:                "Insufficient funds is treated as  error",
			ExpectedTxResult:    "insufficientFunds",
			ExpectedCriticalErr: "",
			ResultsByCode: sendTxResults[any]{
				InsufficientFunds: {newSendTxResult(errors.New("insufficientFunds"))},
			},
		},
		{
			Name:                "Logs critical error on empty ResultsByCode",
			ExpectedCriticalErr: "expected at least one response on SendTransaction",
			ResultsByCode:       sendTxResults[any]{},
		},
		{
			Name:                "Zk terminally stuck",
			ExpectedTxResult:    "not enough keccak counters to continue the execution",
			ExpectedCriticalErr: "",
			ResultsByCode: sendTxResults[any]{
				TerminallyStuck: {newSendTxResult(errors.New("not enough keccak counters to continue the execution"))},
			},
		},
	}

	for _, testCase := range testCases {
		for code := range testCase.ResultsByCode {
			delete(codesToCover, code)
		}

		t.Run(testCase.Name, func(t *testing.T) {
			txResult, err := aggregateTxResults(testCase.ResultsByCode)
			if testCase.ExpectedTxResult != "" {
				require.EqualError(t, txResult.error, testCase.ExpectedTxResult)
			}

			logger.Sugared(logger.Test(t)).Info("Map: " + fmt.Sprint(testCase.ResultsByCode))
			logger.Sugared(logger.Test(t)).Criticalw("observed invariant violation on SendTransaction", "resultsByCode", testCase.ResultsByCode, "err", err)

			if testCase.ExpectedCriticalErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, testCase.ExpectedCriticalErr)
			}
		})
	}

	// explicitly signal that following codes are properly handled in aggregateTxResults,
	// but dedicated test cases won't be beneficial
	for _, codeToIgnore := range []SendTxReturnCode{Unknown, ExceedsMaxFee, FeeOutOfValidRange} {
		delete(codesToCover, codeToIgnore)
	}
	assert.Empty(t, codesToCover, "all of the SendTxReturnCode must be covered by this test")
}

func newSendTxResult(err error) sendTxResult[any] {
	return sendTxResult[any]{error: err}
}

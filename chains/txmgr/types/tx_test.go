package types

import (
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/sqlutil"
)

func TestTxAttemptState(t *testing.T) {
	type stateCompare struct {
		state TxAttemptState
		str   string
	}

	// dynmaically build base states
	states := []stateCompare{}
	for i, v := range txAttemptStateStrings {
		states = append(states, stateCompare{TxAttemptState(i), v})
	}

	t.Run("NewTxAttemptState", func(t *testing.T) {
		// string representation
		addStates := []stateCompare{
			{TxAttemptState(0), "invalid_state"},
		}
		allStates := append(states, addStates...)
		for i := range allStates {
			s := allStates[i]
			t.Run(fmt.Sprintf("%s->%d", s.str, s.state), func(t *testing.T) {
				assert.Equal(t, s.state, NewTxAttemptState(s.str))
			})
		}
	})

	t.Run("String", func(t *testing.T) {
		// string representation
		addStates := []stateCompare{
			{txAttemptStateCount, txAttemptStateStrings[0]},
			{100, txAttemptStateStrings[0]},
		}
		allStates := append(states, addStates...)
		for i := range allStates {
			s := allStates[i]
			t.Run(fmt.Sprintf("%d->%s", s.state, s.str), func(t *testing.T) {
				assert.Equal(t, s.str, s.state.String())
			})
		}
	})
}

func TestTxLogging(t *testing.T) {
	tx := Tx[*big.Int, *big.Int, *big.Int, *big.Int, *big.Int, *big.Int]{
		ID: 1,
		TxAttempts: []TxAttempt[*big.Int, *big.Int, *big.Int, *big.Int, *big.Int, *big.Int]{
			{
				ID: 2,
			},
		},
	}

	attempt := &tx.TxAttempts[0]
	// set recursive reference
	attempt.Tx = tx
	// ensure that in both cases we prevent fmt from stacking in a loop attempt->tx->attempt by defining String method on attempt.
	println(fmt.Sprintf("%+v", *attempt))
	println(fmt.Sprintf("%+v", attempt))
}

type testStringer string

func (s testStringer) String() string { return string(s) }
func (s testStringer) Bytes() []byte  { return []byte(s) }

type testSequence int64

func (s testSequence) String() string { return fmt.Sprint(int64(s)) }
func (s testSequence) Int64() int64   { return int64(s) }

func TestTx_GetLogger_LogsMessageIDsFromMeta(t *testing.T) {
	messageID := "0x8e64ee42d24e46ad38e2a01f79f9b57ffab5f4dfd551bf0b60cf5a2d5b45543f"
	rawMeta, err := json.Marshal(TxMeta[testStringer, testStringer]{MessageIDs: []string{messageID}})
	require.NoError(t, err)
	meta := sqlutil.JSON(rawMeta)

	tx := Tx[testStringer, testStringer, testStringer, testStringer, testSequence, testStringer]{
		ID:   1,
		Meta: &meta,
	}

	lggr, observed := logger.TestObserved(t, zapcore.WarnLevel)
	tx.GetLogger(lggr).Warnw("Transaction rejected due to insufficient funds in sending address, will retry")

	logs := observed.All()
	require.Len(t, logs, 1)
	assert.Equal(t, messageID, logs[0].ContextMap()["messageID"])
}

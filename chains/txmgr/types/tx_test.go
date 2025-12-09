package types

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
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

package multinode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNodeSelector(t *testing.T) {
	// rest of the tests are located in specific node selectors tests
	t.Run("panics on unknown type", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = newNodeSelector[ID, RPCClient[ID, Head]]("unknown", nil)
		})
	})
}

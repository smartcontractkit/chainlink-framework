package multinode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestContext(t *testing.T) {
	ctx := t.Context()
	assert.False(t, CtxIsHealthCheckRequest(ctx), "expected false for test context")
	ctx = CtxAddHealthCheckFlag(ctx)
	assert.True(t, CtxIsHealthCheckRequest(ctx), "expected context to contain the healthcheck flag")
}

package multinode

import "context"

type multiNodeContextKey int

const (
	contextKeyHeathCheckRequest multiNodeContextKey = iota + 1
)

func CtxAddHealthCheckFlag(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyHeathCheckRequest, struct{}{})
}

func CtxIsHealthCheckRequest(ctx context.Context) bool {
	return ctx.Value(contextKeyHeathCheckRequest) != nil
}

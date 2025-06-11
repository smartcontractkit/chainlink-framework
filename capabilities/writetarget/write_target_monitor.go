package writetarget

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	"github.com/smartcontractkit/chainlink-common/pkg/logger"

	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
)

const (
	repoCLLCommon = "https://raw.githubusercontent.com/smartcontractkit/chainlink-framework"
	// TODO: replace with main when merged
	versionRefsDevelop = "refs/heads/refactor-write-target"
	schemaBasePath     = repoCLLCommon + "/" + versionRefsDevelop + "/capabilities/capabilities/writetarget/pb"
)

func NewMonitorEmitter(lggr logger.Logger) beholder.ProtoEmitter {
	// Initialize the Beholder client with a local logger a custom Emitter
	client := beholder.GetClient().ForPackage("write_target")
	return beholder.NewProtoEmitter(lggr, &client, schemaBasePath)
}

type MonitorOpts struct {
	Lggr              logger.Logger
	Processors        map[string]beholder.ProtoProcessor
	EnabledProcessors []string
	Emitter           beholder.ProtoEmitter
}

// NewMonitor initializes a Beholder client for the Write Target
//
// The client is initialized as a BeholderClient extension with a custom ProtoEmitter.
// The ProtoEmitter is proxied with additional processing for emitted messages. This processing
// includes decoding messages as specific types and deriving metrics based on the decoded messages.
// TODO: Report decoding uses the same ABI for EVM and Aptos, however, future chains may need a different
// decoding scheme. Generalize this in the future to support different chains and decoding schemes.
func NewMonitor(opts MonitorOpts) (*beholder.BeholderClient, error) {
	client := beholder.GetClient().ForPackage("write_target")

	// Proxy ProtoEmitter with additional processing
	protoEmitterProxy := protoEmitter{
		lggr:              opts.Lggr,
		emitter:           opts.Emitter,
		processors:        opts.Processors,
		enabledProcessors: opts.EnabledProcessors,
	}
	return &beholder.BeholderClient{Client: &client, ProtoEmitter: &protoEmitterProxy}, nil
}

// ProtoEmitter proxy specific to the WT
type protoEmitter struct {
	lggr              logger.Logger
	emitter           beholder.ProtoEmitter
	processors        map[string]beholder.ProtoProcessor
	enabledProcessors []string
}

// Emit emits a proto.Message and runs additional processing
func (e *protoEmitter) Emit(ctx context.Context, m proto.Message, attrKVs ...any) error {
	err := e.emitter.Emit(ctx, m, attrKVs...)
	if err != nil {
		return fmt.Errorf("failed to emit: %w", err)
	}

	// Notice: we skip processing errors (and continue) so this will never error
	return e.Process(ctx, m, attrKVs...)
}

// EmitWithLog emits a proto.Message and runs additional processing
func (e *protoEmitter) EmitWithLog(ctx context.Context, m proto.Message, attrKVs ...any) error {
	err := e.emitter.EmitWithLog(ctx, m, attrKVs...)
	if err != nil {
		return fmt.Errorf("failed to emit with log: %w", err)
	}

	// Notice: we skip processing errors (and continue) so this will never error
	return e.Process(ctx, m, attrKVs...)
}

// Process aggregates further processing for emitted messages
func (e *protoEmitter) Process(ctx context.Context, m proto.Message, attrKVs ...any) error {
	// Further processing for emitted messages
	for _, processorName := range e.enabledProcessors {
		p, ok := e.processors[processorName]
		if !ok {
			// no processor matching configured one, log error but continue
			e.lggr.Errorf("no required processor with name %s", processorName)
			continue
		}
		err := p.Process(ctx, m, attrKVs...)
		if err != nil {
			// Notice: we swallow and log processing errors
			// These should be investigated and fixed, but are not critical to product runtime,
			// and shouldn't block further processing of the emitted message.
			e.lggr.Errorw("failed to process emitted message", "err", err)
			return nil
		}
	}
	// Product specific processing only for write confirmed
	if msg, ok := m.(*wt.WriteConfirmed); ok {
		if msg.MetaCapabilityProcessor == "" {
			e.lggr.Debugw("No product specific processor specified; skipping.")
			return nil
		}

		if p, ok := e.processors[msg.MetaCapabilityProcessor]; ok {
			if err := p.Process(ctx, msg, attrKVs...); err != nil {
				e.lggr.Errorw("failed to process emitted message", "err", err)
			}
		} else {
			// no processor matching configured one, log error
			e.lggr.Errorf("no matching processor for MetaCapabilityProcessor=%s", msg.MetaCapabilityProcessor)
		}
	}

	return nil
}

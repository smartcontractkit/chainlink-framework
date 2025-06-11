package processor

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/smartcontractkit/chainlink-common/pkg/beholder"
	wt "github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform"
	"github.com/smartcontractkit/chainlink-framework/capabilities/writetarget/monitoring/pb/platform/on-chain/forwarder"
)

// Product-agnostic processors to be injected into WriteTarget Monitor
func NewPlatformProcessors(emitter beholder.ProtoEmitter) (map[string]beholder.ProtoProcessor, error) {
	forwarderMetrics, err := forwarder.NewMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to create new forwarder metrics: %w", err)
	}

	wtMetrics, err := wt.NewMetrics()
	if err != nil {
		return nil, fmt.Errorf("failed to create new write target metrics: %w", err)
	}
	return map[string]beholder.ProtoProcessor{
		"keystone": &keystoneProcessor{
			emitter: emitter,
			metrics: forwarderMetrics,
		},
		"writetarget": &wtProcessor{
			metrics: wtMetrics,
		},
	}, nil
}

func GetDefaultPlatformProcessors() []string {
	return []string{"writetarget", "keystone"}
}

// Write-Target specific processor decodes write messages to derive metrics
type wtProcessor struct {
	metrics *wt.Metrics
}

func (p *wtProcessor) Process(ctx context.Context, m proto.Message, attrKVs ...any) error {
	// Switch on the type of the proto.Message
	switch msg := m.(type) {
	case *wt.WriteInitiated:
		err := p.metrics.OnWriteInitiated(ctx, msg, attrKVs...)
		if err != nil {
			return fmt.Errorf("failed to publish write initiated metrics: %w", err)
		}
		return nil
	case *wt.WriteError:
		err := p.metrics.OnWriteError(ctx, msg, attrKVs...)
		if err != nil {
			return fmt.Errorf("failed to publish write error metrics: %w", err)
		}
		return nil
	case *wt.WriteSent:
		err := p.metrics.OnWriteSent(ctx, msg, attrKVs...)
		if err != nil {
			return fmt.Errorf("failed to publish write sent metrics: %w", err)
		}
		return nil
	case *wt.WriteConfirmed:
		err := p.metrics.OnWriteConfirmed(ctx, msg, attrKVs...)
		if err != nil {
			return fmt.Errorf("failed to publish write confirmed metrics: %w", err)
		}
		return nil
	default:
		return nil // fallthrough
	}
}

// Keystone specific processor decodes writes as 'platform.forwarder.ReportProcessed' messages + metrics
type keystoneProcessor struct {
	emitter beholder.ProtoEmitter
	metrics *forwarder.Metrics
}

func (p *keystoneProcessor) Process(ctx context.Context, m proto.Message, attrKVs ...any) error {
	// Switch on the type of the proto.Message
	switch msg := m.(type) {
	case *wt.WriteConfirmed:
		// TODO: detect the type of write payload (support more than one type of write, first multiple Keystone report versions)
		// https://smartcontract-it.atlassian.net/browse/NONEVM-817
		// Q: Will this msg ever contain different (non-Keystone) types of writes? Hmm.
		// Notice: we assume all writes are Keystone (v1) writes for now

		// Decode as a 'platform.forwarder.ReportProcessed' message
		reportProcessed, err := forwarder.DecodeAsReportProcessed(msg)
		if err != nil {
			return fmt.Errorf("failed to decode as 'platform.forwarder.ReportProcessed': %w", err)
		}
		// Emit the 'platform.forwarder.ReportProcessed' message
		err = p.emitter.EmitWithLog(ctx, reportProcessed, attrKVs...)
		if err != nil {
			return fmt.Errorf("failed to emit with log: %w", err)
		}
		// Process emit and derive metrics
		err = p.metrics.OnReportProcessed(ctx, reportProcessed, attrKVs...)
		if err != nil {
			return fmt.Errorf("failed to publish report processed metrics: %w", err)
		}
		return nil
	default:
		return nil // fallthrough
	}
}

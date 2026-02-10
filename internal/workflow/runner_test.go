package workflow

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
)

func TestRun_WarnDoesNotStop(t *testing.T) {
	state := NewMemoryState()
	steps := []Step{
		NewReadOnlyStep("preflight", func(ctx context.Context, st State) (StepResult, error) {
			return StepResult{Findings: []Finding{{Severity: SeverityInfo, Message: "preflight ok"}}}, nil
		}),
		NewReadOnlyStep("validate", func(ctx context.Context, st State) (StepResult, error) {
			return StepResult{Findings: []Finding{{Severity: SeverityWarn, Message: "minor risk"}}}, nil
		}),
	}

	runner := NewRunner(steps, state, false, log.New(io.Discard, "", 0))
	summary, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected runner error: %v", err)
	}
	if summary.Info != 1 || summary.Warn != 1 || summary.Block != 0 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if !state.IsCompleted("preflight") || !state.IsCompleted("validate") {
		t.Fatalf("expected steps to be marked completed")
	}
}

func TestRun_MutatingStepBlocked(t *testing.T) {
	state := NewMemoryState()
	steps := []Step{
		NewReadOnlyStep("preflight", func(ctx context.Context, st State) (StepResult, error) {
			return StepResult{Findings: []Finding{{Severity: SeverityInfo, Message: "ok"}}}, nil
		}),
		NewMutatingStep("upgrade_replica", func(ctx context.Context, st State) (StepResult, error) {
			return StepResult{Findings: []Finding{{Severity: SeverityInfo, Message: "upgraded"}}}, nil
		}),
	}

	runner := NewRunner(steps, state, false, log.New(io.Discard, "", 0))
	summary, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected runner error: %v", err)
	}
	if summary.Block != 1 {
		t.Fatalf("expected 1 BLOCK when mutating step is blocked, got %+v", summary)
	}
	// preflight should have completed, mutating step should not
	if !state.IsCompleted("preflight") {
		t.Fatalf("preflight should be completed")
	}
	if state.IsCompleted("upgrade_replica") {
		t.Fatalf("mutating step should not be marked completed when blocked")
	}
	res := runner.Results()["upgrade_replica"]
	if len(res.Findings) == 0 || res.Findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK finding for blocked mutating step, got %+v", res)
	}
}

type badStep struct{}

func (b *badStep) Name() string                                          { return "bad" }
func (b *badStep) Run(ctx context.Context, st State) (StepResult, error) { return StepResult{}, nil }
func (b *badStep) Idempotent() bool                                      { return false }
func (b *badStep) Mutates() bool                                         { return false }

func TestRun_NonIdempotentRejected(t *testing.T) {
	steps := []Step{&badStep{}}
	runner := NewRunner(steps, nil, false, nil)
	_, err := runner.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not idempotent") {
		t.Fatalf("expected non-idempotent validation error, got: %v", err)
	}
}

func TestRun_SkipsCompletedSteps(t *testing.T) {
	state := NewMemoryState()
	state.MarkCompleted("preflight")
	steps := []Step{
		NewReadOnlyStep("preflight", func(ctx context.Context, st State) (StepResult, error) {
			return StepResult{Findings: []Finding{{Severity: SeverityInfo, Message: "should be skipped"}}}, nil
		}),
		NewReadOnlyStep("validate", func(ctx context.Context, st State) (StepResult, error) {
			return StepResult{Findings: []Finding{{Severity: SeverityInfo, Message: "ran"}}}, nil
		}),
	}

	runner := NewRunner(steps, state, false, log.New(io.Discard, "", 0))
	summary, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected runner error: %v", err)
	}
	if summary.Info != 1 || summary.Block != 0 {
		t.Fatalf("unexpected summary after skipping: %+v", summary)
	}
	if !state.IsCompleted("preflight") || !state.IsCompleted("validate") {
		t.Fatalf("expected both steps to be completed (skipped and run)")
	}
}

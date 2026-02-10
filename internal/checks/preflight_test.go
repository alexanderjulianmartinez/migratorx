package checks

import (
	"context"
	"strings"
	"testing"
)

type mutatingCheck struct{}

func (m *mutatingCheck) Name() string { return "mutating" }
func (m *mutatingCheck) Run(ctx context.Context, input Input) ([]Finding, error) {
	return []Finding{{Severity: SeverityInfo, Message: "should not run"}}, nil
}
func (m *mutatingCheck) ReadOnly() bool { return false }

func TestRunner_EnforcesReadOnly(t *testing.T) {
	runner := NewRunner([]PreflightCheck{&mutatingCheck{}}, nil)
	_, _, err := runner.Run(context.Background(), Input{})
	if err == nil {
		t.Fatalf("expected error for non-read-only check")
	}
}

func TestRunner_AggregatesSeverities(t *testing.T) {
	checks := []PreflightCheck{
		NewReadOnlyCheck("info", func(ctx context.Context, input Input) ([]Finding, error) {
			return []Finding{{Severity: SeverityInfo, Message: "ok"}}, nil
		}),
		NewReadOnlyCheck("warn", func(ctx context.Context, input Input) ([]Finding, error) {
			return []Finding{{Severity: SeverityWarn, Message: "risk"}}, nil
		}),
		NewReadOnlyCheck("block", func(ctx context.Context, input Input) ([]Finding, error) {
			return []Finding{{Severity: SeverityBlock, Message: "stop"}}, nil
		}),
	}

	runner := NewRunner(checks, nil)
	summary, results, err := runner.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Info != 1 || summary.Warn != 1 || summary.Block != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestRunner_ErrorBecomesBlock(t *testing.T) {
	checks := []PreflightCheck{
		NewReadOnlyCheck("fails", func(ctx context.Context, input Input) ([]Finding, error) {
			return nil, context.Canceled
		}),
	}

	runner := NewRunner(checks, nil)
	summary, results, err := runner.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Block != 1 {
		t.Fatalf("expected BLOCK for check error, got %+v", summary)
	}
	if len(results) != 1 || len(results[0].Findings) != 1 {
		t.Fatalf("expected single BLOCK finding")
	}
	if results[0].Findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK severity")
	}
}

func TestRunner_RequiresMessage(t *testing.T) {
	checks := []PreflightCheck{
		NewReadOnlyCheck("missing-message", func(ctx context.Context, input Input) ([]Finding, error) {
			return []Finding{{Severity: SeverityWarn, Message: ""}}, nil
		}),
	}

	runner := NewRunner(checks, nil)
	_, results, err := runner.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || len(results[0].Findings) != 1 {
		t.Fatalf("expected a single enforced finding")
	}
	if results[0].Findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK when message is missing")
	}
	if !strings.Contains(results[0].Findings[0].Message, "without a message") {
		t.Fatalf("unexpected message: %q", results[0].Findings[0].Message)
	}
}
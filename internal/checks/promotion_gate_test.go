package checks

import (
	"context"
	"testing"
)

func TestPromotionGate_RequiresConfirmation(t *testing.T) {
	gate := &PromotionGate{ConfirmationPhrase: "PROMOTE", Checks: []PreflightCheck{}}
	_, findings, err := gate.Run(context.Background(), Input{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 || findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK when confirmation missing")
	}
}

func TestPromotionGate_MissingRequiredChecksBlocks(t *testing.T) {
	gate := &PromotionGate{ConfirmationPhrase: "PROMOTE", Checks: []PreflightCheck{}}
	_, findings, err := gate.Run(context.Background(), Input{}, "PROMOTE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) == 0 || findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK for missing required checks")
	}
}

func TestPromotionGate_WarnBlocksPromotion(t *testing.T) {
	cdc := NewReadOnlyCheck("cdc_debezium_health", func(ctx context.Context, input Input) ([]Finding, error) {
		return []Finding{{Severity: SeverityWarn, Message: "cdc lag"}}, nil
	})
	schema := NewReadOnlyCheck("schema_parity", func(ctx context.Context, input Input) ([]Finding, error) {
		return []Finding{{Severity: SeverityInfo, Message: "ok"}}, nil
	})

	gate := &PromotionGate{ConfirmationPhrase: "PROMOTE", Checks: []PreflightCheck{cdc, schema}}
	summary, findings, err := gate.Run(context.Background(), Input{}, "PROMOTE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Block == 0 {
		t.Fatalf("expected promotion BLOCK due to WARN")
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings")
	}
}

func TestPromotionGate_InfoAllowsPromotion(t *testing.T) {
	cdc := NewReadOnlyCheck("cdc_debezium_health", func(ctx context.Context, input Input) ([]Finding, error) {
		return []Finding{{Severity: SeverityInfo, Message: "cdc ok"}}, nil
	})
	schema := NewReadOnlyCheck("schema_parity", func(ctx context.Context, input Input) ([]Finding, error) {
		return []Finding{{Severity: SeverityInfo, Message: "schema ok"}}, nil
	})

	gate := &PromotionGate{ConfirmationPhrase: "PROMOTE", Checks: []PreflightCheck{cdc, schema}}
	summary, _, err := gate.Run(context.Background(), Input{}, "PROMOTE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Block != 0 {
		t.Fatalf("expected no BLOCK for INFO-only findings")
	}
}
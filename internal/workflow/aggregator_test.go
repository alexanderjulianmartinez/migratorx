package workflow

import "testing"

func TestResultAggregatorCountsAndBlocks(t *testing.T) {
	agg := &ResultAggregator{}
	if !agg.AddFindings([]Finding{{Severity: SeverityInfo}, {Severity: SeverityWarn}}) {
		t.Fatalf("expected progression to continue without BLOCK")
	}
	if agg.Blocked() {
		t.Fatalf("should not be blocked yet")
	}
	if !agg.AddFindings([]Finding{{Severity: SeverityBlock}}) {
		// expected to be blocked after adding BLOCK
	} else {
		t.Fatalf("expected progression to stop after BLOCK")
	}
	if !agg.Blocked() {
		t.Fatalf("expected blocked after BLOCK finding")
	}
	summary := agg.Summary()
	if summary.Info != 1 || summary.Warn != 1 || summary.Block != 1 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestResultAggregatorSummaryString(t *testing.T) {
	agg := &ResultAggregator{}
	agg.AddFindings([]Finding{{Severity: SeverityInfo}, {Severity: SeverityWarn}, {Severity: SeverityBlock}})
	if agg.SummaryString() != "Summary: 1 INFO / 1 WARN / 1 BLOCK" {
		t.Fatalf("unexpected summary string: %s", agg.SummaryString())
	}
}
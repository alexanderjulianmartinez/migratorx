package workflow

import "fmt"

// ResultAggregator collects findings and determines whether progression is allowed.
type ResultAggregator struct {
	summary Summary
	blocked bool
}

// AddFindings records findings and returns whether progression should continue.
// Once a BLOCK is observed, progression is prevented.
func (a *ResultAggregator) AddFindings(findings []Finding) bool {
	for _, f := range findings {
		switch f.Severity {
		case SeverityInfo:
			a.summary.Info++
		case SeverityWarn:
			a.summary.Warn++
		case SeverityBlock:
			a.summary.Block++
			a.blocked = true
		}
	}
	return !a.blocked
}

// Summary returns the aggregated counts.
func (a *ResultAggregator) Summary() Summary {
	return a.summary
}

// Blocked indicates whether a BLOCK has been observed.
func (a *ResultAggregator) Blocked() bool {
	return a.blocked
}

// SummaryString emits a human-readable summary line.
func (a *ResultAggregator) SummaryString() string {
	return fmt.Sprintf("Summary: %d INFO / %d WARN / %d BLOCK", a.summary.Info, a.summary.Warn, a.summary.Block)
}

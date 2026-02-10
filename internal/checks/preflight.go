package checks

import (
	"context"
	"fmt"
	"log"
	"strings"

	"migratorx/internal/workflow"
)

// Severity indicates the importance of a preflight finding.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityBlock
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityBlock:
		return "BLOCK"
	default:
		return "UNKNOWN"
	}
}

// Finding is a single preflight result.
type Finding struct {
	Severity Severity
	Message  string
	Meta     map[string]interface{}
}

// Summary aggregates counts of findings by severity.
type Summary struct {
	Info  int
	Warn  int
	Block int
}

// Input captures contextual data for checks. Extend as needed.
type Input struct {
	Plan *workflow.MigrationPlan
}

// PreflightCheck is a read-only validation that emits findings.
type PreflightCheck interface {
	Name() string
	Run(ctx context.Context, input Input) ([]Finding, error)
	ReadOnly() bool
}

// Runner executes preflight checks and aggregates findings.
// It enforces read-only checks and validates that all findings have messages.
type Runner struct {
	Checks []PreflightCheck
	Logger *log.Logger
}

// Result captures findings for a single check.
type Result struct {
	CheckName string
	Findings  []Finding
}

// NewRunner constructs a preflight Runner.
func NewRunner(checks []PreflightCheck, logger *log.Logger) *Runner {
	if logger == nil {
		logger = log.Default()
	}
	return &Runner{Checks: checks, Logger: logger}
}

// Run executes all checks sequentially and returns a summary and per-check results.
// Any check error is translated into a BLOCK finding with a clear message.
func (r *Runner) Run(ctx context.Context, input Input) (Summary, []Result, error) {
	var summary Summary
	results := make([]Result, 0, len(r.Checks))

	for _, check := range r.Checks {
		if !check.ReadOnly() {
			return Summary{}, nil, fmt.Errorf("preflight check %q is not read-only", check.Name())
		}

		r.Logger.Printf("running preflight check: %s", check.Name())
		findings, err := check.Run(ctx, input)
		if err != nil {
			findings = append(findings, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("check error: %v", err),
				Meta:     map[string]interface{}{"check": check.Name()},
			})
		}

		findings = enforceMessages(check.Name(), findings)
		applySummary(&summary, findings)

		results = append(results, Result{CheckName: check.Name(), Findings: findings})
	}

	return summary, results, nil
}

func applySummary(summary *Summary, findings []Finding) {
	for _, f := range findings {
		switch f.Severity {
		case SeverityInfo:
			summary.Info++
		case SeverityWarn:
			summary.Warn++
		case SeverityBlock:
			summary.Block++
		}
	}
}

func enforceMessages(checkName string, findings []Finding) []Finding {
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if strings.TrimSpace(f.Message) == "" {
			out = append(out, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("check %q emitted a finding without a message", checkName),
				Meta:     map[string]interface{}{"check": checkName},
			})
			continue
		}
		out = append(out, f)
	}
	return out
}

// ReadOnlyCheck is a helper for building read-only checks.
type ReadOnlyCheck struct {
	name  string
	runFn func(ctx context.Context, input Input) ([]Finding, error)
}

func NewReadOnlyCheck(name string, runFn func(ctx context.Context, input Input) ([]Finding, error)) *ReadOnlyCheck {
	return &ReadOnlyCheck{name: name, runFn: runFn}
}

func (c *ReadOnlyCheck) Name() string { return c.name }
func (c *ReadOnlyCheck) Run(ctx context.Context, input Input) ([]Finding, error) {
	return c.runFn(ctx, input)
}
func (c *ReadOnlyCheck) ReadOnly() bool { return true }

package workflow

import (
	"context"
	"fmt"
	"log"
	"strings"

	"migratorx/internal/checks"
)

// PromotionGate enforces explicit confirmation and re-validates CDC/schema checks.
type PromotionGate struct {
	Checks             []checks.PreflightCheck
	RequiredCheckNames []string
	ConfirmationPhrase string
	Logger             *log.Logger
}

// Run validates confirmation, re-runs checks, and blocks on WARN/BLOCK.
func (g *PromotionGate) Run(ctx context.Context, input checks.Input, confirmation string) (checks.Summary, []checks.Finding, error) {
	if g.Logger == nil {
		g.Logger = log.Default()
	}
	if strings.TrimSpace(g.ConfirmationPhrase) == "" {
		return checks.Summary{}, nil, fmt.Errorf("confirmation phrase is required")
	}
	if confirmation != g.ConfirmationPhrase {
		block := checks.Finding{
			Severity: checks.SeverityBlock,
			Message:  "promotion requires explicit confirmation",
			Meta:     map[string]interface{}{"required": g.ConfirmationPhrase},
		}
		return checks.Summary{Block: 1}, []checks.Finding{block}, nil
	}

	required := g.RequiredCheckNames
	if len(required) == 0 {
		required = []string{"cdc_debezium_health", "schema_parity"}
	}

	missing := missingChecks(required, g.Checks)
	if len(missing) > 0 {
		block := checks.Finding{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("promotion requires checks: %s", strings.Join(missing, ", ")),
			Meta:     map[string]interface{}{"missing": missing},
		}
		return checks.Summary{Block: 1}, []checks.Finding{block}, nil
	}

	runner := checks.NewRunner(g.Checks, g.Logger)
	summary, results, err := runner.Run(ctx, input)
	if err != nil {
		return checks.Summary{}, nil, err
	}

	findings := flattenResults(results)
	if summary.Warn > 0 || summary.Block > 0 {
		block := checks.Finding{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("promotion blocked due to WARN/BLOCK findings (WARN=%d, BLOCK=%d)", summary.Warn, summary.Block),
			Meta:     map[string]interface{}{"warn": summary.Warn, "block": summary.Block},
		}
		findings = append(findings, block)
		summary.Block++
	}

	return summary, findings, nil
}

func missingChecks(required []string, checksList []checks.PreflightCheck) []string {
	seen := map[string]struct{}{}
	for _, c := range checksList {
		seen[c.Name()] = struct{}{}
	}
	missing := []string{}
	for _, name := range required {
		if _, ok := seen[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func flattenResults(results []checks.Result) []checks.Finding {
	findings := []checks.Finding{}
	for _, r := range results {
		for _, f := range r.Findings {
			if f.Meta == nil {
				f.Meta = map[string]interface{}{}
			}
			if _, ok := f.Meta["check"]; !ok {
				f.Meta["check"] = r.CheckName
			}
			findings = append(findings, f)
		}
	}
	return findings
}

package main

import (
	"context"
	"log"
	"time"

	"migratorx/internal/workflow"
)

func main() {
	logger := log.Default()
	state := workflow.NewMemoryState()

	steps := []workflow.Step{
		workflow.NewReadOnlyStep("preflight", func(ctx context.Context, st workflow.State) (workflow.StepResult, error) {
			return workflow.StepResult{Findings: []workflow.Finding{{Severity: workflow.SeverityInfo, Message: "preflight ok"}}}, nil
		}),
		workflow.NewReadOnlyStep("validate_replica", func(ctx context.Context, st workflow.State) (workflow.StepResult, error) {
			return workflow.StepResult{Findings: []workflow.Finding{{Severity: workflow.SeverityWarn, Message: "replica lag observed"}}}, nil
		}),
		workflow.NewMutatingStep("promote", func(ctx context.Context, st workflow.State) (workflow.StepResult, error) {
			// This step would perform a promotion in a real implementation.
			// Runner.AllowMutations=false will block this from running by default.
			return workflow.StepResult{Findings: []workflow.Finding{{Severity: workflow.SeverityInfo, Message: "promoted replica (simulated)"}}}, nil
		}),
	}

	runner := workflow.NewRunner(steps, state, false, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	summary, err := runner.Run(ctx)
	if err != nil {
		logger.Fatalf("runner error: %v", err)
	}
	logger.Printf("summary: INFO=%d WARN=%d BLOCK=%d", summary.Info, summary.Warn, summary.Block)
}

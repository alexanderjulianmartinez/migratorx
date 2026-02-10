package workflow

import (
	"context"
	"fmt"
	"log"
	"sync"
)

// Severity indicates the importance of a finding produced by a Step.
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

// Finding is a single observed result from executing a step.
type Finding struct {
	Severity Severity
	Message  string
	Meta     map[string]interface{}
}

// StepResult contains findings produced by a Step.
type StepResult struct {
	Findings []Finding
}

// State is a minimal interface for tracking checkpoints and sharing lightweight data
// between steps. Implementations may persist/checkpoint externally; default below
// is in-memory for examples and tests.
type State interface {
	Get(key string) (interface{}, bool)
	Set(key string, value interface{})
	MarkCompleted(stepName string)
	IsCompleted(stepName string) bool
}

// Step is a single, idempotent unit of work in a migration plan.
// Rules enforced by the Runner:
//   - Steps must be idempotent (Idempotent() == true) or the Runner will refuse to run.
//   - Steps that report Mutates() == true are considered potentially dangerous: the
//     Runner will only execute them if configured with AllowMutations=true.
//   - Steps produce Findings which the Runner aggregates and converts to the
//     severity model; any BLOCK finding stops further execution.
type Step interface {
	Name() string
	Run(ctx context.Context, st State) (StepResult, error)
	Idempotent() bool
	Mutates() bool
}

// MemoryState is a simple in-memory State useful for local runs and tests.
type MemoryState struct {
	mu        sync.RWMutex
	values    map[string]interface{}
	completed map[string]struct{}
}

func NewMemoryState() *MemoryState {
	return &MemoryState{
		values:    make(map[string]interface{}),
		completed: make(map[string]struct{}),
	}
}

func (m *MemoryState) Get(key string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.values[key]
	return v, ok
}

func (m *MemoryState) Set(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
}

func (m *MemoryState) MarkCompleted(stepName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completed[stepName] = struct{}{}
}

func (m *MemoryState) IsCompleted(stepName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.completed[stepName]
	return ok
}

// Runner executes an ordered list of Steps sequentially.
// Behavior:
//   - Validates all steps are idempotent before running.
//   - Skips steps already marked completed in State.
//   - Enforces AllowMutations: if false and a step reports Mutates()==true,
//     the Runner records a BLOCK finding and halts.
//   - Aggregates findings. Any BLOCK finding halts further steps.
//   - WARN findings are recorded but do not stop the run.
//   - INFO findings are recorded.
type Runner struct {
	Steps          []Step
	State          State
	AllowMutations bool
	Logger         *log.Logger
	results        map[string]StepResult
}

// NewRunner constructs a Runner. If state is nil, a new in-memory state is used.
func NewRunner(steps []Step, state State, allowMutations bool, logger *log.Logger) *Runner {
	if state == nil {
		state = NewMemoryState()
	}
	if logger == nil {
		logger = log.Default()
	}
	return &Runner{Steps: steps, State: state, AllowMutations: allowMutations, Logger: logger, results: make(map[string]StepResult)}
}

// Summary aggregates counts of findings by severity.
type Summary struct {
	Info  int
	Warn  int
	Block int
}

// Run executes the plan sequentially and returns a Summary and any execution error.
// A returned non-nil error indicates an internal failure (invalid plan or runner
// configuration). Step-level failures are represented as BLOCK findings and will
// stop execution but do not surface as runner errors.
func (r *Runner) Run(ctx context.Context) (Summary, error) {
	// Validate idempotence
	for _, s := range r.Steps {
		if !s.Idempotent() {
			return Summary{}, fmt.Errorf("step %q is not idempotent; all steps must be idempotent", s.Name())
		}
	}

	summary := Summary{}

	for _, step := range r.Steps {
		select {
		case <-ctx.Done():
			r.Logger.Printf("context canceled before step %s", step.Name())
			return summary, ctx.Err()
		default:
		}

		if r.State.IsCompleted(step.Name()) {
			r.Logger.Printf("skipping completed step: %s", step.Name())
			continue
		}

		if step.Mutates() && !r.AllowMutations {
			// Record a BLOCK finding and halt — protecting against implicit mutations
			f := Finding{Severity: SeverityBlock, Message: "mutating step blocked by Runner configuration", Meta: map[string]interface{}{"step": step.Name()}}
			r.results[step.Name()] = StepResult{Findings: []Finding{f}}
			r.Logger.Printf("BLOCK: step %s mutates but Runner.AllowMutations is false", step.Name())
			summary.Block++
			return summary, nil
		}

		r.Logger.Printf("running step: %s", step.Name())
		res, err := step.Run(ctx, r.State)
		if err != nil {
			// Treat an execution error as a BLOCK: surface as finding and stop.
			f := Finding{Severity: SeverityBlock, Message: fmt.Sprintf("step error: %v", err), Meta: map[string]interface{}{"step": step.Name()}}
			res.Findings = append(res.Findings, f)
		}

		// Aggregate findings
		blocked := false
		for _, f := range res.Findings {
			switch f.Severity {
			case SeverityInfo:
				summary.Info++
			case SeverityWarn:
				summary.Warn++
			case SeverityBlock:
				summary.Block++
				blocked = true
			}
		}

		r.results[step.Name()] = res

		if blocked {
			r.Logger.Printf("BLOCK encountered in step %s; halting plan execution", step.Name())
			return summary, nil
		}

		// mark completed only if no BLOCK findings
		r.State.MarkCompleted(step.Name())
		toLog := fmt.Sprintf("completed step: %s (INFO=%d WARN=%d BLOCK=%d)", step.Name(), countSeverity(res.Findings, SeverityInfo), countSeverity(res.Findings, SeverityWarn), countSeverity(res.Findings, SeverityBlock))
		r.Logger.Println(toLog)
	}

	return summary, nil
}

func countSeverity(findings []Finding, sv Severity) int {
	c := 0
	for _, f := range findings {
		if f.Severity == sv {
			c++
		}
	}
	return c
}

// Results returns a copy of step results after a run.
func (r *Runner) Results() map[string]StepResult {
	out := make(map[string]StepResult, len(r.results))
	for k, v := range r.results {
		out[k] = v
	}
	return out
}

// --- Example step implementations ---

// ReadOnlyStep is a helper base for steps that do not mutate target systems.
type ReadOnlyStep struct {
	name  string
	runFn func(ctx context.Context, st State) (StepResult, error)
}

func NewReadOnlyStep(name string, runFn func(ctx context.Context, st State) (StepResult, error)) *ReadOnlyStep {
	return &ReadOnlyStep{name: name, runFn: runFn}
}

func (r *ReadOnlyStep) Name() string { return r.name }
func (r *ReadOnlyStep) Run(ctx context.Context, st State) (StepResult, error) {
	return r.runFn(ctx, st)
}
func (r *ReadOnlyStep) Idempotent() bool { return true }
func (r *ReadOnlyStep) Mutates() bool    { return false }

// MutatingStep is an example of a step that performs mutations. Runners
// configured with AllowMutations=false will block this step from running.
type MutatingStep struct {
	name  string
	runFn func(ctx context.Context, st State) (StepResult, error)
}

func NewMutatingStep(name string, runFn func(ctx context.Context, st State) (StepResult, error)) *MutatingStep {
	return &MutatingStep{name: name, runFn: runFn}
}

func (m *MutatingStep) Name() string { return m.name }
func (m *MutatingStep) Run(ctx context.Context, st State) (StepResult, error) {
	return m.runFn(ctx, st)
}
func (m *MutatingStep) Idempotent() bool { return true }
func (m *MutatingStep) Mutates() bool    { return true }

// Example usage removed from source to avoid nested comment issues.

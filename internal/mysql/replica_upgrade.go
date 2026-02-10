package mysql

import (
	"context"
	"fmt"
	"log"
	"strings"

	"migratorx/internal/workflow"
)

// Severity indicates the importance of a replica-upgrade finding.
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

// Finding is a structured result from replica upgrade orchestration.
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

// ReplicationStatus models minimal replication state for partial-progress detection.
type ReplicationStatus struct {
	IOThreadRunning  bool
	SQLThreadRunning bool
}

// ReplicaInspector provides read-only inspection for orchestration decisions.
type ReplicaInspector interface {
	IsPrimary(ctx context.Context, host string) (bool, error)
	ReplicationStatus(ctx context.Context, replica string) (ReplicationStatus, error)
}

// ReplicaActions performs mutating upgrade actions.
type ReplicaActions interface {
	StopReplication(ctx context.Context, replica string) error
	RunUpgrade(ctx context.Context, replica string) error
	StartReplication(ctx context.Context, replica string) error
}

// UpgradeOrchestrator coordinates a safe, idempotent replica upgrade.
type UpgradeOrchestrator struct {
	Inspector ReplicaInspector
	Actions   ReplicaActions
	State     workflow.State
	Primary   string
	Logger    *log.Logger
}

// NewUpgradeOrchestrator constructs an orchestrator with defaults.
func NewUpgradeOrchestrator(inspector ReplicaInspector, actions ReplicaActions, state workflow.State, primary string, logger *log.Logger) *UpgradeOrchestrator {
	if state == nil {
		state = workflow.NewMemoryState()
	}
	if logger == nil {
		logger = log.Default()
	}
	return &UpgradeOrchestrator{Inspector: inspector, Actions: actions, State: state, Primary: primary, Logger: logger}
}

// Run performs the replica upgrade flow, returning structured findings.
// - Never targets primary
// - Safe to re-run (idempotent, checkpoints)
// - Detects partial progress and emits WARN
// - Emits BLOCK on errors and halts
func (o *UpgradeOrchestrator) Run(ctx context.Context, replica string) (Summary, []Finding, error) {
	var summary Summary
	findings := []Finding{}

	replica = strings.TrimSpace(replica)
	if replica == "" {
		return Summary{Block: 1}, []Finding{{Severity: SeverityBlock, Message: "replica is required"}}, nil
	}
	if o.Inspector == nil || o.Actions == nil {
		return Summary{}, nil, fmt.Errorf("inspector and actions are required")
	}

	isPrimary, err := o.Inspector.IsPrimary(ctx, replica)
	if err != nil {
		return Summary{Block: 1}, []Finding{{Severity: SeverityBlock, Message: fmt.Sprintf("failed to determine primary status: %v", err)}}, nil
	}
	if isPrimary || (o.Primary != "" && replica == o.Primary) {
		return Summary{Block: 1}, []Finding{{Severity: SeverityBlock, Message: "refusing to upgrade primary", Meta: map[string]interface{}{"replica": replica}}}, nil
	}

	status, err := o.Inspector.ReplicationStatus(ctx, replica)
	if err != nil {
		warn := Finding{Severity: SeverityWarn, Message: fmt.Sprintf("unable to read replication status: %v", err), Meta: map[string]interface{}{"replica": replica}}
		findings = append(findings, warn)
		applySummary(&summary, []Finding{warn})
	} else {
		findings = append(findings, detectPartialProgress(replica, status, o.State)...)
		applySummary(&summary, findings)
	}

	if hasBlock(findings) {
		return summary, findings, nil
	}

	if ok, _ := getBool(o.State, stoppedKey(replica)); !ok {
		o.Logger.Printf("stopping replication on %s", replica)
		if err := o.Actions.StopReplication(ctx, replica); err != nil {
			return appendBlock(summary, findings, fmt.Sprintf("failed to stop replication: %v", err))
		}
		setBool(o.State, stoppedKey(replica), true)
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "replication stopped", Meta: map[string]interface{}{"replica": replica}})
		applySummary(&summary, []Finding{findings[len(findings)-1]})
	} else {
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "replication already stopped", Meta: map[string]interface{}{"replica": replica}})
		applySummary(&summary, []Finding{findings[len(findings)-1]})
	}

	if ok, _ := getBool(o.State, upgradedKey(replica)); !ok {
		o.Logger.Printf("running upgrade on %s", replica)
		if err := o.Actions.RunUpgrade(ctx, replica); err != nil {
			return appendBlock(summary, findings, fmt.Sprintf("upgrade failed: %v", err))
		}
		setBool(o.State, upgradedKey(replica), true)
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "upgrade completed", Meta: map[string]interface{}{"replica": replica}})
		applySummary(&summary, []Finding{findings[len(findings)-1]})
	} else {
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "upgrade already completed", Meta: map[string]interface{}{"replica": replica}})
		applySummary(&summary, []Finding{findings[len(findings)-1]})
	}

	if ok, _ := getBool(o.State, resumedKey(replica)); !ok {
		o.Logger.Printf("starting replication on %s", replica)
		if err := o.Actions.StartReplication(ctx, replica); err != nil {
			return appendBlock(summary, findings, fmt.Sprintf("failed to start replication: %v", err))
		}
		setBool(o.State, resumedKey(replica), true)
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "replication started", Meta: map[string]interface{}{"replica": replica}})
		applySummary(&summary, []Finding{findings[len(findings)-1]})
	} else {
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "replication already started", Meta: map[string]interface{}{"replica": replica}})
		applySummary(&summary, []Finding{findings[len(findings)-1]})
	}

	return summary, findings, nil
}

func detectPartialProgress(replica string, status ReplicationStatus, state workflow.State) []Finding {
	findings := []Finding{}

	stopped, _ := getBool(state, stoppedKey(replica))
	resumed, _ := getBool(state, resumedKey(replica))

	threadsRunning := status.IOThreadRunning && status.SQLThreadRunning
	threadsStopped := !status.IOThreadRunning && !status.SQLThreadRunning

	if threadsStopped && !stopped {
		findings = append(findings, Finding{Severity: SeverityWarn, Message: "replication appears stopped but checkpoint is missing", Meta: map[string]interface{}{"replica": replica}})
	}
	if threadsRunning && stopped && !resumed {
		findings = append(findings, Finding{Severity: SeverityWarn, Message: "checkpoint indicates replication stopped but status is running", Meta: map[string]interface{}{"replica": replica}})
	}
	if threadsStopped && resumed {
		findings = append(findings, Finding{Severity: SeverityWarn, Message: "checkpoint indicates replication started but status is stopped", Meta: map[string]interface{}{"replica": replica}})
	}

	return findings
}

func hasBlock(findings []Finding) bool {
	for _, f := range findings {
		if f.Severity == SeverityBlock {
			return true
		}
	}
	return false
}

func appendBlock(summary Summary, findings []Finding, message string) (Summary, []Finding, error) {
	block := Finding{Severity: SeverityBlock, Message: message}
	findings = append(findings, block)
	applySummary(&summary, []Finding{block})
	return summary, findings, nil
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

func stoppedKey(replica string) string  { return fmt.Sprintf("replica_upgrade:%s:stopped", replica) }
func upgradedKey(replica string) string { return fmt.Sprintf("replica_upgrade:%s:upgraded", replica) }
func resumedKey(replica string) string  { return fmt.Sprintf("replica_upgrade:%s:resumed", replica) }

func getBool(state workflow.State, key string) (bool, bool) {
	if state == nil {
		return false, false
	}
	val, ok := state.Get(key)
	if !ok {
		return false, false
	}
	b, ok := val.(bool)
	return b, ok
}

func setBool(state workflow.State, key string, value bool) {
	if state == nil {
		return
	}
	state.Set(key, value)
}

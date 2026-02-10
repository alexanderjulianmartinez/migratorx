package cdc

import (
	"context"
	"testing"
	"time"

	"migratorx/internal/checks"
)

type fakeDebeziumInspector struct {
	status ConnectorStatus
	err    error
}

func (f *fakeDebeziumInspector) ConnectorStatus(ctx context.Context, connector string) (ConnectorStatus, error) {
	if f.err != nil {
		return ConnectorStatus{}, f.err
	}
	return f.status, nil
}

func TestDebeziumHealthCheck_TaskFailedBlocks(t *testing.T) {
	inspector := &fakeDebeziumInspector{status: ConnectorStatus{
		Name:           "mysql-prod",
		ConnectorState: "RUNNING",
		Tasks: []TaskStatus{{ID: 0, State: "FAILED", Trace: "stacktrace"}},
	}}

	check := &DebeziumHealthCheck{Inspector: inspector, Connector: "mysql-prod"}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK when task failed")
	}
}

func TestDebeziumHealthCheck_RestartLoopBlocks(t *testing.T) {
	recent := time.Now().Add(-2 * time.Minute)
	inspector := &fakeDebeziumInspector{status: ConnectorStatus{
		Name:           "mysql-prod",
		ConnectorState: "RUNNING",
		Tasks:          []TaskStatus{{ID: 0, State: "RUNNING"}},
		RestartCount:   5,
		LastRestartAt:  &recent,
	}}

	check := &DebeziumHealthCheck{Inspector: inspector, Connector: "mysql-prod", RestartLoopWindow: 10 * time.Minute, RestartLoopMax: 3}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK for restart loop")
	}
}

func TestDebeziumHealthCheck_HealthyInfo(t *testing.T) {
	inspector := &fakeDebeziumInspector{status: ConnectorStatus{
		Name:           "mysql-prod",
		ConnectorState: "RUNNING",
		Tasks:          []TaskStatus{{ID: 0, State: "RUNNING"}},
	}}

	check := &DebeziumHealthCheck{Inspector: inspector, Connector: "mysql-prod"}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, checks.SeverityInfo) {
		t.Fatalf("expected INFO when healthy")
	}
}

func TestDebeziumHealthCheck_ConnectorStoppedBlocks(t *testing.T) {
	inspector := &fakeDebeziumInspector{status: ConnectorStatus{
		Name:           "mysql-prod",
		ConnectorState: "PAUSED",
		Tasks:          []TaskStatus{{ID: 0, State: "RUNNING"}},
	}}

	check := &DebeziumHealthCheck{Inspector: inspector, Connector: "mysql-prod"}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK for connector not RUNNING")
	}
}

func hasSeverity(findings []checks.Finding, severity checks.Severity) bool {
	for _, f := range findings {
		if f.Severity == severity {
			return true
		}
	}
	return false
}
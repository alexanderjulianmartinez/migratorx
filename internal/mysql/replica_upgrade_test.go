package mysql

import (
	"context"
	"errors"
	"testing"

	"migratorx/internal/workflow"
)

type fakeInspector struct {
	isPrimary bool
	status    ReplicationStatus
	err       error
}

func (f *fakeInspector) IsPrimary(ctx context.Context, host string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.isPrimary, nil
}

func (f *fakeInspector) ReplicationStatus(ctx context.Context, replica string) (ReplicationStatus, error) {
	if f.err != nil {
		return ReplicationStatus{}, f.err
	}
	return f.status, nil
}

type fakeActions struct {
	stopCalls    int
	upgradeCalls int
	startCalls   int
	stopErr      error
	upgradeErr   error
	startErr     error
}

func (f *fakeActions) StopReplication(ctx context.Context, replica string) error {
	f.stopCalls++
	return f.stopErr
}

func (f *fakeActions) RunUpgrade(ctx context.Context, replica string) error {
	f.upgradeCalls++
	return f.upgradeErr
}

func (f *fakeActions) StartReplication(ctx context.Context, replica string) error {
	f.startCalls++
	return f.startErr
}

func TestUpgradeOrchestrator_RejectsPrimary(t *testing.T) {
	inspector := &fakeInspector{isPrimary: true}
	actions := &fakeActions{}
	state := workflow.NewMemoryState()

	o := NewUpgradeOrchestrator(inspector, actions, state, "mysql-primary", nil)
	summary, findings, err := o.Run(context.Background(), "mysql-primary")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Block != 1 || len(findings) != 1 || findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK for primary, got %+v", summary)
	}
	if actions.stopCalls != 0 || actions.upgradeCalls != 0 || actions.startCalls != 0 {
		t.Fatalf("actions should not be called when targeting primary")
	}
}

func TestUpgradeOrchestrator_PartialProgressWarns(t *testing.T) {
	inspector := &fakeInspector{status: ReplicationStatus{IOThreadRunning: true, SQLThreadRunning: true}}
	actions := &fakeActions{}
	state := workflow.NewMemoryState()
	state.Set("replica_upgrade:replica-1:stopped", true)

	o := NewUpgradeOrchestrator(inspector, actions, state, "mysql-primary", nil)
	summary, findings, err := o.Run(context.Background(), "replica-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Warn == 0 {
		t.Fatalf("expected WARN for partial progress detection")
	}
	if len(findings) == 0 {
		t.Fatalf("expected findings")
	}
}

func TestUpgradeOrchestrator_IdempotentRerun(t *testing.T) {
	inspector := &fakeInspector{status: ReplicationStatus{IOThreadRunning: true, SQLThreadRunning: true}}
	actions := &fakeActions{}
	state := workflow.NewMemoryState()
	state.Set("replica_upgrade:replica-1:stopped", true)
	state.Set("replica_upgrade:replica-1:upgraded", true)
	state.Set("replica_upgrade:replica-1:resumed", true)

	o := NewUpgradeOrchestrator(inspector, actions, state, "mysql-primary", nil)
	_, _, err := o.Run(context.Background(), "replica-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actions.stopCalls != 0 || actions.upgradeCalls != 0 || actions.startCalls != 0 {
		t.Fatalf("actions should not be called on rerun when checkpoints exist")
	}
}

func TestUpgradeOrchestrator_BlockStopsFlow(t *testing.T) {
	inspector := &fakeInspector{status: ReplicationStatus{IOThreadRunning: true, SQLThreadRunning: true}}
	actions := &fakeActions{upgradeErr: errors.New("boom")}
	state := workflow.NewMemoryState()

	o := NewUpgradeOrchestrator(inspector, actions, state, "mysql-primary", nil)
	summary, findings, err := o.Run(context.Background(), "replica-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Block != 1 {
		t.Fatalf("expected BLOCK when upgrade fails")
	}
	if actions.startCalls != 0 {
		t.Fatalf("start replication should not be called after BLOCK")
	}
	if len(findings) == 0 || findings[len(findings)-1].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK finding")
	}
}

func TestUpgradeOrchestrator_InspectorErrorBlocks(t *testing.T) {
	inspector := &fakeInspector{err: errors.New("status failure")}
	actions := &fakeActions{}
	state := workflow.NewMemoryState()

	o := NewUpgradeOrchestrator(inspector, actions, state, "mysql-primary", nil)
	summary, findings, err := o.Run(context.Background(), "replica-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Block != 1 {
		t.Fatalf("expected BLOCK when primary status can't be determined")
	}
	if len(findings) == 0 || findings[0].Severity != SeverityBlock {
		t.Fatalf("expected BLOCK finding")
	}
}

package cdc

import (
	"context"
	"fmt"
	"time"

	"migratorx/internal/checks"
)

// ConnectorStatus models Debezium connector status response (simplified).
type ConnectorStatus struct {
	Name            string
	ConnectorState  string
	ConnectorWorker string
	Tasks           []TaskStatus
	RestartCount    int
	LastRestartAt   *time.Time
}

// TaskStatus models a Debezium task status.
type TaskStatus struct {
	ID     int
	State  string
	Worker string
	Trace  string
}

// DebeziumInspector provides read-only access to Debezium status.
type DebeziumInspector interface {
	ConnectorStatus(ctx context.Context, connector string) (ConnectorStatus, error)
}

// DebeziumHealthCheck validates connector/task health and restart stability.
type DebeziumHealthCheck struct {
	Inspector          DebeziumInspector
	Connector          string
	RestartLoopWindow  time.Duration
	RestartLoopMax     int
}

func (c *DebeziumHealthCheck) Name() string   { return "cdc_debezium_health" }
func (c *DebeziumHealthCheck) ReadOnly() bool { return true }

func (c *DebeziumHealthCheck) Run(ctx context.Context, input checks.Input) ([]checks.Finding, error) {
	if c.Inspector == nil {
		return nil, fmt.Errorf("debezium inspector is required")
	}
	if c.Connector == "" {
		return nil, fmt.Errorf("connector name is required")
	}
	if c.RestartLoopWindow == 0 {
		c.RestartLoopWindow = 10 * time.Minute
	}
	if c.RestartLoopMax == 0 {
		c.RestartLoopMax = 3
	}

	status, err := c.Inspector.ConnectorStatus(ctx, c.Connector)
	if err != nil {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("failed to read Debezium connector status: %v", err),
			Meta:     map[string]interface{}{"connector": c.Connector},
		}}, nil
	}

	findings := []checks.Finding{}
	if status.ConnectorState != "RUNNING" {
		findings = append(findings, checks.Finding{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("connector %q is %s (expected RUNNING)", status.Name, status.ConnectorState),
			Meta:     map[string]interface{}{"connector": status.Name, "state": status.ConnectorState},
		})
	}

	for _, task := range status.Tasks {
		if task.State != "RUNNING" {
			findings = append(findings, checks.Finding{
				Severity: checks.SeverityBlock,
				Message:  fmt.Sprintf("connector %q task %d is %s", status.Name, task.ID, task.State),
				Meta:     map[string]interface{}{"connector": status.Name, "task_id": task.ID, "state": task.State, "trace": task.Trace},
			})
		}
	}

	if isRestartLoop(status, c.RestartLoopWindow, c.RestartLoopMax) {
		findings = append(findings, checks.Finding{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("connector %q appears to be in a restart loop (%d restarts within %s)", status.Name, status.RestartCount, c.RestartLoopWindow),
			Meta:     map[string]interface{}{"connector": status.Name, "restart_count": status.RestartCount, "window": c.RestartLoopWindow.String()},
		})
	}

	if len(findings) == 0 {
		findings = append(findings, checks.Finding{
			Severity: checks.SeverityInfo,
			Message:  fmt.Sprintf("connector %q and tasks are RUNNING", status.Name),
			Meta:     map[string]interface{}{"connector": status.Name},
		})
	}

	return findings, nil
}

func isRestartLoop(status ConnectorStatus, window time.Duration, max int) bool {
	if status.LastRestartAt == nil {
		return false
	}
	if status.RestartCount < max {
		return false
	}
	return time.Since(*status.LastRestartAt) <= window
}
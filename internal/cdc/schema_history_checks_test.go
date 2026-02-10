package cdc

import (
	"context"
	"errors"
	"testing"

	"migratorx/internal/checks"
)

type fakeKafkaInspector struct {
	exists   bool
	readable bool
	covered  []string
	err      error
}

func (f *fakeKafkaInspector) TopicExists(ctx context.Context, topic string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.exists, nil
}

func (f *fakeKafkaInspector) TopicReadable(ctx context.Context, topic string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.readable, nil
}

func (f *fakeKafkaInspector) SchemaHistoryTables(ctx context.Context, topic string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.covered, nil
}

func TestSchemaHistoryCheck_MissingTopicBlocks(t *testing.T) {
	check := &SchemaHistoryCheck{Inspector: &fakeKafkaInspector{exists: false}, Topic: "schema-history"}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCDCSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK for missing topic")
	}
}

func TestSchemaHistoryCheck_InaccessibleBlocks(t *testing.T) {
	check := &SchemaHistoryCheck{Inspector: &fakeKafkaInspector{exists: true, readable: false}, Topic: "schema-history"}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCDCSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK for unreadable topic")
	}
}

func TestSchemaHistoryCheck_CoverageBlocks(t *testing.T) {
	check := &SchemaHistoryCheck{
		Inspector:     &fakeKafkaInspector{exists: true, readable: true, covered: []string{"db.t1"}},
		Topic:         "schema-history",
		ExpectedTables: []string{"db.t1", "db.t2"},
	}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCDCSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK for missing table coverage")
	}
}

func TestSchemaHistoryCheck_HealthyInfo(t *testing.T) {
	check := &SchemaHistoryCheck{
		Inspector:     &fakeKafkaInspector{exists: true, readable: true, covered: []string{"db.t1"}},
		Topic:         "schema-history",
		ExpectedTables: []string{"db.t1"},
	}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCDCSeverity(findings, checks.SeverityInfo) {
		t.Fatalf("expected INFO for healthy schema history")
	}
}

func TestSchemaHistoryCheck_InspectorErrorBlocks(t *testing.T) {
	check := &SchemaHistoryCheck{Inspector: &fakeKafkaInspector{err: errors.New("boom")}, Topic: "schema-history"}
	findings, err := check.Run(context.Background(), checks.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasCDCSeverity(findings, checks.SeverityBlock) {
		t.Fatalf("expected BLOCK for inspector error")
	}
}

func hasCDCSeverity(findings []checks.Finding, severity checks.Severity) bool {
	for _, f := range findings {
		if f.Severity == severity {
			return true
		}
	}
	return false
}
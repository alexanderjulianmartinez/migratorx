package checks

import (
	"context"
	"testing"
)

type fakeSchemaInspector struct {
	primary Schema
	replica Schema
	err     error
}

func (f *fakeSchemaInspector) Schema(ctx context.Context, host string) (Schema, error) {
	if f.err != nil {
		return Schema{}, f.err
	}
	if host == "primary" {
		return f.primary, nil
	}
	return f.replica, nil
}

func TestSchemaParity_PKMissingOnReplicaBlocks(t *testing.T) {
	inspector := &fakeSchemaInspector{
		primary: Schema{Tables: []Table{{Name: "t", PrimaryKey: []string{"id"}}}},
		replica: Schema{Tables: []Table{{Name: "t", PrimaryKey: nil}}},
	}

	check := &SchemaParityCheck{Inspector: inspector, PrimaryHost: "primary", ReplicaHost: "replica"}
	findings, err := check.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, SeverityBlock) {
		t.Fatalf("expected BLOCK for missing primary key")
	}
}

func TestSchemaParity_ColumnTypeMismatchBlocks(t *testing.T) {
	inspector := &fakeSchemaInspector{
		primary: Schema{Tables: []Table{{
			Name: "t",
			Columns: []Column{{Name: "id", Type: "int"}},
		}}},
		replica: Schema{Tables: []Table{{
			Name: "t",
			Columns: []Column{{Name: "id", Type: "bigint"}},
		}}},
	}

	check := &SchemaParityCheck{Inspector: inspector, PrimaryHost: "primary", ReplicaHost: "replica"}
	findings, err := check.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, SeverityBlock) {
		t.Fatalf("expected BLOCK for type mismatch")
	}
}

func TestSchemaParity_NullabilityWarns(t *testing.T) {
	inspector := &fakeSchemaInspector{
		primary: Schema{Tables: []Table{{
			Name: "t",
			Columns: []Column{{Name: "c", Type: "varchar(10)", Nullable: true}},
		}}},
		replica: Schema{Tables: []Table{{
			Name: "t",
			Columns: []Column{{Name: "c", Type: "varchar(10)", Nullable: false}},
		}}},
	}

	check := &SchemaParityCheck{Inspector: inspector, PrimaryHost: "primary", ReplicaHost: "replica"}
	findings, err := check.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, SeverityWarn) {
		t.Fatalf("expected WARN for nullability mismatch")
	}
}

func TestSchemaParity_ExtraReplicaColumnWarns(t *testing.T) {
	inspector := &fakeSchemaInspector{
		primary: Schema{Tables: []Table{{
			Name: "t",
			Columns: []Column{{Name: "a", Type: "int"}},
		}}},
		replica: Schema{Tables: []Table{{
			Name: "t",
			Columns: []Column{{Name: "a", Type: "int"}, {Name: "b", Type: "int"}},
		}}},
	}

	check := &SchemaParityCheck{Inspector: inspector, PrimaryHost: "primary", ReplicaHost: "replica"}
	findings, err := check.Run(context.Background(), Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverity(findings, SeverityWarn) {
		t.Fatalf("expected WARN for extra replica column")
	}
}

func hasSeverity(findings []Finding, severity Severity) bool {
	for _, f := range findings {
		if f.Severity == severity {
			return true
		}
	}
	return false
}
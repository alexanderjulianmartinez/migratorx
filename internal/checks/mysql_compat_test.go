package checks

import (
	"context"
	"testing"
)

type fakeMySQLInspector struct {
	sqlMode  string
	features []string
}

func (f *fakeMySQLInspector) SQLMode(ctx context.Context, host string) (string, error) {
	return f.sqlMode, nil
}

func (f *fakeMySQLInspector) DeprecatedFeaturesUsed(ctx context.Context, host string) ([]string, error) {
	return f.features, nil
}

type fakeSchemaInspectorCompat struct {
	schema Schema
}

func (f *fakeSchemaInspectorCompat) Schema(ctx context.Context, host string) (Schema, error) {
	return f.schema, nil
}

func TestMySQLCompatibility_MissingPrimaryKeyBlocks(t *testing.T) {
	check := &MySQLCompatibilityCheck{
		Inspector:       &fakeMySQLInspector{sqlMode: "", features: nil},
		SchemaInspector: &fakeSchemaInspectorCompat{schema: Schema{Tables: []Table{{Name: "t"}}}},
		PrimaryHost:     "primary",
	}

	findings, err := check.Run(context.Background(), Input{PlanSourceVersion: "5.7", PlanTargetVersion: "8.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverityCompat(findings, SeverityBlock) {
		t.Fatalf("expected BLOCK for missing primary key")
	}
}

func TestMySQLCompatibility_DeprecatedFeaturesBlock(t *testing.T) {
	check := &MySQLCompatibilityCheck{
		Inspector:          &fakeMySQLInspector{sqlMode: "", features: []string{"OLD_AUTH"}},
		SchemaInspector:    &fakeSchemaInspectorCompat{schema: Schema{Tables: []Table{{Name: "t", PrimaryKey: []string{"id"}}}}},
		PrimaryHost:        "primary",
		DeprecatedFeatures: []string{"OLD_AUTH"},
	}

	findings, err := check.Run(context.Background(), Input{PlanSourceVersion: "5.7", PlanTargetVersion: "8.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverityCompat(findings, SeverityBlock) {
		t.Fatalf("expected BLOCK for deprecated feature")
	}
}

func TestMySQLCompatibility_SQLModeWarns(t *testing.T) {
	check := &MySQLCompatibilityCheck{
		Inspector:          &fakeMySQLInspector{sqlMode: "NO_ZERO_DATE,STRICT_ALL_TABLES", features: nil},
		SchemaInspector:    &fakeSchemaInspectorCompat{schema: Schema{Tables: []Table{{Name: "t", PrimaryKey: []string{"id"}}}}},
		PrimaryHost:        "primary",
		DeprecatedSQLModes: []string{"NO_ZERO_DATE"},
	}

	findings, err := check.Run(context.Background(), Input{PlanSourceVersion: "5.7", PlanTargetVersion: "8.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverityCompat(findings, SeverityWarn) {
		t.Fatalf("expected WARN for deprecated sql_mode")
	}
}

func TestMySQLCompatibility_CharsetCollationWarns(t *testing.T) {
	check := &MySQLCompatibilityCheck{
		Inspector: &fakeMySQLInspector{sqlMode: "", features: nil},
		SchemaInspector: &fakeSchemaInspectorCompat{schema: Schema{Tables: []Table{{
			Name:       "t",
			PrimaryKey: []string{"id"},
			Columns:    []Column{{Name: "c", Charset: "utf8", Collation: "utf8_general_ci"}},
		}}}},
		PrimaryHost:     "primary",
		RiskyCharsets:   []string{"utf8"},
		RiskyCollations: []string{"utf8_general_ci"},
	}

	findings, err := check.Run(context.Background(), Input{PlanSourceVersion: "5.7", PlanTargetVersion: "8.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasSeverityCompat(findings, SeverityWarn) {
		t.Fatalf("expected WARN for charset/collation risk")
	}
}

func hasSeverityCompat(findings []Finding, severity Severity) bool {
	for _, f := range findings {
		if f.Severity == severity {
			return true
		}
	}
	return false
}

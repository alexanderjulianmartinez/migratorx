package checks

import (
	"context"
	"fmt"
	"strings"
)

// MySQLInspector provides read-only access to MySQL settings and metadata.
type MySQLInspector interface {
	SQLMode(ctx context.Context, host string) (string, error)
	DeprecatedFeaturesUsed(ctx context.Context, host string) ([]string, error)
}

// MySQLCompatibilityCheck validates MySQL 5.7 → 8.0 compatibility signals.
// It detects:
// - sql_mode risk modes (WARN)
// - charset/collation risks (WARN)
// - deprecated features (BLOCK)
// - missing primary keys (BLOCK)
type MySQLCompatibilityCheck struct {
	Inspector          MySQLInspector
	SchemaInspector    SchemaInspector
	PrimaryHost        string
	DeprecatedSQLModes []string
	DeprecatedFeatures []string
	RiskyCharsets      []string
	RiskyCollations    []string
}

func (c *MySQLCompatibilityCheck) Name() string   { return "mysql_compat_57_80" }
func (c *MySQLCompatibilityCheck) ReadOnly() bool { return true }

func (c *MySQLCompatibilityCheck) Run(ctx context.Context, input Input) ([]Finding, error) {
	if c.Inspector == nil {
		return nil, fmt.Errorf("mysql inspector is required")
	}
	if c.SchemaInspector == nil {
		return nil, fmt.Errorf("schema inspector is required")
	}
	if strings.TrimSpace(c.PrimaryHost) == "" {
		return nil, fmt.Errorf("primary host is required")
	}

	findings := []Finding{}

	if input.PlanSourceVersion != "" || input.PlanTargetVersion != "" {
		if input.PlanSourceVersion != "5.7" || input.PlanTargetVersion != "8.0" {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  "compatibility check tuned for 5.7 → 8.0 upgrades",
				Meta:     map[string]interface{}{"source_version": input.PlanSourceVersion, "target_version": input.PlanTargetVersion},
			})
		}
	}

	modeStr, err := c.Inspector.SQLMode(ctx, c.PrimaryHost)
	if err != nil {
		return nil, fmt.Errorf("failed to read sql_mode: %v", err)
	}
	modeSet := splitCSV(modeStr)
	for _, mode := range c.DeprecatedSQLModes {
		if modeSet[strings.ToUpper(mode)] {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("sql_mode includes deprecated mode %q for 8.0", mode),
				Meta:     map[string]interface{}{"mode": mode},
			})
		}
	}

	features, err := c.Inspector.DeprecatedFeaturesUsed(ctx, c.PrimaryHost)
	if err != nil {
		return nil, fmt.Errorf("failed to read deprecated features: %v", err)
	}
	featureSet := toUpperSet(features)
	for _, feature := range c.DeprecatedFeatures {
		if featureSet[strings.ToUpper(feature)] {
			findings = append(findings, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("deprecated feature detected: %q", feature),
				Meta:     map[string]interface{}{"feature": feature},
			})
		}
	}

	schema, err := c.SchemaInspector.Schema(ctx, c.PrimaryHost)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema: %v", err)
	}
	for _, table := range schema.Tables {
		if len(table.PrimaryKey) == 0 {
			findings = append(findings, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("table %q missing primary key (CDC risk)", table.Name),
				Meta:     map[string]interface{}{"table": table.Name},
			})
		}
		for _, col := range table.Columns {
			if containsInsensitive(c.RiskyCharsets, col.Charset) {
				findings = append(findings, Finding{
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("table %q column %q uses risky charset %q", table.Name, col.Name, col.Charset),
					Meta:     map[string]interface{}{"table": table.Name, "column": col.Name, "charset": col.Charset},
				})
			}
			if containsInsensitive(c.RiskyCollations, col.Collation) {
				findings = append(findings, Finding{
					Severity: SeverityWarn,
					Message:  fmt.Sprintf("table %q column %q uses risky collation %q", table.Name, col.Name, col.Collation),
					Meta:     map[string]interface{}{"table": table.Name, "column": col.Name, "collation": col.Collation},
				})
			}
		}
	}

	if len(findings) == 0 {
		findings = append(findings, Finding{Severity: SeverityInfo, Message: "no MySQL 5.7 → 8.0 compatibility risks detected"})
	}

	return findings, nil
}

func splitCSV(value string) map[string]bool {
	set := map[string]bool{}
	for _, part := range strings.Split(value, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		set[strings.ToUpper(p)] = true
	}
	return set
}

func toUpperSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, v := range values {
		set[strings.ToUpper(strings.TrimSpace(v))] = true
	}
	return set
}

func containsInsensitive(list []string, value string) bool {
	if value == "" {
		return false
	}
	upper := strings.ToUpper(value)
	for _, item := range list {
		if strings.ToUpper(item) == upper {
			return true
		}
	}
	return false
}

package checks

import (
	"context"
	"fmt"
)

// Column describes a table column in a schema snapshot.
type Column struct {
	Name      string
	Type      string
	Nullable  bool
	Default   *string
	Charset   string
	Collation string
}

// Table describes a table in a schema snapshot.
type Table struct {
	Name       string
	Columns    []Column
	PrimaryKey []string
}

// Schema describes a database schema snapshot.
type Schema struct {
	Tables []Table
}

// SchemaInspector provides read-only schema access for parity checks.
type SchemaInspector interface {
	Schema(ctx context.Context, host string) (Schema, error)
}

// SchemaParityCheck compares primary vs replica schemas and emits findings.
type SchemaParityCheck struct {
	Inspector   SchemaInspector
	PrimaryHost string
	ReplicaHost string
}

func (c *SchemaParityCheck) Name() string   { return "schema_parity" }
func (c *SchemaParityCheck) ReadOnly() bool { return true }

func (c *SchemaParityCheck) Run(ctx context.Context, input Input) ([]Finding, error) {
	if c.Inspector == nil {
		return nil, fmt.Errorf("schema inspector is required")
	}
	if c.PrimaryHost == "" || c.ReplicaHost == "" {
		return nil, fmt.Errorf("primary and replica hosts are required")
	}

	primary, err := c.Inspector.Schema(ctx, c.PrimaryHost)
	if err != nil {
		return nil, fmt.Errorf("failed to read primary schema: %v", err)
	}
	replica, err := c.Inspector.Schema(ctx, c.ReplicaHost)
	if err != nil {
		return nil, fmt.Errorf("failed to read replica schema: %v", err)
	}

	return compareSchemas(primary, replica), nil
}

func compareSchemas(primary Schema, replica Schema) []Finding {
	findings := []Finding{}
	primaryTables := tableIndex(primary.Tables)
	replicaTables := tableIndex(replica.Tables)

	for name, pTable := range primaryTables {
		rTable, ok := replicaTables[name]
		if !ok {
			findings = append(findings, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("table %q missing on replica", name),
				Meta:     map[string]interface{}{"table": name},
			})
			continue
		}

		findings = append(findings, comparePrimaryKey(name, pTable.PrimaryKey, rTable.PrimaryKey)...)
		findings = append(findings, compareColumns(name, pTable.Columns, rTable.Columns)...)
	}

	for name := range replicaTables {
		if _, ok := primaryTables[name]; !ok {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("extra table %q exists on replica", name),
				Meta:     map[string]interface{}{"table": name},
			})
		}
	}

	return findings
}

func comparePrimaryKey(table string, primaryPK []string, replicaPK []string) []Finding {
	if len(primaryPK) == 0 && len(replicaPK) == 0 {
		return nil
	}
	if len(primaryPK) == 0 && len(replicaPK) > 0 {
		return []Finding{{
			Severity: SeverityWarn,
			Message:  fmt.Sprintf("table %q has primary key on replica but not on primary", table),
			Meta:     map[string]interface{}{"table": table},
		}}
	}
	if len(primaryPK) > 0 && len(replicaPK) == 0 {
		return []Finding{{
			Severity: SeverityBlock,
			Message:  fmt.Sprintf("table %q missing primary key on replica", table),
			Meta:     map[string]interface{}{"table": table},
		}}
	}
	if !equalStrings(primaryPK, replicaPK) {
		return []Finding{{
			Severity: SeverityBlock,
			Message:  fmt.Sprintf("table %q primary key mismatch", table),
			Meta:     map[string]interface{}{"table": table, "primary_pk": primaryPK, "replica_pk": replicaPK},
		}}
	}
	return nil
}

func compareColumns(table string, primaryCols []Column, replicaCols []Column) []Finding {
	findings := []Finding{}
	primaryIndex := columnIndex(primaryCols)
	replicaIndex := columnIndex(replicaCols)

	for name, pCol := range primaryIndex {
		rCol, ok := replicaIndex[name]
		if !ok {
			findings = append(findings, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("table %q column %q missing on replica", table, name),
				Meta:     map[string]interface{}{"table": table, "column": name},
			})
			continue
		}

		if pCol.Type != rCol.Type {
			findings = append(findings, Finding{
				Severity: SeverityBlock,
				Message:  fmt.Sprintf("table %q column %q type mismatch", table, name),
				Meta:     map[string]interface{}{"table": table, "column": name, "primary_type": pCol.Type, "replica_type": rCol.Type},
			})
		}
		if pCol.Nullable != rCol.Nullable {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("table %q column %q nullability differs", table, name),
				Meta:     map[string]interface{}{"table": table, "column": name, "primary_nullable": pCol.Nullable, "replica_nullable": rCol.Nullable},
			})
		}
		if !equalDefaults(pCol.Default, rCol.Default) {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("table %q column %q default differs", table, name),
				Meta:     map[string]interface{}{"table": table, "column": name, "primary_default": pCol.Default, "replica_default": rCol.Default},
			})
		}
		if pCol.Collation != rCol.Collation {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("table %q column %q collation differs", table, name),
				Meta:     map[string]interface{}{"table": table, "column": name, "primary_collation": pCol.Collation, "replica_collation": rCol.Collation},
			})
		}
	}

	for name := range replicaIndex {
		if _, ok := primaryIndex[name]; !ok {
			findings = append(findings, Finding{
				Severity: SeverityWarn,
				Message:  fmt.Sprintf("table %q has extra column %q on replica", table, name),
				Meta:     map[string]interface{}{"table": table, "column": name},
			})
		}
	}

	return findings
}

func tableIndex(tables []Table) map[string]Table {
	idx := make(map[string]Table, len(tables))
	for _, t := range tables {
		idx[t.Name] = t
	}
	return idx
}

func columnIndex(cols []Column) map[string]Column {
	idx := make(map[string]Column, len(cols))
	for _, c := range cols {
		idx[c.Name] = c
	}
	return idx
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalDefaults(a *string, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

package workflow

import "testing"

func TestMigrationPlanValidate_Success(t *testing.T) {
	plan := MigrationPlan{
		Migration:     "mysql_57_to_80",
		SourceVersion: "5.7",
		TargetVersion: "8.0",
		Topology: Topology{
			Primary:  "mysql-primary",
			Replicas: []string{"mysql-replica-1"},
		},
		CDC: CDCConfig{
			Type:      "debezium",
			Connector: "mysql-prod",
		},
		Steps: []string{"preflight", "upgrade_replica", "validate_replica", "cdc_check", "promote", "post_validation"},
	}

	if err := plan.Validate(); err != nil {
		t.Fatalf("expected valid plan, got error: %v", err)
	}
}

func TestMigrationPlanValidate_MissingFields(t *testing.T) {
	plan := MigrationPlan{}
	err := plan.Validate()
	if err == nil {
		t.Fatalf("expected validation error for missing fields")
	}
}

func TestMigrationPlanValidate_UnsupportedStep(t *testing.T) {
	plan := MigrationPlan{
		Migration:     "mysql_57_to_80",
		SourceVersion: "5.7",
		TargetVersion: "8.0",
		Topology: Topology{
			Primary:  "mysql-primary",
			Replicas: []string{"mysql-replica-1"},
		},
		CDC: CDCConfig{
			Type:      "debezium",
			Connector: "mysql-prod",
		},
		Steps: []string{"preflight", "unknown_step"},
	}

	err := plan.Validate()
	if err == nil {
		t.Fatalf("expected validation error for unsupported step")
	}
}

func TestMigrationPlanValidate_InvalidOrder(t *testing.T) {
	plan := MigrationPlan{
		Migration:     "mysql_57_to_80",
		SourceVersion: "5.7",
		TargetVersion: "8.0",
		Topology: Topology{
			Primary:  "mysql-primary",
			Replicas: []string{"mysql-replica-1"},
		},
		CDC: CDCConfig{
			Type:      "debezium",
			Connector: "mysql-prod",
		},
		Steps: []string{"validate_replica", "preflight"},
	}

	err := plan.Validate()
	if err == nil {
		t.Fatalf("expected validation error for invalid step ordering")
	}
}

func TestMigrationPlanValidate_DuplicateStep(t *testing.T) {
	plan := MigrationPlan{
		Migration:     "mysql_57_to_80",
		SourceVersion: "5.7",
		TargetVersion: "8.0",
		Topology: Topology{
			Primary:  "mysql-primary",
			Replicas: []string{"mysql-replica-1"},
		},
		CDC: CDCConfig{
			Type:      "debezium",
			Connector: "mysql-prod",
		},
		Steps: []string{"preflight", "preflight"},
	}

	err := plan.Validate()
	if err == nil {
		t.Fatalf("expected validation error for duplicate step")
	}
}

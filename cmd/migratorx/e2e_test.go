package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type cliOutput struct {
	Summary struct {
		Info  int `json:"info"`
		Warn  int `json:"warn"`
		Block int `json:"block"`
	} `json:"summary"`
}

func TestCLI_EndToEndExamplePlan(t *testing.T) {
	root := repoRoot(t)
	temp := t.TempDir()

	planPath := filepath.Join(temp, "migration.yaml")
	schemaPrimary := filepath.Join(temp, "primary_schema.json")
	schemaReplica := filepath.Join(temp, "replica_schema.json")
	cdcStatus := filepath.Join(temp, "cdc_status.json")
	statePath := filepath.Join(temp, "state.json")

	writeFile(t, planPath, examplePlanYAML())
	writeFile(t, schemaPrimary, exampleSchemaJSON())
	writeFile(t, schemaReplica, exampleSchemaJSON())
	writeFile(t, cdcStatus, exampleCDCStatusJSON())

	commands := [][]string{
		{"plan", "--plan", planPath},
		{"preflight", "--plan", planPath, "--schema-primary", schemaPrimary, "--schema-replica", schemaReplica, "--cdc-status", cdcStatus},
		{"upgrade", "replica", "mysql-replica-1", "--plan", planPath, "--state", statePath, "--simulate", "--io-running", "true", "--sql-running", "true"},
		{"validate", "replica", "mysql-replica-1", "--plan", planPath, "--schema-primary", schemaPrimary, "--schema-replica", schemaReplica},
		{"cdc", "check", "--plan", planPath, "--cdc-status", cdcStatus},
		{"promote", "--plan", planPath, "--confirm", "PROMOTE", "--phrase", "PROMOTE", "--schema-primary", schemaPrimary, "--schema-replica", schemaReplica, "--cdc-status", cdcStatus},
		{"validate", "primary", "--plan", planPath, "--schema-primary", schemaPrimary, "--schema-replica", schemaReplica},
	}

	for _, args := range commands {
		out, raw := runCLI(t, root, args...)
		if out.Summary.Block != 0 {
			t.Fatalf("command %v returned BLOCK=%d\noutput: %s", args, out.Summary.Block, raw)
		}
	}
}

func runCLI(t *testing.T, root string, args ...string) (cliOutput, string) {
	cmdArgs := append([]string{"run", "./cmd/migratorx"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = root
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("cli failed: %v\nstderr: %s", err, stderr.String())
	}
	var out cliOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("failed to parse output: %v\nraw: %s", err, stdout.String())
	}
	return out, stdout.String()
}

func repoRoot(t *testing.T) string {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	return root
}

func writeFile(t *testing.T, path string, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func examplePlanYAML() string {
	return "" +
		"migration: mysql_57_to_80\n" +
		"source_version: 5.7\n" +
		"target_version: 8.0\n" +
		"\n" +
		"topology:\n" +
		"  primary: mysql-primary\n" +
		"  replicas:\n" +
		"    - mysql-replica-1\n" +
		"\n" +
		"cdc:\n" +
		"  type: debezium\n" +
		"  connector: mysql-prod\n" +
		"\n" +
		"steps:\n" +
		"  - preflight\n" +
		"  - upgrade_replica\n" +
		"  - validate_replica\n" +
		"  - cdc_check\n" +
		"  - promote\n" +
		"  - post_validation\n"
}

func exampleSchemaJSON() string {
	return `{
  "Tables": [
    {
      "Name": "users",
      "PrimaryKey": ["id"],
      "Columns": [
        {"Name": "id", "Type": "int", "Nullable": false, "Charset": "utf8mb4", "Collation": "utf8mb4_general_ci"},
        {"Name": "email", "Type": "varchar(255)", "Nullable": false, "Charset": "utf8mb4", "Collation": "utf8mb4_general_ci"}
      ]
    }
  ]
}`
}

func exampleCDCStatusJSON() string {
	return `{
  "Name": "mysql-prod",
  "ConnectorState": "RUNNING",
  "ConnectorWorker": "worker-1",
  "Tasks": [
    {"ID": 0, "State": "RUNNING", "Worker": "worker-1", "Trace": ""}
  ],
  "RestartCount": 0
}`
}

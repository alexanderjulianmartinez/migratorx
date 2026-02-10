package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"migratorx/internal/cdc"
	"migratorx/internal/checks"
	"migratorx/internal/mysql"
	"migratorx/internal/state"
	"migratorx/internal/workflow"
)

type Output struct {
	Summary  Summary         `json:"summary"`
	Findings []OutputFinding `json:"findings"`
}

type Summary struct {
	Info  int `json:"info"`
	Warn  int `json:"warn"`
	Block int `json:"block"`
}

type OutputFinding struct {
	Severity string                 `json:"severity"`
	Message  string                 `json:"message"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		printUsageAndExit()
	}

	cmd := os.Args[1]
	switch cmd {
	case "plan":
		handlePlan(os.Args[2:])
	case "preflight":
		handlePreflight(os.Args[2:])
	case "upgrade":
		handleUpgrade(os.Args[2:])
	case "validate":
		handleValidate(os.Args[2:])
	case "cdc":
		handleCDC(os.Args[2:])
	case "promote":
		handlePromote(os.Args[2:])
	default:
		printUsageAndExit()
	}
}

func handlePlan(args []string) {
	fs := flag.NewFlagSet("plan", flag.ExitOnError)
	planPath := fs.String("plan", "migration.yaml", "path to migration plan YAML")
	_ = fs.Parse(args)

	plan, err := workflow.LoadPlan(*planPath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}
	writeOutput(Output{Summary: Summary{Info: 1}, Findings: []OutputFinding{{Severity: "INFO", Message: fmt.Sprintf("plan %q is valid", plan.Migration)}}})
}

func handlePreflight(args []string) {
	fs := flag.NewFlagSet("preflight", flag.ExitOnError)
	planPath := fs.String("plan", "migration.yaml", "path to migration plan YAML")
	primarySchema := fs.String("schema-primary", "", "path to primary schema JSON")
	replicaSchema := fs.String("schema-replica", "", "path to replica schema JSON")
	cdcStatus := fs.String("cdc-status", "", "path to Debezium status JSON")
	_ = fs.Parse(args)

	plan, err := workflow.LoadPlan(*planPath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}

	replicaHost, repErr := selectReplica(plan)
	if repErr != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: repErr.Error()}}})
		return
	}
	checksList := buildChecks(*primarySchema, *replicaSchema, *cdcStatus, plan.Topology.Primary, replicaHost, plan)
	runner := checks.NewRunner(checksList, log.Default())
	summary, results, err := runner.Run(context.Background(), planInput(plan, replicaHost))
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}
	writeOutput(convertCheckResults(summary, results))
}

func handleUpgrade(args []string) {
	if len(args) < 2 || args[0] != "replica" {
		printUsageAndExit()
	}
	fs := flag.NewFlagSet("upgrade replica", flag.ExitOnError)
	planPath := fs.String("plan", "migration.yaml", "path to migration plan YAML")
	statePath := fs.String("state", defaultStatePath(), "path to state file")
	simulate := fs.Bool("simulate", false, "simulate actions without touching MySQL")
	ioRunning := fs.Bool("io-running", true, "replica IO thread running")
	sqlRunning := fs.Bool("sql-running", true, "replica SQL thread running")
	_ = fs.Parse(args[2:])
	replica := args[1]

	plan, err := workflow.LoadPlan(*planPath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}

	st, err := state.NewFileState(*statePath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}

	inspector := &staticReplicaInspector{isPrimary: replica == plan.Topology.Primary, status: mysql.ReplicationStatus{IOThreadRunning: *ioRunning, SQLThreadRunning: *sqlRunning}}
	actions := mysql.ReplicaActions(&notConfiguredActions{})
	if *simulate {
		actions = &simulatedActions{}
	}

	orchestrator := mysql.NewUpgradeOrchestrator(inspector, actions, st, plan.Topology.Primary, log.Default())
	summary, findings, err := orchestrator.Run(context.Background(), replica)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}
	writeOutput(convertMySQLFindings(summary, findings))
}

func handleValidate(args []string) {
	if len(args) < 2 {
		printUsageAndExit()
	}
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	planPath := fs.String("plan", "migration.yaml", "path to migration plan YAML")
	primarySchema := fs.String("schema-primary", "", "path to primary schema JSON")
	replicaSchema := fs.String("schema-replica", "", "path to replica schema JSON")
	if args[0] == "replica" {
		if len(args) < 2 {
			printUsageAndExit()
		}
		_ = fs.Parse(args[2:])
	} else {
		_ = fs.Parse(args[1:])
	}

	plan, err := workflow.LoadPlan(*planPath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}

	switch args[0] {
	case "replica":
		if len(args) < 2 {
			printUsageAndExit()
		}
		check := buildSchemaParityCheck(*primarySchema, *replicaSchema, plan.Topology.Primary, args[1])
		findings, err := check.Run(context.Background(), planInput(plan, args[1]))
		if err != nil {
			writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
			return
		}
		writeOutput(convertCheckFindings(findings))
	case "primary":
		replicaHost, repErr := selectReplica(plan)
		if repErr != nil {
			writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: repErr.Error()}}})
			return
		}
		check := buildSchemaParityCheck(*primarySchema, *replicaSchema, plan.Topology.Primary, replicaHost)
		findings, err := check.Run(context.Background(), planInput(plan, replicaHost))
		if err != nil {
			writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
			return
		}
		writeOutput(convertCheckFindings(findings))
	default:
		printUsageAndExit()
	}
}

func handleCDC(args []string) {
	if len(args) < 1 || args[0] != "check" {
		printUsageAndExit()
	}
	fs := flag.NewFlagSet("cdc check", flag.ExitOnError)
	planPath := fs.String("plan", "migration.yaml", "path to migration plan YAML")
	cdcStatus := fs.String("cdc-status", "", "path to Debezium status JSON")
	_ = fs.Parse(args[1:])

	plan, err := workflow.LoadPlan(*planPath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}

	check := buildDebeziumCheck(*cdcStatus, plan.CDC.Connector)
	findings, err := check.Run(context.Background(), planInput(plan, ""))
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}
	writeOutput(convertCheckFindings(findings))
}

func handlePromote(args []string) {
	fs := flag.NewFlagSet("promote", flag.ExitOnError)
	planPath := fs.String("plan", "migration.yaml", "path to migration plan YAML")
	primarySchema := fs.String("schema-primary", "", "path to primary schema JSON")
	replicaSchema := fs.String("schema-replica", "", "path to replica schema JSON")
	cdcStatus := fs.String("cdc-status", "", "path to Debezium status JSON")
	confirm := fs.String("confirm", "", "confirmation phrase")
	phrase := fs.String("phrase", "PROMOTE", "required confirmation phrase")
	_ = fs.Parse(args)

	plan, err := workflow.LoadPlan(*planPath)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}

	replicaHost, repErr := selectReplica(plan)
	if repErr != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: repErr.Error()}}})
		return
	}
	checksList := buildChecks(*primarySchema, *replicaSchema, *cdcStatus, plan.Topology.Primary, replicaHost, plan)
	gate := workflow.PromotionGate{Checks: checksList, ConfirmationPhrase: *phrase}
	summary, findings, err := gate.Run(context.Background(), planInput(plan, replicaHost), *confirm)
	if err != nil {
		writeOutput(Output{Summary: Summary{Block: 1}, Findings: []OutputFinding{{Severity: "BLOCK", Message: err.Error()}}})
		return
	}
	writeOutput(convertCheckSummary(summary, findings))
}

func buildChecks(primarySchema string, replicaSchema string, cdcStatus string, primaryHost string, replicaHost string, plan workflow.MigrationPlan) []checks.PreflightCheck {
	checksList := []checks.PreflightCheck{}
	checksList = append(checksList, buildSchemaParityCheck(primarySchema, replicaSchema, primaryHost, replicaHost))
	checksList = append(checksList, buildDebeziumCheck(cdcStatus, plan.CDC.Connector))
	return checksList
}

func buildSchemaParityCheck(primarySchema string, replicaSchema string, primaryHost string, replicaHost string) checks.PreflightCheck {
	return &checks.SchemaParityCheck{
		Inspector:   &schemaFileInspector{primaryPath: primarySchema, replicaPath: replicaSchema, primaryHost: primaryHost, replicaHost: replicaHost},
		PrimaryHost: primaryHost,
		ReplicaHost: replicaHost,
	}
}

func buildDebeziumCheck(statusPath string, connector string) checks.PreflightCheck {
	return &cdc.DebeziumHealthCheck{
		Inspector: &debeziumFileInspector{path: statusPath},
		Connector: connector,
	}
}

func convertCheckResults(summary checks.Summary, results []checks.Result) Output {
	findings := []OutputFinding{}
	for _, r := range results {
		for _, f := range r.Findings {
			findings = append(findings, OutputFinding{Severity: f.Severity.String(), Message: f.Message, Meta: f.Meta})
		}
	}
	return Output{Summary: Summary{Info: summary.Info, Warn: summary.Warn, Block: summary.Block}, Findings: findings}
}

func convertCheckSummary(summary checks.Summary, findings []checks.Finding) Output {
	return Output{Summary: Summary{Info: summary.Info, Warn: summary.Warn, Block: summary.Block}, Findings: convertCheckFindings(findings).Findings}
}

func convertCheckFindings(findings []checks.Finding) Output {
	outs := []OutputFinding{}
	var summary Summary
	for _, f := range findings {
		outs = append(outs, OutputFinding{Severity: f.Severity.String(), Message: f.Message, Meta: f.Meta})
		switch f.Severity {
		case checks.SeverityInfo:
			summary.Info++
		case checks.SeverityWarn:
			summary.Warn++
		case checks.SeverityBlock:
			summary.Block++
		}
	}
	return Output{Summary: summary, Findings: outs}
}

func convertMySQLFindings(summary mysql.Summary, findings []mysql.Finding) Output {
	outs := []OutputFinding{}
	for _, f := range findings {
		outs = append(outs, OutputFinding{Severity: f.Severity.String(), Message: f.Message, Meta: f.Meta})
	}
	return Output{Summary: Summary{Info: summary.Info, Warn: summary.Warn, Block: summary.Block}, Findings: outs}
}

func writeOutput(output Output) {
	b, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		log.Fatalf("failed to encode output: %v", err)
	}
	_, _ = os.Stdout.Write(b)
	_, _ = os.Stdout.Write([]byte("\n"))
}

func printUsageAndExit() {
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  migratorx plan --plan migration.yaml")
	fmt.Fprintln(os.Stderr, "  migratorx preflight --plan migration.yaml [--schema-primary path --schema-replica path --cdc-status path]")
	fmt.Fprintln(os.Stderr, "  migratorx upgrade replica <name> --plan migration.yaml [--state path --simulate --io-running --sql-running]")
	fmt.Fprintln(os.Stderr, "  migratorx validate replica <name> --plan migration.yaml --schema-primary path --schema-replica path")
	fmt.Fprintln(os.Stderr, "  migratorx validate primary --plan migration.yaml --schema-primary path --schema-replica path")
	fmt.Fprintln(os.Stderr, "  migratorx cdc check --plan migration.yaml --cdc-status path")
	fmt.Fprintln(os.Stderr, "  migratorx promote --plan migration.yaml --confirm PROMOTE [--phrase PROMOTE] --schema-primary path --schema-replica path --cdc-status path")
	os.Exit(1)
}

func defaultStatePath() string {
	return filepath.Join(".", ".migratorx", "state.json")
}

type schemaFileInspector struct {
	primaryPath string
	replicaPath string
	primaryHost string
	replicaHost string
}

func (s *schemaFileInspector) Schema(ctx context.Context, host string) (checks.Schema, error) {
	var path string
	if host == "" {
		return checks.Schema{}, errors.New("host is required")
	}
	if s.primaryHost != "" && host == s.primaryHost {
		path = s.primaryPath
	} else if s.replicaHost != "" && host == s.replicaHost {
		path = s.replicaPath
	} else if s.primaryHost == "" && s.replicaHost == "" {
		path = s.primaryPath
	}
	if path == "" {
		return checks.Schema{}, fmt.Errorf("schema file path required for host %q", host)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return checks.Schema{}, err
	}
	var schema checks.Schema
	if err := json.Unmarshal(b, &schema); err != nil {
		return checks.Schema{}, err
	}
	return schema, nil
}

type debeziumFileInspector struct {
	path string
}

func (d *debeziumFileInspector) ConnectorStatus(ctx context.Context, connector string) (cdc.ConnectorStatus, error) {
	if d.path == "" {
		return cdc.ConnectorStatus{}, fmt.Errorf("cdc status file path is required")
	}
	b, err := os.ReadFile(d.path)
	if err != nil {
		return cdc.ConnectorStatus{}, err
	}
	var status cdc.ConnectorStatus
	if err := json.Unmarshal(b, &status); err != nil {
		return cdc.ConnectorStatus{}, err
	}
	if status.Name == "" {
		status.Name = connector
	}
	return status, nil
}

type staticReplicaInspector struct {
	isPrimary bool
	status    mysql.ReplicationStatus
}

func (s *staticReplicaInspector) IsPrimary(ctx context.Context, host string) (bool, error) {
	return s.isPrimary, nil
}

func (s *staticReplicaInspector) ReplicationStatus(ctx context.Context, replica string) (mysql.ReplicationStatus, error) {
	return s.status, nil
}

type notConfiguredActions struct{}

func (n *notConfiguredActions) StopReplication(ctx context.Context, replica string) error {
	return fmt.Errorf("replica actions not configured; use --simulate or provide implementation")
}

func (n *notConfiguredActions) RunUpgrade(ctx context.Context, replica string) error {
	return fmt.Errorf("replica actions not configured; use --simulate or provide implementation")
}

func (n *notConfiguredActions) StartReplication(ctx context.Context, replica string) error {
	return fmt.Errorf("replica actions not configured; use --simulate or provide implementation")
}

type simulatedActions struct{}

func (s *simulatedActions) StopReplication(ctx context.Context, replica string) error  { return nil }
func (s *simulatedActions) RunUpgrade(ctx context.Context, replica string) error       { return nil }
func (s *simulatedActions) StartReplication(ctx context.Context, replica string) error { return nil }

func selectReplica(plan workflow.MigrationPlan) (string, error) {
	if len(plan.Topology.Replicas) == 0 {
		return "", fmt.Errorf("no replicas defined in plan")
	}
	return plan.Topology.Replicas[0], nil
}

func planInput(plan workflow.MigrationPlan, replicaHost string) checks.Input {
	return checks.Input{
		PlanSourceVersion: plan.SourceVersion,
		PlanTargetVersion: plan.TargetVersion,
		PrimaryHost:       plan.Topology.Primary,
		ReplicaHost:       replicaHost,
		CDCConnector:      plan.CDC.Connector,
	}
}

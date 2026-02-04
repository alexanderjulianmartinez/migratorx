# MigratorX
MigratorX is a safety-first orchestration tool for MySQL major version upgrades.

It helps teams plan, validate, and execute upgrades like MySQL 5.7 → 8.0 with confidence—especially in environments running CDC (e.g., Debezium)—by surfacing risk before it becomes production impact.

MigratorX treats upgrades as workflows, not commands.

## Why MigratorX Exists

MySQL major upgrades are deceptively risky:
- Configuration incompatibilities
- Deprecated or removed features
- Silent schema changes
- Replication instability
- CDC connectors that appear RUNNING but are actually broken

Most failures don’t happen during the upgrade—they happen after, when downstream systems quietly stop working.

MigratorX exists to make these risks visible, gated, and reversible.

## What MigratorX Is (v1)

- A workflow orchestrator for MySQL major version upgrades
- CDC-aware (Debezium / Kafka)
- Replica-first, safety-oriented
- Human-in-the-loop for promotion
- Designed to be idempotent and observable

## What MigratorX Is Not

- An online schema change tool (use gh-ost / pt-osc)
- An app-level migration framework
- Fully automated cutover
- Cloud-provider-specific automation

MigratorX optimizes for predictability and trust, not speed.

## Supported Use Case (v1)
- MySQL 5.7 → 8.0 upgrades
- Primary + replica topologies
- Debezium-based CDC pipelines
- Teams that want preflight validation and post-cutover confidence

## How It Works

MigratorX executes a declarative migration plan as a series of gated steps.

Example Migration Plan (YAML)
``` yaml
migration: mysql_57_to_80
source_version: 5.7
target_version: 8.0

topology:
  primary: mysql-primary
  replicas:
    - mysql-replica-1

cdc:
  type: debezium
  connector: mysql-prod

steps:
  - preflight
  - upgrade_replica
  - validate_replica
  - cdc_check
  - promote
  - post_validation
```
Each step:
- Is idempotent
- Produces structured results (`INFO` / `WARN` / `BLOCK`)
- Prevents unsafe progression

## Core Capabilities

### 1. Preflight Checks

Detects issues before any upgrade begins:
- MySQL version incompatibilities
- Deprecated config options
- Charset and collation risks
- Missing primary keys (CDC risk)
- Engine and schema invariants

### 2. Replica Upgrade Rehearsal
- Upgrades replicas first
- Safely stops and resumes replication
- Observes lag and recovery
- Never touches the primary in v1

### 3. Schema & Data Validation
- Schema parity checks
- Primary key invariants
- Row count sampling
- System table differences

### 4. CDC Safety Validation

CDC is treated as a first-class dependency.

MigratorX validates:
- Connector reachability
- Task health (`RUNNING` vs `FAILED`)
- Restart loop detection
- Kafka schema history availability
- Table coverage parity

Failures here block promotion.

### 5. Controlled Promotion

MigratorX never auto-promotes.

`migratorx promote mysql-replica-1`

This explicit step is intentional and required.

## CLI Overview

- `migratorx plan migration.yaml`
- `migratorx preflight`
- `migratorx upgrade replica mysql-replica-1`
- `migratorx validate replica mysql-replica-1`
- `migratorx cdc check`
- `migratorx promote mysql-replica-1`
- `migratorx validate primary`

All commands are safe to re-run.

## Output Model

All checks emit structured results:
- `INFO` – informational
- `WARN` – risky but non-blocking
- `BLOCK` – unsafe to proceed

Example:

`Summary: 0 INFO / 3 WARN / 1 BLOCK`

A `BLOCK` always prevents the next step.

## Relationship to DataWatch

MigratorX builds on similar inspection and validation concepts as DataWatch, but focuses on workflow orchestration rather than standalone drift detection.

The two tools are complementary.

## Project Status

🚧 Early development (v1)
The current focus is correctness, clarity, and real-world safety—not feature breadth.

## Contributing

Feedback, issues, and design discussion are welcome.
Code contributions will be considered once v1 stabilizes.

## Philosophy

“If an upgrade can’t explain why it’s safe, it isn’t.”

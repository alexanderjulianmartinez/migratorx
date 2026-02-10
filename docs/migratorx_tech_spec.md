# MigratorX v1 — Technical Specification (Context Doc)

## 1. Purpose & Scope

MigratorX is a safety-first orchestration tool for MySQL major version upgrades (e.g., 5.7 → 8.0).

It does not perform schema changes or online DDL.
It coordinates, validates, and gates the upgrade process to reduce risk and prevent silent failures—especially in CDC-enabled systems.

### In Scope
    •    MySQL major version upgrades
    •    Primary / replica topologies
    •    CDC-aware validation (Debezium / Kafka)
    •    Human-in-the-loop promotion
    •    Idempotent, resumable steps

### Out of Scope
    •    Online schema changes (gh-ost, pt-osc)
    •    App-level migrations
    •    Fully automated cutovers
    •    Cloud-provider specific automation

⸻

## 2. Design Principles
    1.    Upgrades are workflows, not commands
    2.    Detect risk before mutation
    3.    Never auto-promote primaries
    4.    CDC failures are first-class blockers
    5.    Every step is observable and repeatable

⸻

## 3. User Persona
    •    Staff+ backend / infra engineers
    •    Migration owners for MySQL fleets
    •    Teams running Debezium / CDC pipelines
    •    Engineers who want predictability, not magic

⸻

## 4. High-Level Architecture

cmd/migratorx
internal/
  workflow/        # Plan parsing + step execution
  mysql/           # Inspection, upgrade helpers
  checks/          # Preflight, parity, invariants
  cdc/             # Debezium + Kafka validation
  state/           # Migration state + checkpoints

MigratorX is stateless by default; progress is derived from:
    •    MySQL state
    •    CDC state
    •    Migration plan definition

⸻

## 5. Migration Plan Model

Upgrades are defined declaratively.

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

Steps are:
    •    Ordered
    •    Idempotent
    •    Safe to re-run

⸻

## 6. CLI Interface (v1)

migratorx plan migration.yaml
migratorx preflight
migratorx upgrade replica mysql-replica-1
migratorx validate replica mysql-replica-1
migratorx cdc check
migratorx promote mysql-replica-1
migratorx validate primary

Each command:
    •    Produces structured output
    •    Emits WARN / BLOCK states
    •    Never mutates state without explicit intent

⸻

## 7. Core Functional Areas

### 7.1 Preflight Checks

Run before any upgrade:
    •    MySQL version compatibility
    •    Deprecated features (5.7 → 8.0)
    •    Charset / collation risks
    •    SQL mode incompatibilities
    •    Engine validation
    •    Missing primary keys (CDC risk)

Failures block all upgrade steps.

⸻

### 7.2 Replica Upgrade
    •    Target replicas only
    •    Stop replication safely
    •    Perform upgrade steps
    •    Resume replication
    •    Observe lag and recovery

No primary upgrades in v1.

⸻

### 7.3 Schema & Data Validation
    •    Schema parity (pre vs post)
    •    Primary key invariants
    •    Row count sampling
    •    System table differences

Used both pre-promotion and post-cutover.

⸻

### 7.4 CDC Safety Validation (Critical)

CDC is treated as a first-class dependency.

Checks include:
    •    Connector reachability
    •    Task health (RUNNING vs FAILED)
    •    Restart loop detection
    •    Schema history topic health
    •    Table coverage parity

Any CDC failure blocks promotion.

⸻

### 7.5 Promotion (Manual Gate)

MigratorX prepares everything but does not auto-cutover.

migratorx promote mysql-replica-1

This is a deliberate trust-building design choice.

⸻

## 8. Output & Severity Model

All checks produce structured findings:
    •    INFO — informational
    •    WARN — risky but non-blocking
    •    BLOCK — upgrade cannot proceed

Example:

Summary: 2 INFO / 1 WARN / 1 BLOCK

BLOCK always prevents next step execution.

⸻

## 9. Relationship to DataWatch

MigratorX may:
    •    Reuse DataWatch logic (schema, PK, CDC checks)
    •    Or vendor shared inspection utilities

MigratorX focuses on workflow orchestration, not deep inspection logic.

⸻

## 10. Non-Goals (Explicit)
    •    Auto-fixing schema problems
    •    Abstracting MySQL internals
    •    Competing with managed cloud tools
    •    Handling multi-primary topologies (v1)

⸻

## 11. v1 Success Criteria

MigratorX v1 is successful if it:
    •    Prevents unsafe MySQL major upgrades
    •    Surfaces CDC failures before cutover
    •    Is trusted enough to be run in prod read-only mode
    •    Feels boring, predictable, and safe

⸻

## 12. Tone for AI Assistance

When generating code or suggestions:
    •    Prefer correctness over cleverness
    •    Prefer explicit state checks
    •    Avoid hidden side effects
    •    Treat failures as signals, not exceptions

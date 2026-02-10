package workflow

import (
	"fmt"
	"strings"
)

// SupportedSteps defines the canonical step order for migration plans.
var SupportedSteps = []string{
	"preflight",
	"upgrade_replica",
	"validate_replica",
	"cdc_check",
	"promote",
	"post_validation",
}

// MigrationPlan models the declarative migration plan (Section 5).
type MigrationPlan struct {
	Migration     string    `yaml:"migration"`
	SourceVersion string    `yaml:"source_version"`
	TargetVersion string    `yaml:"target_version"`
	Topology      Topology  `yaml:"topology"`
	CDC           CDCConfig `yaml:"cdc"`
	Steps         []string  `yaml:"steps"`
}

// Topology models primary/replica relationships.
type Topology struct {
	Primary  string   `yaml:"primary"`
	Replicas []string `yaml:"replicas"`
}

// CDCConfig models CDC settings.
type CDCConfig struct {
	Type      string `yaml:"type"`
	Connector string `yaml:"connector"`
}

// Validate enforces required fields, supported step names, and valid step ordering.
func (p MigrationPlan) Validate() error {
	var problems []string

	if strings.TrimSpace(p.Migration) == "" {
		problems = append(problems, "migration is required")
	}
	if strings.TrimSpace(p.SourceVersion) == "" {
		problems = append(problems, "source_version is required")
	}
	if strings.TrimSpace(p.TargetVersion) == "" {
		problems = append(problems, "target_version is required")
	}

	if strings.TrimSpace(p.Topology.Primary) == "" {
		problems = append(problems, "topology.primary is required")
	}
	if len(p.Topology.Replicas) == 0 {
		problems = append(problems, "topology.replicas must include at least one replica")
	} else {
		for i, r := range p.Topology.Replicas {
			if strings.TrimSpace(r) == "" {
				problems = append(problems, fmt.Sprintf("topology.replicas[%d] is empty", i))
			}
		}
	}

	if strings.TrimSpace(p.CDC.Type) == "" {
		problems = append(problems, "cdc.type is required")
	}
	if strings.TrimSpace(p.CDC.Connector) == "" {
		problems = append(problems, "cdc.connector is required")
	}

	if len(p.Steps) == 0 {
		problems = append(problems, "steps must include at least one step")
	} else {
		stepOrder := supportedStepOrder()
		seen := map[string]struct{}{}
		lastPos := -1
		for i, step := range p.Steps {
			step = strings.TrimSpace(step)
			if step == "" {
				problems = append(problems, fmt.Sprintf("steps[%d] is empty", i))
				continue
			}
			pos, ok := stepOrder[step]
			if !ok {
				problems = append(problems, fmt.Sprintf("steps[%d]=%q is not supported", i, step))
				continue
			}
			if _, exists := seen[step]; exists {
				problems = append(problems, fmt.Sprintf("steps[%d]=%q is duplicated", i, step))
				continue
			}
			if pos < lastPos {
				problems = append(problems, fmt.Sprintf("step order invalid at steps[%d]=%q", i, step))
				continue
			}
			seen[step] = struct{}{}
			lastPos = pos
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("migration plan validation failed: %s", strings.Join(problems, "; "))
	}
	return nil
}

func supportedStepOrder() map[string]int {
	order := make(map[string]int, len(SupportedSteps))
	for i, s := range SupportedSteps {
		order[s] = i
	}
	return order
}

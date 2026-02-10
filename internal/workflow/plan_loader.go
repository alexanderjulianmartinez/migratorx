package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadPlan reads a YAML migration plan from disk and validates it.
func LoadPlan(path string) (MigrationPlan, error) {
	var plan MigrationPlan
	if path == "" {
		return plan, fmt.Errorf("plan path is required")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return plan, err
	}
	if err := yaml.Unmarshal(b, &plan); err != nil {
		return plan, err
	}
	if err := plan.Validate(); err != nil {
		return plan, err
	}
	return plan, nil
}
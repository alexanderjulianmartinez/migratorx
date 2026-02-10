package cdc

import (
	"context"
	"fmt"
	"strings"

	"migratorx/internal/checks"
)

// KafkaInspector provides read-only access to Kafka topics and coverage metadata.
type KafkaInspector interface {
	TopicExists(ctx context.Context, topic string) (bool, error)
	TopicReadable(ctx context.Context, topic string) (bool, error)
	SchemaHistoryTables(ctx context.Context, topic string) ([]string, error)
}

// SchemaHistoryCheck validates Kafka schema history topic health and coverage.
type SchemaHistoryCheck struct {
	Inspector     KafkaInspector
	Topic         string
	ExpectedTables []string
}

func (c *SchemaHistoryCheck) Name() string   { return "cdc_schema_history" }
func (c *SchemaHistoryCheck) ReadOnly() bool { return true }

func (c *SchemaHistoryCheck) Run(ctx context.Context, input checks.Input) ([]checks.Finding, error) {
	if c.Inspector == nil {
		return nil, fmt.Errorf("kafka inspector is required")
	}
	if strings.TrimSpace(c.Topic) == "" {
		return nil, fmt.Errorf("schema history topic is required")
	}

	exists, err := c.Inspector.TopicExists(ctx, c.Topic)
	if err != nil {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("failed to check schema history topic %q: %v", c.Topic, err),
			Meta:     map[string]interface{}{"topic": c.Topic},
		}}, nil
	}
	if !exists {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("schema history topic %q is missing", c.Topic),
			Meta:     map[string]interface{}{"topic": c.Topic},
		}}, nil
	}

	readable, err := c.Inspector.TopicReadable(ctx, c.Topic)
	if err != nil {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("failed to read schema history topic %q: %v", c.Topic, err),
			Meta:     map[string]interface{}{"topic": c.Topic},
		}}, nil
	}
	if !readable {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("schema history topic %q is not readable", c.Topic),
			Meta:     map[string]interface{}{"topic": c.Topic},
		}}, nil
	}

	coveredTables, err := c.Inspector.SchemaHistoryTables(ctx, c.Topic)
	if err != nil {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("failed to read schema history coverage for %q: %v", c.Topic, err),
			Meta:     map[string]interface{}{"topic": c.Topic},
		}}, nil
	}

	missing := missingTables(c.ExpectedTables, coveredTables)
	if len(missing) > 0 {
		return []checks.Finding{{
			Severity: checks.SeverityBlock,
			Message:  fmt.Sprintf("schema history missing tables: %s", strings.Join(missing, ", ")),
			Meta:     map[string]interface{}{"topic": c.Topic, "missing_tables": missing},
		}}, nil
	}

	return []checks.Finding{{
		Severity: checks.SeverityInfo,
		Message:  fmt.Sprintf("schema history topic %q is healthy", c.Topic),
		Meta:     map[string]interface{}{"topic": c.Topic},
	}}, nil
}

func missingTables(expected []string, covered []string) []string {
	if len(expected) == 0 {
		return nil
	}
	coveredSet := map[string]struct{}{}
	for _, t := range covered {
		coveredSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}
	missing := []string{}
	for _, t := range expected {
		key := strings.ToLower(strings.TrimSpace(t))
		if key == "" {
			continue
		}
		if _, ok := coveredSet[key]; !ok {
			missing = append(missing, t)
		}
	}
	return missing
}
package rules

import (
	"fmt"
)

// AlertStore provides implementation for persistent storage of active alerts
// for alerting rules.
type AlertStore interface {
	// SetAlerts stores the provided list of alerts for a rule
	SetAlerts(groupKey string, rule string, ruleOrder int, alerts []*Alert) error
	// GetAlerts returns a list of alerts for each rule in group,
	// rule is identified by name and its order in the group.
	GetAlerts(groupKey string) (map[string][]*Alert, error)
	// Close releases the underlying resources of the store.
	Close()
}

// no-op implementation of AlertStore interface
// used by default when feature is disabled.
type noopStore struct{}

func (d noopStore) SetAlerts(_ string, _ string, _ int, _ []*Alert) error {
	return fmt.Errorf("feature disabled")
}

func (d noopStore) GetAlerts(_ string) (map[string][]*Alert, error) {
	return nil, fmt.Errorf("feature disabled")
}

func (d noopStore) Close() {}

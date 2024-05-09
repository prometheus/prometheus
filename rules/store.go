// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rules

import (
	"fmt"
)

// AlertStore provides implementation for persistent storage of active alerts
// for alerting rules.
type AlertStore interface {
	// SetAlerts stores the provided list of alerts for a rule
	SetAlerts(groupKey, rule string, ruleOrder int, alerts []*Alert) error
	// GetAlerts returns a list of alerts for each rule in group,
	// rule is identified by name and its order in the group.
	GetAlerts(groupKey string) (map[string][]*Alert, error)
	// Close releases the underlying resources of the store.
	Close()
}

// no-op implementation of AlertStore interface
// used by default when feature is disabled.
type NoopStore struct{}

func (d NoopStore) SetAlerts(_, _ string, _ int, _ []*Alert) error {
	return fmt.Errorf("feature disabled")
}

func (d NoopStore) GetAlerts(_ string) (map[string][]*Alert, error) {
	return nil, fmt.Errorf("feature disabled")
}

func (d NoopStore) Close() {}

// Copyright The Prometheus Authors
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

package notifier

import (
	"fmt"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

// Alert is a generic representation of an alert in the Prometheus eco-system.
type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels labels.Labels `json:"labels"`

	// Extra key/value information which does not define alert identity.
	Annotations labels.Labels `json:"annotations"`

	// The known time range for this alert. Both ends are optional.
	StartsAt     time.Time `json:"startsAt,omitempty"`
	EndsAt       time.Time `json:"endsAt,omitempty"`
	GeneratorURL string    `json:"generatorURL,omitempty"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return a.Labels.Get(labels.AlertName)
}

// Hash returns a hash over the alert. It is equivalent to the alert labels hash.
func (a *Alert) Hash() uint64 {
	return a.Labels.Hash()
}

func (a *Alert) String() string {
	s := fmt.Sprintf("%s[%s]", a.Name(), fmt.Sprintf("%016x", a.Hash())[:7])
	if a.Resolved() {
		return s + "[resolved]"
	}
	return s + "[active]"
}

// Resolved returns true iff the activity interval ended in the past.
func (a *Alert) Resolved() bool {
	return a.ResolvedAt(time.Now())
}

// ResolvedAt returns true iff the activity interval ended before
// the given timestamp.
func (a *Alert) ResolvedAt(ts time.Time) bool {
	if a.EndsAt.IsZero() {
		return false
	}
	return !a.EndsAt.After(ts)
}

func relabelAlerts(relabelConfigs []*relabel.Config, externalLabels labels.Labels, alerts []*Alert) []*Alert {
	lb := labels.NewBuilder(labels.EmptyLabels())
	var relabeledAlerts []*Alert

	for _, a := range alerts {
		lb.Reset(a.Labels)
		externalLabels.Range(func(l labels.Label) {
			if a.Labels.Get(l.Name) == "" {
				lb.Set(l.Name, l.Value)
			}
		})

		keep := relabel.ProcessBuilder(lb, relabelConfigs...)
		if !keep {
			continue
		}

		// If relabeling has altered the labels, create a new Alert to preserve immutability.
		if !labels.Equal(a.Labels, lb.Labels()) {
			a = &Alert{
				Labels:       lb.Labels(),
				Annotations:  a.Annotations,
				StartsAt:     a.StartsAt,
				EndsAt:       a.EndsAt,
				GeneratorURL: a.GeneratorURL,
			}
		}

		relabeledAlerts = append(relabeledAlerts, a)
	}
	return relabeledAlerts
}

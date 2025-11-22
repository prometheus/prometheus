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
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

// createAlerts generates a slice of test alerts with the specified count.
func createAlerts(alertCount, labelCount, annotationCount int) []*Alert {
	alerts := make([]*Alert, alertCount)
	now := time.Now()

	for i := 0; i < alertCount; i++ {
		// Create label pairs
		labelPairs := make([]string, 0, labelCount*2)
		labelPairs = append(labelPairs, "alertname", "TestAlert")
		for j := 1; j < labelCount; j++ {
			labelPairs = append(labelPairs, "label"+string(rune('a'+j)), "value"+string(rune('0'+j)))
		}

		// Create annotation pairs
		annotationPairs := make([]string, 0, annotationCount*2)
		for j := 0; j < annotationCount; j++ {
			annotationPairs = append(annotationPairs, "annotation"+string(rune('a'+j)), "description"+string(rune('0'+j)))
		}

		alerts[i] = &Alert{
			Labels:       labels.FromStrings(labelPairs...),
			Annotations:  labels.FromStrings(annotationPairs...),
			StartsAt:     now,
			EndsAt:       now.Add(time.Hour),
			GeneratorURL: "http://prometheus:9090/graph?g0.expr=test",
		}
	}

	return alerts
}

func BenchmarkAlertsToOpenAPIAlerts_1Alert_SmallLabels(b *testing.B) {
	alerts := createAlerts(1, 3, 2)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_1Alert_ManyLabels(b *testing.B) {
	alerts := createAlerts(1, 20, 10)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_10Alerts_SmallLabels(b *testing.B) {
	alerts := createAlerts(10, 3, 2)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_10Alerts_ManyLabels(b *testing.B) {
	alerts := createAlerts(10, 20, 10)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_100Alerts_SmallLabels(b *testing.B) {
	alerts := createAlerts(100, 3, 2)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_100Alerts_ManyLabels(b *testing.B) {
	alerts := createAlerts(100, 20, 10)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_1000Alerts_SmallLabels(b *testing.B) {
	alerts := createAlerts(1000, 3, 2)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_1000Alerts_ManyLabels(b *testing.B) {
	alerts := createAlerts(1000, 20, 10)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

func BenchmarkAlertsToOpenAPIAlerts_EmptySlice(b *testing.B) {
	alerts := []*Alert{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = alertsToOpenAPIAlerts(alerts)
	}
}

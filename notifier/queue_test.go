// Copyright 2025 The Prometheus Authors
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

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestPushAlertsToQueue(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "existing1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "existing2")}

	// Initialize a queue with capacity 5 and 2 existing alerts
	q := newQueue(5, 100)
	q.alerts = append(q.alerts, alert1, alert2)

	// Create some test alerts
	alert3 := &Alert{Labels: labels.FromStrings("alertname", "new1")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "new2")}
	alerts := []*Alert{alert3, alert4}

	// Push new alerts to queue, expect 0 dropped
	require.Zero(t, q.push(alerts), "Expected 0 dropped alerts")

	// Verify all new alerts were added to the queue
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4}, q.alerts, "Expected 4 alerts in the queue")
}

// Pushing alerts exceeding queue capacity should drop oldest alerts.
func TestPushAlertsToQueueExceedingCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	// Initialize a queue with capacity 3
	q := newQueue(3, 10)
	q.alerts = append(q.alerts, alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alerts := []*Alert{alert3, alert4}

	// Push new alerts to queue, expect 1 dropped
	require.Equal(t, 1, q.push(alerts), "Expected 1 dropped alerts")

	// Verify all new alerts were added to the queue
	require.Equal(t, []*Alert{alert2, alert3, alert4}, q.alerts, "Expected 3 alerts in the queue")
}

// Pushing alerts exceeding total queue capacity should drop alerts from both old and new.
func TestPushAlertsToQueueExceedingTotalCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	// Initialize a queue with capacity 3
	q := newQueue(3, 10)
	q.alerts = append(q.alerts, alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	alert6 := &Alert{Labels: labels.FromStrings("alertname", "alert6")}
	alerts := []*Alert{alert3, alert4, alert5, alert6}

	// Push new alerts to queue, expect 3 dropped: 1 from new batch + 2 from existing queued items
	require.Equal(t, 3, q.push(alerts), "Expected 3 dropped alerts")

	// Verify all new alerts were added to the queue
	require.Equal(t, []*Alert{alert4, alert5, alert6}, q.alerts, "Expected 3 alerts in the queue")
}

func TestPopAlertsFromQueue(t *testing.T) {
	// Initialize a queue with capacity = maxBatchSize = 5
	q := newQueue(5, 5)

	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}
	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}

	// Test 3 alerts in the queue
	q.alerts = []*Alert{alert1, alert2, alert3}
	result1 := q.pop()
	require.Equal(t, []*Alert{alert1, alert2, alert3}, result1, "Expected all 3 alerts")
	require.Empty(t, q.alerts, "expected empty queue")

	// Test full queue
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	q.alerts = []*Alert{alert1, alert2, alert3, alert4, alert5}
	result2 := q.pop()
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4, alert5}, result2, "Expected all 5 alerts")
	require.Empty(t, q.alerts, "expected empty queue")

	// Test smaller maxBatchSize than capacity
	q.maxBatchSize = 3
	q.alerts = []*Alert{alert1, alert2, alert3, alert4, alert5}
	result3 := q.pop()
	require.Equal(t, []*Alert{alert1, alert2, alert3}, result3, "Expected 3 first alerts from queue")
	require.Len(t, q.alerts, 2, "expected 2 alerts in queue")

	// Pop the remaining 2 alerts in queue
	result4 := q.pop()
	require.Equal(t, []*Alert{alert4, alert5}, result4, "Expected 2 last alerts from queue")
	require.Empty(t, q.alerts, "expected empty queue")
}

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

func TestPushAlertsToBuffer(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "existing1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "existing2")}

	// Initialize a buffer with capacity 5 and 2 existing alerts
	b := newBuffer(5)
	b.push(alert1, alert2)
	require.Equal(t, []*Alert{alert1, alert2, nil, nil, nil}, b.data)
	require.Equal(t, 2, b.len(), "Expected buffer length of 2")

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "new1")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "new2")}

	// Push new alerts to buffer, expect 0 dropped
	require.Zero(t, b.push(alert3, alert4), "Expected 0 dropped alerts")

	// Verify all new alerts were added to the buffer
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4, nil}, b.data)
	require.Equal(t, 4, b.len(), "Expected buffer length of 4")
}

// Pushing alerts exceeding buffer capacity should drop oldest alerts.
func TestPushAlertsToBufferExceedingCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	// Initialize a buffer with capacity 3
	b := newBuffer(3)
	b.push(alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}

	// Push new alerts to buffer, expect 1 dropped
	require.Equal(t, 1, b.push(alert3, alert4), "Expected 1 dropped alerts")

	// Verify all new alerts were added to the buffer, alert4 will be at the beginning of buffer, overwritten alert1
	require.Equal(t, []*Alert{alert4, alert2, alert3}, b.data, "Expected 3 alerts in the buffer")
}

// Pushing alerts exceeding total buffer capacity should drop alerts from both old and new.
func TestPushAlertsToBufferExceedingTotalCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	// Initialize a buffer with capacity 3
	b := newBuffer(3)
	b.push(alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	alert6 := &Alert{Labels: labels.FromStrings("alertname", "alert6")}

	// Push new alerts to buffer, expect 3 dropped: 1 from new batch + 2 from existing bufferd items
	require.Equal(t, 3, b.push(alert3, alert4, alert5, alert6), "Expected 3 dropped alerts")

	// Verify all new alerts were added to the buffer
	require.Equal(t, []*Alert{alert4, alert5, alert6}, b.data, "Expected 3 alerts in the buffer")
}

func TestPopAlertsFromBuffer(t *testing.T) {
	// Initialize a buffer with capacity 5
	b := newBuffer(5)

	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}
	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	b.push(alert1, alert2, alert3)

	// Test 3 alerts in the buffer
	result1 := make([]*Alert, 3)
	b.pop(&result1)
	require.Equal(t, []*Alert{alert1, alert2, alert3}, result1, "Expected all 3 alerts")
	require.Equal(t, []*Alert{nil, nil, nil, nil, nil}, b.data, "Expected buffer with nil elements")

	// Test full buffer
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	b.push(alert1, alert2, alert3, alert4, alert5)
	result2 := make([]*Alert, 5)
	b.pop(&result2)
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4, alert5}, result2, "Expected all 5 alerts")
	require.Equal(t, []*Alert{nil, nil, nil, nil, nil}, b.data, "Expected buffer with nil elements")
	require.Zero(t, b.len(), "Expected buffer length of 0")

	// Test smaller max size than capacity
	b.push(alert1, alert2, alert3, alert4, alert5)
	result3 := make([]*Alert, 3)
	b.pop(&result3)
	require.Equal(t, []*Alert{alert1, alert2, alert3}, result3, "Expected 3 first alerts from buffer")
	require.Equal(t, 2, b.len(), "Expected buffer length of 2")

	// Pop the remaining 2 alerts in buffer
	result4 := make([]*Alert, 3)
	b.pop(&result4)
	require.Equal(t, []*Alert{alert4, alert5}, result4, "Expected 2 last alerts from buffer")
	require.Equal(t, []*Alert{nil, nil, nil, nil, nil}, b.data, "Expected buffer with nil elements")
	require.Zero(t, b.len(), "Expected buffer length of 0")
}

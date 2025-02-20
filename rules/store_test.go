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

package rules

import (
	"os"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestAlertStore(t *testing.T) {
	alertStore := NewFileStore(promslog.NewNopLogger(), "alertstoretest", prometheus.NewRegistry())
	t.Cleanup(func() {
		os.Remove("alertstoretest")
	})

	alertsByRule := make(map[uint64][]*Alert)
	baseTime := time.Now()
	al1 := &Alert{State: StateFiring, Labels: labels.FromStrings("a1", "1"), Annotations: labels.FromStrings("annotation1", "a1"), ActiveAt: baseTime, KeepFiringSince: baseTime}
	al2 := &Alert{State: StateFiring, Labels: labels.FromStrings("a2", "2"), Annotations: labels.FromStrings("annotation2", "a2"), ActiveAt: baseTime, KeepFiringSince: baseTime}

	alertsByRule[1] = []*Alert{al1, al2}
	alertsByRule[2] = []*Alert{al2}
	alertsByRule[3] = []*Alert{al1}
	alertsByRule[4] = []*Alert{}

	for key, alerts := range alertsByRule {
		sortAlerts(alerts)
		err := alertStore.SetAlerts(key, "test/test1", alerts)
		require.NoError(t, err)

		got, err := alertStore.GetAlerts(key)
		require.NoError(t, err)
		require.Len(t, got, len(alerts))

		result := make([]*Alert, 0, len(got))
		for _, value := range got {
			result = append(result, value)
		}
		sortAlerts(result)

		j := 0
		for _, al := range result {
			require.Equal(t, alerts[j].State, al.State)
			require.Equal(t, alerts[j].Labels, al.Labels)
			require.Equal(t, alerts[j].Annotations, al.Annotations)
			require.Equal(t, alerts[j].ActiveAt, al.ActiveAt)
			require.Equal(t, alerts[j].KeepFiringSince, al.KeepFiringSince)
			j++
		}
	}
}

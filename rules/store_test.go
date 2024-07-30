package rules

import (
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestAlertStore(t *testing.T) {
	alertStore := NewFileStore(log.NewNopLogger(), "alertstoretest", prometheus.NewRegistry())
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
		require.Equal(t, len(alerts), len(got))

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

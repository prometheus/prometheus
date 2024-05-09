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

package alertstore

import (
	"math"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/rules"
)

var baseTime = time.Now()

func TestBadgerDb(t *testing.T) {
	store, err := NewBadgerDB("testdata/alerts", log.NewNopLogger(), time.Minute*1)
	require.NoError(t, err, "Unable to start alertstore")
	defer func() {
		store.Close()
		os.RemoveAll("testdata")
	}()
	t.Run("store and retrieve alerts", func(t *testing.T) {
		testBadgerDB(t, store)
	})
}

type testItem struct {
	groupKey     string
	ruleName     string
	ruleOrder    int
	activeAlerts []*rules.Alert
}

func testBadgerDB(t *testing.T, store *BadgerDB) {
	storeItems := []testItem{
		{
			groupKey:  "file1;group1",
			ruleName:  "rule1",
			ruleOrder: 0,
			activeAlerts: []*rules.Alert{
				{State: rules.StateFiring, KeepFiringSince: baseTime, Labels: labels.Labels{}},
				{State: rules.StateFiring, KeepFiringSince: baseTime, Labels: labels.Labels{}},
			},
		},
		{
			groupKey:  "file1;group1",
			ruleName:  "rule1",
			ruleOrder: 1,
			activeAlerts: []*rules.Alert{
				{State: rules.StateFiring, KeepFiringSince: baseTime.Add(30 * time.Second), Labels: labels.Labels{}},
				{State: rules.StateFiring, KeepFiringSince: baseTime.Add(30 * time.Second), Labels: labels.Labels{}},
			},
		},
		{
			groupKey:  "file2;group2",
			ruleName:  "alerting_rule",
			ruleOrder: 0,
			activeAlerts: []*rules.Alert{
				{State: rules.StateFiring, KeepFiringSince: baseTime.Add(60 * time.Second), Labels: labels.Labels{}},
				{State: rules.StateFiring, KeepFiringSince: baseTime.Add(60 * time.Second), Labels: labels.Labels{}},
			},
		},
	}

	for _, record := range storeItems {
		store.SetAlerts(record.groupKey, record.ruleName, record.ruleOrder, record.activeAlerts)
	}

	// Verify both duplicate rule names in single group have their own alerts stored
	alertsByRuleKey, err := store.GetAlerts(storeItems[0].groupKey)
	require.NoError(t, err)
	require.Len(t, alertsByRuleKey, 2)

	// Verify active alerts stored for each rule
	for _, record := range storeItems {
		alertsByRuleKey, err = store.GetAlerts(record.groupKey)
		require.NoError(t, err)
		ruleKey := record.ruleName + strconv.Itoa(record.ruleOrder)
		alerts, ok := alertsByRuleKey[ruleKey]
		require.True(t, ok, "No alerts found for rule %v", ruleKey)

		sortAlerts(record.activeAlerts)
		sortAlerts(alerts)

		for i, e := range record.activeAlerts {
			diff := float64(e.KeepFiringSince.Unix() - alerts[i].KeepFiringSince.Unix())
			require.Equal(t, 0.0, math.Abs(diff), "'keep_firing_for' state restored time is wrong")
		}
	}
	// invalid key format
	alertsByRuleKey, _ = store.GetAlerts("file12")
	require.Empty(t, alertsByRuleKey)

	// Non-existent group
	alertsByRuleKey, err = store.GetAlerts("file3;group1")
	require.NoError(t, err)
	require.Empty(t, alertsByRuleKey)
}

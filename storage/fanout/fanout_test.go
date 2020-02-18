// Copyright 2020 The Prometheus Authors
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

package storage

import (
	"context"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/prometheus/prometheus/util/testutil"

	"testing"
)

func TestSelectSorted(t *testing.T) {

	inputLabel := labels.FromStrings(model.MetricNameLabel, "a")
	outputLabel := labels.FromStrings(model.MetricNameLabel, "a")

	inputTotalSize := 0

	priStorage := teststorage.New(t)
	defer priStorage.Close()
	app1 := priStorage.Appender()
	app1.Add(inputLabel, 0, 0)
	inputTotalSize++
	app1.Add(inputLabel, 1000, 1)
	inputTotalSize++
	app1.Add(inputLabel, 2000, 2)
	inputTotalSize++
	err := app1.Commit()
	testutil.Ok(t, err)

	remoteStorage1 := teststorage.New(t)
	defer remoteStorage1.Close()
	app2 := remoteStorage1.Appender()
	app2.Add(inputLabel, 3000, 3)
	inputTotalSize++
	app2.Add(inputLabel, 4000, 4)
	inputTotalSize++
	app2.Add(inputLabel, 5000, 5)
	inputTotalSize++
	err = app2.Commit()
	testutil.Ok(t, err)

	remoteStorage2 := teststorage.New(t)
	defer remoteStorage2.Close()

	app3 := remoteStorage2.Appender()
	app3.Add(inputLabel, 6000, 6)
	inputTotalSize++
	app3.Add(inputLabel, 7000, 7)
	inputTotalSize++
	app3.Add(inputLabel, 8000, 8)
	inputTotalSize++

	err = app3.Commit()
	testutil.Ok(t, err)

	fanoutStorage := storage.NewFanout(nil, priStorage, remoteStorage1, remoteStorage2)

	querier, err := fanoutStorage.Querier(context.Background(), 0, 8000)
	testutil.Ok(t, err)
	defer querier.Close()

	matcher, err := labels.NewMatcher(labels.MatchEqual, model.MetricNameLabel, "a")
	testutil.Ok(t, err)

	seriesSet, _, err := querier.SelectSorted(nil, matcher)
	testutil.Ok(t, err)

	result := make(map[int64]float64)
	var labelsResult labels.Labels
	for seriesSet.Next() {
		series := seriesSet.At()
		seriesLabels := series.Labels()
		labelsResult = seriesLabels
		iterator := series.Iterator()
		for iterator.Next() {
			timestamp, value := iterator.At()
			result[timestamp] = value
		}
	}

	testutil.Equals(t, labelsResult, outputLabel)
	testutil.Equals(t, inputTotalSize, len(result))

}

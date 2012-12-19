package leveldb

import (
	"fmt"
	"github.com/matttproud/prometheus/model"
	"github.com/matttproud/prometheus/storage/metric"
	"io/ioutil"
	"os"
	"testing"
	"time"
)

func TestGetValueAtTime(t *testing.T) {
	temporaryDirectory, _ := ioutil.TempDir("", "leveldb_metric_persistence_test")

	defer func() {
		if removeAllErr := os.RemoveAll(temporaryDirectory); removeAllErr != nil {
			t.Errorf("Could not remove temporary directory: %q\n", removeAllErr)
		}
	}()

	persistence, _ := NewLevelDBMetricPersistence(temporaryDirectory)

	defer func() {
		persistence.Close()
	}()

	m := model.Metric{
		"name": "age_in_years",
	}

	appendErr := persistence.AppendSample(&model.Sample{
		Value:     model.SampleValue(0),
		Timestamp: time.Date(1984, 3, 30, 0, 0, 0, 0, time.UTC),
		Metric:    m,
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	p := &metric.StalenessPolicy{
		AllowStale: false,
	}

	d := time.Date(1984, 3, 30, 0, 0, 0, 0, time.UTC)
	s, sErr := persistence.GetValueAtTime(&m, &d, p)

	if sErr != nil {
		t.Error(sErr)
	}

	if s == nil {
		t.Error("a sample should be returned")
	}

	if s.Value != model.SampleValue(0) {
		t.Error("an incorrect sample value was returned")
	}

	if s.Timestamp != d {
		t.Error("an incorrect timestamp for the sample was returned")
	}

	d = time.Date(1985, 3, 30, 0, 0, 0, 0, time.UTC)

	s, sErr = persistence.GetValueAtTime(&m, &d, p)

	if sErr != nil {
		t.Error(sErr)
	}

	if s != nil {
		t.Error("no sample should be returned")
	}

	d = time.Date(1983, 3, 30, 0, 0, 0, 0, time.UTC)

	s, sErr = persistence.GetValueAtTime(&m, &d, p)

	if sErr != nil {
		t.Error(sErr)
	}

	if s != nil {
		t.Error("no sample should be returned")
	}

	appendErr = persistence.AppendSample(&model.Sample{
		Value:     model.SampleValue(1),
		Timestamp: time.Date(1985, 3, 30, 0, 0, 0, 0, time.UTC),
		Metric:    m,
	})

	if appendErr != nil {
		t.Error(appendErr)
	}

	d = time.Date(1985, 3, 30, 0, 0, 0, 0, time.UTC)
	s, sErr = persistence.GetValueAtTime(&m, &d, p)

	if sErr != nil {
		t.Error(sErr)
	}

	if s == nil {
		t.Error("a sample should be returned")
	}

	if s.Value != model.SampleValue(1) {
		t.Error("an incorrect sample value was returned")
	}

	if s.Timestamp != d {
		t.Error("an incorrect timestamp for the sample was returned")
	}

	d = time.Date(1984, 9, 24, 0, 0, 0, 0, time.UTC)
	s, sErr = persistence.GetValueAtTime(&m, &d, p)

	if sErr != nil {
		t.Error(sErr)
	}

	if s == nil {
		t.Error("a sample should be returned")
	}

	if fmt.Sprintf("%f", s.Value) != model.SampleValue(0.487671).String() {
		t.Errorf("an incorrect sample value was returned: %s\n", s.Value)
	}

	if s.Timestamp != d {
		t.Error("an incorrect timestamp for the sample was returned")
	}
}

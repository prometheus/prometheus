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

package csv

import (
	"io"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/util/testutil"
)

type series struct {
	// TODO(bwplotka) Assert more stuff once supported.
	lbls    labels.Labels
	samples []sample
}

type sample struct {
	t int64
	v float64
}

func TestParser(t *testing.T) {
	for _, tcase := range []struct {
		name  string
		input string

		expectedSeries []series
	}{
		// TODO(bwplotka): Test all edge cases and all entries.
		{
			name: "empty",
		},
		{
			name:  "just header",
			input: `metric_name,label_name,label_value,timestamp_ms,label_name,label_value,help,value,type,exemplar_value,unit,exemplar_timestamp_ms`,
		},
		{
			name: "mixed header with data",
			input: `metric_name,label_name,label_value,timestamp_ms,label_name,label_value,help,value,type,exemplar_value,unit,exemplar_timestamp_ms
metric1,pod,abc-1,1594885435,instance,1,some help,1245214.23423,counter,-0.12,bytes because why not,1
metric1,pod,abc-1,1594885436,instance,1,some help,1.23423,counter,-0.12,bytes because why not,1
metric1,pod,abc-2,1594885432,,,some help2,1245214.23421,gauge,,bytes,
`,
			expectedSeries: []series{
				{
					lbls:    labels.FromStrings(labels.MetricName, "metric1", "pod", "abc-1", "instance", "1"),
					samples: []sample{{v: 1245214.23423, t: 1594885435}, {v: 1.23423, t: 1594885436}},
				},
				{
					lbls:    labels.FromStrings(labels.MetricName, "metric1", "pod", "abc-1"),
					samples: []sample{{v: 1245214.23421, t: 1594885432}},
				},
			},
		},
	} {
		t.Run(tcase.name, func(t *testing.T) {
			p := NewParser(strings.NewReader(tcase.input), ',')
			got := map[uint64]series{}
			for {
				e, err := p.Next()
				if err == io.EOF {
					break
				}
				testutil.Ok(t, err)
				// For now expects only series.
				testutil.Equals(t, textparse.EntrySeries, e)
				l := labels.Labels{}
				p.Metric(&l)

				s, ok := got[l.Hash()]
				if !ok {
					got[l.Hash()] = series{lbls: l}
				}

				_, ts, v := p.Series()
				if ts == nil {
					t.Fatal("got no timestamps")
				}
				s.samples = append(s.samples, sample{t: *ts, v: v})
			}
		})
	}
}

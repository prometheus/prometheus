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

package tsdb

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
)

func TestAddExemplar(t *testing.T) {
	es := NewCircularExemplarStorage(2)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	e := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "qwerty",
			},
		},
		Value: 0.1,
		Ts:    1,
	}

	err := es.AddExemplar(l, 0, e)
	require.NoError(t, err)
	require.Equal(t, es.index[l.String()], 0, "exemplar was not stored correctly")

	e2 := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "zxcvb",
			},
		},
		Value: 0.1,
		Ts:    2,
	}

	err = es.AddExemplar(l, 3, e2)
	require.NoError(t, err)
	require.Equal(t, es.index[l.String()], 1, "exemplar was not stored correctly, location of newest exemplar for series in index did not update")
	require.True(t, es.exemplars[es.index[l.String()]].Exemplar().Equals(e2), "exemplar was not stored correctly, expected %+v got: %+v", e2, es.exemplars[es.index[l.String()]].Exemplar())
	require.True(t, es.exemplars[es.index[l.String()]].se.scrapeTimestamp == 3, "exemplar was not stored correctly, scrape timestamp was not correct")
}

func TestAddDuplicateExemplar(t *testing.T) {
	es := NewCircularExemplarStorage(5)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	e := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "qwerty",
			},
		},
		Value: 0.1,
		HasTs: true,
		Ts:    101,
	}
	e2 := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "qwerty",
			},
		},
		Value: 0.1,
		HasTs: true,
		Ts:    102,
	}

	es.AddExemplar(l, 0, e)
	require.True(t, reflect.DeepEqual(es.exemplars[0].Exemplar(), e), "exemplar was not stored correctly")

	err := es.AddExemplar(l, 0, e2)
	require.Error(t, err, "no error when attempting to add duplicate exemplar")
	require.True(t, err == storage.ErrDuplicateExemplar, "duplicate exemplar was added")
}

func TestAddOutOfOrderExemplar(t *testing.T) {
	es := NewCircularExemplarStorage(5)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	e := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "qwerty",
			},
		},
		Value: 0.1,
		HasTs: true,
		Ts:    101,
	}
	e2 := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "qwerty",
			},
		},
		Value: 0.2,
		HasTs: true,
		Ts:    101,
	}

	es.AddExemplar(l, 0, e)
	require.True(t, reflect.DeepEqual(es.exemplars[0].Exemplar(), e), "exemplar was not stored correctly")

	err := es.AddExemplar(l, 0, e2)
	require.Error(t, err, "no error when attempting to add out of order exemplar")
	require.True(t, err == storage.ErrOutOfOrderExemplar, "out of order exemplar was added")
}

func TestAddExtraExemplar(t *testing.T) {
	es := NewCircularExemplarStorage(5)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	exemplars := []exemplar.Exemplar{
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "a",
				},
			},
			Value: 0.1,
			Ts:    101,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "b",
				},
			},
			Value: 0.2,
			Ts:    102,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "c",
				},
			},
			Value: 0.3,
			Ts:    103,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "d",
				},
			},
			Value: 0.4,
			Ts:    104,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "e",
				},
			},
			Value: 0.5,
			Ts:    105,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "f",
				},
			},
			Value: 0.6,
			Ts:    106,
		},
	}

	for _, e := range exemplars {
		es.AddExemplar(l, e.Ts-1, e)
	}
	require.True(t, reflect.DeepEqual(es.exemplars[0].Exemplar(), exemplars[5]), "exemplar was not stored correctly")
}

func TestSelectExemplar(t *testing.T) {
	es := NewCircularExemplarStorage(5)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	e := exemplar.Exemplar{
		Labels: labels.Labels{
			labels.Label{
				Name:  "traceID",
				Value: "qwerty",
			},
		},
		Value: 0.1,
		HasTs: false,
	}

	es.AddExemplar(l, 0, e)
	require.True(t, reflect.DeepEqual(es.exemplars[0].Exemplar(), e), "exemplar was not stored correctly")

	exemplars, err := es.Select(0, 100, l)
	require.NoError(t, err)

	require.True(t, reflect.DeepEqual([]exemplar.Exemplar{e}, exemplars), "select did not return all exemplars")
}

func TestSelectExemplarOrdering(t *testing.T) {
	es := NewCircularExemplarStorage(5)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	exemplars := []exemplar.Exemplar{
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "a",
				},
			},
			Value: 0.1,
			Ts:    101,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "b",
				},
			},
			Value: 0.2,
			Ts:    102,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "c",
				},
			},
			Value: 0.3,
			Ts:    103,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "d",
				},
			},
			Value: 0.4,
			Ts:    104,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "e",
				},
			},
			Value: 0.5,
			Ts:    105,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "f",
				},
			},
			Value: 0.6,
			Ts:    106,
		},
	}

	for _, e := range exemplars {
		es.AddExemplar(l, e.Ts-1, e)
	}
	require.True(t, reflect.DeepEqual(es.exemplars[0].Exemplar(), exemplars[5]), "exemplar was not stored correctly")

	ret, err := es.Select(100, 110, l)
	require.NoError(t, err)

	require.True(t, reflect.DeepEqual(exemplars[1:], ret), "select did not return all exemplars")
}

func TestSelectExemplar_Circ(t *testing.T) {
	es := NewCircularExemplarStorage(3)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	exemplars := []exemplar.Exemplar{
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "qwerty",
				},
			},
			Value: 0.1,
			Ts:    101,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "zxcvbn",
				},
			},
			Value: 0.1,
			Ts:    102,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "asdfgh",
				},
			},
			Value: 0.1,
			Ts:    103,
		},
	}

	for i, e := range exemplars {
		err := es.AddExemplar(l, e.Ts-1, e)
		require.NoError(t, err)
		require.Equal(t, es.index[l.String()], i, "exemplar was not stored correctly")
	}

	el, err := es.Select(100, 105, l)
	require.NoError(t, err)
	require.True(t, len(el) == 3, "didn't get expected one exemplar")

	for i := range exemplars {
		require.True(t, el[i].Equals(exemplars[i]), "")
	}
}

// This is a set of stored exemplars I scraped and stored locally that resulted in an infinite loop.
// This test ensures Select doesn't infinitely loop on them anymore.
func TestSelectExemplar_OverwriteLoop(t *testing.T) {
	es := NewCircularExemplarStorage(10)

	l1 := labels.Labels{
		{Name: "__name__", Value: "test_metric"},
		{Name: "service", Value: "asdf"},
	}
	l2 := labels.Labels{
		{Name: "__name__", Value: "test_metric"},

		{Name: "service", Value: "qwer"},
	}

	es.index[l1.String()] = 0
	es.exemplars[0] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l1,
			scrapeTimestamp: 4,
		},
		prev: 6,
	}
	es.exemplars[6] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l1,
			scrapeTimestamp: 3,
		},
		prev: 2,
	}

	es.index[l2.String()] = 2
	es.exemplars[2] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 10,
		},
		prev: 1,
	}
	es.exemplars[1] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 9,
		},
		prev: 9,
	}
	es.exemplars[9] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 8,
		},
		prev: 8,
	}
	es.exemplars[8] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 7,
		},
		prev: 7,
	}
	es.exemplars[7] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 6,
		},
		prev: 5,
	}
	es.exemplars[5] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 5,
		},
		prev: 4,
	}
	es.exemplars[4] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 4,
		},
		prev: 3,
	}
	es.exemplars[3] = &circularBufferEntry{
		se: storageExemplar{
			seriesLabels:    l2,
			scrapeTimestamp: 3,
		},
		prev: 1,
	}

	el, err := es.Select(0, 100, l2)
	require.NoError(t, err)

	require.True(t, len(el) == 8, "didn't get expected 8 exemplars, got %d", len(el))
}

func TestSelectExemplar_TimeRange(t *testing.T) {
	es := NewCircularExemplarStorage(4)

	l := labels.Labels{
		{Name: "service", Value: "asdf"},
	}
	exemplars := []exemplar.Exemplar{
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "qwerty",
				},
			},
			Value: 0.1,
			Ts:    101,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "zxcvbn",
				},
			},
			Value: 0.1,
			Ts:    102,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "asdfgh",
				},
			},
			Value: 0.1,
			Ts:    103,
		},
		exemplar.Exemplar{
			Labels: labels.Labels{
				labels.Label{
					Name:  "traceID",
					Value: "hjkl;",
				},
			},
			Value: 0.1,
			Ts:    106,
		},
	}

	for i, e := range exemplars {
		err := es.AddExemplar(l, e.Ts-1, e)
		require.NoError(t, err)
		require.Equal(t, es.index[l.String()], i, "exemplar was not stored correctly")
	}

	el, err := es.Select(102, 105, l)
	require.NoError(t, err)
	require.True(t, len(el) == 2, "didn't get expected one exemplar")

	for i := range el {
		require.True(t, el[i].Equals(exemplars[i+1]), "")
	}
}

func TestIndexOverwrite(t *testing.T) {
	es := NewCircularExemplarStorage(2)

	l1 := labels.Labels{
		{Name: "service", Value: "asdf"},
	}

	l2 := labels.Labels{
		{Name: "service", Value: "qwer"},
	}

	err := es.AddExemplar(l1, 1, exemplar.Exemplar{Value: 1, Ts: 1})
	require.NoError(t, err)
	err = es.AddExemplar(l2, 2, exemplar.Exemplar{Value: 2, Ts: 2})
	require.NoError(t, err)
	err = es.AddExemplar(l2, 3, exemplar.Exemplar{Value: 3, Ts: 3})
	require.NoError(t, err)

	_, ok := es.index[l1.String()]
	require.False(t, ok)

	err = es.AddExemplar(l1, 4, exemplar.Exemplar{Value: 4, Ts: 4})
	require.NoError(t, err)

	i := es.index[l2.String()]
	require.Equal(t, 0, i)
}

// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0

package tsdb

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/index"
)

func TestPostingsForMatchersStoredEmptyValues(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "index")
	w, err := index.NewWriter(ctx, path)
	require.NoError(t, err)
	symbols := []string{"", "\n", "1", "2", "3", "4", "__name__", "id", "metric", "x", "z"}
	slices.Sort(symbols)
	for _, symbol := range symbols {
		require.NoError(t, w.AddSymbol(symbol))
	}
	// The ID label keeps these series distinct and lexicographically ordered.
	for i, lset := range []labels.Labels{
		labels.FromStrings("__name__", "metric", "id", "1"),
		labels.FromStrings("__name__", "metric", "id", "2", "z", ""),
		labels.FromStrings("__name__", "metric", "id", "3", "z", "\n"),
		labels.FromStrings("__name__", "metric", "id", "4", "z", "x"),
	} {
		require.NoError(t, w.AddSeries(storage.SeriesRef(i+1), lset))
	}
	require.NoError(t, w.Close())
	r, err := index.NewFileReader(path, index.DecodePostingsRaw)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close()) })
	var refs []storage.SeriesRef
	for i := 1; i <= 4; i++ {
		p, err := r.Postings(ctx, "id", strconv.Itoa(i))
		require.NoError(t, err)
		got, err := index.ExpandPostings(p)
		require.NoError(t, err)
		require.Len(t, got, 1)
		refs = append(refs, got[0])
	}
	// Presence includes an explicitly stored empty value; retain that API contract.
	present, err := index.ExpandPostings(r.PostingsForAllLabelValues(ctx, "z"))
	require.NoError(t, err)
	require.Equal(t, refs[1:], present)
	values, err := r.LabelValues(ctx, "z", nil)
	require.NoError(t, err)
	require.Equal(t, []string{"", "\n", "x"}, values)
	for _, tc := range []struct {
		name  string
		typ   labels.MatchType
		value string
		want  []storage.SeriesRef
	}{
		{"equal empty", labels.MatchEqual, "", refs[:2]},
		{"not equal empty", labels.MatchNotEqual, "", refs[2:]},
		{"regexp empty", labels.MatchRegexp, "", refs[:2]},
		{"not regexp empty", labels.MatchNotRegexp, "", refs[2:]},
		{"regexp nonempty", labels.MatchRegexp, ".+", refs[2:]},
		{"not regexp nonempty", labels.MatchNotRegexp, ".+", refs[:2]},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, intersect := range []bool{false, true} {
				ms := []*labels.Matcher{labels.MustNewMatcher(tc.typ, "z", tc.value)}
				if intersect {
					ms = append(ms, labels.MustNewMatcher(labels.MatchEqual, "__name__", "metric"))
				}
				p, err := PostingsForMatchers(ctx, r, ms...)
				require.NoError(t, err)
				got, err := index.ExpandPostings(p)
				require.NoError(t, err)
				require.Equal(t, tc.want, got, "intersect=%t", intersect)
			}
		})
	}
}

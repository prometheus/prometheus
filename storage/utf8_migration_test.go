package storage

// var utf8Data = []seriesSamples{
// 	{
// 		lset: map[string]string{"a": "a", "__name__": "foo.bar"},
// 		chunks: [][]sample{
// 			{{1, 2, nil, nil}, {2, 3, nil, nil}, {3, 4, nil, nil}},
// 			{{5, 2, nil, nil}, {6, 3, nil, nil}, {7, 4, nil, nil}},
// 		},
// 	},
// 	{
// 		lset: map[string]string{"a": "b", "__name__": "foo.bar"},
// 		chunks: [][]sample{
// 			{{1, 1, nil, nil}, {2, 2, nil, nil}, {3, 3, nil, nil}},
// 			{{5, 3, nil, nil}, {6, 6, nil, nil}},
// 		},
// 	},
// 	{
// 		lset: map[string]string{"c": "d", "__name__": "baz.qux"},
// 		chunks: [][]sample{
// 			{{1, 1, nil, nil}, {2, 2, nil, nil}, {3, 3, nil, nil}},
// 			{{5, 3, nil, nil}, {6, 6, nil, nil}},
// 		},
// 	},
// }

// var underscoreEscapedUTF8Data = []seriesSamples{
// 	{
// 		lset: map[string]string{"a": "c", "__name__": "foo_bar"},
// 		chunks: [][]sample{
// 			{{1, 3, nil, nil}, {2, 2, nil, nil}, {3, 6, nil, nil}},
// 			{{5, 1, nil, nil}, {6, 7, nil, nil}, {7, 2, nil, nil}},
// 		},
// 	},
// 	{
// 		lset: map[string]string{"e": "f", "__name__": "baz_qux"},
// 		chunks: [][]sample{
// 			{{1, 3, nil, nil}, {2, 2, nil, nil}, {3, 6, nil, nil}},
// 			{{5, 1, nil, nil}, {6, 7, nil, nil}, {7, 2, nil, nil}},
// 		},
// 	},
// 	{
// 		lset: map[string]string{"__name__": "another_metric"},
// 		chunks: [][]sample{
// 			{{1, 41, nil, nil}, {2, 42, nil, nil}, {3, 43, nil, nil}},
// 			{{5, 45, nil, nil}, {6, 46, nil, nil}, {7, 47, nil, nil}},
// 		},
// 	},
// }

// var mixedUTF8Data = append(utf8Data, underscoreEscapedUTF8Data...)

// var s1 = storage.NewListSeries(labels.FromStrings("a", "a", "__name__", "foo.bar"),
// 	[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}, sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
// )
// var c1 = storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "a", "__name__", "foo.bar"),
// 	[]chunks.Sample{sample{1, 2, nil, nil}, sample{2, 3, nil, nil}, sample{3, 4, nil, nil}}, []chunks.Sample{sample{5, 2, nil, nil}, sample{6, 3, nil, nil}, sample{7, 4, nil, nil}},
// )
// var s2 = storage.NewListSeries(labels.FromStrings("a", "b", "__name__", "foo.bar"),
// 	[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}, sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
// )
// var c2 = storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "b", "__name__", "foo.bar"),
// 	[]chunks.Sample{sample{1, 1, nil, nil}, sample{2, 2, nil, nil}, sample{3, 3, nil, nil}}, []chunks.Sample{sample{5, 3, nil, nil}, sample{6, 6, nil, nil}},
// )

// var s3 = storage.NewListSeries(labels.FromStrings("a", "c", "__name__", "foo_bar"),
// 	[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
// )
// var c3 = storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "c", "__name__", "foo_bar"),
// 	[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []chunks.Sample{sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
// )
// var s3Unescaped = storage.NewListSeries(labels.FromStrings("a", "c", "__name__", "foo.bar"),
// 	[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}, sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
// )
// var c3Unescaped = storage.NewListChunkSeriesFromSamples(labels.FromStrings("a", "c", "__name__", "foo.bar"),
// 	[]chunks.Sample{sample{1, 3, nil, nil}, sample{2, 2, nil, nil}, sample{3, 6, nil, nil}}, []chunks.Sample{sample{5, 1, nil, nil}, sample{6, 7, nil, nil}, sample{7, 2, nil, nil}},
// )

// func TestMixedUTF8BlockQuerier_Select(t *testing.T) {
// 	// TODO(npazosmendez): test cases
// 	// * same label set is combines and samples are returned in order

// 	for _, c := range []querierSelectTestCase{
// 		{
// 			ms:      []*labels.Matcher{},
// 			exp:     newMockSeriesSet([]storage.Series{}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
// 		},
// 		{
// 			// No __name__= matcher, no-op
// 			mint:    math.MinInt64,
// 			maxt:    math.MaxInt64,
// 			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".+")},
// 			exp:     newMockSeriesSet([]storage.Series{s1, s2, s3}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{c1, c2, c3}),
// 		},
// 		{
// 			// __name__= matcher, explode query and relabel
// 			mint:    math.MinInt64,
// 			maxt:    math.MaxInt64,
// 			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "foo.bar")},
// 			exp:     newMockSeriesSet([]storage.Series{s1, s2, s3Unescaped}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{c1, c2, c3Unescaped}),
// 		},
// 		{
// 			// __name__= matcher plus other labels
// 			mint:    math.MinInt64,
// 			maxt:    math.MaxInt64,
// 			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "foo.bar"), labels.MustNewMatcher(labels.MatchNotEqual, "a", "b")},
// 			exp:     newMockSeriesSet([]storage.Series{s1, s3Unescaped}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{c1, c3Unescaped}),
// 		},
// 		{
// 			// No need to escape matcher, no-op
// 			mint:    math.MinInt64,
// 			maxt:    math.MaxInt64,
// 			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "foo_bar")},
// 			exp:     newMockSeriesSet([]storage.Series{s3}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{c3}),
// 		},
// 	} {
// 		ir, cr, _, _ := createIdxChkReaders(t, mixedUTF8Data)
// 		q := &blockQuerier{
// 			blockBaseQuerier: &blockBaseQuerier{
// 				index:      ir,
// 				chunks:     cr,
// 				tombstones: tombstones.NewMemTombstones(),

// 				mint: c.mint,
// 				maxt: c.maxt,
// 			},
// 		}

// 		mixedQ := &mixedUTF8BlockQuerier{
// 			blockQuerier: q,
// 			es:           model.UnderscoreEscaping,
// 		}

// 		mixedChunkQ := &mixedUTF8BlockChunkQuerier{
// 			blockChunkQuerier: &blockChunkQuerier{
// 				blockBaseQuerier: &blockBaseQuerier{
// 					index:      ir,
// 					chunks:     cr,
// 					tombstones: tombstones.NewMemTombstones(),
// 					mint:       c.mint,
// 					maxt:       c.maxt,
// 				},
// 			},
// 			es: model.UnderscoreEscaping,
// 		}
// 		testQueriersSelect(t, c, mixedQ, mixedChunkQ)
// 	}
// }

// func TestMixedUTF8BlockQuerier_Labels(t *testing.T) {
// 	for _, c := range []struct {
// 		mint, maxt     int64
// 		ms             []*labels.Matcher
// 		labelName      string
// 		expLabelValues []string
// 		expLabelNames  []string
// 	}{
// 		{
// 			mint:           math.MinInt64,
// 			maxt:           math.MaxInt64,
// 			ms:             []*labels.Matcher{},
// 			labelName:      "__name__",
// 			expLabelValues: []string{"another_metric", "baz.qux", "baz_qux", "foo.bar", "foo_bar"},
// 			expLabelNames:  []string{"", "__name__", "a", "c", "e"},
// 		},
// 		{
// 			mint:           math.MinInt64,
// 			maxt:           math.MaxInt64,
// 			ms:             []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "foo.bar")},
// 			labelName:      "__name__",
// 			expLabelValues: []string{"foo.bar"},
// 			expLabelNames:  []string{"__name__", "a"},
// 		},
// 		{
// 			mint:           math.MinInt64,
// 			maxt:           math.MaxInt64,
// 			ms:             []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "__name__", "baz.qux")},
// 			labelName:      "e",
// 			expLabelValues: []string{"f"},
// 			expLabelNames:  []string{"__name__", "c", "e"},
// 		},
// 	} {
// 		ir, cr, _, _ := createIdxChkReaders(t, mixedUTF8Data)
// 		q := &blockQuerier{
// 			blockBaseQuerier: &blockBaseQuerier{
// 				index:      ir,
// 				chunks:     cr,
// 				tombstones: tombstones.NewMemTombstones(),

// 				mint: c.mint,
// 				maxt: c.maxt,
// 			},
// 		}

// 		mixedQ := &mixedUTF8BlockQuerier{
// 			blockQuerier: q,
// 			es:           model.UnderscoreEscaping,
// 		}

// 		mixedChunkQ := &mixedUTF8BlockChunkQuerier{
// 			blockChunkQuerier: &blockChunkQuerier{
// 				blockBaseQuerier: &blockBaseQuerier{
// 					index:      ir,
// 					chunks:     cr,
// 					tombstones: tombstones.NewMemTombstones(),
// 					mint:       c.mint,
// 					maxt:       c.maxt,
// 				},
// 			},
// 			es: model.UnderscoreEscaping,
// 		}
// 		t.Run("LabelValues", func(t *testing.T) {
// 			lv, _, err := mixedQ.LabelValues(context.Background(), c.labelName, nil, c.ms...)
// 			require.NoError(t, err)
// 			require.Equal(t, c.expLabelValues, lv)
// 			lv, _, err = mixedChunkQ.LabelValues(context.Background(), c.labelName, nil, c.ms...)
// 			require.NoError(t, err)
// 			require.Equal(t, c.expLabelValues, lv)
// 		})

// 		t.Run("LabelNames", func(t *testing.T) {
// 			ln, _, err := mixedQ.LabelNames(context.Background(), nil, c.ms...)
// 			require.NoError(t, err)
// 			require.Equal(t, c.expLabelNames, ln)
// 			ln, _, err = mixedChunkQ.LabelNames(context.Background(), nil, c.ms...)
// 			require.NoError(t, err)
// 			require.Equal(t, c.expLabelNames, ln)
// 		})
// 		require.NoError(t, mixedQ.Close())
// 		require.NoError(t, mixedChunkQ.Close())
// 	}
// }

// func TestEscapedUTF8BlockQuerier(t *testing.T) {
// 	for _, c := range []querierSelectTestCase{
// 		{
// 			ms:      []*labels.Matcher{},
// 			exp:     newMockSeriesSet([]storage.Series{}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{}),
// 		},
// 		{
// 			mint:    math.MinInt64,
// 			maxt:    math.MaxInt64,
// 			ms:      []*labels.Matcher{labels.MustNewMatcher(labels.MatchRegexp, "a", ".+")},
// 			exp:     newMockSeriesSet([]storage.Series{s3}),
// 			expChks: newMockChunkSeriesSet([]storage.ChunkSeries{c3}),
// 		},
// 	} {
// 		ir, cr, _, _ := createIdxChkReaders(t, underscoreEscapedUTF8Data)
// 		q := &blockQuerier{
// 			blockBaseQuerier: &blockBaseQuerier{
// 				index:      ir,
// 				chunks:     cr,
// 				tombstones: tombstones.NewMemTombstones(),

// 				mint: c.mint,
// 				maxt: c.maxt,
// 			},
// 		}

// 		escapedQ := &escapedUTF8BlockQuerier{
// 			blockQuerier: q,
// 			es:           model.UnderscoreEscaping,
// 		}

// 		escapedChunkQ := &escapedUTF8BlockChunkQuerier{
// 			blockChunkQuerier: &blockChunkQuerier{
// 				blockBaseQuerier: &blockBaseQuerier{
// 					index:      ir,
// 					chunks:     cr,
// 					tombstones: tombstones.NewMemTombstones(),
// 					mint:       c.mint,
// 					maxt:       c.maxt,
// 				},
// 			},
// 			es: model.UnderscoreEscaping,
// 		}
// 		testQueriersSelect(t, c, escapedQ, escapedChunkQ)
// 	}
// }

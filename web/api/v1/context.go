// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/seriesmetadata"
)

// SeriesContext is one entry in the shared contexts table returned alongside
// enriched /query and /query_range results. It groups native metadata by
// namespace; currently only the resource namespace is populated.
type SeriesContext struct {
	Resource *ResourceAttributes `json:"resource,omitempty"`
}

// ResourceAttributes holds OTel resource attributes for a series, split into
// identifying attributes (stable service identity, usable as join keys) and
// descriptive attributes (everything else, may change over the series lifetime).
type ResourceAttributes struct {
	Identifying map[string]string `json:"identifying,omitempty"`
	Descriptive map[string]string `json:"descriptive,omitempty"`
}

// contextSelector is the parsed form of the context= query parameter. It is a
// projection over fully-qualified, dot-separated metadata keys (e.g.
// "resource.k8s.pod.name"). Supported forms: "*" (everything), a "<prefix>.*"
// glob, or an exact key. Multiple patterns are comma-separated.
type contextSelector struct {
	all      bool
	patterns []string
}

// parseContextSelector parses the context= parameter value. It returns (nil,
// nil) when the parameter is empty, meaning no enrichment is requested.
func parseContextSelector(s string) (*contextSelector, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	cs := &contextSelector{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
			continue
		case part == "*":
			cs.all = true
		default:
			cs.patterns = append(cs.patterns, part)
		}
	}
	if !cs.all && len(cs.patterns) == 0 {
		return nil, nil
	}
	return cs, nil
}

// matchKey reports whether the fully-qualified key fq (e.g.
// "resource.service.name") is selected by the projection.
func (cs *contextSelector) matchKey(fq string) bool {
	if cs.all {
		return true
	}
	for _, p := range cs.patterns {
		if strings.HasSuffix(p, ".*") {
			if strings.HasPrefix(fq, strings.TrimSuffix(p, "*")) {
				return true
			}
			continue
		}
		if p == fq {
			return true
		}
	}
	return false
}

// contextRun marks the sample index at which a series' context id takes effect,
// applying until the next run. An empty ID means the samples in this run have
// no resolvable context (serialized as null).
type contextRun struct {
	StartIndex int
	ID         string
}

// VectorWithContext wraps a promql.Vector with a per-sample context id (aligned
// by index with the vector). It is used as the query result when context
// enrichment is requested so the JSON codec can emit the "context" field.
type VectorWithContext struct {
	v    promql.Vector
	refs []string
}

// Type implements parser.Value.
func (VectorWithContext) Type() parser.ValueType { return parser.ValueTypeVector }

// String implements parser.Value.
func (w VectorWithContext) String() string { return w.v.String() }

// MatrixWithContext wraps a promql.Matrix with per-series change-point context
// runs (indexed into each series' values, or histograms for histogram-only
// series).
type MatrixWithContext struct {
	m    promql.Matrix
	runs [][]contextRun
}

// Type implements parser.Value.
func (MatrixWithContext) Type() parser.ValueType { return parser.ValueTypeMatrix }

// String implements parser.Value.
func (w MatrixWithContext) String() string { return w.m.String() }

// contextResolver resolves per-series resource attributes into a deduplicated
// table of SeriesContext entries keyed by short ids (r1, r2, ...). Series that
// share identical (projected) resource content share a single entry.
type contextResolver struct {
	rq    storage.ResourceQuerier
	sel   *contextSelector
	table map[string]SeriesContext
	byKey map[string]string
	n     int
}

func newContextResolver(rq storage.ResourceQuerier, sel *contextSelector) *contextResolver {
	return &contextResolver{
		rq:    rq,
		sel:   sel,
		table: map[string]SeriesContext{},
		byKey: map[string]string{},
	}
}

// resolve returns the context id for the series with the given stable labels
// hash at timestamp t. It returns ("", false) when no resource is stored for
// the series or nothing survives the projection.
func (r *contextResolver) resolve(hash uint64, t int64) (string, bool) {
	rv, found := r.rq.GetResourceAt(hash, t)
	if !found || rv == nil {
		return "", false
	}
	entry, ok := r.project(rv)
	if !ok {
		return "", false
	}
	key := contentKey(entry)
	if id, ok := r.byKey[key]; ok {
		return id, true
	}
	r.n++
	id := "r" + strconv.Itoa(r.n)
	r.byKey[key] = id
	r.table[id] = entry
	return id, true
}

// project applies the context= selector to a resource version, keeping only the
// attributes whose fully-qualified key matches. It returns ok=false when
// nothing survives.
func (r *contextResolver) project(rv *seriesmetadata.ResourceVersion) (SeriesContext, bool) {
	res := &ResourceAttributes{}
	for k, v := range rv.Identifying {
		if r.sel.matchKey("resource." + k) {
			if res.Identifying == nil {
				res.Identifying = map[string]string{}
			}
			res.Identifying[k] = v
		}
	}
	for k, v := range rv.Descriptive {
		if r.sel.matchKey("resource." + k) {
			if res.Descriptive == nil {
				res.Descriptive = map[string]string{}
			}
			res.Descriptive[k] = v
		}
	}
	if len(res.Identifying) == 0 && len(res.Descriptive) == 0 {
		return SeriesContext{}, false
	}
	return SeriesContext{Resource: res}, true
}

// buildRuns resolves a matrix series' context per sample and collapses it into
// change-point runs. Returns nil when no sample resolves to a context.
func (r *contextResolver) buildRuns(s promql.Series) []contextRun {
	hash := labels.StableHash(s.Metric)

	n := len(s.Floats)
	useHist := n == 0 && len(s.Histograms) > 0
	if useHist {
		n = len(s.Histograms)
	}
	if n == 0 {
		return nil
	}

	runs := make([]contextRun, 0, 2)
	hasAny := false
	prevSet := false
	var prevID string
	for i := 0; i < n; i++ {
		var t int64
		if useHist {
			t = s.Histograms[i].T
		} else {
			t = s.Floats[i].T
		}
		id, ok := r.resolve(hash, t)
		if !ok {
			id = ""
		} else {
			hasAny = true
		}
		if !prevSet || id != prevID {
			runs = append(runs, contextRun{StartIndex: i, ID: id})
			prevID = id
			prevSet = true
		}
	}
	if !hasAny {
		return nil
	}
	return runs
}

// contentKey builds a deterministic dedup key from a projected context entry.
func contentKey(sc SeriesContext) string {
	var b strings.Builder
	if sc.Resource != nil {
		b.WriteString("I")
		writeSortedMap(&b, sc.Resource.Identifying)
		b.WriteString("D")
		writeSortedMap(&b, sc.Resource.Descriptive)
	}
	return b.String()
}

func writeSortedMap(b *strings.Builder, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte('\x00')
	}
}

// applyContext parses the context= parameter and, when the native-metadata
// feature is enabled, enriches qd.Result in place with per-series context and
// populates qd.Contexts. mint/maxt bound the querier used for the lookup (in
// milliseconds). It returns a non-fatal warning (to attach to the response) and
// a fatal error (for an invalid parameter).
func (api *API) applyContext(r *http.Request, qd *QueryData, mint, maxt int64) (warning, err error) {
	raw := r.FormValue("context")
	if raw == "" {
		return nil, nil
	}
	if !api.enableNativeMetadata {
		return errors.New("context enrichment ignored: native-metadata feature is not enabled"), nil
	}
	sel, perr := parseContextSelector(raw)
	if perr != nil {
		return nil, perr
	}
	if sel == nil {
		return nil, nil
	}
	enriched, table, eerr := api.enrichWithContext(qd.Result, sel, mint, maxt)
	if eerr != nil {
		return fmt.Errorf("context enrichment failed: %w", eerr), nil
	}
	qd.Result = enriched
	qd.Contexts = table
	return nil, nil
}

// enrichWithContext resolves resource attributes for the series in a vector or
// matrix result and returns a context-carrying wrapper plus the shared contexts
// table. mint/maxt bound the querier opened for the lookup. On any error, or
// when the storage does not support resource queries, the original value is
// returned unchanged with a nil table.
func (api *API) enrichWithContext(value parser.Value, sel *contextSelector, mint, maxt int64) (parser.Value, map[string]SeriesContext, error) {
	q, err := api.Queryable.Querier(mint, maxt)
	if err != nil {
		return value, nil, err
	}
	defer q.Close()

	rq, ok := q.(storage.ResourceQuerier)
	if !ok {
		return value, nil, nil
	}
	resolver := newContextResolver(rq, sel)

	switch v := value.(type) {
	case promql.Vector:
		refs := make([]string, len(v))
		for i, s := range v {
			if id, ok := resolver.resolve(labels.StableHash(s.Metric), s.T); ok {
				refs[i] = id
			}
		}
		if len(resolver.table) == 0 {
			return value, nil, nil
		}
		return VectorWithContext{v: v, refs: refs}, resolver.table, nil
	case promql.Matrix:
		runs := make([][]contextRun, len(v))
		for i, s := range v {
			runs[i] = resolver.buildRuns(s)
		}
		if len(resolver.table) == 0 {
			return value, nil, nil
		}
		return MatrixWithContext{m: v, runs: runs}, resolver.table, nil
	default:
		return value, nil, nil
	}
}

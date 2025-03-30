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

package semconv

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/maruel/natural"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/prometheus/model/labels"
)

const cacheTTL = 1 * time.Hour

type schemaEngine struct {
	// TODO(bwplotka): Implement GC logic for ttl and limits.
	cachedIDs       map[string]*ids
	cacheMu         sync.RWMutex
	cachedChangelog map[string]*changelog

	schemaBaseOverride map[string]string
}

func newSchemaEngine() *schemaEngine {
	return &schemaEngine{
		cachedIDs:       map[string]*ids{},
		cachedChangelog: map[string]*changelog{},

		schemaBaseOverride: map[string]string{},
	}
}

type matcherBuilder struct {
	metric labels.MetricIdentity
	other  []*labels.Matcher
}

func newMatcherBuilder(matchers []*labels.Matcher) (matcherBuilder, error) {
	var b matcherBuilder
	for _, m := range matchers {
		switch m.Name {
		case labels.MetricName:
			if m.Type != labels.MatchEqual {
				return b, fmt.Errorf("__name__ matcher must be equal")
			}
			b.metric.Name = m.Value
		case "__type__":
			if m.Type != labels.MatchEqual {
				return b, fmt.Errorf("__type__ matcher must be equal")
			}
			b.metric.Type = model.MetricType(m.Value)
		case "__unit__":
			if m.Type != labels.MatchEqual {
				return b, fmt.Errorf("__unit__ matcher must be equal")
			}
			b.metric.Unit = m.Value
		case schemaURLLabel:
			// Skip it as we will be querying to different versions? We could
			// make regex for registry dir at least, if that helps.
		default:
			b.other = append(b.other, m)
		}
	}
	return b, nil
}

func (b matcherBuilder) Clone() matcherBuilder {
	return matcherBuilder{
		metric: b.metric,
		other:  slices.Clone(b.other),
	}
}

// ToMatchers returns a copy of matchers based on builder details.
func (b matcherBuilder) ToMatchers(extraNameSuffix string) []*labels.Matcher {
	ret := make([]*labels.Matcher, 0, len(b.other)+3)

	if b.metric.Name != "" {
		ret = append(ret, &labels.Matcher{
			Name:  model.MetricNameLabel,
			Type:  labels.MatchEqual,
			Value: b.metric.Name + extraNameSuffix,
		})
	}
	if b.metric.Type != "" && b.metric.Type != model.MetricTypeUnknown {
		ret = append(ret, &labels.Matcher{
			Name:  "__type__",
			Type:  labels.MatchEqual,
			Value: string(b.metric.Type),
		})
	}
	if b.metric.Unit != "" {
		ret = append(ret, &labels.Matcher{
			Name:  "__unit__",
			Type:  labels.MatchEqual,
			Value: b.metric.Unit,
		})
	}
	return append(ret, b.other...)
}

func (e *schemaEngine) ApplyConfig(cfg *config.Config) error {
	e.cacheMu.Lock()
	e.schemaBaseOverride = cfg.SemConv.SchemaOverrides
	e.cacheMu.Unlock()
	return nil
}

func (e *schemaEngine) fetchIDs(schemaURL string) (_ *ids, err error) {
	e.cacheMu.RLock()
	// NOTE(bwplotka): Be careful with path as it cleans potential http:// to http:/
	schemaBase, _ := path.Split(schemaURL)
	schemaBase = strings.TrimSuffix(schemaBase, "/")
	if o, ok := e.schemaBaseOverride[schemaBase]; ok {
		schemaBase = o
	}
	schemaIDsURL := fmt.Sprintf("%v/ids.yaml", schemaBase)
	ids, ok := e.cachedIDs[schemaIDsURL]
	e.cacheMu.RUnlock()
	if ok && time.Now().Sub(ids.fetchTime) < cacheTTL {
		return ids, nil
	}
	// Expired or missing.
	ids, err = fetchIDs(schemaIDsURL)
	if err != nil {
		return nil, err
	}
	e.cacheMu.Lock()
	e.cachedIDs[schemaIDsURL] = ids
	e.cacheMu.Unlock()
	return ids, nil
}

func (e *schemaEngine) fetchChangelog(schemaURL string) (_ *changelog, err error) {
	e.cacheMu.RLock()
	// NOTE(bwplotka): Be careful with path as it cleans potential http:// to http:/
	schemaBase, _ := path.Split(schemaURL)
	schemaBase = strings.TrimSuffix(schemaBase, "/")
	if o, ok := e.schemaBaseOverride[schemaBase]; ok {
		schemaBase = o
	}
	schemaChangelogURL := fmt.Sprintf("%v/changelog.yaml", schemaBase)

	ch, ok := e.cachedChangelog[schemaChangelogURL]
	e.cacheMu.RUnlock()
	if ok && time.Now().Sub(ch.fetchTime) < cacheTTL {
		return ch, nil
	}
	// Expired or missing.
	ch, err = fetchChangelog(schemaChangelogURL)
	if err != nil {
		return nil, err
	}
	e.cacheMu.Lock()
	e.cachedChangelog[schemaChangelogURL] = ch
	e.cacheMu.Unlock()
	return ch, nil
}

// findMetricID returns the metric ID from the schema definition for this identity and schema URL.
// This allows parsing semantic ID and the revision number. This function also returns
// magicSuffix that was matched if any.
func (e *schemaEngine) findMetricID(schemaURL string, metric labels.MetricIdentity) (metricID, string, error) {
	schemaVersion := path.Base(schemaURL)

	// TODO(bwplotka): This assumes such a file structure is part of the spec.
	ids, err := e.fetchIDs(schemaURL)
	if err != nil {
		return "", "", fmt.Errorf("based on __schema_url__=%v; %w", schemaURL, err)
	}

	var (
		vid         []versionedID
		magicSuffix string
	)
	for _, suffix := range []string{"", "_bucket", "_count", "_sum"} {
		magicSuffix = suffix
		m := metric
		m.Name = strings.TrimSuffix(m.Name, magicSuffix)

		var ok bool
		vid, ok = ids.MetricsIDs[m.String()]
		if !ok {
			// Try non-unit search.
			val, ok := ids.uniqueNameTypeToIdentity[m.String()]
			if !ok {
				// Try just name search.
				val, ok = ids.uniqueNameToIdentity[m.Name]
				if !ok {
					// Try different suffix.
					continue
				}
			}
			if val == "" {
				return "", "", fmt.Errorf("ambigous metric ID lookup for %v metric; use __type__ and __unit__ for more specific selection", m.String())
			}
			vid = ids.MetricsIDs[val]
		}
		break
	}
	if len(vid) == 0 {
		return "", "", fmt.Errorf("can't find metric ID in %v entry for version %v; this metric (with or without magic suffixes) is not part of this schema registry", metric.String(), schemaVersion)
	}

	for _, id := range vid {
		if natural.Less(schemaVersion, id.IntroVersion) {
			continue
		}
		return id.ID, magicSuffix, nil
	}
	return "", "", fmt.Errorf("can't find metric ID in %v entry for version %v", metric.String(), schemaVersion)
}

type queryContext struct {
	mID         metricID
	magicSuffix string
	changes     []change
}

// FindMatcherVariants returns all variants to match for a single schematized (referenced by schema_url) metric selection.
// It also returns all changes for found semantic ID.
// It returns error if the given matchers does not point to a single metric or if schema or variants couldn't
// be detected.
func (e *schemaEngine) FindMatcherVariants(schemaURL string, originalMatchers []*labels.Matcher) (variants [][]*labels.Matcher, q queryContext, err error) {
	matchers, err := newMatcherBuilder(originalMatchers)
	if err != nil {
		return nil, q, err
	}

	q.mID, q.magicSuffix, err = e.findMetricID(schemaURL, matchers.metric)
	if err != nil {
		return nil, q, fmt.Errorf("FindMetricID: %w", err)
	}

	ch, err := e.fetchChangelog(schemaURL)
	if err != nil {
		return nil, q, err
	}

	// Original selection (without schema url).
	variants = append(variants, matchers.ToMatchers(""))

	sID, rev := q.mID.semanticID()
	q.changes, _ = ch.MetricsChangelog[sID]
	if len(q.changes) == 0 {
		// Unfortunately this (!ok) might also mean the malformed schema or cache.
		// __schema__id__ idea would be more robust here.
		// We could expect non-changed things in changelog, but that would
		// make changelog overly huge.
		return variants, q, nil
	}

	// Changes are sorted from the newest to the oldest, so reverse this, so
	// it's matches the revisions order.
	slices.Reverse(q.changes)

	// Revision starts with 0, then 2,3,4..., uniform it (0,1,2,3...).
	if rev != 0 {
		rev--
	}

	t := &changeTraverser{
		changes:     q.changes,
		magicSuffix: q.magicSuffix,
	}

	// Changelog contains changes across revisions, traverse forward and backward.
	variants, err = t.traverseForMatchers(rev, false, matchers.Clone(), variants)
	if err != nil {
		return nil, q, fmt.Errorf("can't traverse changes for semantic ID %v: %w", sID, err)
	}
	variants, err = t.traverseForMatchers(rev, true, matchers, variants)
	if err != nil {
		return nil, q, fmt.Errorf("can't traverse changes for semantic ID %v: %w", sID, err)
	}
	return variants, q, nil
}

type changeTraverser struct {
	changes     []change
	magicSuffix string
}

// traverseForMatchers builds the matchers for the variant to be queried.
// It then walks further with the new matchers and result transformation as the base for the next change in the chain.
// This allows handling multi-version variants.
func (t *changeTraverser) traverseForMatchers(revision int, newer bool, b matcherBuilder, v [][]*labels.Matcher) ([][]*labels.Matcher, error) {
	var (
		to, from metricGroupChange
	)
	// Changes are sorted from the oldest to the newest.
	if newer {
		if len(t.changes) <= revision {
			return v, nil
		}
		// We are at the changes from older to newer revision, so to match the new version we
		// have to take the existing matchers forward.
		to = t.changes[revision].Forward
		from = t.changes[revision].Backward
		revision++
	} else {
		revision--
		if revision < 0 {
			return v, nil
		}
		// We are at the changes from newer to older revision, so to match the old version we
		// have to take the existing matchers backward.
		to = t.changes[revision].Backward
		from = t.changes[revision].Forward
	}

	// We have the `b` from the last traversal with potentially
	// already transformed matchers, so just add new changes in.
	if to.MetricName != "" {
		b.metric.Name = to.MetricName
	}
	if to.Unit != "" {
		b.metric.Unit = to.DirectUnit()
	}

	for a := range to.Attributes {
		aTo := to.Attributes[a]
		aFrom := from.Attributes[a]
		// TODO(bwplotka): In current logic, tag MUST be specified,
		// otherwise the engine would need to fetch full metric definition and
		// to get the tag -> ID of attribute (or separate attribute tag -> IDs index).
		for m := range b.other {
			// Find the attribute under the "old" name.
			if b.other[m].Name == aFrom.Tag {
				old := b.other[m]
				value := b.other[m].Value
				for member := range aTo.Members {
					if old.Matches(aFrom.Members[member].Value) {
						// TODO(bwplotka): Pretty yolo e.g. should we also replace partial use in regex?
						value = strings.Replace(value, aFrom.Members[member].Value, aTo.Members[member].Value, -1)
					}
				}
				b.other[m] = labels.MustNewMatcher(old.Type, aTo.Tag, value)
				break
			}
		}
	}
	return t.traverseForMatchers(revision, newer, b, append(v, b.ToMatchers(t.magicSuffix)))
}

// TransformSeries returns transformed series and value transformer for a single series that contains __schema__url__.
// TODO(bwplotka): Decide what to do if non schematized series are returned, currently we error.
func (e *schemaEngine) TransformSeries(q queryContext, originalLabels labels.Labels) (lbls labels.Labels, vt valueTransformer, _ error) {
	schemaURL := originalLabels.Get(schemaURLLabel)
	if schemaURL == "" {
		return originalLabels, vt, fmt.Errorf("selected series %v does not contain __schema_url__", originalLabels)
	}

	identity := originalLabels.MetricIdentity()
	mID, magicSuffix, err := e.findMetricID(schemaURL, identity)
	if err != nil {
		return originalLabels, vt, fmt.Errorf("getMetricID: %w", err)
	}

	sID, fromRev := mID.semanticID()
	toSID, toRev := q.mID.semanticID()
	if sID != toSID {
		// Should not happen?
		return originalLabels, vt, fmt.Errorf("selected series %v (id: %v) are not semantically equivalent (desired id: %v)", originalLabels, sID, toSID)
	}

	builder := labels.NewBuilder(originalLabels)
	// Explicitly remove __schema_url__ as that would be misleading.
	builder.Del(schemaURLLabel)

	if fromRev == toRev {
		return builder.Labels(), vt, nil
	}

	// Transform series fromRev -> toRev.

	// Revision starts with 0, then 2,3,4..., uniform it (0,1,2,3...).
	if fromRev != 0 {
		fromRev--
	}
	if toRev != 0 {
		toRev--
	}

	t := &changeTraverser{
		changes:     q.changes,
		magicSuffix: magicSuffix,
	}

	// Changelog contains changes across revisions, traverse in the required direction.
	vt, err = t.traverseForLabels(fromRev, toRev, identity.Type, magicSuffix, builder, valueTransformer{})
	if err != nil {
		return originalLabels, vt, fmt.Errorf("can't traverse changes for semantic ID %v: %w", sID, err)
	}
	return builder.Labels(), vt, nil
}

// traverseForLabels builds the matchers for the variant to be queried.
// It then walks further with the new matchers and result transformation as the base for the next change in the chain.
// This allows handling multi-version variants.
func (t *changeTraverser) traverseForLabels(fromRev, toRev int, mTyp model.MetricType, magicSuffix string, b *labels.Builder, vt valueTransformer) (valueTransformer, error) {
	if fromRev == toRev {
		return vt, nil
	}

	var to, from metricGroupChange
	// Changes are sorted from the oldest to the newest.
	if fromRev < toRev {
		if len(t.changes) <= fromRev {
			return vt, nil
		}
		// We are at the changes from older to newer revision, so go forward.
		to = t.changes[fromRev].Forward
		from = t.changes[fromRev].Backward
		fromRev++
	} else {
		fromRev--
		if fromRev < 0 {
			return vt, nil
		}
		// We are at the changes from newer to older revision, so go backward.
		to = t.changes[fromRev].Backward
		from = t.changes[fromRev].Forward
	}

	// Find value transformation.
	if to.ValuePromQL != "" {
		var err error
		vt, err = vt.AddPromQL(to.ValuePromQL)
		if err != nil {
			return vt, err
		}
	}

	b.Range(func(l labels.Label) {

	nameswitch:
		switch l.Name {
		case labels.MetricName:
			if to.MetricName != "" {
				b.Set(l.Name, to.MetricName+magicSuffix)
			}
		case "__type__":
			return
		case "__unit__":
			if to.Unit != "" {
				b.Set(l.Name, to.DirectUnit())
			}
		case "le":
			// NOTE(bwplotka): le renames are not possible.
			if len(vt.expr) == 0 {
				break nameswitch
			}
			if mTyp == model.MetricTypeHistogram {
				val, err := strconv.ParseFloat(l.Value, 64)
				if err != nil {
					fmt.Println("ERROR", err)
				}
				b.Set(l.Name, model.FloatString(vt.Transform(val)).String())
			}
		default:
			for a := range to.Attributes {
				if l.Name != from.Attributes[a].Tag {
					continue
				}
				b.Del(l.Name)
				for m := range from.Attributes[a].Members {
					if l.Value != from.Attributes[a].Members[m].Value {
						continue
					}
					b.Set(to.Attributes[a].Tag, to.Attributes[a].Members[m].Value)
					break nameswitch
				}
				b.Set(to.Attributes[a].Tag, l.Value)
				break nameswitch
			}
		}
	})
	return t.traverseForLabels(fromRev, toRev, mTyp, magicSuffix, b, vt)
}

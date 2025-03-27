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

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

const cacheTTL = 1 * time.Hour

type schemaEngine struct {
	// TODO(bwplotka): Implement GC logic for ttl and limits.
	cachedIDs        map[string]*ids
	cacheIDsMu       sync.RWMutex
	cachedChangelog  map[string]*changelog
	cacheChangelogMu sync.RWMutex
}

func newSchemaEngine() *schemaEngine {
	return &schemaEngine{
		cachedIDs:       map[string]*ids{},
		cachedChangelog: map[string]*changelog{},
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

func (e *schemaEngine) getMetricID(schemaURL string, matchers matcherBuilder) (metricID, string, error) {
	schemaVersion := path.Base(schemaURL)

	// TODO(bwplotka): This assumes such a file structure is part of the spec.
	ids, err := e.fetchIDs(schemaIDsURL(schemaURL))
	if err != nil {
		return "", "", fmt.Errorf("based on __schema_url__=%v; %w", schemaURL, err)
	}

	var (
		vid         []versionedID
		magicSuffix string
	)
	for _, suffix := range []string{"", "_bucket", "_count", "_sum"} {
		magicSuffix = suffix
		m := matchers.metric
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
			break
		}
	}
	if len(vid) == 0 {
		return "", "", fmt.Errorf("can't find metric ID in %v entry for version %v; this metric (with or without magic suffixes) is not part of this schema registry", matchers.metric.String(), schemaVersion)
	}

	for _, id := range vid {
		if natural.Less(schemaVersion, id.IntroVersion) {
			continue
		}
		return id.ID, magicSuffix, nil
	}
	return "", "", fmt.Errorf("can't find metric ID in %v entry for version %v", matchers.metric.String(), schemaVersion)
}

func (e *schemaEngine) fetchIDs(schemaIDsURL string) (_ *ids, err error) {
	e.cacheIDsMu.RLock()
	ids, ok := e.cachedIDs[schemaIDsURL]
	e.cacheIDsMu.RUnlock()
	if ok && time.Now().Sub(ids.fetchTime) < cacheTTL {
		return ids, nil
	}
	// Expired or missing.
	ids, err = fetchIDs(schemaIDsURL)
	if err != nil {
		return nil, err
	}
	e.cacheIDsMu.Lock()
	e.cachedIDs[schemaIDsURL] = ids
	e.cacheIDsMu.Unlock()
	return ids, nil
}

func (e *schemaEngine) fetchChangelog(schemaChangelogURL string) (_ *changelog, err error) {
	e.cacheChangelogMu.RLock()
	ch, ok := e.cachedChangelog[schemaChangelogURL]
	e.cacheChangelogMu.RUnlock()
	if ok && time.Now().Sub(ch.fetchTime) < cacheTTL {
		return ch, nil
	}
	// Expired or missing.
	ch, err = fetchChangelog(schemaChangelogURL)
	if err != nil {
		return nil, err
	}
	e.cacheChangelogMu.Lock()
	e.cachedChangelog[schemaChangelogURL] = ch
	e.cacheChangelogMu.Unlock()
	return ch, nil
}

func schemaChangelogURL(schemaURL string) string {
	// NOTE(bwplotka): Be careful with path as it cleans potential http:// to http:/
	dir, _ := path.Split(schemaURL)
	return fmt.Sprintf("%v/changelog.yaml", dir)
}

func schemaIDsURL(schemaURL string) string {
	// NOTE(bwplotka): Be careful with path as it cleans potential http:// to http:/
	dir, _ := path.Split(schemaURL)
	return fmt.Sprintf("%v/ids.yaml", dir)
}

// FindVariants returns all variants for a single schematized (referenced by schema_url) metric.
// It returns error if the given matchers does not point to a single metric or if schema or variants couldn't
// be detected.
func (e *schemaEngine) FindVariants(schemaURL string, originalMatchers []*labels.Matcher) (variants []*variant, _ error) {
	matchers, err := newMatcherBuilder(originalMatchers)
	if err != nil {
		return nil, err
	}

	mID, magicSuffix, err := e.getMetricID(schemaURL, matchers)
	if err != nil {
		return nil, fmt.Errorf("getMetricID: %w", err)
	}
	ch, err := e.fetchChangelog(schemaChangelogURL(schemaURL))
	if err != nil {
		return nil, err
	}

	// Original selection (without schema url).
	variants = append(variants, &variant{matchers: matchers.ToMatchers("")})

	sID, rev := mID.semanticID()
	changes, _ := ch.MetricsChangelog[sID]
	if len(changes) == 0 {
		// Unfortunately this (!ok) might also mean the malformed schema or cache.
		// __schema__id__ idea would be more robust here.
		// We could expect non-changed things in changelog, but that would
		// make changelog overly huge.
		return variants, nil
	}

	// Changes are sorted from the newest to the oldest, so reverse this, so
	// it's matches the revisions order.
	slices.Reverse(changes)

	// Revision starts with 0, then 2,3,4..., uniform it (0,1,2,3...).
	if rev != 0 {
		rev--
	}

	t := &changeTraverser{
		changes:     changes,
		magicSuffix: magicSuffix,
	}

	// Changelog contains changes across revisions, traverse forward and backward.
	variants, err = t.traverse(rev, false, matchers, resultTransform{}, variants)
	if err != nil {
		return nil, fmt.Errorf("can't traverse changes for semantic ID %v: %w", sID, err)
	}
	variants, err = t.traverse(rev, true, matchers, resultTransform{}, variants)
	if err != nil {
		return nil, fmt.Errorf("can't traverse changes for semantic ID %v: %w", sID, err)
	}
	return variants, nil
}

type resultTransform struct {
	to          metricGroupChange
	from        metricGroupChange
	vt          valueTransformer
	magicSuffix string
}

type variant struct {
	matchers []*labels.Matcher
	result   resultTransform
}

type changeTraverser struct {
	changes     []change
	magicSuffix string
}

// traverse builds the matchers and result transformations for the variant to be queried.
// It then walks further with the new matchers and result transformation as the base for the next change in the chain.
// This allows handling multi-version variants.
// TODO(bwplotka): Consider refactoring for clarity, it's complex, transition to math operations vs semantics.
func (t *changeTraverser) traverse(revision int, newer bool, b matcherBuilder, r resultTransform, v []*variant) ([]*variant, error) {
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

	// Transform matchers first. We have the `b` from the last traversal with potentially
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

	// Prepare result transformations. Here we don't have the series data yet, so
	// we need to prepare the full "from", "to" that will be used on the result.
	// Transformation [from -> to] is for matchers, for results we need to revert that.
	to, from = from, to

	// Update "to", so the final state. For final state, we prioritize the old values.
	if r.to.MetricName == "" {
		r.to.MetricName = to.MetricName
	}
	if r.to.Unit == "" {
		r.to.Unit = to.Unit
	}
	for _, newAttr := range to.Attributes {
		var found bool
		// To find relation we are checking the "from" part of the existing result.
		for _, oldAttr := range r.from.Attributes {
			if newAttr.Tag == oldAttr.Tag {
				found = true
				break
			}
		}
		if found {
			continue
		}
		r.to.Attributes = append(r.to.Attributes, newAttr)
	}

	// Update "from", so the current, expected data set for a queried variant.
	// Here we prioritize the new values.
	if from.MetricName != "" {
		r.from.MetricName = from.MetricName
	}
	if from.Unit != "" {
		r.from.Unit = from.Unit
	}
	for _, oldAttr := range r.from.Attributes {
		var found bool
		// To find relation we are checking the "to" part of the new result.
		for _, newAttr := range to.Attributes {
			if newAttr.Tag == oldAttr.Tag {
				found = true
				break
			}
		}
		if found {
			continue
		}
		from.Attributes = append(from.Attributes, oldAttr)
	}
	r.from.Attributes = from.Attributes

	// Value transformation for results just needs "to" to be accumulated.
	if to.ValuePromQL != "" {
		var err error
		r.vt, err = r.vt.AddPromQL(to.ValuePromQL)
		if err != nil {
			return nil, err
		}
	}
	return t.traverse(revision, newer, b, r, append(v, &variant{
		matchers: b.ToMatchers(t.magicSuffix),
		result:   r,
	}))
}

type transformingSeriesSet struct {
	storage.SeriesSet

	result resultTransform
}

// SeriesSet returns variant SeriesSet that transforms data on the fly
// based on variant to and from change details.
func (v *variant) SeriesSet(s storage.SeriesSet) storage.SeriesSet {
	return &transformingSeriesSet{SeriesSet: s, result: v.result}
}

type transformingSeries struct {
	storage.Series

	lbls labels.Labels

	result resultTransform
}

func (s *transformingSeriesSet) At() storage.Series {
	at := s.SeriesSet.At()
	return &transformingSeries{Series: at, lbls: at.Labels(), result: s.result}
}

func (s *transformingSeries) Labels() labels.Labels {
	typ := s.lbls.MetricIdentity().Type

	builder := labels.NewBuilder(s.lbls)
	builder.Range(func(l labels.Label) {

	nameswitch:
		switch l.Name {
		case labels.MetricName:
			if s.result.to.MetricName != "" {
				builder.Set(l.Name, s.result.to.MetricName+s.result.magicSuffix)
			}
		case "__type__":
			return
		case schemaURLLabel:
			// Explicitly remove __schema_url__ as that would be misleading.
			builder.Del(l.Name)
		case "__unit__":
			if s.result.to.Unit != "" {
				builder.Set(l.Name, s.result.to.DirectUnit())
			}
		case "le":
			if typ == model.MetricTypeHistogram {
				val, err := strconv.ParseFloat(l.Value, 64)
				if err != nil {
					fmt.Println("ERROR", err)
				}
				builder.Set(l.Name, model.FloatString(s.result.vt.Transform(val)).String())
			}
		default:
			for a := range s.result.to.Attributes {
				if l.Name != s.result.from.Attributes[a].Tag {
					continue
				}
				builder.Del(l.Name)
				for m := range s.result.from.Attributes[a].Members {
					if l.Value != s.result.from.Attributes[a].Members[m].Value {
						continue
					}
					builder.Set(s.result.to.Attributes[a].Tag, s.result.to.Attributes[a].Members[m].Value)
					break nameswitch
				}
				builder.Set(s.result.to.Attributes[a].Tag, l.Value)
				break nameswitch
			}
		}
	})
	return builder.Labels()
}

type transformingIterator struct {
	chunkenc.Iterator

	typ    model.MetricType
	result resultTransform
}

func (s *transformingSeries) Iterator(i chunkenc.Iterator) chunkenc.Iterator {
	return &transformingIterator{Iterator: s.Series.Iterator(i), typ: s.lbls.MetricIdentity().Type, result: s.result}
}

func (i *transformingIterator) At() (int64, float64) {
	t, v := i.Iterator.At()
	// TODO(bwplotka): Do the same for summaries.
	if i.typ == model.MetricTypeHistogram && (i.result.magicSuffix == "_count" || i.result.magicSuffix == "_bucket") {
		return t, v
	}
	return t, i.result.vt.Transform(v)
}

func (i *transformingIterator) AtHistogram(h *histogram.Histogram) (int64, *histogram.Histogram) {
	t, hist := i.Iterator.AtHistogram(h)
	// TODO: You can't really scale native histograms with exponential scheme. Handle this (error, approx, validation).

	if hist.UsesCustomBuckets() {
		hist = hist.Copy()
		hist.Sum = i.result.vt.Transform(hist.Sum)
		for cvi := range hist.CustomValues {
			hist.CustomValues[cvi] = i.result.vt.Transform(hist.CustomValues[cvi])
		}
	}
	return t, hist
}

func (i *transformingIterator) AtFloatHistogram(fh *histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	t, hist := i.Iterator.AtFloatHistogram(fh)
	// TODO: You can't really scale native histograms with exponential scheme. Handle this (error, approx, validation).

	if hist.UsesCustomBuckets() {
		hist = hist.Copy()
		hist.Sum = i.result.vt.Transform(hist.Sum)
		for cvi := range hist.CustomValues {
			hist.CustomValues[cvi] = i.result.vt.Transform(hist.CustomValues[cvi])
		}
	}
	return t, hist
}

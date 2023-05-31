// Copyright 2013 The Prometheus Authors
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

package scrape

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
)

// TargetHealth describes the health state of a target.
type TargetHealth string

// The possible health states of a target based on the last performed scrape.
const (
	HealthUnknown TargetHealth = "unknown"
	HealthGood    TargetHealth = "up"
	HealthBad     TargetHealth = "down"
)

// Target refers to a singular HTTP or HTTPS endpoint.
type Target struct {
	// Labels before any processing.
	discoveredLabels labels.Labels
	// Any labels that are added to this target and its metrics.
	labels labels.Labels
	// Additional URL parameters that are part of the target URL.
	params url.Values

	mtx                sync.RWMutex
	lastError          error
	lastScrape         time.Time
	lastScrapeDuration time.Duration
	health             TargetHealth
	metadata           MetricMetadataStore
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(labels, discoveredLabels labels.Labels, params url.Values) *Target {
	return &Target{
		labels:           labels,
		discoveredLabels: discoveredLabels,
		params:           params,
		health:           HealthUnknown,
	}
}

func (t *Target) String() string {
	return t.URL().String()
}

// MetricMetadataStore represents a storage for metadata.
type MetricMetadataStore interface {
	ListMetadata() []MetricMetadata
	GetMetadata(metric string) (MetricMetadata, bool)
	SizeMetadata() int
	LengthMetadata() int
}

// MetricMetadata is a piece of metadata for a metric.
type MetricMetadata struct {
	Metric string
	Type   textparse.MetricType
	Help   string
	Unit   string
}

func (t *Target) MetadataList() []MetricMetadata {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	if t.metadata == nil {
		return nil
	}
	return t.metadata.ListMetadata()
}

func (t *Target) MetadataSize() int {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	if t.metadata == nil {
		return 0
	}

	return t.metadata.SizeMetadata()
}

func (t *Target) MetadataLength() int {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	if t.metadata == nil {
		return 0
	}

	return t.metadata.LengthMetadata()
}

// Metadata returns type and help metadata for the given metric.
func (t *Target) Metadata(metric string) (MetricMetadata, bool) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	if t.metadata == nil {
		return MetricMetadata{}, false
	}
	return t.metadata.GetMetadata(metric)
}

func (t *Target) SetMetadataStore(s MetricMetadataStore) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.metadata = s
}

// hash returns an identifying hash for the target.
func (t *Target) hash() uint64 {
	h := fnv.New64a()

	//nolint: errcheck
	h.Write([]byte(fmt.Sprintf("%016d", t.labels.Hash())))
	//nolint: errcheck
	h.Write([]byte(t.URL().String()))

	return h.Sum64()
}

// offset returns the time until the next scrape cycle for the target.
// It includes the global server offsetSeed for scrapes from multiple Prometheus to try to be at different times.
func (t *Target) offset(interval time.Duration, offsetSeed uint64) time.Duration {
	now := time.Now().UnixNano()

	// Base is a pinned to absolute time, no matter how often offset is called.
	var (
		base   = int64(interval) - now%int64(interval)
		offset = (t.hash() ^ offsetSeed) % uint64(interval)
		next   = base + int64(offset)
	)

	if next > int64(interval) {
		next -= int64(interval)
	}
	return time.Duration(next)
}

// Labels returns a copy of the set of all public labels of the target.
func (t *Target) Labels() labels.Labels {
	b := labels.NewScratchBuilder(t.labels.Len())
	t.labels.Range(func(l labels.Label) {
		if !strings.HasPrefix(l.Name, model.ReservedLabelPrefix) {
			b.Add(l.Name, l.Value)
		}
	})
	return b.Labels()
}

// LabelsRange calls f on each public label of the target.
func (t *Target) LabelsRange(f func(l labels.Label)) {
	t.labels.Range(func(l labels.Label) {
		if !strings.HasPrefix(l.Name, model.ReservedLabelPrefix) {
			f(l)
		}
	})
}

// DiscoveredLabels returns a copy of the target's labels before any processing.
func (t *Target) DiscoveredLabels() labels.Labels {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return t.discoveredLabels.Copy()
}

// SetDiscoveredLabels sets new DiscoveredLabels
func (t *Target) SetDiscoveredLabels(l labels.Labels) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.discoveredLabels = l
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	params := url.Values{}

	for k, v := range t.params {
		params[k] = make([]string, len(v))
		copy(params[k], v)
	}
	t.labels.Range(func(l labels.Label) {
		if !strings.HasPrefix(l.Name, model.ParamLabelPrefix) {
			return
		}
		ks := l.Name[len(model.ParamLabelPrefix):]

		if len(params[ks]) > 0 {
			params[ks][0] = l.Value
		} else {
			params[ks] = []string{l.Value}
		}
	})

	return &url.URL{
		Scheme:   t.labels.Get(model.SchemeLabel),
		Host:     t.labels.Get(model.AddressLabel),
		Path:     t.labels.Get(model.MetricsPathLabel),
		RawQuery: params.Encode(),
	}
}

// Report sets target data about the last scrape.
func (t *Target) Report(start time.Time, dur time.Duration, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if err == nil {
		t.health = HealthGood
	} else {
		t.health = HealthBad
	}

	t.lastError = err
	t.lastScrape = start
	t.lastScrapeDuration = dur
}

// LastError returns the error encountered during the last scrape.
func (t *Target) LastError() error {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastError
}

// LastScrape returns the time of the last scrape.
func (t *Target) LastScrape() time.Time {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrape
}

// LastScrapeDuration returns how long the last scrape of the target took.
func (t *Target) LastScrapeDuration() time.Duration {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrapeDuration
}

// Health returns the last known health state of the target.
func (t *Target) Health() TargetHealth {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.health
}

// intervalAndTimeout returns the interval and timeout derived from
// the targets labels.
func (t *Target) intervalAndTimeout(defaultInterval, defaultDuration time.Duration) (time.Duration, time.Duration, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	intervalLabel := t.labels.Get(model.ScrapeIntervalLabel)
	interval, err := model.ParseDuration(intervalLabel)
	if err != nil {
		return defaultInterval, defaultDuration, errors.Errorf("Error parsing interval label %q: %v", intervalLabel, err)
	}
	timeoutLabel := t.labels.Get(model.ScrapeTimeoutLabel)
	timeout, err := model.ParseDuration(timeoutLabel)
	if err != nil {
		return defaultInterval, defaultDuration, errors.Errorf("Error parsing timeout label %q: %v", timeoutLabel, err)
	}

	return time.Duration(interval), time.Duration(timeout), nil
}

// GetValue gets a label value from the entire label set.
func (t *Target) GetValue(name string) string {
	return t.labels.Get(name)
}

// Targets is a sortable list of targets.
type Targets []*Target

func (ts Targets) Len() int           { return len(ts) }
func (ts Targets) Less(i, j int) bool { return ts[i].URL().String() < ts[j].URL().String() }
func (ts Targets) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }

var (
	errSampleLimit = errors.New("sample limit exceeded")
	errBucketLimit = errors.New("histogram bucket limit exceeded")
)

// limitAppender limits the number of total appended samples in a batch.
type limitAppender struct {
	storage.Appender

	limit int
	i     int
}

func (app *limitAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if !value.IsStaleNaN(v) {
		app.i++
		if app.i > app.limit {
			return 0, errSampleLimit
		}
	}
	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

type timeLimitAppender struct {
	storage.Appender

	maxTime int64
}

func (app *timeLimitAppender) Append(ref storage.SeriesRef, lset labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	if t > app.maxTime {
		return 0, storage.ErrOutOfBounds
	}

	ref, err := app.Appender.Append(ref, lset, t, v)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

// bucketLimitAppender limits the number of total appended samples in a batch.
type bucketLimitAppender struct {
	storage.Appender

	limit int
}

func (app *bucketLimitAppender) AppendHistogram(ref storage.SeriesRef, lset labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	if h != nil {
		if len(h.PositiveBuckets)+len(h.NegativeBuckets) > app.limit {
			return 0, errBucketLimit
		}
	}
	if fh != nil {
		if len(fh.PositiveBuckets)+len(fh.NegativeBuckets) > app.limit {
			return 0, errBucketLimit
		}
	}
	ref, err := app.Appender.AppendHistogram(ref, lset, t, h, fh)
	if err != nil {
		return 0, err
	}
	return ref, nil
}

// PopulateLabels builds a label set from the given label set and scrape configuration.
// It returns a label set before relabeling was applied as the second return value.
// Returns the original discovered label set found before relabelling was applied if the target is dropped during relabeling.
func PopulateLabels(lb *labels.Builder, cfg *config.ScrapeConfig, noDefaultPort bool) (res, orig labels.Labels, err error) {
	// Copy labels into the labelset for the target if they are not set already.
	scrapeLabels := []labels.Label{
		{Name: model.JobLabel, Value: cfg.JobName},
		{Name: model.ScrapeIntervalLabel, Value: cfg.ScrapeInterval.String()},
		{Name: model.ScrapeTimeoutLabel, Value: cfg.ScrapeTimeout.String()},
		{Name: model.MetricsPathLabel, Value: cfg.MetricsPath},
		{Name: model.SchemeLabel, Value: cfg.Scheme},
	}

	for _, l := range scrapeLabels {
		if lb.Get(l.Name) == "" {
			lb.Set(l.Name, l.Value)
		}
	}
	// Encode scrape query parameters as labels.
	for k, v := range cfg.Params {
		if len(v) > 0 {
			lb.Set(model.ParamLabelPrefix+k, v[0])
		}
	}

	preRelabelLabels := lb.Labels()
	keep := relabel.ProcessBuilder(lb, cfg.RelabelConfigs...)

	// Check if the target was dropped.
	if !keep {
		return labels.EmptyLabels(), preRelabelLabels, nil
	}
	if v := lb.Get(model.AddressLabel); v == "" {
		return labels.EmptyLabels(), labels.EmptyLabels(), errors.New("no address")
	}

	// addPort checks whether we should add a default port to the address.
	// If the address is not valid, we don't append a port either.
	addPort := func(s string) (string, string, bool) {
		// If we can split, a port exists and we don't have to add one.
		if host, port, err := net.SplitHostPort(s); err == nil {
			return host, port, false
		}
		// If adding a port makes it valid, the previous error
		// was not due to an invalid address and we can append a port.
		_, _, err := net.SplitHostPort(s + ":1234")
		return "", "", err == nil
	}

	addr := lb.Get(model.AddressLabel)
	scheme := lb.Get(model.SchemeLabel)
	host, port, add := addPort(addr)
	// If it's an address with no trailing port, infer it based on the used scheme
	// unless the no-default-scrape-port feature flag is present.
	if !noDefaultPort && add {
		// Addresses reaching this point are already wrapped in [] if necessary.
		switch scheme {
		case "http", "":
			addr += ":80"
		case "https":
			addr += ":443"
		default:
			return labels.EmptyLabels(), labels.EmptyLabels(), errors.Errorf("invalid scheme: %q", cfg.Scheme)
		}
		lb.Set(model.AddressLabel, addr)
	}

	if noDefaultPort {
		// If it's an address with a trailing default port and the
		// no-default-scrape-port flag is present, remove the port.
		switch port {
		case "80":
			if scheme == "http" {
				lb.Set(model.AddressLabel, host)
			}
		case "443":
			if scheme == "https" {
				lb.Set(model.AddressLabel, host)
			}
		}
	}

	if err := config.CheckTargetAddress(model.LabelValue(addr)); err != nil {
		return labels.EmptyLabels(), labels.EmptyLabels(), err
	}

	interval := lb.Get(model.ScrapeIntervalLabel)
	intervalDuration, err := model.ParseDuration(interval)
	if err != nil {
		return labels.EmptyLabels(), labels.EmptyLabels(), errors.Errorf("error parsing scrape interval: %v", err)
	}
	if time.Duration(intervalDuration) == 0 {
		return labels.EmptyLabels(), labels.EmptyLabels(), errors.New("scrape interval cannot be 0")
	}

	timeout := lb.Get(model.ScrapeTimeoutLabel)
	timeoutDuration, err := model.ParseDuration(timeout)
	if err != nil {
		return labels.EmptyLabels(), labels.EmptyLabels(), errors.Errorf("error parsing scrape timeout: %v", err)
	}
	if time.Duration(timeoutDuration) == 0 {
		return labels.EmptyLabels(), labels.EmptyLabels(), errors.New("scrape timeout cannot be 0")
	}

	if timeoutDuration > intervalDuration {
		return labels.EmptyLabels(), labels.EmptyLabels(), errors.Errorf("scrape timeout cannot be greater than scrape interval (%q > %q)", timeout, interval)
	}

	// Meta labels are deleted after relabelling. Other internal labels propagate to
	// the target which decides whether they will be part of their label set.
	lb.Range(func(l labels.Label) {
		if strings.HasPrefix(l.Name, model.MetaLabelPrefix) {
			lb.Del(l.Name)
		}
	})

	// Default the instance label to the target address.
	if v := lb.Get(model.InstanceLabel); v == "" {
		lb.Set(model.InstanceLabel, addr)
	}

	res = lb.Labels()
	err = res.Validate(func(l labels.Label) error {
		// Check label values are valid, drop the target if not.
		if !model.LabelValue(l.Value).IsValid() {
			return errors.Errorf("invalid label value for %q: %q", l.Name, l.Value)
		}
		return nil
	})
	if err != nil {
		return labels.EmptyLabels(), labels.EmptyLabels(), err
	}
	return res, preRelabelLabels, nil
}

// TargetsFromGroup builds targets based on the given TargetGroup and config.
func TargetsFromGroup(tg *targetgroup.Group, cfg *config.ScrapeConfig, noDefaultPort bool, targets []*Target, lb *labels.Builder) ([]*Target, []error) {
	targets = targets[:0]
	failures := []error{}

	for i, tlset := range tg.Targets {
		lb.Reset(labels.EmptyLabels())

		for ln, lv := range tlset {
			lb.Set(string(ln), string(lv))
		}
		for ln, lv := range tg.Labels {
			if _, ok := tlset[ln]; !ok {
				lb.Set(string(ln), string(lv))
			}
		}

		lset, origLabels, err := PopulateLabels(lb, cfg, noDefaultPort)
		if err != nil {
			failures = append(failures, errors.Wrapf(err, "instance %d in group %s", i, tg))
		}
		if !lset.IsEmpty() || !origLabels.IsEmpty() {
			targets = append(targets, NewTarget(lset, origLabels, cfg.Params))
		}
	}
	return targets, failures
}

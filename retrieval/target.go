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

package retrieval

import (
	"fmt"
	"hash/fnv"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/relabel"
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
	discoveredLabels model.LabelSet
	// Any labels that are added to this target and its metrics.
	labels model.LabelSet
	// Additional URL parmeters that are part of the target URL.
	params url.Values

	mtx        sync.RWMutex
	lastError  error
	lastScrape time.Time
	health     TargetHealth
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(labels, discoveredLabels model.LabelSet, params url.Values) *Target {
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

// hash returns an identifying hash for the target.
func (t *Target) hash() uint64 {
	h := fnv.New64a()
	h.Write([]byte(t.labels.Fingerprint().String()))
	h.Write([]byte(t.URL().String()))

	return h.Sum64()
}

// offset returns the time until the next scrape cycle for the target.
func (t *Target) offset(interval time.Duration) time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now % int64(interval)
		offset = t.hash() % uint64(interval)
		next   = base + int64(offset)
	)

	if next > int64(interval) {
		next -= int64(interval)
	}
	return time.Duration(next)
}

// Labels returns a copy of the set of all public labels of the target.
func (t *Target) Labels() model.LabelSet {
	lset := make(model.LabelSet, len(t.labels))
	for ln, lv := range t.labels {
		if !strings.HasPrefix(string(ln), model.ReservedLabelPrefix) {
			lset[ln] = lv
		}
	}
	return lset
}

// DiscoveredLabels returns a copy of the target's labels before any processing.
func (t *Target) DiscoveredLabels() model.LabelSet {
	return t.discoveredLabels.Clone()
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	params := url.Values{}

	for k, v := range t.params {
		params[k] = make([]string, len(v))
		copy(params[k], v)
	}
	for k, v := range t.labels {
		if !strings.HasPrefix(string(k), model.ParamLabelPrefix) {
			continue
		}
		ks := string(k[len(model.ParamLabelPrefix):])

		if len(params[ks]) > 0 {
			params[ks][0] = string(v)
		} else {
			params[ks] = []string{string(v)}
		}
	}

	return &url.URL{
		Scheme:   string(t.labels[model.SchemeLabel]),
		Host:     string(t.labels[model.AddressLabel]),
		Path:     string(t.labels[model.MetricsPathLabel]),
		RawQuery: params.Encode(),
	}
}

func (t *Target) report(start time.Time, dur time.Duration, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if err == nil {
		t.health = HealthGood
	} else {
		t.health = HealthBad
	}

	t.lastError = err
	t.lastScrape = start
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

// Health returns the last known health state of the target.
func (t *Target) Health() TargetHealth {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.health
}

// Targets is a sortable list of targets.
type Targets []*Target

func (ts Targets) Len() int           { return len(ts) }
func (ts Targets) Less(i, j int) bool { return ts[i].URL().String() < ts[j].URL().String() }
func (ts Targets) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }

// Merges the ingested sample's metric with the label set. On a collision the
// value of the ingested label is stored in a label prefixed with 'exported_'.
type ruleLabelsAppender struct {
	storage.SampleAppender
	labels model.LabelSet
}

func (app ruleLabelsAppender) Append(s *model.Sample) error {
	for ln, lv := range app.labels {
		if v, ok := s.Metric[ln]; ok && v != "" {
			s.Metric[model.ExportedLabelPrefix+ln] = v
		}
		s.Metric[ln] = lv
	}

	return app.SampleAppender.Append(s)
}

type honorLabelsAppender struct {
	storage.SampleAppender
	labels model.LabelSet
}

// Merges the sample's metric with the given labels if the label is not
// already present in the metric.
// This also considers labels explicitly set to the empty string.
func (app honorLabelsAppender) Append(s *model.Sample) error {
	for ln, lv := range app.labels {
		if _, ok := s.Metric[ln]; !ok {
			s.Metric[ln] = lv
		}
	}

	return app.SampleAppender.Append(s)
}

// Applies a set of relabel configurations to the sample's metric
// before actually appending it.
type relabelAppender struct {
	storage.SampleAppender
	relabelings []*config.RelabelConfig
}

func (app relabelAppender) Append(s *model.Sample) error {
	labels := relabel.Process(model.LabelSet(s.Metric), app.relabelings...)

	// Check if the timeseries was dropped.
	if labels == nil {
		return nil
	}
	s.Metric = model.Metric(labels)

	return app.SampleAppender.Append(s)
}

// bufferAppender appends samples to the given buffer.
type bufferAppender struct {
	buffer model.Samples
}

func (app *bufferAppender) Append(s *model.Sample) error {
	app.buffer = append(app.buffer, s)
	return nil
}

func (app *bufferAppender) NeedsThrottling() bool { return false }

// countingAppender counts the samples appended to the underlying appender.
type countingAppender struct {
	storage.SampleAppender
	count int
}

func (app *countingAppender) Append(s *model.Sample) error {
	app.count++
	return app.SampleAppender.Append(s)
}

// populateLabels builds a label set from the given label set and scrape configuration.
// It returns a label set before relabeling was applied as the second return value.
// Returns a nil label set if the target is dropped during relabeling.
func populateLabels(lset model.LabelSet, cfg *config.ScrapeConfig) (res, orig model.LabelSet, err error) {
	lset = lset.Clone()
	if _, ok := lset[model.AddressLabel]; !ok {
		return nil, nil, fmt.Errorf("no address")
	}
	// Copy labels into the labelset for the target if they are not
	// set already. Apply the labelsets in order of decreasing precedence.
	scrapeLabels := model.LabelSet{
		model.SchemeLabel:      model.LabelValue(cfg.Scheme),
		model.MetricsPathLabel: model.LabelValue(cfg.MetricsPath),
		model.JobLabel:         model.LabelValue(cfg.JobName),
	}
	for ln, lv := range scrapeLabels {
		if _, ok := lset[ln]; !ok {
			lset[ln] = lv
		}
	}
	// Encode scrape query parameters as labels.
	for k, v := range cfg.Params {
		if len(v) > 0 {
			lset[model.LabelName(model.ParamLabelPrefix+k)] = model.LabelValue(v[0])
		}
	}

	preRelabelLabels := lset.Clone()
	lset = relabel.Process(lset, cfg.RelabelConfigs...)

	// Check if the target was dropped.
	if lset == nil {
		return nil, nil, nil
	}

	// addPort checks whether we should add a default port to the address.
	// If the address is not valid, we don't append a port either.
	addPort := func(s string) bool {
		// If we can split, a port exists and we don't have to add one.
		if _, _, err := net.SplitHostPort(s); err == nil {
			return false
		}
		// If adding a port makes it valid, the previous error
		// was not due to an invalid address and we can append a port.
		_, _, err := net.SplitHostPort(s + ":1234")
		return err == nil
	}
	// If it's an address with no trailing port, infer it based on the used scheme.
	if addr := string(lset[model.AddressLabel]); addPort(addr) {
		// Addresses reaching this point are already wrapped in [] if necessary.
		switch lset[model.SchemeLabel] {
		case "http", "":
			addr = addr + ":80"
		case "https":
			addr = addr + ":443"
		default:
			return nil, nil, fmt.Errorf("invalid scheme: %q", cfg.Scheme)
		}
		lset[model.AddressLabel] = model.LabelValue(addr)
	}
	if err := config.CheckTargetAddress(lset[model.AddressLabel]); err != nil {
		return nil, nil, err
	}

	// Meta labels are deleted after relabelling. Other internal labels propagate to
	// the target which decides whether they will be part of their label set.
	for ln := range lset {
		if strings.HasPrefix(string(ln), model.MetaLabelPrefix) {
			delete(lset, ln)
		}
	}

	// Default the instance label to the target address.
	if _, ok := lset[model.InstanceLabel]; !ok {
		lset[model.InstanceLabel] = lset[model.AddressLabel]
	}
	return lset, preRelabelLabels, nil
}

// targetsFromGroup builds targets based on the given TargetGroup and config.
func targetsFromGroup(tg *config.TargetGroup, cfg *config.ScrapeConfig) ([]*Target, error) {
	targets := make([]*Target, 0, len(tg.Targets))

	for i, lset := range tg.Targets {
		// Combine target labels with target group labels.
		for ln, lv := range tg.Labels {
			if _, ok := lset[ln]; !ok {
				lset[ln] = lv
			}
		}
		labels, origLabels, err := populateLabels(lset, cfg)
		if err != nil {
			return nil, fmt.Errorf("instance %d in group %s: %s", i, tg, err)
		}
		if labels != nil {
			targets = append(targets, NewTarget(labels, origLabels, cfg.Params))
		}
	}
	return targets, nil
}

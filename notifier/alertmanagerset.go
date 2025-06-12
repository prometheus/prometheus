// Copyright The Prometheus Authors
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

package notifier

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/sigv4"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
)

// alertmanagerSet contains a set of Alertmanagers discovered via a group of service
// discovery definitions that have a common configuration on how alerts should be sent.
type alertmanagerSet struct {
	cfg    *config.AlertmanagerConfig
	client *http.Client
	opts   *Options

	metrics *alertMetrics

	mtx        sync.RWMutex
	ams        []alertmanager
	droppedAms []alertmanager
	buffers    map[string]*buffer
	logger     *slog.Logger
}

func newAlertmanagerSet(cfg *config.AlertmanagerConfig, opts *Options, logger *slog.Logger, metrics *alertMetrics) (*alertmanagerSet, error) {
	client, err := config_util.NewClientFromConfig(cfg.HTTPClientConfig, "alertmanager")
	if err != nil {
		return nil, err
	}
	t := client.Transport

	if cfg.SigV4Config != nil {
		t, err = sigv4.NewSigV4RoundTripper(cfg.SigV4Config, client.Transport)
		if err != nil {
			return nil, err
		}
	}

	client.Transport = t

	s := &alertmanagerSet{
		client:  client,
		cfg:     cfg,
		opts:    opts,
		buffers: make(map[string]*buffer),
		logger:  logger,
		metrics: metrics,
	}
	return s, nil
}

// sync extracts a deduplicated set of Alertmanager endpoints from a list
// of target groups definitions.
func (s *alertmanagerSet) sync(tgs []*targetgroup.Group) {
	allAms := []alertmanager{}
	allDroppedAms := []alertmanager{}

	for _, tg := range tgs {
		ams, droppedAms, err := AlertmanagerFromGroup(tg, s.cfg)
		if err != nil {
			s.logger.Error("Creating discovered Alertmanagers failed", "err", err)
			continue
		}
		allAms = append(allAms, ams...)
		allDroppedAms = append(allDroppedAms, droppedAms...)
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	previousAms := s.ams
	// Set new Alertmanagers and deduplicate them along their unique URL.
	s.ams = []alertmanager{}
	s.droppedAms = []alertmanager{}
	s.droppedAms = append(s.droppedAms, allDroppedAms...)
	seen := map[string]struct{}{}

	for _, am := range allAms {
		us := am.url().String()
		if _, ok := seen[us]; ok {
			continue
		}

		// This will initialize the Counters for the AM to 0.
		s.metrics.dropped.WithLabelValues(us)
		s.metrics.errors.WithLabelValues(us)
		s.metrics.sent.WithLabelValues(us)

		seen[us] = struct{}{}
		s.ams = append(s.ams, am)
	}
	s.startSendLoops(allAms)

	// Now remove counters for any removed Alertmanagers.
	for _, am := range previousAms {
		us := am.url().String()
		if _, ok := seen[us]; ok {
			continue
		}
		s.metrics.dropped.DeleteLabelValues(us)
		s.metrics.errors.DeleteLabelValues(us)
		s.metrics.latency.DeleteLabelValues(us)
		s.metrics.queueLength.DeleteLabelValues(us)
		s.metrics.sent.DeleteLabelValues(us)
		seen[us] = struct{}{}
	}
	s.cleanSendLoops(previousAms)
}

func (s *alertmanagerSet) configHash() (string, error) {
	b, err := yaml.Marshal(s.cfg)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(b)
	return hex.EncodeToString(hash[:]), nil
}

func (s *alertmanagerSet) send(alerts ...*Alert) map[string]int {
	dropped := make(map[string]int)

	if len(s.cfg.AlertRelabelConfigs) > 0 {
		alerts = relabelAlerts(s.cfg.AlertRelabelConfigs, labels.Labels{}, alerts)
		if len(alerts) == 0 {
			return dropped
		}
	}

	for am, q := range s.buffers {
		d := q.push(alerts...)
		dropped[am] += d
	}

	return dropped
}

// startSendLoops create buffers for newly discovered alertmanager and
// starts a send loop for each.
// This function expects the caller to acquire needed locks.
func (s *alertmanagerSet) startSendLoops(all []alertmanager) {
	for _, am := range all {
		us := am.url().String()
		// create new buffers and start send loops for new alertmanagers in the set.
		if _, ok := s.buffers[us]; !ok {
			s.buffers[us] = newBuffer(s.opts.QueueCapacity)
			go s.sendLoop(am)
		}
	}
}

// stopSendLoops stops the send loops for each removed alertmanager by
// closing and removing their respective buffers.
// This function expects the caller to acquire needed locks.
func (s *alertmanagerSet) cleanSendLoops(removed []alertmanager) {
	for _, am := range removed {
		us := am.url().String()
		s.buffers[us].close()
		delete(s.buffers, us)
	}
}

func (s *alertmanagerSet) sendLoop(am alertmanager) {
	url := am.url().String()

	// allocate an alerts buffer for alerts with length and capacity equal to max batch size.
	alerts := make([]*Alert, s.opts.MaxBatchSize)
	for {
		b := s.getBuffer(url)
		if b == nil {
			return
		}

		_, ok := <-b.hasWork
		if !ok {
			return
		}

		b.pop(&alerts)

		if !s.postNotifications(am, alerts) {
			s.metrics.dropped.WithLabelValues(url).Add(float64(len(alerts)))
		}
	}
}

func (s *alertmanagerSet) postNotifications(am alertmanager, alerts []*Alert) bool {
	if len(alerts) == 0 {
		return true
	}

	begin := time.Now()

	var payload []byte
	var err error
	switch s.cfg.APIVersion {
	case config.AlertmanagerAPIVersionV2:
		{
			openAPIAlerts := alertsToOpenAPIAlerts(alerts)

			payload, err = json.Marshal(openAPIAlerts)
			if err != nil {
				s.logger.Error("Encoding alerts for Alertmanager API v2 failed", "err", err)
				return false
			}
		}

	default:
		{
			s.logger.Error(
				fmt.Sprintf("Invalid Alertmanager API version '%v', expected one of '%v'", s.cfg.APIVersion, config.SupportedAlertmanagerAPIVersions),
				"err", err,
			)
			return false
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.Timeout))
	defer cancel()

	url := am.url().String()
	if err := s.sendOne(ctx, s.client, url, payload); err != nil {
		s.logger.Error("Error sending alerts", "alertmanager", url, "count", len(alerts), "err", err)
		s.metrics.errors.WithLabelValues(url).Add(float64(len(alerts)))
		return false
	}
	s.metrics.latency.WithLabelValues(url).Observe(time.Since(begin).Seconds())
	s.metrics.sent.WithLabelValues(url).Add(float64(len(alerts)))

	return true
}

func (s *alertmanagerSet) sendOne(ctx context.Context, c *http.Client, url string, b []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := s.opts.Do(ctx, c, req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Any HTTP status 2xx is OK.
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bad response status %s", resp.Status)
	}

	return nil
}

func (s *alertmanagerSet) getBuffer(url string) *buffer {
	s.mtx.RLock()
	defer s.mtx.RUnlock()
	if q, ok := s.buffers[url]; ok {
		return q
	}
	return nil
}

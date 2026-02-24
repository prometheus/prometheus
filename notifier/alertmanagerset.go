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
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/sigv4"
	"go.yaml.in/yaml/v2"

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
	sendLoops  map[string]*sendLoop

	logger *slog.Logger
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
		client:    client,
		cfg:       cfg,
		opts:      opts,
		sendLoops: make(map[string]*sendLoop),
		logger:    logger,
		metrics:   metrics,
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
	s.ams = []alertmanager{}
	s.droppedAms = []alertmanager{}
	s.droppedAms = append(s.droppedAms, allDroppedAms...)

	// Deduplicate Alertmanagers and add sendloops for new Alertmanagers.
	seen := map[string]struct{}{}
	for _, am := range allAms {
		us := am.url().String()
		if _, ok := seen[us]; ok {
			continue
		}

		seen[us] = struct{}{}
		s.ams = append(s.ams, am)
	}
	s.addSendLoops(s.ams)

	// Populate a list of Alertmanagers to clean up,
	// avoid cleaning up what we just added.
	for _, am := range previousAms {
		us := am.url().String()
		if _, ok := seen[us]; ok {
			continue
		}
		seen[us] = struct{}{}
		s.cleanSendLoops(am)
	}
}

func (s *alertmanagerSet) configHash() (string, error) {
	b, err := yaml.Marshal(s.cfg)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(b)
	return hex.EncodeToString(hash[:]), nil
}

func (s *alertmanagerSet) send(alerts ...*Alert) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if len(s.cfg.AlertRelabelConfigs) > 0 {
		alerts = relabelAlerts(s.cfg.AlertRelabelConfigs, labels.Labels{}, alerts)
		if len(alerts) == 0 {
			return
		}
	}

	for _, sendLoop := range s.sendLoops {
		sendLoop.add(alerts...)
	}
}

// addSendLoops creates and starts a send loop for newly discovered alertmanager.
// This function expects the caller to acquire needed locks.
func (s *alertmanagerSet) addSendLoops(ams []alertmanager) {
	for _, am := range ams {
		us := am.url().String()
		// Only add if sendloop doesn't already exist
		if loop, exists := s.sendLoops[us]; exists {
			loop.logger.Debug("Alertmanager already has send loop running, skipping")
			continue
		}
		sendLoop := newSendLoop(us, s.client, s.cfg, s.opts, s.logger.With("alertmanager", us), s.metrics)
		go sendLoop.loop()
		s.sendLoops[us] = sendLoop
	}
}

// cleanSendLoops stops and cleans the send loops for each removed alertmanager.
// This function expects the caller to acquire needed locks.
func (s *alertmanagerSet) cleanSendLoops(ams ...alertmanager) {
	for _, am := range ams {
		us := am.url().String()
		if sendLoop, ok := s.sendLoops[us]; ok {
			sendLoop.stop()
			delete(s.sendLoops, us)
		}
	}
}

// startSendLoops starts a send loop for newly discovered alertmanager.
// This function expects the caller to acquire needed locks.
// This is mainly needed for testing where the loops are added as part of the test setup.
func (s *alertmanagerSet) startSendLoops(ams []alertmanager) {
	for _, am := range ams {
		us := am.url().String()

		if l, ok := s.sendLoops[us]; ok {
			go l.loop()
			continue
		}
		panic(fmt.Sprintf("send loop not found for %s", us))
	}
}

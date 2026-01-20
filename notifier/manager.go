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
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/version"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

const (
	// DefaultMaxBatchSize is the default maximum number of alerts to send in a single request to the alertmanager.
	DefaultMaxBatchSize = 256

	contentTypeJSON = "application/json"
)

// String constants for instrumentation.
const (
	namespace         = "prometheus"
	subsystem         = "notifications"
	alertmanagerLabel = "alertmanager"
)

var userAgent = version.PrometheusUserAgent()

// Manager is responsible for dispatching alert notifications to an
// alert manager service.
type Manager struct {
	opts *Options

	metrics *alertMetrics

	mtx sync.RWMutex

	stopOnce      *sync.Once
	stopRequested chan struct{}

	alertmanagers map[string]*alertmanagerSet
	logger        *slog.Logger
}

// Options are the configurable parameters of a Handler.
type Options struct {
	QueueCapacity   int
	DrainOnShutdown bool
	ExternalLabels  labels.Labels
	RelabelConfigs  []*relabel.Config
	// Used for sending HTTP requests to the Alertmanager.
	Do func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)

	Registerer prometheus.Registerer

	// MaxBatchSize determines the maximum number of alerts to send in a single request to the alertmanager.
	MaxBatchSize int
}

func do(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req.WithContext(ctx))
}

// NewManager is the manager constructor.
func NewManager(o *Options, nameValidationScheme model.ValidationScheme, logger *slog.Logger) *Manager {
	if o.Do == nil {
		o.Do = do
	}
	// Set default MaxBatchSize if not provided.
	if o.MaxBatchSize <= 0 {
		o.MaxBatchSize = DefaultMaxBatchSize
	}
	if logger == nil {
		logger = promslog.NewNopLogger()
	}

	for _, rc := range o.RelabelConfigs {
		switch rc.NameValidationScheme {
		case model.LegacyValidation, model.UTF8Validation:
		default:
			rc.NameValidationScheme = nameValidationScheme
		}
	}

	n := &Manager{
		stopRequested: make(chan struct{}),
		stopOnce:      &sync.Once{},
		opts:          o,
		logger:        logger,
	}

	alertmanagersDiscoveredFunc := func() float64 { return float64(len(n.Alertmanagers())) }

	n.metrics = newAlertMetrics(o.Registerer, alertmanagersDiscoveredFunc)
	n.metrics.queueCapacity.Set(float64(o.QueueCapacity))

	return n
}

// ApplyConfig updates the status state as the new config requires.
func (n *Manager) ApplyConfig(conf *config.Config) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.opts.ExternalLabels = conf.GlobalConfig.ExternalLabels
	n.opts.RelabelConfigs = conf.AlertingConfig.AlertRelabelConfigs
	for i, rc := range n.opts.RelabelConfigs {
		switch rc.NameValidationScheme {
		case model.LegacyValidation, model.UTF8Validation:
		default:
			n.opts.RelabelConfigs[i].NameValidationScheme = conf.GlobalConfig.MetricNameValidationScheme
		}
	}

	amSets := make(map[string]*alertmanagerSet)
	// configToAlertmanagers maps alertmanager sets for each unique AlertmanagerConfig,
	// helping to avoid dropping known alertmanagers and re-use them without waiting for SD updates when applying the config.
	configToAlertmanagers := make(map[string]*alertmanagerSet, len(n.alertmanagers))
	for _, oldAmSet := range n.alertmanagers {
		hash, err := oldAmSet.configHash()
		if err != nil {
			return err
		}
		configToAlertmanagers[hash] = oldAmSet
	}

	for k, cfg := range conf.AlertingConfig.AlertmanagerConfigs.ToMap() {
		ams, err := newAlertmanagerSet(cfg, n.opts, n.logger, n.metrics)
		if err != nil {
			return err
		}

		hash, err := ams.configHash()
		if err != nil {
			return err
		}

		if oldAmSet, ok := configToAlertmanagers[hash]; ok {
			ams.ams = oldAmSet.ams
			ams.droppedAms = oldAmSet.droppedAms
			ams.sendLoops = oldAmSet.sendLoops
		}

		amSets[k] = ams
	}

	// Clean up the send loops of sets that don't exist in the new config.
	for k, oldAmSet := range n.alertmanagers {
		if _, exists := amSets[k]; !exists {
			oldAmSet.mtx.Lock()
			oldAmSet.cleanSendLoops(oldAmSet.ams...)
			oldAmSet.mtx.Unlock()
		}
	}

	n.alertmanagers = amSets

	return nil
}

// Run dispatches notifications continuously, returning once Stop has been called and all
// pending notifications have been drained from the queue (if draining is enabled).
//
// Dispatching of notifications occurs in parallel to processing target updates to avoid one starving the other.
// Refer to https://github.com/prometheus/prometheus/issues/13676 for more details.
func (n *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) {
	n.targetUpdateLoop(tsets)

	n.mtx.Lock()
	defer n.mtx.Unlock()
	for _, ams := range n.alertmanagers {
		ams.mtx.Lock()
		ams.cleanSendLoops(ams.ams...)
		ams.mtx.Unlock()
	}
}

// targetUpdateLoop receives updates of target groups and triggers a reload.
func (n *Manager) targetUpdateLoop(tsets <-chan map[string][]*targetgroup.Group) {
	for {
		// If we've been asked to stop, that takes priority over processing any further target group updates.
		select {
		case <-n.stopRequested:
			return
		default:
			select {
			case <-n.stopRequested:
				return
			case ts, ok := <-tsets:
				if !ok {
					break
				}
				n.reload(ts)
			}
		}
	}
}

func (n *Manager) reload(tgs map[string][]*targetgroup.Group) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	for id, tgroup := range tgs {
		am, ok := n.alertmanagers[id]
		if !ok {
			n.logger.Error("couldn't sync alert manager set", "err", fmt.Sprintf("invalid id:%v", id))
			continue
		}
		am.sync(tgroup)
	}
}

// Send queues the given notification requests for processing.
// Panics if called on a handler that is not running.
func (n *Manager) Send(alerts ...*Alert) {
	// If we've been asked to stop, that takes priority over accepting new alerts.
	select {
	case <-n.stopRequested:
		return
	default:
	}

	n.mtx.RLock()
	defer n.mtx.RUnlock()

	alerts = relabelAlerts(n.opts.RelabelConfigs, n.opts.ExternalLabels, alerts)
	if len(alerts) == 0 {
		return
	}

	for _, ams := range n.alertmanagers {
		ams.send(alerts...)
	}
}

// Alertmanagers returns a slice of Alertmanager URLs.
func (n *Manager) Alertmanagers() []*url.URL {
	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var res []*url.URL

	for _, ams := range amSets {
		ams.mtx.RLock()
		for _, am := range ams.ams {
			res = append(res, am.url())
		}
		ams.mtx.RUnlock()
	}

	return res
}

// DroppedAlertmanagers returns a slice of Alertmanager URLs.
func (n *Manager) DroppedAlertmanagers() []*url.URL {
	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var res []*url.URL

	for _, ams := range amSets {
		ams.mtx.RLock()
		for _, dam := range ams.droppedAms {
			res = append(res, dam.url())
		}
		ams.mtx.RUnlock()
	}

	return res
}

// Stop signals the notification manager to shut down and immediately returns.
//
// Run will return once the notification manager has successfully shut down.
//
// The manager will optionally drain send loops before shutting down.
//
// Stop is safe to call multiple times.
func (n *Manager) Stop() {
	n.logger.Info("Stopping notification manager...")

	n.stopOnce.Do(func() {
		close(n.stopRequested)
	})
}

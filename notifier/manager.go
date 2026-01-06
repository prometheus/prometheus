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
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

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
	queue []*Alert
	opts  *Options

	metrics *alertMetrics

	more chan struct{}
	mtx  sync.RWMutex

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
		queue:         make([]*Alert, 0, o.QueueCapacity),
		more:          make(chan struct{}, 1),
		stopRequested: make(chan struct{}),
		stopOnce:      &sync.Once{},
		opts:          o,
		logger:        logger,
	}

	queueLenFunc := func() float64 { return float64(n.queueLen()) }
	alertmanagersDiscoveredFunc := func() float64 { return float64(len(n.Alertmanagers())) }

	n.metrics = newAlertMetrics(
		o.Registerer,
		o.QueueCapacity,
		queueLenFunc,
		alertmanagersDiscoveredFunc,
	)

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
		ams, err := newAlertmanagerSet(cfg, n.logger, n.metrics)
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
		}

		amSets[k] = ams
	}

	n.alertmanagers = amSets

	return nil
}

func (n *Manager) queueLen() int {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return len(n.queue)
}

func (n *Manager) nextBatch() []*Alert {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	var alerts []*Alert

	if maxBatchSize := n.opts.MaxBatchSize; len(n.queue) > maxBatchSize {
		alerts = append(make([]*Alert, 0, maxBatchSize), n.queue[:maxBatchSize]...)
		n.queue = n.queue[maxBatchSize:]
	} else {
		alerts = append(make([]*Alert, 0, len(n.queue)), n.queue...)
		n.queue = n.queue[:0]
	}

	return alerts
}

// Run dispatches notifications continuously, returning once Stop has been called and all
// pending notifications have been drained from the queue (if draining is enabled).
//
// Dispatching of notifications occurs in parallel to processing target updates to avoid one starving the other.
// Refer to https://github.com/prometheus/prometheus/issues/13676 for more details.
func (n *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		n.targetUpdateLoop(tsets)
	}()

	go func() {
		defer wg.Done()
		n.sendLoop()
		n.drainQueue()
	}()

	wg.Wait()
	n.logger.Info("Notification manager stopped")
}

// sendLoop continuously consumes the notifications queue and sends alerts to
// the configured Alertmanagers.
func (n *Manager) sendLoop() {
	for {
		// If we've been asked to stop, that takes priority over sending any further notifications.
		select {
		case <-n.stopRequested:
			return
		default:
			select {
			case <-n.stopRequested:
				return

			case <-n.more:
				n.sendOneBatch()

				// If the queue still has items left, kick off the next iteration.
				if n.queueLen() > 0 {
					n.setMore()
				}
			}
		}
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

func (n *Manager) sendOneBatch() {
	alerts := n.nextBatch()

	if !n.sendAll(alerts...) {
		n.metrics.dropped.Add(float64(len(alerts)))
	}
}

func (n *Manager) drainQueue() {
	if !n.opts.DrainOnShutdown {
		if n.queueLen() > 0 {
			n.logger.Warn("Draining remaining notifications on shutdown is disabled, and some notifications have been dropped", "count", n.queueLen())
			n.metrics.dropped.Add(float64(n.queueLen()))
		}

		return
	}

	n.logger.Info("Draining any remaining notifications...")

	for n.queueLen() > 0 {
		n.sendOneBatch()
	}

	n.logger.Info("Remaining notifications drained")
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
	n.mtx.Lock()
	defer n.mtx.Unlock()

	alerts = relabelAlerts(n.opts.RelabelConfigs, n.opts.ExternalLabels, alerts)
	if len(alerts) == 0 {
		return
	}

	// Queue capacity should be significantly larger than a single alert
	// batch could be.
	if d := len(alerts) - n.opts.QueueCapacity; d > 0 {
		alerts = alerts[d:]

		n.logger.Warn("Alert batch larger than queue capacity, dropping alerts", "num_dropped", d)
		n.metrics.dropped.Add(float64(d))
	}

	// If the queue is full, remove the oldest alerts in favor
	// of newer ones.
	if d := (len(n.queue) + len(alerts)) - n.opts.QueueCapacity; d > 0 {
		n.queue = n.queue[d:]

		n.logger.Warn("Alert notification queue full, dropping alerts", "num_dropped", d)
		n.metrics.dropped.Add(float64(d))
	}
	n.queue = append(n.queue, alerts...)

	// Notify sending goroutine that there are alerts to be processed.
	n.setMore()
}

// setMore signals that the alert queue has items.
func (n *Manager) setMore() {
	// If we cannot send on the channel, it means the signal already exists
	// and has not been consumed yet.
	select {
	case n.more <- struct{}{}:
	default:
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

// sendAll sends the alerts to all configured Alertmanagers concurrently.
// It returns true if the alerts could be sent successfully to at least one Alertmanager.
func (n *Manager) sendAll(alerts ...*Alert) bool {
	if len(alerts) == 0 {
		return true
	}

	begin := time.Now()

	// cachedPayload represent 'alerts' marshaled for Alertmanager API v2.
	// Marshaling happens below. Reference here is for caching between
	// for loop iterations.
	var cachedPayload []byte

	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var (
		wg           sync.WaitGroup
		amSetCovered sync.Map
	)
	for k, ams := range amSets {
		var (
			payload  []byte
			err      error
			amAlerts = alerts
		)

		ams.mtx.RLock()

		if len(ams.ams) == 0 {
			ams.mtx.RUnlock()
			continue
		}

		if len(ams.cfg.AlertRelabelConfigs) > 0 {
			amAlerts = relabelAlerts(ams.cfg.AlertRelabelConfigs, labels.Labels{}, alerts)
			if len(amAlerts) == 0 {
				ams.mtx.RUnlock()
				continue
			}
			// We can't use the cached values from previous iteration.
			cachedPayload = nil
		}

		switch ams.cfg.APIVersion {
		case config.AlertmanagerAPIVersionV2:
			{
				if cachedPayload == nil {
					openAPIAlerts := alertsToOpenAPIAlerts(amAlerts)

					cachedPayload, err = json.Marshal(openAPIAlerts)
					if err != nil {
						n.logger.Error("Encoding alerts for Alertmanager API v2 failed", "err", err)
						ams.mtx.RUnlock()
						return false
					}
				}

				payload = cachedPayload
			}
		default:
			{
				n.logger.Error(
					fmt.Sprintf("Invalid Alertmanager API version '%v', expected one of '%v'", ams.cfg.APIVersion, config.SupportedAlertmanagerAPIVersions),
					"err", err,
				)
				ams.mtx.RUnlock()
				return false
			}
		}

		if len(ams.cfg.AlertRelabelConfigs) > 0 {
			// We can't use the cached values on the next iteration.
			cachedPayload = nil
		}

		// Being here means len(ams.ams) > 0
		amSetCovered.Store(k, false)
		for _, am := range ams.ams {
			wg.Add(1)

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ams.cfg.Timeout))
			defer cancel()

			go func(ctx context.Context, k string, client *http.Client, url string, payload []byte, count int) {
				err := n.sendOne(ctx, client, url, payload)
				if err != nil {
					n.logger.Error("Error sending alerts", "alertmanager", url, "count", count, "err", err)
					n.metrics.errors.WithLabelValues(url).Add(float64(count))
				} else {
					amSetCovered.CompareAndSwap(k, false, true)
				}

				durationSeconds := time.Since(begin).Seconds()
				n.metrics.latencySummary.WithLabelValues(url).Observe(durationSeconds)
				n.metrics.latencyHistogram.WithLabelValues(url).Observe(durationSeconds)
				n.metrics.sent.WithLabelValues(url).Add(float64(count))

				wg.Done()
			}(ctx, k, ams.client, am.url().String(), payload, len(amAlerts))
		}

		ams.mtx.RUnlock()
	}

	wg.Wait()

	// Return false if there are any sets which were attempted (e.g. not filtered
	// out) but have no successes.
	allAmSetsCovered := true
	amSetCovered.Range(func(_, value any) bool {
		if !value.(bool) {
			allAmSetsCovered = false
			return false
		}
		return true
	})

	return allAmSetsCovered
}

func (n *Manager) sendOne(ctx context.Context, c *http.Client, url string, b []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := n.opts.Do(ctx, c, req)
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

// Stop signals the notification manager to shut down and immediately returns.
//
// Run will return once the notification manager has successfully shut down.
//
// The manager will optionally drain any queued notifications before shutting down.
//
// Stop is safe to call multiple times.
func (n *Manager) Stop() {
	n.logger.Info("Stopping notification manager...")

	n.stopOnce.Do(func() {
		close(n.stopRequested)
	})
}

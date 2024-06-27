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

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/sigv4"
	"github.com/prometheus/common/version"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

const (
	contentTypeJSON = "application/json"
)

// String constants for instrumentation.
const (
	namespace         = "prometheus"
	subsystem         = "notifications"
	alertmanagerLabel = "alertmanager"
)

var userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

// Alert is a generic representation of an alert in the Prometheus eco-system.
type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels labels.Labels `json:"labels"`

	// Extra key/value information which does not define alert identity.
	Annotations labels.Labels `json:"annotations"`

	// The known time range for this alert. Both ends are optional.
	StartsAt     time.Time `json:"startsAt,omitempty"`
	EndsAt       time.Time `json:"endsAt,omitempty"`
	GeneratorURL string    `json:"generatorURL,omitempty"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return a.Labels.Get(labels.AlertName)
}

// Hash returns a hash over the alert. It is equivalent to the alert labels hash.
func (a *Alert) Hash() uint64 {
	return a.Labels.Hash()
}

func (a *Alert) String() string {
	s := fmt.Sprintf("%s[%s]", a.Name(), fmt.Sprintf("%016x", a.Hash())[:7])
	if a.Resolved() {
		return s + "[resolved]"
	}
	return s + "[active]"
}

// Resolved returns true iff the activity interval ended in the past.
func (a *Alert) Resolved() bool {
	return a.ResolvedAt(time.Now())
}

// ResolvedAt returns true iff the activity interval ended before
// the given timestamp.
func (a *Alert) ResolvedAt(ts time.Time) bool {
	if a.EndsAt.IsZero() {
		return false
	}
	return !a.EndsAt.After(ts)
}

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
	logger        log.Logger
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
}

type alertMetrics struct {
	latency                 *prometheus.SummaryVec
	errors                  *prometheus.CounterVec
	sent                    *prometheus.CounterVec
	dropped                 prometheus.Counter
	queueLength             prometheus.GaugeFunc
	queueCapacity           prometheus.Gauge
	alertmanagersDiscovered prometheus.GaugeFunc
}

func newAlertMetrics(r prometheus.Registerer, queueCap int, queueLen, alertmanagersDiscovered func() float64) *alertMetrics {
	m := &alertMetrics{
		latency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  subsystem,
			Name:       "latency_seconds",
			Help:       "Latency quantiles for sending alert notifications.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
			[]string{alertmanagerLabel},
		),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors sending alert notifications.",
		},
			[]string{alertmanagerLabel},
		),
		sent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_total",
			Help:      "Total number of alerts sent.",
		},
			[]string{alertmanagerLabel},
		),
		dropped: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dropped_total",
			Help:      "Total number of alerts dropped due to errors when sending to Alertmanager.",
		}),
		queueLength: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_length",
			Help:      "The number of alert notifications in the queue.",
		}, queueLen),
		queueCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_capacity",
			Help:      "The capacity of the alert notifications queue.",
		}),
		alertmanagersDiscovered: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_notifications_alertmanagers_discovered",
			Help: "The number of alertmanagers discovered and active.",
		}, alertmanagersDiscovered),
	}

	m.queueCapacity.Set(float64(queueCap))

	if r != nil {
		r.MustRegister(
			m.latency,
			m.errors,
			m.sent,
			m.dropped,
			m.queueLength,
			m.queueCapacity,
			m.alertmanagersDiscovered,
		)
	}

	return m
}

func do(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req.WithContext(ctx))
}

// NewManager is the manager constructor.
func NewManager(o *Options, logger log.Logger) *Manager {
	if o.Do == nil {
		o.Do = do
	}
	if logger == nil {
		logger = log.NewNopLogger()
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

	amSets := make(map[string]*alertmanagerSet)

	for k, cfg := range conf.AlertingConfig.AlertmanagerConfigs.ToMap() {
		ams, err := newAlertmanagerSet(cfg, n.logger, n.metrics)
		if err != nil {
			return err
		}

		amSets[k] = ams
	}

	n.alertmanagers = amSets

	return nil
}

const maxBatchSize = 64

func (n *Manager) queueLen() int {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return len(n.queue)
}

func (n *Manager) nextBatch() []*Alert {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	var alerts []*Alert

	if len(n.queue) > maxBatchSize {
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
	level.Info(n.logger).Log("msg", "Notification manager stopped")
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
			case ts := <-tsets:
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
			level.Warn(n.logger).Log("msg", "Draining remaining notifications on shutdown is disabled, and some notifications have been dropped", "count", n.queueLen())
			n.metrics.dropped.Add(float64(n.queueLen()))
		}

		return
	}

	level.Info(n.logger).Log("msg", "Draining any remaining notifications...")

	for n.queueLen() > 0 {
		n.sendOneBatch()
	}

	level.Info(n.logger).Log("msg", "Remaining notifications drained")
}

func (n *Manager) reload(tgs map[string][]*targetgroup.Group) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	for id, tgroup := range tgs {
		am, ok := n.alertmanagers[id]
		if !ok {
			level.Error(n.logger).Log("msg", "couldn't sync alert manager set", "err", fmt.Sprintf("invalid id:%v", id))
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

		level.Warn(n.logger).Log("msg", "Alert batch larger than queue capacity, dropping alerts", "num_dropped", d)
		n.metrics.dropped.Add(float64(d))
	}

	// If the queue is full, remove the oldest alerts in favor
	// of newer ones.
	if d := (len(n.queue) + len(alerts)) - n.opts.QueueCapacity; d > 0 {
		n.queue = n.queue[d:]

		level.Warn(n.logger).Log("msg", "Alert notification queue full, dropping alerts", "num_dropped", d)
		n.metrics.dropped.Add(float64(d))
	}
	n.queue = append(n.queue, alerts...)

	// Notify sending goroutine that there are alerts to be processed.
	n.setMore()
}

func relabelAlerts(relabelConfigs []*relabel.Config, externalLabels labels.Labels, alerts []*Alert) []*Alert {
	lb := labels.NewBuilder(labels.EmptyLabels())
	var relabeledAlerts []*Alert

	for _, a := range alerts {
		lb.Reset(a.Labels)
		externalLabels.Range(func(l labels.Label) {
			if a.Labels.Get(l.Name) == "" {
				lb.Set(l.Name, l.Value)
			}
		})

		keep := relabel.ProcessBuilder(lb, relabelConfigs...)
		if !keep {
			continue
		}
		a.Labels = lb.Labels()
		relabeledAlerts = append(relabeledAlerts, a)
	}
	return relabeledAlerts
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

	// v1Payload and v2Payload represent 'alerts' marshaled for Alertmanager API
	// v1 or v2. Marshaling happens below. Reference here is for caching between
	// for loop iterations.
	var v1Payload, v2Payload []byte

	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var (
		wg         sync.WaitGroup
		numSuccess atomic.Uint64
	)
	for _, ams := range amSets {
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
			v1Payload, v2Payload = nil, nil
		}

		switch ams.cfg.APIVersion {
		case config.AlertmanagerAPIVersionV1:
			{
				if v1Payload == nil {
					v1Payload, err = json.Marshal(amAlerts)
					if err != nil {
						level.Error(n.logger).Log("msg", "Encoding alerts for Alertmanager API v1 failed", "err", err)
						ams.mtx.RUnlock()
						return false
					}
				}

				payload = v1Payload
			}
		case config.AlertmanagerAPIVersionV2:
			{
				if v2Payload == nil {
					openAPIAlerts := alertsToOpenAPIAlerts(amAlerts)

					v2Payload, err = json.Marshal(openAPIAlerts)
					if err != nil {
						level.Error(n.logger).Log("msg", "Encoding alerts for Alertmanager API v2 failed", "err", err)
						ams.mtx.RUnlock()
						return false
					}
				}

				payload = v2Payload
			}
		default:
			{
				level.Error(n.logger).Log(
					"msg", fmt.Sprintf("Invalid Alertmanager API version '%v', expected one of '%v'", ams.cfg.APIVersion, config.SupportedAlertmanagerAPIVersions),
					"err", err,
				)
				ams.mtx.RUnlock()
				return false
			}
		}

		if len(ams.cfg.AlertRelabelConfigs) > 0 {
			// We can't use the cached values on the next iteration.
			v1Payload, v2Payload = nil, nil
		}

		for _, am := range ams.ams {
			wg.Add(1)

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ams.cfg.Timeout))
			defer cancel()

			go func(ctx context.Context, client *http.Client, url string, payload []byte, count int) {
				if err := n.sendOne(ctx, client, url, payload); err != nil {
					level.Error(n.logger).Log("alertmanager", url, "count", count, "msg", "Error sending alert", "err", err)
					n.metrics.errors.WithLabelValues(url).Inc()
				} else {
					numSuccess.Inc()
				}
				n.metrics.latency.WithLabelValues(url).Observe(time.Since(begin).Seconds())
				n.metrics.sent.WithLabelValues(url).Add(float64(len(amAlerts)))

				wg.Done()
			}(ctx, ams.client, am.url().String(), payload, len(amAlerts))
		}

		ams.mtx.RUnlock()
	}

	wg.Wait()

	return numSuccess.Load() > 0
}

func alertsToOpenAPIAlerts(alerts []*Alert) models.PostableAlerts {
	openAPIAlerts := models.PostableAlerts{}
	for _, a := range alerts {
		start := strfmt.DateTime(a.StartsAt)
		end := strfmt.DateTime(a.EndsAt)
		openAPIAlerts = append(openAPIAlerts, &models.PostableAlert{
			Annotations: labelsToOpenAPILabelSet(a.Annotations),
			EndsAt:      end,
			StartsAt:    start,
			Alert: models.Alert{
				GeneratorURL: strfmt.URI(a.GeneratorURL),
				Labels:       labelsToOpenAPILabelSet(a.Labels),
			},
		})
	}

	return openAPIAlerts
}

func labelsToOpenAPILabelSet(modelLabelSet labels.Labels) models.LabelSet {
	apiLabelSet := models.LabelSet{}
	modelLabelSet.Range(func(label labels.Label) {
		apiLabelSet[label.Name] = label.Value
	})

	return apiLabelSet
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
	//nolint:usestdlibvars
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
	level.Info(n.logger).Log("msg", "Stopping notification manager...")

	n.stopOnce.Do(func() {
		close(n.stopRequested)
	})
}

// Alertmanager holds Alertmanager endpoint information.
type alertmanager interface {
	url() *url.URL
}

type alertmanagerLabels struct{ labels.Labels }

const pathLabel = "__alerts_path__"

func (a alertmanagerLabels) url() *url.URL {
	return &url.URL{
		Scheme: a.Get(model.SchemeLabel),
		Host:   a.Get(model.AddressLabel),
		Path:   a.Get(pathLabel),
	}
}

// alertmanagerSet contains a set of Alertmanagers discovered via a group of service
// discovery definitions that have a common configuration on how alerts should be sent.
type alertmanagerSet struct {
	cfg    *config.AlertmanagerConfig
	client *http.Client

	metrics *alertMetrics

	mtx        sync.RWMutex
	ams        []alertmanager
	droppedAms []alertmanager
	logger     log.Logger
}

func newAlertmanagerSet(cfg *config.AlertmanagerConfig, logger log.Logger, metrics *alertMetrics) (*alertmanagerSet, error) {
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
			level.Error(s.logger).Log("msg", "Creating discovered Alertmanagers failed", "err", err)
			continue
		}
		allAms = append(allAms, ams...)
		allDroppedAms = append(allDroppedAms, droppedAms...)
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
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
		s.metrics.sent.WithLabelValues(us)
		s.metrics.errors.WithLabelValues(us)

		seen[us] = struct{}{}
		s.ams = append(s.ams, am)
	}
}

func postPath(pre string, v config.AlertmanagerAPIVersion) string {
	alertPushEndpoint := fmt.Sprintf("/api/%v/alerts", string(v))
	return path.Join("/", pre, alertPushEndpoint)
}

// AlertmanagerFromGroup extracts a list of alertmanagers from a target group
// and an associated AlertmanagerConfig.
func AlertmanagerFromGroup(tg *targetgroup.Group, cfg *config.AlertmanagerConfig) ([]alertmanager, []alertmanager, error) {
	var res []alertmanager
	var droppedAlertManagers []alertmanager
	lb := labels.NewBuilder(labels.EmptyLabels())

	for _, tlset := range tg.Targets {
		lb.Reset(labels.EmptyLabels())

		for ln, lv := range tlset {
			lb.Set(string(ln), string(lv))
		}
		// Set configured scheme as the initial scheme label for overwrite.
		lb.Set(model.SchemeLabel, cfg.Scheme)
		lb.Set(pathLabel, postPath(cfg.PathPrefix, cfg.APIVersion))

		// Combine target labels with target group labels.
		for ln, lv := range tg.Labels {
			if _, ok := tlset[ln]; !ok {
				lb.Set(string(ln), string(lv))
			}
		}

		preRelabel := lb.Labels()
		keep := relabel.ProcessBuilder(lb, cfg.RelabelConfigs...)
		if !keep {
			droppedAlertManagers = append(droppedAlertManagers, alertmanagerLabels{preRelabel})
			continue
		}

		addr := lb.Get(model.AddressLabel)
		if err := config.CheckTargetAddress(model.LabelValue(addr)); err != nil {
			return nil, nil, err
		}

		res = append(res, alertmanagerLabels{lb.Labels()})
	}
	return res, droppedAlertManagers, nil
}

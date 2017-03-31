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
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/relabel"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	alertPushEndpoint = "/api/v1/alerts"
	contentTypeJSON   = "application/json"
)

// String constants for instrumentation.
const (
	namespace         = "prometheus"
	subsystem         = "notifications"
	alertmanagerLabel = "alertmanager"
)

// Notifier is responsible for dispatching alert notifications to an
// alert manager service.
type Notifier struct {
	queue model.Alerts
	opts  *Options

	metrics *alertMetrics

	more   chan struct{}
	mtx    sync.RWMutex
	ctx    context.Context
	cancel func()

	alertmanagers   []*alertmanagerSet
	cancelDiscovery func()
}

// Options are the configurable parameters of a Handler.
type Options struct {
	QueueCapacity  int
	ExternalLabels model.LabelSet
	RelabelConfigs []*config.RelabelConfig
	// Used for sending HTTP requests to the Alertmanager.
	Do func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)

	Registerer prometheus.Registerer
}

type alertMetrics struct {
	latency       *prometheus.SummaryVec
	errors        *prometheus.CounterVec
	sent          *prometheus.CounterVec
	dropped       prometheus.Counter
	queueLength   prometheus.GaugeFunc
	queueCapacity prometheus.Gauge
}

func newAlertMetrics(r prometheus.Registerer, queueCap int, queueLen func() float64) *alertMetrics {
	m := &alertMetrics{
		latency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "latency_seconds",
			Help:      "Latency quantiles for sending alert notifications (not including dropped notifications).",
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
			Help:      "Total number of alerts successfully sent.",
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
		)
	}

	return m
}

// New constructs a new Notifier.
func New(o *Options) *Notifier {
	ctx, cancel := context.WithCancel(context.Background())

	if o.Do == nil {
		o.Do = ctxhttp.Do
	}

	n := &Notifier{
		queue:  make(model.Alerts, 0, o.QueueCapacity),
		ctx:    ctx,
		cancel: cancel,
		more:   make(chan struct{}, 1),
		opts:   o,
	}

	queueLenFunc := func() float64 { return float64(n.queueLen()) }
	n.metrics = newAlertMetrics(o.Registerer, o.QueueCapacity, queueLenFunc)
	return n
}

// ApplyConfig updates the status state as the new config requires.
func (n *Notifier) ApplyConfig(conf *config.Config) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	n.opts.ExternalLabels = conf.GlobalConfig.ExternalLabels
	n.opts.RelabelConfigs = conf.AlertingConfig.AlertRelabelConfigs

	amSets := []*alertmanagerSet{}
	ctx, cancel := context.WithCancel(n.ctx)

	for _, cfg := range conf.AlertingConfig.AlertmanagerConfigs {
		ams, err := newAlertmanagerSet(cfg)
		if err != nil {
			return err
		}

		ams.metrics = n.metrics

		amSets = append(amSets, ams)
	}

	// After all sets were created successfully, start them and cancel the
	// old ones.
	for _, ams := range amSets {
		go ams.ts.Run(ctx)
		ams.ts.UpdateProviders(discovery.ProvidersFromConfig(ams.cfg.ServiceDiscoveryConfig))
	}
	if n.cancelDiscovery != nil {
		n.cancelDiscovery()
	}

	n.cancelDiscovery = cancel
	n.alertmanagers = amSets

	return nil
}

const maxBatchSize = 64

func (n *Notifier) queueLen() int {
	n.mtx.RLock()
	defer n.mtx.RUnlock()

	return len(n.queue)
}

func (n *Notifier) nextBatch() []*model.Alert {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	var alerts model.Alerts

	if len(n.queue) > maxBatchSize {
		alerts = append(make(model.Alerts, 0, maxBatchSize), n.queue[:maxBatchSize]...)
		n.queue = n.queue[maxBatchSize:]
	} else {
		alerts = append(make(model.Alerts, 0, len(n.queue)), n.queue...)
		n.queue = n.queue[:0]
	}

	return alerts
}

// Run dispatches notifications continuously.
func (n *Notifier) Run() {
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-n.more:
		}
		alerts := n.nextBatch()

		if !n.sendAll(alerts...) {
			n.metrics.dropped.Add(float64(len(alerts)))
		}
		// If the queue still has items left, kick off the next iteration.
		if n.queueLen() > 0 {
			n.setMore()
		}
	}
}

// Send queues the given notification requests for processing.
// Panics if called on a handler that is not running.
func (n *Notifier) Send(alerts ...*model.Alert) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	// Attach external labels before relabelling and sending.
	for _, a := range alerts {
		for ln, lv := range n.opts.ExternalLabels {
			if _, ok := a.Labels[ln]; !ok {
				a.Labels[ln] = lv
			}
		}
	}

	alerts = n.relabelAlerts(alerts)

	// Queue capacity should be significantly larger than a single alert
	// batch could be.
	if d := len(alerts) - n.opts.QueueCapacity; d > 0 {
		alerts = alerts[d:]

		log.Warnf("Alert batch larger than queue capacity, dropping %d alerts", d)
		n.metrics.dropped.Add(float64(d))
	}

	// If the queue is full, remove the oldest alerts in favor
	// of newer ones.
	if d := (len(n.queue) + len(alerts)) - n.opts.QueueCapacity; d > 0 {
		n.queue = n.queue[d:]

		log.Warnf("Alert notification queue full, dropping %d alerts", d)
		n.metrics.dropped.Add(float64(d))
	}
	n.queue = append(n.queue, alerts...)

	// Notify sending goroutine that there are alerts to be processed.
	n.setMore()
}

func (n *Notifier) relabelAlerts(alerts []*model.Alert) []*model.Alert {
	var relabeledAlerts []*model.Alert
	for _, alert := range alerts {
		labels := relabel.Process(alert.Labels, n.opts.RelabelConfigs...)
		if labels != nil {
			alert.Labels = labels
			relabeledAlerts = append(relabeledAlerts, alert)
		}
	}
	return relabeledAlerts
}

// setMore signals that the alert queue has items.
func (n *Notifier) setMore() {
	// If we cannot send on the channel, it means the signal already exists
	// and has not been consumed yet.
	select {
	case n.more <- struct{}{}:
	default:
	}
}

// Alertmanagers returns a list Alertmanager URLs.
func (n *Notifier) Alertmanagers() []string {
	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var res []string

	for _, ams := range amSets {
		ams.mtx.RLock()
		for _, am := range ams.ams {
			res = append(res, am.url())
		}
		ams.mtx.RUnlock()
	}

	return res
}

// sendAll sends the alerts to all configured Alertmanagers concurrently.
// It returns true if the alerts could be sent successfully to at least one Alertmanager.
func (n *Notifier) sendAll(alerts ...*model.Alert) bool {
	begin := time.Now()

	b, err := json.Marshal(alerts)
	if err != nil {
		log.Errorf("Encoding alerts failed: %s", err)
		return false
	}

	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var (
		wg         sync.WaitGroup
		numSuccess uint64
	)
	for _, ams := range amSets {
		ams.mtx.RLock()

		for _, am := range ams.ams {
			wg.Add(1)

			ctx, cancel := context.WithTimeout(n.ctx, ams.cfg.Timeout)
			defer cancel()

			go func(am alertmanager) {
				u := am.url()

				if err := n.sendOne(ctx, ams.client, u, b); err != nil {
					log.With("alertmanager", u).With("count", len(alerts)).Errorf("Error sending alerts: %s", err)
					n.metrics.errors.WithLabelValues(u).Inc()
				} else {
					atomic.AddUint64(&numSuccess, 1)
				}
				n.metrics.latency.WithLabelValues(u).Observe(time.Since(begin).Seconds())
				n.metrics.sent.WithLabelValues(u).Add(float64(len(alerts)))

				wg.Done()
			}(am)
		}
		ams.mtx.RUnlock()
	}
	wg.Wait()

	return numSuccess > 0
}

func (n *Notifier) sendOne(ctx context.Context, c *http.Client, url string, b []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := n.opts.Do(ctx, c, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Any HTTP status 2xx is OK.
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bad response status %v", resp.Status)
	}
	return err
}

// Stop shuts down the notification handler.
func (n *Notifier) Stop() {
	log.Info("Stopping notification handler...")
	n.cancel()
}

// alertmanager holds Alertmanager endpoint information.
type alertmanager interface {
	url() string
}

type alertmanagerLabels model.LabelSet

const pathLabel = "__alerts_path__"

func (a alertmanagerLabels) url() string {
	u := &url.URL{
		Scheme: string(a[model.SchemeLabel]),
		Host:   string(a[model.AddressLabel]),
		Path:   string(a[pathLabel]),
	}
	return u.String()
}

// alertmanagerSet contains a set of Alertmanagers discovered via a group of service
// discovery definitions that have a common configuration on how alerts should be sent.
type alertmanagerSet struct {
	ts     *discovery.TargetSet
	cfg    *config.AlertmanagerConfig
	client *http.Client

	metrics *alertMetrics

	mtx sync.RWMutex
	ams []alertmanager
}

func newAlertmanagerSet(cfg *config.AlertmanagerConfig) (*alertmanagerSet, error) {
	client, err := httputil.NewClientFromConfig(cfg.HTTPClientConfig)
	if err != nil {
		return nil, err
	}
	s := &alertmanagerSet{
		client: client,
		cfg:    cfg,
	}
	s.ts = discovery.NewTargetSet(s)

	return s, nil
}

// Sync extracts a deduplicated set of Alertmanager endpoints from a list
// of target groups definitions.
func (s *alertmanagerSet) Sync(tgs []*config.TargetGroup) {
	all := []alertmanager{}

	for _, tg := range tgs {
		ams, err := alertmanagerFromGroup(tg, s.cfg)
		if err != nil {
			log.With("err", err).Error("generating discovered Alertmanagers failed")
			continue
		}
		all = append(all, ams...)
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	// Set new Alertmanagers and deduplicate them along their unique URL.
	s.ams = []alertmanager{}
	seen := map[string]struct{}{}

	for _, am := range all {
		us := am.url()
		if _, ok := seen[us]; ok {
			continue
		}

		// This will initialise the Counters for the AM to 0.
		s.metrics.sent.WithLabelValues(us)
		s.metrics.errors.WithLabelValues(us)

		seen[us] = struct{}{}
		s.ams = append(s.ams, am)
	}
}

func postPath(pre string) string {
	return path.Join("/", pre, alertPushEndpoint)
}

// alertmanagersFromGroup extracts a list of alertmanagers from a target group and an associcated
// AlertmanagerConfig.
func alertmanagerFromGroup(tg *config.TargetGroup, cfg *config.AlertmanagerConfig) ([]alertmanager, error) {
	var res []alertmanager

	for _, lset := range tg.Targets {
		// Set configured scheme as the initial scheme label for overwrite.
		lset[model.SchemeLabel] = model.LabelValue(cfg.Scheme)
		lset[pathLabel] = model.LabelValue(postPath(cfg.PathPrefix))

		// Combine target labels with target group labels.
		for ln, lv := range tg.Labels {
			if _, ok := lset[ln]; !ok {
				lset[ln] = lv
			}
		}
		lset := relabel.Process(lset, cfg.RelabelConfigs...)
		if lset == nil {
			continue
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
				return nil, fmt.Errorf("invalid scheme: %q", cfg.Scheme)
			}
			lset[model.AddressLabel] = model.LabelValue(addr)
		}
		if err := config.CheckTargetAddress(lset[model.AddressLabel]); err != nil {
			return nil, err
		}

		// Meta labels are deleted after relabelling. Other internal labels propagate to
		// the target which decides whether they will be part of their label set.
		for ln := range lset {
			if strings.HasPrefix(string(ln), model.MetaLabelPrefix) {
				delete(lset, ln)
			}
		}

		res = append(res, alertmanagerLabels(lset))
	}
	return res, nil
}

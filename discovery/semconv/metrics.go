// Code generated from semantic convention specification. DO NOT EDIT.

// Package metrics provides Prometheus instrumentation types for metrics
// defined in this semantic convention registry.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Attribute is an interface for metric label attributes.
type Attribute interface {
	ID() string
	Value() string
}
type ConfigAttr string

func (a ConfigAttr) ID() string {
	return "config"
}

func (a ConfigAttr) Value() string {
	return string(a)
}

type EventAttr string

func (a EventAttr) ID() string {
	return "event"
}

func (a EventAttr) Value() string {
	return string(a)
}

type FilenameAttr string

func (a FilenameAttr) ID() string {
	return "filename"
}

func (a FilenameAttr) Value() string {
	return string(a)
}

type MechanismAttr string

func (a MechanismAttr) ID() string {
	return "mechanism"
}

func (a MechanismAttr) Value() string {
	return string(a)
}

type NameAttr string

func (a NameAttr) ID() string {
	return "name"
}

func (a NameAttr) Value() string {
	return string(a)
}

type RoleAttr string

func (a RoleAttr) ID() string {
	return "role"
}

func (a RoleAttr) Value() string {
	return string(a)
}

// PrometheusSDAzureCacheHitTotal records the number of cache hits during Azure SD.
type PrometheusSDAzureCacheHitTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDAzureCacheHitTotal returns a new PrometheusSDAzureCacheHitTotal instrument.
func NewPrometheusSDAzureCacheHitTotal() PrometheusSDAzureCacheHitTotal {
	labels := []string{}
	return PrometheusSDAzureCacheHitTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_azure_cache_hit_total",
			Help: "Number of cache hits during Azure SD.",
		}, labels),
	}
}

type PrometheusSDAzureCacheHitTotalAttr interface {
	Attribute
	implPrometheusSDAzureCacheHitTotal()
}

func (m PrometheusSDAzureCacheHitTotal) With(
	extra ...PrometheusSDAzureCacheHitTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDAzureFailuresTotal records the number of Azure SD failures.
type PrometheusSDAzureFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDAzureFailuresTotal returns a new PrometheusSDAzureFailuresTotal instrument.
func NewPrometheusSDAzureFailuresTotal() PrometheusSDAzureFailuresTotal {
	labels := []string{}
	return PrometheusSDAzureFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_azure_failures_total",
			Help: "Number of Azure SD failures.",
		}, labels),
	}
}

type PrometheusSDAzureFailuresTotalAttr interface {
	Attribute
	implPrometheusSDAzureFailuresTotal()
}

func (m PrometheusSDAzureFailuresTotal) With(
	extra ...PrometheusSDAzureFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDConsulRpcDurationSeconds records the duration of a Consul RPC call.
type PrometheusSDConsulRpcDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusSDConsulRpcDurationSeconds returns a new PrometheusSDConsulRpcDurationSeconds instrument.
func NewPrometheusSDConsulRpcDurationSeconds() PrometheusSDConsulRpcDurationSeconds {
	labels := []string{}
	return PrometheusSDConsulRpcDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_sd_consul_rpc_duration_seconds",
			Help: "The duration of a Consul RPC call.",
		}, labels),
	}
}

type PrometheusSDConsulRpcDurationSecondsAttr interface {
	Attribute
	implPrometheusSDConsulRpcDurationSeconds()
}

func (m PrometheusSDConsulRpcDurationSeconds) With(
	extra ...PrometheusSDConsulRpcDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusSDConsulRpcFailuresTotal records the number of Consul RPC call failures.
type PrometheusSDConsulRpcFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDConsulRpcFailuresTotal returns a new PrometheusSDConsulRpcFailuresTotal instrument.
func NewPrometheusSDConsulRpcFailuresTotal() PrometheusSDConsulRpcFailuresTotal {
	labels := []string{}
	return PrometheusSDConsulRpcFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_consul_rpc_failures_total",
			Help: "Number of Consul RPC call failures.",
		}, labels),
	}
}

type PrometheusSDConsulRpcFailuresTotalAttr interface {
	Attribute
	implPrometheusSDConsulRpcFailuresTotal()
}

func (m PrometheusSDConsulRpcFailuresTotal) With(
	extra ...PrometheusSDConsulRpcFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDDiscoveredTargets records the current number of discovered targets.
type PrometheusSDDiscoveredTargets struct {
	*prometheus.GaugeVec
}

// NewPrometheusSDDiscoveredTargets returns a new PrometheusSDDiscoveredTargets instrument.
func NewPrometheusSDDiscoveredTargets() PrometheusSDDiscoveredTargets {
	labels := []string{
		"name",
		"config",
	}
	return PrometheusSDDiscoveredTargets{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_sd_discovered_targets",
			Help: "Current number of discovered targets.",
		}, labels),
	}
}

type PrometheusSDDiscoveredTargetsAttr interface {
	Attribute
	implPrometheusSDDiscoveredTargets()
}

func (a NameAttr) implPrometheusSDDiscoveredTargets()   {}
func (a ConfigAttr) implPrometheusSDDiscoveredTargets() {}

func (m PrometheusSDDiscoveredTargets) With(
	extra ...PrometheusSDDiscoveredTargetsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"name":   "",
		"config": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusSDDNSLookupFailuresTotal records the number of DNS SD lookup failures.
type PrometheusSDDNSLookupFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDDNSLookupFailuresTotal returns a new PrometheusSDDNSLookupFailuresTotal instrument.
func NewPrometheusSDDNSLookupFailuresTotal() PrometheusSDDNSLookupFailuresTotal {
	labels := []string{}
	return PrometheusSDDNSLookupFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_dns_lookup_failures_total",
			Help: "Number of DNS SD lookup failures.",
		}, labels),
	}
}

type PrometheusSDDNSLookupFailuresTotalAttr interface {
	Attribute
	implPrometheusSDDNSLookupFailuresTotal()
}

func (m PrometheusSDDNSLookupFailuresTotal) With(
	extra ...PrometheusSDDNSLookupFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDDNSLookupsTotal records the number of DNS SD lookups.
type PrometheusSDDNSLookupsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDDNSLookupsTotal returns a new PrometheusSDDNSLookupsTotal instrument.
func NewPrometheusSDDNSLookupsTotal() PrometheusSDDNSLookupsTotal {
	labels := []string{}
	return PrometheusSDDNSLookupsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_dns_lookups_total",
			Help: "Number of DNS SD lookups.",
		}, labels),
	}
}

type PrometheusSDDNSLookupsTotalAttr interface {
	Attribute
	implPrometheusSDDNSLookupsTotal()
}

func (m PrometheusSDDNSLookupsTotal) With(
	extra ...PrometheusSDDNSLookupsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDFailedConfigs records the current number of service discovery configurations that failed to load.
type PrometheusSDFailedConfigs struct {
	*prometheus.GaugeVec
}

// NewPrometheusSDFailedConfigs returns a new PrometheusSDFailedConfigs instrument.
func NewPrometheusSDFailedConfigs() PrometheusSDFailedConfigs {
	labels := []string{
		"name",
	}
	return PrometheusSDFailedConfigs{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_sd_failed_configs",
			Help: "Current number of service discovery configurations that failed to load.",
		}, labels),
	}
}

type PrometheusSDFailedConfigsAttr interface {
	Attribute
	implPrometheusSDFailedConfigs()
}

func (a NameAttr) implPrometheusSDFailedConfigs() {}

func (m PrometheusSDFailedConfigs) With(
	extra ...PrometheusSDFailedConfigsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"name": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusSDFileMtimeSeconds records the modification time of the SD file.
type PrometheusSDFileMtimeSeconds struct {
	*prometheus.GaugeVec
}

// NewPrometheusSDFileMtimeSeconds returns a new PrometheusSDFileMtimeSeconds instrument.
func NewPrometheusSDFileMtimeSeconds() PrometheusSDFileMtimeSeconds {
	labels := []string{
		"filename",
	}
	return PrometheusSDFileMtimeSeconds{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_sd_file_mtime_seconds",
			Help: "The modification time of the SD file.",
		}, labels),
	}
}

type PrometheusSDFileMtimeSecondsAttr interface {
	Attribute
	implPrometheusSDFileMtimeSeconds()
}

func (a FilenameAttr) implPrometheusSDFileMtimeSeconds() {}

func (m PrometheusSDFileMtimeSeconds) With(
	extra ...PrometheusSDFileMtimeSecondsAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{
		"filename": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusSDFileReadErrorsTotal records the number of file SD read errors.
type PrometheusSDFileReadErrorsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDFileReadErrorsTotal returns a new PrometheusSDFileReadErrorsTotal instrument.
func NewPrometheusSDFileReadErrorsTotal() PrometheusSDFileReadErrorsTotal {
	labels := []string{}
	return PrometheusSDFileReadErrorsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_file_read_errors_total",
			Help: "Number of file SD read errors.",
		}, labels),
	}
}

type PrometheusSDFileReadErrorsTotalAttr interface {
	Attribute
	implPrometheusSDFileReadErrorsTotal()
}

func (m PrometheusSDFileReadErrorsTotal) With(
	extra ...PrometheusSDFileReadErrorsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDFileScanDurationSeconds records the duration of the file SD scan.
type PrometheusSDFileScanDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusSDFileScanDurationSeconds returns a new PrometheusSDFileScanDurationSeconds instrument.
func NewPrometheusSDFileScanDurationSeconds() PrometheusSDFileScanDurationSeconds {
	labels := []string{}
	return PrometheusSDFileScanDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_sd_file_scan_duration_seconds",
			Help: "The duration of the file SD scan.",
		}, labels),
	}
}

type PrometheusSDFileScanDurationSecondsAttr interface {
	Attribute
	implPrometheusSDFileScanDurationSeconds()
}

func (m PrometheusSDFileScanDurationSeconds) With(
	extra ...PrometheusSDFileScanDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusSDFileWatcherErrorsTotal records the number of file SD watcher errors.
type PrometheusSDFileWatcherErrorsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDFileWatcherErrorsTotal returns a new PrometheusSDFileWatcherErrorsTotal instrument.
func NewPrometheusSDFileWatcherErrorsTotal() PrometheusSDFileWatcherErrorsTotal {
	labels := []string{}
	return PrometheusSDFileWatcherErrorsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_file_watcher_errors_total",
			Help: "Number of file SD watcher errors.",
		}, labels),
	}
}

type PrometheusSDFileWatcherErrorsTotalAttr interface {
	Attribute
	implPrometheusSDFileWatcherErrorsTotal()
}

func (m PrometheusSDFileWatcherErrorsTotal) With(
	extra ...PrometheusSDFileWatcherErrorsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDHTTPFailuresTotal records the number of HTTP SD failures.
type PrometheusSDHTTPFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDHTTPFailuresTotal returns a new PrometheusSDHTTPFailuresTotal instrument.
func NewPrometheusSDHTTPFailuresTotal() PrometheusSDHTTPFailuresTotal {
	labels := []string{}
	return PrometheusSDHTTPFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_http_failures_total",
			Help: "Number of HTTP SD failures.",
		}, labels),
	}
}

type PrometheusSDHTTPFailuresTotalAttr interface {
	Attribute
	implPrometheusSDHTTPFailuresTotal()
}

func (m PrometheusSDHTTPFailuresTotal) With(
	extra ...PrometheusSDHTTPFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDKubernetesEventsTotal records the number of Kubernetes events processed.
type PrometheusSDKubernetesEventsTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDKubernetesEventsTotal returns a new PrometheusSDKubernetesEventsTotal instrument.
func NewPrometheusSDKubernetesEventsTotal() PrometheusSDKubernetesEventsTotal {
	labels := []string{
		"role",
		"event",
	}
	return PrometheusSDKubernetesEventsTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_kubernetes_events_total",
			Help: "Number of Kubernetes events processed.",
		}, labels),
	}
}

type PrometheusSDKubernetesEventsTotalAttr interface {
	Attribute
	implPrometheusSDKubernetesEventsTotal()
}

func (a RoleAttr) implPrometheusSDKubernetesEventsTotal()  {}
func (a EventAttr) implPrometheusSDKubernetesEventsTotal() {}

func (m PrometheusSDKubernetesEventsTotal) With(
	extra ...PrometheusSDKubernetesEventsTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"role":  "",
		"event": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDKubernetesFailuresTotal records the number of Kubernetes SD failures.
type PrometheusSDKubernetesFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDKubernetesFailuresTotal returns a new PrometheusSDKubernetesFailuresTotal instrument.
func NewPrometheusSDKubernetesFailuresTotal() PrometheusSDKubernetesFailuresTotal {
	labels := []string{}
	return PrometheusSDKubernetesFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_kubernetes_failures_total",
			Help: "Number of Kubernetes SD failures.",
		}, labels),
	}
}

type PrometheusSDKubernetesFailuresTotalAttr interface {
	Attribute
	implPrometheusSDKubernetesFailuresTotal()
}

func (m PrometheusSDKubernetesFailuresTotal) With(
	extra ...PrometheusSDKubernetesFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDKumaFetchDurationSeconds records the duration of a Kuma MADS fetch call.
type PrometheusSDKumaFetchDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusSDKumaFetchDurationSeconds returns a new PrometheusSDKumaFetchDurationSeconds instrument.
func NewPrometheusSDKumaFetchDurationSeconds() PrometheusSDKumaFetchDurationSeconds {
	labels := []string{}
	return PrometheusSDKumaFetchDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_sd_kuma_fetch_duration_seconds",
			Help: "The duration of a Kuma MADS fetch call.",
		}, labels),
	}
}

type PrometheusSDKumaFetchDurationSecondsAttr interface {
	Attribute
	implPrometheusSDKumaFetchDurationSeconds()
}

func (m PrometheusSDKumaFetchDurationSeconds) With(
	extra ...PrometheusSDKumaFetchDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusSDKumaFetchFailuresTotal records the number of Kuma SD fetch failures.
type PrometheusSDKumaFetchFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDKumaFetchFailuresTotal returns a new PrometheusSDKumaFetchFailuresTotal instrument.
func NewPrometheusSDKumaFetchFailuresTotal() PrometheusSDKumaFetchFailuresTotal {
	labels := []string{}
	return PrometheusSDKumaFetchFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_kuma_fetch_failures_total",
			Help: "Number of Kuma SD fetch failures.",
		}, labels),
	}
}

type PrometheusSDKumaFetchFailuresTotalAttr interface {
	Attribute
	implPrometheusSDKumaFetchFailuresTotal()
}

func (m PrometheusSDKumaFetchFailuresTotal) With(
	extra ...PrometheusSDKumaFetchFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDKumaFetchSkippedUpdatesTotal records the number of Kuma SD updates skipped due to no changes.
type PrometheusSDKumaFetchSkippedUpdatesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDKumaFetchSkippedUpdatesTotal returns a new PrometheusSDKumaFetchSkippedUpdatesTotal instrument.
func NewPrometheusSDKumaFetchSkippedUpdatesTotal() PrometheusSDKumaFetchSkippedUpdatesTotal {
	labels := []string{}
	return PrometheusSDKumaFetchSkippedUpdatesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_kuma_fetch_skipped_updates_total",
			Help: "Number of Kuma SD updates skipped due to no changes.",
		}, labels),
	}
}

type PrometheusSDKumaFetchSkippedUpdatesTotalAttr interface {
	Attribute
	implPrometheusSDKumaFetchSkippedUpdatesTotal()
}

func (m PrometheusSDKumaFetchSkippedUpdatesTotal) With(
	extra ...PrometheusSDKumaFetchSkippedUpdatesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDLinodeFailuresTotal records the number of Linode SD failures.
type PrometheusSDLinodeFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDLinodeFailuresTotal returns a new PrometheusSDLinodeFailuresTotal instrument.
func NewPrometheusSDLinodeFailuresTotal() PrometheusSDLinodeFailuresTotal {
	labels := []string{}
	return PrometheusSDLinodeFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_linode_failures_total",
			Help: "Number of Linode SD failures.",
		}, labels),
	}
}

type PrometheusSDLinodeFailuresTotalAttr interface {
	Attribute
	implPrometheusSDLinodeFailuresTotal()
}

func (m PrometheusSDLinodeFailuresTotal) With(
	extra ...PrometheusSDLinodeFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDNomadFailuresTotal records the number of Nomad SD failures.
type PrometheusSDNomadFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDNomadFailuresTotal returns a new PrometheusSDNomadFailuresTotal instrument.
func NewPrometheusSDNomadFailuresTotal() PrometheusSDNomadFailuresTotal {
	labels := []string{}
	return PrometheusSDNomadFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_nomad_failures_total",
			Help: "Number of Nomad SD failures.",
		}, labels),
	}
}

type PrometheusSDNomadFailuresTotalAttr interface {
	Attribute
	implPrometheusSDNomadFailuresTotal()
}

func (m PrometheusSDNomadFailuresTotal) With(
	extra ...PrometheusSDNomadFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDReceivedUpdatesTotal records the total number of update events received from the SD providers.
type PrometheusSDReceivedUpdatesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDReceivedUpdatesTotal returns a new PrometheusSDReceivedUpdatesTotal instrument.
func NewPrometheusSDReceivedUpdatesTotal() PrometheusSDReceivedUpdatesTotal {
	labels := []string{
		"name",
	}
	return PrometheusSDReceivedUpdatesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_received_updates_total",
			Help: "Total number of update events received from the SD providers.",
		}, labels),
	}
}

type PrometheusSDReceivedUpdatesTotalAttr interface {
	Attribute
	implPrometheusSDReceivedUpdatesTotal()
}

func (a NameAttr) implPrometheusSDReceivedUpdatesTotal() {}

func (m PrometheusSDReceivedUpdatesTotal) With(
	extra ...PrometheusSDReceivedUpdatesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"name": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDRefreshDurationHistogramSeconds records the duration of a SD refresh cycle as a histogram.
type PrometheusSDRefreshDurationHistogramSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusSDRefreshDurationHistogramSeconds returns a new PrometheusSDRefreshDurationHistogramSeconds instrument.
func NewPrometheusSDRefreshDurationHistogramSeconds() PrometheusSDRefreshDurationHistogramSeconds {
	labels := []string{
		"mechanism",
	}
	return PrometheusSDRefreshDurationHistogramSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_sd_refresh_duration_histogram_seconds",
			Help: "The duration of a SD refresh cycle as a histogram.",
		}, labels),
	}
}

type PrometheusSDRefreshDurationHistogramSecondsAttr interface {
	Attribute
	implPrometheusSDRefreshDurationHistogramSeconds()
}

func (a MechanismAttr) implPrometheusSDRefreshDurationHistogramSeconds() {}

func (m PrometheusSDRefreshDurationHistogramSeconds) With(
	extra ...PrometheusSDRefreshDurationHistogramSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{
		"mechanism": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusSDRefreshDurationSeconds records the duration of a SD refresh cycle.
type PrometheusSDRefreshDurationSeconds struct {
	*prometheus.HistogramVec
}

// NewPrometheusSDRefreshDurationSeconds returns a new PrometheusSDRefreshDurationSeconds instrument.
func NewPrometheusSDRefreshDurationSeconds() PrometheusSDRefreshDurationSeconds {
	labels := []string{}
	return PrometheusSDRefreshDurationSeconds{
		HistogramVec: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "prometheus_sd_refresh_duration_seconds",
			Help: "The duration of a SD refresh cycle.",
		}, labels),
	}
}

type PrometheusSDRefreshDurationSecondsAttr interface {
	Attribute
	implPrometheusSDRefreshDurationSeconds()
}

func (m PrometheusSDRefreshDurationSeconds) With(
	extra ...PrometheusSDRefreshDurationSecondsAttr,
) prometheus.Observer {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.HistogramVec.With(labels)
}

// PrometheusSDRefreshFailuresTotal records the number of SD refresh failures.
type PrometheusSDRefreshFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDRefreshFailuresTotal returns a new PrometheusSDRefreshFailuresTotal instrument.
func NewPrometheusSDRefreshFailuresTotal() PrometheusSDRefreshFailuresTotal {
	labels := []string{
		"config",
		"mechanism",
	}
	return PrometheusSDRefreshFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_refresh_failures_total",
			Help: "Number of SD refresh failures.",
		}, labels),
	}
}

type PrometheusSDRefreshFailuresTotalAttr interface {
	Attribute
	implPrometheusSDRefreshFailuresTotal()
}

func (a ConfigAttr) implPrometheusSDRefreshFailuresTotal()    {}
func (a MechanismAttr) implPrometheusSDRefreshFailuresTotal() {}

func (m PrometheusSDRefreshFailuresTotal) With(
	extra ...PrometheusSDRefreshFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"config":    "",
		"mechanism": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDUpdatesDelayedTotal records the total number of update events that couldn't be sent immediately.
type PrometheusSDUpdatesDelayedTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDUpdatesDelayedTotal returns a new PrometheusSDUpdatesDelayedTotal instrument.
func NewPrometheusSDUpdatesDelayedTotal() PrometheusSDUpdatesDelayedTotal {
	labels := []string{
		"name",
	}
	return PrometheusSDUpdatesDelayedTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_updates_delayed_total",
			Help: "Total number of update events that couldn't be sent immediately.",
		}, labels),
	}
}

type PrometheusSDUpdatesDelayedTotalAttr interface {
	Attribute
	implPrometheusSDUpdatesDelayedTotal()
}

func (a NameAttr) implPrometheusSDUpdatesDelayedTotal() {}

func (m PrometheusSDUpdatesDelayedTotal) With(
	extra ...PrometheusSDUpdatesDelayedTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"name": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusSDUpdatesTotal records the total number of update events sent to the SD consumers.
type PrometheusSDUpdatesTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusSDUpdatesTotal returns a new PrometheusSDUpdatesTotal instrument.
func NewPrometheusSDUpdatesTotal() PrometheusSDUpdatesTotal {
	labels := []string{
		"name",
	}
	return PrometheusSDUpdatesTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_sd_updates_total",
			Help: "Total number of update events sent to the SD consumers.",
		}, labels),
	}
}

type PrometheusSDUpdatesTotalAttr interface {
	Attribute
	implPrometheusSDUpdatesTotal()
}

func (a NameAttr) implPrometheusSDUpdatesTotal() {}

func (m PrometheusSDUpdatesTotal) With(
	extra ...PrometheusSDUpdatesTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{
		"name": "",
	}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

// PrometheusTreecacheWatcherGoroutines records the current number of treecache watcher goroutines.
type PrometheusTreecacheWatcherGoroutines struct {
	*prometheus.GaugeVec
}

// NewPrometheusTreecacheWatcherGoroutines returns a new PrometheusTreecacheWatcherGoroutines instrument.
func NewPrometheusTreecacheWatcherGoroutines() PrometheusTreecacheWatcherGoroutines {
	labels := []string{}
	return PrometheusTreecacheWatcherGoroutines{
		GaugeVec: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "prometheus_treecache_watcher_goroutines",
			Help: "The current number of treecache watcher goroutines.",
		}, labels),
	}
}

type PrometheusTreecacheWatcherGoroutinesAttr interface {
	Attribute
	implPrometheusTreecacheWatcherGoroutines()
}

func (m PrometheusTreecacheWatcherGoroutines) With(
	extra ...PrometheusTreecacheWatcherGoroutinesAttr,
) prometheus.Gauge {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.GaugeVec.With(labels)
}

// PrometheusTreecacheZookeeperFailuresTotal records the total number of ZooKeeper failures.
type PrometheusTreecacheZookeeperFailuresTotal struct {
	*prometheus.CounterVec
}

// NewPrometheusTreecacheZookeeperFailuresTotal returns a new PrometheusTreecacheZookeeperFailuresTotal instrument.
func NewPrometheusTreecacheZookeeperFailuresTotal() PrometheusTreecacheZookeeperFailuresTotal {
	labels := []string{}
	return PrometheusTreecacheZookeeperFailuresTotal{
		CounterVec: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "prometheus_treecache_zookeeper_failures_total",
			Help: "Total number of ZooKeeper failures.",
		}, labels),
	}
}

type PrometheusTreecacheZookeeperFailuresTotalAttr interface {
	Attribute
	implPrometheusTreecacheZookeeperFailuresTotal()
}

func (m PrometheusTreecacheZookeeperFailuresTotal) With(
	extra ...PrometheusTreecacheZookeeperFailuresTotalAttr,
) prometheus.Counter {
	labels := prometheus.Labels{}
	for _, v := range extra {
		labels[v.ID()] = v.Value()
	}
	return m.CounterVec.With(labels)
}

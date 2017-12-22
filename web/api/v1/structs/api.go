package structs

import (
	"fmt"
	"time"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
)

type Status string

const (
	StatusSuccess Status = "success"
	StatusError          = "error"
)

type ErrorType string

const (
	ErrorNone     ErrorType = ""
	ErrorTimeout            = "timeout"
	ErrorCanceled           = "canceled"
	ErrorExec               = "execution"
	ErrorBadData            = "bad_data"
	ErrorInternal           = "internal"
)

type ApiError struct {
	Typ ErrorType
	Err error
}

func (e *ApiError) Error() string {
	return fmt.Sprintf("%s: %s", e.Typ, e.Err)
}

type Response struct {
	Status    Status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType ErrorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
}

type QueryData struct {
	ResultType promql.ValueType `json:"resultType"`
	Result     promql.Value     `json:"result"`
}

// Target has the information for one target.
type Target struct {
	// Labels before any processing.
	DiscoveredLabels map[string]string `json:"discoveredLabels"`
	// Any labels that are added to this target and its metrics.
	Labels map[string]string `json:"labels"`

	ScrapeURL string `json:"scrapeUrl"`

	LastError  string                 `json:"lastError"`
	LastScrape time.Time              `json:"lastScrape"`
	Health     retrieval.TargetHealth `json:"health"`
}

// TargetDiscovery has all the active targets.
type TargetDiscovery struct {
	ActiveTargets []*Target `json:"activeTargets"`
}

// AlertmanagerDiscovery has all the active Alertmanagers.
type AlertmanagerDiscovery struct {
	ActiveAlertmanagers []*AlertmanagerTarget `json:"activeAlertmanagers"`
}

// AlertmanagerTarget has info on one AM.
type AlertmanagerTarget struct {
	URL string `json:"url"`
}

type PrometheusConfig struct {
	YAML string `json:"yaml"`
}

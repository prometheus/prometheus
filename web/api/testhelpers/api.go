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

// Package testhelpers provides utilities for testing the Prometheus HTTP API.
// This file contains helper functions for creating test API instances and managing test lifecycles.
package testhelpers

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/notifications"
)

// RulesRetriever provides a list of active rules and alerts.
type RulesRetriever interface {
	RuleGroups() []*rules.Group
	AlertingRules() []*rules.AlertingRule
}

// TargetRetriever provides the list of active/dropped targets to scrape or not.
type TargetRetriever interface {
	TargetsActive() map[string][]*scrape.Target
	TargetsDropped() map[string][]*scrape.Target
	TargetsDroppedCounts() map[string]int
	ScrapePoolConfig(string) (*config.ScrapeConfig, error)
}

// ScrapePoolsRetriever provide the list of all scrape pools.
type ScrapePoolsRetriever interface {
	ScrapePools() []string
}

// AlertmanagerRetriever provides a list of all/dropped AlertManager URLs.
type AlertmanagerRetriever interface {
	Alertmanagers() []*url.URL
	DroppedAlertmanagers() []*url.URL
}

// TSDBAdminStats provides TSDB admin statistics.
type TSDBAdminStats interface {
	CleanTombstones() error
	Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error
	Snapshot(dir string, withHead bool) error
	Stats(statsByLabelName string, limit int) (*tsdb.Stats, error)
	WALReplayStatus() (tsdb.WALReplayStatus, error)
	BlockMetas() ([]tsdb.BlockMeta, error)
}

// APIConfig holds configuration for creating a test API instance.
type APIConfig struct {
	// Core dependencies.
	QueryEngine       *LazyLoader[promql.QueryEngine]
	Queryable         *LazyLoader[storage.SampleAndChunkQueryable]
	ExemplarQueryable *LazyLoader[storage.ExemplarQueryable]

	// Retrievers.
	RulesRetriever        *LazyLoader[RulesRetriever]
	TargetRetriever       *LazyLoader[TargetRetriever]
	ScrapePoolsRetriever  *LazyLoader[ScrapePoolsRetriever]
	AlertmanagerRetriever *LazyLoader[AlertmanagerRetriever]

	// Admin.
	TSDBAdmin *LazyLoader[TSDBAdminStats]
	DBDir     string

	// Optional overrides.
	Config   func() config.Config
	FlagsMap map[string]string
	Now      func() time.Time
}

// APIWrapper wraps the API and provides a handler for testing.
type APIWrapper struct {
	Handler http.Handler
}

// PrometheusVersion contains build information about Prometheus.
type PrometheusVersion struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildUser string `json:"buildUser"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

// RuntimeInfo contains runtime information about Prometheus.
type RuntimeInfo struct {
	StartTime           time.Time `json:"startTime"`
	CWD                 string    `json:"CWD"`
	Hostname            string    `json:"hostname"`
	ServerTime          time.Time `json:"serverTime"`
	ReloadConfigSuccess bool      `json:"reloadConfigSuccess"`
	LastConfigTime      time.Time `json:"lastConfigTime"`
	CorruptionCount     int64     `json:"corruptionCount"`
	GoroutineCount      int       `json:"goroutineCount"`
	GOMAXPROCS          int       `json:"GOMAXPROCS"`
	GOMEMLIMIT          int64     `json:"GOMEMLIMIT"`
	GOGC                string    `json:"GOGC"`
	GODEBUG             string    `json:"GODEBUG"`
	StorageRetention    string    `json:"storageRetention"`
}

// NewAPIParams holds all the parameters needed to create a v1.API instance.
type NewAPIParams struct {
	QueryEngine           promql.QueryEngine
	Queryable             storage.SampleAndChunkQueryable
	ExemplarQueryable     storage.ExemplarQueryable
	ScrapePoolsRetriever  func(context.Context) ScrapePoolsRetriever
	TargetRetriever       func(context.Context) TargetRetriever
	AlertmanagerRetriever func(context.Context) AlertmanagerRetriever
	ConfigFunc            func() config.Config
	FlagsMap              map[string]string
	ReadyFunc             func(http.HandlerFunc) http.HandlerFunc
	TSDBAdmin             TSDBAdminStats
	DBDir                 string
	Logger                *slog.Logger
	RulesRetriever        func(context.Context) RulesRetriever
	RuntimeInfoFunc       func() (RuntimeInfo, error)
	BuildInfo             *PrometheusVersion
	NotificationsGetter   func() []notifications.Notification
	NotificationsSub      func() (<-chan notifications.Notification, func(), bool)
	Gatherer              prometheus.Gatherer
	Registerer            prometheus.Registerer
}

// PrepareAPI creates a NewAPIParams with sensible defaults for testing.
func PrepareAPI(t *testing.T, cfg APIConfig) NewAPIParams {
	t.Helper()

	// Create defaults for unset lazy loaders.
	if cfg.QueryEngine == nil {
		cfg.QueryEngine = NewLazyLoader(func() promql.QueryEngine {
			return promqltest.NewTestEngineWithOpts(t, promql.EngineOpts{
				Logger:                   nil,
				Reg:                      nil,
				MaxSamples:               10000,
				Timeout:                  100 * time.Second,
				NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
				EnableAtModifier:         true,
				EnableNegativeOffset:     true,
				EnablePerStepStats:       true,
			})
		})
	}

	if cfg.Queryable == nil {
		cfg.Queryable = NewLazyLoader(NewEmptyQueryable)
	}

	if cfg.ExemplarQueryable == nil {
		cfg.ExemplarQueryable = NewLazyLoader(NewEmptyExemplarQueryable)
	}

	if cfg.RulesRetriever == nil {
		cfg.RulesRetriever = NewLazyLoader(func() RulesRetriever {
			return NewEmptyRulesRetriever()
		})
	}

	if cfg.TargetRetriever == nil {
		cfg.TargetRetriever = NewLazyLoader(func() TargetRetriever {
			return NewEmptyTargetRetriever()
		})
	}

	if cfg.ScrapePoolsRetriever == nil {
		cfg.ScrapePoolsRetriever = NewLazyLoader(func() ScrapePoolsRetriever {
			return NewEmptyScrapePoolsRetriever()
		})
	}

	if cfg.AlertmanagerRetriever == nil {
		cfg.AlertmanagerRetriever = NewLazyLoader(func() AlertmanagerRetriever {
			return NewEmptyAlertmanagerRetriever()
		})
	}

	if cfg.TSDBAdmin == nil {
		cfg.TSDBAdmin = NewLazyLoader(func() TSDBAdminStats {
			return NewEmptyTSDBAdminStats()
		})
	}

	if cfg.Config == nil {
		cfg.Config = func() config.Config { return config.Config{} }
	}

	if cfg.FlagsMap == nil {
		cfg.FlagsMap = map[string]string{}
	}

	if cfg.DBDir == "" {
		cfg.DBDir = t.TempDir()
	}

	return NewAPIParams{
		QueryEngine:           cfg.QueryEngine.Get(),
		Queryable:             cfg.Queryable.Get(),
		ExemplarQueryable:     cfg.ExemplarQueryable.Get(),
		ScrapePoolsRetriever:  func(context.Context) ScrapePoolsRetriever { return cfg.ScrapePoolsRetriever.Get() },
		TargetRetriever:       func(context.Context) TargetRetriever { return cfg.TargetRetriever.Get() },
		AlertmanagerRetriever: func(context.Context) AlertmanagerRetriever { return cfg.AlertmanagerRetriever.Get() },
		ConfigFunc:            cfg.Config,
		FlagsMap:              cfg.FlagsMap,
		ReadyFunc:             func(f http.HandlerFunc) http.HandlerFunc { return f },
		TSDBAdmin:             cfg.TSDBAdmin.Get(),
		DBDir:                 cfg.DBDir,
		Logger:                promslog.NewNopLogger(),
		RulesRetriever:        func(context.Context) RulesRetriever { return cfg.RulesRetriever.Get() },
		RuntimeInfoFunc:       func() (RuntimeInfo, error) { return RuntimeInfo{}, nil },
		BuildInfo:             &PrometheusVersion{},
		NotificationsGetter:   func() []notifications.Notification { return nil },
		NotificationsSub:      func() (<-chan notifications.Notification, func(), bool) { return nil, func() {}, false },
		Gatherer:              prometheus.NewRegistry(),
		Registerer:            prometheus.NewRegistry(),
	}
}

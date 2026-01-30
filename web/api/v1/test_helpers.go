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

package v1

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/web/api/testhelpers"
)

// newTestAPI creates a new API instance for testing using testhelpers.
func newTestAPI(t *testing.T, cfg testhelpers.APIConfig) *testhelpers.APIWrapper {
	t.Helper()

	params := testhelpers.PrepareAPI(t, cfg)

	// Adapt the testhelpers interfaces to v1 interfaces.
	api := NewAPI(
		params.QueryEngine,
		params.Queryable,
		nil, // appendable
		params.ExemplarQueryable,
		func(ctx context.Context) ScrapePoolsRetriever {
			return adaptScrapePoolsRetriever(params.ScrapePoolsRetriever(ctx))
		},
		func(ctx context.Context) TargetRetriever {
			return adaptTargetRetriever(params.TargetRetriever(ctx))
		},
		func(ctx context.Context) AlertmanagerRetriever {
			return adaptAlertmanagerRetriever(params.AlertmanagerRetriever(ctx))
		},
		params.ConfigFunc,
		params.FlagsMap,
		GlobalURLOptions{},
		params.ReadyFunc,
		adaptTSDBAdminStats(params.TSDBAdmin),
		params.DBDir,
		false, // enableAdmin
		params.Logger,
		func(ctx context.Context) RulesRetriever {
			return adaptRulesRetriever(params.RulesRetriever(ctx))
		},
		0,     // remoteReadSampleLimit
		0,     // remoteReadConcurrencyLimit
		0,     // remoteReadMaxBytesInFrame
		false, // isAgent
		nil,   // corsOrigin
		func() (RuntimeInfo, error) {
			info, err := params.RuntimeInfoFunc()
			return RuntimeInfo{
				StartTime:           info.StartTime,
				CWD:                 info.CWD,
				Hostname:            info.Hostname,
				ServerTime:          info.ServerTime,
				ReloadConfigSuccess: info.ReloadConfigSuccess,
				LastConfigTime:      info.LastConfigTime,
				CorruptionCount:     info.CorruptionCount,
				GoroutineCount:      info.GoroutineCount,
				GOMAXPROCS:          info.GOMAXPROCS,
				GOMEMLIMIT:          info.GOMEMLIMIT,
				GOGC:                info.GOGC,
				GODEBUG:             info.GODEBUG,
				StorageRetention:    info.StorageRetention,
			}, err
		},
		&PrometheusVersion{
			Version:   params.BuildInfo.Version,
			Revision:  params.BuildInfo.Revision,
			Branch:    params.BuildInfo.Branch,
			BuildUser: params.BuildInfo.BuildUser,
			BuildDate: params.BuildInfo.BuildDate,
			GoVersion: params.BuildInfo.GoVersion,
		},
		params.NotificationsGetter,
		params.NotificationsSub,
		params.Gatherer,
		params.Registerer,
		nil,              // statsRenderer
		false,            // rwEnabled
		nil,              // acceptRemoteWriteProtoMsgs
		false,            // otlpEnabled
		false,            // otlpDeltaToCumulative
		false,            // otlpNativeDeltaIngestion
		false,            // stZeroIngestionEnabled
		5*time.Minute,    // lookbackDelta
		false,            // enableTypeAndUnitLabels
		false,            // appendMetadata
		nil,              // overrideErrorCode
		nil,              // featureRegistry
		OpenAPIOptions{}, // openAPIOptions
	)

	// Register routes.
	router := route.New()
	api.Register(router.WithPrefix("/api/v1"))

	return &testhelpers.APIWrapper{
		Handler: router,
	}
}

// Adapter functions to convert testhelpers interfaces to v1 interfaces.

type rulesRetrieverAdapter struct {
	testhelpers.RulesRetriever
}

func adaptRulesRetriever(r testhelpers.RulesRetriever) RulesRetriever {
	return &rulesRetrieverAdapter{r}
}

type targetRetrieverAdapter struct {
	testhelpers.TargetRetriever
}

func adaptTargetRetriever(t testhelpers.TargetRetriever) TargetRetriever {
	return &targetRetrieverAdapter{t}
}

type scrapePoolsRetrieverAdapter struct {
	testhelpers.ScrapePoolsRetriever
}

func adaptScrapePoolsRetriever(s testhelpers.ScrapePoolsRetriever) ScrapePoolsRetriever {
	return &scrapePoolsRetrieverAdapter{s}
}

type alertmanagerRetrieverAdapter struct {
	testhelpers.AlertmanagerRetriever
}

func adaptAlertmanagerRetriever(a testhelpers.AlertmanagerRetriever) AlertmanagerRetriever {
	return &alertmanagerRetrieverAdapter{a}
}

type tsdbAdminStatsAdapter struct {
	testhelpers.TSDBAdminStats
}

func adaptTSDBAdminStats(t testhelpers.TSDBAdminStats) TSDBAdminStats {
	return &tsdbAdminStatsAdapter{t}
}

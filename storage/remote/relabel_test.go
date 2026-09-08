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

package remote

import (
	"context"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

var (
	relabelTestDropConfig = []*relabel.Config{{
		SourceLabels:         model.LabelNames{"__name__"},
		Regex:                relabel.MustNewRegexp("drop_me"),
		Action:               relabel.Drop,
		NameValidationScheme: model.UTF8Validation,
	}}
	relabelTestRewriteConfig = []*relabel.Config{{
		SourceLabels:         model.LabelNames{"env"},
		Regex:                relabel.MustNewRegexp("(.*)"),
		TargetLabel:          "environment",
		Replacement:          "$1",
		Action:               relabel.Replace,
		NameValidationScheme: model.UTF8Validation,
	}}
)

func relabelTestConfigFunc(cfgs []*relabel.Config) func() config.Config {
	return func() config.Config {
		return config.Config{ReceiveRelabelConfigs: cfgs}
	}
}

func TestNewRelabelingAppendable(t *testing.T) {
	keepLabels := labels.FromStrings("__name__", "keep_me", "env", "prod")
	dropLabels := labels.FromStrings("__name__", "drop_me")
	relabeledLabels := labels.FromStrings("__name__", "keep_me", "env", "prod", "environment", "prod")

	for _, tc := range []struct {
		name        string
		configs     []*relabel.Config
		in          labels.Labels
		wantDropped bool
		wantLabels  labels.Labels
	}{
		{name: "no configs, passthrough", configs: nil, in: keepLabels, wantLabels: keepLabels},
		{name: "kept and relabeled", configs: relabelTestRewriteConfig, in: keepLabels, wantLabels: relabeledLabels},
		{name: "dropped", configs: relabelTestDropConfig, in: dropLabels, wantDropped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendable(appendable, relabelTestConfigFunc(tc.configs))
			app := wrapped.Appender(context.Background())

			ref, err := app.Append(0, tc.in, 10, 1)
			require.NoError(t, err)
			_, err = app.AppendExemplar(ref, tc.in, exemplar.Exemplar{Value: 1, Ts: 10})
			require.NoError(t, err)
			_, err = app.UpdateMetadata(ref, tc.in, metadata.Metadata{Type: model.MetricTypeCounter})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			results := appendable.ResultSamples()
			if tc.wantDropped {
				require.Empty(t, results)
				return
			}

			require.Len(t, results, 1)
			require.True(t, labels.Equal(tc.wantLabels, results[0].L), "got labels %v, want %v", results[0].L, tc.wantLabels)
			require.Len(t, results[0].ES, 1)
			require.Equal(t, model.MetricTypeCounter, results[0].M.Type)
		})
	}
}

func TestNewRelabelingAppendableV2(t *testing.T) {
	keepLabels := labels.FromStrings("__name__", "keep_me", "env", "prod")
	dropLabels := labels.FromStrings("__name__", "drop_me")
	relabeledLabels := labels.FromStrings("__name__", "keep_me", "env", "prod", "environment", "prod")

	for _, tc := range []struct {
		name        string
		configs     []*relabel.Config
		in          labels.Labels
		wantDropped bool
		wantLabels  labels.Labels
	}{
		{name: "no configs, passthrough", configs: nil, in: keepLabels, wantLabels: keepLabels},
		{name: "kept and relabeled", configs: relabelTestRewriteConfig, in: keepLabels, wantLabels: relabeledLabels},
		{name: "dropped", configs: relabelTestDropConfig, in: dropLabels, wantDropped: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			appendable := teststorage.NewAppendable()
			wrapped := NewRelabelingAppendableV2(appendable, relabelTestConfigFunc(tc.configs))
			app := wrapped.AppenderV2(context.Background())

			_, err := app.Append(0, tc.in, 0, 10, 1, nil, nil, storage.AOptions{
				Metadata:  metadata.Metadata{Type: model.MetricTypeCounter},
				Exemplars: []exemplar.Exemplar{{Value: 1, Ts: 10}},
			})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			results := appendable.ResultSamples()
			if tc.wantDropped {
				require.Empty(t, results)
				return
			}

			require.Len(t, results, 1)
			require.True(t, labels.Equal(tc.wantLabels, results[0].L), "got labels %v, want %v", results[0].L, tc.wantLabels)
			require.Len(t, results[0].ES, 1)
			require.Equal(t, model.MetricTypeCounter, results[0].M.Type)
		})
	}
}

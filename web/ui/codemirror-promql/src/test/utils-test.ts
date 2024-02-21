// Copyright 2021 The Prometheus Authors
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

import { parser } from '@prometheus-io/lezer-promql';
import { EditorState } from '@codemirror/state';
import { LRLanguage } from '@codemirror/language';
import nock from 'nock';
import path from 'path';
import { fileURLToPath } from 'url';

const lightPromQLSyntax = LRLanguage.define({ parser: parser });

const __dirname = path.dirname(fileURLToPath(import.meta.url));

export function createEditorState(expr: string): EditorState {
  return EditorState.create({
    doc: expr,
    extensions: lightPromQLSyntax,
  });
}

export function mockPrometheusServer(): void {
  nock('http://localhost:8080')
    .get('/api/v1/label/__name__/values')
    .query(true)
    .replyWithFile(200, __dirname + '/metric_name.json')
    .get('/api/v1/metadata')
    .replyWithFile(200, __dirname + '/metadata.json')
    .get('/api/v1/series')
    .query(true)
    .replyWithFile(200, __dirname + '/alertmanager_alerts_series.json')
    .post('/api/v1/series')
    .replyWithFile(200, __dirname + '/alertmanager_alerts_series.json');
}

export const mockedMetricsTerms = [
  {
    label: 'ALERTS',
    type: 'constant',
  },
  {
    label: 'ALERTS_FOR_STATE',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: 'How many alerts by state.',
    label: 'alertmanager_alerts',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total number of received alerts that were invalid.',
    label: 'alertmanager_alerts_invalid_total',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total number of received alerts.',
    label: 'alertmanager_alerts_received_total',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: "A metric with a constant '1' value labeled by version, revision, branch, and goversion from which alertmanager was built.",
    label: 'alertmanager_build_info',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: 'Indicates whether the clustering is enabled or not.',
    label: 'alertmanager_cluster_enabled',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: 'Hash of the currently loaded alertmanager configuration.',
    label: 'alertmanager_config_hash',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: 'Timestamp of the last successful configuration reload.',
    label: 'alertmanager_config_last_reload_success_timestamp_seconds',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: 'Whether the last configuration reload attempt was successful.',
    label: 'alertmanager_config_last_reload_successful',
    type: 'constant',
  },
  {
    detail: 'gauge',
    info: 'Number of active aggregation groups',
    label: 'alertmanager_dispatcher_aggregation_groups',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total number of observations for: Summary of latencies for the processing of alerts.',
    label: 'alertmanager_dispatcher_alert_processing_duration_seconds_count',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total sum of observations for: Summary of latencies for the processing of alerts.',
    label: 'alertmanager_dispatcher_alert_processing_duration_seconds_sum',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'Total number of times an HTTP request failed because the concurrency limit was reached.',
    label: 'alertmanager_http_concurrency_limit_exceeded_total',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total count of observations for a bucket in the histogram: Histogram of latencies for HTTP requests.',
    label: 'alertmanager_http_request_duration_seconds_bucket',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total number of observations for: Histogram of latencies for HTTP requests.',
    label: 'alertmanager_http_request_duration_seconds_count',
    type: 'constant',
  },
  {
    detail: 'counter',
    info: 'The total sum of observations for: Histogram of latencies for HTTP requests.',
    label: 'alertmanager_http_request_duration_seconds_sum',
    type: 'constant',
  },
];

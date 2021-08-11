// The MIT License (MIT)
//
// Copyright (c) 2020 The Prometheus Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

import { parser } from 'lezer-promql';
import { EditorState } from '@codemirror/state';
import { LezerLanguage } from '@codemirror/language';
import nock from 'nock';

// used to inject an implementation of fetch in NodeJS
require('isomorphic-fetch');

const lightPromQLSyntax = LezerLanguage.define({ parser: parser });

export function createEditorState(expr: string): EditorState {
  return EditorState.create({
    doc: expr,
    extensions: lightPromQLSyntax,
  });
}

export function mockPrometheusServer() {
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

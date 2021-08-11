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

import { HybridComplete } from './hybrid';
import { CachedPrometheusClient, HTTPPrometheusClient, PrometheusClient, PrometheusConfig } from '../client/prometheus';
import { CompletionContext, CompletionResult } from '@codemirror/autocomplete';

// Complete is the interface that defines the simple method that returns a CompletionResult.
// Every different completion mode must implement this interface.
export interface CompleteStrategy {
  promQL(context: CompletionContext): Promise<CompletionResult | null> | CompletionResult | null;
}

// CompleteConfiguration should be used to customize the autocompletion.
export interface CompleteConfiguration {
  remote?: PrometheusConfig | PrometheusClient;
  // maxMetricsMetadata is the maximum number of metrics in Prometheus for which metadata is fetched.
  // If the number of metrics exceeds this limit, no metric metadata is fetched at all.
  maxMetricsMetadata?: number;
  // When providing this custom CompleteStrategy, the settings above will not be used.
  completeStrategy?: CompleteStrategy;
}

function isPrometheusConfig(remoteConfig: PrometheusConfig | PrometheusClient): remoteConfig is PrometheusConfig {
  return (remoteConfig as PrometheusConfig).url !== undefined;
}

export function newCompleteStrategy(conf?: CompleteConfiguration): CompleteStrategy {
  if (conf?.completeStrategy) {
    return conf.completeStrategy;
  }
  if (conf?.remote) {
    if (!isPrometheusConfig(conf.remote)) {
      return new HybridComplete(conf.remote, conf.maxMetricsMetadata);
    }
    return new HybridComplete(new CachedPrometheusClient(new HTTPPrometheusClient(conf.remote), conf.remote.cache), conf.maxMetricsMetadata);
  }
  return new HybridComplete();
}

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

import { FetchFn } from './index';
import { Matcher } from '../types';
import { labelMatchersToString } from '../parser';
import { LRUCache } from 'lru-cache';

export interface MetricMetadata {
  type: string;
  help: string;
}

export interface PrometheusClient {
  labelNames(metricName?: string): Promise<string[]>;

  // labelValues return a list of the value associated to the given labelName.
  // In case a metric is provided, then the list of values is then associated to the couple <MetricName, LabelName>
  labelValues(labelName: string, metricName?: string, matchers?: Matcher[]): Promise<string[]>;

  metricMetadata(): Promise<Record<string, MetricMetadata[]>>;

  series(metricName: string, matchers?: Matcher[], labelName?: string): Promise<Map<string, string>[]>;

  // metricNames returns a list of suggestions for the metric name given the `prefix`.
  // Note that the returned list can be a superset of those suggestions for the prefix (i.e., including ones without the
  // prefix), as codemirror will filter these out when displaying suggestions to the user.
  metricNames(prefix?: string): Promise<string[]>;
  // flags returns flag values that prometheus was configured with.
  flags(): Promise<Record<string, string>>;
}

export interface CacheConfig {
  // maxAge is the maximum amount of time that a cached completion item is valid before it needs to be refreshed.
  // It is in milliseconds. Default value:  300 000 (5min)
  maxAge?: number;
  // the cache can be initialized with a list of metrics
  initialMetricList?: string[];
}

export interface PrometheusConfig {
  url?: string;
  lookbackInterval?: number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  httpErrorHandler?: (error: any) => void;
  fetchFn?: FetchFn;
  // cache will allow user to change the configuration of the cached Prometheus client (which is used by default)
  cache?: CacheConfig;
  httpMethod?: 'POST' | 'GET';
  apiPrefix?: string;
  requestHeaders?: Headers;
}

interface APIResponse<T> {
  status: 'success' | 'error';
  data?: T;
  error?: string;
  warnings?: string[];
  infos?: string[];
}

// These are status codes where the Prometheus API still returns a valid JSON body,
// with an error encoded within the JSON.
const badRequest = 400;
const unprocessableEntity = 422;
const serviceUnavailable = 503;

// HTTPPrometheusClient is the HTTP client that should be used to get some information from the different endpoint provided by prometheus.
export class HTTPPrometheusClient implements PrometheusClient {
  private readonly lookbackInterval = 60 * 60 * 1000 * 12; //12 hours
  private readonly url: string;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  private readonly errorHandler?: (error: any) => void;
  private readonly httpMethod: 'POST' | 'GET' = 'POST';
  private readonly apiPrefix: string = '/api/v1';
  // For some reason, just assigning via "= fetch" here does not end up executing fetch correctly
  // when calling it, thus the indirection via another function wrapper.
  private readonly fetchFn: FetchFn = (input: RequestInfo, init?: RequestInit): Promise<Response> => fetch(input, init);
  private requestHeaders: Headers = new Headers();

  constructor(config: PrometheusConfig) {
    this.url = config.url ? config.url : '';
    this.errorHandler = config.httpErrorHandler;
    if (config.lookbackInterval) {
      this.lookbackInterval = config.lookbackInterval;
    }
    if (config.fetchFn) {
      this.fetchFn = config.fetchFn;
    }
    if (config.httpMethod) {
      this.httpMethod = config.httpMethod;
    }
    if (config.apiPrefix) {
      this.apiPrefix = config.apiPrefix;
    }
    if (config.requestHeaders) {
      this.requestHeaders = config.requestHeaders;
    }
  }

  labelNames(metricName?: string): Promise<string[]> {
    const end = new Date();
    const start = new Date(end.getTime() - this.lookbackInterval);
    if (metricName === undefined || metricName === '') {
      const request = this.buildRequest(
        this.labelsEndpoint(),
        new URLSearchParams({
          start: start.toISOString(),
          end: end.toISOString(),
        })
      );
      // See https://prometheus.io/docs/prometheus/latest/querying/api/#getting-label-names
      return this.fetchAPI<string[]>(request.uri, {
        method: this.httpMethod,
        body: request.body,
      }).catch((error) => {
        if (this.errorHandler) {
          this.errorHandler(error);
        }
        return [];
      });
    }

    return this.series(metricName).then((series) => {
      const labelNames = new Set<string>();
      for (const labelSet of series) {
        for (const [key] of Object.entries(labelSet)) {
          if (key === '__name__') {
            continue;
          }
          labelNames.add(key);
        }
      }
      return Array.from(labelNames);
    });
  }

  // labelValues return a list of the value associated to the given labelName.
  // In case a metric is provided, then the list of values is then associated to the couple <MetricName, LabelName>
  labelValues(labelName: string, metricName?: string, matchers?: Matcher[]): Promise<string[]> {
    const end = new Date();
    const start = new Date(end.getTime() - this.lookbackInterval);

    if (!metricName || metricName.length === 0) {
      const params: URLSearchParams = new URLSearchParams({
        start: start.toISOString(),
        end: end.toISOString(),
      });
      // See https://prometheus.io/docs/prometheus/latest/querying/api/#querying-label-values
      return this.fetchAPI<string[]>(`${this.labelValuesEndpoint().replace(/:name/gi, labelName)}?${params}`).catch((error) => {
        if (this.errorHandler) {
          this.errorHandler(error);
        }
        return [];
      });
    }

    return this.series(metricName, matchers, labelName).then((series) => {
      const labelValues = new Set<string>();
      for (const labelSet of series) {
        for (const [key, value] of Object.entries(labelSet)) {
          if (key === '__name__') {
            continue;
          }
          if (key === labelName) {
            labelValues.add(value);
          }
        }
      }
      return Array.from(labelValues);
    });
  }

  metricMetadata(): Promise<Record<string, MetricMetadata[]>> {
    return this.fetchAPI<Record<string, MetricMetadata[]>>(this.metricMetadataEndpoint()).catch((error) => {
      if (this.errorHandler) {
        this.errorHandler(error);
      }
      return {};
    });
  }

  series(metricName: string, matchers?: Matcher[], labelName?: string): Promise<Map<string, string>[]> {
    const end = new Date();
    const start = new Date(end.getTime() - this.lookbackInterval);
    const request = this.buildRequest(
      this.seriesEndpoint(),
      new URLSearchParams({
        start: start.toISOString(),
        end: end.toISOString(),
        'match[]': labelMatchersToString(metricName, matchers, labelName),
      })
    );
    // See https://prometheus.io/docs/prometheus/latest/querying/api/#finding-series-by-label-matchers
    return this.fetchAPI<Map<string, string>[]>(request.uri, {
      method: this.httpMethod,
      body: request.body,
    }).catch((error) => {
      if (this.errorHandler) {
        this.errorHandler(error);
      }
      return [];
    });
  }

  metricNames(): Promise<string[]> {
    return this.labelValues('__name__');
  }

  flags(): Promise<Record<string, string>> {
    return this.fetchAPI<Record<string, string>>(this.flagsEndpoint()).catch((error) => {
      if (this.errorHandler) {
        this.errorHandler(error);
      }
      return {};
    });
  }

  private fetchAPI<T>(resource: string, init?: RequestInit): Promise<T> {
    if (init) {
      init.headers = this.requestHeaders;
    } else {
      init = { headers: this.requestHeaders };
    }
    return this.fetchFn(this.url + resource, init)
      .then((res) => {
        if (!res.ok && ![badRequest, unprocessableEntity, serviceUnavailable].includes(res.status)) {
          throw new Error(res.statusText);
        }
        return res;
      })
      .then((res) => res.json())
      .then((apiRes: APIResponse<T>) => {
        if (apiRes.status === 'error') {
          throw new Error(apiRes.error !== undefined ? apiRes.error : 'missing "error" field in response JSON');
        }
        if (apiRes.data === undefined) {
          throw new Error('missing "data" field in response JSON');
        }
        return apiRes.data;
      });
  }

  private buildRequest(endpoint: string, params: URLSearchParams) {
    let uri = endpoint;
    let body: URLSearchParams | null = params;
    if (this.httpMethod === 'GET') {
      uri = `${uri}?${params}`;
      body = null;
    }
    return { uri, body };
  }

  private labelsEndpoint(): string {
    return `${this.apiPrefix}/labels`;
  }

  private labelValuesEndpoint(): string {
    return `${this.apiPrefix}/label/:name/values`;
  }

  private seriesEndpoint(): string {
    return `${this.apiPrefix}/series`;
  }

  private metricMetadataEndpoint(): string {
    return `${this.apiPrefix}/metadata`;
  }

  private flagsEndpoint(): string {
    return `${this.apiPrefix}/status/flags`;
  }
}

class Cache {
  // completeAssociation is the association between a metric name, a label name and the possible label values
  private readonly completeAssociation: LRUCache<string, Map<string, Set<string>>>;
  // metricMetadata is the association between a metric name and the associated metadata
  private metricMetadata: Record<string, MetricMetadata[]>;
  private labelValues: LRUCache<string, string[]>;
  private labelNames: string[];
  private flags: Record<string, string>;

  constructor(config?: CacheConfig) {
    const maxAge = {
      ttl: config && config.maxAge ? config.maxAge : 5 * 60 * 1000,
      ttlAutopurge: true,
    };
    this.completeAssociation = new LRUCache<string, Map<string, Set<string>>>(maxAge);
    this.metricMetadata = {};
    this.labelValues = new LRUCache<string, string[]>(maxAge);
    this.labelNames = [];
    this.flags = {};
    if (config?.initialMetricList) {
      this.setLabelValues('__name__', config.initialMetricList);
    }
  }

  setAssociations(metricName: string, series: Map<string, string>[]): void {
    series.forEach((labelSet: Map<string, string>) => {
      let currentAssociation = this.completeAssociation.get(metricName);
      if (!currentAssociation) {
        currentAssociation = new Map<string, Set<string>>();
        this.completeAssociation.set(metricName, currentAssociation);
      }

      for (const [key, value] of Object.entries(labelSet)) {
        if (key === '__name__') {
          continue;
        }
        const labelValues = currentAssociation.get(key);
        if (labelValues === undefined) {
          currentAssociation.set(key, new Set<string>([value]));
        } else {
          labelValues.add(value);
        }
      }
    });
  }

  setFlags(flags: Record<string, string>): void {
    this.flags = flags;
  }

  getFlags(): Record<string, string> {
    return this.flags;
  }

  setMetricMetadata(metadata: Record<string, MetricMetadata[]>): void {
    this.metricMetadata = metadata;
  }

  getMetricMetadata(): Record<string, MetricMetadata[]> {
    return this.metricMetadata;
  }

  setLabelNames(labelNames: string[]): void {
    this.labelNames = labelNames;
  }

  getLabelNames(metricName?: string): string[] {
    if (!metricName || metricName.length === 0) {
      return this.labelNames;
    }
    const labelSet = this.completeAssociation.get(metricName);
    return labelSet ? Array.from(labelSet.keys()) : [];
  }

  setLabelValues(labelName: string, labelValues: string[]): void {
    this.labelValues.set(labelName, labelValues);
  }

  getLabelValues(labelName: string, metricName?: string): string[] {
    if (!metricName || metricName.length === 0) {
      const result = this.labelValues.get(labelName);
      return result ? result : [];
    }

    const labelSet = this.completeAssociation.get(metricName);
    if (labelSet) {
      const labelValues = labelSet.get(labelName);
      return labelValues ? Array.from(labelValues) : [];
    }
    return [];
  }
}

export class CachedPrometheusClient implements PrometheusClient {
  private readonly cache: Cache;
  private readonly client: PrometheusClient;

  constructor(client: PrometheusClient, config?: CacheConfig) {
    this.client = client;
    this.cache = new Cache(config);
  }

  labelNames(metricName?: string): Promise<string[]> {
    const cachedLabel = this.cache.getLabelNames(metricName);
    if (cachedLabel && cachedLabel.length > 0) {
      return Promise.resolve(cachedLabel);
    }

    if (metricName === undefined || metricName === '') {
      return this.client.labelNames().then((labelNames) => {
        this.cache.setLabelNames(labelNames);
        return labelNames;
      });
    }
    return this.series(metricName).then(() => {
      return this.cache.getLabelNames(metricName);
    });
  }

  labelValues(labelName: string, metricName?: string): Promise<string[]> {
    const cachedLabel = this.cache.getLabelValues(labelName, metricName);
    if (cachedLabel && cachedLabel.length > 0) {
      return Promise.resolve(cachedLabel);
    }

    if (metricName === undefined || metricName === '') {
      return this.client.labelValues(labelName).then((labelValues) => {
        this.cache.setLabelValues(labelName, labelValues);
        return labelValues;
      });
    }

    return this.series(metricName).then(() => {
      return this.cache.getLabelValues(labelName, metricName);
    });
  }

  metricMetadata(): Promise<Record<string, MetricMetadata[]>> {
    const cachedMetadata = this.cache.getMetricMetadata();
    if (cachedMetadata && Object.keys(cachedMetadata).length > 0) {
      return Promise.resolve(cachedMetadata);
    }

    return this.client.metricMetadata().then((metadata) => {
      this.cache.setMetricMetadata(metadata);
      return metadata;
    });
  }

  series(metricName: string): Promise<Map<string, string>[]> {
    return this.client.series(metricName).then((series) => {
      this.cache.setAssociations(metricName, series);
      return series;
    });
  }

  metricNames(): Promise<string[]> {
    return this.labelValues('__name__');
  }

  flags(): Promise<Record<string, string>> {
    const cachedFlags = this.cache.getFlags();
    if (cachedFlags && Object.keys(cachedFlags).length > 0) {
      return Promise.resolve(cachedFlags);
    }

    return this.client.flags().then((flags) => {
      this.cache.setFlags(flags);
      return flags;
    });
  }
}

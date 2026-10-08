// Copyright 2025 The Prometheus Authors
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

import nock from 'nock';
import { HTTPPrometheusClient, CachedPrometheusClient } from './prometheus';

describe('HTTPPrometheusClient destroy', () => {
  it('should be safe to call destroy multiple times', () => {
    const client = new HTTPPrometheusClient({ url: 'http://localhost:8080' });
    // First call
    client.destroy();
    // Second call should not throw
    expect(() => client.destroy()).not.toThrow();
  });

  it('should abort in-flight requests when destroy is called', async () => {
    let abortSignal: AbortSignal | null | undefined;

    const mockFetch = (_url: RequestInfo, init?: RequestInit): Promise<Response> => {
      abortSignal = init?.signal;
      // Return a promise that never resolves to simulate an in-flight request
      return new Promise(() => {});
    };

    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: mockFetch,
    });

    // Start a request (don't await it)
    client.labelNames();

    // Verify the signal was captured and not aborted yet
    expect(abortSignal).toBeDefined();
    expect(abortSignal?.aborted).toBe(false);

    // Destroy the client
    client.destroy();

    // Verify the request was aborted
    expect(abortSignal?.aborted).toBe(true);
  });
});

describe('CachedPrometheusClient destroy', () => {
  it('should be safe to call destroy multiple times', () => {
    const httpClient = new HTTPPrometheusClient({ url: 'http://localhost:8080' });
    const cachedClient = new CachedPrometheusClient(httpClient);

    // First call
    cachedClient.destroy();
    // Second call should not throw
    expect(() => cachedClient.destroy()).not.toThrow();
  });

  it('should call destroy on the underlying HTTPPrometheusClient', () => {
    const httpClient = new HTTPPrometheusClient({ url: 'http://localhost:8080' });

    let destroyCalled = false;
    const originalDestroy = httpClient.destroy.bind(httpClient);
    httpClient.destroy = () => {
      destroyCalled = true;
      originalDestroy();
    };

    const cachedClient = new CachedPrometheusClient(httpClient);
    cachedClient.destroy();

    expect(destroyCalled).toBe(true);
  });

  it('should handle underlying clients without destroy method', () => {
    // Create a minimal PrometheusClient without destroy
    const minimalClient = {
      labelNames: () => Promise.resolve([]),
      labelValues: () => Promise.resolve([]),
      metricMetadata: () => Promise.resolve({}),
      series: () => Promise.resolve([]),
      metricNames: () => Promise.resolve([]),
      flags: () => Promise.resolve({}),
    };

    const cachedClient = new CachedPrometheusClient(minimalClient);

    // Should not throw even though underlying client has no destroy
    expect(() => cachedClient.destroy()).not.toThrow();
  });
});

describe('HTTPPrometheusClient utf-8 names', () => {
  afterEach(() => {
    nock.cleanAll();
  });

  const testCases = [
    {
      title: 'escapes the label name in the path, and quotes the metric name in the selector',
      labelName: 'http.status_code',
      metricName: 'http.requests',
      path: '/api/v1/label/U__http_2e_status__code/values',
      selector: '{"http.requests"}',
    },
    { title: 'does not escape a legacy label name', labelName: 'job', metricName: 'up', path: '/api/v1/label/job/values', selector: 'up' },
    { title: 'does not escape __name__ and has no selector without a metric name', labelName: '__name__', path: '/api/v1/label/__name__/values' },
    { title: 'escapes non-ascii characters of the label name', labelName: 'é😀', path: '/api/v1/label/U___e9__1f600_/values' },
    { title: 'escapes characters that have a meaning in a replacement string', labelName: 'a$&b', path: '/api/v1/label/U__a_24__26_b/values' },
    { title: 'escapes a label name containing a colon', labelName: 'a:b', path: '/api/v1/label/a:b/values' },
  ];
  testCases.forEach((value) => {
    it(value.title, async () => {
      const scope = nock('http://localhost:8080')
        .get(value.path)
        .query((query) => query['match[]'] === value.selector)
        .reply(200, { status: 'success', data: ['200', '500'] });
      const client = new HTTPPrometheusClient({ url: 'http://localhost:8080' });
      await expect(client.labelValues(value.labelName, value.metricName)).resolves.toEqual(['200', '500']);
      expect(scope.isDone()).toBe(true);
    });
  });
});

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

describe('CachedPrometheusClient label caches', () => {
  // A client that knows two metrics: "up" has the label "job", "node_load1" has "job" and "instance".
  const fakeClient = () => {
    const calls = { labelNames: 0, labelValues: 0 };
    const client = {
      labelNames: (metricName?: string) => {
        calls.labelNames++;
        return Promise.resolve(metricName === 'up' ? ['__name__', 'job'] : ['__name__', 'instance', 'job']);
      },
      labelValues: (labelName: string, metricName?: string) => {
        calls.labelValues++;
        return Promise.resolve(metricName === 'up' ? ['prometheus'] : ['node', 'prometheus']);
      },
      metricMetadata: () => Promise.resolve({}),
      series: () => Promise.resolve([]),
      metricNames: () => Promise.resolve([]),
      flags: () => Promise.resolve({}),
    };
    return { client, calls };
  };

  it('does not replace the global label names with the names of one metric', async () => {
    const { client, calls } = fakeClient();
    const cachedClient = new CachedPrometheusClient(client);

    expect(await cachedClient.labelNames('up')).toEqual(['job']);
    expect(await cachedClient.labelNames()).toEqual(['__name__', 'instance', 'job']);
    expect(calls.labelNames).toBe(2);
  });

  it('does not replace the global label values with the values of one metric', async () => {
    const { client, calls } = fakeClient();
    const cachedClient = new CachedPrometheusClient(client);

    expect(await cachedClient.labelValues('job', 'up')).toEqual(['prometheus']);
    expect(await cachedClient.labelValues('job')).toEqual(['node', 'prometheus']);
    expect(calls.labelValues).toBe(2);
  });

  it('still caches the global lists', async () => {
    const { client, calls } = fakeClient();
    const cachedClient = new CachedPrometheusClient(client);

    await cachedClient.labelNames();
    await cachedClient.labelNames();
    await cachedClient.labelValues('job');
    await cachedClient.labelValues('job');
    expect(calls).toEqual({ labelNames: 1, labelValues: 1 });
  });
});

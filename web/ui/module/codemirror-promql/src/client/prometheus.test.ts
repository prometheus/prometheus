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

describe('HTTPPrometheusClient labelValues', () => {
  const requestedPath = async (labelName: string): Promise<string> => {
    let requested = '';
    const mockFetch = (input: RequestInfo): Promise<Response> => {
      requested = input as string;
      return Promise.resolve(new Response(JSON.stringify({ status: 'success', data: [] })));
    };
    const client = new HTTPPrometheusClient({ url: 'http://localhost:8080', fetchFn: mockFetch });
    await client.labelValues(labelName);
    return requested.replace('http://localhost:8080', '').split('?')[0];
  };

  it('requests a legacy label name as is', async () => {
    expect(await requestedPath('job')).toBe('/api/v1/label/job/values');
    expect(await requestedPath('__name__')).toBe('/api/v1/label/__name__/values');
  });

  it('escapes label names that are not legacy-valid with the U__ encoding', async () => {
    // The API requires this encoding for names that contain a "/".
    expect(await requestedPath('app.kubernetes.io/name')).toBe('/api/v1/label/U__app_2e_kubernetes_2e_io_2f_name/values');
    expect(await requestedPath('http.status_code')).toBe('/api/v1/label/U__http_2e_status__code/values');
    expect(await requestedPath('0abc')).toBe('/api/v1/label/U___30_abc/values');
  });

  it('escapes label names that start with the U__ prefix', async () => {
    expect(await requestedPath('U__foo')).toBe('/api/v1/label/U__U____foo/values');
  });
});

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

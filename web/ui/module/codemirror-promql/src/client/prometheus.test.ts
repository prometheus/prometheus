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

import { HTTPPrometheusClient, CachedPrometheusClient, type InfoLabelSearchRequest, type InfoSearchResult, type PrometheusClient } from '.';
import { jest } from '@jest/globals';

// NdjsonResponse yields string or byte chunks; bytes can split UTF-8 sequences.
function ndjsonResponse(chunks: (string | Uint8Array)[], buffered = false): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder();
      for (const c of chunks) {
        controller.enqueue(typeof c === 'string' ? encoder.encode(c) : c);
      }
      controller.close();
    },
  });
  const response = new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'application/x-ndjson; charset=utf-8' },
  });
  if (buffered) {
    // Simulate a runtime that exposes text() without a streaming body.
    Object.defineProperty(response, 'body', { value: null });
  }
  return response;
}

describe('HTTPPrometheusClient info-label NDJSON parsing', () => {
  describe.each([false, true])('buffered=%s', (buffered) => {
    describe.each(['name', 'value'] as const)('%s results', (field) => {
      it.each([
        { name: 'batch and trailer warnings', batches: [['partial', 'café'], ['partial']], trailer: ['late'], hasMore: false },
        { name: 'truncation with warnings', batches: [['partial']], trailer: [], hasMore: true },
        { name: 'terminal-only success', batches: [], trailer: ['late'], hasMore: false },
        { name: 'absent warnings', batches: [undefined], trailer: undefined, hasMore: false },
      ])('preserves $name', async ({ batches, trailer, hasMore }) => {
        const value = field === 'name' ? 'région' : 'café';
        const lines = batches.map((warnings, index) => ({
          results: index === 0 ? [{ [field]: value, extensions: { mimir: { cardinality: 42 } } }] : [],
          warnings,
        }));
        const body = [...lines, { status: 'success', has_more: hasMore, warnings: trailer }].map((line) => JSON.stringify(line) + '\n').join('');
        const bytes = new TextEncoder().encode(body);
        const client = new HTTPPrometheusClient({
          fetchFn: () =>
            Promise.resolve(
              ndjsonResponse(
                Array.from(bytes, (byte) => new Uint8Array([byte])),
                buffered
              )
            ),
        });
        const result = await (field === 'name' ? client.infoLabelNames() : client.infoLabelValues('env'));
        expect(result).toEqual({
          results: batches.length === 0 ? [] : [value],
          hasMore,
          warnings: [...batches.flatMap((warnings) => warnings ?? []), ...(trailer ?? [])],
        });
      });
    });

    it.each([
      { name: 'invalid JSON', body: '{\n' },
      { name: 'array line', body: '[]\n' },
      { name: 'null line', body: 'null\n' },
      { name: 'null results', body: '{"results":null}\n' },
      { name: 'string warnings', body: '{"results":[],"warnings":"partial"}\n' },
      { name: 'non-string batch warning', body: '{"results":[],"warnings":[1]}\n' },
      { name: 'null trailer warnings', body: '{"status":"success","has_more":false,"warnings":null}\n' },
      { name: 'non-string trailer warning', body: '{"status":"success","has_more":false,"warnings":[1]}\n' },
      { name: 'mixed batch and success', body: '{"results":[],"status":"success","has_more":false}\n{"status":"success","has_more":false}\n' },
      { name: 'mixed batch and error', body: '{"results":[],"status":"error","errorType":"internal","error":"boom"}\n' },
      { name: 'batch with terminal flag', body: '{"results":[],"has_more":false}\n{"status":"success","has_more":false}\n' },
      { name: 'missing trailer', body: '{"results":[{"name":"env"}]}\n' },
      { name: 'duplicate trailer', body: '{"status":"success","has_more":false}\n{"status":"success","has_more":false}\n' },
      { name: 'content after trailer', body: '{"status":"success","has_more":false}\n{"results":[]}\n' },
      { name: 'non-boolean has_more', body: '{"status":"success","has_more":"false"}\n' },
      { name: 'unknown record', body: '{"warnings":[]}\n' },
      { name: 'terminal error after a batch', body: '{"results":[{"name":"env"}]}\n{"status":"error","errorType":"timeout","error":"timed out"}\n' },
      { name: 'invalid name record', body: '{"results":[{"value":"prod"}]}\n{"status":"success","has_more":false}\n' },
    ])('rejects $name', async ({ body }) => {
      const client = new HTTPPrometheusClient({ fetchFn: () => Promise.resolve(ndjsonResponse([body], buffered)) });
      await expect(client.infoLabelNames()).rejects.toThrow();
    });
  });

  it.each([400, 422, 503, 500])('preserves the JSON error message for HTTP %s', async (status) => {
    const client = new HTTPPrometheusClient({
      fetchFn: () =>
        Promise.resolve(new Response(JSON.stringify({ status: 'error', errorType: 'unavailable', error: 'search backend unavailable' }), { status })),
    });
    await expect(client.infoLabelValues('env')).rejects.toThrow('search backend unavailable');
  });

  it.each([400, 422, 503])('rejects apparent success in HTTP %s', async (status) => {
    const client = new HTTPPrometheusClient({
      fetchFn: () => Promise.resolve(new Response('{"results":[]}\n{"status":"success","has_more":false}\n', { status })),
    });
    await expect(client.infoLabelNames()).rejects.toThrow(`HTTP ${status}`);
  });

  it('uses the HTTP status when an error body is unusable', async () => {
    const client = new HTTPPrometheusClient({
      fetchFn: () => Promise.resolve(new Response('<html>unavailable</html>', { status: 502, statusText: 'Bad Gateway' })),
    });
    await expect(client.infoLabelNames()).rejects.toThrow('Bad Gateway');
  });

  it('cancels a malformed stream and releases its reader without masking the error', async () => {
    let signal: AbortSignal | null | undefined;
    const cancel = jest.fn(() => Promise.reject(new Error('cancel failed')));
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode('{"results":[],"warnings":[1]}\n'));
      },
      cancel,
    });
    const client = new HTTPPrometheusClient({
      fetchFn: (_input, init) => {
        signal = init?.signal;
        return Promise.resolve(new Response(stream));
      },
    });
    await expect(client.infoLabelNames()).rejects.toThrow('invalid info label warnings');
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(signal?.aborted).toBe(true);
    expect(stream.locked).toBe(false);
  });
  it('accumulates name results across batches and preserves has_more', async () => {
    const body = ['{"results":[{"name":"env"}]}\n', '{"results":[{"name":"version"}]}\n', '{"status":"success","has_more":true}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env', 'version'], hasMore: true, warnings: [] });
  });

  it('handles a body without a trailing newline', async () => {
    const body = ['{"results":[{"name":"env"}]}\n', '{"status":"success","has_more":false}'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env'], hasMore: false, warnings: [] });
  });

  it('handles chunked arrival that splits inside a line', async () => {
    // Chunks are split mid-record to exercise the streaming line buffer:
    // the decoder must hold the partial line across reads and reassemble it
    // before the per-line parser sees a complete JSON document.
    const body = ['{"results":[{"name":"e', 'nv"}', ']}\n{"results":[{"name":"region"', '}]}\n', '{"status":"success","has_more":false}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env', 'region'], hasMore: false, warnings: [] });
  });

  describe.each(['GET', 'POST'] as const)('%s request routing', (httpMethod) => {
    it.each([
      { name: 'default API prefix', apiPrefix: undefined, expectedPrefix: '/api/v1' },
      { name: 'custom API prefix', apiPrefix: '/prometheus/api/v1', expectedPrefix: '/prometheus/api/v1' },
    ])('uses the search endpoints with $name', async ({ apiPrefix, expectedPrefix }) => {
      const requests: { url: URL; init?: RequestInit }[] = [];
      const client = new HTTPPrometheusClient({
        url: 'http://localhost:8080',
        httpMethod,
        apiPrefix,
        fetchFn: (input, init) => {
          requests.push({ url: new URL(String(input)), init });
          const record = requests.length === 1 ? { name: 'k8s.cluster' } : { value: 'prod' };
          return Promise.resolve(ndjsonResponse([JSON.stringify({ results: [record] }) + '\n', '{"status":"success","has_more":false}\n']));
        },
      });
      const request = {
        expr: 'up',
        dataMatches: ['__name__=~".*_info"', '__name__!~"build.*"', 'env="prod"'],
        search: 'pr',
      };

      await expect(client.infoLabelNames(request)).resolves.toEqual({ results: ['k8s.cluster'], hasMore: false, warnings: [] });
      await expect(client.infoLabelValues('k8s.cluster', request)).resolves.toEqual({ results: ['prod'], hasMore: false, warnings: [] });
      expect(requests).toHaveLength(2);
      for (const [i, endpoint] of ['info_labels', 'info_label_values'].entries()) {
        const { url, init } = requests[i];
        expect(url.origin).toBe('http://localhost:8080');
        expect(url.pathname).toBe(`${expectedPrefix}/search/${endpoint}`);
        expect(init?.method).toBe(httpMethod);
        const params = httpMethod === 'GET' ? url.searchParams : new URLSearchParams(String(init?.body));
        if (httpMethod === 'POST') {
          expect(url.search).toBe('');
        } else {
          expect(init?.body).toBeNull();
        }
        expect(params.get('label')).toBe(i === 0 ? null : 'k8s.cluster');
        expect(params.get('expr')).toBe('up');
        expect(params.get('search[]')).toBe('pr');
        expect(params.getAll('data_match[]')).toEqual(request.dataMatches);
        expect(params.has('limit')).toBe(false);
        expect(params.has('match[]')).toBe(false);
      }
    });
  });

  it('surfaces an in-band errorType line as a rejected Promise to the error handler', async () => {
    const body = ['{"results":[{"name":"env"}]}\n', '{"status":"error","errorType":"internal","error":"boom"}\n'];
    let handledError: unknown;
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
      httpErrorHandler: (err) => {
        handledError = err;
      },
    });

    await expect(client.infoLabelNames()).rejects.toThrow('boom');
    expect((handledError as Error).message).toBe('boom');
  });

  it('rejects an incomplete stream without a success trailer', async () => {
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(['{"results":[{"name":"env"}]}\n'])),
    });
    await expect(client.infoLabelNames()).rejects.toThrow('without a success trailer');
  });

  it('rejects a non-object NDJSON line', async () => {
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(['[]\n'])),
    });
    await expect(client.infoLabelNames()).rejects.toThrow('invalid info label NDJSON line');
  });

  it('handles an empty first batch carrying warnings', async () => {
    const body = ['{"results":[],"warnings":["something happened"]}\n', '{"status":"success","has_more":false}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: [], hasMore: false, warnings: ['something happened'] });
  });

  it('ignores blank lines in the stream', async () => {
    const body = ['\n', '{"results":[{"name":"env"}]}\n', '\n', '{"status":"success","has_more":false}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env'], hasMore: false, warnings: [] });
  });

  it('aborts a streaming read when destroy() is called mid-stream', async () => {
    // Producer enqueues one batch line and then parks — never closes the
    // controller, never enqueues more — simulating a slow server. The
    // client should observe destroy() via its AbortSignal, exit the read
    // loop cleanly, and reject the incomplete request.
    let capturedSignal: AbortSignal | null | undefined;
    let streamController!: ReadableStreamDefaultController<Uint8Array>;
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        streamController = controller;
        const encoder = new TextEncoder();
        controller.enqueue(encoder.encode('{"results":[{"name":"env"}]}\n'));
        // No close(); the next read() parks until aborted.
      },
    });
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: (_url: RequestInfo, init?: RequestInit) => {
        capturedSignal = init?.signal;
        capturedSignal?.addEventListener('abort', () => streamController.error(new DOMException('aborted', 'AbortError')));
        return Promise.resolve(
          new Response(stream, {
            status: 200,
            headers: { 'Content-Type': 'application/x-ndjson; charset=utf-8' },
          })
        );
      },
    });

    const pending = client.infoLabelNames();
    // Yield once so the streaming reader gets a chance to start.
    await Promise.resolve();
    client.destroy();

    await expect(pending).rejects.toThrow();
    expect(capturedSignal?.aborted).toBe(true);
  });
});

function stubPrometheusClient(overrides: Partial<PrometheusClient> = {}): PrometheusClient {
  return {
    labelNames: () => Promise.resolve([]),
    labelValues: () => Promise.resolve([]),
    metricMetadata: () => Promise.resolve({}),
    series: () => Promise.resolve([]),
    metricNames: () => Promise.resolve([]),
    flags: () => Promise.resolve({}),
    infoLabelNames: () => Promise.resolve({ results: [], hasMore: false, warnings: [] }),
    infoLabelValues: () => Promise.resolve({ results: [], hasMore: false, warnings: [] }),
    ...overrides,
  };
}

describe('CachedPrometheusClient info-label caching', () => {
  describe.each(['names', 'values'] as const)('%s cache identity', (operation) => {
    it.each([
      {
        name: 'property order',
        first: { expr: 'up', dataMatches: ['env="prod"'], search: 'ver' },
        second: { search: 'ver', expr: 'up', dataMatches: ['env="prod"'] },
      },
      { name: 'omitted fields', first: {}, second: { expr: '', dataMatches: [], search: '' } },
    ])('deduplicates and caches requests with equivalent $name', async ({ first, second }) => {
      let resolveRequest!: (result: InfoSearchResult<string>) => void;
      const pending = new Promise<InfoSearchResult<string>>((resolve) => {
        resolveRequest = resolve;
      });
      const lookup = jest.fn(() => pending);
      const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelNames: lookup, infoLabelValues: lookup }));
      const request = (query: InfoLabelSearchRequest) =>
        operation === 'names' ? client.infoLabelNames(query) : client.infoLabelValues('env', query);
      const firstResult = request(first);
      const secondResult = request(second);
      expect(lookup).toHaveBeenCalledTimes(1);
      const expected = { results: ['region'], hasMore: false, warnings: ['partial'] };
      resolveRequest(expected);
      await expect(Promise.all([firstResult, secondResult])).resolves.toEqual([expected, expected]);
      await expect(request(second)).resolves.toEqual(expected);
      expect(lookup).toHaveBeenCalledTimes(1);
    });
  });

  it('keeps expression, matcher order and spelling, and search text in cache identity', async () => {
    const lookup = jest.fn(() => Promise.resolve({ results: [], hasMore: false, warnings: [] }));
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelNames: lookup }));
    const query = { expr: 'up', dataMatches: ['env="prod"', 'version=~"v.+"'], search: 'ver' };
    const queries = [
      query,
      { ...query, expr: 'rate(http_requests_total[5m])' },
      { ...query, dataMatches: [...query.dataMatches].reverse() },
      { ...query, dataMatches: ['"env"="prod"', 'version=~"v.+"'] },
      { ...query, search: 'region' },
    ];
    for (const request of queries) {
      await client.infoLabelNames(request);
    }
    await client.infoLabelNames(query);
    expect(lookup).toHaveBeenCalledTimes(queries.length);
    expect(lookup).toHaveBeenNthCalledWith(3, queries[2]);
  });

  it('keeps the operation and exact selected label in cache identity', async () => {
    const lookup = jest.fn(() => Promise.resolve({ results: [], hasMore: false, warnings: [] }));
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelNames: lookup, infoLabelValues: lookup }));
    const query = { expr: 'up' };
    await client.infoLabelNames(query);
    await client.infoLabelValues('env', query);
    await client.infoLabelValues('Env', query);
    await client.infoLabelValues('env', query);
    expect(lookup).toHaveBeenCalledTimes(3);
  });

  it('deduplicates in-flight requests for the same effective name query', async () => {
    let resolveRequest!: (value: InfoSearchResult<string>) => void;
    const request = new Promise<InfoSearchResult<string>>((resolve) => {
      resolveRequest = resolve;
    });
    const infoLabelNames = jest.fn(() => request);
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelNames }));

    const query = { expr: 'up', dataMatches: ['env="prod"'], search: 'ver' };
    const first = client.infoLabelNames(query);
    const second = client.infoLabelNames(query);
    expect(infoLabelNames).toHaveBeenCalledTimes(1);
    resolveRequest({ results: ['version'], hasMore: false, warnings: ['partial'] });
    await expect(Promise.all([first, second])).resolves.toEqual([
      { results: ['version'], hasMore: false, warnings: ['partial'] },
      { results: ['version'], hasMore: false, warnings: ['partial'] },
    ]);
    await expect(client.infoLabelNames(query)).resolves.toEqual({ results: ['version'], hasMore: false, warnings: ['partial'] });
    expect(infoLabelNames).toHaveBeenCalledTimes(1);
  });

  it.each(['name', 'value'] as const)('caches %s warnings and truncation metadata', async (field) => {
    const fetchFn = jest.fn(() =>
      Promise.resolve(
        ndjsonResponse([
          JSON.stringify({ results: [{ [field]: 'region' }], warnings: ['partial'] }) + '\n',
          '{"status":"success","has_more":true,"warnings":["late"]}\n',
        ])
      )
    );
    const client = new CachedPrometheusClient(new HTTPPrometheusClient({ fetchFn }));
    const request = field === 'name' ? () => client.infoLabelNames() : () => client.infoLabelValues('env');
    const expected = { results: ['region'], hasMore: true, warnings: ['partial', 'late'] };
    await expect(request()).resolves.toEqual(expected);
    await expect(request()).resolves.toEqual(expected);
    expect(fetchFn).toHaveBeenCalledTimes(1);
  });

  it.each([
    { name: 'malformed warnings', body: '{"results":[{"name":"env"}],"warnings":[1]}\n{"status":"success","has_more":false}\n', status: 200 },
    { name: 'incomplete stream', body: '{"results":[{"name":"env"}]}\n', status: 200 },
    { name: 'terminal error', body: '{"results":[{"name":"env"}]}\n{"status":"error","errorType":"timeout","error":"timed out"}\n', status: 200 },
    { name: 'non-success HTTP status', body: '{"results":[{"name":"env"}]}\n{"status":"success","has_more":false}\n', status: 400 },
  ])('retries after $name and caches only the completed response', async ({ body, status }) => {
    const fetchFn = jest
      .fn()
      .mockResolvedValueOnce(new Response(body, { status }))
      .mockImplementation(() =>
        Promise.resolve(ndjsonResponse(['{"results":[{"name":"region"}]}\n{"status":"success","has_more":false,"warnings":["late"]}\n']))
      );
    const client = new CachedPrometheusClient(new HTTPPrometheusClient({ fetchFn }));
    await expect(client.infoLabelNames()).rejects.toThrow();
    const expected = { results: ['region'], hasMore: false, warnings: ['late'] };
    await expect(client.infoLabelNames()).resolves.toEqual(expected);
    await expect(client.infoLabelNames()).resolves.toEqual(expected);
    expect(fetchFn).toHaveBeenCalledTimes(2);
  });

  it('evicts failed requests so a later call retries', async () => {
    const infoLabelValues = jest
      .fn()
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce({ results: ['prod'], hasMore: false, warnings: [] });
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelValues }));

    await expect(client.infoLabelValues('env', { expr: 'up' })).rejects.toThrow('network down');
    await expect(client.infoLabelValues('env', { expr: 'up' })).resolves.toEqual({ results: ['prod'], hasMore: false, warnings: [] });
    expect(infoLabelValues).toHaveBeenCalledTimes(2);
  });

  it('bounds each info-label cache to 100 effective requests', async () => {
    const infoLabelNames = jest.fn((request: InfoLabelSearchRequest = {}) =>
      Promise.resolve({ results: [request.search ?? ''], hasMore: false, warnings: [] })
    );
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelNames }));

    for (let i = 0; i <= 100; i++) {
      await client.infoLabelNames({ expr: 'up', search: String(i) });
    }
    await client.infoLabelNames({ expr: 'up', search: '0' });
    expect(infoLabelNames).toHaveBeenCalledTimes(102);
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
    const cachedClient = new CachedPrometheusClient(stubPrometheusClient());

    // Should not throw even though underlying client has no destroy
    expect(() => cachedClient.destroy()).not.toThrow();
  });
});

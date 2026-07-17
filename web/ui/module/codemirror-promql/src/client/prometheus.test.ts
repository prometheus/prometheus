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

import { HTTPPrometheusClient, CachedPrometheusClient, type InfoLabelSearchRequest, type PrometheusClient } from '.';
import { jest } from '@jest/globals';

// ndjsonResponse builds a Response whose body is a ReadableStream that yields
// the given chunks in order, simulating chunked NDJSON arrival from the
// server. Each chunk is encoded as UTF-8 bytes before being enqueued, so the
// client's TextDecoder sees realistic byte input that may split inside
// multi-byte sequences or mid-line.
function ndjsonResponse(chunks: string[]): Response {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      const encoder = new TextEncoder();
      for (const c of chunks) {
        controller.enqueue(encoder.encode(c));
      }
      controller.close();
    },
  });
  return new Response(stream, {
    status: 200,
    headers: { 'Content-Type': 'application/x-ndjson; charset=utf-8' },
  });
}

describe('HTTPPrometheusClient info-label NDJSON parsing', () => {
  it('accumulates name results across batches and preserves has_more', async () => {
    const body = ['{"results":[{"name":"env"}]}\n', '{"results":[{"name":"version"}]}\n', '{"status":"success","has_more":true}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env', 'version'], hasMore: true });
  });

  it('handles a body without a trailing newline', async () => {
    const body = ['{"results":[{"name":"env"}]}\n', '{"status":"success","has_more":false}'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env'], hasMore: false });
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
    expect(result).toEqual({ results: ['env', 'region'], hasMore: false });
  });

  it('uses the exact label and typed search without a client-selected limit', async () => {
    let requestURL = '';
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      httpMethod: 'GET',
      fetchFn: (input) => {
        requestURL = String(input);
        return Promise.resolve(ndjsonResponse(['{"results":[{"value":"prod"}]}\n', '{"status":"success","has_more":false}\n']));
      },
    });

    await expect(
      client.infoLabelValues('k8s.cluster', {
        expr: 'up',
        metricMatches: ['__name__=~".*_info"', '__name__!~"build.*"'],
        dataMatches: ['env="prod"'],
        search: 'pr',
      })
    ).resolves.toEqual({
      results: ['prod'],
      hasMore: false,
    });
    const url = new URL(requestURL);
    expect(url.pathname).toBe('/api/v1/info_label_values');
    expect(url.searchParams.get('label')).toBe('k8s.cluster');
    expect(url.searchParams.get('search[]')).toBe('pr');
    expect(url.searchParams.getAll('metric_match[]')).toEqual(['__name__=~".*_info"', '__name__!~"build.*"']);
    expect(url.searchParams.getAll('data_match[]')).toEqual(['env="prod"']);
    expect(url.searchParams.has('limit')).toBe(false);
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
    expect(result).toEqual({ results: [], hasMore: false });
  });

  it('ignores blank lines in the stream', async () => {
    const body = ['\n', '{"results":[{"name":"env"}]}\n', '\n', '{"status":"success","has_more":false}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelNames();
    expect(result).toEqual({ results: ['env'], hasMore: false });
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
    infoLabelNames: () => Promise.resolve({ results: [], hasMore: false }),
    infoLabelValues: () => Promise.resolve({ results: [], hasMore: false }),
    ...overrides,
  };
}

describe('CachedPrometheusClient info-label caching', () => {
  it('deduplicates in-flight requests for the same effective name query', async () => {
    let resolveRequest!: (value: { results: string[]; hasMore: boolean }) => void;
    const request = new Promise<{ results: string[]; hasMore: boolean }>((resolve) => {
      resolveRequest = resolve;
    });
    const infoLabelNames = jest.fn(() => request);
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelNames }));

    const query = { expr: 'up', dataMatches: ['env="prod"'], search: 'ver' };
    const first = client.infoLabelNames(query);
    const second = client.infoLabelNames(query);
    expect(infoLabelNames).toHaveBeenCalledTimes(1);
    resolveRequest({ results: ['version'], hasMore: false });
    await expect(Promise.all([first, second])).resolves.toEqual([
      { results: ['version'], hasMore: false },
      { results: ['version'], hasMore: false },
    ]);
  });

  it('evicts failed requests so a later call retries', async () => {
    const infoLabelValues = jest
      .fn()
      .mockRejectedValueOnce(new Error('network down'))
      .mockResolvedValueOnce({ results: ['prod'], hasMore: false });
    const client = new CachedPrometheusClient(stubPrometheusClient({ infoLabelValues }));

    await expect(client.infoLabelValues('env', { expr: 'up' })).rejects.toThrow('network down');
    await expect(client.infoLabelValues('env', { expr: 'up' })).resolves.toEqual({ results: ['prod'], hasMore: false });
    expect(infoLabelValues).toHaveBeenCalledTimes(2);
  });

  it('bounds each info-label cache to 100 effective requests', async () => {
    const infoLabelNames = jest.fn((request: InfoLabelSearchRequest = {}) => Promise.resolve({ results: [request.search ?? ''], hasMore: false }));
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

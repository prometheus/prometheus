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

import { HTTPPrometheusClient, CachedPrometheusClient } from '.';

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

describe('HTTPPrometheusClient.infoLabelPairs NDJSON parsing', () => {
  it('accumulates results across batches and surfaces them as a Record', async () => {
    const body = [
      '{"results":[{"name":"env","values":["prod","staging"]}]}\n',
      '{"results":[{"name":"version","values":["1.0","2.0"]}]}\n',
      '{"status":"success","has_more":false}\n',
    ];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelPairs();
    expect(result).toEqual({
      env: ['prod', 'staging'],
      version: ['1.0', '2.0'],
    });
  });

  it('handles a body without a trailing newline', async () => {
    const body = ['{"results":[{"name":"env","values":["prod"]}]}\n', '{"status":"success","has_more":false}'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelPairs();
    expect(result).toEqual({ env: ['prod'] });
  });

  it('handles chunked arrival that splits inside a line', async () => {
    // Chunks are split mid-record to exercise the streaming line buffer:
    // the decoder must hold the partial line across reads and reassemble it
    // before the per-line parser sees a complete JSON document.
    const body = [
      '{"results":[{"name":"e',
      'nv","values":["prod","staging"]}',
      ']}\n{"results":[{"name":"region",',
      '"values":["us"]}]}\n',
      '{"status":"success","has_more":false}\n',
    ];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelPairs();
    expect(result).toEqual({
      env: ['prod', 'staging'],
      region: ['us'],
    });
  });

  it('surfaces an in-band errorType line as a rejected Promise to the error handler', async () => {
    const body = ['{"results":[{"name":"env","values":["prod"]}]}\n', '{"status":"error","errorType":"internal","error":"boom"}\n'];
    let handledError: unknown;
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
      httpErrorHandler: (err) => {
        handledError = err;
      },
    });

    const result = await client.infoLabelPairs();
    // infoLabelPairs catches and returns {} on error.
    expect(result).toEqual({});
    expect((handledError as Error).message).toBe('boom');
  });

  it('handles an empty first batch carrying warnings', async () => {
    const body = ['{"results":[],"warnings":["something happened"]}\n', '{"status":"success","has_more":false}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelPairs();
    expect(result).toEqual({});
  });

  it('ignores blank lines in the stream', async () => {
    const body = ['\n', '{"results":[{"name":"env","values":["prod"]}]}\n', '\n', '{"status":"success","has_more":false}\n'];
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: () => Promise.resolve(ndjsonResponse(body)),
    });

    const result = await client.infoLabelPairs();
    expect(result).toEqual({ env: ['prod'] });
  });

  it('aborts a streaming read when destroy() is called mid-stream', async () => {
    // Producer enqueues one batch line and then parks — never closes the
    // controller, never enqueues more — simulating a slow server. The
    // client should observe destroy() via its AbortSignal, exit the read
    // loop cleanly, and surface {} (per infoLabelPairs' catch-all).
    let capturedSignal: AbortSignal | null | undefined;
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        const encoder = new TextEncoder();
        controller.enqueue(encoder.encode('{"results":[{"name":"env","values":["prod"]}]}\n'));
        // No close(); the next read() parks until aborted.
      },
    });
    const client = new HTTPPrometheusClient({
      url: 'http://localhost:8080',
      fetchFn: (_url: RequestInfo, init?: RequestInit) => {
        capturedSignal = init?.signal;
        return Promise.resolve(
          new Response(stream, {
            status: 200,
            headers: { 'Content-Type': 'application/x-ndjson; charset=utf-8' },
          })
        );
      },
    });

    const pending = client.infoLabelPairs();
    // Yield once so the streaming reader gets a chance to start.
    await Promise.resolve();
    client.destroy();

    const result = await pending;
    expect(result).toEqual({});
    expect(capturedSignal?.aborted).toBe(true);
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
      infoLabelPairs: () => Promise.resolve({}),
    };

    const cachedClient = new CachedPrometheusClient(minimalClient);

    // Should not throw even though underlying client has no destroy
    expect(() => cachedClient.destroy()).not.toThrow();
  });
});

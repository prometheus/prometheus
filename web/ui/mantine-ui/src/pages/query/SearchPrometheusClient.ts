// Copyright The Prometheus Authors
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

import {
  labelMatchersToString,
  Matcher,
  PrometheusClient,
} from "@prometheus-io/codemirror-promql";
import { filter as fuzzyFilter } from "@nexucis/fuzzy";
import { API_PATH } from "../../api/api";

export interface MetricMetadata {
  type: string;
  help: string;
}

interface NDJSONResult {
  results: string[];
  hasMore: boolean;
  metadata: Record<string, MetricMetadata[]>;
}

// parseMetricNamesNDJSON parses a streaming NDJSON response from the
// metric_names search endpoint, extracting names and optional metadata.
async function parseMetricNamesNDJSON(
  response: Response
): Promise<NDJSONResult> {
  const text = await response.text();
  const lines = text.trim().split("\n").filter(Boolean);
  const results: string[] = [];
  const metadata: Record<string, MetricMetadata[]> = {};
  let hasMore = false;

  for (const line of lines) {
    const obj = JSON.parse(line) as Record<string, unknown>;
    if ("status" in obj) {
      if (obj.status === "error") {
        throw new Error(
          typeof obj.error === "string" ? obj.error : "unknown search error"
        );
      }
      hasMore = obj.has_more === true;
    } else if ("results" in obj && Array.isArray(obj.results)) {
      for (const item of obj.results as Record<string, unknown>[]) {
        if (typeof item.name !== "string") {
          continue;
        }
        results.push(item.name);
        if (item.type !== undefined || item.help !== undefined) {
          metadata[item.name] = [
            {
              type: typeof item.type === "string" ? item.type : "",
              help: typeof item.help === "string" ? item.help : "",
            },
          ];
        }
      }
    }
  }

  return { results, hasMore, metadata };
}

// parseSearchNDJSON parses a streaming NDJSON response from the search API
// and extracts string values using the provided extractor function.
async function parseSearchNDJSON(
  response: Response,
  extract: (obj: Record<string, unknown>) => string | undefined
): Promise<NDJSONResult> {
  const text = await response.text();
  const lines = text.trim().split("\n").filter(Boolean);
  const results: string[] = [];
  let hasMore = false;

  for (const line of lines) {
    const obj = JSON.parse(line) as Record<string, unknown>;
    if ("status" in obj) {
      if (obj.status === "error") {
        throw new Error(
          typeof obj.error === "string" ? obj.error : "unknown search error"
        );
      }
      hasMore = obj.has_more === true;
    } else if ("results" in obj && Array.isArray(obj.results)) {
      for (const item of obj.results as Record<string, unknown>[]) {
        const value = extract(item);
        if (value !== undefined) {
          results.push(value);
        }
      }
    }
  }

  return { results, hasMore, metadata: {} };
}

// fetchJSONData fetches a standard Prometheus JSON API endpoint and returns
// the data field.
async function fetchJSONData<T>(url: string): Promise<T> {
  const res = await fetch(url, {
    cache: "no-store",
    credentials: "same-origin",
  });
  if (!res.ok) {
    throw new Error(res.statusText);
  }
  const json = (await res.json()) as {
    status: string;
    data?: T;
    error?: string;
  };
  if (json.status === "error") {
    throw new Error(json.error ?? "unknown API error");
  }
  if (json.data === undefined) {
    throw new Error('missing "data" field in response');
  }
  return json.data;
}

// PrefixCache caches search results keyed by prefix. When a query with a
// longer prefix is requested and a shorter prefix already has a complete
// result set (hasMore=false), the cached results are returned directly
// without a network request. CodemirrorPromQL filters them client-side.
class PrefixCache {
  private readonly cache = new Map<string, NDJSONResult>();

  // lookup returns cached results if a shorter-or-equal cached prefix exists
  // with hasMore=false (meaning the full result set was returned).
  // The cached results are re-filtered using the subsequence fuzzy algorithm.
  lookup(prefix: string): string[] | null {
    for (let len = prefix.length; len >= 0; len--) {
      const entry = this.cache.get(prefix.slice(0, len));
      if (entry && !entry.hasMore) {
        if (!prefix) {
          return entry.results;
        }
        return fuzzyFilter(prefix, entry.results).map((r) => r.original);
      }
    }
    return null;
  }

  set(prefix: string, result: NDJSONResult): void {
    this.cache.set(prefix, result);
  }
}

// SearchPrometheusClient is a PrometheusClient implementation that uses
// the Prometheus search API endpoints for metric names, label names, and
// label values. Other methods fall back to the standard API.
export class SearchPrometheusClient implements PrometheusClient {
  private readonly baseURL: string;
  private readonly metricNamesCache = new PrefixCache();
  private cachedMetadata: Record<string, MetricMetadata[]> = {};
  // labelNamesCache is keyed by metricName (no user-typed prefix in the interface).
  private readonly labelNamesCache = new Map<string, string[]>();
  // labelValuesCache is keyed by "labelName\0matchStr".
  private readonly labelValuesCache = new Map<string, string[]>();

  constructor(pathPrefix: string) {
    this.baseURL = `${pathPrefix}/${API_PATH}`;
  }

  async metricNames(prefix?: string): Promise<string[]> {
    const key = prefix ?? "";

    const cached = this.metricNamesCache.lookup(key);
    if (cached !== null) {
      return cached;
    }

    const params = new URLSearchParams({
      fuzz_alg: "subsequence",
      fuzz_threshold: "0",
      include_metadata: "true",
      limit: "200",
    });
    if (prefix) {
      params.set("search", prefix);
    }
    const res = await fetch(`${this.baseURL}/search/metric_names?${params}`, {
      cache: "no-store",
      credentials: "same-origin",
    });
    if (!res.ok) {
      return [];
    }
    const result = await parseMetricNamesNDJSON(res).catch(() => ({
      results: [] as string[],
      hasMore: false,
      metadata: {} as Record<string, MetricMetadata[]>,
    }));

    Object.assign(this.cachedMetadata, result.metadata);
    this.metricNamesCache.set(key, result);
    return result.results;
  }

  async labelNames(metricName?: string): Promise<string[]> {
    const key = metricName ?? "";

    const cached = this.labelNamesCache.get(key);
    if (cached !== undefined) {
      return cached;
    }

    const params = new URLSearchParams({
      fuzz_alg: "subsequence",
      fuzz_threshold: "0",
      limit: "200",
    });
    if (metricName) {
      params.set("match[]", `{__name__="${metricName}"}`);
    }
    const res = await fetch(`${this.baseURL}/search/label_names?${params}`, {
      cache: "no-store",
      credentials: "same-origin",
    });
    if (!res.ok) {
      return [];
    }
    const result = await parseSearchNDJSON(
      res,
      (item) => (typeof item.name === "string" ? item.name : undefined)
    ).catch(() => ({ results: [] as string[], hasMore: false }));

    this.labelNamesCache.set(key, result.results);
    return result.results;
  }

  async labelValues(
    labelName: string,
    metricName?: string,
    matchers?: Matcher[]
  ): Promise<string[]> {
    const matchStr = metricName
      ? labelMatchersToString(metricName, matchers)
      : "";
    const key = `${labelName}\0${matchStr}`;

    const cached = this.labelValuesCache.get(key);
    if (cached !== undefined) {
      return cached;
    }

    const params = new URLSearchParams({
      label: labelName,
      fuzz_alg: "subsequence",
      fuzz_threshold: "0",
      limit: "200",
    });
    if (metricName) {
      params.set("match[]", matchStr);
    }
    const res = await fetch(`${this.baseURL}/search/label_values?${params}`, {
      cache: "no-store",
      credentials: "same-origin",
    });
    if (!res.ok) {
      return [];
    }
    const result = await parseSearchNDJSON(
      res,
      (item) => (typeof item.name === "string" ? item.name : undefined)
    ).catch(() => ({ results: [] as string[], hasMore: false }));

    this.labelValuesCache.set(key, result.results);
    return result.results;
  }

  async metricMetadata(): Promise<Record<string, MetricMetadata[]>> {
    return this.cachedMetadata;
  }

  async series(
    metricName: string,
    matchers?: Matcher[]
  ): Promise<Map<string, string>[]> {
    const params = new URLSearchParams({
      "match[]": labelMatchersToString(metricName, matchers),
    });
    return fetchJSONData<Map<string, string>[]>(
      `${this.baseURL}/series?${params}`
    ).catch(() => []);
  }

  async flags(): Promise<Record<string, string>> {
    return fetchJSONData<Record<string, string>>(
      `${this.baseURL}/status/flags`
    ).catch(() => ({}));
  }
}

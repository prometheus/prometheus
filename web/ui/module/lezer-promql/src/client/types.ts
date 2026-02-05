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

// =============================================================================
// IMPORTANT: Parser Term ID Coupling
// =============================================================================
// These constants MUST match the values generated in ../parser.terms.js.
// They are duplicated here because the client bundle is built separately
// from the parser and cannot import from the generated parser files.
//
// If you regenerate the parser (via `npm run build`), verify these values
// still match by checking src/parser.terms.js:
//   - EqlSingle, EqlRegex, Neq, NeqRegex
//
// If they differ, update the values below. Failure to keep these in sync
// will cause Matcher.matchesEmpty() and labelMatchersToString() to silently
// produce incorrect results.
// =============================================================================
export const EqlSingle = 162;
export const EqlRegex = 163;
export const Neq = 63;
export const NeqRegex = 164;

export type FetchFn = (input: RequestInfo, init?: RequestInit) => Promise<Response>;

export interface MetricMetadata {
  type: string;
  help: string;
}

export class Matcher {
  type: number;
  name: string;
  value: string;

  constructor(type: number, name: string, value: string) {
    this.type = type;
    this.name = name;
    this.value = value;
  }

  matchesEmpty(): boolean {
    switch (this.type) {
      case EqlSingle:
        return this.value === '';
      case Neq:
        return this.value !== '';
      default:
        return false;
    }
  }
}

export function labelMatchersToString(metricName: string, matchers?: Matcher[], labelName?: string): string {
  if (!matchers || matchers.length === 0) {
    return metricName;
  }

  let matchersAsString = '';
  for (const matcher of matchers) {
    if (matcher.name === labelName || matcher.value === '') {
      continue;
    }
    let type = '';
    switch (matcher.type) {
      case EqlSingle:
        type = '=';
        break;
      case Neq:
        type = '!=';
        break;
      case NeqRegex:
        type = '!~';
        break;
      case EqlRegex:
        type = '=~';
        break;
      default:
        type = '=';
    }
    const m = `${matcher.name}${type}"${matcher.value}"`;
    if (matchersAsString === '') {
      matchersAsString = m;
    } else {
      matchersAsString = `${matchersAsString},${m}`;
    }
  }
  return `${metricName}{${matchersAsString}}`;
}

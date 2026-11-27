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

import { EqlRegex, EqlSingle, Neq, NeqRegex } from '@prometheus-io/lezer-promql';

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
      case EqlRegex:
        return regexMatchesEmpty(this.value) === true;
      case NeqRegex:
        return regexMatchesEmpty(this.value) === false;
      default:
        return false;
    }
  }
}

// regexMatchesEmpty tells whether the regular expression written in a double-quoted string
// (with its escape sequences) fully matches the empty string, like the matchers of Prometheus.
// It returns undefined when the expression can't be evaluated here: escape sequences that
// JSON doesn't support, or a syntax that JavaScript regular expressions don't support.
function regexMatchesEmpty(value: string): boolean | undefined {
  try {
    const re = new RegExp(`^(?:${JSON.parse(`"${value}"`)})$`);
    return re.test('');
  } catch {
    return undefined;
  }
}

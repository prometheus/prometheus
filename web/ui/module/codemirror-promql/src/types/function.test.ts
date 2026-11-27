// Copyright 2026 The Prometheus Authors
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

import { parser } from '@prometheus-io/lezer-promql';
import { getFunction, PromQLFunction } from './function';
// functionSignatures.ts is generated from promql/parser/functions.go, and CI checks that it is up to date.
import { functionSignatures } from '../../../../mantine-ui/src/promql/functionSignatures';

// min_of and max_of are regular (experimental) functions in the Go parser, but lezer-promql only supports them
// inside duration expressions so far, so they have no entry in the function table yet.
const durationOnlyFunctions = ['min_of', 'max_of'];

describe('promqlFunctions', () => {
  it('matches the function signatures of the Go parser', () => {
    // The table is keyed by the grammar's node IDs, so probe every node ID to collect all of its entries.
    const actual: { [name: string]: PromQLFunction } = {};
    for (let id = 0; id < parser.nodeSet.types.length; id++) {
      const fn = getFunction(id);
      if (fn) {
        actual[fn.name] = fn;
      }
    }

    const expected: typeof functionSignatures = {};
    for (const { name, argTypes, variadic, returnType } of Object.values(functionSignatures)) {
      if (!durationOnlyFunctions.includes(name)) {
        expected[name] = { name, argTypes, variadic, returnType };
      }
    }

    expect(actual).toEqual(expected);
  });
});

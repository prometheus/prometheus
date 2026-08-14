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

import { PromQLExtension } from './promql';
import { CompleteStrategy } from './complete';
import { CompletionResult } from '@codemirror/autocomplete';

describe('PromQLExtension destroy', () => {
  it('should be safe to call destroy multiple times', () => {
    const extension = new PromQLExtension();
    // First call
    extension.destroy();
    // Second call should not throw
    expect(() => extension.destroy()).not.toThrow();
  });

  it('should call destroy on the complete strategy if available', () => {
    const extension = new PromQLExtension();

    // Set up a mock complete strategy with destroy
    let destroyCalled = false;
    const mockCompleteStrategy: CompleteStrategy = {
      promQL: (): CompletionResult | null => null,
      destroy: () => {
        destroyCalled = true;
      },
    };

    extension.setComplete({ completeStrategy: mockCompleteStrategy });
    extension.destroy();

    expect(destroyCalled).toBe(true);
  });

  it('should handle complete strategies without destroy method', () => {
    const extension = new PromQLExtension();

    // Set up a mock complete strategy without destroy
    const mockCompleteStrategy: CompleteStrategy = {
      promQL: (): CompletionResult | null => null,
    };

    extension.setComplete({ completeStrategy: mockCompleteStrategy });

    // Should not throw even though complete strategy has no destroy
    expect(() => extension.destroy()).not.toThrow();
  });
});

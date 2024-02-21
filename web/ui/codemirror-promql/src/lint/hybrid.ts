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

import { LintStrategy } from './index';
import { EditorView } from '@codemirror/view';
import { Diagnostic } from '@codemirror/lint';
import { Parser } from '../parser';

// HybridLint will provide a promQL linter with static analysis
export class HybridLint implements LintStrategy {
  public promQL(this: HybridLint): (view: EditorView) => readonly Diagnostic[] {
    return (view: EditorView) => {
      const parser = new Parser(view.state);
      parser.analyze();
      return parser.getDiagnostics();
    };
  }
}

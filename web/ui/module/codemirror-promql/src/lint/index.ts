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

import { EditorView } from '@codemirror/view';
import { Diagnostic, linter } from '@codemirror/lint';
import { HybridLint } from './hybrid';
import { Extension } from '@codemirror/state';

type lintFunc = (view: EditorView) => readonly Diagnostic[] | Promise<readonly Diagnostic[]>;

// LintStrategy is the interface that defines the simple method that returns a DiagnosticResult.
// Every different lint mode must implement this interface.
export interface LintStrategy {
  promQL(this: LintStrategy): lintFunc;
}

export function newLintStrategy(): LintStrategy {
  return new HybridLint();
}

export function promQLLinter(callbackFunc: (this: LintStrategy) => lintFunc, thisArg: LintStrategy): Extension {
  return linter(callbackFunc.call(thisArg));
}

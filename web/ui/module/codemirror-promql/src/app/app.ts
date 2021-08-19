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

import { basicSetup } from '@codemirror/basic-setup';
import { EditorState } from '@codemirror/state';
import { EditorView } from '@codemirror/view';
import { LanguageType, PromQLExtension } from '../lang-promql';
import { customTheme, promQLHighlightMaterialTheme } from './theme';

const promqlExtension = new PromQLExtension();
let editor: EditorView;

function getLanguageType(): LanguageType {
  const completionSelect = document.getElementById('languageType') as HTMLSelectElement;
  const completionValue = completionSelect.options[completionSelect.selectedIndex].value;
  switch (completionValue) {
    case 'promql':
      return LanguageType.PromQL;
    case 'metricName':
      return LanguageType.MetricName;
    default:
      return LanguageType.PromQL;
  }
}

function setCompletion() {
  const completionSelect = document.getElementById('completion') as HTMLSelectElement;
  const completionValue = completionSelect.options[completionSelect.selectedIndex].value;
  switch (completionValue) {
    case 'offline':
      promqlExtension.setComplete();
      break;
    case 'prometheus':
      promqlExtension.setComplete({
        remote: {
          url: 'https://prometheus.demo.do.prometheus.io',
        },
      });
      break;
    default:
      promqlExtension.setComplete();
  }
}

function createEditor() {
  let doc = '';
  if (editor) {
    // When the linter is changed, it required to reload completely the editor.
    // So the first thing to do, is to completely delete the previous editor and to recreate it from scratch
    // We should preserve the current text entered as well.
    doc = editor.state.sliceDoc(0, editor.state.doc.length);
    editor.destroy();
  }
  editor = new EditorView({
    state: EditorState.create({
      extensions: [basicSetup, promqlExtension.asExtension(getLanguageType()), promQLHighlightMaterialTheme, customTheme],
      doc: doc,
    }),
    // eslint-disable-next-line @typescript-eslint/no-non-null-assertion
    parent: document.querySelector('#editor')!,
  });
}

function applyConfiguration(): void {
  setCompletion();
  createEditor();
}

createEditor();

// eslint-disable-next-line @typescript-eslint/no-non-null-assertion,@typescript-eslint/ban-ts-ignore
// @ts-ignore
document.getElementById('apply').addEventListener('click', function () {
  applyConfiguration();
});

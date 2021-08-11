// The MIT License (MIT)
//
// Copyright (c) 2020 The Prometheus Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

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

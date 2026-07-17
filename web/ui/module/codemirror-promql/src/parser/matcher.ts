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

import { SyntaxNode } from '@lezer/common';
import { EqlSingle, LabelName, MatchOp, StringLiteral, UnquotedLabelMatcher, QuotedLabelMatcher, QuotedLabelName } from '@prometheus-io/lezer-promql';
import { EditorState } from '@codemirror/state';
import { Matcher } from '@prometheus-io/lezer-promql/client';

// Re-export labelMatchersToString from lezer-promql/client for backwards compatibility
export { labelMatchersToString } from '@prometheus-io/lezer-promql/client';

function decodeHexEscape(value: string, digits: number): string {
  if (value.length !== digits || !/^[0-9a-fA-F]+$/.test(value)) {
    throw new Error('invalid PromQL string escape');
  }
  const codePoint = Number.parseInt(value, 16);
  if (codePoint > 0x10ffff || (codePoint >= 0xd800 && codePoint <= 0xdfff)) {
    throw new Error('invalid PromQL Unicode escape');
  }
  return String.fromCodePoint(codePoint);
}

/** Decodes one complete PromQL string literal. */
export function unquotePromQLStringLiteral(value: string): string {
  if (value.length < 2 || value[0] !== value[value.length - 1] || ![`'`, `"`, '`'].includes(value[0])) {
    throw new Error('invalid PromQL string literal');
  }
  const quote = value[0];
  const inner = value.slice(1, -1);
  if (quote === '`') {
    return inner.replace(/\r/g, '');
  }

  const simpleEscapes: Record<string, string> = {
    a: '\u0007',
    b: '\b',
    f: '\f',
    n: '\n',
    r: '\r',
    t: '\t',
    v: '\v',
    '\\': '\\',
    "'": "'",
    '"': '"',
  };
  let result = '';
  for (let i = 0; i < inner.length; i++) {
    if (inner[i] !== '\\') {
      result += inner[i];
      continue;
    }
    i++;
    if (i >= inner.length) {
      throw new Error('invalid PromQL string escape');
    }
    const escaped = inner[i];
    if (simpleEscapes[escaped] !== undefined) {
      result += simpleEscapes[escaped];
      continue;
    }
    if (escaped === 'x' || escaped === 'u' || escaped === 'U') {
      const digits = escaped === 'x' ? 2 : escaped === 'u' ? 4 : 8;
      result += decodeHexEscape(inner.slice(i + 1, i + 1 + digits), digits);
      i += digits;
      continue;
    }
    if (/[0-7]/.test(escaped)) {
      const octal = escaped + inner.slice(i + 1, i + 3);
      if (!/^[0-7]{3}$/.test(octal)) {
        throw new Error('invalid PromQL octal escape');
      }
      result += String.fromCodePoint(Number.parseInt(octal, 8));
      i += 2;
      continue;
    }
    throw new Error('invalid PromQL string escape');
  }
  return result;
}

function createMatcher(labelMatcher: SyntaxNode, state: EditorState): Matcher {
  const matcher = new Matcher(0, '', '');
  const cursor = labelMatcher.cursor();
  switch (cursor.type.id) {
    case QuotedLabelMatcher:
      if (!cursor.next()) {
        // weird case, that would mean the QuotedLabelMatcher doesn't have any child.
        return matcher;
      }
      do {
        switch (cursor.type.id) {
          case QuotedLabelName:
            matcher.name = unquotePromQLStringLiteral(state.sliceDoc(cursor.from, cursor.to));
            break;
          case MatchOp: {
            const ope = cursor.node.firstChild;
            if (ope) {
              matcher.type = ope.type.id;
            }
            break;
          }
          case StringLiteral:
            matcher.value = unquotePromQLStringLiteral(state.sliceDoc(cursor.from, cursor.to));
            break;
        }
      } while (cursor.nextSibling());
      break;
    case UnquotedLabelMatcher:
      if (!cursor.next()) {
        // weird case, that would mean the UnquotedLabelMatcher doesn't have any child.
        return matcher;
      }
      do {
        switch (cursor.type.id) {
          case LabelName:
            matcher.name = state.sliceDoc(cursor.from, cursor.to);
            break;
          case MatchOp: {
            const ope = cursor.node.firstChild;
            if (ope) {
              matcher.type = ope.type.id;
            }
            break;
          }
          case StringLiteral:
            matcher.value = unquotePromQLStringLiteral(state.sliceDoc(cursor.from, cursor.to));
            break;
        }
      } while (cursor.nextSibling());
      break;
    case QuotedLabelName:
      matcher.name = '__name__';
      matcher.value = unquotePromQLStringLiteral(state.sliceDoc(cursor.from, cursor.to));
      matcher.type = EqlSingle;
      break;
  }
  return matcher;
}

export function buildLabelMatchers(labelMatchers: SyntaxNode[], state: EditorState): Matcher[] {
  const matchers: Matcher[] = [];
  labelMatchers.forEach((value) => {
    matchers.push(createMatcher(value, state));
  });
  return matchers;
}

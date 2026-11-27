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
import {
  EqlRegex,
  EqlSingle,
  LabelName,
  MatchOp,
  Neq,
  NeqRegex,
  StringLiteral,
  UnquotedLabelMatcher,
  QuotedLabelMatcher,
  QuotedLabelName,
} from '@prometheus-io/lezer-promql';
import { EditorState } from '@codemirror/state';
import { Matcher } from '../types';

// stringLiteralValue returns the content of a PromQL string literal as it is written between
// double quotes, so labelMatchersToString can put it back between double quotes.
// Single-quoted and raw (backtick) strings follow other escaping rules.
function stringLiteralValue(literal: string): string {
  const content = literal.slice(1, -1);
  switch (literal[0]) {
    case "'":
      // Keep escape sequences, except \' which isn't needed between double quotes, and escape ".
      return content.replace(/\\.|"/g, (s) => (s === '"' ? '\\"' : s === "\\'" ? "'" : s));
    case '`':
      // Raw strings have no escape sequences: escape backslashes, double quotes and line breaks.
      return content.replace(/[\\"\n]/g, (s) => (s === '\n' ? '\\n' : `\\${s}`));
    default:
      return content;
  }
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
            matcher.name = state.sliceDoc(cursor.from, cursor.to).slice(1, -1);
            break;
          case MatchOp: {
            const ope = cursor.node.firstChild;
            if (ope) {
              matcher.type = ope.type.id;
            }
            break;
          }
          case StringLiteral:
            matcher.value = stringLiteralValue(state.sliceDoc(cursor.from, cursor.to));
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
            matcher.value = stringLiteralValue(state.sliceDoc(cursor.from, cursor.to));
            break;
        }
      } while (cursor.nextSibling());
      break;
    case QuotedLabelName:
      matcher.name = '__name__';
      matcher.value = state.sliceDoc(cursor.from, cursor.to).slice(1, -1);
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

export function labelMatchersToString(metricName: string, matchers?: Matcher[], labelName?: string): string {
  if (!matchers || matchers.length === 0) {
    return metricName;
  }

  let matchersAsString = '';
  for (const matcher of matchers) {
    if (matcher.name === labelName || matcher.value === '') {
      continue;
    }
    let type: string;
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

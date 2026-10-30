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
import { labelNameNeedsQuoting, metricNameNeedsQuoting, quotePromQLString, unquotePromQLString } from './utf8';

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
            matcher.name = unquotePromQLString(state.sliceDoc(cursor.from, cursor.to));
            break;
          case MatchOp: {
            const ope = cursor.node.firstChild;
            if (ope) {
              matcher.type = ope.type.id;
            }
            break;
          }
          case StringLiteral:
            matcher.value = unquotePromQLString(state.sliceDoc(cursor.from, cursor.to));
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
            matcher.value = unquotePromQLString(state.sliceDoc(cursor.from, cursor.to));
            break;
        }
      } while (cursor.nextSibling());
      break;
    case QuotedLabelName:
      matcher.name = '__name__';
      matcher.value = unquotePromQLString(state.sliceDoc(cursor.from, cursor.to));
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
  // A metric name that is not a valid legacy name has to be quoted, and moved inside the braces.
  const quotedMetricName = metricName !== '' && metricNameNeedsQuoting(metricName);
  if (!matchers || matchers.length === 0) {
    return quotedMetricName ? `{${quotePromQLString(metricName)}}` : metricName;
  }

  let matchersAsString = quotedMetricName ? quotePromQLString(metricName) : '';
  for (const matcher of matchers) {
    // The metric name is already set by the quoted metric name.
    const isQuotedMetricName = quotedMetricName && matcher.type === EqlSingle && matcher.name === '__name__' && matcher.value === metricName;
    if (matcher.name === labelName || matcher.value === '' || isQuotedMetricName) {
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
    const name = labelNameNeedsQuoting(matcher.name) ? quotePromQLString(matcher.name) : matcher.name;
    const m = `${name}${type}${quotePromQLString(matcher.value)}`;
    if (matchersAsString === '') {
      matchersAsString = m;
    } else {
      matchersAsString = `${matchersAsString},${m}`;
    }
  }
  return `${quotedMetricName ? '' : metricName}{${matchersAsString}}`;
}

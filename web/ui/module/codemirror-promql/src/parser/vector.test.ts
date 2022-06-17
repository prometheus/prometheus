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

import { buildVectorMatching } from './vector';
import { createEditorState } from '../test/utils-test';
import { walkThrough } from './path-finder';
import { BinaryExpr, Expr } from '@prometheus-io/lezer-promql';
import { syntaxTree } from '@codemirror/language';
import { VectorMatchCardinality } from '../types';

describe('buildVectorMatching test', () => {
  const testCases = [
    {
      binaryExpr: 'foo * bar',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [] },
    },
    {
      binaryExpr: 'foo * sum',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [] },
    },
    {
      binaryExpr: 'foo == 1',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [] },
    },
    {
      binaryExpr: 'foo == bool 1',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [] },
    },
    {
      binaryExpr: '2.5 / bar',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [] },
    },
    {
      binaryExpr: 'foo and bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
      },
    },
    {
      binaryExpr: 'foo or bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
      },
    },
    {
      binaryExpr: 'foo unless bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
      },
    },
    {
      // Test and/or precedence and reassigning of operands.
      // Here it will test only the first VectorMatching so (a + b) or (c and d) ==> ManyToMany
      binaryExpr: 'foo + bar or bla and blub',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
      },
    },
    {
      // Test and/or/unless precedence.
      // Here it will test only the first VectorMatching so ((a and b) unless c) or d ==> ManyToMany
      binaryExpr: 'foo and bar unless baz or qux',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
      },
    },
    {
      binaryExpr: 'foo * on(test,blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: [],
      },
    },
    {
      binaryExpr: 'foo * on(test,blub) group_left bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: [],
      },
    },
    {
      binaryExpr: 'foo and on(test,blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: [],
      },
    },
    {
      binaryExpr: 'foo and on() bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: true,
        include: [],
      },
    },
    {
      binaryExpr: 'foo and ignoring(test,blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: [],
      },
    },
    {
      binaryExpr: 'foo and ignoring() bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
      },
    },
    {
      binaryExpr: 'foo unless on(bar) baz',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: ['bar'],
        on: true,
        include: [],
      },
    },
    {
      binaryExpr: 'foo / on(test,blub) group_left(bar) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: ['bar'],
      },
    },
    {
      binaryExpr: 'foo / ignoring(test,blub) group_left(blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: ['blub'],
      },
    },
    {
      binaryExpr: 'foo / ignoring(test,blub) group_left(bar) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: ['bar'],
      },
    },
    {
      binaryExpr: 'foo - on(test,blub) group_right(bar,foo) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToMany,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: ['bar', 'foo'],
      },
    },
    {
      binaryExpr: 'foo - ignoring(test,blub) group_right(bar,foo) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToMany,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: ['bar', 'foo'],
      },
    },
  ];
  testCases.forEach((value) => {
    it(value.binaryExpr, () => {
      const state = createEditorState(value.binaryExpr);
      const node = walkThrough(syntaxTree(state).topNode, Expr, BinaryExpr);
      expect(node).toBeTruthy();
      if (node) {
        expect(value.expectedVectorMatching).toEqual(buildVectorMatching(state, node));
      }
    });
  });
});

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
import { BinaryExpr } from '@prometheus-io/lezer-promql';
import { syntaxTree } from '@codemirror/language';
import { VectorMatchCardinality, VectorMatching } from '../types';

const noFill = { fill: { lhs: null, rhs: null } };

describe('buildVectorMatching test', () => {
  const testCases: { binaryExpr: string; expectedVectorMatching: VectorMatching }[] = [
    {
      binaryExpr: 'foo * bar',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [], ...noFill },
    },
    {
      binaryExpr: 'foo * sum',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [], ...noFill },
    },
    {
      binaryExpr: 'foo == 1',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [], ...noFill },
    },
    {
      binaryExpr: 'foo == bool 1',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [], ...noFill },
    },
    {
      binaryExpr: '2.5 / bar',
      expectedVectorMatching: { card: VectorMatchCardinality.CardOneToOne, matchingLabels: [], on: false, include: [], ...noFill },
    },
    {
      binaryExpr: 'foo and bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo or bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo unless bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
        ...noFill,
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
        ...noFill,
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
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo * on(test,blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo * on(test,blub) group_left bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo and on(test,blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo and on() bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: true,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo and ignoring(test,blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo and ignoring() bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: [],
        on: false,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo unless on(bar) baz',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToMany,
        matchingLabels: ['bar'],
        on: true,
        include: [],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo / on(test,blub) group_left(bar) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: ['bar'],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo / ignoring(test,blub) group_left(blub) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: ['blub'],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo / ignoring(test,blub) group_left(bar) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardManyToOne,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: ['bar'],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo - on(test,blub) group_right(bar,foo) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToMany,
        matchingLabels: ['test', 'blub'],
        on: true,
        include: ['bar', 'foo'],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo - ignoring(test,blub) group_right(bar,foo) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToMany,
        matchingLabels: ['test', 'blub'],
        on: false,
        include: ['bar', 'foo'],
        ...noFill,
      },
    },
    {
      binaryExpr: 'foo + fill(23) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: [],
        on: false,
        include: [],
        fill: { lhs: 23, rhs: 23 },
      },
    },
    {
      binaryExpr: 'foo + fill_left(23) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: [],
        on: false,
        include: [],
        fill: { lhs: 23, rhs: null },
      },
    },
    {
      binaryExpr: 'foo + fill_right(23) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: [],
        on: false,
        include: [],
        fill: { lhs: null, rhs: 23 },
      },
    },
    {
      binaryExpr: 'foo + fill_left(23) fill_right(42) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: [],
        on: false,
        include: [],
        fill: { lhs: 23, rhs: 42 },
      },
    },
    {
      binaryExpr: 'foo + fill_right(23) fill_left(42) bar',
      expectedVectorMatching: {
        card: VectorMatchCardinality.CardOneToOne,
        matchingLabels: [],
        on: false,
        include: [],
        fill: { lhs: 42, rhs: 23 },
      },
    },
  ];
  testCases.forEach((value) => {
    it(value.binaryExpr, () => {
      const state = createEditorState(value.binaryExpr);
      const node = syntaxTree(state).topNode.getChild(BinaryExpr);
      expect(node).toBeTruthy();
      if (node) {
        expect(buildVectorMatching(state, node)).toEqual(value.expectedVectorMatching);
      }
    });
  });
});

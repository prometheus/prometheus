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

import chai from 'chai';
import {
  Add,
  AggregateExpr,
  BinaryExpr,
  Div,
  Eql,
  Expr,
  FunctionCall,
  FunctionCallArgs,
  FunctionCallBody,
  Gte,
  Gtr,
  Lss,
  Lte,
  Mod,
  Mul,
  Neq,
  NumberLiteral,
  Sub,
  VectorSelector,
} from '../grammar/parser.terms';
import { createEditorState } from '../test/utils.test';
import { containsAtLeastOneChild, containsChild, retrieveAllRecursiveNodes, walkBackward, walkThrough } from './path-finder';
import { SyntaxNode } from '@lezer/common';
import { syntaxTree } from '@codemirror/language';

describe('walkThrough test', () => {
  const testCases = [
    {
      title: 'should return the node when no path is given',
      expr: '1 > bool 2',
      pos: 0,
      expectedNode: 'PromQL',
      path: [] as number[],
      expectedDoc: '1 > bool 2',
    },
    {
      title: 'should find the path',
      expr: "100 * (1 - avg by(instance)(irate(node_cpu{mode='idle'}[5m])))",
      pos: 11,
      path: [Expr, NumberLiteral],
      // for the moment the function walkThrough is not able to find the following path.
      // That's because the function is iterating through the tree by searching the first possible node that matched
      // the node ID path[i].
      // So for the current expression, and the given position we are in the sub expr (1 - avg ...).
      // Expr is matching 1 and not avg.
      // TODO fix this issue
      // path: [Expr, AggregateExpr, AggregateOp, Avg],
      expectedNode: NumberLiteral,
      expectedDoc: '1',
    },
    {
      title: 'should not find the path',
      expr: 'topk(10, count by (job)({__name__=~".+"}))',
      pos: 12,
      path: [Expr, BinaryExpr],
      expectedNode: undefined,
      expectedDoc: undefined,
    },
    {
      title: 'should find a node in a recursive node definition',
      expr: 'rate(1, 2, 3)',
      pos: 0,
      path: [Expr, FunctionCall, FunctionCallBody, FunctionCallArgs, FunctionCallArgs, Expr, NumberLiteral],
      expectedNode: NumberLiteral,
      expectedDoc: '2',
    },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const subTree = syntaxTree(state).resolve(value.pos, -1);
      const node = walkThrough(subTree, ...value.path);
      if (typeof value.expectedNode === 'number') {
        chai.expect(value.expectedNode).to.equal(node?.type.id);
      } else {
        chai.expect(value.expectedNode).to.equal(node?.type.name);
      }
      if (node) {
        chai.expect(value.expectedDoc).to.equal(state.sliceDoc(node.from, node.to));
      }
    });
  });
});

describe('containsAtLeastOneChild test', () => {
  const testCases = [
    {
      title: 'should not find a node if none is defined',
      expr: '1 > 2',
      pos: 3,
      expectedResult: false,
      walkThrough: [],
      child: [],
    },
    {
      title: 'should find a node in the given list',
      expr: '1 > 2',
      pos: 0,
      walkThrough: [Expr, BinaryExpr],
      child: [Eql, Neq, Lte, Lss, Gte, Gtr],
      expectedResult: true,
    },
    {
      title: 'should not find a node in the given list',
      expr: '1 > 2',
      pos: 0,
      walkThrough: [Expr, BinaryExpr],
      child: [Mul, Div, Mod, Add, Sub],
      expectedResult: false,
    },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const subTree = syntaxTree(state).resolve(value.pos, -1);
      const node = walkThrough(subTree, ...value.walkThrough);
      chai.expect(node).to.not.null;
      if (node) {
        chai.expect(value.expectedResult).to.equal(containsAtLeastOneChild(node, ...value.child));
      }
    });
  });
});

describe('containsChild test', () => {
  const testCases = [
    {
      title: 'Should find all expr in a subtree',
      expr: 'metric_name / ignor',
      pos: 0,
      expectedResult: true,
      walkThrough: [Expr, BinaryExpr],
      child: [Expr, Expr],
    },
    {
      title: 'Should not find all child required',
      expr: 'sum(ra)',
      pos: 0,
      expectedResult: false,
      walkThrough: [Expr, AggregateExpr, FunctionCallBody, FunctionCallArgs],
      child: [Expr, Expr],
    },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const subTree = syntaxTree(state).resolve(value.pos, -1);
      const node: SyntaxNode | null = walkThrough(subTree, ...value.walkThrough);

      chai.expect(node).to.not.null;
      if (node) {
        chai.expect(value.expectedResult).to.equal(containsChild(node, ...value.child));
      }
    });
  });
});

describe('retrieveAllRecursiveNodes test', () => {
  it('should find every occurrence', () => {
    const state = createEditorState('rate(1,2,3)');
    const tree = syntaxTree(state).topNode.firstChild;
    chai.expect(tree).to.not.null;
    if (tree) {
      chai.expect(3).to.equal(retrieveAllRecursiveNodes(walkThrough(tree, FunctionCall, FunctionCallBody), FunctionCallArgs, Expr).length);
    }
  });
});

describe('walkbackward test', () => {
  const testCases = [
    {
      title: 'should find the parent',
      expr: 'metric_name{}',
      pos: 12,
      exit: VectorSelector,
      expectedResult: VectorSelector,
    },
  ];
  testCases.forEach((value) => {
    it(value.title, () => {
      const state = createEditorState(value.expr);
      const tree = syntaxTree(state).resolve(value.pos, -1);
      chai.expect(value.expectedResult).to.equal(walkBackward(tree, value.exit)?.type.id);
    });
  });
});

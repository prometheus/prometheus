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

import { Diagnostic } from '@codemirror/lint';
import { SyntaxNode, Tree } from '@lezer/common';
import {
  AggregateExpr,
  And,
  BinaryExpr,
  BinModifiers,
  Bool,
  Bottomk,
  CountValues,
  Eql,
  EqlSingle,
  Expr,
  FunctionCall,
  FunctionCallArgs,
  FunctionCallBody,
  Gte,
  Gtr,
  Identifier,
  LabelMatcher,
  LabelMatchers,
  LabelMatchList,
  Lss,
  Lte,
  MatrixSelector,
  MetricIdentifier,
  Neq,
  Or,
  ParenExpr,
  Quantile,
  StepInvariantExpr,
  SubqueryExpr,
  Topk,
  UnaryExpr,
  Unless,
  VectorSelector,
} from '@prometheus-io/lezer-promql';
import { containsAtLeastOneChild, retrieveAllRecursiveNodes, walkThrough } from './path-finder';
import { getType } from './type';
import { buildLabelMatchers } from './matcher';
import { EditorState } from '@codemirror/state';
import { syntaxTree } from '@codemirror/language';
import { getFunction, Matcher, VectorMatchCardinality, ValueType } from '../types';
import { buildVectorMatching } from './vector';

export class Parser {
  private readonly tree: Tree;
  private readonly state: EditorState;
  private readonly diagnostics: Diagnostic[];

  constructor(state: EditorState) {
    this.tree = syntaxTree(state);
    this.state = state;
    this.diagnostics = [];
  }

  getDiagnostics(): Diagnostic[] {
    return this.diagnostics.sort((a, b) => {
      return a.from - b.from;
    });
  }

  analyze(): void {
    // when you are at the root of the tree, the first node is not `Expr` but a node with no name.
    // So to be able to iterate other the node relative to the promql node, we have to get the first child at the beginning
    this.checkAST(this.tree.topNode.firstChild);
    this.diagnoseAllErrorNodes();
  }

  private diagnoseAllErrorNodes() {
    const cursor = this.tree.cursor();
    while (cursor.next()) {
      // usually there is an error node at the end of the expression when user is typing
      // so it's not really a useful information to say the expression is wrong.
      // Hopefully if there is an error node at the end of the tree, checkAST should yell more precisely
      if (cursor.type.id === 0 && cursor.to !== this.tree.topNode.to) {
        const node = cursor.node.parent;
        this.diagnostics.push({
          severity: 'error',
          message: 'unexpected expression',
          from: node ? node.from : cursor.from,
          to: node ? node.to : cursor.to,
        });
      }
    }
  }

  // checkAST is inspired of the same named method from prometheus/prometheus:
  // https://github.com/prometheus/prometheus/blob/3470ee1fbf9d424784eb2613bab5ab0f14b4d222/promql/parser/parse.go#L433
  checkAST(node: SyntaxNode | null): ValueType {
    if (!node) {
      return ValueType.none;
    }
    switch (node.type.id) {
      case Expr:
        return this.checkAST(node.firstChild);
      case AggregateExpr:
        this.checkAggregationExpr(node);
        break;
      case BinaryExpr:
        this.checkBinaryExpr(node);
        break;
      case FunctionCall:
        this.checkCallFunction(node);
        break;
      case ParenExpr:
        this.checkAST(walkThrough(node, Expr));
        break;
      case UnaryExpr:
        const unaryExprType = this.checkAST(walkThrough(node, Expr));
        if (unaryExprType !== ValueType.scalar && unaryExprType !== ValueType.vector) {
          this.addDiagnostic(node, `unary expression only allowed on expressions of type scalar or instant vector, got ${unaryExprType}`);
        }
        break;
      case SubqueryExpr:
        const subQueryExprType = this.checkAST(walkThrough(node, Expr));
        if (subQueryExprType !== ValueType.vector) {
          this.addDiagnostic(node, `subquery is only allowed on instant vector, got ${subQueryExprType} in ${node.name} instead`);
        }
        break;
      case MatrixSelector:
        this.checkAST(walkThrough(node, Expr));
        break;
      case VectorSelector:
        this.checkVectorSelector(node);
        break;
      case StepInvariantExpr:
        const exprValue = this.checkAST(walkThrough(node, Expr));
        if (exprValue !== ValueType.vector && exprValue !== ValueType.matrix) {
          this.addDiagnostic(node, `@ modifier must be preceded by an instant selector vector or range vector selector or a subquery`);
        }
        // if you are looking at the Prometheus code, you will likely find that some checks are missing here.
        // Specially the one checking if the timestamp after the `@` is ok: https://github.com/prometheus/prometheus/blob/ad5ed416ba635834370bfa06139258b31f8c33f9/promql/parser/parse.go#L722-L725
        // Since Javascript is managing the number as a float64 and so on 53 bits, we cannot validate that the maxInt64 number is a valid value.
        // So, to manage properly this issue, we would need to use the BigInt which is possible or by using ES2020.BigInt, or by using the lib: https://github.com/GoogleChromeLabs/jsbi.
        //   * Introducing a lib just for theses checks is quite overkilled
        //   * Using ES2020 would be the way to go. Unfortunately moving to ES2020 is breaking the build of the lib.
        //     So far I didn't find the way to fix it. I think it's likely due to the fact we are building an ESM package which is now something stable in nodeJS/javascript but still experimental in typescript.
        // For the above reason, we decided to drop these checks.
        break;
    }

    return getType(node);
  }

  private checkAggregationExpr(node: SyntaxNode): void {
    // according to https://github.com/promlabs/lezer-promql/blob/master/src/promql.grammar#L26
    // the name of the aggregator function is stored in the first child
    const aggregateOp = node.firstChild?.firstChild;
    if (!aggregateOp) {
      this.addDiagnostic(node, 'aggregation operator expected in aggregation expression but got nothing');
      return;
    }
    const expr = walkThrough(node, FunctionCallBody, FunctionCallArgs, Expr);
    if (!expr) {
      this.addDiagnostic(node, 'unable to find the parameter for the expression');
      return;
    }
    this.expectType(expr, ValueType.vector, 'aggregation expression');
    // get the parameter of the aggregation operator
    const params = walkThrough(node, FunctionCallBody, FunctionCallArgs, FunctionCallArgs, Expr);
    if (aggregateOp.type.id === Topk || aggregateOp.type.id === Bottomk || aggregateOp.type.id === Quantile) {
      if (!params) {
        this.addDiagnostic(node, 'no parameter found');
        return;
      }
      this.expectType(params, ValueType.scalar, 'aggregation parameter');
    }
    if (aggregateOp.type.id === CountValues) {
      if (!params) {
        this.addDiagnostic(node, 'no parameter found');
        return;
      }
      this.expectType(params, ValueType.string, 'aggregation parameter');
    }
  }

  private checkBinaryExpr(node: SyntaxNode): void {
    // Following the definition of the BinaryExpr, the left and the right
    // expression are respectively the first and last child
    // https://github.com/promlabs/lezer-promql/blob/master/src/promql.grammar#L52
    const lExpr = node.firstChild;
    const rExpr = node.lastChild;
    if (!lExpr || !rExpr) {
      this.addDiagnostic(node, 'left or right expression is missing in binary expression');
      return;
    }
    const lt = this.checkAST(lExpr);
    const rt = this.checkAST(rExpr);
    const boolModifierUsed = walkThrough(node, BinModifiers, Bool);
    const isComparisonOperator = containsAtLeastOneChild(node, Eql, Neq, Lte, Lss, Gte, Gtr);
    const isSetOperator = containsAtLeastOneChild(node, And, Or, Unless);

    // BOOL modifier check
    if (boolModifierUsed) {
      if (!isComparisonOperator) {
        this.addDiagnostic(node, 'bool modifier can only be used on comparison operators');
      }
    } else {
      if (isComparisonOperator && lt === ValueType.scalar && rt === ValueType.scalar) {
        this.addDiagnostic(node, 'comparisons between scalars must use BOOL modifier');
      }
    }

    const vectorMatching = buildVectorMatching(this.state, node);
    if (vectorMatching !== null && vectorMatching.on) {
      for (const l1 of vectorMatching.matchingLabels) {
        for (const l2 of vectorMatching.include) {
          if (l1 === l2) {
            this.addDiagnostic(node, `label "${l1}" must not occur in ON and GROUP clause at once`);
          }
        }
      }
    }

    if (lt !== ValueType.scalar && lt !== ValueType.vector) {
      this.addDiagnostic(lExpr, 'binary expression must contain only scalar and instant vector types');
    }
    if (rt !== ValueType.scalar && rt !== ValueType.vector) {
      this.addDiagnostic(rExpr, 'binary expression must contain only scalar and instant vector types');
    }

    if ((lt !== ValueType.vector || rt !== ValueType.vector) && vectorMatching !== null) {
      if (vectorMatching.matchingLabels.length > 0) {
        this.addDiagnostic(node, 'vector matching only allowed between instant vectors');
      }
    } else {
      if (isSetOperator) {
        if (vectorMatching?.card === VectorMatchCardinality.CardOneToMany || vectorMatching?.card === VectorMatchCardinality.CardManyToOne) {
          this.addDiagnostic(node, 'no grouping allowed for set operations');
        }
        if (vectorMatching?.card !== VectorMatchCardinality.CardManyToMany) {
          this.addDiagnostic(node, 'set operations must always be many-to-many');
        }
      }
    }
    if ((lt === ValueType.scalar || rt === ValueType.scalar) && isSetOperator) {
      this.addDiagnostic(node, 'set operator not allowed in binary scalar expression');
    }
  }

  private checkCallFunction(node: SyntaxNode): void {
    const funcID = node.firstChild?.firstChild;
    if (!funcID) {
      this.addDiagnostic(node, 'function not defined');
      return;
    }

    const args = retrieveAllRecursiveNodes(walkThrough(node, FunctionCallBody), FunctionCallArgs, Expr);
    const funcSignature = getFunction(funcID.type.id);
    const nargs = funcSignature.argTypes.length;

    if (funcSignature.variadic === 0) {
      if (args.length !== nargs) {
        this.addDiagnostic(node, `expected ${nargs} argument(s) in call to "${funcSignature.name}", got ${args.length}`);
      }
    } else {
      const na = nargs - 1;
      if (na > args.length) {
        this.addDiagnostic(node, `expected at least ${na} argument(s) in call to "${funcSignature.name}", got ${args.length}`);
      } else {
        const nargsmax = na + funcSignature.variadic;
        if (funcSignature.variadic > 0 && nargsmax < args.length) {
          this.addDiagnostic(node, `expected at most ${nargsmax} argument(s) in call to "${funcSignature.name}", got ${args.length}`);
        }
      }
    }

    let j = 0;
    for (let i = 0; i < args.length; i++) {
      j = i;
      if (j >= funcSignature.argTypes.length) {
        if (funcSignature.variadic === 0) {
          // This is not a vararg function so we should not check the
          // type of the extra arguments.
          break;
        }
        j = funcSignature.argTypes.length - 1;
      }
      this.expectType(args[i], funcSignature.argTypes[j], `call to function "${funcSignature.name}"`);
    }
  }

  private checkVectorSelector(node: SyntaxNode): void {
    const labelMatchers = buildLabelMatchers(
      retrieveAllRecursiveNodes(walkThrough(node, LabelMatchers, LabelMatchList), LabelMatchList, LabelMatcher),
      this.state
    );
    let vectorSelectorName = '';
    // VectorSelector ( MetricIdentifier ( Identifier ) )
    // https://github.com/promlabs/lezer-promql/blob/71e2f9fa5ae6f5c5547d5738966cd2512e6b99a8/src/promql.grammar#L200
    const vectorSelectorNodeName = walkThrough(node, MetricIdentifier, Identifier);
    if (vectorSelectorNodeName) {
      vectorSelectorName = this.state.sliceDoc(vectorSelectorNodeName.from, vectorSelectorNodeName.to);
    }
    if (vectorSelectorName !== '') {
      // In this case the last LabelMatcher is checking for the metric name
      // set outside the braces. This checks if the name has already been set
      // previously
      const labelMatcherMetricName = labelMatchers.find((lm) => lm.name === '__name__');
      if (labelMatcherMetricName) {
        this.addDiagnostic(node, `metric name must not be set twice: ${vectorSelectorName} or ${labelMatcherMetricName.value}`);
      }
      // adding the metric name as a Matcher to avoid a false positive for this kind of expression:
      // foo{bare=''}
      labelMatchers.push(new Matcher(EqlSingle, '__name__', vectorSelectorName));
    }

    // A Vector selector must contain at least one non-empty matcher to prevent
    // implicit selection of all metrics (e.g. by a typo).
    const empty = labelMatchers.every((lm) => lm.matchesEmpty());
    if (empty) {
      this.addDiagnostic(node, 'vector selector must contain at least one non-empty matcher');
    }
  }

  private expectType(node: SyntaxNode, want: ValueType, context: string): void {
    const t = this.checkAST(node);
    if (t !== want) {
      this.addDiagnostic(node, `expected type ${want} in ${context}, got ${t}`);
    }
  }

  private addDiagnostic(node: SyntaxNode, msg: string): void {
    this.diagnostics.push({
      severity: 'error',
      message: msg,
      from: node.from,
      to: node.to,
    });
  }
}

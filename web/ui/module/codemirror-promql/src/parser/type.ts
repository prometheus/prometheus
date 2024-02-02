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
  AggregateExpr,
  BinaryExpr,
  FunctionCall,
  MatrixSelector,
  NumberLiteral,
  OffsetExpr,
  ParenExpr,
  StepInvariantExpr,
  StringLiteral,
  SubqueryExpr,
  UnaryExpr,
  VectorSelector,
} from '@prometheus-io/lezer-promql';
import { getFunction, ValueType } from '../types';

// Based on https://github.com/prometheus/prometheus/blob/d668a7efe3107dbdcc67bf4e9f12430ed8e2b396/promql/parser/ast.go#L191
export function getType(node: SyntaxNode | null): ValueType {
  if (!node) {
    return ValueType.none;
  }
  switch (node.type.id) {
    case AggregateExpr:
      return ValueType.vector;
    case VectorSelector:
      return ValueType.vector;
    case OffsetExpr:
      return getType(node.firstChild);
    case StringLiteral:
      return ValueType.string;
    case NumberLiteral:
      return ValueType.scalar;
    case MatrixSelector:
      return ValueType.matrix;
    case SubqueryExpr:
      return ValueType.matrix;
    case ParenExpr:
      return getType(node.getChild('Expr'));
    case UnaryExpr:
      return getType(node.getChild('Expr'));
    case BinaryExpr:
      const lt = getType(node.firstChild);
      const rt = getType(node.lastChild);
      if (lt === ValueType.scalar && rt === ValueType.scalar) {
        return ValueType.scalar;
      }
      return ValueType.vector;
    case FunctionCall:
      const funcNode = node.firstChild?.firstChild;
      if (!funcNode) {
        return ValueType.none;
      }
      return getFunction(funcNode.type.id).returnType;
    case StepInvariantExpr:
      return getType(node.getChild('Expr'));
    default:
      return ValueType.none;
  }
}

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

import { EditorState } from '@codemirror/state';
import { SyntaxNode } from '@lezer/common';
import {
  And,
  BinaryExpr,
  MatchingModifierClause,
  LabelName,
  QuotedLabelName,
  GroupingLabels,
  GroupLeft,
  GroupRight,
  On,
  Or,
  Unless,
  NumberDurationLiteral,
  FillModifier,
  FillClause,
  FillLeftClause,
  FillRightClause,
} from '@prometheus-io/lezer-promql';
import { VectorMatchCardinality, VectorMatching } from '../types';
import { containsAtLeastOneChild } from './path-finder';
import { unquotePromQLString } from './utf8';

// Returns the label names of a GroupingLabels node, whether they are quoted or not.
function getLabelNames(state: EditorState, groupingLabels: SyntaxNode | null | undefined): string[] {
  const names: string[] = [];
  for (let child = groupingLabels?.firstChild ?? null; child; child = child.nextSibling) {
    if (child.type.id === LabelName) {
      names.push(state.sliceDoc(child.from, child.to));
    } else if (child.type.id === QuotedLabelName) {
      names.push(unquotePromQLString(state.sliceDoc(child.from, child.to)));
    }
  }
  return names;
}

export function buildVectorMatching(state: EditorState, binaryNode: SyntaxNode): VectorMatching | null {
  if (!binaryNode || binaryNode.type.id !== BinaryExpr) {
    return null;
  }
  const result: VectorMatching = {
    card: VectorMatchCardinality.CardOneToOne,
    matchingLabels: [],
    on: false,
    include: [],
    fill: {
      lhs: null,
      rhs: null,
    },
  };
  const modifierClause = binaryNode.getChild(MatchingModifierClause);
  if (modifierClause) {
    result.on = modifierClause.getChild(On) !== null;
    result.matchingLabels.push(...getLabelNames(state, modifierClause.getChild(GroupingLabels)));

    const groupLeft = modifierClause.getChild(GroupLeft);
    const groupRight = modifierClause.getChild(GroupRight);
    const group = groupLeft || groupRight;
    if (group) {
      result.card = groupLeft ? VectorMatchCardinality.CardManyToOne : VectorMatchCardinality.CardOneToMany;
      result.include.push(...getLabelNames(state, group.nextSibling));
    }
  }

  const fillModifier = binaryNode.getChild(FillModifier);
  if (fillModifier) {
    const fill = fillModifier.getChild(FillClause);
    const fillLeft = fillModifier.getChild(FillLeftClause);
    const fillRight = fillModifier.getChild(FillRightClause);

    const getFillValue = (node: SyntaxNode) => {
      const valueNode = node.getChild(NumberDurationLiteral);
      return valueNode ? parseFloat(state.sliceDoc(valueNode.from, valueNode.to)) : null;
    };

    if (fill) {
      const value = getFillValue(fill);
      result.fill.lhs = value;
      result.fill.rhs = value;
    }

    if (fillLeft) {
      result.fill.lhs = getFillValue(fillLeft);
    }

    if (fillRight) {
      result.fill.rhs = getFillValue(fillRight);
    }
  }

  const isSetOperator = containsAtLeastOneChild(binaryNode, And, Or, Unless);
  if (isSetOperator && result.card === VectorMatchCardinality.CardOneToOne) {
    result.card = VectorMatchCardinality.CardManyToMany;
  }
  return result;
}

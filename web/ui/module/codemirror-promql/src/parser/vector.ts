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
  GroupingLabels,
  GroupLeft,
  GroupRight,
  On,
  Or,
  Unless,
} from '@prometheus-io/lezer-promql';
import { VectorMatchCardinality, VectorMatching } from '../types';
import { containsAtLeastOneChild } from './path-finder';

export function buildVectorMatching(state: EditorState, binaryNode: SyntaxNode): VectorMatching | null {
  if (!binaryNode || binaryNode.type.id !== BinaryExpr) {
    return null;
  }
  const result: VectorMatching = {
    card: VectorMatchCardinality.CardOneToOne,
    matchingLabels: [],
    on: false,
    include: [],
  };
  const modifierClause = binaryNode.getChild(MatchingModifierClause);
  if (modifierClause) {
    result.on = modifierClause.getChild(On) !== null;
    const labelNode = modifierClause.getChild(GroupingLabels);
    const labels = labelNode ? labelNode.getChildren(LabelName) : [];
    for (const label of labels) {
      result.matchingLabels.push(state.sliceDoc(label.from, label.to));
    }

    const groupLeft = modifierClause.getChild(GroupLeft);
    const groupRight = modifierClause.getChild(GroupRight);
    const group = groupLeft || groupRight;
    if (group) {
      result.card = groupLeft ? VectorMatchCardinality.CardManyToOne : VectorMatchCardinality.CardOneToMany;
      const labelNode = group.nextSibling;
      const labels = labelNode?.getChildren(LabelName) || [];
      for (const label of labels) {
        result.include.push(state.sliceDoc(label.from, label.to));
      }
    }
  }

  const isSetOperator = containsAtLeastOneChild(binaryNode, And, Or, Unless);
  if (isSetOperator && result.card === VectorMatchCardinality.CardOneToOne) {
    result.card = VectorMatchCardinality.CardManyToMany;
  }
  return result;
}

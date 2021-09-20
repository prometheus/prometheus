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
  BinModifiers,
  GroupingLabel,
  GroupingLabelList,
  GroupingLabels,
  GroupLeft,
  GroupRight,
  On,
  OnOrIgnoring,
  Or,
  Unless,
} from '../grammar/parser.terms';
import { VectorMatchCardinality, VectorMatching } from '../types';
import { containsAtLeastOneChild, retrieveAllRecursiveNodes } from './path-finder';

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
  const binModifiers = binaryNode.getChild(BinModifiers);
  if (binModifiers) {
    const onOrIgnoring = binModifiers.getChild(OnOrIgnoring);
    if (onOrIgnoring) {
      result.on = onOrIgnoring.getChild(On) !== null;
      const labels = retrieveAllRecursiveNodes(onOrIgnoring.getChild(GroupingLabels), GroupingLabelList, GroupingLabel);
      if (labels.length > 0) {
        for (const label of labels) {
          result.matchingLabels.push(state.sliceDoc(label.from, label.to));
        }
      }
    }

    const groupLeft = binModifiers.getChild(GroupLeft);
    const groupRight = binModifiers.getChild(GroupRight);
    if (groupLeft || groupRight) {
      result.card = groupLeft ? VectorMatchCardinality.CardManyToOne : VectorMatchCardinality.CardOneToMany;
      const includeLabels = retrieveAllRecursiveNodes(binModifiers.getChild(GroupingLabels), GroupingLabelList, GroupingLabel);
      if (includeLabels.length > 0) {
        for (const label of includeLabels) {
          result.include.push(state.sliceDoc(label.from, label.to));
        }
      }
    }
  }

  const isSetOperator = containsAtLeastOneChild(binaryNode, And, Or, Unless);
  if (isSetOperator && result.card === VectorMatchCardinality.CardOneToOne) {
    result.card = VectorMatchCardinality.CardManyToMany;
  }
  return result;
}

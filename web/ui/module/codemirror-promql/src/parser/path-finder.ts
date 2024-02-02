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

// walkBackward will iterate other the tree from the leaf to the root until it founds the given `exit` node.
// It returns null if the exit is not found.
export function walkBackward(node: SyntaxNode | null, exit: number): SyntaxNode | null {
  for (;;) {
    if (!node || node.type.id === exit) {
      return node;
    }
    node = node.parent;
  }
  return null;
}

export function containsAtLeastOneChild(node: SyntaxNode, ...child: (number | string)[]): boolean {
  const cursor = node.cursor();
  if (!cursor.next()) {
    // let's try to move directly to the children level and
    // return false immediately if the current node doesn't have any child
    return false;
  }
  let result = false;
  do {
    result = child.some((n) => cursor.type.id === n || cursor.type.name === n);
  } while (!result && cursor.nextSibling());
  return result;
}

export function containsChild(node: SyntaxNode, ...child: (number | string)[]): boolean {
  const cursor = node.cursor();
  if (!cursor.next()) {
    // let's try to move directly to the children level and
    // return false immediately if the current node doesn't have any child
    return false;
  }
  let i = 0;

  do {
    if (cursor.type.is(child[i])) {
      i++;
    }
  } while (i < child.length && cursor.nextSibling());

  return i >= child.length;
}

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
export function walkBackward(node: SyntaxNode, exit: number): SyntaxNode | null {
  const cursor = node.cursor();
  let cursorIsMoving = true;
  while (cursorIsMoving && cursor.type.id !== exit) {
    cursorIsMoving = cursor.parent();
  }
  return cursor.type.id === exit ? cursor.node : null;
}

// walkThrough is going to follow the path passed in parameter.
// If it succeeds to reach the last id/name of the path, then it will return the corresponding Subtree.
// Otherwise if it's not possible to reach the last id/name of the path, it will return `null`
// Note: the way followed during the iteration of the tree to find the given path, is only from the root to the leaf.
export function walkThrough(node: SyntaxNode, ...path: (number | string)[]): SyntaxNode | null {
  const cursor = node.cursor();
  let i = 0;
  let cursorIsMoving = true;
  path.unshift(cursor.type.id);
  while (i < path.length && cursorIsMoving) {
    if (cursor.type.id === path[i] || cursor.type.name === path[i]) {
      i++;
      if (i < path.length) {
        cursorIsMoving = cursor.next();
      }
    } else {
      cursorIsMoving = cursor.nextSibling();
    }
  }
  if (i >= path.length) {
    return cursor.node;
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
    if (cursor.type.id === child[i] || cursor.type.name === child[i]) {
      i++;
    }
  } while (i < child.length && cursor.nextSibling());

  return i >= child.length;
}

export function retrieveAllRecursiveNodes(parentNode: SyntaxNode | null, recursiveNode: number, leaf: number): SyntaxNode[] {
  const nodes: SyntaxNode[] = [];

  function recursiveRetrieveNode(node: SyntaxNode | null, nodes: SyntaxNode[]) {
    const subNode = node?.getChild(recursiveNode);
    const le = node?.lastChild;
    if (subNode && subNode.type.id === recursiveNode) {
      recursiveRetrieveNode(subNode, nodes);
    }
    if (le && le.type.id === leaf) {
      nodes.push(le);
    }
  }

  recursiveRetrieveNode(parentNode, nodes);
  return nodes;
}

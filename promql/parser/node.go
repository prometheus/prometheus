// Copyright 2020 The Prometheus Authors
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

package parser

const (
	grouping props = iota
	scalars
	multiArguments
	aggregateParent
)

const (
	aggregateExpr exprs = iota
	binaryExpr
	matrixExpr
	parenExpr
	subQueryExpr
)

type (
	exprs uint8
	props uint8
)

type nodeInfo struct {
	head, currentNode Node
	// Node details.
	columnLimit int
	ancestors   []Node
	item        Item
	items       []Item
	buf         int
	baseIndent  int
}

func (n *nodeInfo) violatesColumnLimit() bool {
	var items []Item
	if n.currentNode == nil {
		panic("current node not set")
	}
	for _, item := range n.items {
		if n.currentNode.PositionRange().Contains(item.PositionRange()) && item.Typ != COMMENT {
			items = append(items, item)
		}
	}
	content := stringifyItems(items)
	return len(content) > n.columnLimit
}

// node returns the node corresponding to the given position range in the AST.
func (n *nodeInfo) node() Node {
	ancestors, node := n.nodeHistory(n.head, n.item.PositionRange(), []Node{})
	n.ancestors = reduceNonNewLineExprs(ancestors)
	n.baseIndent = len(n.ancestors)
	n.currentNode = node
	return node
}

func (n *nodeInfo) getBaseIndent(item Item) int {
	ancestors, _ := n.nodeHistory(n.head, item.PositionRange(), []Node{})
	ancestors = reduceNonNewLineExprs(ancestors)
	n.buf = len(ancestors)
	return n.buf
}

// isLastItem returns true if the item passed is the last item of the current node.
func (n *nodeInfo) isLastItem(item Item) bool {
	return n.currentNode.PositionRange().End == item.PositionRange().End
}

func (n *nodeInfo) previousIndent() int {
	return n.buf
}

// has verifies whether the current node contains a particular entity.
func (n *nodeInfo) has(element props) bool {
	switch element {
	case grouping:
		switch node := n.currentNode.(type) {
		case *BinaryExpr:
			if node.VectorMatching == nil {
				return node.ReturnBool
			}
			return len(node.VectorMatching.MatchingLabels) > 0 || node.ReturnBool || node.VectorMatching.On
		case *AggregateExpr:
			return len(node.Grouping) > 0
		}
	case scalars:
		if node, ok := n.currentNode.(*BinaryExpr); ok {
			if node.LHS.Type() == ValueTypeScalar || node.RHS.Type() == ValueTypeScalar {
				return true
			}
		}
	case multiArguments:
		if node, ok := n.currentNode.(*Call); ok {
			return len(node.Args) > 1
		}
	case aggregateParent:
		if len(n.ancestors) < 2 {
			return false
		}
		if _, ok := n.ancestors[len(n.ancestors)-2].(*AggregateExpr); ok {
			return true
		}
	}
	return false
}

// nodeHistory returns the ancestors of the node the item position range is passed of,
// along with the node in which the item is present. This is done with the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (n *nodeInfo) nodeHistory(head Node, posRange PositionRange, stack []Node) ([]Node, Node) {
	if head.PositionRange().Contains(posRange) {
		stack = append(stack, head)
	}
	for _, child := range Children(head) {
		if child.PositionRange().Contains(posRange) {
			return n.nodeHistory(child, posRange, stack)
		}
	}
	return stack, head
}

func (n *nodeInfo) parent() Node {
	stack, _ := n.nodeHistory(n.head, n.item.PositionRange(), []Node{})
	if len(stack) < 2 {
		return nil
	}
	return stack[len(stack)-2]
}

// childIs confirms the type of the child of current node.
func (n *nodeInfo) childIsBinary() bool {
	for _, child := range Children(n.currentNode) {
		if _, ok := child.(*BinaryExpr); ok {
			return true
		}
	}
	return false
}

func isChildOfTypeBinary(node Node) bool {
	info := &nodeInfo{currentNode: node}
	return info.childIsBinary()
}

// reduceNonNewLineExprs reduces those expressions from the history that are not
// necessary for base indent. Base indent is applied on those items that have the
// tendency to fall on a new line. However, items like Left_Bracket, Right_Bracket
// never fall on new line. This means that when they (node in which they are present)
// will be encountered, the stack already has MatrixSelector or SubQuery in it, but
// their very existence harms the base indent. This is because they never contribute
// to the indent, meaning the (metric_name[5m]) will get base indent as 3 (ParenExpr,
// MatrixSelector, VectorSelector) but it should actually be 2 according to the requirements.
// Hence, these unwanted expressions that do not contribute to the base indent
// should be reduced.
func reduceNonNewLineExprs(history []Node) []Node {
	var temp []Node
	if !(len(history) > 1) {
		return history
	}
	for i := range history {
		if !(isNodeType(history[i], matrixExpr) || isNodeType(history[i], subQueryExpr)) {
			temp = append(temp, history[i])
		}
	}
	return reduceBinary(temp)
}

// reduceBinary reduces from end, the continuous occurrence
// of binary expression to its single representation.
func reduceBinary(history []Node) []Node {
	var temp []Node
	if !(len(history) > 1) {
		return history
	}
	for i := 0; i < len(history)-1; i++ {
		if !(isNodeType(history[i], binaryExpr) && isNodeType(history[i+1], binaryExpr)) {
			temp = append(temp, history[i])
		}
	}
	temp = append(temp, history[len(history)-1])
	return temp
}

// containsIgnoring is used to check if the node contains IGNORING as item.
// The current AST structure do not carry any information for cases like `... ignoring() ...`.
// Hence, this is achieved by scanning individual lex items in that node.
func containsIgnoring(nodeStr string) bool {
	for _, item := range LexItems(nodeStr) {
		if item.Typ == IGNORING {
			return true
		}
	}
	return false
}

func isNodeType(node Node, typs ...exprs) bool {
	for _, typ := range typs {
		switch typ {
		case aggregateExpr:
			if _, ok := node.(*AggregateExpr); ok {
				return true
			}
		case binaryExpr:
			if _, ok := node.(*BinaryExpr); ok {
				return true
			}
		case matrixExpr:
			if _, ok := node.(*MatrixSelector); ok {
				return true
			}
		case parenExpr:
			if _, ok := node.(*ParenExpr); ok {
				return true
			}
		case subQueryExpr:
			if _, ok := node.(*SubqueryExpr); ok {
				return true
			}
		}
	}
	return false
}

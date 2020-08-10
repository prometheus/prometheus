package prettier

import (
	"fmt"
	"github.com/prometheus/prometheus/promql/parser"
)

const (
	grouping = iota
	scalars
	multiArguments
)

type nodeInfo struct {
	head, currentNode parser.Node
	// Node details.
	columnLimit int
	ancestors   []parser.Node
	item        parser.Item
	buf         int
	baseIndent  int
	exprType    string
}

func (n *nodeInfo) violatesColumnLimit() bool {
	if n.currentNode == nil {
		panic("current node not set")
	}
	return len(n.currentNode.String()) > n.columnLimit
}

// node returns the node corresponding to the given position range in the AST.
func (n *nodeInfo) node() parser.Node {
	ancestors, node := n.nodeHistory(n.head, n.item.PositionRange(), []parser.Node{})
	n.ancestors = reduceNonNewLineExprs(ancestors)
	n.baseIndent = len(n.ancestors)
	n.currentNode = node
	n.exprType = fmt.Sprintf("%T", node)
	return node
}

func (n *nodeInfo) getBaseIndent(item parser.Item) int {
	ancestors, _ := n.nodeHistory(n.head, item.PositionRange(), []parser.Node{})
	ancestors = reduceNonNewLineExprs(ancestors)
	n.buf = len(ancestors)
	return n.buf
}

func (n *nodeInfo) previousIndent() int {
	return n.buf
}

// has verifies whether the current node contains a particular entity.
func (n *nodeInfo) has(element uint) bool {
	switch element {
	case grouping:
		switch node := n.currentNode.(type) {
		case *parser.BinaryExpr:
			if node.VectorMatching == nil {
				return node.ReturnBool
			}
			return len(node.VectorMatching.MatchingLabels) > 0 || node.ReturnBool || node.VectorMatching.On
		case *parser.AggregateExpr:
			return len(node.Grouping) > 0
		}
	case scalars:
		if node, ok := n.currentNode.(*parser.BinaryExpr); ok {
			if node.LHS.Type() == parser.ValueTypeScalar || node.RHS.Type() == parser.ValueTypeScalar {
				return true
			}
		}
	case multiArguments:
		if node, ok := n.currentNode.(*parser.Call); ok {
			return len(node.Args) > 1
		}
	}
	return false
}

// parentNode returns the parent node of the given node/item position range.
func (n *nodeInfo) parentNode(head parser.Expr, rnge parser.PositionRange) parser.Node {
	ancestors, _ := n.nodeHistory(head, rnge, []parser.Node{})
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// nodeHistory returns the ancestors of the node the item position range is passed of,
// along with the node in which the item is present. This is done with the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (n *nodeInfo) nodeHistory(head parser.Node, posRange parser.PositionRange, stack []parser.Node) ([]parser.Node, parser.Node) {
	if head.PositionRange().Contains(posRange) {
		stack = append(stack, head)
	}
	for _, child := range parser.Children(head) {
		if child.PositionRange().Contains(posRange) {
			return n.nodeHistory(child, posRange, stack)
		}
	}
	return stack, head
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
func reduceNonNewLineExprs(history []parser.Node) []parser.Node {
	var temp []parser.Node
	if !(len(history) > 1) {
		return history
	}
	for i := range history {
		if !(isMatrixSelector(history[i]) || isSubQuery(history[i])) {
			temp = append(temp, history[i])
		}
	}
	return reduceBinary(temp)
}

// reduceBinary reduces from end, the continuous occurrence
// of binary expression to its single representation.
func reduceBinary(history []parser.Node) []parser.Node {
	var temp []parser.Node
	if !(len(history) > 1) {
		return history
	}
	for i := 0; i < len(history)-1; i++ {
		if !(isBinary(history[i]) && isBinary(history[i+1])) {
			temp = append(temp, history[i])
		}
	}
	temp = append(temp, history[len(history)-1])
	return temp
}

func isBinary(node parser.Node) bool {
	if _, ok := node.(*parser.BinaryExpr); ok {
		return true
	}
	return false
}

func isMatrixSelector(node parser.Node) bool {
	if _, ok := node.(*parser.MatrixSelector); ok {
		return true
	}
	return false
}
func isSubQuery(node parser.Node) bool {
	if _, ok := node.(*parser.SubqueryExpr); ok {
		return true
	}
	return false
}

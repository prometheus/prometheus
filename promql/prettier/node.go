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
	head, currentNode parser.Expr
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
func (n *nodeInfo) node() parser.Expr {
	ancestors, node, _ := n.nodeHistory(n.head, n.item.PositionRange(), []parser.Node{})
	n.ancestors = reduceNonNewLineExprs(ancestors)
	n.baseIndent = len(n.ancestors)
	n.currentNode = node
	n.exprType = fmt.Sprintf("%T", node)
	return node
}

func (n *nodeInfo) getBaseIndent(item parser.Item) int {
	ancestors, _, _ := n.nodeHistory(n.head, item.PositionRange(), []parser.Node{})
	ancestors = reduceNonNewLineExprs(ancestors)
	n.buf = len(ancestors)
	return n.buf
}

func (n *nodeInfo) previousIndent() int {
	return n.buf
}

// contains verifies whether the current node contains a particular entity.
func (n *nodeInfo) is(element uint) bool {
	switch element {
	case grouping:
		if n, ok := n.currentNode.(*parser.BinaryExpr); ok {
			if n.VectorMatching == nil {
				return n.ReturnBool
			}
			return len(n.VectorMatching.MatchingLabels) > 0 || n.ReturnBool
		}
	case scalars:
		if n, ok := n.currentNode.(*parser.BinaryExpr); ok {
			if n.LHS.Type() == parser.ValueTypeScalar || n.RHS.Type() == parser.ValueTypeScalar {
				return true
			}
		}
	case multiArguments:
		if n, ok := n.currentNode.(*parser.Call); ok {
			return len(n.Args) > 1
		}
	}
	return false
}

// parentNode returns the parent node of the given node/item position range.
func (n *nodeInfo) parentNode(head parser.Expr, rnge parser.PositionRange) parser.Node {
	ancestors, _, found := n.nodeHistory(head, rnge, []parser.Node{})
	if !found {
		return nil
	}
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// nodeHistory returns the ancestors of the node the item position range is passed of,
// along with the node in which the item is present. This is done with the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (n *nodeInfo) nodeHistory(head parser.Expr, posRange parser.PositionRange, stack []parser.Node) ([]parser.Node, parser.Expr, bool) {
	var nodeMatch bool
	switch node := head.(type) {
	case *parser.ParenExpr:
		if node.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
			if _stack, _node, found := n.nodeHistory(node.Expr, posRange, stack); found {
				head = _node
				stack = _stack
			}
		}
	case *parser.VectorSelector:
		if node.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
		}
	case *parser.BinaryExpr:
		if node.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
			stmp, _node, found := n.nodeHistory(node.LHS, posRange, stack)
			if found {
				return stmp, _node, found
			}
			if stmp, _node, found = n.nodeHistory(node.RHS, posRange, stack); found {
				return stmp, _node, found
			}
			// Since the item exists in both the child. This means that it is in binary expr range,
			// but not satisfied by a single child. This is possible only for Op and grouping
			// modifiers.
			if node.PositionRange().Contains(posRange) {
				return stack, head, true
			}
		}
	case *parser.AggregateExpr:
		if node.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
			stmp, _head, found := n.nodeHistory(node.Expr, posRange, stack)
			if found {
				return stmp, _head, true
			}
			if node.Param != nil {
				if stmp, _head, found = n.nodeHistory(node.Param, posRange, stack); found {
					return stmp, _head, true
				}
			}
		}
	case *parser.Call:
		if node.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
			for _, exprs := range node.Args {
				if exprs.PositionRange().Contains(posRange) {
					if stmp, _head, found := n.nodeHistory(exprs, posRange, stack); found {
						return stmp, _head, true
					}
				}
			}
		}
	case *parser.MatrixSelector:
		if node.VectorSelector.PositionRange().Contains(posRange) {
			stack = append(stack, node)
			nodeMatch = true
			if _stack, _node, found := n.nodeHistory(node.VectorSelector, posRange, stack); found {
				stack = _stack
				head = _node
			}
		}
	case *parser.UnaryExpr:
		stack = append(stack, node)
		if node.Expr.PositionRange().Contains(posRange) {
			nodeMatch = true
			if _stack, _node, found := n.nodeHistory(node.Expr, posRange, stack); found {
				stack = _stack
				head = _node
			}
		}
	case *parser.SubqueryExpr:
		if node.Expr.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
			if _stack, _node, found := n.nodeHistory(node.Expr, posRange, stack); found {
				stack = _stack
				head = _node
			}
		}
	case *parser.NumberLiteral, *parser.StringLiteral:
		if node.PositionRange().Contains(posRange) {
			nodeMatch = true
			stack = append(stack, node)
		}
	}
	return stack, head, nodeMatch
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

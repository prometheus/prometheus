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

func (p *nodeInfo) violatesColumnLimit() bool {
	if p.currentNode == nil {
		panic("current node not set")
	}
	return len(p.currentNode.String()) > p.columnLimit
}

// node returns the node corresponding to the given position range in the AST.
func (p *nodeInfo) node() parser.Expr {
	ancestors, node, _ := p.nodeHistory(p.head, p.item.PositionRange(), []parser.Node{})
	p.ancestors = reduceBinary(ancestors)
	p.baseIndent = len(p.ancestors)
	p.currentNode = node
	p.exprType = fmt.Sprintf("%T", node)
	return node
}

func (p *nodeInfo) getBaseIndent(item parser.Item) int {
	ancestors, _, _ := p.nodeHistory(p.head, item.PositionRange(), []parser.Node{})
	ancestors = reduceBinary(ancestors)
	p.buf = len(ancestors)
	return p.buf
}

func (p *nodeInfo) previousIndent() int {
	return p.buf
}

// contains verifies whether the current node contains a particular entity.
func (p *nodeInfo) is(element uint) bool {
	switch element {
	case grouping:
		if n, ok := p.currentNode.(*parser.BinaryExpr); ok {
			return len(n.VectorMatching.MatchingLabels) > 0 || n.ReturnBool
		}
	case scalars:
		if n, ok := p.currentNode.(*parser.BinaryExpr); ok {
			if n.LHS.Type() == parser.ValueTypeScalar || n.RHS.Type() == parser.ValueTypeScalar {
				return true
			}
		}
	case multiArguments:
		if n, ok := p.currentNode.(*parser.Call); ok {
			return len(n.Args) > 1
		}
	}
	return false
}

// parentNode returns the parent node of the given node/item position range.
func (p *nodeInfo) parentNode(head parser.Expr, rnge parser.PositionRange) parser.Node {
	ancestors, _, found := p.nodeHistory(head, rnge, []parser.Node{})
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
func (p *nodeInfo) nodeHistory(head parser.Expr, posRange parser.PositionRange, stack []parser.Node) ([]parser.Node, parser.Expr, bool) {
	var nodeMatch bool
	switch n := head.(type) {
	case *parser.ParenExpr:
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, n)
			return p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.VectorSelector:
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, n)
		}
	case *parser.BinaryExpr:
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, n)
			stmp, node, found := p.nodeHistory(n.LHS, posRange, stack)
			if found {
				return stmp, node, found
			}
			stmp, node, found = p.nodeHistory(n.RHS, posRange, stack)
			if found {
				return stmp, node, found
			}
			// Since the item exists in both the child. This means that it is in binary expr range,
			// but not satisfied by a single child. This is possible only for Op and grouping
			// modifiers.
			if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
				return stack, head, true
			}
		}
	case *parser.AggregateExpr:

		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, n)
			stmp, _head, found := p.nodeHistory(n.Expr, posRange, stack)
			if found {
				return stmp, _head, true
			}
			if n.Param != nil {
				if stmp, _head, found := p.nodeHistory(n.Param, posRange, stack); found {
					return stmp, _head, true
				}
			}
		}
	case *parser.Call:
		if n.PositionRange().Start <= posRange.Start && n.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, n)
			for _, exprs := range n.Args {
				if exprs.PositionRange().Start <= posRange.Start && exprs.PositionRange().End >= posRange.End {
					stmp, _head, found := p.nodeHistory(exprs, posRange, stack)
					if found {
						return stmp, _head, true
					}
				}
			}
		}
	case *parser.MatrixSelector:
		if n.VectorSelector.PositionRange().Start <= posRange.Start && n.VectorSelector.PositionRange().End >= posRange.End {
			stack = append(stack, n)
			nodeMatch = true
			p.nodeHistory(n.VectorSelector, posRange, stack)
		}
	case *parser.UnaryExpr:
		stack = append(stack, n)
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			nodeMatch = true
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.SubqueryExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			nodeMatch = true
			stack = append(stack, n)
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.NumberLiteral, *parser.StringLiteral:
		nodeMatch = true
		stack = append(stack, n)
	}
	return stack, head, nodeMatch
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

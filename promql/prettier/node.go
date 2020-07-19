package prettier

import (
	"github.com/prometheus/prometheus/promql/parser"
	"reflect"
)

type nodeInfo struct {
	head parser.Expr
	// node information details.
	// information details should be used only after calling the fetch.
	shouldSplit bool
	columnLimit int
	history     []reflect.Type
}

func newNodeInfo(head parser.Expr, columnLimit int) *nodeInfo {
	return &nodeInfo{
		head:        head,
		columnLimit: columnLimit,
	}
}

// fetch fetches the node information.
func (p *nodeInfo) fetch() {
	p.shouldSplit = p.violatesColumnLimit()
}

func (p *nodeInfo) violatesColumnLimit() bool {
	return len(p.head.String()) > p.columnLimit
}

// getNode returns the node corresponding to the given position range in the AST.
func (p *nodeInfo) getNode(root parser.Expr, item parser.Item) parser.Expr {
	history, node, _ := p.nodeHistory(root, item.PositionRange(), []reflect.Type{})
	p.history = history
	return node
}

func (p *nodeInfo) baseIndent(item parser.Item) int {
	if len(p.history) == 0 {
		history, _, _ := p.nodeHistory(p.head, item.PositionRange(), []reflect.Type{})
		p.history = history
	}
	return len(p.history)
}

// parentNode returns the parent node of the given node/item position range.
func (p *nodeInfo) parentNode(head parser.Expr, rnge parser.PositionRange) reflect.Type {
	// TODO: var found is of no use. Hence, remove it and the bool return from the getNodeAncestorsStack
	// since found condition can be handled by ancestors alone(confirmation needed).
	ancestors, _, found := p.nodeHistory(head, rnge, []reflect.Type{})
	if !found {
		return nil
	}
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// nodeHistory returns the ancestors along with the node in which the item is present, using the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (p *nodeInfo) nodeHistory(head parser.Expr, posRange parser.PositionRange, stack []reflect.Type) ([]reflect.Type, parser.Expr, bool) {
	switch n := head.(type) {
	case *parser.ParenExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.VectorSelector:
		stack = append(stack, reflect.TypeOf(n))
	case *parser.BinaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		stmp, _head, found := p.nodeHistory(n.LHS, posRange, stack)
		if found {
			return stmp, _head, true
		}
		stmp, _head, found = p.nodeHistory(n.RHS, posRange, stack)
		if found {
			return stmp, _head, true
		}
		// if none of the above are true, that implies, the position range is of a symbol or a grouping modifier.
	case *parser.AggregateExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.Call:
		stack = append(stack, reflect.TypeOf(n))
		for _, exprs := range n.Args {
			if exprs.PositionRange().Start <= posRange.Start && exprs.PositionRange().End >= posRange.End {
				stmp, _head, found := p.nodeHistory(exprs, posRange, stack)
				if found {
					return stmp, _head, true
				}
			}
		}
	case *parser.MatrixSelector:
		stack = append(stack, reflect.TypeOf(n))
		if n.VectorSelector.PositionRange().Start <= posRange.Start && n.VectorSelector.PositionRange().End >= posRange.End {
			p.nodeHistory(n.VectorSelector, posRange, stack)
		}
	case *parser.UnaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.SubqueryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			p.nodeHistory(n.Expr, posRange, stack)
		}
	case *parser.NumberLiteral, *parser.StringLiteral:
		stack = append(stack, reflect.TypeOf(n))
	}
	return stack, head, true
}

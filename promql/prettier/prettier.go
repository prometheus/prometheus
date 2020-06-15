package prettier

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
)

const (
	// PrettifyRules for prettifying rules files along with the
	// expressions in them.
	PrettifyRules = iota
	// PrettifyExpression for prettifying instantaneous expressions.
	PrettifyExpression
)

// Prettier handles the prettifying and formatting operation over a
// list of rules files or a single expression.
type Prettier struct {
	files      []string
	expression string
	Type       uint
	Node       parser.Expr
	pd         *padder
}

type padder struct {
	indent                                                                                         string
	previous, buff                                                                                 int
	isNewLineApplied, expectNextRHS, leftParenNotIndented, disableBaseIndent, inAggregateModifiers bool
	expectNextIdentifier, previousBaseIndent                                                       string
}

// New returns a new prettier over the given slice of files.
func New(Type uint, content interface{}) (*Prettier, error) {
	var (
		ok         bool
		files      []string
		expression string
		typ        uint
	)
	switch Type {
	case PrettifyRules:
		files, ok = content.([]string)
		typ = PrettifyRules
	case PrettifyExpression:
		expression, ok = content.(string)
		typ = PrettifyExpression
	}
	if !ok {
		return nil, errors.Errorf("invalid type: %T", reflect.TypeOf(content))
	}
	return &Prettier{
		files:      files,
		expression: expression,
		Type:       typ,
		pd:         newPadder(),
	}, nil
}

func newPadder() *padder {
	return &padder{
		indent:             "  ", // 2 space
		isNewLineApplied:   true,
		expectNextRHS:      false,
		previousBaseIndent: "  ", // start with initial indent as previous val to skip new line at start.
	}
}

// func (p *Prettier) prettifyItems(items []parser.Item, index int, result string) string {
// 	item := items[index]
// 	value := item.Val
// 	switch item.Typ {
// 	case parser.LEFT_PAREN:
// 		result += p.pd.apply() + value + "\n" + p.pd.inc(1).resume()
// 	case parser.RIGHT_PAREN:
// 		result += "\n" + p.pd.dec(1).apply() + value
// 	case parser.LEFT_BRACE, parser.LEFT_BRACKET:
// 		result += value + "\n" + p.pd.inc(1).resume()
// 	case parser.RIGHT_BRACE, parser.RIGHT_BRACKET:
// 		result += p.pd.dec(1).apply() + value
// 	case parser.IDENTIFIER, parser.DURATION, parser.NUMBER:
// 		result += p.pd.apply() + value
// 	case parser.STRING:
// 		result += value + ",\n"
// 	case parser.SUM:
// 		result += p.pd.apply() + value
// 	case parser.COLON, parser.EQL, parser.EQL_REGEX, parser.NEQ, parser.NEQ_REGEX:
// 		result += value
// 	case parser.ADD, parser.SUB, parser.DIV, parser.MUL:
// 		result += "\n" + p.pd.dec(1).apply() + value + "\n" + p.pd.inc(1).resume()
// 	case parser.GTR, parser.BOOL:
// 		if items[index+1].Typ == parser.BOOL {
// 			index++
// 			result += "\n" + p.pd.dec(1).apply() + value + " " + items[index].Val + "\n" + p.pd.inc(1).resume()
// 		} else {
// 			result += "\n" + p.pd.dec(1).apply() + value + "\n" + p.pd.inc(1).resume()
// 		}
// 	case parser.COMMENT:
// 		result += p.pd.apply() + value + "\n"

// 	case parser.BLANK:
// 	}
// 	if item.Typ == parser.EOF || len(items) == index+1 {
// 		return p.removeTrailingLines(result)
// 	}
// 	return p.prettifyItems(items, index+1, result)
// }

func (p *Prettier) prettify(items []parser.Item, index int, result string) (string, error) {
	var (
		baseIndent string
		item       = items[index]
		// nodeType = p.getNode(p.Node, item.PositionRange())
	)
	if item.Typ == parser.EOF {
		return result, nil
	}
	fmt.Println("item: ", item.Val)
	if !p.exmptBaseIndent(item) && !p.pd.disableBaseIndent && !p.pd.inAggregateModifiers {
		baseIndent = p.getBaseIndent(item, p.Node)
		if baseIndent != p.pd.previousBaseIndent && !p.pd.isNewLineApplied { // denotes change of node
			result += "\n"
		}
		p.pd.isNewLineApplied = false
		result += baseIndent
		p.pd.previousBaseIndent = baseIndent
	}

	switch item.Typ {
	case parser.LEFT_PAREN:
		result += item.Val
		if !p.pd.inAggregateModifiers {
			p.pd.disableBaseIndent = false
		}
	case parser.LEFT_BRACE:
		result += item.Val + p.pd.insertNewLine()
		p.pd.expectNextIdentifier = "label"
	case parser.RIGHT_BRACE:
		if p.pd.isNewLineApplied {
			result += p.pd.previousBaseIndent + item.Val
		} else {
			result += p.pd.insertNewLine() + p.pd.previousBaseIndent + item.Val
		}
		p.pd.isNewLineApplied = false
		p.pd.expectNextIdentifier = ""
	case parser.IDENTIFIER:
		if p.pd.expectNextIdentifier == "label" {
			result += p.pd.apply()
		}
		result += item.Val
	case parser.STRING, parser.NUMBER:
		result += item.Val
		p.pd.isNewLineApplied = false
	case parser.RIGHT_PAREN:
		if p.pd.leftParenNotIndented {
			result += item.Val
		} else {
			result += p.pd.dec(1).apply() + item.Val
		}
		if p.pd.inAggregateModifiers {
			result += " "
		}
		p.pd.expectNextIdentifier = ""
		p.pd.inAggregateModifiers = false
	case parser.EQL, parser.EQL_REGEX, parser.NEQ, parser.NEQ_REGEX, parser.POW:
		result += item.Val
	case parser.SUM, parser.AVG, parser.COUNT, parser.MIN, parser.MAX, parser.STDDEV, parser.STDVAR, parser.TOPK, parser.BOTTOMK, parser.COUNT_VALUES, parser.QUANTILE:
		result += item.Val
		p.pd.disableBaseIndent = true
	case parser.ADD, parser.SUB, parser.MUL, parser.DIV:
		result += item.Val
	case parser.WITHOUT, parser.BY, parser.IGNORING, parser.BOOL, parser.GROUP_LEFT, parser.GROUP_RIGHT, parser.OFFSET, parser.ON:
		result += " " + item.Val + " "
		p.pd.disableBaseIndent = true
		p.pd.inAggregateModifiers = true
	case parser.COMMA:
		result += item.Val + p.pd.insertNewLine()
	case parser.COMMENT:
		if p.pd.isNewLineApplied {
			result += p.pd.previousBaseIndent + item.Val + p.pd.insertNewLine()
		} else {
			result += item.Val + p.pd.insertNewLine()
		}
	}
	if index+1 == len(items) {
		return result, nil
	}
	ptmp, err := p.prettify(items, index+1, result)
	if err != nil {
		return result, errors.Wrap(err, "prettify")
	}
	return ptmp, nil
}

func (p *Prettier) exmptBaseIndent(item parser.Item) bool {
	switch item.Typ {
	case parser.EQL, parser.LEFT_BRACE, parser.RIGHT_BRACE, parser.STRING, parser.COMMA, parser.NUMBER:
		return true
	}
	return false
}

// getNode returns the node of the given position range in the AST.
func (p *Prettier) getNode(headAST parser.Expr, rnge parser.PositionRange) reflect.Type {
	ancestors, found := p.getNodeAncestorsStack(headAST, rnge, []reflect.Type{})
	if !found {
		return nil
	}
	if len(ancestors) < 1 {
		return nil
	}
	return ancestors[len(ancestors)-1]
}

// getParentNode returns the parent node of the given node/item position range.
func (p *Prettier) getParentNode(headAST parser.Expr, rnge parser.PositionRange) reflect.Type {
	ancestors, found := p.getNodeAncestorsStack(headAST, rnge, []reflect.Type{})
	if !found {
		return nil
	}
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// getNodeAncestorsStack returns the ancestors (including the node itself) of the input node from the AST.
func (p *Prettier) getNodeAncestorsStack(head parser.Expr, posRange parser.PositionRange, stack []reflect.Type) ([]reflect.Type, bool) {
	switch n := head.(type) {
	case *parser.ParenExpr:
		if n.PosRange.Start <= posRange.Start && n.PosRange.End >= posRange.End {
			fmt.Println("ancestor ", n.PosRange)
			stack = append(stack, reflect.TypeOf(n))
			return p.getNodeAncestorsStack(n.Expr, posRange, stack)
		}
		return stack, false
	case *parser.VectorSelector:
		if n.PosRange.Start <= posRange.Start && n.PosRange.End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			return stack, true
		}
		return stack, false
	case *parser.BinaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		stmp, found := p.getNodeAncestorsStack(n.LHS, posRange, stack)
		if found {
			return stmp, true
		}
		stmp, found = p.getNodeAncestorsStack(n.RHS, posRange, stack)
		if found {
			return stmp, true
		}
		// if none of the above are true, that implies, the position range is of a symbol.
		return stack, true
	case *parser.AggregateExpr:
		if n.PosRange.Start <= posRange.Start && n.PosRange.End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			return p.getNodeAncestorsStack(n.Expr, posRange, stack)
		}
	}
	return stack, true
}

// removeNodesWithoutPosRange removes the nodes (immediately after) that are added to the stack because they do not
// have any way to check the position range with the given item.
func (p *Prettier) removeNodesWithoutPosRange(stack []reflect.Type, typ string) []reflect.Type {
	for i := len(stack) - 1; i >= 0; i++ {
		if stack[i].String() == typ {
			stack = stack[:len(stack)-1]
		}
		break
	}
	return stack
}

// getBaseIndent returns the base indent for a lex item depending on the depth
// of the node the item belongs to in the AST.
func (p *Prettier) getBaseIndent(item parser.Item, node parser.Expr) string {
	var (
		indent        int
		tmp           parser.Expr
		expectNextRHS bool
		previousNode  string
		head          = node
	)
	if head == nil {
		return ""
	}
	for {
		indent++
		if head != nil && p.isItemWithinNode(head, item) {
			switch n := head.(type) {
			case *parser.AggregateExpr:
				head = n.Expr
			case *parser.ParenExpr:
				head = n.Expr
				previousNode = "*parser.ParenExpr"
			case *parser.VectorSelector:
				head = nil
				previousNode = "*parser.VectorSelector"
			case *parser.BinaryExpr:
				head = n.LHS
				tmp = n.RHS
				expectNextRHS = true
				if previousNode == "*parser.BinaryExpr" {
					indent--
				}
				previousNode = "*parser.BinaryExpr"
			}
			continue
		} else if expectNextRHS && head != nil && !item.Typ.IsOperator() {
			expectNextRHS = false
			head = tmp
			if previousNode == "*parser.BinaryExpr" {
				indent--
			}
			continue
		}
		indent--
		return p.pd.pad(indent)
	}
}

func (p *Prettier) isItemWithinNode(node parser.Node, item parser.Item) bool {
	posNode := node.PositionRange()
	itemNode := item.PositionRange()
	if posNode.Start <= itemNode.Start && posNode.End >= itemNode.End {
		return true
	}
	return false
}

func (p *Prettier) removeTrailingLines(s string) string {
	lines := strings.Split(s, "\n")
	result := ""
	for i := 0; i < len(lines); i++ {
		if len(strings.TrimSpace(lines[i])) != 0 {
			result += lines[i] + "\n"
		}
	}
	return result
}

type ruleGroupFiles struct {
	filename   string
	ruleGroups *rulefmt.RuleGroups
}

// Run executes the prettier over the rules files or expression.
func (p *Prettier) Run() []error {
	var (
		groupFiles []*rulefmt.RuleGroups
		errs       []error
	)
	switch p.Type {
	case PrettifyRules:
		for _, f := range p.files {
			ruleGroups, err := p.parseFile(f)
			if err != nil {
				for _, e := range err {
					errs = append(errs, errors.Wrapf(e, "file: %s", f))
				}
			}
			groupFiles = append(groupFiles, ruleGroups)
		}
		if errs != nil {
			return errs
		}
		for _, rgs := range groupFiles {
			for _, grps := range rgs.Groups {
				for _, rules := range grps.Rules {
					exprStr := rules.Expr.Value
					if err := p.parseExpr(exprStr); err != nil {
						return []error{err}
					}
					res := p.lexItems(exprStr)
					p.pd.buff = 1
					formattedExpr, err := p.prettify(res, 0, "")
					if err != nil {
						return []error{errors.Wrap(err, "Run")}
					}
					fmt.Println("output is ")
					fmt.Println(formattedExpr)
				}
			}
		}

	}
	return nil
}

// lexItems converts the given expression into a slice of Items.
func (p *Prettier) lexItems(expression string) (items []parser.Item) {
	l := parser.Lex(expression)

	for l.State = parser.LexStatements; l.State != nil; {
		items = append(items, parser.Item{})
		l.NextItem(&items[len(items)-1])
	}
	return
}

func (p *Prettier) parseExpr(expression string) error {
	expr, err := parser.ParseExpr(expression)
	if err != nil {
		return errors.Wrap(err, "parse error")
	}
	p.Node = expr
	return nil
}

func (p *Prettier) parseFile(name string) (*rulefmt.RuleGroups, []error) {
	b, err := ioutil.ReadFile(name)
	if err != nil {
		return nil, []error{errors.Wrap(err, "unable to read file")}
	}
	groups, errs := rulefmt.Parse(b)
	if errs != nil {
		return nil, errs
	}
	return groups, nil
}

func parseFloat(v float64) string {
	return strconv.FormatFloat(v, 'E', -1, 64)
}

// inc increments the padding by by adding the pad value
// to the previous padded value.
func (pd *padder) inc(iter int) *padder {
	pd.buff += iter
	return pd
}

// dec decrements the padding by by removing the pad value
// to the previous padded value.
func (pd *padder) dec(iter int) *padder {
	pd.buff -= iter
	return pd
}

// apply applies the padding.
func (pd *padder) apply() string {
	pad := ""
	for i := 1; i <= pd.buff; i++ {
		pad += pd.indent
	}
	return pad
}

// pad provides an instantenous padding.
func (pd *padder) pad(iter int) string {
	pad := ""
	for i := 1; i <= iter; i++ {
		pad += pd.indent
	}
	return pad
}

func (pd *padder) getIndentAmount(indent string) int {
	tmp := ""
	amount := 0
	for {
		if tmp == indent {
			return amount
		}
		tmp += pd.indent
		amount++
	}
}

func (pd *padder) resume() string {
	return ""
}

func (pd *padder) insertNewLine() string {
	pd.isNewLineApplied = true
	return "\n"
}

func (pd *padder) resumeLine() {
	pd.isNewLineApplied = false
}

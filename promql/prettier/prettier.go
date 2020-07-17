package prettier

import (
	"fmt"
	"io/ioutil"
	"reflect"
	"regexp"
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
	indent                                                string
	previous, buff, columnLimit                           int
	isNewLineApplied, expectNextRHS, leftParenNotIndented bool
	disableBaseIndent, inAggregateModifiers               bool
	expectNextIdentifier, previousBaseIndent              string
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
		columnLimit:        100,
		previousBaseIndent: "  ", // start with initial indent as previous val to skip new line at start.
	}
}

func (p *Prettier) prettify(items []parser.Item, index int, result string) (string, error) {
	var (
		//baseIndent string
		item = items[index]
	)
	if item.Typ == parser.EOF {
		return result, nil
	}
	//if !p.exmptBaseIndent(item) && !p.pd.disableBaseIndent && !p.pd.inAggregateModifiers {
	//	baseIndent = p.getBaseIndent(item, p.Node) // p.Node is the root node (TODO: renaming node to root node)
	//	if baseIndent != p.pd.previousBaseIndent && !p.pd.isNewLineApplied { // denotes change of node
	//		result += "\n"
	//	}
	//	p.pd.isNewLineApplied = false
	//	result += baseIndent
	//	p.pd.previousBaseIndent = baseIndent
	//}

	node := p.getNode(p.Node, item.PositionRange())
	nodeInformation := newNodeInfo(node, 100)
	nodeInformation.fetch()

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

// getNode returns the node corresponding to the given position range in the AST.
func (p *Prettier) getNode(headAST parser.Expr, rnge parser.PositionRange) parser.Expr {
	_, node, _ := p.getCurrentNodeHistory(headAST, rnge, []reflect.Type{})
	return node
}

// getParentNode returns the parent node of the given node/item position range.
func (p *Prettier) getParentNode(headAST parser.Expr, rnge parser.PositionRange) reflect.Type {
	// TODO: var found is of no use. Hence, remove it and the bool return from the getNodeAncestorsStack
	// since found condition can be handled by ancestors alone(confirmation needed).
	ancestors, _, found := p.getCurrentNodeHistory(headAST, rnge, []reflect.Type{})
	if !found {
		return nil
	}
	if len(ancestors) < 2 {
		return nil
	}
	return ancestors[len(ancestors)-2]
}

// getCurrentNodeHistory returns the ancestors along with the node in which the item is present, using the help of AST.
// posRange can also be called as itemPosRange since carries the position range of the lexical item.
func (p *Prettier) getCurrentNodeHistory(head parser.Expr, posRange parser.PositionRange, stack []reflect.Type) ([]reflect.Type, parser.Expr, bool) {
	switch n := head.(type) {
	case *parser.ParenExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.VectorSelector:
		stack = append(stack, reflect.TypeOf(n))
	case *parser.BinaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		stmp, _head, found := p.getCurrentNodeHistory(n.LHS, posRange, stack)
		if found {
			return stmp, _head, true
		}
		stmp, _head, found = p.getCurrentNodeHistory(n.RHS, posRange, stack)
		if found {
			return stmp, _head, true
		}
		// if none of the above are true, that implies, the position range is of a symbol or a grouping modifier.
	case *parser.AggregateExpr:
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			stack = append(stack, reflect.TypeOf(n))
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.Call:
		stack = append(stack, reflect.TypeOf(n))
		for _, exprs := range n.Args {
			if exprs.PositionRange().Start <= posRange.Start && exprs.PositionRange().End >= posRange.End {
				stmp, _head, found := p.getCurrentNodeHistory(exprs, posRange, stack)
				if found {
					return stmp, _head, true
				}
			}
		}
	case *parser.MatrixSelector:
		stack = append(stack, reflect.TypeOf(n))
		if n.VectorSelector.PositionRange().Start <= posRange.Start && n.VectorSelector.PositionRange().End >= posRange.End {
			p.getCurrentNodeHistory(n.VectorSelector, posRange, stack)
		}
	case *parser.UnaryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.SubqueryExpr:
		stack = append(stack, reflect.TypeOf(n))
		if n.Expr.PositionRange().Start <= posRange.Start && n.Expr.PositionRange().End >= posRange.End {
			p.getCurrentNodeHistory(n.Expr, posRange, stack)
		}
	case *parser.NumberLiteral, *parser.StringLiteral:
		stack = append(stack, reflect.TypeOf(n))
	}
	return stack, head, true
}

// removeNodesWithoutPosRange removes the nodes (immediately after) that are added to the stack because they do not
// have any way to check the position range with the given item.
func (p *Prettier) removeNodesWithoutPosRange(stack []reflect.Type, typ string) []reflect.Type {
	for i := len(stack) - 1; i >= 0; i-- {
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
	//var (
	//	indent        int
	//	tmp           parser.Expr
	//	expectNextRHS bool
	//	previousNode  string
	//	head          = node
	//)
	//if head == nil {
	//	return ""
	//}
	//for {
	//	indent++
	//	if head != nil && p.isItemWithinNode(head, item) {
	//		switch n := head.(type) {
	//		case *parser.AggregateExpr:
	//			head = n.Expr
	//		case *parser.ParenExpr:
	//			head = n.Expr
	//			previousNode = "*parser.ParenExpr"
	//		case *parser.VectorSelector:
	//			head = nil
	//			previousNode = "*parser.VectorSelector"
	//		case *parser.BinaryExpr:
	//			head = n.LHS
	//			tmp = n.RHS
	//			expectNextRHS = true
	//			if previousNode == "*parser.BinaryExpr" {
	//				indent--
	//			}
	//			previousNode = "*parser.BinaryExpr"
	//		}
	//		continue
	//	} else if expectNextRHS && head != nil && !item.Typ.IsOperator() {
	//		expectNextRHS = false
	//		head = tmp
	//		if previousNode == "*parser.BinaryExpr" {
	//			indent--
	//		}
	//		continue
	//	}
	//	indent--
	//	return p.pd.pad(indent)
	//}
	//nodes, found := p.getNodeAncestorsStack(node, item.PositionRange(), []reflect.Type{})
	//if !found || len(nodes) == 1 {
	//	return ""
	//}
	//return p.pd.pad(len(nodes))
	return ""
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
					lexItemSlice := p.lexItems(exprStr)
					fmt.Println("before sorting")
					fmt.Println(lexItemSlice)
					lexItemSlice = p.sortItems(lexItemSlice, false)
					fmt.Println("after sorting")
					fmt.Println(lexItemSlice)
					break
					p.pd.buff = 1
					formattedExpr, err := p.prettify(lexItemSlice, 0, "")
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

func (p *Prettier) sortItems(items []parser.Item, isTest bool) []parser.Item {
	var requiresRefresh bool
	for i := 0; i < len(items); i++ {
		item := items[i]
		switch item.Typ {
		case parser.SUM, parser.AVG, parser.MIN, parser.MAX, parser.COUNT, parser.COUNT_VALUES, parser.STDDEV,
			parser.STDVAR, parser.TOPK, parser.BOTTOMK, parser.QUANTILE:
			var (
				formatItems       bool
				aggregatorIndex   = i
				keywordIndex      = -1
				closingParenIndex = -1
				openBracketsCount = 0
			)
			for index := i; index < len(items); index++ {
				it := items[index]
				switch it.Typ {
				case parser.LEFT_PAREN, parser.LEFT_BRACE:
					openBracketsCount++
				case parser.RIGHT_PAREN, parser.RIGHT_BRACE:
					openBracketsCount--
				}
				if openBracketsCount < 0 {
					break
				}
				if openBracketsCount == 0 && it.Typ.IsKeyword() {
					keywordIndex = index
					break
				}
			}
			if keywordIndex < 0 {
				continue
			}

			if items[keywordIndex-1].Typ != parser.COMMENT && keywordIndex-1 != aggregatorIndex {
				formatItems = true
			} else if items[keywordIndex-1].Typ == parser.COMMENT {
				// peek back until no comment is found
				pos := -1
				for j := keywordIndex; j > aggregatorIndex; j-- {
					if items[j].Typ == parser.COMMENT {
						continue
					} else {
						pos = j
						break
					}
				}
				if pos != aggregatorIndex {
					formatItems = true
				}
			}
			if formatItems {
				requiresRefresh = true
				// get first index of the closing paren.
				for j := keywordIndex; j <= len(items); j++ {
					if items[j].Typ == parser.RIGHT_PAREN {
						closingParenIndex = j
						break
					}
				}
				if closingParenIndex == -1 {
					panic("invalid paren index: closing paren not found")
				}
				// order elements. TODO: consider using slicing of lists
				var tempItems []parser.Item
				var finishedKeywordWriting bool
				for itemp := 0; itemp < len(items); itemp++ {
					if (itemp <= aggregatorIndex || itemp > closingParenIndex) && !(itemp >= keywordIndex && itemp <= closingParenIndex) {
						tempItems = append(tempItems, items[itemp])
					} else if !finishedKeywordWriting {
						for j := keywordIndex; j <= closingParenIndex; j++ {
							tempItems = append(tempItems, items[j])
						}
						tempItems = append(tempItems, items[itemp])
						finishedKeywordWriting = true
					} else if !(itemp >= keywordIndex && itemp <= closingParenIndex) {
						tempItems = append(tempItems, items[itemp])
					}
				}
				items = tempItems
			}

		case parser.IDENTIFIER:
			var (
				leftBraceIndex  = -1
				rightBraceIndex = -1
				metricName      = labelValItem.Val[1 : len(labelValItem.Val)-1]
				labelValItem    = items[i+2]
				tmp             []parser.Item
				skipBraces      bool
			)
			//	TODO: currently its assumed that these are always labels.
			if i < 1 || item.Val != "__name__" || items[i+2].Typ != parser.STRING {
				continue
			}
			if !regexp.MustCompile("^[a-z_A-Z]+$").MatchString(labelValItem.Val[1 : len(labelValItem.Val)-1]) {
				continue
			}
			for backScanIndex := i; backScanIndex >= 0; backScanIndex-- {
				if items[backScanIndex].Typ == parser.LEFT_BRACE {
					leftBraceIndex = backScanIndex
					break
				}
			}
			for forwardScanIndex := i; forwardScanIndex < len(items); forwardScanIndex++ {
				if items[forwardScanIndex].Typ == parser.RIGHT_BRACE {
					rightBraceIndex = forwardScanIndex
					break
				}
			}
			// TODO: assuming comments are not present at this place.
			if items[i+3].Typ == parser.COMMA {
				skipBraces = rightBraceIndex-5 == leftBraceIndex
			} else {
				skipBraces = rightBraceIndex-4 == leftBraceIndex
			}
			identifierItem := parser.Item{Typ: parser.IDENTIFIER, Val: metricName, Pos: 0}
			for j := 0; j < len(items); j++ {
				if j >= leftBraceIndex && j <= rightBraceIndex {
					if j == leftBraceIndex {
						tmp = append(tmp, identifierItem)
						if !skipBraces {
							tmp = append(tmp, items[j])
						}
						continue
					} else if items[i+3].Typ == parser.COMMA && j == i+3 || j >= i && j < i+3 {
						continue
					}
					if skipBraces && (items[j].Typ == parser.LEFT_BRACE || items[j].Typ == parser.RIGHT_BRACE) {
						continue
					}
				}
				tmp = append(tmp, items[j])
			}
			items = tmp
		}
	}
	if requiresRefresh || isTest {
		return p.refreshLexItems(items)
	}
	return items
}

// refreshLexItems refreshes the contents of the lex slice and the properties
// within it. This is expected to be called after sorting the lexItems in the
// pre-format checks.
func (p *Prettier) refreshLexItems(items []parser.Item) []parser.Item {
	return p.lexItems(p.expressionFromItems(items))
}

// expressionFromItems returns the raw expression from slice of items.
// This is mostly used in re-ordering activities where the position of
// the items changes during sorting in the sortItems.
func (p *Prettier) expressionFromItems(items []parser.Item) string {
	expression := ""
	for _, item := range items {
		switch item.Typ {
		case parser.COMMENT:
			expression += item.Val + "\n"
		case parser.COMMA:
			expression += item.Val + " "
		default:
			expression += item.Val + " "
		}
	}
	return expression
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

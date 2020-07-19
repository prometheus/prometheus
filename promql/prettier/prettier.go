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
	indent                      string
	previous, buff, columnLimit int
	// isNewLineApplied keeps the track of whether a new line was added
	// after appending the item to the result. It should be set to default
	// after applying baseIndent.
	expectNextRHS, leftParenNotIndented      bool
	disableBaseIndent, inAggregateModifiers  bool
	expectNextIdentifier, previousBaseIndent string
	containsGrouping                         bool
	isNewLineApplied                         bool
	//	new ones
	insideMultilineBraces bool
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
	if items[index].Typ == parser.EOF {
		return result, nil
	}
	var (
		item       = items[index]
		nodeInfo   = newNodeInfo(p.Node, 100)
		node       = nodeInfo.getNode(p.Node, item)
		currNode   = newNodeInfo(node, 100)
		baseIndent int
	)
	currNode.fetch()
	if item.Typ.IsOperator() && reflect.TypeOf(node).String() == "parser.BinaryExpr" {
		p.pd.containsGrouping = currNode.contains("grouping-modifier")
	}
	nodeSplittable := currNode.violatesColumnLimit()
	if p.pd.isNewLineApplied {
		baseIndent = nodeInfo.baseIndent(item)
		result += p.pd.pad(baseIndent)
		p.pd.isNewLineApplied = false
	}

	switch item.Typ {
	case parser.LEFT_PAREN:
		result += item.Val
		if !p.pd.inAggregateModifiers {
			p.pd.disableBaseIndent = false
		}
	case parser.LEFT_BRACE:
		result += item.Val
		if nodeSplittable {
			result += p.pd.newLine()
			p.pd.insideMultilineBraces = true
		}
	case parser.RIGHT_BRACE:
		if p.pd.insideMultilineBraces {
			if items[index-1].Typ != parser.COMMA {
				// Edge-case: if the labels are multi-line split, but do not have
				// a pre-applied comma.
				result += "," + p.pd.newLine() + p.pd.pad(currNode.baseIndent(item))
			}
			p.pd.insideMultilineBraces = false
		}
		result += item.Val
	case parser.IDENTIFIER:
		if p.pd.insideMultilineBraces {
			result += p.pd.pad(1) + item.Val
		} else {
			result += item.Val
		}
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
	case parser.ADD, parser.SUB, parser.MUL, parser.DIV:
		if p.pd.containsGrouping {
			result += p.pd.newLine() + p.pd.pad(baseIndent) + item.Val + " "
		} else if nodeSplittable {
			result += p.pd.newLine() + p.pd.pad(baseIndent) + item.Val + p.pd.newLine()
		} else {
			result += " " + item.Val + " "
		}
	case parser.WITHOUT, parser.BY, parser.IGNORING, parser.BOOL, parser.GROUP_LEFT, parser.GROUP_RIGHT, parser.OFFSET, parser.ON:
		result += " " + item.Val + " "
		p.pd.disableBaseIndent = true
		p.pd.inAggregateModifiers = true
	case parser.COMMA:
		result += item.Val
		if nodeSplittable {
			result += p.pd.newLine()
		} else {
			result += " "
		}
	case parser.COMMENT:
		if p.pd.isNewLineApplied {
			result += p.pd.previousBaseIndent + item.Val + p.pd.newLine()
		} else {
			result += item.Val + p.pd.newLine()
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
					standardizeExprStr := p.expressionFromItems(p.lexItems(exprStr))
					if err := p.parseExpr(standardizeExprStr); err != nil {
						return []error{err}
					}
					formattedExpr, err := p.prettify(
						p.sortItems(
							p.lexItems(exprStr),
							false,
						),
						0,
						"",
					)
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
			//	TODO: currently its assumed that these are always labels.
			if i < 1 || item.Val != "__name__" || items[i+2].Typ != parser.STRING {
				continue
			}
			var (
				leftBraceIndex  = -1
				rightBraceIndex = -1
				labelValItem    = items[i+2]
				metricName      = labelValItem.Val[1 : len(labelValItem.Val)-1]
				tmp             []parser.Item
				skipBraces      bool
			)
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
	return p.refreshLexItems(items)
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
// This function also standardizes the expression without any spaces,
// so that the length determined by .String() is done purely on the
// character length.
func (p *Prettier) expressionFromItems(items []parser.Item) string {
	expression := ""
	for _, item := range items {
		switch item.Typ {
		case parser.COMMENT:
			expression += item.Val + "\n"
		default:
			expression += item.Val
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

func (pd *padder) newLine() string {
	pd.isNewLineApplied = true
	return "\n"
}

func (pd *padder) resumeLine() {
	pd.isNewLineApplied = false
}

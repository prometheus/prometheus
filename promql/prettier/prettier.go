package prettier

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"io/ioutil"
	"reflect"
	"regexp"
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
	indent      string
	columnLimit int
	// isNewLineApplied keeps the track of whether a new line was added
	// after appending the item to the result. It should be set to default
	// after applying baseIndent.
	isNewLineApplied      bool
	containsGrouping      bool
	insideMultilineBraces bool
	previousBaseIndent    string
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
		nodeInfo   = newNodeInfo(p.Node, 100, item)
		node       = nodeInfo.getNode(p.Node, item)
		currNode   = newNodeInfo(node, 100, item)
		baseIndent int
	)
	if item.Typ.IsOperator() && reflect.TypeOf(node).String() == "*parser.BinaryExpr" {
		p.pd.containsGrouping = currNode.contains("grouping-modifier") || items[index+1].Typ == parser.BOOL
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
		if nodeSplittable && !p.pd.containsGrouping {
			result += p.pd.newLine()
		}
	case parser.RIGHT_PAREN:
		if nodeSplittable && !p.pd.containsGrouping {
			result += p.pd.newLine() + p.pd.pad(nodeInfo.baseIndent(item))
		}
		result += item.Val
		if p.pd.containsGrouping {
			p.pd.containsGrouping = false
			result += p.pd.newLine()
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
				result += "," + p.pd.newLine() + p.pd.pad(nodeInfo.baseIndent(items[index]))
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
	case parser.SUM, parser.AVG, parser.COUNT, parser.MIN, parser.MAX, parser.STDDEV, parser.STDVAR,
		parser.TOPK, parser.BOTTOMK, parser.COUNT_VALUES, parser.QUANTILE:
		result += item.Val
	case parser.EQL, parser.EQL_REGEX, parser.NEQ, parser.NEQ_REGEX:
		result += item.Val
	case parser.ADD, parser.SUB, parser.DIV, parser.GTE, parser.GTR, parser.LOR, parser.LAND,
		parser.LSS, parser.LTE, parser.LUNLESS, parser.MOD, parser.POW:
		if p.pd.containsGrouping {
			result += p.pd.newLine() + p.pd.pad(baseIndent) + item.Val + " "
		} else if nodeSplittable {
			result += p.pd.newLine() + p.pd.pad(baseIndent) + item.Val + p.pd.newLine()
		} else {
			result += " " + item.Val + " "
		}
	case parser.WITHOUT, parser.BY, parser.IGNORING, parser.BOOL, parser.GROUP_LEFT, parser.GROUP_RIGHT,
		parser.OFFSET, parser.ON:
		result += item.Val
		if item.Typ == parser.BOOL {
			result += p.pd.newLine()
		}
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
						p.sortItems(p.lexItems(exprStr)),
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

func (p *Prettier) sortItems(items []parser.Item) []parser.Item {
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
				// Peek back until no comment is found.
				// If the item immediately before the comment is not aggregator, that means
				// the keyword is not (immediately) after aggregator item and hence formatting
				// is required.
				var j int
				for j = keywordIndex; j > aggregatorIndex; j-- {
					if items[j].Typ == parser.COMMENT {
						continue
					}
					break
				}
				if j != aggregatorIndex {
					formatItems = true
				}
			}
			if formatItems {
				var (
					tempItems              []parser.Item
					finishedKeywordWriting bool
				)
				// Get index of the closing paren.
				for j := keywordIndex; j <= len(items); j++ {
					if items[j].Typ == parser.RIGHT_PAREN {
						closingParenIndex = j
						break
					}
				}
				if closingParenIndex == -1 {
					panic("invalid paren index: closing paren not found")
				}
				// Re-order lexical items. TODO: consider using slicing of lists.
				for itemp := 0; itemp < len(items); itemp++ {
					if itemp <= aggregatorIndex || itemp > closingParenIndex {
						tempItems = append(tempItems, items[itemp])
					} else if !finishedKeywordWriting {
						// Immediately after writing the aggregator, write the keyword and
						// the following expression till the closing paren. This is done
						// to be in-order with the expected result.
						for j := keywordIndex; j <= closingParenIndex; j++ {
							tempItems = append(tempItems, items[j])
						}
						// itemp has not been updated yet and its holding the left paren.
						tempItems = append(tempItems, items[itemp])
						finishedKeywordWriting = true
					} else if !(itemp >= keywordIndex && itemp <= closingParenIndex) {
						tempItems = append(tempItems, items[itemp])
					}
				}
				items = tempItems
			}

		case parser.IDENTIFIER:
			//	TODO: currently its assumed that STRING are always label values.
			itr := advanceComments(items, i)
			if i < 1 || item.Val != "__name__" || items[itr+2].Typ != parser.STRING {
				continue
			}
			var (
				leftBraceIndex  = -1
				rightBraceIndex = -1
				labelValItem    = items[itr+2]
				metricName      = labelValItem.Val[1 : len(labelValItem.Val)-1] // Trim inverted commas.
				tmp             []parser.Item
				skipBraces      bool // For handling metric_name{} -> metric_name
			)
			// We aim to convert __name__ to an ideal metric_name if the value of that labels is atomic.
			// Since a non-atomic metric_name will contain alphabets other than a-z and A-Z including _,
			// anything that violates this ceases the formatting of that particular label item.
			// If this is not done then the output from the prettier might be an un-parsable expression.
			if !regexp.MustCompile("^[a-z_A-Z]+$").MatchString(metricName) {
				continue
			}
			for backScanIndex := itr; backScanIndex >= 0; backScanIndex-- {
				if items[backScanIndex].Typ == parser.LEFT_BRACE {
					leftBraceIndex = backScanIndex
					break
				}
			}
			for forwardScanIndex := itr; forwardScanIndex < len(items); forwardScanIndex++ {
				if items[forwardScanIndex].Typ == parser.RIGHT_BRACE {
					rightBraceIndex = forwardScanIndex
					break
				}
			}
			// TODO: assuming comments are not present at this place.
			itr = advanceComments(items, itr)
			if items[itr+3].Typ == parser.COMMA {
				skipBraces = rightBraceIndex-5 == advanceComments(items, leftBraceIndex)
			} else {
				skipBraces = rightBraceIndex-4 == advanceComments(items, leftBraceIndex)
			}
			identifierItem := parser.Item{Typ: parser.IDENTIFIER, Val: metricName, Pos: 0}
			for j := 0; j < len(items); j++ {
				if j <= rightBraceIndex && j >= leftBraceIndex {
					if j == leftBraceIndex {
						tmp = append(tmp, identifierItem)
						if !skipBraces {
							tmp = append(tmp, items[j])
						}
						continue
					} else if items[itr+3].Typ == parser.COMMA && j == itr+3 || j >= itr && j < itr+3 {
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
// the items change during sorting in the sortItems.
// This function also standardizes the expression without any spaces,
// so that the length determined by .String() is done purely on the
// character length with standard indent.
func (p *Prettier) expressionFromItems(items []parser.Item) string {
	expression := ""
	for _, item := range items {
		if item.Typ.IsOperator() || item.Typ.IsAggregator() {
			expression += " " + item.Val + " "
			continue
		}
		switch item.Typ {
		case parser.COMMENT:
			expression += item.Val + "\n"
		case parser.BOOL:
			expression += item.Val + " "
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

// advanceComments advances the index until a non-comment item is
// encountered.
func advanceComments(items []parser.Item, index int) int {
	var i int
	for i = index; i < len(items); i++ {
		if items[i].Typ == parser.COMMENT {
			continue
		}
		break
	}
	return i
}

// pad provides an instantaneous padding.
func (pd *padder) pad(iter int) string {
	pad := ""
	for i := 1; i <= iter; i++ {
		pad += pd.indent
	}
	return pad
}

func (pd *padder) newLine() string {
	pd.isNewLineApplied = true
	return "\n"
}

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
	// itemNotFound is used to initialize the index of an item.
	itemNotFound = -1
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
	// isNewLineApplied represents a new line was added previously, after
	// appending the item to the result. It should be set to default
	// after applying baseIndent. This property is dependent across the padder states
	// and hence needs to be carried along while formatting.
	isNewLineApplied bool
	// isPreviousItemComment confirms whether the previous formatted item was
	// a comment. This property is dependent on previous padder states and hence needs
	// to be carried along while formatting.
	isPreviousItemComment bool
	// expectAggregationLabels expects the following item(s) to be labels of aggregation
	// expression. Labels are used in AggregateExpr and in VectorSelector but only the ones
	// in VectorSelector are splittable.
	expectAggregationLabels bool
	// labelsSplittable determines whether the current formatting region lies in labels
	// matchers in VectorSelector. Since labels in VectorSelectors are only splittable,
	// is property is applied only after encountering a Left_Brace. This is dependent across
	// multiple padder states and hence needs to be carried along.
	labelsSplittable bool
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
		indent:           "  ", // 2 space
		isNewLineApplied: true,
		columnLimit:      100,
	}
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
					standardizeExprStr := p.stringifyItems(p.sortItems(p.lexItems(exprStr)))
					if err := p.parseExpr(standardizeExprStr); err != nil {
						return []error{err}
					}
					formattedExpr, err := p.prettify(
						p.sortItems(p.lexItems(exprStr)),
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
		case parser.SUM, parser.AVG, parser.MIN, parser.MAX, parser.COUNT, parser.COUNT_VALUES,
			parser.STDDEV, parser.STDVAR, parser.TOPK, parser.BOTTOMK, parser.QUANTILE:
			var (
				formatItems       bool
				aggregatorIndex   = i
				keywordIndex      = itemNotFound
				closingParenIndex = itemNotFound
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
			if keywordIndex == itemNotFound {
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
				if closingParenIndex == itemNotFound {
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
			itr := advanceComments(items, i)
			if i < 1 || item.Val != "__name__" || items[itr+2].Typ != parser.STRING {
				continue
			}
			var (
				leftBraceIndex  = itemNotFound
				rightBraceIndex = itemNotFound
				labelValItem    = items[itr+2]
				metricName      = labelValItem.Val[1 : len(labelValItem.Val)-1] // Trim inverted-commas.
				tmp             []parser.Item
				skipBraces      bool // For handling metric_name{} -> metric_name.
			)
			if isMetricNameNotAtomic(metricName) {
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
			if leftBraceIndex == itemNotFound || rightBraceIndex == itemNotFound {
				continue
			}
			itr = advanceComments(items, itr)
			if items[itr+3].Typ == parser.COMMA {
				skipBraces = rightBraceIndex-5 == advanceComments(items, leftBraceIndex)
			} else {
				skipBraces = rightBraceIndex-4 == advanceComments(items, leftBraceIndex)
			}
			identifierItem := parser.Item{Typ: parser.IDENTIFIER, Val: metricName, Pos: 0}
			// The code below re-orders the lex items in the items array and update the same. The re-ordering of
			// lex items should be such that the key of any key-value pair if is `__name__` and its corresponding value
			// being atomic (excluding `bool`), then the value of those key-value pair will be rearranged (written)
			// prior to the leftBraceIndex.
			//
			// By now, we have the indexes of leftBrace and rightBrace. Since the leftBraceIndex and rightBraceIndex
			// forms a range, all those items out of this range are simply copied as they do not any updates to their
			// relative positions. However, those items within the range are added to the lex item slice after
			// appending the identifier (i.e., atomic value of the `__name__` key) to the lex item slice.
			//
			// Also, we want to avoid `metric_name{}` case while re-ordering a `{__name__="metric_name"}`.
			// Hence, when we are inside the leftBraceIndex and rightBraceIndex range, we check if the left and
			// right braces are next to each other, then we skip appending them (braces) to the items slice.
			for j := range items {
				if j <= rightBraceIndex && j >= leftBraceIndex {
					// Before printing the left_brace, we print the metric_name. After this, we check for metric_name{}
					// condition. If __name__ is the only label inside the label_matchers, we skip printing '{' and '}'.
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

func (p *Prettier) prettify(items []parser.Item) (string, error) {
	var (
		it     = itemsIterator{items, -1}
		result = ""
	)
	for it.next() {
		var (
			item           = it.peek()
			nodeInfo       = &nodeInfo{head: p.Node, columnLimit: 100, item: item}
			node           = nodeInfo.node()
			nodeSplittable = nodeInfo.violatesColumnLimit()
			// Binary expression use-case.
			hasImmediateScalar bool
			hasGrouping        bool
			// Aggregate expression use-case.
			hasMultiArgumentCalls bool
		)
		if p.pd.isPreviousItemComment {
			p.pd.isPreviousItemComment = false
			result += p.pd.newLine()
		}
		switch node.(type) {
		case *parser.BinaryExpr:
			hasImmediateScalar = nodeInfo.is(scalars)
			hasGrouping = nodeInfo.is(grouping)
		case *parser.Call:
			hasMultiArgumentCalls = nodeInfo.is(multiArguments)
		}
		if p.pd.isNewLineApplied {
			result += p.pd.pad(nodeInfo.baseIndent)
			p.pd.isNewLineApplied = false
		}

		switch item.Typ {
		case parser.LEFT_PAREN:
			result += item.Val
			if ((nodeSplittable && !hasGrouping) || hasMultiArgumentCalls) && (!p.pd.expectAggregationLabels) {
				result += p.pd.newLine()
			}
		case parser.RIGHT_PAREN:
			if ((nodeSplittable && !hasGrouping) || hasMultiArgumentCalls) && !p.pd.expectAggregationLabels {
				result += p.pd.newLine() + p.pd.pad(nodeInfo.baseIndent)
				p.pd.isNewLineApplied = false
			}
			result += item.Val
			if hasGrouping {
				hasGrouping = false
				result += p.pd.newLine()
			}
			if p.pd.expectAggregationLabels {
				p.pd.expectAggregationLabels = false
				result += " "
			}
		case parser.LEFT_BRACE:
			result += item.Val
			if nodeSplittable {
				// This situation arises only in vector selectors that violate column limit and have labels.
				result += p.pd.newLine()
				p.pd.labelsSplittable = true
			}
		case parser.RIGHT_BRACE:
			if p.pd.labelsSplittable {
				if it.prev().Typ != parser.COMMA {
					// Edge-case: if the labels are multi-line split, but do not have
					// a pre-applied comma.
					result += "," + p.pd.newLine() + p.pd.pad(nodeInfo.getBaseIndent(item))
				}
				p.pd.labelsSplittable = false
				p.pd.isNewLineApplied = false
			}
			result += item.Val
		case parser.IDENTIFIER:
			if p.pd.labelsSplittable {
				result += p.pd.pad(1) + item.Val
			} else {
				result += item.Val
			}
		case parser.STRING, parser.NUMBER:
			result += item.Val
		case parser.SUM, parser.BOTTOMK, parser.COUNT_VALUES, parser.COUNT, parser.GROUP, parser.MAX, parser.MIN,
			parser.QUANTILE, parser.STDVAR, parser.STDDEV, parser.TOPK:
			// Aggregations.
			result += item.Val + " "
		case parser.EQL, parser.EQL_REGEX, parser.NEQ, parser.NEQ_REGEX, parser.DURATION, parser.COLON,
			parser.LEFT_BRACKET, parser.RIGHT_BRACKET:
			// Comparison operators.
			result += item.Val
		case parser.ADD, parser.SUB, parser.DIV, parser.GTE, parser.GTR, parser.LOR, parser.LAND,
			parser.LSS, parser.LTE, parser.LUNLESS, parser.MOD, parser.POW:
			// Vector matching operators.
			if hasImmediateScalar {
				result += " " + item.Val + " "
				hasImmediateScalar = false
			} else {
				result += p.pd.newLine() + p.pd.pad(nodeInfo.baseIndent) + item.Val
				if hasGrouping {
					result += " "
					p.pd.isNewLineApplied = false
				} else {
					result += p.pd.newLine()
				}
			}
		case parser.WITHOUT, parser.BY, parser.IGNORING, parser.BOOL, parser.GROUP_LEFT, parser.GROUP_RIGHT,
			parser.OFFSET, parser.ON:
			// Keywords.
			result += item.Val
			if item.Typ == parser.BOOL {
				result += p.pd.newLine()
			} else if nodeInfo.exprType == "*parser.AggregateExpr" {
				p.pd.expectAggregationLabels = true
			}
		case parser.COMMA:
			result += item.Val
			if nodeSplittable || hasMultiArgumentCalls {
				result += p.pd.newLine()
			} else {
				result += " "
			}
		case parser.COMMENT:
			if result[len(result)-1] != ' ' {
				result += " "
			}
			if p.pd.isNewLineApplied {
				result += p.pd.pad(nodeInfo.previousIndent()) + item.Val
			} else {
				result += item.Val
			}
			p.pd.isPreviousItemComment = true
		}
	}
	return result, nil
}

// refreshLexItems refreshes the contents of the lex slice and the properties
// within it. This is expected to be called after sorting the lexItems in the
// pre-format checks.
func (p *Prettier) refreshLexItems(items []parser.Item) []parser.Item {
	return p.lexItems(p.stringifyItems(items))
}

// stringifyItems returns a standardized string expression
// from slice of items. This is mostly used in re-ordering activities
// where the position of the items change during sorting in the sortItems.
// This function also standardizes the expression without any spaces,
// so that the length determined by .String() is done purely on the
// character length with standard indent.
func (p *Prettier) stringifyItems(items []parser.Item) string {
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
	var (
		l    = parser.Lex(expression)
		item parser.Item
	)
	for l.NextItem(&item); item.Typ != parser.EOF; l.NextItem(&item) {
		items = append(items, item)
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

type itemsIterator struct {
	itemsSlice []parser.Item
	index      int
}

// next increments the index and returns the true if there exists an lex item after.
func (it *itemsIterator) next() bool {
	it.index++
	if it.index == len(it.itemsSlice) {
		return false
	}
	return true
}

// peek returns the lex item at the current index.
func (it *itemsIterator) peek() parser.Item {
	return it.itemsSlice[it.index]
}

// prev returns the previous lex item.
func (it *itemsIterator) prev() parser.Item {
	if it.index == 0 {
		return parser.Item{}
	}
	return it.itemsSlice[it.index-1]
}

// isMetricNameAtomic returns true if the metric name is singular in nature.
// This is used during sorting of lex items.
func isMetricNameNotAtomic(metricName string) bool {
	// We aim to convert __name__ to an ideal metric_name if the value of that labels is atomic.
	// Since a non-atomic metric_name will contain alphabets other than a-z and A-Z including _,
	// anything that violates this ceases the formatting of that particular label item.
	// If this is not done then the output from the prettier might be an un-parsable expression.
	if regexp.MustCompile("^[a-z_A-Z]+$").MatchString(metricName) && metricName != "bool" {
		return false
	}
	return true
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

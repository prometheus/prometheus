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

import (
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

const (
	// itemNotFound is used to initialize the index of an item.
	itemNotFound  = -1
	columnLimit   = 100
	defaultIndent = "  "
)

// Prettier handles the prettifying and formatting operation over a
// list of rules files or a single expression.
type Prettier struct {
	Node       Expr
	expression string
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
	isFirstLabel     bool
}

// PromqlPrettier returns a new prettier.
func PromqlPrettier(expression string) *Prettier {
	return &Prettier{
		expression: expression,
		pd:         &padder{indent: defaultIndent, isNewLineApplied: true, columnLimit: columnLimit},
	}
}

// Prettify prettifies the current expression in the prettier. It standardizes the input expression,
// sorts the items slice and calls the prettify.
func (p *Prettier) Prettify() (string, error) {
	standardizeExprStr := stringifyItems(
		p.sortItems(p.lexItems(p.expression)),
	)
	if err := p.parseExpr(standardizeExprStr); err != nil {
		return p.expression, err
	}
	formattedExpr, err := p.prettify(p.lexItems(standardizeExprStr))
	if err != nil {
		return p.expression, errors.Wrap(err, "Prettify")
	}
	return formattedExpr, nil
}

func (p *Prettier) sortItems(items []Item) []Item {
	for i := 0; i < len(items); i++ {
		item := items[i]
		switch item.Typ {
		case SUM, AVG, MIN, MAX, COUNT, COUNT_VALUES,
			STDDEV, STDVAR, TOPK, BOTTOMK, QUANTILE:
			// Case: sum(metric_name) by(job) => sum by(job) (metric_name)
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
				case LEFT_PAREN, LEFT_BRACE:
					openBracketsCount++
				case RIGHT_PAREN, RIGHT_BRACE:
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

			if items[keywordIndex-1].Typ != COMMENT && keywordIndex-1 != aggregatorIndex {
				formatItems = true
			} else if items[keywordIndex-1].Typ == COMMENT {
				// Peek back until no comment is found.
				// If the item immediately before the comment is not aggregator, that means
				// the keyword is not (immediately) after aggregator item and hence formatting
				// is required.
				var j int
				for j = keywordIndex; j > aggregatorIndex; j-- {
					if items[j].Typ == COMMENT {
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
					tempItems              []Item
					finishedKeywordWriting bool
				)
				// Get index of the closing paren.
				for j := keywordIndex; j <= len(items); j++ {
					if items[j].Typ == RIGHT_PAREN {
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

		case IDENTIFIER, METRIC_IDENTIFIER:
			// Case-1: {__name__="metric_name"} => metric_name
			// Case-2: {__name__="metric_name", job="prettier"} => metric_name{job="prettier"}
			itr := skipComments(items, i)
			if i < 1 || item.Val != "__name__" || items[itr+2].Typ != STRING {
				continue
			}
			var (
				leftBraceIndex  = itemNotFound
				rightBraceIndex = itemNotFound
				labelValItem    = items[itr+2]
				metricName      = labelValItem.Val[1 : len(labelValItem.Val)-1] // Trim inverted-commas.
				tmp             []Item
				skipBraces      bool // For handling metric_name{} -> metric_name.
			)
			if isMetricNameNotAtomic(metricName) {
				continue
			}
			for backScanIndex := itr; backScanIndex >= 0; backScanIndex-- {
				if items[backScanIndex].Typ == LEFT_BRACE {
					leftBraceIndex = backScanIndex
					break
				}
			}
			for forwardScanIndex := itr; forwardScanIndex < len(items); forwardScanIndex++ {
				if items[forwardScanIndex].Typ == RIGHT_BRACE {
					rightBraceIndex = forwardScanIndex
					break
				}
			}
			if leftBraceIndex == itemNotFound || rightBraceIndex == itemNotFound {
				continue
			}
			itr = skipComments(items, itr)
			if items[itr+3].Typ == COMMA {
				skipBraces = rightBraceIndex-5 == skipComments(items, leftBraceIndex)
			} else {
				skipBraces = rightBraceIndex-4 == skipComments(items, leftBraceIndex)
			}
			identifierItem := Item{Typ: IDENTIFIER, Val: metricName, Pos: 0}
			// The code below re-orders the lex items in the items array. The re-ordering of
			// lex items must occur only when in a key-value pair, the key is `__name__`
			// and the corresponding value is atomic.
			//
			// The leftBraceIndex and rightBraceIndex that form a range. Since the position of items beyond
			// this range do not change, they are simply copied. However, the items within the range require
			// re-ordering. So, they are added to the lex item slice after appending the identifier (metric_name)
			// to the lex item slice.
			//
			// Also, we need to avoid `metric_name{}` case while re-ordering a `{__name__="metric_name"}`.
			// Hence, if the indexes of left and right braces are next to each other, we skip
			// appending them (braces) to the items slice.
			for j := range items {
				if j <= rightBraceIndex && j >= leftBraceIndex {
					// Before printing the left_brace, we print the metric_name. After this, we check for metric_name{}
					// situation. If __name__ is the only label_matcher, we skip printing '{' and '}'.
					if j == leftBraceIndex {
						tmp = append(tmp, identifierItem)
						if !skipBraces {
							tmp = append(tmp, items[j])
						}
						continue
					} else if items[itr+3].Typ == COMMA && j == itr+3 || j >= itr && j < itr+3 {
						continue
					}
					if skipBraces && (items[j].Typ == LEFT_BRACE || items[j].Typ == RIGHT_BRACE) {
						continue
					}
				}
				tmp = append(tmp, items[j])
			}
			items = tmp
		case BY, IGNORING, ON, WITHOUT:
			// Case: sum by() (metric_name) => sum(metric_name)
			var (
				leftParenIndex       = itemNotFound
				rightParenIndex      = itemNotFound
				groupingKeywordIndex = i
				itr                  = skipComments(items, i)
				tempItems            []Item
			)
			for index := itr; index < len(items); index++ {
				if items[index].Typ == LEFT_PAREN {
					leftParenIndex = index
				}
				if items[index].Typ == RIGHT_PAREN {
					rightParenIndex = index
					break
				}
			}
			if leftParenIndex+1 == rightParenIndex || leftParenIndex == rightParenIndex {
				for index := 0; index < len(items); index++ {
					if index >= groupingKeywordIndex && index <= rightParenIndex {
						continue
					}
					tempItems = append(tempItems, items[index])
				}
				items = tempItems
			}
		}
	}
	return p.refreshLexItems(items)
}

func (p *Prettier) prettify(items []Item) (string, error) {
	var (
		it     = itemsIterator{items, -1}
		result = ""
		root   struct {
			node      Node
			hasScalar bool
		}
	)
	for it.next() {
		var (
			item           = it.peek()
			nodeInfo       = &nodeInfo{head: p.Node, columnLimit: 100, item: item, items: items}
			node           = nodeInfo.node()
			nodeSplittable = nodeInfo.violatesColumnLimit()
			// Binary expression use-case.
			hasImmediateScalar               bool
			hasGrouping, aggregationGrouping bool
			// Aggregate expression use-case.
			isAggregation, hasMultiArgumentCalls bool
		)
		if p.pd.isPreviousItemComment {
			p.pd.isPreviousItemComment = false
			result += p.pd.newLine()
		}
		switch node.(type) {
		case *AggregateExpr:
			isAggregation = true
			aggregationGrouping = nodeInfo.has(grouping)
		case *BinaryExpr:
			hasImmediateScalar = nodeInfo.has(scalars)
			hasGrouping = nodeInfo.has(grouping)
			if nodeInfo.baseIndent == 1 {
				root.node = node
				root.hasScalar = hasImmediateScalar
			}
		case *Call:
			hasMultiArgumentCalls = nodeInfo.has(multiArguments)
		}
		if p.pd.isNewLineApplied {
			result += p.pd.pad(nodeInfo.baseIndent)
			p.pd.isNewLineApplied = false
		}

		switch item.Typ {
		case LEFT_PAREN:
			result += item.Val
			if ((nodeSplittable && !hasGrouping) || hasMultiArgumentCalls) && !p.pd.expectAggregationLabels {
				result += p.pd.newLine()
			}
		case RIGHT_PAREN:
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
			if isAggregation && nodeInfo.has(aggregateParent) && nodeInfo.isLastItem(item) {
				result += p.pd.newLine()
			}
		case LEFT_BRACE:
			result += item.Val
			p.pd.isFirstLabel = true
			if nodeSplittable {
				// This situation arises only in vector selectors that violate column limit and have labels.
				result += p.pd.newLine()
				p.pd.labelsSplittable = true
			}
		case RIGHT_BRACE:
			if p.pd.labelsSplittable {
				if it.prev().Typ != COMMA {
					// Edge-case: if the labels are multi-line split, but do not have
					// a pre-applied comma.
					result += ","
				}
				result += p.pd.newLine() + p.pd.pad(nodeInfo.getBaseIndent(item))
				p.pd.labelsSplittable = false
				p.pd.isNewLineApplied = false
			}
			p.pd.isFirstLabel = false
			result += item.Val
		case IDENTIFIER, METRIC_IDENTIFIER:
			if p.pd.labelsSplittable {
				if p.pd.isFirstLabel {
					p.pd.isFirstLabel = false
					result += p.pd.pad(1)
				} else {
					result += " "
				}
				result += item.Val
			} else {
				result += item.Val
			}
		case STRING, NUMBER:
			result += item.Val
		case SUM, BOTTOMK, COUNT_VALUES, COUNT, GROUP, MAX, MIN,
			QUANTILE, STDVAR, STDDEV, TOPK, AVG:
			// Aggregations.
			if nodeInfo.has(aggregateParent) {
				result = p.pd.removePreviousBlank(result)
				result += p.pd.newLine() + p.pd.pad(nodeInfo.getBaseIndent(item))
				p.pd.isNewLineApplied = false
			}
			result += item.Val
			if aggregationGrouping {
				result += " "
			}
		case EQL, EQL_REGEX, NEQ, NEQ_REGEX, DURATION, COLON,
			LEFT_BRACKET, RIGHT_BRACKET:
			// Comparison operators.
			result += item.Val
		case ADD, SUB, MUL, DIV, GTE, GTR, LOR, LAND,
			LSS, LTE, LUNLESS, MOD, POW:
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
		case WITHOUT, BY, IGNORING, ON:
			// Keywords.
			result += item.Val
			if item.Typ == BOOL && hasImmediateScalar {
				result += " "
			} else if isNodeType(node, aggregateExpr) {
				p.pd.expectAggregationLabels = true
			}
		case BOOL:
			result += item.Val
			if hasImmediateScalar {
				result += " "
			} else {
				result += p.pd.newLine()
			}
		case OFFSET, GROUP_LEFT, GROUP_RIGHT:
			result += " " + item.Val + " "
		case COMMA:
			result += item.Val
			if (nodeSplittable || hasMultiArgumentCalls) && !(hasGrouping || aggregationGrouping || p.pd.labelsSplittable) {
				result += p.pd.newLine()
			} else if !p.pd.labelsSplittable {
				result += " "
			}
		case COMMENT:
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
	if isNodeType(root.node, binaryExpr) && !root.hasScalar {
		return result, nil
	}
	return strings.TrimSpace(result), nil
}

// refreshLexItems refreshes the contents of the lex slice and the properties
// within it. This is expected to be called after sorting the lexItems in the
// pre-format checks.
func (p *Prettier) refreshLexItems(items []Item) []Item {
	return p.lexItems(stringifyItems(items))
}

// lexItems converts the given expression into a slice of Items.
func (p *Prettier) lexItems(expression string) (items []Item) {
	var (
		l    = Lex(expression)
		item Item
	)
	for l.NextItem(&item); item.Typ != EOF; l.NextItem(&item) {
		items = append(items, item)
	}
	return
}

func (p *Prettier) parseExpr(expression string) error {
	expr, err := ParseExpr(expression)
	if err != nil {
		return errors.Wrap(err, "parse error")
	}
	p.Node = expr
	return nil
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

// skipComments advances the index until a non-comment item is
// encountered.
func skipComments(items []Item, index int) int {
	var i int
	for i = index; i < len(items); i++ {
		if items[i].Typ == COMMENT {
			continue
		}
		break
	}
	return i
}

// stringifyItems returns a standardized string expression
// from slice of items without any trailing whitespaces.
func stringifyItems(items []Item) string {
	var (
		it         = itemsIterator{items, -1}
		item       Item
		expression string
	)
	for it.next() {
		item = it.peek()
		if item.Typ.IsAggregator() {
			expression += item.Val
			continue
		}
		switch item.Typ {
		case EQL, EQL_REGEX, NEQ, NEQ_REGEX, COLON, LEFT_BRACE, LEFT_BRACKET, LEFT_PAREN,
			RIGHT_BRACE, RIGHT_BRACKET, RIGHT_PAREN, STRING, NUMBER, IDENTIFIER, METRIC_IDENTIFIER:
			expression += item.Val
		case COMMA:
			expression += item.Val + " "
		case COMMENT:
			expression += item.Val + "\n"
		default:
			expression += " " + item.Val + " "
		}
	}
	return expression
}

// pad provides an instantaneous padding.
func (pd *padder) pad(iter int) string {
	var pad string
	for i := 1; i <= iter; i++ {
		pad += pd.indent
	}
	return pad
}

func (pd *padder) newLine() string {
	pd.isNewLineApplied = true
	return "\n"
}

func (pd *padder) removePreviousBlank(result string) string {
	if result[len(result)-1] == ' ' {
		result = result[:len(result)-1]
	}
	return result
}

type itemsIterator struct {
	itemsSlice []Item
	index      int
}

// next increments the index and returns the true if there exists an lex item after.
func (it *itemsIterator) next() bool {
	it.index++
	return it.index != len(it.itemsSlice)
}

// peek returns the lex item at the current index.
func (it *itemsIterator) peek() Item {
	return it.itemsSlice[it.index]
}

// prev returns the previous lex item.
func (it *itemsIterator) prev() Item {
	if it.index == 0 {
		return Item{}
	}
	return it.itemsSlice[it.index-1]
}

package prettier

import (
	"fmt"
	"io/ioutil"
	"reflect"
	// "sort"
	"strconv"
	"strings"
	// "time"

	"github.com/pkg/errors"
	// "github.com/prometheus/common/model"
	// "github.com/prometheus/prometheus/pkg/labels"
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
	indent                                         string
	previous, buff, intraNodeIndent                int
	isNewLineApplied, changeNewLine, expectNextRHS bool
	expectNextIdentifier, previousBaseIndent       string
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

func (p *Prettier) prettifyItems(items []parser.Item, index int, result string) string {
	item := items[index]
	value := item.Val
	switch item.Typ {
	case parser.LEFT_PAREN:
		result += p.pd.apply() + value + "\n" + p.pd.inc(1).resume()
	case parser.RIGHT_PAREN:
		result += "\n" + p.pd.dec(1).apply() + value
	case parser.LEFT_BRACE, parser.LEFT_BRACKET:
		result += value + "\n" + p.pd.inc(1).resume()
	case parser.RIGHT_BRACE, parser.RIGHT_BRACKET:
		result += p.pd.dec(1).apply() + value
	case parser.IDENTIFIER, parser.DURATION, parser.NUMBER:
		result += p.pd.apply() + value
	case parser.STRING:
		result += value + ",\n"
	case parser.SUM:
		result += p.pd.apply() + value
	case parser.COLON, parser.EQL, parser.EQL_REGEX, parser.NEQ, parser.NEQ_REGEX:
		result += value
	case parser.ADD, parser.SUB, parser.DIV, parser.MUL:
		result += "\n" + p.pd.dec(1).apply() + value + "\n" + p.pd.inc(1).resume()
	case parser.GTR, parser.BOOL:
		if items[index+1].Typ == parser.BOOL {
			index++
			result += "\n" + p.pd.dec(1).apply() + value + " " + items[index].Val + "\n" + p.pd.inc(1).resume()
		} else {
			result += "\n" + p.pd.dec(1).apply() + value + "\n" + p.pd.inc(1).resume()
		}
	case parser.COMMENT:
		result += p.pd.apply() + value + "\n"

	case parser.BLANK:
	}
	if item.Typ == parser.EOF || len(items) == index+1 {
		return p.removeTrailingLines(result)
	}
	return p.prettifyItems(items, index+1, result)
}

var exmpt = []uint{
	parser.LEFT_BRACE, parser.EQL,
}

func (p *Prettier) prettify(items []parser.Item, index int, result string) string {
	item := items[index]
	fmt.Println("item => ", item.String())
	if !p.exmptBaseIndent(item) {
		baseIndent := p.getBaseIndent(item, p.Node)
		if baseIndent != p.pd.previousBaseIndent { // denotes change of node
			result += "\n"
		}
		result += baseIndent
		p.pd.previousBaseIndent = baseIndent
	}

	switch item.Typ {
	case parser.LEFT_PAREN:
		result += item.Val
	case parser.LEFT_BRACE:
		result += item.Val + p.pd.insertNewLine()
		p.pd.expectNextIdentifier = "label"
	case parser.RIGHT_BRACE:
		result += p.pd.previousBaseIndent + item.Val
		p.pd.expectNextIdentifier = ""
	case parser.IDENTIFIER:
		if p.pd.expectNextIdentifier == "label" {
			result += p.pd.apply()
		}
		result += item.Val
	case parser.STRING:
		result += item.Val + ",\n"
	case parser.RIGHT_PAREN:
		result += p.pd.dec(1).apply() + item.Val
		p.pd.expectNextIdentifier = ""
	case parser.EQL:
		result += item.Val
	case parser.ADD:
		result += item.Val
	}
	if index+1 == len(items) {
		return result
	}
	return p.prettify(items, index+1, result)
}

func (p *Prettier) exmptBaseIndent(item parser.Item) bool {
	switch item.Typ {
	case parser.EQL, parser.LEFT_BRACE, parser.RIGHT_BRACE, parser.STRING, parser.COMMA:
		return true
	}
	return false
}

func (p *Prettier) getBaseIndent(item parser.Item, node parser.Expr) string {
	indent := 0
	head := node
	var tmp parser.Expr
	expectNextRHS := false
	round := false
	fmt.Println("+++++++++++++++++++++++++++")
	fmt.Println("item => ", item.String(), " ", item.Typ.IsOperator())
	if head == nil {
		return ""
	}
	for {
		if head != nil {
			fmt.Println("head is => ", head, " ", reflect.TypeOf(head))
			// break
		}
		//
		indent++
		fmt.Println(head == nil, " ", expectNextRHS, " ", !round)
		if head != nil && p.isItemWithinNode(head, item) {
			fmt.Println("item within node")
			switch n := head.(type) {
			case *parser.ParenExpr:
				head = n.Expr
			case *parser.VectorSelector:
				head = nil
			case *parser.BinaryExpr:
				head = n.LHS
				tmp = n.RHS
				expectNextRHS = true
			}
			continue
		} else if expectNextRHS && head != nil && !item.Typ.IsOperator() {
			expectNextRHS = false
			head = tmp
			indent-- // decrease indent otherwise it will signify a next level node.
			continue
		}
		indent--
		fmt.Println("sending indent as ", indent)
		fmt.Println("-----------------------------")
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
					// fmt.Printf("%v\n", expr)
					// formattedExpr, err := p.Prettify(expr, reflect.TypeOf(""), 0, "")
					// if err != nil {
					// 	return []error{errors.Wrap(err, "prettier error")}
					// }
					// fmt.Println("raw\n", formattedExpr)
					// rules.Expr.SetString(formattedExpr)
					res := p.lexItems(exprStr)
					fmt.Println(res)
					for i := 0; i < len(res); i++ {
						fmt.Println(res[i].Typ, " ", res[i].Val)
					}
					p.pd.buff = 1
					formattedExpr := p.prettify(res, 0, "")
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
	fmt.Println("=======", pad, "===========")
	return pad
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

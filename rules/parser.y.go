//line parser.y:15
package rules

import __yyfmt__ "fmt"

//line parser.y:15
import (
	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
)

//line parser.y:25
type yySymType struct {
	yys            int
	num            clientmodel.SampleValue
	str            string
	ruleNode       ast.Node
	ruleNodeSlice  []ast.Node
	boolean        bool
	labelNameSlice clientmodel.LabelNames
	labelSet       clientmodel.LabelSet
	labelMatcher   *metric.LabelMatcher
	labelMatchers  metric.LabelMatchers
}

const START_RULES = 57346
const START_EXPRESSION = 57347
const IDENTIFIER = 57348
const STRING = 57349
const DURATION = 57350
const METRICNAME = 57351
const NUMBER = 57352
const PERMANENT = 57353
const GROUP_OP = 57354
const KEEPING_EXTRA = 57355
const AGGR_OP = 57356
const CMP_OP = 57357
const ADDITIVE_OP = 57358
const MULT_OP = 57359
const ALERT = 57360
const IF = 57361
const FOR = 57362
const WITH = 57363
const SUMMARY = 57364
const DESCRIPTION = 57365

var yyToknames = []string{
	"START_RULES",
	"START_EXPRESSION",
	"IDENTIFIER",
	"STRING",
	"DURATION",
	"METRICNAME",
	"NUMBER",
	"PERMANENT",
	"GROUP_OP",
	"KEEPING_EXTRA",
	"AGGR_OP",
	"CMP_OP",
	"ADDITIVE_OP",
	"MULT_OP",
	"ALERT",
	"IF",
	"FOR",
	"WITH",
	"SUMMARY",
	"DESCRIPTION",
	" =",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line parser.y:250

//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	1, 1,
	-2, 10,
}

const yyNprod = 50
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 137

var yyAct = []int{

	56, 72, 50, 53, 30, 24, 6, 20, 49, 59,
	22, 10, 51, 18, 13, 12, 21, 19, 20, 11,
	18, 85, 36, 37, 38, 21, 19, 20, 81, 82,
	8, 18, 52, 7, 48, 66, 21, 19, 20, 87,
	18, 10, 51, 31, 13, 12, 60, 55, 28, 11,
	65, 18, 21, 19, 20, 57, 21, 19, 20, 29,
	8, 74, 23, 7, 62, 40, 39, 18, 73, 77,
	76, 18, 80, 75, 10, 19, 20, 13, 12, 79,
	86, 78, 11, 64, 89, 63, 41, 40, 71, 18,
	46, 25, 93, 8, 44, 27, 7, 83, 69, 96,
	94, 91, 58, 43, 9, 17, 54, 31, 92, 35,
	33, 45, 16, 13, 97, 95, 90, 61, 73, 88,
	32, 68, 25, 34, 2, 3, 14, 5, 4, 1,
	42, 84, 15, 26, 70, 67, 47,
}
var yyPact = []int{

	120, -1000, -1000, 68, 94, -1000, 41, 68, 116, 70,
	20, 31, -1000, -1000, -1000, 104, 117, -1000, 101, 68,
	68, 68, 37, 60, -1000, 79, -1000, 85, 5, 68,
	93, 19, 30, -1000, 83, -22, -10, -17, 59, -1000,
	116, -1000, 110, -1000, -1000, -1000, 38, 56, -1000, -1000,
	41, -1000, 21, 7, -1000, 115, 74, 62, 68, -1000,
	-1000, -1000, -1000, -1000, 35, 95, 68, 52, -1000, 68,
	2, -1000, -1000, 73, 1, -1000, 93, 10, -1000, 113,
	41, -1000, 112, 109, 80, 100, -1000, -1000, -1000, -1000,
	-1000, 30, -1000, 78, 108, 76, 107, -1000,
}
var yyPgo = []int{

	0, 136, 135, 4, 1, 134, 0, 5, 62, 133,
	2, 8, 132, 3, 131, 104, 130, 129, 128, 127,
	126,
}
var yyR1 = []int{

	0, 17, 17, 18, 18, 19, 20, 20, 14, 14,
	12, 12, 15, 15, 6, 6, 6, 5, 5, 4,
	9, 9, 9, 8, 8, 7, 16, 16, 10, 10,
	10, 10, 10, 10, 10, 10, 10, 10, 10, 10,
	13, 13, 3, 3, 2, 2, 1, 1, 11, 11,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 11, 0, 2,
	0, 1, 1, 1, 0, 3, 2, 1, 3, 3,
	0, 2, 3, 1, 3, 3, 1, 1, 3, 3,
	2, 4, 3, 4, 6, 6, 3, 3, 3, 1,
	0, 1, 0, 4, 1, 3, 1, 3, 1, 1,
}
var yyChk = []int{

	-1000, -17, 4, 5, -18, -19, -10, 28, 25, -15,
	6, 14, 10, 9, -20, -12, 18, 11, 30, 16,
	17, 15, -10, -8, -7, 6, -9, 25, 28, 28,
	-3, 12, -15, 6, 6, 8, -10, -10, -10, 29,
	27, 26, -16, 24, 15, 26, -8, -1, 29, -11,
	-10, 7, -10, -13, 13, 28, -6, 25, 19, 31,
	-7, 7, 26, 29, 27, 29, 28, -2, 6, 24,
	-5, 26, -4, 6, -10, -11, -3, -10, 29, 27,
	-10, 26, 27, 24, -14, 20, -13, 29, 6, -4,
	7, 21, 8, -6, 22, 7, 23, 7,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 0, 20,
	13, 42, 39, 12, 4, 0, 0, 11, 0, 0,
	0, 0, 0, 0, 23, 0, 30, 0, 0, 0,
	40, 0, 14, 13, 0, 0, 36, 37, 38, 28,
	0, 29, 0, 26, 27, 21, 0, 0, 32, 46,
	48, 49, 0, 0, 41, 0, 0, 0, 0, 33,
	24, 25, 22, 31, 0, 42, 0, 0, 44, 0,
	0, 16, 17, 0, 8, 47, 40, 0, 43, 0,
	6, 15, 0, 0, 0, 0, 34, 35, 45, 18,
	19, 14, 9, 0, 0, 0, 0, 7,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	28, 29, 3, 3, 27, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 24, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 30, 3, 31, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 25, 3, 26,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23,
}
var yyTok3 = []int{
	0,
}

//line yaccpar:1

/*	parser for yacc output	*/

var yyDebug = 0

type yyLexer interface {
	Lex(lval *yySymType) int
	Error(s string)
}

const yyFlag = -1000

func yyTokname(c int) string {
	// 4 is TOKSTART above
	if c >= 4 && c-4 < len(yyToknames) {
		if yyToknames[c-4] != "" {
			return yyToknames[c-4]
		}
	}
	return __yyfmt__.Sprintf("tok-%v", c)
}

func yyStatname(s int) string {
	if s >= 0 && s < len(yyStatenames) {
		if yyStatenames[s] != "" {
			return yyStatenames[s]
		}
	}
	return __yyfmt__.Sprintf("state-%v", s)
}

func yylex1(lex yyLexer, lval *yySymType) int {
	c := 0
	char := lex.Lex(lval)
	if char <= 0 {
		c = yyTok1[0]
		goto out
	}
	if char < len(yyTok1) {
		c = yyTok1[char]
		goto out
	}
	if char >= yyPrivate {
		if char < yyPrivate+len(yyTok2) {
			c = yyTok2[char-yyPrivate]
			goto out
		}
	}
	for i := 0; i < len(yyTok3); i += 2 {
		c = yyTok3[i+0]
		if c == char {
			c = yyTok3[i+1]
			goto out
		}
	}

out:
	if c == 0 {
		c = yyTok2[1] /* unknown char */
	}
	if yyDebug >= 3 {
		__yyfmt__.Printf("lex %s(%d)\n", yyTokname(c), uint(char))
	}
	return c
}

func yyParse(yylex yyLexer) int {
	var yyn int
	var yylval yySymType
	var yyVAL yySymType
	yyS := make([]yySymType, yyMaxDepth)

	Nerrs := 0   /* number of errors */
	Errflag := 0 /* error recovery flag */
	yystate := 0
	yychar := -1
	yyp := -1
	goto yystack

ret0:
	return 0

ret1:
	return 1

yystack:
	/* put a state and value onto the stack */
	if yyDebug >= 4 {
		__yyfmt__.Printf("char %v in %v\n", yyTokname(yychar), yyStatname(yystate))
	}

	yyp++
	if yyp >= len(yyS) {
		nyys := make([]yySymType, len(yyS)*2)
		copy(nyys, yyS)
		yyS = nyys
	}
	yyS[yyp] = yyVAL
	yyS[yyp].yys = yystate

yynewstate:
	yyn = yyPact[yystate]
	if yyn <= yyFlag {
		goto yydefault /* simple state */
	}
	if yychar < 0 {
		yychar = yylex1(yylex, &yylval)
	}
	yyn += yychar
	if yyn < 0 || yyn >= yyLast {
		goto yydefault
	}
	yyn = yyAct[yyn]
	if yyChk[yyn] == yychar { /* valid shift */
		yychar = -1
		yyVAL = yylval
		yystate = yyn
		if Errflag > 0 {
			Errflag--
		}
		goto yystack
	}

yydefault:
	/* default state action */
	yyn = yyDef[yystate]
	if yyn == -2 {
		if yychar < 0 {
			yychar = yylex1(yylex, &yylval)
		}

		/* look through exception table */
		xi := 0
		for {
			if yyExca[xi+0] == -1 && yyExca[xi+1] == yystate {
				break
			}
			xi += 2
		}
		for xi += 2; ; xi += 2 {
			yyn = yyExca[xi+0]
			if yyn < 0 || yyn == yychar {
				break
			}
		}
		yyn = yyExca[xi+1]
		if yyn < 0 {
			goto ret0
		}
	}
	if yyn == 0 {
		/* error ... attempt to resume parsing */
		switch Errflag {
		case 0: /* brand new error */
			yylex.Error("syntax error")
			Nerrs++
			if yyDebug >= 1 {
				__yyfmt__.Printf("%s", yyStatname(yystate))
				__yyfmt__.Printf(" saw %s\n", yyTokname(yychar))
			}
			fallthrough

		case 1, 2: /* incompletely recovered error ... try again */
			Errflag = 3

			/* find a state where "error" is a legal shift action */
			for yyp >= 0 {
				yyn = yyPact[yyS[yyp].yys] + yyErrCode
				if yyn >= 0 && yyn < yyLast {
					yystate = yyAct[yyn] /* simulate a shift of "error" */
					if yyChk[yystate] == yyErrCode {
						goto yystack
					}
				}

				/* the current p has no shift on "error", pop stack */
				if yyDebug >= 2 {
					__yyfmt__.Printf("error recovery pops state %d\n", yyS[yyp].yys)
				}
				yyp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if yyDebug >= 2 {
				__yyfmt__.Printf("error recovery discards %s\n", yyTokname(yychar))
			}
			if yychar == yyEofCode {
				goto ret1
			}
			yychar = -1
			goto yynewstate /* try again in the same state */
		}
	}

	/* reduction by production yyn */
	if yyDebug >= 2 {
		__yyfmt__.Printf("reduce %v in:\n\t%v\n", yyn, yyStatname(yystate))
	}

	yynt := yyn
	yypt := yyp
	_ = yypt // guard against "declared and not used"

	yyp -= yyR2[yyn]
	yyVAL = yyS[yyp+1]

	/* consult goto table to find next state */
	yyn = yyR1[yyn]
	yyg := yyPgo[yyn]
	yyj := yyg + yyS[yyp].yys + 1

	if yyj >= yyLast {
		yystate = yyAct[yyg]
	} else {
		yystate = yyAct[yyj]
		if yyChk[yystate] != -yyn {
			yystate = yyAct[yyg]
		}
	}
	// dummy call; replaced with literal code
	switch yynt {

	case 5:
		//line parser.y:74
		{
			yylex.(*RulesLexer).parsedExpr = yyS[yypt-0].ruleNode
		}
	case 6:
		//line parser.y:79
		{
			rule, err := CreateRecordingRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
			yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
		}
	case 7:
		//line parser.y:85
		{
			rule, err := CreateAlertingRule(yyS[yypt-9].str, yyS[yypt-7].ruleNode, yyS[yypt-6].str, yyS[yypt-4].labelSet, yyS[yypt-2].str, yyS[yypt-0].str)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
			yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
		}
	case 8:
		//line parser.y:93
		{
			yyVAL.str = "0s"
		}
	case 9:
		//line parser.y:95
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 10:
		//line parser.y:99
		{
			yyVAL.boolean = false
		}
	case 11:
		//line parser.y:101
		{
			yyVAL.boolean = true
		}
	case 12:
		//line parser.y:105
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 13:
		//line parser.y:107
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 14:
		//line parser.y:111
		{
			yyVAL.labelSet = clientmodel.LabelSet{}
		}
	case 15:
		//line parser.y:113
		{
			yyVAL.labelSet = yyS[yypt-1].labelSet
		}
	case 16:
		//line parser.y:115
		{
			yyVAL.labelSet = clientmodel.LabelSet{}
		}
	case 17:
		//line parser.y:118
		{
			yyVAL.labelSet = yyS[yypt-0].labelSet
		}
	case 18:
		//line parser.y:120
		{
			for k, v := range yyS[yypt-0].labelSet {
				yyVAL.labelSet[k] = v
			}
		}
	case 19:
		//line parser.y:124
		{
			yyVAL.labelSet = clientmodel.LabelSet{clientmodel.LabelName(yyS[yypt-2].str): clientmodel.LabelValue(yyS[yypt-0].str)}
		}
	case 20:
		//line parser.y:128
		{
			yyVAL.labelMatchers = metric.LabelMatchers{}
		}
	case 21:
		//line parser.y:130
		{
			yyVAL.labelMatchers = metric.LabelMatchers{}
		}
	case 22:
		//line parser.y:132
		{
			yyVAL.labelMatchers = yyS[yypt-1].labelMatchers
		}
	case 23:
		//line parser.y:136
		{
			yyVAL.labelMatchers = metric.LabelMatchers{yyS[yypt-0].labelMatcher}
		}
	case 24:
		//line parser.y:138
		{
			yyVAL.labelMatchers = append(yyVAL.labelMatchers, yyS[yypt-0].labelMatcher)
		}
	case 25:
		//line parser.y:142
		{
			var err error
			yyVAL.labelMatcher, err = newLabelMatcher(yyS[yypt-1].str, clientmodel.LabelName(yyS[yypt-2].str), clientmodel.LabelValue(yyS[yypt-0].str))
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 26:
		//line parser.y:150
		{
			yyVAL.str = "="
		}
	case 27:
		//line parser.y:152
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 28:
		//line parser.y:156
		{
			yyVAL.ruleNode = yyS[yypt-1].ruleNode
		}
	case 29:
		//line parser.y:158
		{
			yyVAL.ruleNode = ast.NewVectorSelector(yyS[yypt-1].labelMatchers)
		}
	case 30:
		//line parser.y:160
		{
			var err error
			m, err := metric.NewLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, clientmodel.LabelValue(yyS[yypt-1].str))
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
			yyS[yypt-0].labelMatchers = append(yyS[yypt-0].labelMatchers, m)
			yyVAL.ruleNode = ast.NewVectorSelector(yyS[yypt-0].labelMatchers)
		}
	case 31:
		//line parser.y:168
		{
			var err error
			yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 32:
		//line parser.y:174
		{
			var err error
			yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 33:
		//line parser.y:180
		{
			var err error
			yyVAL.ruleNode, err = NewMatrixSelector(yyS[yypt-3].ruleNode, yyS[yypt-1].str)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 34:
		//line parser.y:186
		{
			var err error
			yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-5].str, yyS[yypt-3].ruleNode, yyS[yypt-1].labelNameSlice, yyS[yypt-0].boolean)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 35:
		//line parser.y:192
		{
			var err error
			yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-5].str, yyS[yypt-1].ruleNode, yyS[yypt-4].labelNameSlice, yyS[yypt-3].boolean)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 36:
		//line parser.y:200
		{
			var err error
			yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 37:
		//line parser.y:206
		{
			var err error
			yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 38:
		//line parser.y:212
		{
			var err error
			yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 39:
		//line parser.y:218
		{
			yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)
		}
	case 40:
		//line parser.y:222
		{
			yyVAL.boolean = false
		}
	case 41:
		//line parser.y:224
		{
			yyVAL.boolean = true
		}
	case 42:
		//line parser.y:228
		{
			yyVAL.labelNameSlice = clientmodel.LabelNames{}
		}
	case 43:
		//line parser.y:230
		{
			yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice
		}
	case 44:
		//line parser.y:234
		{
			yyVAL.labelNameSlice = clientmodel.LabelNames{clientmodel.LabelName(yyS[yypt-0].str)}
		}
	case 45:
		//line parser.y:236
		{
			yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, clientmodel.LabelName(yyS[yypt-0].str))
		}
	case 46:
		//line parser.y:240
		{
			yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode}
		}
	case 47:
		//line parser.y:242
		{
			yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode)
		}
	case 48:
		//line parser.y:246
		{
			yyVAL.ruleNode = yyS[yypt-0].ruleNode
		}
	case 49:
		//line parser.y:248
		{
			yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str)
		}
	}
	goto yystack /* stack new state and value */
}

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
	vectorMatching *vectorMatching
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
const OFFSET = 57356
const MATCH_OP = 57357
const AGGR_OP = 57358
const CMP_OP = 57359
const ADDITIVE_OP = 57360
const MULT_OP = 57361
const MATCH_MOD = 57362
const ALERT = 57363
const IF = 57364
const FOR = 57365
const WITH = 57366
const SUMMARY = 57367
const DESCRIPTION = 57368

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
	"OFFSET",
	"MATCH_OP",
	"AGGR_OP",
	"CMP_OP",
	"ADDITIVE_OP",
	"MULT_OP",
	"MATCH_MOD",
	"ALERT",
	"IF",
	"FOR",
	"WITH",
	"SUMMARY",
	"DESCRIPTION",
	"'='",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line parser.y:279

//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	1, 1,
	-2, 10,
}

const yyNprod = 55
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 155

var yyAct = []int{

	76, 59, 81, 56, 53, 52, 30, 46, 6, 24,
	10, 54, 22, 13, 12, 21, 19, 20, 19, 20,
	11, 62, 21, 19, 20, 20, 18, 90, 96, 111,
	99, 18, 8, 18, 55, 7, 51, 60, 18, 18,
	90, 63, 97, 65, 66, 21, 19, 20, 107, 75,
	68, 67, 10, 54, 64, 13, 12, 21, 19, 20,
	74, 18, 11, 31, 58, 85, 83, 90, 94, 89,
	84, 27, 40, 18, 8, 28, 23, 7, 112, 86,
	88, 87, 29, 91, 21, 19, 20, 82, 73, 10,
	72, 98, 13, 12, 92, 93, 101, 71, 41, 11,
	18, 42, 41, 25, 49, 106, 45, 78, 109, 108,
	80, 8, 103, 36, 7, 61, 44, 17, 105, 37,
	9, 47, 57, 31, 104, 33, 48, 16, 13, 70,
	35, 113, 110, 102, 38, 39, 32, 69, 77, 82,
	100, 25, 34, 2, 3, 14, 5, 4, 1, 43,
	95, 15, 26, 79, 50,
}
var yyPact = []int{

	139, -1000, -1000, 83, 106, -1000, 67, 83, 135, 43,
	44, 51, -1000, -1000, -1000, 119, 136, -1000, 122, 104,
	104, 104, 40, 72, -1000, 89, 107, 97, 4, 83,
	109, 33, 9, -1000, 93, -13, 83, 23, 83, 83,
	-1000, 135, 107, 130, -1000, -1000, -1000, 121, -1000, 68,
	58, -1000, -1000, 67, -1000, 28, 18, -1000, 132, 80,
	81, 83, 107, 6, 132, -7, 0, -1000, -1000, -1000,
	-1000, -1000, -1000, 46, 111, 83, 37, -1000, 83, 65,
	-1000, -1000, 41, 5, -1000, 10, -1000, 109, -2, -1000,
	134, 67, -1000, 133, 126, 88, 116, 98, -1000, -1000,
	-1000, -1000, -1000, 9, -1000, 17, 84, 132, 125, -3,
	52, -1000, 124, -1000,
}
var yyPgo = []int{

	0, 154, 0, 6, 2, 153, 1, 9, 76, 152,
	113, 4, 5, 151, 3, 150, 120, 149, 7, 148,
	147, 146, 145,
}
var yyR1 = []int{

	0, 19, 19, 20, 20, 21, 22, 22, 15, 15,
	13, 13, 16, 16, 6, 6, 6, 5, 5, 4,
	9, 9, 9, 8, 8, 7, 17, 17, 18, 18,
	11, 11, 11, 11, 11, 11, 11, 11, 11, 11,
	11, 11, 14, 14, 10, 10, 10, 3, 3, 2,
	2, 1, 1, 12, 12,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 11, 0, 2,
	0, 1, 1, 1, 0, 3, 2, 1, 3, 3,
	0, 2, 3, 1, 3, 3, 1, 1, 0, 2,
	3, 4, 3, 4, 3, 5, 6, 6, 4, 4,
	4, 1, 0, 1, 0, 4, 8, 0, 4, 1,
	3, 1, 3, 1, 1,
}
var yyChk = []int{

	-1000, -19, 4, 5, -20, -21, -11, 31, 28, -16,
	6, 16, 10, 9, -22, -13, 21, 11, 33, 18,
	19, 17, -11, -8, -7, 6, -9, 28, 31, 31,
	-3, 12, -16, 6, 6, 8, -10, 15, -10, -10,
	32, 30, 29, -17, 27, 17, -18, 14, 29, -8,
	-1, 32, -12, -11, 7, -11, -14, 13, 31, -6,
	28, 22, 34, -11, 31, -11, -11, -7, -18, 7,
	8, 29, 32, 30, 32, 31, -2, 6, 27, -5,
	29, -4, 6, -11, -18, -2, -12, -3, -11, 32,
	30, -11, 29, 30, 27, -15, 23, 32, -14, 32,
	6, -4, 7, 24, 8, 20, -6, 31, 25, -2,
	7, 32, 26, 7,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 0, 20,
	13, 47, 41, 12, 4, 0, 0, 11, 0, 44,
	44, 44, 0, 0, 23, 0, 28, 0, 0, 0,
	42, 0, 14, 13, 0, 0, 0, 0, 0, 0,
	30, 0, 28, 0, 26, 27, 32, 0, 21, 0,
	0, 34, 51, 53, 54, 0, 0, 43, 0, 0,
	0, 0, 28, 38, 0, 39, 40, 24, 31, 25,
	29, 22, 33, 0, 47, 0, 0, 49, 0, 0,
	16, 17, 0, 8, 35, 0, 52, 42, 0, 48,
	0, 6, 15, 0, 0, 0, 0, 45, 36, 37,
	50, 18, 19, 14, 9, 0, 0, 0, 0, 0,
	0, 46, 0, 7,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	31, 32, 3, 3, 30, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 27, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 33, 3, 34, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 28, 3, 29,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22, 23, 24, 25, 26,
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
		//line parser.y:76
		{
			yylex.(*RulesLexer).parsedExpr = yyS[yypt-0].ruleNode
		}
	case 6:
		//line parser.y:81
		{
			rule, err := CreateRecordingRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
			yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
		}
	case 7:
		//line parser.y:87
		{
			rule, err := CreateAlertingRule(yyS[yypt-9].str, yyS[yypt-7].ruleNode, yyS[yypt-6].str, yyS[yypt-4].labelSet, yyS[yypt-2].str, yyS[yypt-0].str)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
			yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
		}
	case 8:
		//line parser.y:95
		{
			yyVAL.str = "0s"
		}
	case 9:
		//line parser.y:97
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 10:
		//line parser.y:101
		{
			yyVAL.boolean = false
		}
	case 11:
		//line parser.y:103
		{
			yyVAL.boolean = true
		}
	case 12:
		//line parser.y:107
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 13:
		//line parser.y:109
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 14:
		//line parser.y:113
		{
			yyVAL.labelSet = clientmodel.LabelSet{}
		}
	case 15:
		//line parser.y:115
		{
			yyVAL.labelSet = yyS[yypt-1].labelSet
		}
	case 16:
		//line parser.y:117
		{
			yyVAL.labelSet = clientmodel.LabelSet{}
		}
	case 17:
		//line parser.y:120
		{
			yyVAL.labelSet = yyS[yypt-0].labelSet
		}
	case 18:
		//line parser.y:122
		{
			for k, v := range yyS[yypt-0].labelSet {
				yyVAL.labelSet[k] = v
			}
		}
	case 19:
		//line parser.y:126
		{
			yyVAL.labelSet = clientmodel.LabelSet{clientmodel.LabelName(yyS[yypt-2].str): clientmodel.LabelValue(yyS[yypt-0].str)}
		}
	case 20:
		//line parser.y:130
		{
			yyVAL.labelMatchers = metric.LabelMatchers{}
		}
	case 21:
		//line parser.y:132
		{
			yyVAL.labelMatchers = metric.LabelMatchers{}
		}
	case 22:
		//line parser.y:134
		{
			yyVAL.labelMatchers = yyS[yypt-1].labelMatchers
		}
	case 23:
		//line parser.y:138
		{
			yyVAL.labelMatchers = metric.LabelMatchers{yyS[yypt-0].labelMatcher}
		}
	case 24:
		//line parser.y:140
		{
			yyVAL.labelMatchers = append(yyVAL.labelMatchers, yyS[yypt-0].labelMatcher)
		}
	case 25:
		//line parser.y:144
		{
			var err error
			yyVAL.labelMatcher, err = newLabelMatcher(yyS[yypt-1].str, clientmodel.LabelName(yyS[yypt-2].str), clientmodel.LabelValue(yyS[yypt-0].str))
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 26:
		//line parser.y:152
		{
			yyVAL.str = "="
		}
	case 27:
		//line parser.y:154
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 28:
		//line parser.y:158
		{
			yyVAL.str = "0s"
		}
	case 29:
		//line parser.y:160
		{
			yyVAL.str = yyS[yypt-0].str
		}
	case 30:
		//line parser.y:164
		{
			yyVAL.ruleNode = yyS[yypt-1].ruleNode
		}
	case 31:
		//line parser.y:166
		{
			var err error
			yyVAL.ruleNode, err = NewVectorSelector(yyS[yypt-2].labelMatchers, yyS[yypt-0].str)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 32:
		//line parser.y:172
		{
			var err error
			m, err := metric.NewLabelMatcher(metric.Equal, clientmodel.MetricNameLabel, clientmodel.LabelValue(yyS[yypt-2].str))
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
			yyS[yypt-1].labelMatchers = append(yyS[yypt-1].labelMatchers, m)
			yyVAL.ruleNode, err = NewVectorSelector(yyS[yypt-1].labelMatchers, yyS[yypt-0].str)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 33:
		//line parser.y:181
		{
			var err error
			yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 34:
		//line parser.y:187
		{
			var err error
			yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 35:
		//line parser.y:193
		{
			var err error
			yyVAL.ruleNode, err = NewMatrixSelector(yyS[yypt-4].ruleNode, yyS[yypt-2].str, yyS[yypt-0].str)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 36:
		//line parser.y:199
		{
			var err error
			yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-5].str, yyS[yypt-3].ruleNode, yyS[yypt-1].labelNameSlice, yyS[yypt-0].boolean)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 37:
		//line parser.y:205
		{
			var err error
			yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-5].str, yyS[yypt-1].ruleNode, yyS[yypt-4].labelNameSlice, yyS[yypt-3].boolean)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 38:
		//line parser.y:213
		{
			var err error
			yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-2].str, yyS[yypt-3].ruleNode, yyS[yypt-0].ruleNode, yyS[yypt-1].vectorMatching)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 39:
		//line parser.y:219
		{
			var err error
			yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-2].str, yyS[yypt-3].ruleNode, yyS[yypt-0].ruleNode, yyS[yypt-1].vectorMatching)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 40:
		//line parser.y:225
		{
			var err error
			yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-2].str, yyS[yypt-3].ruleNode, yyS[yypt-0].ruleNode, yyS[yypt-1].vectorMatching)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 41:
		//line parser.y:231
		{
			yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)
		}
	case 42:
		//line parser.y:235
		{
			yyVAL.boolean = false
		}
	case 43:
		//line parser.y:237
		{
			yyVAL.boolean = true
		}
	case 44:
		//line parser.y:241
		{
			yyVAL.vectorMatching = nil
		}
	case 45:
		//line parser.y:243
		{
			var err error
			yyVAL.vectorMatching, err = newVectorMatching("", yyS[yypt-1].labelNameSlice, nil)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 46:
		//line parser.y:249
		{
			var err error
			yyVAL.vectorMatching, err = newVectorMatching(yyS[yypt-3].str, yyS[yypt-5].labelNameSlice, yyS[yypt-1].labelNameSlice)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 47:
		//line parser.y:257
		{
			yyVAL.labelNameSlice = clientmodel.LabelNames{}
		}
	case 48:
		//line parser.y:259
		{
			yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice
		}
	case 49:
		//line parser.y:263
		{
			yyVAL.labelNameSlice = clientmodel.LabelNames{clientmodel.LabelName(yyS[yypt-0].str)}
		}
	case 50:
		//line parser.y:265
		{
			yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, clientmodel.LabelName(yyS[yypt-0].str))
		}
	case 51:
		//line parser.y:269
		{
			yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode}
		}
	case 52:
		//line parser.y:271
		{
			yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode)
		}
	case 53:
		//line parser.y:275
		{
			yyVAL.ruleNode = yyS[yypt-0].ruleNode
		}
	case 54:
		//line parser.y:277
		{
			yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str)
		}
	}
	goto yystack /* stack new state and value */
}

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

//line parser.y:281

//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	1, 1,
	-2, 10,
}

const yyNprod = 56
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 159

var yyAct = []int{

	78, 61, 83, 58, 55, 54, 31, 48, 6, 25,
	20, 21, 23, 21, 10, 56, 64, 14, 12, 10,
	56, 19, 14, 12, 11, 19, 13, 19, 92, 11,
	113, 13, 22, 20, 21, 57, 8, 32, 109, 7,
	53, 8, 77, 65, 7, 67, 68, 101, 19, 22,
	20, 21, 70, 69, 10, 98, 30, 14, 12, 22,
	20, 21, 94, 95, 11, 19, 13, 87, 85, 92,
	96, 99, 86, 84, 76, 19, 8, 66, 60, 7,
	29, 88, 90, 89, 24, 93, 22, 20, 21, 22,
	20, 21, 92, 100, 91, 75, 82, 74, 103, 73,
	43, 42, 19, 44, 43, 19, 26, 108, 62, 47,
	111, 28, 80, 51, 114, 110, 38, 105, 63, 46,
	18, 107, 39, 9, 49, 59, 32, 33, 35, 50,
	17, 14, 106, 72, 37, 115, 112, 104, 40, 41,
	34, 71, 79, 84, 102, 26, 36, 2, 3, 15,
	5, 4, 1, 45, 97, 16, 27, 81, 52,
}
var yyPact = []int{

	143, -1000, -1000, 48, 109, -1000, 72, 48, 139, 83,
	49, 25, -1000, 117, -1000, -1000, 122, 140, -1000, 126,
	107, 107, 107, 69, 74, -1000, 92, 110, 100, 8,
	48, 112, 47, -1000, 80, -1000, 96, -18, 48, 46,
	48, 48, -1000, 139, 110, 134, -1000, -1000, -1000, 125,
	-1000, 70, 65, -1000, -1000, 72, -1000, 42, 11, -1000,
	136, 85, 67, 48, 110, -6, 136, -12, -8, -1000,
	-1000, -1000, -1000, -1000, -1000, 13, 114, 48, 62, -1000,
	48, 33, -1000, -1000, 43, 32, -1000, 39, -1000, 112,
	15, -1000, 138, 72, -1000, 137, 130, 93, 124, 101,
	-1000, -1000, -1000, -1000, -1000, 80, -1000, 7, 90, 136,
	129, -2, 88, -1000, 128, -1000,
}
var yyPgo = []int{

	0, 158, 0, 6, 2, 157, 1, 9, 84, 156,
	116, 4, 5, 155, 3, 154, 123, 153, 7, 152,
	151, 150, 149,
}
var yyR1 = []int{

	0, 19, 19, 20, 20, 21, 22, 22, 15, 15,
	13, 13, 16, 16, 6, 6, 6, 5, 5, 4,
	9, 9, 9, 8, 8, 7, 17, 17, 18, 18,
	11, 11, 11, 11, 11, 11, 11, 11, 11, 11,
	11, 11, 11, 14, 14, 10, 10, 10, 3, 3,
	2, 2, 1, 1, 12, 12,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 11, 0, 2,
	0, 1, 1, 1, 0, 3, 2, 1, 3, 3,
	0, 2, 3, 1, 3, 3, 1, 1, 0, 2,
	3, 4, 3, 4, 3, 5, 6, 6, 4, 4,
	4, 1, 2, 0, 1, 0, 4, 8, 0, 4,
	1, 3, 1, 3, 1, 1,
}
var yyChk = []int{

	-1000, -19, 4, 5, -20, -21, -11, 31, 28, -16,
	6, 16, 10, 18, 9, -22, -13, 21, 11, 33,
	18, 19, 17, -11, -8, -7, 6, -9, 28, 31,
	31, -3, 12, 10, -16, 6, 6, 8, -10, 15,
	-10, -10, 32, 30, 29, -17, 27, 17, -18, 14,
	29, -8, -1, 32, -12, -11, 7, -11, -14, 13,
	31, -6, 28, 22, 34, -11, 31, -11, -11, -7,
	-18, 7, 8, 29, 32, 30, 32, 31, -2, 6,
	27, -5, 29, -4, 6, -11, -18, -2, -12, -3,
	-11, 32, 30, -11, 29, 30, 27, -15, 23, 32,
	-14, 32, 6, -4, 7, 24, 8, 20, -6, 31,
	25, -2, 7, 32, 26, 7,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 0, 20,
	13, 48, 41, 0, 12, 4, 0, 0, 11, 0,
	45, 45, 45, 0, 0, 23, 0, 28, 0, 0,
	0, 43, 0, 42, 14, 13, 0, 0, 0, 0,
	0, 0, 30, 0, 28, 0, 26, 27, 32, 0,
	21, 0, 0, 34, 52, 54, 55, 0, 0, 44,
	0, 0, 0, 0, 28, 38, 0, 39, 40, 24,
	31, 25, 29, 22, 33, 0, 48, 0, 0, 50,
	0, 0, 16, 17, 0, 8, 35, 0, 53, 43,
	0, 49, 0, 6, 15, 0, 0, 0, 0, 46,
	36, 37, 51, 18, 19, 14, 9, 0, 0, 0,
	0, 0, 0, 47, 0, 7,
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
			yyVAL.ruleNode = NewScalarLiteral(yyS[yypt-0].num, "+")
		}
	case 42:
		//line parser.y:233
		{
			yyVAL.ruleNode = NewScalarLiteral(yyS[yypt-0].num, yyS[yypt-1].str)
		}
	case 43:
		//line parser.y:237
		{
			yyVAL.boolean = false
		}
	case 44:
		//line parser.y:239
		{
			yyVAL.boolean = true
		}
	case 45:
		//line parser.y:243
		{
			yyVAL.vectorMatching = nil
		}
	case 46:
		//line parser.y:245
		{
			var err error
			yyVAL.vectorMatching, err = newVectorMatching("", yyS[yypt-1].labelNameSlice, nil)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 47:
		//line parser.y:251
		{
			var err error
			yyVAL.vectorMatching, err = newVectorMatching(yyS[yypt-3].str, yyS[yypt-5].labelNameSlice, yyS[yypt-1].labelNameSlice)
			if err != nil {
				yylex.Error(err.Error())
				return 1
			}
		}
	case 48:
		//line parser.y:259
		{
			yyVAL.labelNameSlice = clientmodel.LabelNames{}
		}
	case 49:
		//line parser.y:261
		{
			yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice
		}
	case 50:
		//line parser.y:265
		{
			yyVAL.labelNameSlice = clientmodel.LabelNames{clientmodel.LabelName(yyS[yypt-0].str)}
		}
	case 51:
		//line parser.y:267
		{
			yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, clientmodel.LabelName(yyS[yypt-0].str))
		}
	case 52:
		//line parser.y:271
		{
			yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode}
		}
	case 53:
		//line parser.y:273
		{
			yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode)
		}
	case 54:
		//line parser.y:277
		{
			yyVAL.ruleNode = yyS[yypt-0].ruleNode
		}
	case 55:
		//line parser.y:279
		{
			yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str)
		}
	}
	goto yystack /* stack new state and value */
}

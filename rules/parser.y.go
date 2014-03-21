
//line parser.y:15
        package rules
import __yyfmt__ "fmt"
//line parser.y:15
		
        import (
          clientmodel "github.com/prometheus/client_golang/model"

          "github.com/prometheus/prometheus/rules/ast"
        )

//line parser.y:24
type yySymType struct {
	yys int
        num clientmodel.SampleValue
        str string
        ruleNode ast.Node
        ruleNodeSlice []ast.Node
        boolean bool
        labelNameSlice clientmodel.LabelNames
        labelSet clientmodel.LabelSet
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

//line parser.y:204


//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	1, 1,
	-2, 10,
}

const yyNprod = 40
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 108

var yyAct = []int{

	22, 40, 41, 36, 19, 46, 6, 17, 9, 42,
	21, 12, 11, 72, 23, 71, 10, 17, 20, 18,
	19, 30, 31, 32, 20, 18, 19, 44, 43, 62,
	7, 39, 52, 17, 65, 20, 18, 19, 25, 17,
	20, 18, 19, 9, 42, 24, 12, 11, 54, 33,
	17, 10, 55, 57, 9, 17, 60, 12, 11, 18,
	19, 51, 10, 50, 37, 7, 73, 70, 47, 48,
	53, 49, 76, 17, 66, 45, 7, 8, 16, 64,
	59, 67, 29, 27, 35, 15, 12, 77, 75, 56,
	74, 69, 26, 37, 28, 2, 3, 13, 5, 4,
	1, 61, 63, 14, 34, 58, 68, 38,
}
var yyPact = []int{

	91, -1000, -1000, 48, 67, -1000, 25, 48, -11, 17,
	10, -1000, -1000, -1000, 77, 88, -1000, 74, 48, 48,
	48, 20, -1000, 58, 2, 48, -11, -1000, 56, -26,
	-13, -23, 43, -1000, 42, -1000, -1000, 47, 34, -1000,
	-1000, 25, -1000, 3, 46, 48, -1000, -1000, 87, 82,
	-1000, 37, 68, 48, 9, -1000, -1000, -1000, 66, 6,
	25, 53, 73, -1000, -1000, 85, -11, -1000, -14, -1000,
	44, -1000, 84, 81, -1000, 49, 80, -1000,
}
var yyPgo = []int{

	0, 107, 106, 105, 3, 104, 0, 2, 1, 103,
	102, 101, 77, 100, 99, 98, 97,
}
var yyR1 = []int{

	0, 13, 13, 14, 14, 15, 16, 16, 11, 11,
	9, 9, 12, 12, 6, 6, 6, 5, 5, 4,
	7, 7, 7, 7, 7, 7, 7, 7, 7, 7,
	10, 10, 3, 3, 2, 2, 1, 1, 8, 8,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 11, 0, 2,
	0, 1, 1, 1, 0, 3, 2, 1, 3, 3,
	3, 2, 4, 3, 4, 6, 3, 3, 3, 1,
	0, 1, 0, 4, 1, 3, 1, 3, 1, 1,
}
var yyChk = []int{

	-1000, -13, 4, 5, -14, -15, -7, 28, -12, 6,
	14, 10, 9, -16, -9, 18, 11, 30, 16, 17,
	15, -7, -6, 25, 28, 28, -12, 6, 6, 8,
	-7, -7, -7, 29, -5, 26, -4, 6, -1, 29,
	-8, -7, 7, -7, -6, 19, 31, 26, 27, 24,
	29, 27, 29, 24, -7, -4, 7, -8, -3, 12,
	-7, -11, 20, -10, 13, 28, 21, 8, -2, 6,
	-6, 29, 27, 22, 6, 7, 23, 7,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 14, 13,
	0, 29, 12, 4, 0, 0, 11, 0, 0, 0,
	0, 0, 21, 0, 0, 0, 14, 13, 0, 0,
	26, 27, 28, 20, 0, 16, 17, 0, 0, 23,
	36, 38, 39, 0, 0, 0, 24, 15, 0, 0,
	22, 0, 32, 0, 8, 18, 19, 37, 30, 0,
	6, 0, 0, 25, 31, 0, 14, 9, 0, 34,
	0, 33, 0, 0, 35, 0, 0, 7,
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
		//line parser.y:69
		{ yylex.(*RulesLexer).parsedExpr = yyS[yypt-0].ruleNode }
	case 6:
		//line parser.y:74
		{
	                       rule, err := CreateRecordingRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 7:
		//line parser.y:80
		{
	                       rule, err := CreateAlertingRule(yyS[yypt-9].str, yyS[yypt-7].ruleNode, yyS[yypt-6].str, yyS[yypt-4].labelSet, yyS[yypt-2].str, yyS[yypt-0].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 8:
		//line parser.y:88
		{ yyVAL.str = "0s" }
	case 9:
		//line parser.y:90
		{ yyVAL.str = yyS[yypt-0].str }
	case 10:
		//line parser.y:94
		{ yyVAL.boolean = false }
	case 11:
		//line parser.y:96
		{ yyVAL.boolean = true }
	case 12:
		//line parser.y:100
		{ yyVAL.str = yyS[yypt-0].str }
	case 13:
		//line parser.y:102
		{ yyVAL.str = yyS[yypt-0].str }
	case 14:
		//line parser.y:106
		{ yyVAL.labelSet = clientmodel.LabelSet{} }
	case 15:
		//line parser.y:108
		{ yyVAL.labelSet = yyS[yypt-1].labelSet  }
	case 16:
		//line parser.y:110
		{ yyVAL.labelSet = clientmodel.LabelSet{} }
	case 17:
		//line parser.y:113
		{ yyVAL.labelSet = yyS[yypt-0].labelSet }
	case 18:
		//line parser.y:115
		{ for k, v := range yyS[yypt-0].labelSet { yyVAL.labelSet[k] = v } }
	case 19:
		//line parser.y:119
		{ yyVAL.labelSet = clientmodel.LabelSet{ clientmodel.LabelName(yyS[yypt-2].str): clientmodel.LabelValue(yyS[yypt-0].str) } }
	case 20:
		//line parser.y:124
		{ yyVAL.ruleNode = yyS[yypt-1].ruleNode }
	case 21:
		//line parser.y:126
		{ yyS[yypt-0].labelSet[clientmodel.MetricNameLabel] = clientmodel.LabelValue(yyS[yypt-1].str); yyVAL.ruleNode = ast.NewVectorSelector(yyS[yypt-0].labelSet) }
	case 22:
		//line parser.y:128
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 23:
		//line parser.y:134
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 24:
		//line parser.y:140
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewMatrixSelector(yyS[yypt-3].ruleNode, yyS[yypt-1].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 25:
		//line parser.y:146
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-5].str, yyS[yypt-3].ruleNode, yyS[yypt-1].labelNameSlice, yyS[yypt-0].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 26:
		//line parser.y:154
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 27:
		//line parser.y:160
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 28:
		//line parser.y:166
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 29:
		//line parser.y:172
		{ yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)}
	case 30:
		//line parser.y:176
		{ yyVAL.boolean = false }
	case 31:
		//line parser.y:178
		{ yyVAL.boolean = true }
	case 32:
		//line parser.y:182
		{ yyVAL.labelNameSlice = clientmodel.LabelNames{} }
	case 33:
		//line parser.y:184
		{ yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice }
	case 34:
		//line parser.y:188
		{ yyVAL.labelNameSlice = clientmodel.LabelNames{clientmodel.LabelName(yyS[yypt-0].str)} }
	case 35:
		//line parser.y:190
		{ yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, clientmodel.LabelName(yyS[yypt-0].str)) }
	case 36:
		//line parser.y:194
		{ yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode} }
	case 37:
		//line parser.y:196
		{ yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode) }
	case 38:
		//line parser.y:200
		{ yyVAL.ruleNode = yyS[yypt-0].ruleNode }
	case 39:
		//line parser.y:202
		{ yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str) }
	}
	goto yystack /* stack new state and value */
}

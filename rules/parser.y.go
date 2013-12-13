
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
const NUMBER = 57351
const PERMANENT = 57352
const GROUP_OP = 57353
const KEEPING_EXTRA = 57354
const AGGR_OP = 57355
const CMP_OP = 57356
const ADDITIVE_OP = 57357
const MULT_OP = 57358
const ALERT = 57359
const IF = 57360
const FOR = 57361
const WITH = 57362
const SUMMARY = 57363
const DESCRIPTION = 57364

var yyToknames = []string{
	"START_RULES",
	"START_EXPRESSION",
	"IDENTIFIER",
	"STRING",
	"DURATION",
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

//line parser.y:197


//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	6, 10,
	-2, 1,
}

const yyNprod = 38
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 103

var yyAct = []int{

	20, 38, 34, 17, 33, 43, 6, 18, 16, 17,
	19, 15, 59, 18, 16, 17, 15, 62, 23, 27,
	28, 29, 15, 46, 47, 41, 40, 49, 15, 22,
	8, 35, 69, 10, 68, 50, 45, 9, 44, 39,
	18, 16, 17, 48, 73, 51, 18, 16, 17, 53,
	52, 7, 32, 57, 30, 15, 8, 35, 37, 10,
	70, 15, 8, 9, 67, 10, 16, 17, 22, 9,
	61, 21, 63, 42, 14, 64, 56, 7, 26, 74,
	15, 13, 72, 7, 54, 71, 66, 39, 25, 24,
	2, 3, 11, 5, 4, 1, 58, 60, 12, 36,
	55, 65, 31,
}
var yyPact = []int{

	86, -1000, -1000, 56, 64, -1000, 32, 56, 44, -9,
	-1000, -1000, 83, 82, -1000, 70, 56, 56, 56, 26,
	-1000, 24, 33, 56, 5, 55, -25, -13, -18, 51,
	-1000, 10, -1000, -1000, 32, -1000, -2, -1000, -1000, 20,
	-1, 12, 56, -1000, -1000, 50, -1000, 81, 77, 65,
	56, -7, -1000, -1000, -1000, 58, -10, 32, 52, 67,
	-1000, -1000, 80, 5, -1000, 6, -1000, 39, -1000, 79,
	75, -1000, 22, 72, -1000,
}
var yyPgo = []int{

	0, 102, 101, 100, 1, 99, 0, 2, 4, 98,
	97, 96, 95, 94, 93, 92,
}
var yyR1 = []int{

	0, 12, 12, 13, 13, 14, 15, 15, 11, 11,
	9, 9, 6, 6, 6, 5, 5, 4, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 10, 10,
	3, 3, 2, 2, 1, 1, 8, 8,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 11, 0, 2,
	0, 1, 0, 3, 2, 1, 3, 3, 3, 2,
	4, 3, 4, 6, 3, 3, 3, 1, 0, 1,
	0, 4, 1, 3, 1, 3, 1, 1,
}
var yyChk = []int{

	-1000, -12, 4, 5, -13, -14, -7, 27, 6, 13,
	9, -15, -9, 17, 10, 29, 15, 16, 14, -7,
	-6, 27, 24, 27, 6, 6, 8, -7, -7, -7,
	28, -1, 28, -8, -7, 7, -5, 25, -4, 6,
	-7, -6, 18, 30, 28, 26, 25, 26, 23, 28,
	23, -7, -8, -4, 7, -3, 11, -7, -11, 19,
	-10, 12, 27, 20, 8, -2, 6, -6, 28, 26,
	21, 6, 7, 22, 7,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 12, 0,
	27, 4, 0, 0, 11, 0, 0, 0, 0, 0,
	19, 0, 0, 0, 12, 0, 0, 24, 25, 26,
	18, 0, 21, 34, 36, 37, 0, 14, 15, 0,
	0, 0, 0, 22, 20, 0, 13, 0, 0, 30,
	0, 8, 35, 16, 17, 28, 0, 6, 0, 0,
	23, 29, 0, 12, 9, 0, 32, 0, 31, 0,
	0, 33, 0, 0, 7,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	27, 28, 3, 3, 26, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 23, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 29, 3, 30, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 24, 3, 25,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19, 20, 21,
	22,
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
		__yyfmt__.Printf("lex %U %s\n", uint(char), yyTokname(c))
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
				__yyfmt__.Printf("saw %s\n", yyTokname(yychar))
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
		//line parser.y:73
		{
	                       rule, err := CreateRecordingRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 7:
		//line parser.y:79
		{
	                       rule, err := CreateAlertingRule(yyS[yypt-9].str, yyS[yypt-7].ruleNode, yyS[yypt-6].str, yyS[yypt-4].labelSet, yyS[yypt-2].str, yyS[yypt-0].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 8:
		//line parser.y:87
		{ yyVAL.str = "0s" }
	case 9:
		//line parser.y:89
		{ yyVAL.str = yyS[yypt-0].str }
	case 10:
		//line parser.y:93
		{ yyVAL.boolean = false }
	case 11:
		//line parser.y:95
		{ yyVAL.boolean = true }
	case 12:
		//line parser.y:99
		{ yyVAL.labelSet = clientmodel.LabelSet{} }
	case 13:
		//line parser.y:101
		{ yyVAL.labelSet = yyS[yypt-1].labelSet  }
	case 14:
		//line parser.y:103
		{ yyVAL.labelSet = clientmodel.LabelSet{} }
	case 15:
		//line parser.y:106
		{ yyVAL.labelSet = yyS[yypt-0].labelSet }
	case 16:
		//line parser.y:108
		{ for k, v := range yyS[yypt-0].labelSet { yyVAL.labelSet[k] = v } }
	case 17:
		//line parser.y:112
		{ yyVAL.labelSet = clientmodel.LabelSet{ clientmodel.LabelName(yyS[yypt-2].str): clientmodel.LabelValue(yyS[yypt-0].str) } }
	case 18:
		//line parser.y:117
		{ yyVAL.ruleNode = yyS[yypt-1].ruleNode }
	case 19:
		//line parser.y:119
		{ yyS[yypt-0].labelSet[clientmodel.MetricNameLabel] = clientmodel.LabelValue(yyS[yypt-1].str); yyVAL.ruleNode = ast.NewVectorLiteral(yyS[yypt-0].labelSet) }
	case 20:
		//line parser.y:121
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 21:
		//line parser.y:127
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 22:
		//line parser.y:133
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewMatrix(yyS[yypt-3].ruleNode, yyS[yypt-1].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 23:
		//line parser.y:139
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-5].str, yyS[yypt-3].ruleNode, yyS[yypt-1].labelNameSlice, yyS[yypt-0].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 24:
		//line parser.y:147
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 25:
		//line parser.y:153
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 26:
		//line parser.y:159
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 27:
		//line parser.y:165
		{ yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)}
	case 28:
		//line parser.y:169
		{ yyVAL.boolean = false }
	case 29:
		//line parser.y:171
		{ yyVAL.boolean = true }
	case 30:
		//line parser.y:175
		{ yyVAL.labelNameSlice = clientmodel.LabelNames{} }
	case 31:
		//line parser.y:177
		{ yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice }
	case 32:
		//line parser.y:181
		{ yyVAL.labelNameSlice = clientmodel.LabelNames{clientmodel.LabelName(yyS[yypt-0].str)} }
	case 33:
		//line parser.y:183
		{ yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, clientmodel.LabelName(yyS[yypt-0].str)) }
	case 34:
		//line parser.y:187
		{ yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode} }
	case 35:
		//line parser.y:189
		{ yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode) }
	case 36:
		//line parser.y:193
		{ yyVAL.ruleNode = yyS[yypt-0].ruleNode }
	case 37:
		//line parser.y:195
		{ yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str) }
	}
	goto yystack /* stack new state and value */
}

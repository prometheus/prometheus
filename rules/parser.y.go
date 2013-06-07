
//line parser.y:15
        package rules
import __yyfmt__ "fmt"
//line parser.y:15
		
        import "github.com/prometheus/prometheus/model"
        import "github.com/prometheus/prometheus/rules/ast"

//line parser.y:21
type yySymType struct {
	yys int
        num model.SampleValue
        str string
        ruleNode ast.Node
        ruleNodeSlice []ast.Node
        boolean bool
        labelNameSlice model.LabelNames
        labelSet model.LabelSet
}

const START_RULES = 57346
const START_EXPRESSION = 57347
const IDENTIFIER = 57348
const STRING = 57349
const DURATION = 57350
const NUMBER = 57351
const PERMANENT = 57352
const GROUP_OP = 57353
const AGGR_OP = 57354
const CMP_OP = 57355
const ADDITIVE_OP = 57356
const MULT_OP = 57357
const ALERT = 57358
const IF = 57359
const FOR = 57360
const WITH = 57361

var yyToknames = []string{
	"START_RULES",
	"START_EXPRESSION",
	"IDENTIFIER",
	"STRING",
	"DURATION",
	"NUMBER",
	"PERMANENT",
	"GROUP_OP",
	"AGGR_OP",
	"CMP_OP",
	"ADDITIVE_OP",
	"MULT_OP",
	"ALERT",
	"IF",
	"FOR",
	"WITH",
	" =",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line parser.y:188


//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	6, 10,
	-2, 1,
}

const yyNprod = 36
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 97

var yyAct = []int{

	20, 38, 34, 17, 33, 43, 6, 67, 15, 66,
	19, 18, 16, 17, 15, 45, 59, 44, 60, 27,
	28, 29, 16, 17, 15, 41, 40, 18, 16, 17,
	18, 16, 17, 22, 15, 23, 21, 46, 47, 49,
	15, 8, 35, 15, 10, 51, 22, 9, 50, 53,
	52, 48, 61, 57, 18, 16, 17, 39, 8, 7,
	32, 10, 65, 42, 9, 56, 30, 15, 8, 35,
	54, 10, 62, 37, 9, 14, 7, 26, 68, 64,
	39, 13, 25, 24, 2, 3, 7, 11, 5, 4,
	1, 58, 12, 36, 55, 63, 31,
}
var yyPact = []int{

	80, -1000, -1000, 52, 65, -1000, 17, 52, 12, 11,
	-1000, -1000, 77, 76, -1000, 69, 52, 52, 52, 41,
	-1000, 35, 51, 52, 25, 46, -22, -12, -18, 8,
	-1000, -8, -1000, -1000, 17, -1000, 15, -1000, -1000, 31,
	14, 28, 52, -1000, -1000, 62, -1000, 74, 63, 54,
	52, -2, -1000, -1000, -1000, -1000, -6, 17, 33, 64,
	73, 25, -1000, -16, -1000, -1000, -1000, 72, -1000,
}
var yyPgo = []int{

	0, 96, 95, 94, 1, 93, 0, 2, 4, 92,
	91, 90, 89, 88, 87,
}
var yyR1 = []int{

	0, 11, 11, 12, 12, 13, 14, 14, 10, 10,
	9, 9, 6, 6, 6, 5, 5, 4, 7, 7,
	7, 7, 7, 7, 7, 7, 7, 7, 3, 3,
	2, 2, 1, 1, 8, 8,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 7, 0, 2,
	0, 1, 0, 3, 2, 1, 3, 3, 3, 2,
	4, 3, 4, 5, 3, 3, 3, 1, 0, 4,
	1, 3, 1, 3, 1, 1,
}
var yyChk = []int{

	-1000, -11, 4, 5, -12, -13, -7, 24, 6, 12,
	9, -14, -9, 16, 10, 26, 14, 15, 13, -7,
	-6, 24, 21, 24, 6, 6, 8, -7, -7, -7,
	25, -1, 25, -8, -7, 7, -5, 22, -4, 6,
	-7, -6, 17, 27, 25, 23, 22, 23, 20, 25,
	20, -7, -8, -4, 7, -3, 11, -7, -10, 18,
	24, 19, 8, -2, 6, -6, 25, 23, 6,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 12, 0,
	27, 4, 0, 0, 11, 0, 0, 0, 0, 0,
	19, 0, 0, 0, 12, 0, 0, 24, 25, 26,
	18, 0, 21, 32, 34, 35, 0, 14, 15, 0,
	0, 0, 0, 22, 20, 0, 13, 0, 0, 28,
	0, 8, 33, 16, 17, 23, 0, 6, 0, 0,
	0, 12, 9, 0, 30, 7, 29, 0, 31,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	24, 25, 3, 3, 23, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 20, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 26, 3, 27, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 21, 3, 22,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15, 16, 17, 18, 19,
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
		//line parser.y:66
		{ yylex.(*RulesLexer).parsedExpr = yyS[yypt-0].ruleNode }
	case 6:
		//line parser.y:70
		{
	                       rule, err := CreateRecordingRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 7:
		//line parser.y:76
		{
	                       rule, err := CreateAlertingRule(yyS[yypt-5].str, yyS[yypt-3].ruleNode, yyS[yypt-2].str, yyS[yypt-0].labelSet)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 8:
		//line parser.y:84
		{ yyVAL.str = "0s" }
	case 9:
		//line parser.y:86
		{ yyVAL.str = yyS[yypt-0].str }
	case 10:
		//line parser.y:90
		{ yyVAL.boolean = false }
	case 11:
		//line parser.y:92
		{ yyVAL.boolean = true }
	case 12:
		//line parser.y:96
		{ yyVAL.labelSet = model.LabelSet{} }
	case 13:
		//line parser.y:98
		{ yyVAL.labelSet = yyS[yypt-1].labelSet  }
	case 14:
		//line parser.y:100
		{ yyVAL.labelSet = model.LabelSet{} }
	case 15:
		//line parser.y:103
		{ yyVAL.labelSet = yyS[yypt-0].labelSet }
	case 16:
		//line parser.y:105
		{ for k, v := range yyS[yypt-0].labelSet { yyVAL.labelSet[k] = v } }
	case 17:
		//line parser.y:109
		{ yyVAL.labelSet = model.LabelSet{ model.LabelName(yyS[yypt-2].str): model.LabelValue(yyS[yypt-0].str) } }
	case 18:
		//line parser.y:114
		{ yyVAL.ruleNode = yyS[yypt-1].ruleNode }
	case 19:
		//line parser.y:116
		{ yyS[yypt-0].labelSet[model.MetricNameLabel] = model.LabelValue(yyS[yypt-1].str); yyVAL.ruleNode = ast.NewVectorLiteral(yyS[yypt-0].labelSet) }
	case 20:
		//line parser.y:118
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 21:
		//line parser.y:124
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 22:
		//line parser.y:130
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewMatrix(yyS[yypt-3].ruleNode, yyS[yypt-1].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 23:
		//line parser.y:136
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-4].str, yyS[yypt-2].ruleNode, yyS[yypt-0].labelNameSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 24:
		//line parser.y:144
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 25:
		//line parser.y:150
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 26:
		//line parser.y:156
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 27:
		//line parser.y:162
		{ yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)}
	case 28:
		//line parser.y:166
		{ yyVAL.labelNameSlice = model.LabelNames{} }
	case 29:
		//line parser.y:168
		{ yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice }
	case 30:
		//line parser.y:172
		{ yyVAL.labelNameSlice = model.LabelNames{model.LabelName(yyS[yypt-0].str)} }
	case 31:
		//line parser.y:174
		{ yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, model.LabelName(yyS[yypt-0].str)) }
	case 32:
		//line parser.y:178
		{ yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode} }
	case 33:
		//line parser.y:180
		{ yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode) }
	case 34:
		//line parser.y:184
		{ yyVAL.ruleNode = yyS[yypt-0].ruleNode }
	case 35:
		//line parser.y:186
		{ yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str) }
	}
	goto yystack /* stack new state and value */
}

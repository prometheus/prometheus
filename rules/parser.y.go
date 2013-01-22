
//line parser.y:2
        package rules

        import "fmt"
        import "github.com/matttproud/prometheus/model"
        import "github.com/matttproud/prometheus/rules/ast"

//line parser.y:9
type yySymType struct {
	yys int
        num model.SampleValue
        str string
        ruleNode ast.Node
        ruleNodeSlice []ast.Node
        boolean bool
        labelNameSlice []model.LabelName
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
	" =",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line parser.y:162


//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
	-1, 4,
	6, 7,
	-2, 1,
}

const yyNprod = 33
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 84

var yyAct = []int{

	32, 36, 31, 16, 6, 17, 15, 16, 18, 40,
	14, 14, 21, 46, 14, 20, 25, 26, 27, 8,
	33, 54, 10, 38, 22, 9, 19, 17, 15, 16,
	17, 15, 16, 7, 30, 28, 14, 8, 33, 14,
	10, 15, 16, 9, 47, 48, 49, 21, 53, 14,
	39, 7, 8, 37, 58, 10, 57, 42, 9, 41,
	43, 44, 52, 13, 45, 35, 7, 24, 50, 59,
	56, 37, 23, 2, 3, 11, 5, 4, 1, 12,
	34, 51, 55, 29,
}
var yyPact = []int{

	69, -1000, -1000, 46, 53, -1000, 17, 46, -5, 4,
	-1000, -1000, 66, -1000, 59, 46, 46, 46, 14, -1000,
	13, 47, 46, 30, -14, -12, -11, 27, -1000, 38,
	-1000, -1000, 17, -1000, 42, -1000, -1000, 48, -8, 28,
	-1000, -1000, 31, -1000, 65, 61, 51, 46, -1000, -1000,
	-1000, -1000, 1, 17, 64, 35, -1000, -1000, 63, -1000,
}
var yyPgo = []int{

	0, 83, 82, 81, 1, 80, 26, 0, 2, 79,
	78, 77, 76, 75,
}
var yyR1 = []int{

	0, 10, 10, 11, 11, 12, 13, 9, 9, 6,
	6, 6, 5, 5, 4, 7, 7, 7, 7, 7,
	7, 7, 7, 7, 7, 3, 3, 2, 2, 1,
	1, 8, 8,
}
var yyR2 = []int{

	0, 2, 2, 0, 2, 1, 5, 0, 1, 0,
	3, 2, 1, 3, 3, 3, 2, 4, 3, 4,
	5, 3, 3, 3, 1, 0, 4, 1, 3, 1,
	3, 1, 1,
}
var yyChk = []int{

	-1000, -10, 4, 5, -11, -12, -7, 20, 6, 12,
	9, -13, -9, 10, 22, 14, 15, 13, -7, -6,
	20, 17, 20, 6, 8, -7, -7, -7, 21, -1,
	21, -8, -7, 7, -5, 18, -4, 6, -7, -6,
	23, 21, 19, 18, 19, 16, 21, 16, -8, -4,
	7, -3, 11, -7, 20, -2, 6, 21, 19, 6,
}
var yyDef = []int{

	0, -2, 3, 0, -2, 2, 5, 0, 9, 0,
	24, 4, 0, 8, 0, 0, 0, 0, 0, 16,
	0, 0, 0, 9, 0, 21, 22, 23, 15, 0,
	18, 29, 31, 32, 0, 11, 12, 0, 0, 0,
	19, 17, 0, 10, 0, 0, 25, 0, 30, 13,
	14, 20, 0, 6, 0, 0, 27, 26, 0, 28,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	20, 21, 3, 3, 19, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 16, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 22, 3, 23, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 17, 3, 18,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12, 13, 14, 15,
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
	if c > 0 && c <= len(yyToknames) {
		if yyToknames[c-1] != "" {
			return yyToknames[c-1]
		}
	}
	return fmt.Sprintf("tok-%v", c)
}

func yyStatname(s int) string {
	if s >= 0 && s < len(yyStatenames) {
		if yyStatenames[s] != "" {
			return yyStatenames[s]
		}
	}
	return fmt.Sprintf("state-%v", s)
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
		fmt.Printf("lex %U %s\n", uint(char), yyTokname(c))
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
		fmt.Printf("char %v in %v\n", yyTokname(yychar), yyStatname(yystate))
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
				fmt.Printf("%s", yyStatname(yystate))
				fmt.Printf("saw %s\n", yyTokname(yychar))
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
					fmt.Printf("error recovery pops state %d\n", yyS[yyp].yys)
				}
				yyp--
			}
			/* there is no state on the stack with an error shift ... abort */
			goto ret1

		case 3: /* no shift yet; clobber input char */
			if yyDebug >= 2 {
				fmt.Printf("error recovery discards %s\n", yyTokname(yychar))
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
		fmt.Printf("reduce %v in:\n\t%v\n", yyn, yyStatname(yystate))
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
		//line parser.y:52
		{ yylex.(*RulesLexer).parsedExpr = yyS[yypt-0].ruleNode }
	case 6:
		//line parser.y:56
		{
	                       rule, err := CreateRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       yylex.(*RulesLexer).parsedRules = append(yylex.(*RulesLexer).parsedRules, rule)
	                     }
	case 7:
		//line parser.y:64
		{ yyVAL.boolean = false }
	case 8:
		//line parser.y:66
		{ yyVAL.boolean = true }
	case 9:
		//line parser.y:70
		{ yyVAL.labelSet = model.LabelSet{} }
	case 10:
		//line parser.y:72
		{ yyVAL.labelSet = yyS[yypt-1].labelSet  }
	case 11:
		//line parser.y:74
		{ yyVAL.labelSet = model.LabelSet{} }
	case 12:
		//line parser.y:77
		{ yyVAL.labelSet = yyS[yypt-0].labelSet }
	case 13:
		//line parser.y:79
		{ for k, v := range yyS[yypt-0].labelSet { yyVAL.labelSet[k] = v } }
	case 14:
		//line parser.y:83
		{ yyVAL.labelSet = model.LabelSet{ model.LabelName(yyS[yypt-2].str): model.LabelValue(yyS[yypt-0].str) } }
	case 15:
		//line parser.y:88
		{ yyVAL.ruleNode = yyS[yypt-1].ruleNode }
	case 16:
		//line parser.y:90
		{ yyS[yypt-0].labelSet["name"] = model.LabelValue(yyS[yypt-1].str); yyVAL.ruleNode = ast.NewVectorLiteral(yyS[yypt-0].labelSet) }
	case 17:
		//line parser.y:92
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 18:
		//line parser.y:98
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 19:
		//line parser.y:104
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewMatrix(yyS[yypt-3].ruleNode, yyS[yypt-1].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 20:
		//line parser.y:110
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-4].str, yyS[yypt-2].ruleNode, yyS[yypt-0].labelNameSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 21:
		//line parser.y:118
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 22:
		//line parser.y:124
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 23:
		//line parser.y:130
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 24:
		//line parser.y:136
		{ yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)}
	case 25:
		//line parser.y:140
		{ yyVAL.labelNameSlice = []model.LabelName{} }
	case 26:
		//line parser.y:142
		{ yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice }
	case 27:
		//line parser.y:146
		{ yyVAL.labelNameSlice = []model.LabelName{model.LabelName(yyS[yypt-0].str)} }
	case 28:
		//line parser.y:148
		{ yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, model.LabelName(yyS[yypt-0].str)) }
	case 29:
		//line parser.y:152
		{ yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode} }
	case 30:
		//line parser.y:154
		{ yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode) }
	case 31:
		//line parser.y:158
		{ yyVAL.ruleNode = yyS[yypt-0].ruleNode }
	case 32:
		//line parser.y:160
		{ yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str) }
	}
	goto yystack /* stack new state and value */
}

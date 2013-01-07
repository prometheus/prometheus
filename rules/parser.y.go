
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

const IDENTIFIER = 57346
const STRING = 57347
const NUMBER = 57348
const PERMANENT = 57349
const GROUP_OP = 57350
const AGGR_OP = 57351
const CMP_OP = 57352
const ADDITIVE_OP = 57353
const MULT_OP = 57354

var yyToknames = []string{
	"IDENTIFIER",
	"STRING",
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

//line parser.y:148


//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 4,
}

const yyNprod = 30
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 77

var yyAct = []int{

	36, 37, 48, 49, 15, 38, 17, 27, 7, 16,
	13, 23, 21, 22, 18, 19, 24, 14, 35, 53,
	42, 52, 20, 30, 31, 32, 15, 38, 17, 39,
	11, 16, 8, 23, 21, 22, 23, 21, 22, 14,
	22, 43, 44, 15, 33, 17, 12, 41, 16, 40,
	28, 7, 6, 47, 26, 54, 14, 10, 23, 21,
	22, 21, 22, 4, 45, 29, 51, 12, 25, 5,
	2, 1, 3, 9, 46, 50, 34,
}
var yyPact = []int{

	-1000, 56, -1000, 65, -1000, -6, 19, 42, 39, -1,
	-1000, -1000, 9, 48, 39, 37, -10, -1000, -1000, 63,
	60, 39, 39, 39, 26, -1000, 0, 39, -1000, -1000,
	28, -1000, 50, -1000, 31, -1000, -1000, 1, -1000, 23,
	-1000, 22, 59, 45, -1000, -18, -1000, -14, -1000, 62,
	3, -1000, -1000, 51, -1000,
}
var yyPgo = []int{

	0, 76, 75, 74, 30, 73, 52, 1, 0, 72,
	71, 70,
}
var yyR1 = []int{

	0, 10, 10, 11, 9, 9, 6, 6, 6, 5,
	5, 4, 7, 7, 7, 7, 7, 7, 7, 7,
	7, 3, 3, 2, 2, 1, 1, 8, 8, 8,
}
var yyR2 = []int{

	0, 0, 2, 5, 0, 1, 0, 3, 2, 1,
	3, 3, 3, 2, 4, 3, 5, 3, 3, 3,
	1, 0, 4, 1, 3, 1, 3, 1, 4, 1,
}
var yyChk = []int{

	-1000, -10, -11, -9, 7, 4, -6, 14, 13, -5,
	15, -4, 4, -7, 17, 4, 9, 6, 15, 16,
	13, 11, 12, 10, -7, -6, 17, 17, -4, 5,
	-7, -7, -7, 18, -1, 18, -8, -7, 5, -7,
	18, 16, 19, 18, -8, 5, -3, 8, 20, 17,
	-2, 4, 18, 16, 4,
}
var yyDef = []int{

	1, -2, 2, 0, 5, 6, 0, 0, 0, 0,
	8, 9, 0, 3, 0, 6, 0, 20, 7, 0,
	0, 0, 0, 0, 0, 13, 0, 0, 10, 11,
	17, 18, 19, 12, 0, 15, 25, 27, 29, 0,
	14, 0, 0, 21, 26, 0, 16, 0, 28, 0,
	0, 23, 22, 0, 24,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	17, 18, 3, 3, 16, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 13, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 19, 3, 20, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 14, 3, 15,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
	12,
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

	case 3:
		//line parser.y:42
		{
	                       rule, err := CreateRule(yyS[yypt-3].str, yyS[yypt-2].labelSet, yyS[yypt-0].ruleNode, yyS[yypt-4].boolean)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                       addRule(rule)
	                     }
	case 4:
		//line parser.y:50
		{ yyVAL.boolean = false }
	case 5:
		//line parser.y:52
		{ yyVAL.boolean = true }
	case 6:
		//line parser.y:56
		{ yyVAL.labelSet = model.LabelSet{} }
	case 7:
		//line parser.y:58
		{ yyVAL.labelSet = yyS[yypt-1].labelSet  }
	case 8:
		//line parser.y:60
		{ yyVAL.labelSet = model.LabelSet{} }
	case 9:
		//line parser.y:63
		{ yyVAL.labelSet = yyS[yypt-0].labelSet }
	case 10:
		//line parser.y:65
		{ for k, v := range yyS[yypt-0].labelSet { yyVAL.labelSet[k] = v } }
	case 11:
		//line parser.y:69
		{ yyVAL.labelSet = model.LabelSet{ model.LabelName(yyS[yypt-2].str): model.LabelValue(yyS[yypt-0].str) } }
	case 12:
		//line parser.y:74
		{ yyVAL.ruleNode = yyS[yypt-1].ruleNode }
	case 13:
		//line parser.y:76
		{ yyS[yypt-0].labelSet["name"] = model.LabelValue(yyS[yypt-1].str); yyVAL.ruleNode = ast.NewVectorLiteral(yyS[yypt-0].labelSet) }
	case 14:
		//line parser.y:78
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-3].str, yyS[yypt-1].ruleNodeSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 15:
		//line parser.y:84
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewFunctionCall(yyS[yypt-2].str, []ast.Node{})
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 16:
		//line parser.y:90
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewVectorAggregation(yyS[yypt-4].str, yyS[yypt-2].ruleNode, yyS[yypt-0].labelNameSlice)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 17:
		//line parser.y:98
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 18:
		//line parser.y:104
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 19:
		//line parser.y:110
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewArithExpr(yyS[yypt-1].str, yyS[yypt-2].ruleNode, yyS[yypt-0].ruleNode)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 20:
		//line parser.y:116
		{ yyVAL.ruleNode = ast.NewScalarLiteral(yyS[yypt-0].num)}
	case 21:
		//line parser.y:120
		{ yyVAL.labelNameSlice = []model.LabelName{} }
	case 22:
		//line parser.y:122
		{ yyVAL.labelNameSlice = yyS[yypt-1].labelNameSlice }
	case 23:
		//line parser.y:126
		{ yyVAL.labelNameSlice = []model.LabelName{model.LabelName(yyS[yypt-0].str)} }
	case 24:
		//line parser.y:128
		{ yyVAL.labelNameSlice = append(yyVAL.labelNameSlice, model.LabelName(yyS[yypt-0].str)) }
	case 25:
		//line parser.y:132
		{ yyVAL.ruleNodeSlice = []ast.Node{yyS[yypt-0].ruleNode} }
	case 26:
		//line parser.y:134
		{ yyVAL.ruleNodeSlice = append(yyVAL.ruleNodeSlice, yyS[yypt-0].ruleNode) }
	case 27:
		//line parser.y:138
		{ yyVAL.ruleNode = yyS[yypt-0].ruleNode }
	case 28:
		//line parser.y:140
		{
	                       var err error
	                       yyVAL.ruleNode, err = NewMatrix(yyS[yypt-3].ruleNode, yyS[yypt-1].str)
	                       if err != nil { yylex.Error(err.Error()); return 1 }
	                     }
	case 29:
		//line parser.y:146
		{ yyVAL.ruleNode = ast.NewStringLiteral(yyS[yypt-0].str) }
	}
	goto yystack /* stack new state and value */
}

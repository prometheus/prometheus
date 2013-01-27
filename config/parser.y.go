
//line parser.y:2
        package config

        import "fmt"
        import "github.com/prometheus/prometheus/model"

//line parser.y:8
type yySymType struct {
	yys int
        num model.SampleValue
        str string
        stringSlice []string
        labelSet model.LabelSet
}

const IDENTIFIER = 57346
const STRING = 57347
const GLOBAL = 57348
const JOB = 57349
const RULE_FILES = 57350
const LABELS = 57351
const TARGETS = 57352
const ENDPOINTS = 57353

var yyToknames = []string{
	"IDENTIFIER",
	"STRING",
	"GLOBAL",
	"JOB",
	"RULE_FILES",
	"LABELS",
	"TARGETS",
	"ENDPOINTS",
}
var yyStatenames = []string{}

const yyEofCode = 1
const yyErrCode = 2
const yyMaxDepth = 200

//line parser.y:102


//line yacctab:1
var yyExca = []int{
	-1, 1,
	1, -1,
	-2, 0,
}

const yyNprod = 29
const yyPrivate = 57344

var yyTokenNames []string
var yyStates []string

const yyLast = 53

var yyAct = []int{

	30, 28, 12, 48, 39, 47, 31, 34, 11, 35,
	49, 36, 15, 14, 29, 18, 38, 9, 14, 23,
	44, 19, 40, 27, 16, 50, 22, 20, 24, 21,
	6, 5, 3, 4, 46, 32, 43, 45, 25, 29,
	41, 33, 17, 10, 8, 7, 2, 1, 26, 42,
	51, 13, 37,
}
var yyPact = []int{

	-1000, 26, -1000, 19, 18, -1000, -1000, 4, 11, -1000,
	-1000, 13, -1000, -1000, 17, 12, -1000, -1000, 5, 16,
	33, 10, -10, 30, -1000, -1000, -6, -1000, -1000, -3,
	-1000, -1, -1000, 9, -1000, 35, 29, -12, -1000, -1000,
	-1000, -1000, -1000, -1000, -4, -1000, -1000, -1000, 20, -10,
	-1000, -1000,
}
var yyPgo = []int{

	0, 0, 52, 51, 49, 2, 1, 48, 47, 46,
	45, 44, 43, 42, 41, 40,
}
var yyR1 = []int{

	0, 8, 8, 9, 9, 10, 10, 12, 12, 12,
	5, 5, 7, 7, 6, 3, 11, 11, 13, 13,
	14, 14, 15, 15, 4, 1, 1, 2, 2,
}
var yyR2 = []int{

	0, 0, 2, 4, 4, 0, 2, 3, 1, 1,
	4, 3, 1, 3, 3, 3, 0, 2, 3, 4,
	0, 2, 1, 1, 3, 3, 2, 1, 3,
}
var yyChk = []int{

	-1000, -8, -9, 6, 7, 12, 12, -10, -11, 13,
	-12, 4, -5, -3, 9, 8, 13, -13, 4, 10,
	14, 12, 14, 14, 12, 5, -7, 13, -6, 4,
	-1, 16, 5, -14, 13, 15, 14, -2, 17, 5,
	13, -15, -4, -5, 11, -6, 5, 17, 15, 14,
	5, -1,
}
var yyDef = []int{

	1, -2, 2, 0, 0, 5, 16, 0, 0, 3,
	6, 0, 8, 9, 0, 0, 4, 17, 0, 0,
	0, 0, 0, 0, 20, 7, 0, 11, 12, 0,
	15, 0, 18, 0, 10, 0, 0, 0, 26, 27,
	19, 21, 22, 23, 0, 13, 14, 25, 0, 0,
	28, 24,
}
var yyTok1 = []int{

	1, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 15, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 14, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 16, 3, 17, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 3, 3, 3, 3, 3, 3, 3,
	3, 3, 3, 12, 3, 13,
}
var yyTok2 = []int{

	2, 3, 4, 5, 6, 7, 8, 9, 10, 11,
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

	case 4:
		//line parser.y:32
		{ PopJob() }
	case 7:
		//line parser.y:40
		{ parsedConfig.Global.SetOption(yyS[yypt-2].str, yyS[yypt-0].str) }
	case 8:
		//line parser.y:42
		{ parsedConfig.Global.SetLabels(yyS[yypt-0].labelSet) }
	case 9:
		//line parser.y:44
		{ parsedConfig.Global.AddRuleFiles(yyS[yypt-0].stringSlice) }
	case 10:
		//line parser.y:48
		{ yyVAL.labelSet = yyS[yypt-1].labelSet }
	case 11:
		//line parser.y:50
		{ yyVAL.labelSet = model.LabelSet{} }
	case 12:
		//line parser.y:54
		{ yyVAL.labelSet = yyS[yypt-0].labelSet }
	case 13:
		//line parser.y:56
		{ for k, v := range yyS[yypt-0].labelSet { yyVAL.labelSet[k] = v } }
	case 14:
		//line parser.y:60
		{ yyVAL.labelSet = model.LabelSet{ model.LabelName(yyS[yypt-2].str): model.LabelValue(yyS[yypt-0].str) } }
	case 15:
		//line parser.y:64
		{ yyVAL.stringSlice = yyS[yypt-0].stringSlice }
	case 18:
		//line parser.y:72
		{ PushJobOption(yyS[yypt-2].str, yyS[yypt-0].str) }
	case 19:
		//line parser.y:74
		{ PushJobTargets() }
	case 22:
		//line parser.y:82
		{ PushTargetEndpoints(yyS[yypt-0].stringSlice) }
	case 23:
		//line parser.y:84
		{ PushTargetLabels(yyS[yypt-0].labelSet) }
	case 24:
		//line parser.y:88
		{ yyVAL.stringSlice = yyS[yypt-0].stringSlice }
	case 25:
		//line parser.y:92
		{ yyVAL.stringSlice = yyS[yypt-1].stringSlice }
	case 26:
		//line parser.y:94
		{ yyVAL.stringSlice = []string{} }
	case 27:
		//line parser.y:98
		{ yyVAL.stringSlice = []string{yyS[yypt-0].str} }
	case 28:
		//line parser.y:100
		{ yyVAL.stringSlice = append(yyVAL.stringSlice, yyS[yypt-0].str) }
	}
	goto yystack /* stack new state and value */
}

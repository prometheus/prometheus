// Package staticcheck contains a linter for Go source code.
package staticcheck // import "honnef.co/go/tools/staticcheck"

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	htmltemplate "html/template"
	"net/http"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"sync"
	texttemplate "text/template"

	. "honnef.co/go/tools/arg"
	"honnef.co/go/tools/deprecated"
	"honnef.co/go/tools/functions"
	"honnef.co/go/tools/internal/sharedcheck"
	"honnef.co/go/tools/lint"
	. "honnef.co/go/tools/lint/lintdsl"
	"honnef.co/go/tools/ssa"
	"honnef.co/go/tools/ssautil"
	"honnef.co/go/tools/staticcheck/vrp"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

func validRegexp(call *Call) {
	arg := call.Args[0]
	err := ValidateRegexp(arg.Value)
	if err != nil {
		arg.Invalid(err.Error())
	}
}

type runeSlice []rune

func (rs runeSlice) Len() int               { return len(rs) }
func (rs runeSlice) Less(i int, j int) bool { return rs[i] < rs[j] }
func (rs runeSlice) Swap(i int, j int)      { rs[i], rs[j] = rs[j], rs[i] }

func utf8Cutset(call *Call) {
	arg := call.Args[1]
	if InvalidUTF8(arg.Value) {
		arg.Invalid(MsgInvalidUTF8)
	}
}

func uniqueCutset(call *Call) {
	arg := call.Args[1]
	if !UniqueStringCutset(arg.Value) {
		arg.Invalid(MsgNonUniqueCutset)
	}
}

func unmarshalPointer(name string, arg int) CallCheck {
	return func(call *Call) {
		if !Pointer(call.Args[arg].Value) {
			call.Args[arg].Invalid(fmt.Sprintf("%s expects to unmarshal into a pointer, but the provided value is not a pointer", name))
		}
	}
}

func pointlessIntMath(call *Call) {
	if ConvertedFromInt(call.Args[0].Value) {
		call.Invalid(fmt.Sprintf("calling %s on a converted integer is pointless", CallName(call.Instr.Common())))
	}
}

func checkValidHostPort(arg int) CallCheck {
	return func(call *Call) {
		if !ValidHostPort(call.Args[arg].Value) {
			call.Args[arg].Invalid(MsgInvalidHostPort)
		}
	}
}

var (
	checkRegexpRules = map[string]CallCheck{
		"regexp.MustCompile": validRegexp,
		"regexp.Compile":     validRegexp,
		"regexp.Match":       validRegexp,
		"regexp.MatchReader": validRegexp,
		"regexp.MatchString": validRegexp,
	}

	checkTimeParseRules = map[string]CallCheck{
		"time.Parse": func(call *Call) {
			arg := call.Args[Arg("time.Parse.layout")]
			err := ValidateTimeLayout(arg.Value)
			if err != nil {
				arg.Invalid(err.Error())
			}
		},
	}

	checkEncodingBinaryRules = map[string]CallCheck{
		"encoding/binary.Write": func(call *Call) {
			arg := call.Args[Arg("encoding/binary.Write.data")]
			if !CanBinaryMarshal(call.Job, arg.Value) {
				arg.Invalid(fmt.Sprintf("value of type %s cannot be used with binary.Write", arg.Value.Value.Type()))
			}
		},
	}

	checkURLsRules = map[string]CallCheck{
		"net/url.Parse": func(call *Call) {
			arg := call.Args[Arg("net/url.Parse.rawurl")]
			err := ValidateURL(arg.Value)
			if err != nil {
				arg.Invalid(err.Error())
			}
		},
	}

	checkSyncPoolValueRules = map[string]CallCheck{
		"(*sync.Pool).Put": func(call *Call) {
			arg := call.Args[Arg("(*sync.Pool).Put.x")]
			typ := arg.Value.Value.Type()
			if !IsPointerLike(typ) {
				arg.Invalid("argument should be pointer-like to avoid allocations")
			}
		},
	}

	checkRegexpFindAllRules = map[string]CallCheck{
		"(*regexp.Regexp).FindAll":                    RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllIndex":               RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllString":              RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllStringIndex":         RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllStringSubmatch":      RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllStringSubmatchIndex": RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllSubmatch":            RepeatZeroTimes("a FindAll method", 1),
		"(*regexp.Regexp).FindAllSubmatchIndex":       RepeatZeroTimes("a FindAll method", 1),
	}

	checkUTF8CutsetRules = map[string]CallCheck{
		"strings.IndexAny":     utf8Cutset,
		"strings.LastIndexAny": utf8Cutset,
		"strings.ContainsAny":  utf8Cutset,
		"strings.Trim":         utf8Cutset,
		"strings.TrimLeft":     utf8Cutset,
		"strings.TrimRight":    utf8Cutset,
	}

	checkUniqueCutsetRules = map[string]CallCheck{
		"strings.Trim":      uniqueCutset,
		"strings.TrimLeft":  uniqueCutset,
		"strings.TrimRight": uniqueCutset,
	}

	checkUnmarshalPointerRules = map[string]CallCheck{
		"encoding/xml.Unmarshal":                unmarshalPointer("xml.Unmarshal", 1),
		"(*encoding/xml.Decoder).Decode":        unmarshalPointer("Decode", 0),
		"(*encoding/xml.Decoder).DecodeElement": unmarshalPointer("DecodeElement", 0),
		"encoding/json.Unmarshal":               unmarshalPointer("json.Unmarshal", 1),
		"(*encoding/json.Decoder).Decode":       unmarshalPointer("Decode", 0),
	}

	checkUnbufferedSignalChanRules = map[string]CallCheck{
		"os/signal.Notify": func(call *Call) {
			arg := call.Args[Arg("os/signal.Notify.c")]
			if UnbufferedChannel(arg.Value) {
				arg.Invalid("the channel used with signal.Notify should be buffered")
			}
		},
	}

	checkMathIntRules = map[string]CallCheck{
		"math.Ceil":  pointlessIntMath,
		"math.Floor": pointlessIntMath,
		"math.IsNaN": pointlessIntMath,
		"math.Trunc": pointlessIntMath,
		"math.IsInf": pointlessIntMath,
	}

	checkStringsReplaceZeroRules = map[string]CallCheck{
		"strings.Replace": RepeatZeroTimes("strings.Replace", 3),
		"bytes.Replace":   RepeatZeroTimes("bytes.Replace", 3),
	}

	checkListenAddressRules = map[string]CallCheck{
		"net/http.ListenAndServe":    checkValidHostPort(0),
		"net/http.ListenAndServeTLS": checkValidHostPort(0),
	}

	checkBytesEqualIPRules = map[string]CallCheck{
		"bytes.Equal": func(call *Call) {
			if ConvertedFrom(call.Args[Arg("bytes.Equal.a")].Value, "net.IP") &&
				ConvertedFrom(call.Args[Arg("bytes.Equal.b")].Value, "net.IP") {
				call.Invalid("use net.IP.Equal to compare net.IPs, not bytes.Equal")
			}
		},
	}

	checkRegexpMatchLoopRules = map[string]CallCheck{
		"regexp.Match":       loopedRegexp("regexp.Match"),
		"regexp.MatchReader": loopedRegexp("regexp.MatchReader"),
		"regexp.MatchString": loopedRegexp("regexp.MatchString"),
	}
)

type Checker struct {
	CheckGenerated bool
	funcDescs      *functions.Descriptions
	deprecatedObjs map[types.Object]string
}

func NewChecker() *Checker {
	return &Checker{}
}

func (*Checker) Name() string   { return "staticcheck" }
func (*Checker) Prefix() string { return "SA" }

func (c *Checker) Checks() []lint.Check {
	return []lint.Check{
		{ID: "SA1000", FilterGenerated: false, Fn: c.callChecker(checkRegexpRules)},
		{ID: "SA1001", FilterGenerated: false, Fn: c.CheckTemplate},
		{ID: "SA1002", FilterGenerated: false, Fn: c.callChecker(checkTimeParseRules)},
		{ID: "SA1003", FilterGenerated: false, Fn: c.callChecker(checkEncodingBinaryRules)},
		{ID: "SA1004", FilterGenerated: false, Fn: c.CheckTimeSleepConstant},
		{ID: "SA1005", FilterGenerated: false, Fn: c.CheckExec},
		{ID: "SA1006", FilterGenerated: false, Fn: c.CheckUnsafePrintf},
		{ID: "SA1007", FilterGenerated: false, Fn: c.callChecker(checkURLsRules)},
		{ID: "SA1008", FilterGenerated: false, Fn: c.CheckCanonicalHeaderKey},
		{ID: "SA1010", FilterGenerated: false, Fn: c.callChecker(checkRegexpFindAllRules)},
		{ID: "SA1011", FilterGenerated: false, Fn: c.callChecker(checkUTF8CutsetRules)},
		{ID: "SA1012", FilterGenerated: false, Fn: c.CheckNilContext},
		{ID: "SA1013", FilterGenerated: false, Fn: c.CheckSeeker},
		{ID: "SA1014", FilterGenerated: false, Fn: c.callChecker(checkUnmarshalPointerRules)},
		{ID: "SA1015", FilterGenerated: false, Fn: c.CheckLeakyTimeTick},
		{ID: "SA1016", FilterGenerated: false, Fn: c.CheckUntrappableSignal},
		{ID: "SA1017", FilterGenerated: false, Fn: c.callChecker(checkUnbufferedSignalChanRules)},
		{ID: "SA1018", FilterGenerated: false, Fn: c.callChecker(checkStringsReplaceZeroRules)},
		{ID: "SA1019", FilterGenerated: false, Fn: c.CheckDeprecated},
		{ID: "SA1020", FilterGenerated: false, Fn: c.callChecker(checkListenAddressRules)},
		{ID: "SA1021", FilterGenerated: false, Fn: c.callChecker(checkBytesEqualIPRules)},
		{ID: "SA1023", FilterGenerated: false, Fn: c.CheckWriterBufferModified},
		{ID: "SA1024", FilterGenerated: false, Fn: c.callChecker(checkUniqueCutsetRules)},
		{ID: "SA1025", FilterGenerated: false, Fn: c.CheckTimerResetReturnValue},

		{ID: "SA2000", FilterGenerated: false, Fn: c.CheckWaitgroupAdd},
		{ID: "SA2001", FilterGenerated: false, Fn: c.CheckEmptyCriticalSection},
		{ID: "SA2002", FilterGenerated: false, Fn: c.CheckConcurrentTesting},
		{ID: "SA2003", FilterGenerated: false, Fn: c.CheckDeferLock},

		{ID: "SA3000", FilterGenerated: false, Fn: c.CheckTestMainExit},
		{ID: "SA3001", FilterGenerated: false, Fn: c.CheckBenchmarkN},

		{ID: "SA4000", FilterGenerated: false, Fn: c.CheckLhsRhsIdentical},
		{ID: "SA4001", FilterGenerated: false, Fn: c.CheckIneffectiveCopy},
		{ID: "SA4002", FilterGenerated: false, Fn: c.CheckDiffSizeComparison},
		{ID: "SA4003", FilterGenerated: false, Fn: c.CheckUnsignedComparison},
		{ID: "SA4004", FilterGenerated: false, Fn: c.CheckIneffectiveLoop},
		{ID: "SA4006", FilterGenerated: false, Fn: c.CheckUnreadVariableValues},
		{ID: "SA4008", FilterGenerated: false, Fn: c.CheckLoopCondition},
		{ID: "SA4009", FilterGenerated: false, Fn: c.CheckArgOverwritten},
		{ID: "SA4010", FilterGenerated: false, Fn: c.CheckIneffectiveAppend},
		{ID: "SA4011", FilterGenerated: false, Fn: c.CheckScopedBreak},
		{ID: "SA4012", FilterGenerated: false, Fn: c.CheckNaNComparison},
		{ID: "SA4013", FilterGenerated: false, Fn: c.CheckDoubleNegation},
		{ID: "SA4014", FilterGenerated: false, Fn: c.CheckRepeatedIfElse},
		{ID: "SA4015", FilterGenerated: false, Fn: c.callChecker(checkMathIntRules)},
		{ID: "SA4016", FilterGenerated: false, Fn: c.CheckSillyBitwiseOps},
		{ID: "SA4017", FilterGenerated: false, Fn: c.CheckPureFunctions},
		{ID: "SA4018", FilterGenerated: true, Fn: c.CheckSelfAssignment},
		{ID: "SA4019", FilterGenerated: true, Fn: c.CheckDuplicateBuildConstraints},

		{ID: "SA5000", FilterGenerated: false, Fn: c.CheckNilMaps},
		{ID: "SA5001", FilterGenerated: false, Fn: c.CheckEarlyDefer},
		{ID: "SA5002", FilterGenerated: false, Fn: c.CheckInfiniteEmptyLoop},
		{ID: "SA5003", FilterGenerated: false, Fn: c.CheckDeferInInfiniteLoop},
		{ID: "SA5004", FilterGenerated: false, Fn: c.CheckLoopEmptyDefault},
		{ID: "SA5005", FilterGenerated: false, Fn: c.CheckCyclicFinalizer},
		{ID: "SA5007", FilterGenerated: false, Fn: c.CheckInfiniteRecursion},

		{ID: "SA6000", FilterGenerated: false, Fn: c.callChecker(checkRegexpMatchLoopRules)},
		{ID: "SA6001", FilterGenerated: false, Fn: c.CheckMapBytesKey},
		{ID: "SA6002", FilterGenerated: false, Fn: c.callChecker(checkSyncPoolValueRules)},
		{ID: "SA6003", FilterGenerated: false, Fn: c.CheckRangeStringRunes},
		// {ID: "SA6004", FilterGenerated: false, Fn: c.CheckSillyRegexp},

		{ID: "SA9001", FilterGenerated: false, Fn: c.CheckDubiousDeferInChannelRangeLoop},
		{ID: "SA9002", FilterGenerated: false, Fn: c.CheckNonOctalFileMode},
		{ID: "SA9003", FilterGenerated: false, Fn: c.CheckEmptyBranch},
		{ID: "SA9004", FilterGenerated: false, Fn: c.CheckMissingEnumTypesInDeclaration},
	}

	// "SA5006": c.CheckSliceOutOfBounds,
	// "SA4007": c.CheckPredeterminedBooleanExprs,
}

func (c *Checker) findDeprecated(prog *lint.Program) {
	var docs []*ast.CommentGroup
	var names []*ast.Ident

	doDocs := func(pkg *packages.Package, names []*ast.Ident, docs []*ast.CommentGroup) {
		var alt string
		for _, doc := range docs {
			if doc == nil {
				continue
			}
			parts := strings.Split(doc.Text(), "\n\n")
			last := parts[len(parts)-1]
			if !strings.HasPrefix(last, "Deprecated: ") {
				continue
			}
			alt = last[len("Deprecated: "):]
			alt = strings.Replace(alt, "\n", " ", -1)
			break
		}
		if alt == "" {
			return
		}

		for _, name := range names {
			obj := pkg.TypesInfo.ObjectOf(name)
			c.deprecatedObjs[obj] = alt
		}
	}

	for _, pkg := range prog.AllPackages {
		for _, f := range pkg.Syntax {
			fn := func(node ast.Node) bool {
				if node == nil {
					return true
				}
				var ret bool
				switch node := node.(type) {
				case *ast.GenDecl:
					switch node.Tok {
					case token.TYPE, token.CONST, token.VAR:
						docs = append(docs, node.Doc)
						return true
					default:
						return false
					}
				case *ast.FuncDecl:
					docs = append(docs, node.Doc)
					names = []*ast.Ident{node.Name}
					ret = false
				case *ast.TypeSpec:
					docs = append(docs, node.Doc)
					names = []*ast.Ident{node.Name}
					ret = true
				case *ast.ValueSpec:
					docs = append(docs, node.Doc)
					names = node.Names
					ret = false
				case *ast.File:
					return true
				case *ast.StructType:
					for _, field := range node.Fields.List {
						doDocs(pkg, field.Names, []*ast.CommentGroup{field.Doc})
					}
					return false
				case *ast.InterfaceType:
					for _, field := range node.Methods.List {
						doDocs(pkg, field.Names, []*ast.CommentGroup{field.Doc})
					}
					return false
				default:
					return false
				}
				if len(names) == 0 || len(docs) == 0 {
					return ret
				}
				doDocs(pkg, names, docs)

				docs = docs[:0]
				names = nil
				return ret
			}
			ast.Inspect(f, fn)
		}
	}
}

func (c *Checker) Init(prog *lint.Program) {
	wg := &sync.WaitGroup{}
	wg.Add(2)
	go func() {
		c.funcDescs = functions.NewDescriptions(prog.SSA)
		for _, fn := range prog.AllFunctions {
			if fn.Blocks != nil {
				applyStdlibKnowledge(fn)
				ssa.OptimizeBlocks(fn)
			}
		}
		wg.Done()
	}()

	go func() {
		c.deprecatedObjs = map[types.Object]string{}
		c.findDeprecated(prog)
		wg.Done()
	}()

	wg.Wait()
}

func (c *Checker) isInLoop(b *ssa.BasicBlock) bool {
	sets := c.funcDescs.Get(b.Parent()).Loops
	for _, set := range sets {
		if set[b] {
			return true
		}
	}
	return false
}

func applyStdlibKnowledge(fn *ssa.Function) {
	if len(fn.Blocks) == 0 {
		return
	}

	// comma-ok receiving from a time.Tick channel will never return
	// ok == false, so any branching on the value of ok can be
	// replaced with an unconditional jump. This will primarily match
	// `for range time.Tick(x)` loops, but it can also match
	// user-written code.
	for _, block := range fn.Blocks {
		if len(block.Instrs) < 3 {
			continue
		}
		if len(block.Succs) != 2 {
			continue
		}
		var instrs []*ssa.Instruction
		for i, ins := range block.Instrs {
			if _, ok := ins.(*ssa.DebugRef); ok {
				continue
			}
			instrs = append(instrs, &block.Instrs[i])
		}

		for i, ins := range instrs {
			unop, ok := (*ins).(*ssa.UnOp)
			if !ok || unop.Op != token.ARROW {
				continue
			}
			call, ok := unop.X.(*ssa.Call)
			if !ok {
				continue
			}
			if !IsCallTo(call.Common(), "time.Tick") {
				continue
			}
			ex, ok := (*instrs[i+1]).(*ssa.Extract)
			if !ok || ex.Tuple != unop || ex.Index != 1 {
				continue
			}

			ifstmt, ok := (*instrs[i+2]).(*ssa.If)
			if !ok || ifstmt.Cond != ex {
				continue
			}

			*instrs[i+2] = ssa.NewJump(block)
			succ := block.Succs[1]
			block.Succs = block.Succs[0:1]
			succ.RemovePred(block)
		}
	}
}

func hasType(j *lint.Job, expr ast.Expr, name string) bool {
	T := TypeOf(j, expr)
	return IsType(T, name)
}

func (c *Checker) CheckUntrappableSignal(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAnyAST(j, call,
			"os/signal.Ignore", "os/signal.Notify", "os/signal.Reset") {
			return true
		}
		for _, arg := range call.Args {
			if conv, ok := arg.(*ast.CallExpr); ok && isName(j, conv.Fun, "os.Signal") {
				arg = conv.Args[0]
			}

			if isName(j, arg, "os.Kill") || isName(j, arg, "syscall.SIGKILL") {
				j.Errorf(arg, "%s cannot be trapped (did you mean syscall.SIGTERM?)", Render(j, arg))
			}
			if isName(j, arg, "syscall.SIGSTOP") {
				j.Errorf(arg, "%s signal cannot be trapped", Render(j, arg))
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckTemplate(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var kind string
		if IsCallToAST(j, call, "(*text/template.Template).Parse") {
			kind = "text"
		} else if IsCallToAST(j, call, "(*html/template.Template).Parse") {
			kind = "html"
		} else {
			return true
		}
		sel := call.Fun.(*ast.SelectorExpr)
		if !IsCallToAST(j, sel.X, "text/template.New") &&
			!IsCallToAST(j, sel.X, "html/template.New") {
			// TODO(dh): this is a cheap workaround for templates with
			// different delims. A better solution with less false
			// negatives would use data flow analysis to see where the
			// template comes from and where it has been
			return true
		}
		s, ok := ExprToString(j, call.Args[Arg("(*text/template.Template).Parse.text")])
		if !ok {
			return true
		}
		var err error
		switch kind {
		case "text":
			_, err = texttemplate.New("").Parse(s)
		case "html":
			_, err = htmltemplate.New("").Parse(s)
		}
		if err != nil {
			// TODO(dominikh): whitelist other parse errors, if any
			if strings.Contains(err.Error(), "unexpected") {
				j.Errorf(call.Args[Arg("(*text/template.Template).Parse.text")], "%s", err)
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckTimeSleepConstant(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, call, "time.Sleep") {
			return true
		}
		lit, ok := call.Args[Arg("time.Sleep.d")].(*ast.BasicLit)
		if !ok {
			return true
		}
		n, err := strconv.Atoi(lit.Value)
		if err != nil {
			return true
		}
		if n == 0 || n > 120 {
			// time.Sleep(0) is a seldom used pattern in concurrency
			// tests. >120 might be intentional. 120 was chosen
			// because the user could've meant 2 minutes.
			return true
		}
		recommendation := "time.Sleep(time.Nanosecond)"
		if n != 1 {
			recommendation = fmt.Sprintf("time.Sleep(%d * time.Nanosecond)", n)
		}
		j.Errorf(call.Args[Arg("time.Sleep.d")],
			"sleeping for %d nanoseconds is probably a bug. Be explicit if it isn't: %s", n, recommendation)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckWaitgroupAdd(j *lint.Job) {
	fn := func(node ast.Node) bool {
		g, ok := node.(*ast.GoStmt)
		if !ok {
			return true
		}
		fun, ok := g.Call.Fun.(*ast.FuncLit)
		if !ok {
			return true
		}
		if len(fun.Body.List) == 0 {
			return true
		}
		stmt, ok := fun.Body.List[0].(*ast.ExprStmt)
		if !ok {
			return true
		}
		call, ok := stmt.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		fn, ok := ObjectOf(j, sel.Sel).(*types.Func)
		if !ok {
			return true
		}
		if fn.FullName() == "(*sync.WaitGroup).Add" {
			j.Errorf(sel, "should call %s before starting the goroutine to avoid a race",
				Render(j, stmt))
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckInfiniteEmptyLoop(j *lint.Job) {
	fn := func(node ast.Node) bool {
		loop, ok := node.(*ast.ForStmt)
		if !ok || len(loop.Body.List) != 0 || loop.Post != nil {
			return true
		}

		if loop.Init != nil {
			// TODO(dh): this isn't strictly necessary, it just makes
			// the check easier.
			return true
		}
		// An empty loop is bad news in two cases: 1) The loop has no
		// condition. In that case, it's just a loop that spins
		// forever and as fast as it can, keeping a core busy. 2) The
		// loop condition only consists of variable or field reads and
		// operators on those. The only way those could change their
		// value is with unsynchronised access, which constitutes a
		// data race.
		//
		// If the condition contains any function calls, its behaviour
		// is dynamic and the loop might terminate. Similarly for
		// channel receives.

		if loop.Cond != nil && hasSideEffects(loop.Cond) {
			return true
		}

		j.Errorf(loop, "this loop will spin, using 100%% CPU")
		if loop.Cond != nil {
			j.Errorf(loop, "loop condition never changes or has a race condition")
		}

		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckDeferInInfiniteLoop(j *lint.Job) {
	fn := func(node ast.Node) bool {
		mightExit := false
		var defers []ast.Stmt
		loop, ok := node.(*ast.ForStmt)
		if !ok || loop.Cond != nil {
			return true
		}
		fn2 := func(node ast.Node) bool {
			switch stmt := node.(type) {
			case *ast.ReturnStmt:
				mightExit = true
			case *ast.BranchStmt:
				// TODO(dominikh): if this sees a break in a switch or
				// select, it doesn't check if it breaks the loop or
				// just the select/switch. This causes some false
				// negatives.
				if stmt.Tok == token.BREAK {
					mightExit = true
				}
			case *ast.DeferStmt:
				defers = append(defers, stmt)
			case *ast.FuncLit:
				// Don't look into function bodies
				return false
			}
			return true
		}
		ast.Inspect(loop.Body, fn2)
		if mightExit {
			return true
		}
		for _, stmt := range defers {
			j.Errorf(stmt, "defers in this infinite loop will never run")
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckDubiousDeferInChannelRangeLoop(j *lint.Job) {
	fn := func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		typ := TypeOf(j, loop.X)
		_, ok = typ.Underlying().(*types.Chan)
		if !ok {
			return true
		}
		fn2 := func(node ast.Node) bool {
			switch stmt := node.(type) {
			case *ast.DeferStmt:
				j.Errorf(stmt, "defers in this range loop won't run unless the channel gets closed")
			case *ast.FuncLit:
				// Don't look into function bodies
				return false
			}
			return true
		}
		ast.Inspect(loop.Body, fn2)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckTestMainExit(j *lint.Job) {
	fn := func(node ast.Node) bool {
		if !isTestMain(j, node) {
			return true
		}

		arg := ObjectOf(j, node.(*ast.FuncDecl).Type.Params.List[0].Names[0])
		callsRun := false
		fn2 := func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if arg != ObjectOf(j, ident) {
				return true
			}
			if sel.Sel.Name == "Run" {
				callsRun = true
				return false
			}
			return true
		}
		ast.Inspect(node.(*ast.FuncDecl).Body, fn2)

		callsExit := false
		fn3 := func(node ast.Node) bool {
			if IsCallToAST(j, node, "os.Exit") {
				callsExit = true
				return false
			}
			return true
		}
		ast.Inspect(node.(*ast.FuncDecl).Body, fn3)
		if !callsExit && callsRun {
			j.Errorf(node, "TestMain should call os.Exit to set exit code")
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func isTestMain(j *lint.Job, node ast.Node) bool {
	decl, ok := node.(*ast.FuncDecl)
	if !ok {
		return false
	}
	if decl.Name.Name != "TestMain" {
		return false
	}
	if len(decl.Type.Params.List) != 1 {
		return false
	}
	arg := decl.Type.Params.List[0]
	if len(arg.Names) != 1 {
		return false
	}
	return IsOfType(j, arg.Type, "*testing.M")
}

func (c *Checker) CheckExec(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, call, "os/exec.Command") {
			return true
		}
		val, ok := ExprToString(j, call.Args[Arg("os/exec.Command.name")])
		if !ok {
			return true
		}
		if !strings.Contains(val, " ") || strings.Contains(val, `\`) || strings.Contains(val, "/") {
			return true
		}
		j.Errorf(call.Args[Arg("os/exec.Command.name")],
			"first argument to exec.Command looks like a shell command, but a program name or path are expected")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckLoopEmptyDefault(j *lint.Job) {
	fn := func(node ast.Node) bool {
		loop, ok := node.(*ast.ForStmt)
		if !ok || len(loop.Body.List) != 1 || loop.Cond != nil || loop.Init != nil {
			return true
		}
		sel, ok := loop.Body.List[0].(*ast.SelectStmt)
		if !ok {
			return true
		}
		for _, c := range sel.Body.List {
			if comm, ok := c.(*ast.CommClause); ok && comm.Comm == nil && len(comm.Body) == 0 {
				j.Errorf(comm, "should not have an empty default case in a for+select loop. The loop will spin.")
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckLhsRhsIdentical(j *lint.Job) {
	fn := func(node ast.Node) bool {
		op, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch op.Op {
		case token.EQL, token.NEQ:
			if basic, ok := TypeOf(j, op.X).(*types.Basic); ok {
				if kind := basic.Kind(); kind == types.Float32 || kind == types.Float64 {
					// f == f and f != f might be used to check for NaN
					return true
				}
			}
		case token.SUB, token.QUO, token.AND, token.REM, token.OR, token.XOR, token.AND_NOT,
			token.LAND, token.LOR, token.LSS, token.GTR, token.LEQ, token.GEQ:
		default:
			// For some ops, such as + and *, it can make sense to
			// have identical operands
			return true
		}

		if Render(j, op.X) != Render(j, op.Y) {
			return true
		}
		j.Errorf(op, "identical expressions on the left and right side of the '%s' operator", op.Op)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckScopedBreak(j *lint.Job) {
	fn := func(node ast.Node) bool {
		var body *ast.BlockStmt
		switch node := node.(type) {
		case *ast.ForStmt:
			body = node.Body
		case *ast.RangeStmt:
			body = node.Body
		default:
			return true
		}
		for _, stmt := range body.List {
			var blocks [][]ast.Stmt
			switch stmt := stmt.(type) {
			case *ast.SwitchStmt:
				for _, c := range stmt.Body.List {
					blocks = append(blocks, c.(*ast.CaseClause).Body)
				}
			case *ast.SelectStmt:
				for _, c := range stmt.Body.List {
					blocks = append(blocks, c.(*ast.CommClause).Body)
				}
			default:
				continue
			}

			for _, body := range blocks {
				if len(body) == 0 {
					continue
				}
				lasts := []ast.Stmt{body[len(body)-1]}
				// TODO(dh): unfold all levels of nested block
				// statements, not just a single level if statement
				if ifs, ok := lasts[0].(*ast.IfStmt); ok {
					if len(ifs.Body.List) == 0 {
						continue
					}
					lasts[0] = ifs.Body.List[len(ifs.Body.List)-1]

					if block, ok := ifs.Else.(*ast.BlockStmt); ok {
						if len(block.List) != 0 {
							lasts = append(lasts, block.List[len(block.List)-1])
						}
					}
				}
				for _, last := range lasts {
					branch, ok := last.(*ast.BranchStmt)
					if !ok || branch.Tok != token.BREAK || branch.Label != nil {
						continue
					}
					j.Errorf(branch, "ineffective break statement. Did you mean to break out of the outer loop?")
				}
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckUnsafePrintf(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAnyAST(j, call, "fmt.Printf", "fmt.Sprintf", "log.Printf") {
			return true
		}
		if len(call.Args) != 1 {
			return true
		}
		switch call.Args[Arg("fmt.Printf.format")].(type) {
		case *ast.CallExpr, *ast.Ident:
		default:
			return true
		}
		j.Errorf(call.Args[Arg("fmt.Printf.format")],
			"printf-style function with dynamic first argument and no further arguments should use print-style function instead")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckEarlyDefer(j *lint.Job) {
	fn := func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		if len(block.List) < 2 {
			return true
		}
		for i, stmt := range block.List {
			if i == len(block.List)-1 {
				break
			}
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok {
				continue
			}
			if len(assign.Rhs) != 1 {
				continue
			}
			if len(assign.Lhs) < 2 {
				continue
			}
			if lhs, ok := assign.Lhs[len(assign.Lhs)-1].(*ast.Ident); ok && lhs.Name == "_" {
				continue
			}
			call, ok := assign.Rhs[0].(*ast.CallExpr)
			if !ok {
				continue
			}
			sig, ok := TypeOf(j, call.Fun).(*types.Signature)
			if !ok {
				continue
			}
			if sig.Results().Len() < 2 {
				continue
			}
			last := sig.Results().At(sig.Results().Len() - 1)
			// FIXME(dh): check that it's error from universe, not
			// another type of the same name
			if last.Type().String() != "error" {
				continue
			}
			lhs, ok := assign.Lhs[0].(*ast.Ident)
			if !ok {
				continue
			}
			def, ok := block.List[i+1].(*ast.DeferStmt)
			if !ok {
				continue
			}
			sel, ok := def.Call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			ident, ok := selectorX(sel).(*ast.Ident)
			if !ok {
				continue
			}
			if ident.Obj != lhs.Obj {
				continue
			}
			if sel.Sel.Name != "Close" {
				continue
			}
			j.Errorf(def, "should check returned error before deferring %s", Render(j, def.Call))
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func selectorX(sel *ast.SelectorExpr) ast.Node {
	switch x := sel.X.(type) {
	case *ast.SelectorExpr:
		return selectorX(x)
	default:
		return x
	}
}

func (c *Checker) CheckEmptyCriticalSection(j *lint.Job) {
	// Initially it might seem like this check would be easier to
	// implement in SSA. After all, we're only checking for two
	// consecutive method calls. In reality, however, there may be any
	// number of other instructions between the lock and unlock, while
	// still constituting an empty critical section. For example,
	// given `m.x().Lock(); m.x().Unlock()`, there will be a call to
	// x(). In the AST-based approach, this has a tiny potential for a
	// false positive (the second call to x might be doing work that
	// is protected by the mutex). In an SSA-based approach, however,
	// it would miss a lot of real bugs.

	mutexParams := func(s ast.Stmt) (x ast.Expr, funcName string, ok bool) {
		expr, ok := s.(*ast.ExprStmt)
		if !ok {
			return nil, "", false
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			return nil, "", false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return nil, "", false
		}

		fn, ok := ObjectOf(j, sel.Sel).(*types.Func)
		if !ok {
			return nil, "", false
		}
		sig := fn.Type().(*types.Signature)
		if sig.Params().Len() != 0 || sig.Results().Len() != 0 {
			return nil, "", false
		}

		return sel.X, fn.Name(), true
	}

	fn := func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		if len(block.List) < 2 {
			return true
		}
		for i := range block.List[:len(block.List)-1] {
			sel1, method1, ok1 := mutexParams(block.List[i])
			sel2, method2, ok2 := mutexParams(block.List[i+1])

			if !ok1 || !ok2 || Render(j, sel1) != Render(j, sel2) {
				continue
			}
			if (method1 == "Lock" && method2 == "Unlock") ||
				(method1 == "RLock" && method2 == "RUnlock") {
				j.Errorf(block.List[i+1], "empty critical section")
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

// cgo produces code like fn(&*_Cvar_kSomeCallbacks) which we don't
// want to flag.
var cgoIdent = regexp.MustCompile(`^_C(func|var)_.+$`)

func (c *Checker) CheckIneffectiveCopy(j *lint.Job) {
	fn := func(node ast.Node) bool {
		if unary, ok := node.(*ast.UnaryExpr); ok {
			if star, ok := unary.X.(*ast.StarExpr); ok && unary.Op == token.AND {
				ident, ok := star.X.(*ast.Ident)
				if !ok || !cgoIdent.MatchString(ident.Name) {
					j.Errorf(unary, "&*x will be simplified to x. It will not copy x.")
				}
			}
		}

		if star, ok := node.(*ast.StarExpr); ok {
			if unary, ok := star.X.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				j.Errorf(star, "*&x will be simplified to x. It will not copy x.")
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckDiffSizeComparison(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, b := range ssafn.Blocks {
			for _, ins := range b.Instrs {
				binop, ok := ins.(*ssa.BinOp)
				if !ok {
					continue
				}
				if binop.Op != token.EQL && binop.Op != token.NEQ {
					continue
				}
				_, ok1 := binop.X.(*ssa.Slice)
				_, ok2 := binop.Y.(*ssa.Slice)
				if !ok1 && !ok2 {
					continue
				}
				r := c.funcDescs.Get(ssafn).Ranges
				r1, ok1 := r.Get(binop.X).(vrp.StringInterval)
				r2, ok2 := r.Get(binop.Y).(vrp.StringInterval)
				if !ok1 || !ok2 {
					continue
				}
				if r1.Length.Intersection(r2.Length).Empty() {
					j.Errorf(binop, "comparing strings of different sizes for equality will always return false")
				}
			}
		}
	}
}

func (c *Checker) CheckCanonicalHeaderKey(j *lint.Job) {
	fn := func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if ok {
			// TODO(dh): This risks missing some Header reads, for
			// example in `h1["foo"] = h2["foo"]` – these edge
			// cases are probably rare enough to ignore for now.
			for _, expr := range assign.Lhs {
				op, ok := expr.(*ast.IndexExpr)
				if !ok {
					continue
				}
				if hasType(j, op.X, "net/http.Header") {
					return false
				}
			}
			return true
		}
		op, ok := node.(*ast.IndexExpr)
		if !ok {
			return true
		}
		if !hasType(j, op.X, "net/http.Header") {
			return true
		}
		s, ok := ExprToString(j, op.Index)
		if !ok {
			return true
		}
		if s == http.CanonicalHeaderKey(s) {
			return true
		}
		j.Errorf(op, "keys in http.Header are canonicalized, %q is not canonical; fix the constant or use http.CanonicalHeaderKey", s)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckBenchmarkN(j *lint.Job) {
	fn := func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		sel, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "N" {
			return true
		}
		if !hasType(j, sel.X, "*testing.B") {
			return true
		}
		j.Errorf(assign, "should not assign to %s", Render(j, sel))
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckUnreadVariableValues(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		if IsExample(ssafn) {
			continue
		}
		node := ssafn.Syntax()
		if node == nil {
			continue
		}

		ast.Inspect(node, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			if len(assign.Lhs) > 1 && len(assign.Rhs) == 1 {
				// Either a function call with multiple return values,
				// or a comma-ok assignment

				val, _ := ssafn.ValueForExpr(assign.Rhs[0])
				if val == nil {
					return true
				}
				refs := val.Referrers()
				if refs == nil {
					return true
				}
				for _, ref := range *refs {
					ex, ok := ref.(*ssa.Extract)
					if !ok {
						continue
					}
					exrefs := ex.Referrers()
					if exrefs == nil {
						continue
					}
					if len(FilterDebug(*exrefs)) == 0 {
						lhs := assign.Lhs[ex.Index]
						if ident, ok := lhs.(*ast.Ident); !ok || ok && ident.Name == "_" {
							continue
						}
						j.Errorf(lhs, "this value of %s is never used", lhs)
					}
				}
				return true
			}
			for i, lhs := range assign.Lhs {
				rhs := assign.Rhs[i]
				if ident, ok := lhs.(*ast.Ident); !ok || ok && ident.Name == "_" {
					continue
				}
				val, _ := ssafn.ValueForExpr(rhs)
				if val == nil {
					continue
				}

				refs := val.Referrers()
				if refs == nil {
					// TODO investigate why refs can be nil
					return true
				}
				if len(FilterDebug(*refs)) == 0 {
					j.Errorf(lhs, "this value of %s is never used", lhs)
				}
			}
			return true
		})
	}
}

func (c *Checker) CheckPredeterminedBooleanExprs(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				ssabinop, ok := ins.(*ssa.BinOp)
				if !ok {
					continue
				}
				switch ssabinop.Op {
				case token.GTR, token.LSS, token.EQL, token.NEQ, token.LEQ, token.GEQ:
				default:
					continue
				}

				xs, ok1 := consts(ssabinop.X, nil, nil)
				ys, ok2 := consts(ssabinop.Y, nil, nil)
				if !ok1 || !ok2 || len(xs) == 0 || len(ys) == 0 {
					continue
				}

				trues := 0
				for _, x := range xs {
					for _, y := range ys {
						if x.Value == nil {
							if y.Value == nil {
								trues++
							}
							continue
						}
						if constant.Compare(x.Value, ssabinop.Op, y.Value) {
							trues++
						}
					}
				}
				b := trues != 0
				if trues == 0 || trues == len(xs)*len(ys) {
					j.Errorf(ssabinop, "binary expression is always %t for all possible values (%s %s %s)",
						b, xs, ssabinop.Op, ys)
				}
			}
		}
	}
}

func (c *Checker) CheckNilMaps(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				mu, ok := ins.(*ssa.MapUpdate)
				if !ok {
					continue
				}
				c, ok := mu.Map.(*ssa.Const)
				if !ok {
					continue
				}
				if c.Value != nil {
					continue
				}
				j.Errorf(mu, "assignment to nil map")
			}
		}
	}
}

func (c *Checker) CheckUnsignedComparison(j *lint.Job) {
	fn := func(node ast.Node) bool {
		expr, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		tx := TypeOf(j, expr.X)
		basic, ok := tx.Underlying().(*types.Basic)
		if !ok {
			return true
		}
		if (basic.Info() & types.IsUnsigned) == 0 {
			return true
		}
		lit, ok := expr.Y.(*ast.BasicLit)
		if !ok || lit.Value != "0" {
			return true
		}
		switch expr.Op {
		case token.GEQ:
			j.Errorf(expr, "unsigned values are always >= 0")
		case token.LSS:
			j.Errorf(expr, "unsigned values are never < 0")
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func consts(val ssa.Value, out []*ssa.Const, visitedPhis map[string]bool) ([]*ssa.Const, bool) {
	if visitedPhis == nil {
		visitedPhis = map[string]bool{}
	}
	var ok bool
	switch val := val.(type) {
	case *ssa.Phi:
		if visitedPhis[val.Name()] {
			break
		}
		visitedPhis[val.Name()] = true
		vals := val.Operands(nil)
		for _, phival := range vals {
			out, ok = consts(*phival, out, visitedPhis)
			if !ok {
				return nil, false
			}
		}
	case *ssa.Const:
		out = append(out, val)
	case *ssa.Convert:
		out, ok = consts(val.X, out, visitedPhis)
		if !ok {
			return nil, false
		}
	default:
		return nil, false
	}
	if len(out) < 2 {
		return out, true
	}
	uniq := []*ssa.Const{out[0]}
	for _, val := range out[1:] {
		if val.Value == uniq[len(uniq)-1].Value {
			continue
		}
		uniq = append(uniq, val)
	}
	return uniq, true
}

func (c *Checker) CheckLoopCondition(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		fn := func(node ast.Node) bool {
			loop, ok := node.(*ast.ForStmt)
			if !ok {
				return true
			}
			if loop.Init == nil || loop.Cond == nil || loop.Post == nil {
				return true
			}
			init, ok := loop.Init.(*ast.AssignStmt)
			if !ok || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
				return true
			}
			cond, ok := loop.Cond.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			x, ok := cond.X.(*ast.Ident)
			if !ok {
				return true
			}
			lhs, ok := init.Lhs[0].(*ast.Ident)
			if !ok {
				return true
			}
			if x.Obj != lhs.Obj {
				return true
			}
			if _, ok := loop.Post.(*ast.IncDecStmt); !ok {
				return true
			}

			v, isAddr := ssafn.ValueForExpr(cond.X)
			if v == nil || isAddr {
				return true
			}
			switch v := v.(type) {
			case *ssa.Phi:
				ops := v.Operands(nil)
				if len(ops) != 2 {
					return true
				}
				_, ok := (*ops[0]).(*ssa.Const)
				if !ok {
					return true
				}
				sigma, ok := (*ops[1]).(*ssa.Sigma)
				if !ok {
					return true
				}
				if sigma.X != v {
					return true
				}
			case *ssa.UnOp:
				return true
			}
			j.Errorf(cond, "variable in loop condition never changes")

			return true
		}
		Inspect(ssafn.Syntax(), fn)
	}
}

func (c *Checker) CheckArgOverwritten(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		fn := func(node ast.Node) bool {
			var typ *ast.FuncType
			var body *ast.BlockStmt
			switch fn := node.(type) {
			case *ast.FuncDecl:
				typ = fn.Type
				body = fn.Body
			case *ast.FuncLit:
				typ = fn.Type
				body = fn.Body
			}
			if body == nil {
				return true
			}
			if len(typ.Params.List) == 0 {
				return true
			}
			for _, field := range typ.Params.List {
				for _, arg := range field.Names {
					obj := ObjectOf(j, arg)
					var ssaobj *ssa.Parameter
					for _, param := range ssafn.Params {
						if param.Object() == obj {
							ssaobj = param
							break
						}
					}
					if ssaobj == nil {
						continue
					}
					refs := ssaobj.Referrers()
					if refs == nil {
						continue
					}
					if len(FilterDebug(*refs)) != 0 {
						continue
					}

					assigned := false
					ast.Inspect(body, func(node ast.Node) bool {
						assign, ok := node.(*ast.AssignStmt)
						if !ok {
							return true
						}
						for _, lhs := range assign.Lhs {
							ident, ok := lhs.(*ast.Ident)
							if !ok {
								continue
							}
							if ObjectOf(j, ident) == obj {
								assigned = true
								return false
							}
						}
						return true
					})
					if assigned {
						j.Errorf(arg, "argument %s is overwritten before first use", arg)
					}
				}
			}
			return true
		}
		Inspect(ssafn.Syntax(), fn)
	}
}

func (c *Checker) CheckIneffectiveLoop(j *lint.Job) {
	// This check detects some, but not all unconditional loop exits.
	// We give up in the following cases:
	//
	// - a goto anywhere in the loop. The goto might skip over our
	// return, and we don't check that it doesn't.
	//
	// - any nested, unlabelled continue, even if it is in another
	// loop or closure.
	fn := func(node ast.Node) bool {
		var body *ast.BlockStmt
		switch fn := node.(type) {
		case *ast.FuncDecl:
			body = fn.Body
		case *ast.FuncLit:
			body = fn.Body
		default:
			return true
		}
		if body == nil {
			return true
		}
		labels := map[*ast.Object]ast.Stmt{}
		ast.Inspect(body, func(node ast.Node) bool {
			label, ok := node.(*ast.LabeledStmt)
			if !ok {
				return true
			}
			labels[label.Label.Obj] = label.Stmt
			return true
		})

		ast.Inspect(body, func(node ast.Node) bool {
			var loop ast.Node
			var body *ast.BlockStmt
			switch node := node.(type) {
			case *ast.ForStmt:
				body = node.Body
				loop = node
			case *ast.RangeStmt:
				typ := TypeOf(j, node.X)
				if _, ok := typ.Underlying().(*types.Map); ok {
					// looping once over a map is a valid pattern for
					// getting an arbitrary element.
					return true
				}
				body = node.Body
				loop = node
			default:
				return true
			}
			if len(body.List) < 2 {
				// avoid flagging the somewhat common pattern of using
				// a range loop to get the first element in a slice,
				// or the first rune in a string.
				return true
			}
			var unconditionalExit ast.Node
			hasBranching := false
			for _, stmt := range body.List {
				switch stmt := stmt.(type) {
				case *ast.BranchStmt:
					switch stmt.Tok {
					case token.BREAK:
						if stmt.Label == nil || labels[stmt.Label.Obj] == loop {
							unconditionalExit = stmt
						}
					case token.CONTINUE:
						if stmt.Label == nil || labels[stmt.Label.Obj] == loop {
							unconditionalExit = nil
							return false
						}
					}
				case *ast.ReturnStmt:
					unconditionalExit = stmt
				case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.SelectStmt:
					hasBranching = true
				}
			}
			if unconditionalExit == nil || !hasBranching {
				return false
			}
			ast.Inspect(body, func(node ast.Node) bool {
				if branch, ok := node.(*ast.BranchStmt); ok {

					switch branch.Tok {
					case token.GOTO:
						unconditionalExit = nil
						return false
					case token.CONTINUE:
						if branch.Label != nil && labels[branch.Label.Obj] != loop {
							return true
						}
						unconditionalExit = nil
						return false
					}
				}
				return true
			})
			if unconditionalExit != nil {
				j.Errorf(unconditionalExit, "the surrounding loop is unconditionally terminated")
			}
			return true
		})
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckNilContext(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}
		if typ, ok := TypeOf(j, call.Args[0]).(*types.Basic); !ok || typ.Kind() != types.UntypedNil {
			return true
		}
		sig, ok := TypeOf(j, call.Fun).(*types.Signature)
		if !ok {
			return true
		}
		if sig.Params().Len() == 0 {
			return true
		}
		if !IsType(sig.Params().At(0).Type(), "context.Context") {
			return true
		}
		j.Errorf(call.Args[0],
			"do not pass a nil Context, even if a function permits it; pass context.TODO if you are unsure about which Context to use")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckSeeker(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if sel.Sel.Name != "Seek" {
			return true
		}
		if len(call.Args) != 2 {
			return true
		}
		arg0, ok := call.Args[Arg("(io.Seeker).Seek.offset")].(*ast.SelectorExpr)
		if !ok {
			return true
		}
		switch arg0.Sel.Name {
		case "SeekStart", "SeekCurrent", "SeekEnd":
		default:
			return true
		}
		pkg, ok := arg0.X.(*ast.Ident)
		if !ok {
			return true
		}
		if pkg.Name != "io" {
			return true
		}
		j.Errorf(call, "the first argument of io.Seeker is the offset, but an io.Seek* constant is being used instead")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckIneffectiveAppend(j *lint.Job) {
	isAppend := func(ins ssa.Value) bool {
		call, ok := ins.(*ssa.Call)
		if !ok {
			return false
		}
		if call.Call.IsInvoke() {
			return false
		}
		if builtin, ok := call.Call.Value.(*ssa.Builtin); !ok || builtin.Name() != "append" {
			return false
		}
		return true
	}

	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				val, ok := ins.(ssa.Value)
				if !ok || !isAppend(val) {
					continue
				}

				isUsed := false
				visited := map[ssa.Instruction]bool{}
				var walkRefs func(refs []ssa.Instruction)
				walkRefs = func(refs []ssa.Instruction) {
				loop:
					for _, ref := range refs {
						if visited[ref] {
							continue
						}
						visited[ref] = true
						if _, ok := ref.(*ssa.DebugRef); ok {
							continue
						}
						switch ref := ref.(type) {
						case *ssa.Phi:
							walkRefs(*ref.Referrers())
						case *ssa.Sigma:
							walkRefs(*ref.Referrers())
						case ssa.Value:
							if !isAppend(ref) {
								isUsed = true
							} else {
								walkRefs(*ref.Referrers())
							}
						case ssa.Instruction:
							isUsed = true
							break loop
						}
					}
				}
				refs := val.Referrers()
				if refs == nil {
					continue
				}
				walkRefs(*refs)
				if !isUsed {
					j.Errorf(ins, "this result of append is never used, except maybe in other appends")
				}
			}
		}
	}
}

func (c *Checker) CheckConcurrentTesting(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				gostmt, ok := ins.(*ssa.Go)
				if !ok {
					continue
				}
				var fn *ssa.Function
				switch val := gostmt.Call.Value.(type) {
				case *ssa.Function:
					fn = val
				case *ssa.MakeClosure:
					fn = val.Fn.(*ssa.Function)
				default:
					continue
				}
				if fn.Blocks == nil {
					continue
				}
				for _, block := range fn.Blocks {
					for _, ins := range block.Instrs {
						call, ok := ins.(*ssa.Call)
						if !ok {
							continue
						}
						if call.Call.IsInvoke() {
							continue
						}
						callee := call.Call.StaticCallee()
						if callee == nil {
							continue
						}
						recv := callee.Signature.Recv()
						if recv == nil {
							continue
						}
						if !IsType(recv.Type(), "*testing.common") {
							continue
						}
						fn, ok := call.Call.StaticCallee().Object().(*types.Func)
						if !ok {
							continue
						}
						name := fn.Name()
						switch name {
						case "FailNow", "Fatal", "Fatalf", "SkipNow", "Skip", "Skipf":
						default:
							continue
						}
						j.Errorf(gostmt, "the goroutine calls T.%s, which must be called in the same goroutine as the test", name)
					}
				}
			}
		}
	}
}

func (c *Checker) CheckCyclicFinalizer(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		node := c.funcDescs.CallGraph.CreateNode(ssafn)
		for _, edge := range node.Out {
			if edge.Callee.Func.RelString(nil) != "runtime.SetFinalizer" {
				continue
			}
			arg0 := edge.Site.Common().Args[Arg("runtime.SetFinalizer.obj")]
			if iface, ok := arg0.(*ssa.MakeInterface); ok {
				arg0 = iface.X
			}
			unop, ok := arg0.(*ssa.UnOp)
			if !ok {
				continue
			}
			v, ok := unop.X.(*ssa.Alloc)
			if !ok {
				continue
			}
			arg1 := edge.Site.Common().Args[Arg("runtime.SetFinalizer.finalizer")]
			if iface, ok := arg1.(*ssa.MakeInterface); ok {
				arg1 = iface.X
			}
			mc, ok := arg1.(*ssa.MakeClosure)
			if !ok {
				continue
			}
			for _, b := range mc.Bindings {
				if b == v {
					pos := j.Program.DisplayPosition(mc.Fn.Pos())
					j.Errorf(edge.Site, "the finalizer closes over the object, preventing the finalizer from ever running (at %s)", pos)
				}
			}
		}
	}
}

func (c *Checker) CheckSliceOutOfBounds(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				ia, ok := ins.(*ssa.IndexAddr)
				if !ok {
					continue
				}
				if _, ok := ia.X.Type().Underlying().(*types.Slice); !ok {
					continue
				}
				sr, ok1 := c.funcDescs.Get(ssafn).Ranges[ia.X].(vrp.SliceInterval)
				idxr, ok2 := c.funcDescs.Get(ssafn).Ranges[ia.Index].(vrp.IntInterval)
				if !ok1 || !ok2 || !sr.IsKnown() || !idxr.IsKnown() || sr.Length.Empty() || idxr.Empty() {
					continue
				}
				if idxr.Lower.Cmp(sr.Length.Upper) >= 0 {
					j.Errorf(ia, "index out of bounds")
				}
			}
		}
	}
}

func (c *Checker) CheckDeferLock(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			instrs := FilterDebug(block.Instrs)
			if len(instrs) < 2 {
				continue
			}
			for i, ins := range instrs[:len(instrs)-1] {
				call, ok := ins.(*ssa.Call)
				if !ok {
					continue
				}
				if !IsCallTo(call.Common(), "(*sync.Mutex).Lock") && !IsCallTo(call.Common(), "(*sync.RWMutex).RLock") {
					continue
				}
				nins, ok := instrs[i+1].(*ssa.Defer)
				if !ok {
					continue
				}
				if !IsCallTo(&nins.Call, "(*sync.Mutex).Lock") && !IsCallTo(&nins.Call, "(*sync.RWMutex).RLock") {
					continue
				}
				if call.Common().Args[0] != nins.Call.Args[0] {
					continue
				}
				name := shortCallName(call.Common())
				alt := ""
				switch name {
				case "Lock":
					alt = "Unlock"
				case "RLock":
					alt = "RUnlock"
				}
				j.Errorf(nins, "deferring %s right after having locked already; did you mean to defer %s?", name, alt)
			}
		}
	}
}

func (c *Checker) CheckNaNComparison(j *lint.Job) {
	isNaN := func(v ssa.Value) bool {
		call, ok := v.(*ssa.Call)
		if !ok {
			return false
		}
		return IsCallTo(call.Common(), "math.NaN")
	}
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				ins, ok := ins.(*ssa.BinOp)
				if !ok {
					continue
				}
				if isNaN(ins.X) || isNaN(ins.Y) {
					j.Errorf(ins, "no value is equal to NaN, not even NaN itself")
				}
			}
		}
	}
}

func (c *Checker) CheckInfiniteRecursion(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		node := c.funcDescs.CallGraph.CreateNode(ssafn)
		for _, edge := range node.Out {
			if edge.Callee != node {
				continue
			}
			if _, ok := edge.Site.(*ssa.Go); ok {
				// Recursively spawning goroutines doesn't consume
				// stack space infinitely, so don't flag it.
				continue
			}

			block := edge.Site.Block()
			canReturn := false
			for _, b := range ssafn.Blocks {
				if block.Dominates(b) {
					continue
				}
				if len(b.Instrs) == 0 {
					continue
				}
				if _, ok := b.Instrs[len(b.Instrs)-1].(*ssa.Return); ok {
					canReturn = true
					break
				}
			}
			if canReturn {
				continue
			}
			j.Errorf(edge.Site, "infinite recursive call")
		}
	}
}

func objectName(obj types.Object) string {
	if obj == nil {
		return "<nil>"
	}
	var name string
	if obj.Pkg() != nil && obj.Pkg().Scope().Lookup(obj.Name()) == obj {
		s := obj.Pkg().Path()
		if s != "" {
			name += s + "."
		}
	}
	name += obj.Name()
	return name
}

func isName(j *lint.Job, expr ast.Expr, name string) bool {
	var obj types.Object
	switch expr := expr.(type) {
	case *ast.Ident:
		obj = ObjectOf(j, expr)
	case *ast.SelectorExpr:
		obj = ObjectOf(j, expr.Sel)
	}
	return objectName(obj) == name
}

func (c *Checker) CheckLeakyTimeTick(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		if IsInMain(j, ssafn) || IsInTest(j, ssafn) {
			continue
		}
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				call, ok := ins.(*ssa.Call)
				if !ok || !IsCallTo(call.Common(), "time.Tick") {
					continue
				}
				if c.funcDescs.Get(call.Parent()).Infinite {
					continue
				}
				j.Errorf(call, "using time.Tick leaks the underlying ticker, consider using it only in endless functions, tests and the main package, and use time.NewTicker here")
			}
		}
	}
}

func (c *Checker) CheckDoubleNegation(j *lint.Job) {
	fn := func(node ast.Node) bool {
		unary1, ok := node.(*ast.UnaryExpr)
		if !ok {
			return true
		}
		unary2, ok := unary1.X.(*ast.UnaryExpr)
		if !ok {
			return true
		}
		if unary1.Op != token.NOT || unary2.Op != token.NOT {
			return true
		}
		j.Errorf(unary1, "negating a boolean twice has no effect; is this a typo?")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func hasSideEffects(node ast.Node) bool {
	dynamic := false
	ast.Inspect(node, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.CallExpr:
			dynamic = true
			return false
		case *ast.UnaryExpr:
			if node.Op == token.ARROW {
				dynamic = true
				return false
			}
		}
		return true
	})
	return dynamic
}

func (c *Checker) CheckRepeatedIfElse(j *lint.Job) {
	seen := map[ast.Node]bool{}

	var collectConds func(ifstmt *ast.IfStmt, inits []ast.Stmt, conds []ast.Expr) ([]ast.Stmt, []ast.Expr)
	collectConds = func(ifstmt *ast.IfStmt, inits []ast.Stmt, conds []ast.Expr) ([]ast.Stmt, []ast.Expr) {
		seen[ifstmt] = true
		if ifstmt.Init != nil {
			inits = append(inits, ifstmt.Init)
		}
		conds = append(conds, ifstmt.Cond)
		if elsestmt, ok := ifstmt.Else.(*ast.IfStmt); ok {
			return collectConds(elsestmt, inits, conds)
		}
		return inits, conds
	}
	fn := func(node ast.Node) bool {
		ifstmt, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		if seen[ifstmt] {
			return true
		}
		inits, conds := collectConds(ifstmt, nil, nil)
		if len(inits) > 0 {
			return true
		}
		for _, cond := range conds {
			if hasSideEffects(cond) {
				return true
			}
		}
		counts := map[string]int{}
		for _, cond := range conds {
			s := Render(j, cond)
			counts[s]++
			if counts[s] == 2 {
				j.Errorf(cond, "this condition occurs multiple times in this if/else if chain")
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckSillyBitwiseOps(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				ins, ok := ins.(*ssa.BinOp)
				if !ok {
					continue
				}

				if c, ok := ins.Y.(*ssa.Const); !ok || c.Value == nil || c.Value.Kind() != constant.Int || c.Uint64() != 0 {
					continue
				}
				switch ins.Op {
				case token.AND, token.OR, token.XOR:
				default:
					// we do not flag shifts because too often, x<<0 is part
					// of a pattern, x<<0, x<<8, x<<16, ...
					continue
				}
				path, _ := astutil.PathEnclosingInterval(j.File(ins), ins.Pos(), ins.Pos())
				if len(path) == 0 {
					continue
				}
				if node, ok := path[0].(*ast.BinaryExpr); !ok || !IsZero(node.Y) {
					continue
				}

				switch ins.Op {
				case token.AND:
					j.Errorf(ins, "x & 0 always equals 0")
				case token.OR, token.XOR:
					j.Errorf(ins, "x %s 0 always equals x", ins.Op)
				}
			}
		}
	}
}

func (c *Checker) CheckNonOctalFileMode(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sig, ok := TypeOf(j, call.Fun).(*types.Signature)
		if !ok {
			return true
		}
		n := sig.Params().Len()
		var args []int
		for i := 0; i < n; i++ {
			typ := sig.Params().At(i).Type()
			if IsType(typ, "os.FileMode") {
				args = append(args, i)
			}
		}
		for _, i := range args {
			lit, ok := call.Args[i].(*ast.BasicLit)
			if !ok {
				continue
			}
			if len(lit.Value) == 3 &&
				lit.Value[0] != '0' &&
				lit.Value[0] >= '0' && lit.Value[0] <= '7' &&
				lit.Value[1] >= '0' && lit.Value[1] <= '7' &&
				lit.Value[2] >= '0' && lit.Value[2] <= '7' {

				v, err := strconv.ParseInt(lit.Value, 10, 64)
				if err != nil {
					continue
				}
				j.Errorf(call.Args[i], "file mode '%s' evaluates to %#o; did you mean '0%s'?", lit.Value, v, lit.Value)
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckPureFunctions(j *lint.Job) {
fnLoop:
	for _, ssafn := range j.Program.InitialFunctions {
		if IsInTest(j, ssafn) {
			params := ssafn.Signature.Params()
			for i := 0; i < params.Len(); i++ {
				param := params.At(i)
				if IsType(param.Type(), "*testing.B") {
					// Ignore discarded pure functions in code related
					// to benchmarks. Instead of matching BenchmarkFoo
					// functions, we match any function accepting a
					// *testing.B. Benchmarks sometimes call generic
					// functions for doing the actual work, and
					// checking for the parameter is a lot easier and
					// faster than analyzing call trees.
					continue fnLoop
				}
			}
		}

		for _, b := range ssafn.Blocks {
			for _, ins := range b.Instrs {
				ins, ok := ins.(*ssa.Call)
				if !ok {
					continue
				}
				refs := ins.Referrers()
				if refs == nil || len(FilterDebug(*refs)) > 0 {
					continue
				}
				callee := ins.Common().StaticCallee()
				if callee == nil {
					continue
				}
				if c.funcDescs.Get(callee).Pure && !c.funcDescs.Get(callee).Stub {
					j.Errorf(ins, "%s is a pure function but its return value is ignored", callee.Name())
					continue
				}
			}
		}
	}
}

func (c *Checker) isDeprecated(j *lint.Job, ident *ast.Ident) (bool, string) {
	obj := ObjectOf(j, ident)
	if obj.Pkg() == nil {
		return false, ""
	}
	alt := c.deprecatedObjs[obj]
	return alt != "", alt
}

func (c *Checker) CheckDeprecated(j *lint.Job) {
	// Selectors can appear outside of function literals, e.g. when
	// declaring package level variables.

	var ssafn *ssa.Function
	stack := 0
	fn := func(node ast.Node) bool {
		if node == nil {
			stack--
		} else {
			stack++
		}
		if stack == 1 {
			ssafn = nil
		}
		if fn, ok := node.(*ast.FuncDecl); ok {
			ssafn = j.Program.SSA.FuncValue(ObjectOf(j, fn.Name).(*types.Func))
		}
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		obj := ObjectOf(j, sel.Sel)
		if obj.Pkg() == nil {
			return true
		}
		nodePkg := j.NodePackage(node).Types
		if nodePkg == obj.Pkg() || obj.Pkg().Path()+"_test" == nodePkg.Path() {
			// Don't flag stuff in our own package
			return true
		}
		if ok, alt := c.isDeprecated(j, sel.Sel); ok {
			// Look for the first available alternative, not the first
			// version something was deprecated in. If a function was
			// deprecated in Go 1.6, an alternative has been available
			// already in 1.0, and we're targeting 1.2, it still
			// makes sense to use the alternative from 1.0, to be
			// future-proof.
			minVersion := deprecated.Stdlib[SelectorName(j, sel)].AlternativeAvailableSince
			if !IsGoVersion(j, minVersion) {
				return true
			}

			if ssafn != nil {
				if _, ok := c.deprecatedObjs[ssafn.Object()]; ok {
					// functions that are deprecated may use deprecated
					// symbols
					return true
				}
			}
			j.Errorf(sel, "%s is deprecated: %s", Render(j, sel), alt)
			return true
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) callChecker(rules map[string]CallCheck) func(j *lint.Job) {
	return func(j *lint.Job) {
		c.checkCalls(j, rules)
	}
}

func (c *Checker) checkCalls(j *lint.Job, rules map[string]CallCheck) {
	for _, ssafn := range j.Program.InitialFunctions {
		node := c.funcDescs.CallGraph.CreateNode(ssafn)
		for _, edge := range node.Out {
			callee := edge.Callee.Func
			obj, ok := callee.Object().(*types.Func)
			if !ok {
				continue
			}

			r, ok := rules[obj.FullName()]
			if !ok {
				continue
			}
			var args []*Argument
			ssaargs := edge.Site.Common().Args
			if callee.Signature.Recv() != nil {
				ssaargs = ssaargs[1:]
			}
			for _, arg := range ssaargs {
				if iarg, ok := arg.(*ssa.MakeInterface); ok {
					arg = iarg.X
				}
				vr := c.funcDescs.Get(edge.Site.Parent()).Ranges[arg]
				args = append(args, &Argument{Value: Value{arg, vr}})
			}
			call := &Call{
				Job:     j,
				Instr:   edge.Site,
				Args:    args,
				Checker: c,
				Parent:  edge.Site.Parent(),
			}
			r(call)
			for idx, arg := range call.Args {
				_ = idx
				for _, e := range arg.invalids {
					// path, _ := astutil.PathEnclosingInterval(f.File, edge.Site.Pos(), edge.Site.Pos())
					// if len(path) < 2 {
					// 	continue
					// }
					// astcall, ok := path[0].(*ast.CallExpr)
					// if !ok {
					// 	continue
					// }
					// j.Errorf(astcall.Args[idx], "%s", e)

					j.Errorf(edge.Site, "%s", e)
				}
			}
			for _, e := range call.invalids {
				j.Errorf(call.Instr.Common(), "%s", e)
			}
		}
	}
}

func shortCallName(call *ssa.CallCommon) string {
	if call.IsInvoke() {
		return ""
	}
	switch v := call.Value.(type) {
	case *ssa.Function:
		fn, ok := v.Object().(*types.Func)
		if !ok {
			return ""
		}
		return fn.Name()
	case *ssa.Builtin:
		return v.Name()
	}
	return ""
}

func (c *Checker) CheckWriterBufferModified(j *lint.Job) {
	// TODO(dh): this might be a good candidate for taint analysis.
	// Taint the argument as MUST_NOT_MODIFY, then propagate that
	// through functions like bytes.Split

	for _, ssafn := range j.Program.InitialFunctions {
		sig := ssafn.Signature
		if ssafn.Name() != "Write" || sig.Recv() == nil || sig.Params().Len() != 1 || sig.Results().Len() != 2 {
			continue
		}
		tArg, ok := sig.Params().At(0).Type().(*types.Slice)
		if !ok {
			continue
		}
		if basic, ok := tArg.Elem().(*types.Basic); !ok || basic.Kind() != types.Byte {
			continue
		}
		if basic, ok := sig.Results().At(0).Type().(*types.Basic); !ok || basic.Kind() != types.Int {
			continue
		}
		if named, ok := sig.Results().At(1).Type().(*types.Named); !ok || !IsType(named, "error") {
			continue
		}

		for _, block := range ssafn.Blocks {
			for _, ins := range block.Instrs {
				switch ins := ins.(type) {
				case *ssa.Store:
					addr, ok := ins.Addr.(*ssa.IndexAddr)
					if !ok {
						continue
					}
					if addr.X != ssafn.Params[1] {
						continue
					}
					j.Errorf(ins, "io.Writer.Write must not modify the provided buffer, not even temporarily")
				case *ssa.Call:
					if !IsCallTo(ins.Common(), "append") {
						continue
					}
					if ins.Common().Args[0] != ssafn.Params[1] {
						continue
					}
					j.Errorf(ins, "io.Writer.Write must not modify the provided buffer, not even temporarily")
				}
			}
		}
	}
}

func loopedRegexp(name string) CallCheck {
	return func(call *Call) {
		if len(extractConsts(call.Args[0].Value.Value)) == 0 {
			return
		}
		if !call.Checker.isInLoop(call.Instr.Block()) {
			return
		}
		call.Invalid(fmt.Sprintf("calling %s in a loop has poor performance, consider using regexp.Compile", name))
	}
}

func (c *Checker) CheckEmptyBranch(j *lint.Job) {
	for _, ssafn := range j.Program.InitialFunctions {
		if ssafn.Syntax() == nil {
			continue
		}
		if IsGenerated(j.File(ssafn.Syntax())) {
			continue
		}
		if IsExample(ssafn) {
			continue
		}
		fn := func(node ast.Node) bool {
			ifstmt, ok := node.(*ast.IfStmt)
			if !ok {
				return true
			}
			if ifstmt.Else != nil {
				b, ok := ifstmt.Else.(*ast.BlockStmt)
				if !ok || len(b.List) != 0 {
					return true
				}
				j.Errorf(ifstmt.Else, "empty branch")
			}
			if len(ifstmt.Body.List) != 0 {
				return true
			}
			j.Errorf(ifstmt, "empty branch")
			return true
		}
		Inspect(ssafn.Syntax(), fn)
	}
}

func (c *Checker) CheckMapBytesKey(j *lint.Job) {
	for _, fn := range j.Program.InitialFunctions {
		for _, b := range fn.Blocks {
		insLoop:
			for _, ins := range b.Instrs {
				// find []byte -> string conversions
				conv, ok := ins.(*ssa.Convert)
				if !ok || conv.Type() != types.Universe.Lookup("string").Type() {
					continue
				}
				if s, ok := conv.X.Type().(*types.Slice); !ok || s.Elem() != types.Universe.Lookup("byte").Type() {
					continue
				}
				refs := conv.Referrers()
				// need at least two (DebugRef) references: the
				// conversion and the *ast.Ident
				if refs == nil || len(*refs) < 2 {
					continue
				}
				ident := false
				// skip first reference, that's the conversion itself
				for _, ref := range (*refs)[1:] {
					switch ref := ref.(type) {
					case *ssa.DebugRef:
						if _, ok := ref.Expr.(*ast.Ident); !ok {
							// the string seems to be used somewhere
							// unexpected; the default branch should
							// catch this already, but be safe
							continue insLoop
						} else {
							ident = true
						}
					case *ssa.Lookup:
					default:
						// the string is used somewhere else than a
						// map lookup
						continue insLoop
					}
				}

				// the result of the conversion wasn't assigned to an
				// identifier
				if !ident {
					continue
				}
				j.Errorf(conv, "m[string(key)] would be more efficient than k := string(key); m[k]")
			}
		}
	}
}

func (c *Checker) CheckRangeStringRunes(j *lint.Job) {
	sharedcheck.CheckRangeStringRunes(j)
}

func (c *Checker) CheckSelfAssignment(j *lint.Job) {
	fn := func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		if assign.Tok != token.ASSIGN || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, stmt := range assign.Lhs {
			rlh := Render(j, stmt)
			rrh := Render(j, assign.Rhs[i])
			if rlh == rrh {
				j.Errorf(assign, "self-assignment of %s to %s", rrh, rlh)
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func buildTagsIdentical(s1, s2 []string) bool {
	if len(s1) != len(s2) {
		return false
	}
	s1s := make([]string, len(s1))
	copy(s1s, s1)
	sort.Strings(s1s)
	s2s := make([]string, len(s2))
	copy(s2s, s2)
	sort.Strings(s2s)
	for i, s := range s1s {
		if s != s2s[i] {
			return false
		}
	}
	return true
}

func (c *Checker) CheckDuplicateBuildConstraints(job *lint.Job) {
	for _, f := range job.Program.Files {
		constraints := buildTags(f)
		for i, constraint1 := range constraints {
			for j, constraint2 := range constraints {
				if i >= j {
					continue
				}
				if buildTagsIdentical(constraint1, constraint2) {
					job.Errorf(f, "identical build constraints %q and %q",
						strings.Join(constraint1, " "),
						strings.Join(constraint2, " "))
				}
			}
		}
	}
}

func (c *Checker) CheckSillyRegexp(j *lint.Job) {
	// We could use the rule checking engine for this, but the
	// arguments aren't really invalid.
	for _, fn := range j.Program.InitialFunctions {
		for _, b := range fn.Blocks {
			for _, ins := range b.Instrs {
				call, ok := ins.(*ssa.Call)
				if !ok {
					continue
				}
				switch CallName(call.Common()) {
				case "regexp.MustCompile", "regexp.Compile", "regexp.Match", "regexp.MatchReader", "regexp.MatchString":
				default:
					continue
				}
				c, ok := call.Common().Args[0].(*ssa.Const)
				if !ok {
					continue
				}
				s := constant.StringVal(c.Value)
				re, err := syntax.Parse(s, 0)
				if err != nil {
					continue
				}
				if re.Op != syntax.OpLiteral && re.Op != syntax.OpEmptyMatch {
					continue
				}
				j.Errorf(call, "regular expression does not contain any meta characters")
			}
		}
	}
}

func (c *Checker) CheckMissingEnumTypesInDeclaration(j *lint.Job) {
	fn := func(node ast.Node) bool {
		decl, ok := node.(*ast.GenDecl)
		if !ok {
			return true
		}
		if !decl.Lparen.IsValid() {
			// not a parenthesised gendecl
			//
			// TODO(dh): do we need this check, considering we require
			// decl.Specs to contain 2+ elements?
			return true
		}
		if decl.Tok != token.CONST {
			return true
		}
		if len(decl.Specs) < 2 {
			return true
		}
		if decl.Specs[0].(*ast.ValueSpec).Type == nil {
			// first constant doesn't have a type
			return true
		}
		for i, spec := range decl.Specs {
			spec := spec.(*ast.ValueSpec)
			if len(spec.Names) != 1 || len(spec.Values) != 1 {
				return true
			}
			switch v := spec.Values[0].(type) {
			case *ast.BasicLit:
			case *ast.UnaryExpr:
				if _, ok := v.X.(*ast.BasicLit); !ok {
					return true
				}
			default:
				// if it's not a literal it might be typed, such as
				// time.Microsecond = 1000 * Nanosecond
				return true
			}
			if i == 0 {
				continue
			}
			if spec.Type != nil {
				return true
			}
		}
		j.Errorf(decl, "only the first constant has an explicit type")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) CheckTimerResetReturnValue(j *lint.Job) {
	for _, fn := range j.Program.InitialFunctions {
		for _, block := range fn.Blocks {
			for _, ins := range block.Instrs {
				call, ok := ins.(*ssa.Call)
				if !ok {
					continue
				}
				if !IsCallTo(call.Common(), "(*time.Timer).Reset") {
					continue
				}
				refs := call.Referrers()
				if refs == nil {
					continue
				}
				for _, ref := range FilterDebug(*refs) {
					ifstmt, ok := ref.(*ssa.If)
					if !ok {
						continue
					}

					found := false
					for _, succ := range ifstmt.Block().Succs {
						if len(succ.Preds) != 1 {
							// Merge point, not a branch in the
							// syntactical sense.

							// FIXME(dh): this is broken for if
							// statements a la "if x || y"
							continue
						}
						ssautil.Walk(succ, func(b *ssa.BasicBlock) bool {
							if !succ.Dominates(b) {
								// We've reached the end of the branch
								return false
							}
							for _, ins := range b.Instrs {
								// TODO(dh): we should check that
								// we're receiving from the channel of
								// a time.Timer to further reduce
								// false positives. Not a key
								// priority, considering the rarity of
								// Reset and the tiny likeliness of a
								// false positive
								if ins, ok := ins.(*ssa.UnOp); ok && ins.Op == token.ARROW && IsType(ins.X.Type(), "<-chan time.Time") {
									found = true
									return false
								}
							}
							return true
						})
					}

					if found {
						j.Errorf(call, "it is not possible to use Reset's return value correctly, as there is a race condition between draining the channel and the new timer expiring")
					}
				}
			}
		}
	}
}

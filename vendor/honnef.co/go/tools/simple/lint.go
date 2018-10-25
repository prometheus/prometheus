// Package simple contains a linter for Go source code.
package simple // import "honnef.co/go/tools/simple"

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"reflect"
	"strings"

	. "honnef.co/go/tools/arg"
	"honnef.co/go/tools/internal/sharedcheck"
	"honnef.co/go/tools/lint"
	. "honnef.co/go/tools/lint/lintdsl"

	"golang.org/x/tools/go/types/typeutil"
)

type Checker struct {
	CheckGenerated bool
	MS             *typeutil.MethodSetCache
}

func NewChecker() *Checker {
	return &Checker{
		MS: &typeutil.MethodSetCache{},
	}
}

func (*Checker) Name() string   { return "gosimple" }
func (*Checker) Prefix() string { return "S" }

func (c *Checker) Init(prog *lint.Program) {}

func (c *Checker) Checks() []lint.Check {
	return []lint.Check{
		{ID: "S1000", FilterGenerated: true, Fn: c.LintSingleCaseSelect},
		{ID: "S1001", FilterGenerated: true, Fn: c.LintLoopCopy},
		{ID: "S1002", FilterGenerated: true, Fn: c.LintIfBoolCmp},
		{ID: "S1003", FilterGenerated: true, Fn: c.LintStringsContains},
		{ID: "S1004", FilterGenerated: true, Fn: c.LintBytesCompare},
		{ID: "S1005", FilterGenerated: true, Fn: c.LintUnnecessaryBlank},
		{ID: "S1006", FilterGenerated: true, Fn: c.LintForTrue},
		{ID: "S1007", FilterGenerated: true, Fn: c.LintRegexpRaw},
		{ID: "S1008", FilterGenerated: true, Fn: c.LintIfReturn},
		{ID: "S1009", FilterGenerated: true, Fn: c.LintRedundantNilCheckWithLen},
		{ID: "S1010", FilterGenerated: true, Fn: c.LintSlicing},
		{ID: "S1011", FilterGenerated: true, Fn: c.LintLoopAppend},
		{ID: "S1012", FilterGenerated: true, Fn: c.LintTimeSince},
		{ID: "S1016", FilterGenerated: true, Fn: c.LintSimplerStructConversion},
		{ID: "S1017", FilterGenerated: true, Fn: c.LintTrim},
		{ID: "S1018", FilterGenerated: true, Fn: c.LintLoopSlide},
		{ID: "S1019", FilterGenerated: true, Fn: c.LintMakeLenCap},
		{ID: "S1020", FilterGenerated: true, Fn: c.LintAssertNotNil},
		{ID: "S1021", FilterGenerated: true, Fn: c.LintDeclareAssign},
		{ID: "S1023", FilterGenerated: true, Fn: c.LintRedundantBreak},
		{ID: "S1024", FilterGenerated: true, Fn: c.LintTimeUntil},
		{ID: "S1025", FilterGenerated: true, Fn: c.LintRedundantSprintf},
		{ID: "S1028", FilterGenerated: true, Fn: c.LintErrorsNewSprintf},
		{ID: "S1029", FilterGenerated: false, Fn: c.LintRangeStringRunes},
		{ID: "S1030", FilterGenerated: true, Fn: c.LintBytesBufferConversions},
		{ID: "S1031", FilterGenerated: true, Fn: c.LintNilCheckAroundRange},
		{ID: "S1032", FilterGenerated: true, Fn: c.LintSortHelpers},
	}
}

func (c *Checker) LintSingleCaseSelect(j *lint.Job) {
	isSingleSelect := func(node ast.Node) bool {
		v, ok := node.(*ast.SelectStmt)
		if !ok {
			return false
		}
		return len(v.Body.List) == 1
	}

	seen := map[ast.Node]struct{}{}
	fn := func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.ForStmt:
			if len(v.Body.List) != 1 {
				return true
			}
			if !isSingleSelect(v.Body.List[0]) {
				return true
			}
			if _, ok := v.Body.List[0].(*ast.SelectStmt).Body.List[0].(*ast.CommClause).Comm.(*ast.SendStmt); ok {
				// Don't suggest using range for channel sends
				return true
			}
			seen[v.Body.List[0]] = struct{}{}
			j.Errorf(node, "should use for range instead of for { select {} }")
		case *ast.SelectStmt:
			if _, ok := seen[v]; ok {
				return true
			}
			if !isSingleSelect(v) {
				return true
			}
			j.Errorf(node, "should use a simple channel send/receive instead of select with a single case")
			return true
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintLoopCopy(j *lint.Job) {
	fn := func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}

		if loop.Key == nil {
			return true
		}
		if len(loop.Body.List) != 1 {
			return true
		}
		stmt, ok := loop.Body.List[0].(*ast.AssignStmt)
		if !ok {
			return true
		}
		if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
			return true
		}
		lhs, ok := stmt.Lhs[0].(*ast.IndexExpr)
		if !ok {
			return true
		}

		if _, ok := TypeOf(j, lhs.X).(*types.Slice); !ok {
			return true
		}
		lidx, ok := lhs.Index.(*ast.Ident)
		if !ok {
			return true
		}
		key, ok := loop.Key.(*ast.Ident)
		if !ok {
			return true
		}
		if TypeOf(j, lhs) == nil || TypeOf(j, stmt.Rhs[0]) == nil {
			return true
		}
		if ObjectOf(j, lidx) != ObjectOf(j, key) {
			return true
		}
		if !types.Identical(TypeOf(j, lhs), TypeOf(j, stmt.Rhs[0])) {
			return true
		}
		if _, ok := TypeOf(j, loop.X).(*types.Slice); !ok {
			return true
		}

		if rhs, ok := stmt.Rhs[0].(*ast.IndexExpr); ok {
			rx, ok := rhs.X.(*ast.Ident)
			_ = rx
			if !ok {
				return true
			}
			ridx, ok := rhs.Index.(*ast.Ident)
			if !ok {
				return true
			}
			if ObjectOf(j, ridx) != ObjectOf(j, key) {
				return true
			}
		} else if rhs, ok := stmt.Rhs[0].(*ast.Ident); ok {
			value, ok := loop.Value.(*ast.Ident)
			if !ok {
				return true
			}
			if ObjectOf(j, rhs) != ObjectOf(j, value) {
				return true
			}
		} else {
			return true
		}
		j.Errorf(loop, "should use copy() instead of a loop")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintIfBoolCmp(j *lint.Job) {
	fn := func(node ast.Node) bool {
		expr, ok := node.(*ast.BinaryExpr)
		if !ok || (expr.Op != token.EQL && expr.Op != token.NEQ) {
			return true
		}
		x := IsBoolConst(j, expr.X)
		y := IsBoolConst(j, expr.Y)
		if !x && !y {
			return true
		}
		var other ast.Expr
		var val bool
		if x {
			val = BoolConst(j, expr.X)
			other = expr.Y
		} else {
			val = BoolConst(j, expr.Y)
			other = expr.X
		}
		basic, ok := TypeOf(j, other).Underlying().(*types.Basic)
		if !ok || basic.Kind() != types.Bool {
			return true
		}
		op := ""
		if (expr.Op == token.EQL && !val) || (expr.Op == token.NEQ && val) {
			op = "!"
		}
		r := op + Render(j, other)
		l1 := len(r)
		r = strings.TrimLeft(r, "!")
		if (l1-len(r))%2 == 1 {
			r = "!" + r
		}
		if IsInTest(j, node) {
			return true
		}
		j.Errorf(expr, "should omit comparison to bool constant, can be simplified to %s", r)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintBytesBufferConversions(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return true
		}

		argCall, ok := call.Args[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := argCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		typ := TypeOf(j, call.Fun)
		if typ == types.Universe.Lookup("string").Type() && IsCallToAST(j, call.Args[0], "(*bytes.Buffer).Bytes") {
			j.Errorf(call, "should use %v.String() instead of %v", Render(j, sel.X), Render(j, call))
		} else if typ, ok := typ.(*types.Slice); ok && typ.Elem() == types.Universe.Lookup("byte").Type() && IsCallToAST(j, call.Args[0], "(*bytes.Buffer).String") {
			j.Errorf(call, "should use %v.Bytes() instead of %v", Render(j, sel.X), Render(j, call))
		}

		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintStringsContains(j *lint.Job) {
	// map of value to token to bool value
	allowed := map[int64]map[token.Token]bool{
		-1: {token.GTR: true, token.NEQ: true, token.EQL: false},
		0:  {token.GEQ: true, token.LSS: false},
	}
	fn := func(node ast.Node) bool {
		expr, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch expr.Op {
		case token.GEQ, token.GTR, token.NEQ, token.LSS, token.EQL:
		default:
			return true
		}

		value, ok := ExprToInt(j, expr.Y)
		if !ok {
			return true
		}

		allowedOps, ok := allowed[value]
		if !ok {
			return true
		}
		b, ok := allowedOps[expr.Op]
		if !ok {
			return true
		}

		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		funIdent := sel.Sel
		if pkgIdent.Name != "strings" && pkgIdent.Name != "bytes" {
			return true
		}
		newFunc := ""
		switch funIdent.Name {
		case "IndexRune":
			newFunc = "ContainsRune"
		case "IndexAny":
			newFunc = "ContainsAny"
		case "Index":
			newFunc = "Contains"
		default:
			return true
		}

		prefix := ""
		if !b {
			prefix = "!"
		}
		j.Errorf(node, "should use %s%s.%s(%s) instead", prefix, pkgIdent.Name, newFunc, RenderArgs(j, call.Args))

		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintBytesCompare(j *lint.Job) {
	fn := func(node ast.Node) bool {
		expr, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if expr.Op != token.NEQ && expr.Op != token.EQL {
			return true
		}
		call, ok := expr.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, call, "bytes.Compare") {
			return true
		}
		value, ok := ExprToInt(j, expr.Y)
		if !ok || value != 0 {
			return true
		}
		args := RenderArgs(j, call.Args)
		prefix := ""
		if expr.Op == token.NEQ {
			prefix = "!"
		}
		j.Errorf(node, "should use %sbytes.Equal(%s) instead", prefix, args)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintForTrue(j *lint.Job) {
	fn := func(node ast.Node) bool {
		loop, ok := node.(*ast.ForStmt)
		if !ok {
			return true
		}
		if loop.Init != nil || loop.Post != nil {
			return true
		}
		if !IsBoolConst(j, loop.Cond) || !BoolConst(j, loop.Cond) {
			return true
		}
		j.Errorf(loop, "should use for {} instead of for true {}")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintRegexpRaw(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, call, "regexp.MustCompile") &&
			!IsCallToAST(j, call, "regexp.Compile") {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if len(call.Args) != 1 {
			// invalid function call
			return true
		}
		lit, ok := call.Args[Arg("regexp.Compile.expr")].(*ast.BasicLit)
		if !ok {
			// TODO(dominikh): support string concat, maybe support constants
			return true
		}
		if lit.Kind != token.STRING {
			// invalid function call
			return true
		}
		if lit.Value[0] != '"' {
			// already a raw string
			return true
		}
		val := lit.Value
		if !strings.Contains(val, `\\`) {
			return true
		}
		if strings.Contains(val, "`") {
			return true
		}

		bs := false
		for _, c := range val {
			if !bs && c == '\\' {
				bs = true
				continue
			}
			if bs && c == '\\' {
				bs = false
				continue
			}
			if bs {
				// backslash followed by non-backslash -> escape sequence
				return true
			}
		}

		j.Errorf(call, "should use raw string (`...`) with regexp.%s to avoid having to escape twice", sel.Sel.Name)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintIfReturn(j *lint.Job) {
	fn := func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		l := len(block.List)
		if l < 2 {
			return true
		}
		n1, n2 := block.List[l-2], block.List[l-1]

		if len(block.List) >= 3 {
			if _, ok := block.List[l-3].(*ast.IfStmt); ok {
				// Do not flag a series of if statements
				return true
			}
		}
		// if statement with no init, no else, a single condition
		// checking an identifier or function call and just a return
		// statement in the body, that returns a boolean constant
		ifs, ok := n1.(*ast.IfStmt)
		if !ok {
			return true
		}
		if ifs.Else != nil || ifs.Init != nil {
			return true
		}
		if len(ifs.Body.List) != 1 {
			return true
		}
		if op, ok := ifs.Cond.(*ast.BinaryExpr); ok {
			switch op.Op {
			case token.EQL, token.LSS, token.GTR, token.NEQ, token.LEQ, token.GEQ:
			default:
				return true
			}
		}
		ret1, ok := ifs.Body.List[0].(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(ret1.Results) != 1 {
			return true
		}
		if !IsBoolConst(j, ret1.Results[0]) {
			return true
		}

		ret2, ok := n2.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(ret2.Results) != 1 {
			return true
		}
		if !IsBoolConst(j, ret2.Results[0]) {
			return true
		}
		j.Errorf(n1, "should use 'return <expr>' instead of 'if <expr> { return <bool> }; return <bool>'")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

// LintRedundantNilCheckWithLen checks for the following reduntant nil-checks:
//
//   if x == nil || len(x) == 0 {}
//   if x != nil && len(x) != 0 {}
//   if x != nil && len(x) == N {} (where N != 0)
//   if x != nil && len(x) > N {}
//   if x != nil && len(x) >= N {} (where N != 0)
//
func (c *Checker) LintRedundantNilCheckWithLen(j *lint.Job) {
	isConstZero := func(expr ast.Expr) (isConst bool, isZero bool) {
		_, ok := expr.(*ast.BasicLit)
		if ok {
			return true, IsZero(expr)
		}
		id, ok := expr.(*ast.Ident)
		if !ok {
			return false, false
		}
		c, ok := ObjectOf(j, id).(*types.Const)
		if !ok {
			return false, false
		}
		return true, c.Val().Kind() == constant.Int && c.Val().String() == "0"
	}

	fn := func(node ast.Node) bool {
		// check that expr is "x || y" or "x && y"
		expr, ok := node.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if expr.Op != token.LOR && expr.Op != token.LAND {
			return true
		}
		eqNil := expr.Op == token.LOR

		// check that x is "xx == nil" or "xx != nil"
		x, ok := expr.X.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if eqNil && x.Op != token.EQL {
			return true
		}
		if !eqNil && x.Op != token.NEQ {
			return true
		}
		xx, ok := x.X.(*ast.Ident)
		if !ok {
			return true
		}
		if !IsNil(j, x.Y) {
			return true
		}

		// check that y is "len(xx) == 0" or "len(xx) ... "
		y, ok := expr.Y.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if eqNil && y.Op != token.EQL { // must be len(xx) *==* 0
			return false
		}
		yx, ok := y.X.(*ast.CallExpr)
		if !ok {
			return true
		}
		yxFun, ok := yx.Fun.(*ast.Ident)
		if !ok || yxFun.Name != "len" || len(yx.Args) != 1 {
			return true
		}
		yxArg, ok := yx.Args[Arg("len.v")].(*ast.Ident)
		if !ok {
			return true
		}
		if yxArg.Name != xx.Name {
			return true
		}

		if eqNil && !IsZero(y.Y) { // must be len(x) == *0*
			return true
		}

		if !eqNil {
			isConst, isZero := isConstZero(y.Y)
			if !isConst {
				return true
			}
			switch y.Op {
			case token.EQL:
				// avoid false positive for "xx != nil && len(xx) == 0"
				if isZero {
					return true
				}
			case token.GEQ:
				// avoid false positive for "xx != nil && len(xx) >= 0"
				if isZero {
					return true
				}
			case token.NEQ:
				// avoid false positive for "xx != nil && len(xx) != <non-zero>"
				if !isZero {
					return true
				}
			case token.GTR:
				// ok
			default:
				return true
			}
		}

		// finally check that xx type is one of array, slice, map or chan
		// this is to prevent false positive in case if xx is a pointer to an array
		var nilType string
		switch TypeOf(j, xx).(type) {
		case *types.Slice:
			nilType = "nil slices"
		case *types.Map:
			nilType = "nil maps"
		case *types.Chan:
			nilType = "nil channels"
		default:
			return true
		}
		j.Errorf(expr, "should omit nil check; len() for %s is defined as zero", nilType)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintSlicing(j *lint.Job) {
	fn := func(node ast.Node) bool {
		n, ok := node.(*ast.SliceExpr)
		if !ok {
			return true
		}
		if n.Max != nil {
			return true
		}
		s, ok := n.X.(*ast.Ident)
		if !ok || s.Obj == nil {
			return true
		}
		call, ok := n.High.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return true
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok || fun.Name != "len" {
			return true
		}
		if _, ok := ObjectOf(j, fun).(*types.Builtin); !ok {
			return true
		}
		arg, ok := call.Args[Arg("len.v")].(*ast.Ident)
		if !ok || arg.Obj != s.Obj {
			return true
		}
		j.Errorf(n, "should omit second index in slice, s[a:len(s)] is identical to s[a:]")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func refersTo(j *lint.Job, expr ast.Expr, ident *ast.Ident) bool {
	found := false
	fn := func(node ast.Node) bool {
		ident2, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if ObjectOf(j, ident) == ObjectOf(j, ident2) {
			found = true
			return false
		}
		return true
	}
	ast.Inspect(expr, fn)
	return found
}

func (c *Checker) LintLoopAppend(j *lint.Job) {
	fn := func(node ast.Node) bool {
		loop, ok := node.(*ast.RangeStmt)
		if !ok {
			return true
		}
		if !IsBlank(loop.Key) {
			return true
		}
		val, ok := loop.Value.(*ast.Ident)
		if !ok {
			return true
		}
		if len(loop.Body.List) != 1 {
			return true
		}
		stmt, ok := loop.Body.List[0].(*ast.AssignStmt)
		if !ok {
			return true
		}
		if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
			return true
		}
		if refersTo(j, stmt.Lhs[0], val) {
			return true
		}
		call, ok := stmt.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		if len(call.Args) != 2 || call.Ellipsis.IsValid() {
			return true
		}
		fun, ok := call.Fun.(*ast.Ident)
		if !ok {
			return true
		}
		obj := ObjectOf(j, fun)
		fn, ok := obj.(*types.Builtin)
		if !ok || fn.Name() != "append" {
			return true
		}

		src := TypeOf(j, loop.X)
		dst := TypeOf(j, call.Args[Arg("append.slice")])
		// TODO(dominikh) remove nil check once Go issue #15173 has
		// been fixed
		if src == nil {
			return true
		}
		if !types.Identical(src, dst) {
			return true
		}

		if Render(j, stmt.Lhs[0]) != Render(j, call.Args[Arg("append.slice")]) {
			return true
		}

		el, ok := call.Args[Arg("append.elems")].(*ast.Ident)
		if !ok {
			return true
		}
		if ObjectOf(j, val) != ObjectOf(j, el) {
			return true
		}
		j.Errorf(loop, "should replace loop with %s = append(%s, %s...)",
			Render(j, stmt.Lhs[0]), Render(j, call.Args[Arg("append.slice")]), Render(j, loop.X))
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintTimeSince(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, sel.X, "time.Now") {
			return true
		}
		if sel.Sel.Name != "Sub" {
			return true
		}
		j.Errorf(call, "should use time.Since instead of time.Now().Sub")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintTimeUntil(j *lint.Job) {
	if !IsGoVersion(j, 8) {
		return
	}
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, call, "(time.Time).Sub") {
			return true
		}
		if !IsCallToAST(j, call.Args[Arg("(time.Time).Sub.u")], "time.Now") {
			return true
		}
		j.Errorf(call, "should use time.Until instead of t.Sub(time.Now())")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintUnnecessaryBlank(j *lint.Job) {
	fn1 := func(node ast.Node) {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return
		}
		if len(assign.Lhs) != 2 || len(assign.Rhs) != 1 {
			return
		}
		if !IsBlank(assign.Lhs[1]) {
			return
		}
		switch rhs := assign.Rhs[0].(type) {
		case *ast.IndexExpr:
			// The type-checker should make sure that it's a map, but
			// let's be safe.
			if _, ok := TypeOf(j, rhs.X).Underlying().(*types.Map); !ok {
				return
			}
		case *ast.UnaryExpr:
			if rhs.Op != token.ARROW {
				return
			}
		default:
			return
		}
		cp := *assign
		cp.Lhs = cp.Lhs[0:1]
		j.Errorf(assign, "should write %s instead of %s", Render(j, &cp), Render(j, assign))
	}

	fn2 := func(node ast.Node) {
		stmt, ok := node.(*ast.AssignStmt)
		if !ok {
			return
		}
		if len(stmt.Lhs) != len(stmt.Rhs) {
			return
		}
		for i, lh := range stmt.Lhs {
			rh := stmt.Rhs[i]
			if !IsBlank(lh) {
				continue
			}
			expr, ok := rh.(*ast.UnaryExpr)
			if !ok {
				continue
			}
			if expr.Op != token.ARROW {
				continue
			}
			j.Errorf(lh, "'_ = <-ch' can be simplified to '<-ch'")
		}
	}

	fn3 := func(node ast.Node) {
		rs, ok := node.(*ast.RangeStmt)
		if !ok {
			return
		}

		// for x, _
		if !IsBlank(rs.Key) && IsBlank(rs.Value) {
			j.Errorf(rs.Value, "should omit value from range; this loop is equivalent to `for %s %s range ...`", Render(j, rs.Key), rs.Tok)
		}
		// for _, _ || for _
		if IsBlank(rs.Key) && (IsBlank(rs.Value) || rs.Value == nil) {
			j.Errorf(rs.Key, "should omit values from range; this loop is equivalent to `for range ...`")
		}
	}

	fn := func(node ast.Node) bool {
		fn1(node)
		fn2(node)
		if IsGoVersion(j, 4) {
			fn3(node)
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintSimplerStructConversion(j *lint.Job) {
	var skip ast.Node
	fn := func(node ast.Node) bool {
		// Do not suggest type conversion between pointers
		if unary, ok := node.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if lit, ok := unary.X.(*ast.CompositeLit); ok {
				skip = lit
			}
			return true
		}

		if node == skip {
			return true
		}

		lit, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		typ1, _ := TypeOf(j, lit.Type).(*types.Named)
		if typ1 == nil {
			return true
		}
		s1, ok := typ1.Underlying().(*types.Struct)
		if !ok {
			return true
		}

		var typ2 *types.Named
		var ident *ast.Ident
		getSelType := func(expr ast.Expr) (types.Type, *ast.Ident, bool) {
			sel, ok := expr.(*ast.SelectorExpr)
			if !ok {
				return nil, nil, false
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok {
				return nil, nil, false
			}
			typ := TypeOf(j, sel.X)
			return typ, ident, typ != nil
		}
		if len(lit.Elts) == 0 {
			return true
		}
		if s1.NumFields() != len(lit.Elts) {
			return true
		}
		for i, elt := range lit.Elts {
			var t types.Type
			var id *ast.Ident
			var ok bool
			switch elt := elt.(type) {
			case *ast.SelectorExpr:
				t, id, ok = getSelType(elt)
				if !ok {
					return true
				}
				if i >= s1.NumFields() || s1.Field(i).Name() != elt.Sel.Name {
					return true
				}
			case *ast.KeyValueExpr:
				var sel *ast.SelectorExpr
				sel, ok = elt.Value.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if elt.Key.(*ast.Ident).Name != sel.Sel.Name {
					return true
				}
				t, id, ok = getSelType(elt.Value)
			}
			if !ok {
				return true
			}
			// All fields must be initialized from the same object
			if ident != nil && ident.Obj != id.Obj {
				return true
			}
			typ2, _ = t.(*types.Named)
			if typ2 == nil {
				return true
			}
			ident = id
		}

		if typ2 == nil {
			return true
		}

		if typ1.Obj().Pkg() != typ2.Obj().Pkg() {
			// Do not suggest type conversions between different
			// packages. Types in different packages might only match
			// by coincidence. Furthermore, if the dependency ever
			// adds more fields to its type, it could break the code
			// that relies on the type conversion to work.
			return true
		}

		s2, ok := typ2.Underlying().(*types.Struct)
		if !ok {
			return true
		}
		if typ1 == typ2 {
			return true
		}
		if IsGoVersion(j, 8) {
			if !types.IdenticalIgnoreTags(s1, s2) {
				return true
			}
		} else {
			if !types.Identical(s1, s2) {
				return true
			}
		}
		j.Errorf(node, "should convert %s (type %s) to %s instead of using struct literal",
			ident.Name, typ2.Obj().Name(), typ1.Obj().Name())
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintTrim(j *lint.Job) {
	sameNonDynamic := func(node1, node2 ast.Node) bool {
		if reflect.TypeOf(node1) != reflect.TypeOf(node2) {
			return false
		}

		switch node1 := node1.(type) {
		case *ast.Ident:
			return node1.Obj == node2.(*ast.Ident).Obj
		case *ast.SelectorExpr:
			return Render(j, node1) == Render(j, node2)
		case *ast.IndexExpr:
			return Render(j, node1) == Render(j, node2)
		}
		return false
	}

	isLenOnIdent := func(fn ast.Expr, ident ast.Expr) bool {
		call, ok := fn.(*ast.CallExpr)
		if !ok {
			return false
		}
		if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "len" {
			return false
		}
		if len(call.Args) != 1 {
			return false
		}
		return sameNonDynamic(call.Args[Arg("len.v")], ident)
	}

	fn := func(node ast.Node) bool {
		var pkg string
		var fun string

		ifstmt, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		if ifstmt.Init != nil {
			return true
		}
		if ifstmt.Else != nil {
			return true
		}
		if len(ifstmt.Body.List) != 1 {
			return true
		}
		condCall, ok := ifstmt.Cond.(*ast.CallExpr)
		if !ok {
			return true
		}
		call, ok := condCall.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if IsIdent(call.X, "strings") {
			pkg = "strings"
		} else if IsIdent(call.X, "bytes") {
			pkg = "bytes"
		} else {
			return true
		}
		if IsIdent(call.Sel, "HasPrefix") {
			fun = "HasPrefix"
		} else if IsIdent(call.Sel, "HasSuffix") {
			fun = "HasSuffix"
		} else {
			return true
		}

		assign, ok := ifstmt.Body.List[0].(*ast.AssignStmt)
		if !ok {
			return true
		}
		if assign.Tok != token.ASSIGN {
			return true
		}
		if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return true
		}
		if !sameNonDynamic(condCall.Args[0], assign.Lhs[0]) {
			return true
		}
		slice, ok := assign.Rhs[0].(*ast.SliceExpr)
		if !ok {
			return true
		}
		if slice.Slice3 {
			return true
		}
		if !sameNonDynamic(slice.X, condCall.Args[0]) {
			return true
		}
		var index ast.Expr
		switch fun {
		case "HasPrefix":
			// TODO(dh) We could detect a High that is len(s), but another
			// rule will already flag that, anyway.
			if slice.High != nil {
				return true
			}
			index = slice.Low
		case "HasSuffix":
			if slice.Low != nil {
				n, ok := ExprToInt(j, slice.Low)
				if !ok || n != 0 {
					return true
				}
			}
			index = slice.High
		}

		switch index := index.(type) {
		case *ast.CallExpr:
			if fun != "HasPrefix" {
				return true
			}
			if fn, ok := index.Fun.(*ast.Ident); !ok || fn.Name != "len" {
				return true
			}
			if len(index.Args) != 1 {
				return true
			}
			id3 := index.Args[Arg("len.v")]
			switch oid3 := condCall.Args[1].(type) {
			case *ast.BasicLit:
				if pkg != "strings" {
					return false
				}
				lit, ok := id3.(*ast.BasicLit)
				if !ok {
					return true
				}
				s1, ok1 := ExprToString(j, lit)
				s2, ok2 := ExprToString(j, condCall.Args[1])
				if !ok1 || !ok2 || s1 != s2 {
					return true
				}
			default:
				if !sameNonDynamic(id3, oid3) {
					return true
				}
			}
		case *ast.BasicLit, *ast.Ident:
			if fun != "HasPrefix" {
				return true
			}
			if pkg != "strings" {
				return true
			}
			string, ok1 := ExprToString(j, condCall.Args[1])
			int, ok2 := ExprToInt(j, slice.Low)
			if !ok1 || !ok2 || int != int64(len(string)) {
				return true
			}
		case *ast.BinaryExpr:
			if fun != "HasSuffix" {
				return true
			}
			if index.Op != token.SUB {
				return true
			}
			if !isLenOnIdent(index.X, condCall.Args[0]) ||
				!isLenOnIdent(index.Y, condCall.Args[1]) {
				return true
			}
		default:
			return true
		}

		var replacement string
		switch fun {
		case "HasPrefix":
			replacement = "TrimPrefix"
		case "HasSuffix":
			replacement = "TrimSuffix"
		}
		j.Errorf(ifstmt, "should replace this if statement with an unconditional %s.%s", pkg, replacement)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintLoopSlide(j *lint.Job) {
	// TODO(dh): detect bs[i+offset] in addition to bs[offset+i]
	// TODO(dh): consider merging this function with LintLoopCopy
	// TODO(dh): detect length that is an expression, not a variable name
	// TODO(dh): support sliding to a different offset than the beginning of the slice

	fn := func(node ast.Node) bool {
		/*
			for i := 0; i < n; i++ {
				bs[i] = bs[offset+i]
			}

						↓

			copy(bs[:n], bs[offset:offset+n])
		*/

		loop, ok := node.(*ast.ForStmt)
		if !ok || len(loop.Body.List) != 1 || loop.Init == nil || loop.Cond == nil || loop.Post == nil {
			return true
		}
		assign, ok := loop.Init.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || !IsZero(assign.Rhs[0]) {
			return true
		}
		initvar, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		post, ok := loop.Post.(*ast.IncDecStmt)
		if !ok || post.Tok != token.INC {
			return true
		}
		postvar, ok := post.X.(*ast.Ident)
		if !ok || ObjectOf(j, postvar) != ObjectOf(j, initvar) {
			return true
		}
		bin, ok := loop.Cond.(*ast.BinaryExpr)
		if !ok || bin.Op != token.LSS {
			return true
		}
		binx, ok := bin.X.(*ast.Ident)
		if !ok || ObjectOf(j, binx) != ObjectOf(j, initvar) {
			return true
		}
		biny, ok := bin.Y.(*ast.Ident)
		if !ok {
			return true
		}

		assign, ok = loop.Body.List[0].(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 || assign.Tok != token.ASSIGN {
			return true
		}
		lhs, ok := assign.Lhs[0].(*ast.IndexExpr)
		if !ok {
			return true
		}
		rhs, ok := assign.Rhs[0].(*ast.IndexExpr)
		if !ok {
			return true
		}

		bs1, ok := lhs.X.(*ast.Ident)
		if !ok {
			return true
		}
		bs2, ok := rhs.X.(*ast.Ident)
		if !ok {
			return true
		}
		obj1 := ObjectOf(j, bs1)
		obj2 := ObjectOf(j, bs2)
		if obj1 != obj2 {
			return true
		}
		if _, ok := obj1.Type().Underlying().(*types.Slice); !ok {
			return true
		}

		index1, ok := lhs.Index.(*ast.Ident)
		if !ok || ObjectOf(j, index1) != ObjectOf(j, initvar) {
			return true
		}
		index2, ok := rhs.Index.(*ast.BinaryExpr)
		if !ok || index2.Op != token.ADD {
			return true
		}
		add1, ok := index2.X.(*ast.Ident)
		if !ok {
			return true
		}
		add2, ok := index2.Y.(*ast.Ident)
		if !ok || ObjectOf(j, add2) != ObjectOf(j, initvar) {
			return true
		}

		j.Errorf(loop, "should use copy(%s[:%s], %s[%s:]) instead", Render(j, bs1), Render(j, biny), Render(j, bs1), Render(j, add1))
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintMakeLenCap(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); !ok || fn.Name != "make" {
			// FIXME check whether make is indeed the built-in function
			return true
		}
		switch len(call.Args) {
		case 2:
			// make(T, len)
			if _, ok := TypeOf(j, call.Args[Arg("make.t")]).Underlying().(*types.Slice); ok {
				break
			}
			if IsZero(call.Args[Arg("make.size[0]")]) {
				j.Errorf(call.Args[Arg("make.size[0]")], "should use make(%s) instead", Render(j, call.Args[Arg("make.t")]))
			}
		case 3:
			// make(T, len, cap)
			if Render(j, call.Args[Arg("make.size[0]")]) == Render(j, call.Args[Arg("make.size[1]")]) {
				j.Errorf(call.Args[Arg("make.size[0]")],
					"should use make(%s, %s) instead",
					Render(j, call.Args[Arg("make.t")]), Render(j, call.Args[Arg("make.size[0]")]))
			}
		}
		return false
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintAssertNotNil(j *lint.Job) {
	isNilCheck := func(ident *ast.Ident, expr ast.Expr) bool {
		xbinop, ok := expr.(*ast.BinaryExpr)
		if !ok || xbinop.Op != token.NEQ {
			return false
		}
		xident, ok := xbinop.X.(*ast.Ident)
		if !ok || xident.Obj != ident.Obj {
			return false
		}
		if !IsNil(j, xbinop.Y) {
			return false
		}
		return true
	}
	isOKCheck := func(ident *ast.Ident, expr ast.Expr) bool {
		yident, ok := expr.(*ast.Ident)
		if !ok || yident.Obj != ident.Obj {
			return false
		}
		return true
	}
	fn := func(node ast.Node) bool {
		ifstmt, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}
		assign, ok := ifstmt.Init.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 2 || len(assign.Rhs) != 1 || !IsBlank(assign.Lhs[0]) {
			return true
		}
		assert, ok := assign.Rhs[0].(*ast.TypeAssertExpr)
		if !ok {
			return true
		}
		binop, ok := ifstmt.Cond.(*ast.BinaryExpr)
		if !ok || binop.Op != token.LAND {
			return true
		}
		assertIdent, ok := assert.X.(*ast.Ident)
		if !ok {
			return true
		}
		assignIdent, ok := assign.Lhs[1].(*ast.Ident)
		if !ok {
			return true
		}
		if !(isNilCheck(assertIdent, binop.X) && isOKCheck(assignIdent, binop.Y)) &&
			!(isNilCheck(assertIdent, binop.Y) && isOKCheck(assignIdent, binop.X)) {
			return true
		}
		j.Errorf(ifstmt, "when %s is true, %s can't be nil", Render(j, assignIdent), Render(j, assertIdent))
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintDeclareAssign(j *lint.Job) {
	fn := func(node ast.Node) bool {
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		if len(block.List) < 2 {
			return true
		}
		for i, stmt := range block.List[:len(block.List)-1] {
			_ = i
			decl, ok := stmt.(*ast.DeclStmt)
			if !ok {
				continue
			}
			gdecl, ok := decl.Decl.(*ast.GenDecl)
			if !ok || gdecl.Tok != token.VAR || len(gdecl.Specs) != 1 {
				continue
			}
			vspec, ok := gdecl.Specs[0].(*ast.ValueSpec)
			if !ok || len(vspec.Names) != 1 || len(vspec.Values) != 0 {
				continue
			}

			assign, ok := block.List[i+1].(*ast.AssignStmt)
			if !ok || assign.Tok != token.ASSIGN {
				continue
			}
			if len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				continue
			}
			ident, ok := assign.Lhs[0].(*ast.Ident)
			if !ok {
				continue
			}
			if vspec.Names[0].Obj != ident.Obj {
				continue
			}

			if refersTo(j, assign.Rhs[0], ident) {
				continue
			}
			j.Errorf(decl, "should merge variable declaration with assignment on next line")
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintRedundantBreak(j *lint.Job) {
	fn1 := func(node ast.Node) {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return
		}
		if len(clause.Body) < 2 {
			return
		}
		branch, ok := clause.Body[len(clause.Body)-1].(*ast.BranchStmt)
		if !ok || branch.Tok != token.BREAK || branch.Label != nil {
			return
		}
		j.Errorf(branch, "redundant break statement")
	}
	fn2 := func(node ast.Node) {
		var ret *ast.FieldList
		var body *ast.BlockStmt
		switch x := node.(type) {
		case *ast.FuncDecl:
			ret = x.Type.Results
			body = x.Body
		case *ast.FuncLit:
			ret = x.Type.Results
			body = x.Body
		default:
			return
		}
		// if the func has results, a return can't be redundant.
		// similarly, if there are no statements, there can be
		// no return.
		if ret != nil || body == nil || len(body.List) < 1 {
			return
		}
		rst, ok := body.List[len(body.List)-1].(*ast.ReturnStmt)
		if !ok {
			return
		}
		// we don't need to check rst.Results as we already
		// checked x.Type.Results to be nil.
		j.Errorf(rst, "redundant return statement")
	}
	fn := func(node ast.Node) bool {
		fn1(node)
		fn2(node)
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) Implements(j *lint.Job, typ types.Type, iface string) bool {
	// OPT(dh): we can cache the type lookup
	idx := strings.IndexRune(iface, '.')
	var scope *types.Scope
	var ifaceName string
	if idx == -1 {
		scope = types.Universe
		ifaceName = iface
	} else {
		pkgName := iface[:idx]
		pkg := j.Program.Package(pkgName)
		if pkg == nil {
			return false
		}
		scope = pkg.Types.Scope()
		ifaceName = iface[idx+1:]
	}

	obj := scope.Lookup(ifaceName)
	if obj == nil {
		return false
	}
	i, ok := obj.Type().Underlying().(*types.Interface)
	if !ok {
		return false
	}
	return types.Implements(typ, i)
}

func (c *Checker) LintRedundantSprintf(j *lint.Job) {
	fn := func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !IsCallToAST(j, call, "fmt.Sprintf") {
			return true
		}
		if len(call.Args) != 2 {
			return true
		}
		if s, ok := ExprToString(j, call.Args[Arg("fmt.Sprintf.format")]); !ok || s != "%s" {
			return true
		}
		arg := call.Args[Arg("fmt.Sprintf.a[0]")]
		typ := TypeOf(j, arg)

		if c.Implements(j, typ, "fmt.Stringer") {
			j.Errorf(call, "should use String() instead of fmt.Sprintf")
			return true
		}

		if typ.Underlying() == types.Universe.Lookup("string").Type() {
			if typ == types.Universe.Lookup("string").Type() {
				j.Errorf(call, "the argument is already a string, there's no need to use fmt.Sprintf")
			} else {
				j.Errorf(call, "the argument's underlying type is a string, should use a simple conversion instead of fmt.Sprintf")
			}
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintErrorsNewSprintf(j *lint.Job) {
	fn := func(node ast.Node) bool {
		if !IsCallToAST(j, node, "errors.New") {
			return true
		}
		call := node.(*ast.CallExpr)
		if !IsCallToAST(j, call.Args[Arg("errors.New.text")], "fmt.Sprintf") {
			return true
		}
		j.Errorf(node, "should use fmt.Errorf(...) instead of errors.New(fmt.Sprintf(...))")
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func (c *Checker) LintRangeStringRunes(j *lint.Job) {
	sharedcheck.CheckRangeStringRunes(j)
}

func (c *Checker) LintNilCheckAroundRange(j *lint.Job) {
	fn := func(node ast.Node) bool {
		ifstmt, ok := node.(*ast.IfStmt)
		if !ok {
			return true
		}

		cond, ok := ifstmt.Cond.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		if cond.Op != token.NEQ || !IsNil(j, cond.Y) || len(ifstmt.Body.List) != 1 {
			return true
		}

		loop, ok := ifstmt.Body.List[0].(*ast.RangeStmt)
		if !ok {
			return true
		}
		ifXIdent, ok := cond.X.(*ast.Ident)
		if !ok {
			return true
		}
		rangeXIdent, ok := loop.X.(*ast.Ident)
		if !ok {
			return true
		}
		if ifXIdent.Obj != rangeXIdent.Obj {
			return true
		}
		switch TypeOf(j, rangeXIdent).(type) {
		case *types.Slice, *types.Map:
			j.Errorf(node, "unnecessary nil check around range")
		}
		return true
	}
	for _, f := range j.Program.Files {
		ast.Inspect(f, fn)
	}
}

func isPermissibleSort(j *lint.Job, node ast.Node) bool {
	call := node.(*ast.CallExpr)
	typeconv, ok := call.Args[0].(*ast.CallExpr)
	if !ok {
		return true
	}

	sel, ok := typeconv.Fun.(*ast.SelectorExpr)
	if !ok {
		return true
	}
	name := SelectorName(j, sel)
	switch name {
	case "sort.IntSlice", "sort.Float64Slice", "sort.StringSlice":
	default:
		return true
	}

	return false
}

func (c *Checker) LintSortHelpers(j *lint.Job) {
	fnFuncs := func(node ast.Node) bool {
		var body *ast.BlockStmt
		switch node := node.(type) {
		case *ast.FuncLit:
			body = node.Body
		case *ast.FuncDecl:
			body = node.Body
		default:
			return true
		}
		if body == nil {
			return true
		}

		type Error struct {
			node lint.Positioner
			msg  string
		}
		var errors []Error
		permissible := false
		fnSorts := func(node ast.Node) bool {
			if permissible {
				return false
			}
			if !IsCallToAST(j, node, "sort.Sort") {
				return true
			}
			if isPermissibleSort(j, node) {
				permissible = true
				return false
			}
			call := node.(*ast.CallExpr)
			typeconv := call.Args[Arg("sort.Sort.data")].(*ast.CallExpr)
			sel := typeconv.Fun.(*ast.SelectorExpr)
			name := SelectorName(j, sel)

			switch name {
			case "sort.IntSlice":
				errors = append(errors, Error{node, "should use sort.Ints(...) instead of sort.Sort(sort.IntSlice(...))"})
			case "sort.Float64Slice":
				errors = append(errors, Error{node, "should use sort.Float64s(...) instead of sort.Sort(sort.Float64Slice(...))"})
			case "sort.StringSlice":
				errors = append(errors, Error{node, "should use sort.Strings(...) instead of sort.Sort(sort.StringSlice(...))"})
			}
			return true
		}
		ast.Inspect(body, fnSorts)

		if permissible {
			return false
		}
		for _, err := range errors {
			j.Errorf(err.node, "%s", err.msg)
		}
		return false
	}

	for _, f := range j.Program.Files {
		ast.Inspect(f, fnFuncs)
	}
}

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"unicode"
)

func export(s string) string {
	return strings.Title(s)
}

func unexport(s string) string {
	first := true
	return strings.Map(func(r rune) rune {
		if first {
			first = false
			return unicode.ToLower(r)
		}
		return r
	}, s)
}

func inventName(t ast.Expr, scope *ast.Scope) *ast.Ident {
	n := baseName(t)
	for try := 0; ; try++ {
		nstr := pickName(n, try)
		obj := ast.NewObj(ast.Var, nstr)
		if alt := scope.Insert(obj); alt == nil {
			return ast.NewIdent(nstr)
		}
	}
}

func baseName(t ast.Expr) string {
	switch tt := t.(type) {
	default:
		panic(fmt.Sprintf("don't know how to choose a base name for %T (%[1]v)", tt))
	case *ast.ArrayType:
		return "slice"
	case *ast.Ident:
		return tt.Name
	case *ast.SelectorExpr:
		return tt.Sel.Name
	}
}

func pickName(base string, idx int) string {
	if idx == 0 {
		switch base {
		default:
			return strings.Split(base, "")[0]
		case "Context":
			return "ctx"
		case "error":
			return "err"
		}
	}
	return fmt.Sprintf("%s%d", base, idx)
}

func scopeWith(names ...string) *ast.Scope {
	scope := ast.NewScope(nil)
	for _, name := range names {
		scope.Insert(ast.NewObj(ast.Var, name))
	}
	return scope
}

type visitFn func(ast.Node, func(ast.Node))

func (fn visitFn) Visit(node ast.Node, r func(ast.Node)) Visitor {
	fn(node, r)
	return fn
}

func replaceIdent(src ast.Node, named string, with ast.Node) ast.Node {
	r := visitFn(func(node ast.Node, replaceWith func(ast.Node)) {
		switch id := node.(type) {
		case *ast.Ident:
			if id.Name == named {
				replaceWith(with)
			}
		}
	})
	return WalkReplace(r, src)
}

func replaceLit(src ast.Node, from, to string) ast.Node {
	r := visitFn(func(node ast.Node, replaceWith func(ast.Node)) {
		switch lit := node.(type) {
		case *ast.BasicLit:
			if lit.Value == from {
				replaceWith(&ast.BasicLit{Value: to})
			}
		}
	})
	return WalkReplace(r, src)
}

func fullAST() *ast.File {
	full, err := ASTTemplates.Open("full.go")
	if err != nil {
		panic(err)
	}
	f, err := parser.ParseFile(token.NewFileSet(), "templates/full.go", full, parser.DeclarationErrors)
	if err != nil {
		panic(err)
	}
	return f
}

func fetchImports() []*ast.ImportSpec {
	return fullAST().Imports
}

func fetchFuncDecl(name string) *ast.FuncDecl {
	f := fullAST()
	for _, decl := range f.Decls {
		if f, ok := decl.(*ast.FuncDecl); ok && f.Name.Name == name {
			return f
		}
	}
	panic(fmt.Errorf("no function called %q in 'templates/full.go'", name))
}

func id(name string) *ast.Ident {
	return ast.NewIdent(name)
}

func sel(ids ...*ast.Ident) ast.Expr {
	switch len(ids) {
	default:
		return &ast.SelectorExpr{
			X:   sel(ids[:len(ids)-1]...),
			Sel: ids[len(ids)-1],
		}
	case 1:
		return ids[0]
	case 0:
		panic("zero ids to sel()")
	}
}

func typeField(t ast.Expr) *ast.Field {
	return &ast.Field{Type: t}
}

func field(n *ast.Ident, t ast.Expr) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{n},
		Type:  t,
	}
}

func fieldList(list ...*ast.Field) *ast.FieldList {
	return &ast.FieldList{List: list}
}

func mappedFieldList(fn func(arg) *ast.Field, args ...arg) *ast.FieldList {
	fl := &ast.FieldList{List: []*ast.Field{}}
	for _, a := range args {
		fl.List = append(fl.List, fn(a))
	}
	return fl
}

func blockStmt(stmts ...ast.Stmt) *ast.BlockStmt {
	return &ast.BlockStmt{
		List: stmts,
	}
}

func structDecl(name *ast.Ident, fields *ast.FieldList) ast.Decl {
	return typeDecl(&ast.TypeSpec{
		Name: name,
		Type: &ast.StructType{
			Fields: fields,
		},
	})
}

func typeDecl(ts *ast.TypeSpec) ast.Decl {
	return &ast.GenDecl{
		Tok:   token.TYPE,
		Specs: []ast.Spec{ts},
	}
}

func pasteStmts(body *ast.BlockStmt, idx int, stmts []ast.Stmt) {
	list := body.List
	prefix := list[:idx]
	suffix := make([]ast.Stmt, len(list)-idx-1)
	copy(suffix, list[idx+1:])

	body.List = append(append(prefix, stmts...), suffix...)
}

func importFor(is *ast.ImportSpec) *ast.GenDecl {
	return &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{is}}
}

func importSpec(path string) *ast.ImportSpec {
	return &ast.ImportSpec{Path: &ast.BasicLit{Kind: token.STRING, Value: `"` + filepath.ToSlash(path) + `"`}}
}

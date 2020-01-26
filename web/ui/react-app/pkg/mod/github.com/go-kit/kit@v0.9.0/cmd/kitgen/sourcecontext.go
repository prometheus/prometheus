package main

import (
	"fmt"
	"go/ast"
	"go/token"
)

type sourceContext struct {
	pkg        *ast.Ident
	imports    []*ast.ImportSpec
	interfaces []iface
	types      []*ast.TypeSpec
}

func (sc *sourceContext) validate() error {
	if len(sc.interfaces) != 1 {
		return fmt.Errorf("found %d interfaces, expecting exactly 1", len(sc.interfaces))
	}
	for _, i := range sc.interfaces {
		for _, m := range i.methods {
			if len(m.results) < 1 {
				return fmt.Errorf("method %q of interface %q has no result types", m.name, i.name)
			}
		}
	}
	return nil
}

func (sc *sourceContext) importDecls() (decls []ast.Decl) {
	have := map[string]struct{}{}
	notHave := func(is *ast.ImportSpec) bool {
		if _, has := have[is.Path.Value]; has {
			return false
		}
		have[is.Path.Value] = struct{}{}
		return true
	}

	for _, is := range sc.imports {
		if notHave(is) {
			decls = append(decls, importFor(is))
		}
	}

	for _, is := range fetchImports() {
		if notHave(is) {
			decls = append(decls, &ast.GenDecl{Tok: token.IMPORT, Specs: []ast.Spec{is}})
		}
	}

	return
}

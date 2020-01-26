package main

import "go/ast"

type arg struct {
	name, asField *ast.Ident
	typ           ast.Expr
}

func (a arg) chooseName(scope *ast.Scope) *ast.Ident {
	if a.name == nil || scope.Lookup(a.name.Name) != nil {
		return inventName(a.typ, scope)
	}
	return a.name
}

func (a arg) field(scope *ast.Scope) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{a.chooseName(scope)},
		Type:  a.typ,
	}
}

func (a arg) result() *ast.Field {
	return &ast.Field{
		Names: nil,
		Type:  a.typ,
	}
}

func (a arg) exported() *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{id(export(a.asField.Name))},
		Type:  a.typ,
	}
}

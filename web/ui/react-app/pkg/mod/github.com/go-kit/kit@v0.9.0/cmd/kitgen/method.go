package main

import (
	"go/ast"
	"go/token"
	"strings"
)

type method struct {
	name            *ast.Ident
	params          []arg
	results         []arg
	structsResolved bool
}

func (m method) definition(ifc iface) ast.Decl {
	notImpl := fetchFuncDecl("ExampleEndpoint")

	notImpl.Name = m.name
	notImpl.Recv = fieldList(ifc.receiver())
	scope := scopeWith(notImpl.Recv.List[0].Names[0].Name)
	notImpl.Type.Params = m.funcParams(scope)
	notImpl.Type.Results = m.funcResults()

	return notImpl
}

func (m method) endpointMaker(ifc iface) ast.Decl {
	endpointFn := fetchFuncDecl("makeExampleEndpoint")
	scope := scopeWith("ctx", "req", ifc.receiverName().Name)

	anonFunc := endpointFn.Body.List[0].(*ast.ReturnStmt).Results[0].(*ast.FuncLit)
	if !m.hasContext() {
		// strip context param from endpoint function
		anonFunc.Type.Params.List = anonFunc.Type.Params.List[1:]
	}

	anonFunc = replaceIdent(anonFunc, "ExampleRequest", m.requestStructName()).(*ast.FuncLit)
	callMethod := m.called(ifc, scope, "ctx", "req")
	anonFunc.Body.List[1] = callMethod
	anonFunc.Body.List[2].(*ast.ReturnStmt).Results[0] = m.wrapResult(callMethod.Lhs)

	endpointFn.Body.List[0].(*ast.ReturnStmt).Results[0] = anonFunc
	endpointFn.Name = m.endpointMakerName()
	endpointFn.Type.Params = fieldList(ifc.receiver())
	endpointFn.Type.Results = fieldList(typeField(sel(id("endpoint"), id("Endpoint"))))
	return endpointFn
}

func (m method) pathName() string {
	return "/" + strings.ToLower(m.name.Name)
}

func (m method) encodeFuncName() *ast.Ident {
	return id("Encode" + m.name.Name + "Response")
}

func (m method) decodeFuncName() *ast.Ident {
	return id("Decode" + m.name.Name + "Request")
}

func (m method) resultNames(scope *ast.Scope) []*ast.Ident {
	ids := []*ast.Ident{}
	for _, rz := range m.results {
		ids = append(ids, rz.chooseName(scope))
	}
	return ids
}

func (m method) called(ifc iface, scope *ast.Scope, ctxName, spreadStruct string) *ast.AssignStmt {
	m.resolveStructNames()

	resNamesExpr := []ast.Expr{}
	for _, r := range m.resultNames(scope) {
		resNamesExpr = append(resNamesExpr, ast.Expr(r))
	}

	arglist := []ast.Expr{}
	if m.hasContext() {
		arglist = append(arglist, id(ctxName))
	}
	ssid := id(spreadStruct)
	for _, f := range m.requestStructFields().List {
		arglist = append(arglist, sel(ssid, f.Names[0]))
	}

	return &ast.AssignStmt{
		Lhs: resNamesExpr,
		Tok: token.DEFINE,
		Rhs: []ast.Expr{
			&ast.CallExpr{
				Fun:  sel(ifc.receiverName(), m.name),
				Args: arglist,
			},
		},
	}
}

func (m method) wrapResult(results []ast.Expr) ast.Expr {
	kvs := []ast.Expr{}
	m.resolveStructNames()

	for i, a := range m.results {
		kvs = append(kvs, &ast.KeyValueExpr{
			Key:   ast.NewIdent(export(a.asField.Name)),
			Value: results[i],
		})
	}
	return &ast.CompositeLit{
		Type: m.responseStructName(),
		Elts: kvs,
	}
}

func (m method) resolveStructNames() {
	if m.structsResolved {
		return
	}
	m.structsResolved = true
	scope := ast.NewScope(nil)
	for i, p := range m.params {
		p.asField = p.chooseName(scope)
		m.params[i] = p
	}
	scope = ast.NewScope(nil)
	for i, r := range m.results {
		r.asField = r.chooseName(scope)
		m.results[i] = r
	}
}

func (m method) decoderFunc() ast.Decl {
	fn := fetchFuncDecl("DecodeExampleRequest")
	fn.Name = m.decodeFuncName()
	fn = replaceIdent(fn, "ExampleRequest", m.requestStructName()).(*ast.FuncDecl)
	return fn
}

func (m method) encoderFunc() ast.Decl {
	fn := fetchFuncDecl("EncodeExampleResponse")
	fn.Name = m.encodeFuncName()
	return fn
}

func (m method) endpointMakerName() *ast.Ident {
	return id("Make" + m.name.Name + "Endpoint")
}

func (m method) requestStruct() ast.Decl {
	m.resolveStructNames()
	return structDecl(m.requestStructName(), m.requestStructFields())
}

func (m method) responseStruct() ast.Decl {
	m.resolveStructNames()
	return structDecl(m.responseStructName(), m.responseStructFields())
}

func (m method) hasContext() bool {
	if len(m.params) < 1 {
		return false
	}
	carg := m.params[0].typ
	// ugh. this is maybe okay for the one-off, but a general case for matching
	// types would be helpful
	if sel, is := carg.(*ast.SelectorExpr); is && sel.Sel.Name == "Context" {
		if id, is := sel.X.(*ast.Ident); is && id.Name == "context" {
			return true
		}
	}
	return false
}

func (m method) nonContextParams() []arg {
	if m.hasContext() {
		return m.params[1:]
	}
	return m.params
}

func (m method) funcParams(scope *ast.Scope) *ast.FieldList {
	parms := &ast.FieldList{}
	if m.hasContext() {
		parms.List = []*ast.Field{{
			Names: []*ast.Ident{ast.NewIdent("ctx")},
			Type:  sel(id("context"), id("Context")),
		}}
		scope.Insert(ast.NewObj(ast.Var, "ctx"))
	}
	parms.List = append(parms.List, mappedFieldList(func(a arg) *ast.Field {
		return a.field(scope)
	}, m.nonContextParams()...).List...)
	return parms
}

func (m method) funcResults() *ast.FieldList {
	return mappedFieldList(func(a arg) *ast.Field {
		return a.result()
	}, m.results...)
}

func (m method) requestStructName() *ast.Ident {
	return id(export(m.name.Name) + "Request")
}

func (m method) requestStructFields() *ast.FieldList {
	return mappedFieldList(func(a arg) *ast.Field {
		return a.exported()
	}, m.nonContextParams()...)
}

func (m method) responseStructName() *ast.Ident {
	return id(export(m.name.Name) + "Response")
}

func (m method) responseStructFields() *ast.FieldList {
	return mappedFieldList(func(a arg) *ast.Field {
		return a.exported()
	}, m.results...)
}

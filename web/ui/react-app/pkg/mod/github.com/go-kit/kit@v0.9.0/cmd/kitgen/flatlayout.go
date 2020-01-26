package main

import "go/ast"

type flat struct{}

func (f flat) transformAST(ctx *sourceContext) (files, error) {
	root := &ast.File{
		Name:  ctx.pkg,
		Decls: []ast.Decl{},
	}

	addImports(root, ctx)

	for _, typ := range ctx.types {
		addType(root, typ)
	}

	for _, iface := range ctx.interfaces { //only one...
		addStubStruct(root, iface)

		for _, meth := range iface.methods {
			addMethod(root, iface, meth)
			addRequestStruct(root, meth)
			addResponseStruct(root, meth)
			addEndpointMaker(root, iface, meth)
		}

		addEndpointsStruct(root, iface)
		addHTTPHandler(root, iface)

		for _, meth := range iface.methods {
			addDecoder(root, meth)
			addEncoder(root, meth)
		}
	}

	return formatNodes(outputTree{"gokit.go": root})
}

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"

	"golang.org/x/tools/imports"
)

type (
	files  map[string]io.Reader
	layout interface {
		transformAST(ctx *sourceContext) (files, error)
	}
	outputTree map[string]*ast.File
)

func (ot outputTree) addFile(path, pkgname string) *ast.File {
	file := &ast.File{
		Name:  id(pkgname),
		Decls: []ast.Decl{},
	}
	ot[path] = file
	return file
}

func getGopath() string {
	gopath, set := os.LookupEnv("GOPATH")
	if !set {
		return filepath.Join(os.Getenv("HOME"), "go")
	}
	return gopath
}

func importPath(targetDir, gopath string) (string, error) {
	if !filepath.IsAbs(targetDir) {
		return "", fmt.Errorf("%q is not an absolute path", targetDir)
	}

	for _, dir := range filepath.SplitList(gopath) {
		abspath, err := filepath.Abs(dir)
		if err != nil {
			continue
		}
		srcPath := filepath.Join(abspath, "src")

		res, err := filepath.Rel(srcPath, targetDir)
		if err != nil {
			continue
		}
		if strings.Index(res, "..") == -1 {
			return res, nil
		}
	}
	return "", fmt.Errorf("%q is not in GOPATH (%s)", targetDir, gopath)

}

func selectify(file *ast.File, pkgName, identName, importPath string) *ast.File {
	if file.Name.Name == pkgName {
		return file
	}

	selector := sel(id(pkgName), id(identName))
	var did bool
	if file, did = selectifyIdent(identName, file, selector); did {
		addImport(file, importPath)
	}
	return file
}

type selIdentFn func(ast.Node, func(ast.Node)) Visitor

func (f selIdentFn) Visit(node ast.Node, r func(ast.Node)) Visitor {
	return f(node, r)
}

func selectifyIdent(identName string, file *ast.File, selector ast.Expr) (*ast.File, bool) {
	var replaced bool
	var r selIdentFn
	r = selIdentFn(func(node ast.Node, replaceWith func(ast.Node)) Visitor {
		switch id := node.(type) {
		case *ast.SelectorExpr:
			return nil
		case *ast.Ident:
			if id.Name == identName {
				replaced = true
				replaceWith(selector)
			}
		}
		return r
	})
	return WalkReplace(r, file).(*ast.File), replaced
}

func formatNode(fname string, node ast.Node) (*bytes.Buffer, error) {
	if file, is := node.(*ast.File); is {
		sort.Stable(sortableDecls(file.Decls))
	}
	outfset := token.NewFileSet()
	buf := &bytes.Buffer{}
	err := format.Node(buf, outfset, node)
	if err != nil {
		return nil, err
	}
	imps, err := imports.Process(fname, buf.Bytes(), nil)
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(imps), nil
}

type sortableDecls []ast.Decl

func (sd sortableDecls) Len() int {
	return len(sd)
}

func (sd sortableDecls) Less(i int, j int) bool {
	switch left := sd[i].(type) {
	case *ast.GenDecl:
		switch right := sd[j].(type) {
		default:
			return left.Tok == token.IMPORT
		case *ast.GenDecl:
			return left.Tok == token.IMPORT && right.Tok != token.IMPORT
		}
	}
	return false
}

func (sd sortableDecls) Swap(i int, j int) {
	sd[i], sd[j] = sd[j], sd[i]
}

func formatNodes(nodes outputTree) (files, error) {
	res := files{}
	var err error
	for fn, node := range nodes {
		res[fn], err = formatNode(fn, node)
		if err != nil {
			return nil, errors.Wrapf(err, "formatNodes")
		}
	}
	return res, nil
}

// XXX debug
func spewDecls(f *ast.File) {
	for _, d := range f.Decls {
		switch dcl := d.(type) {
		default:
			spew.Dump(dcl)
		case *ast.GenDecl:
			spew.Dump(dcl.Tok)
		case *ast.FuncDecl:
			spew.Dump(dcl.Name.Name)
		}
	}
}

func addImports(root *ast.File, ctx *sourceContext) {
	root.Decls = append(root.Decls, ctx.importDecls()...)
}

func addImport(root *ast.File, path string) {
	for _, d := range root.Decls {
		if imp, is := d.(*ast.GenDecl); is && imp.Tok == token.IMPORT {
			for _, s := range imp.Specs {
				if s.(*ast.ImportSpec).Path.Value == `"`+filepath.ToSlash(path)+`"` {
					return // already have one
					// xxx aliased imports?
				}
			}
		}
	}
	root.Decls = append(root.Decls, importFor(importSpec(path)))
}

func addStubStruct(root *ast.File, iface iface) {
	root.Decls = append(root.Decls, iface.stubStructDecl())
}

func addType(root *ast.File, typ *ast.TypeSpec) {
	root.Decls = append(root.Decls, typeDecl(typ))
}

func addMethod(root *ast.File, iface iface, meth method) {
	def := meth.definition(iface)
	root.Decls = append(root.Decls, def)
}

func addRequestStruct(root *ast.File, meth method) {
	root.Decls = append(root.Decls, meth.requestStruct())
}

func addResponseStruct(root *ast.File, meth method) {
	root.Decls = append(root.Decls, meth.responseStruct())
}

func addEndpointMaker(root *ast.File, ifc iface, meth method) {
	root.Decls = append(root.Decls, meth.endpointMaker(ifc))
}

func addEndpointsStruct(root *ast.File, ifc iface) {
	root.Decls = append(root.Decls, ifc.endpointsStruct())
}

func addHTTPHandler(root *ast.File, ifc iface) {
	root.Decls = append(root.Decls, ifc.httpHandler())
}

func addDecoder(root *ast.File, meth method) {
	root.Decls = append(root.Decls, meth.decoderFunc())
}

func addEncoder(root *ast.File, meth method) {
	root.Decls = append(root.Decls, meth.encoderFunc())
}

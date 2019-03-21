// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

// The gcexportdata command is a diagnostic tool that displays the
// contents of gc export data files.
package main

import (
	"flag"
	"fmt"
	"go/token"
	"go/types"
	"log"
	"os"

	"golang.org/x/tools/go/gcexportdata"
	"golang.org/x/tools/go/types/typeutil"
)

var packageFlag = flag.String("package", "", "alternative package to print")

func main() {
	log.SetPrefix("gcexportdata: ")
	log.SetFlags(0)
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: gcexportdata [-package path] file.a")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	filename := flag.Args()[0]

	f, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}

	r, err := gcexportdata.NewReader(f)
	if err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	// Decode the package.
	const primary = "<primary>"
	imports := make(map[string]*types.Package)
	fset := token.NewFileSet()
	pkg, err := gcexportdata.Read(r, fset, imports, primary)
	if err != nil {
		log.Fatalf("%s: %s", filename, err)
	}

	// Optionally select an indirectly mentioned package.
	if *packageFlag != "" {
		pkg = imports[*packageFlag]
		if pkg == nil {
			fmt.Fprintf(os.Stderr, "export data file %s does not mention %s; has:\n",
				filename, *packageFlag)
			for p := range imports {
				if p != primary {
					fmt.Fprintf(os.Stderr, "\t%s\n", p)
				}
			}
			os.Exit(1)
		}
	}

	// Print all package-level declarations, including non-exported ones.
	fmt.Printf("package %s\n", pkg.Name())
	for _, imp := range pkg.Imports() {
		fmt.Printf("import %q\n", imp.Path())
	}
	qual := func(p *types.Package) string {
		if pkg == p {
			return ""
		}
		return p.Name()
	}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		fmt.Printf("%s: %s\n",
			fset.Position(obj.Pos()),
			types.ObjectString(obj, qual))

		// For types, print each method.
		if _, ok := obj.(*types.TypeName); ok {
			for _, method := range typeutil.IntuitiveMethodSet(obj.Type(), nil) {
				fmt.Printf("%s: %s\n",
					fset.Position(method.Obj().Pos()),
					types.SelectionString(method, qual))
			}
		}
	}
}

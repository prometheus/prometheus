package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path"

	"github.com/pkg/errors"
)

// go get github.com/nyarly/inlinefiles
//go:generate inlinefiles --package=main --vfs=ASTTemplates ./templates ast_templates.go

func usage() string {
	return fmt.Sprintf("Usage: %s <filename> (try -h)", os.Args[0])
}

var (
	help       = flag.Bool("h", false, "print this help")
	layoutkind = flag.String("repo-layout", "default", "default, flat...")
	outdirrel  = flag.String("target-dir", ".", "base directory to emit into")
	//contextOmittable = flag.Bool("allow-no-context", false, "allow service methods to omit context parameter")
)

func helpText() {
	fmt.Println("USAGE")
	fmt.Println("  kitgen [flags] path/to/service.go")
	fmt.Println("")
	fmt.Println("FLAGS")
	flag.PrintDefaults()
}

func main() {
	flag.Parse()

	if *help {
		helpText()
		os.Exit(0)
	}

	outdir := *outdirrel
	if !path.IsAbs(*outdirrel) {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatalf("error getting current working directory: %v", err)
		}
		outdir = path.Join(wd, *outdirrel)
	}

	var layout layout
	switch *layoutkind {
	default:
		log.Fatalf("Unrecognized layout kind: %q - try 'default' or 'flat'", *layoutkind)
	case "default":
		gopath := getGopath()
		importBase, err := importPath(outdir, gopath)
		if err != nil {
			log.Fatal(err)
		}
		layout = deflayout{targetDir: importBase}
	case "flat":
		layout = flat{}
	}

	if len(os.Args) < 2 {
		log.Fatal(usage())
	}
	filename := flag.Arg(0)
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("error while opening %q: %v", filename, err)
	}

	tree, err := process(filename, file, layout)
	if err != nil {
		log.Fatal(err)
	}

	err = splat(outdir, tree)
	if err != nil {
		log.Fatal(err)
	}
}

func process(filename string, source io.Reader, layout layout) (files, error) {
	f, err := parseFile(filename, source)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing input %q", filename)
	}

	context, err := extractContext(f)
	if err != nil {
		return nil, errors.Wrapf(err, "examining input file %q", filename)
	}

	tree, err := layout.transformAST(context)
	if err != nil {
		return nil, errors.Wrapf(err, "generating AST")
	}
	return tree, nil
}

/*
	buf, err := formatNode(dest)
	if err != nil {
		return nil, errors.Wrapf(err, "formatting")
	}
	return buf, nil
}
*/

func parseFile(fname string, source io.Reader) (ast.Node, error) {
	f, err := parser.ParseFile(token.NewFileSet(), fname, source, parser.DeclarationErrors)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func extractContext(f ast.Node) (*sourceContext, error) {
	context := &sourceContext{}
	visitor := &parseVisitor{src: context}

	ast.Walk(visitor, f)

	return context, context.validate()
}

func splat(dir string, tree files) error {
	for fn, buf := range tree {
		if err := splatFile(path.Join(dir, fn), buf); err != nil {
			return err
		}
	}
	return nil
}

func splatFile(target string, buf io.Reader) error {
	err := os.MkdirAll(path.Dir(target), os.ModePerm)
	if err != nil {
		return errors.Wrapf(err, "Couldn't create directory for %q", target)
	}
	f, err := os.Create(target)
	if err != nil {
		return errors.Wrapf(err, "Couldn't create file %q", target)
	}
	defer f.Close()
	_, err = io.Copy(f, buf)
	return errors.Wrapf(err, "Error writing data to file %q", target)
}

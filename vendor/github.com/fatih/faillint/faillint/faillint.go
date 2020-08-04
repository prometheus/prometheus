// Package faillint defines an Analyzer that fails when a package is imported
// that matches against a set of defined packages.
package faillint

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"dmitri.shuralyov.com/go/generated"
	"golang.org/x/tools/go/analysis"
)

const (
	// ignoreKey is used in a faillint directive to ignore a line-based problem.
	ignoreKey = "ignore"
	// fileIgnoreKey is used in a faillint directive to ignore a whole file.
	fileIgnoreKey = "file-ignore"
	// missingReasonTemplate is used when a faillint directive is missing a reason.
	missingReasonTemplate = "missing reason on faillint:%s directive"
	// unrecognizedOptionTemplate is used when a faillint directive has an option other than ignore or file-ignore.
	unrecognizedOptionTemplate = "unrecognized option on faillint directive: %s"

	unspecifiedUsage = "unspecified"
)

var (
	// Analyzer is a global instance of the linter.
	// DEPRECATED: Use faillint.New instead.
	Analyzer = NewAnalyzer()

	// pathsRegexp represents a regexp that is used to parse -paths flag.
	// It parses flag content in set of 3 subgroups:
	//
	// * import: Mandatory part. Go import path in URL format to be unwanted or have unwanted declarations.
	// * recursive: Optional part. Import paths in the form of foo/bar/... indicates that all recursive sub matchs should be also matched.
	// * declarations: Optional declarations in `{ }`. If set, using the import is allowed expect give declarations.
	// * suggestion: Optional suggestion to print when unwanted import or declaration is found.
	pathsRegexp = regexp.MustCompile(`(?P<import>[\w/.-]+[\w])(/?(?P<recursive>\.\.\.)|)(\.?{(?P<declarations>[\w-,]+)}|)(=(?P<suggestion>[\w/.-]+[\w](\.?{[\w-,]+}|))|)`)
)

// path represents a single parsed directive parsed with the pathsRegexp regex
type path struct {
	// imp contains the full import path
	imp string

	// recursive is true if the import path should encapsulate all sub paths
	// for the given import path as well.
	recursive bool

	// declarations contains the declarations to fail for the given import path
	decls []string

	// sugg defines the suggestion for a given import path
	sugg string
}

type faillint struct {
	paths       string // -paths flag
	ignoretests bool   // -ignore-tests flag
}

// NewAnalyzer create a faillint analyzer.
func NewAnalyzer() *analysis.Analyzer {
	f := faillint{
		paths:       "",
		ignoretests: false,
	}
	a := &analysis.Analyzer{
		Name:             "faillint",
		Doc:              "Report unwanted import path or exported declaration usages",
		Run:              f.run,
		RunDespiteErrors: true,
	}

	a.Flags.StringVar(&f.paths, "paths", "", `import paths or exported declarations (i.e: functions, constant, types or variables) to fail. E.g.:

Fail on the usage of errors and fmt.Errorf. Also suggest packages for the failures
  --paths errors=github.com/pkg/errors,fmt.{Errorf}=github.com/pkg/errors.{Errorf}

Fail on the usage of fmt.Println, fmt.Print and fmt.Printf
  --paths fmt.{Println,Print,Printf}

Fail on the usage of prometheus.DefaultGatherer and prometheus.MustRegister
  --paths github.com/prometheus/client_golang/prometheus.{DefaultGatherer,MustRegister}

Fail on the usage of errors, golang.org/x/net and all sub packages under golang.org/x/net
  --paths errors,golang.org/x/net/...`)
	a.Flags.BoolVar(&f.ignoretests, "ignore-tests", false, "ignore all _test.go files")
	return a
}

func trimAllWhitespaces(str string) string {
	var b strings.Builder
	b.Grow(len(str))
	for _, ch := range str {
		if !unicode.IsSpace(ch) {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

// run is the runner for an analysis pass.
func (f *faillint) run(pass *analysis.Pass) (interface{}, error) {
	if f.paths == "" {
		return nil, nil
	}

	for _, file := range pass.Files {
		filename := pass.Fset.File(file.Package).Name()
		isGenerated, err := generated.ParseFile(filename)
		if err != nil {
			return nil, err
		}

		if isGenerated {
			continue
		}

		if f.ignoretests && strings.Contains(pass.Fset.File(file.Package).Name(), "_test.go") {
			continue
		}
		if anyHasDirective(pass, file.Comments, fileIgnoreKey) {
			continue
		}
		commentMap := ast.NewCommentMap(pass.Fset, file, file.Comments)
		for _, path := range parsePaths(f.paths) {
			specs := importSpec(file, path.imp, path.recursive)
			if len(specs) == 0 {
				continue
			}

			for _, spec := range specs {
				if usageHasDirective(pass, commentMap, spec, spec.Pos(), ignoreKey) {
					continue
				}

				usages := importUsages(pass, commentMap, file, spec)
				if len(usages) == 0 {
					continue
				}

				if _, ok := usages[unspecifiedUsage]; ok || len(path.decls) == 0 {
					// File using unwanted import. Report.
					msg := fmt.Sprintf("package %q shouldn't be imported", importPath(spec))
					if path.sugg != "" {
						msg += fmt.Sprintf(", suggested: %q", path.sugg)
					}
					pass.Reportf(spec.Path.Pos(), msg)
					continue
				}

				// Not all usages are forbidden. Report only unwanted declarations.
				for _, declaration := range path.decls {
					positions, ok := usages[declaration]
					if !ok {
						continue
					}
					msg := fmt.Sprintf("declaration %q from package %q shouldn't be used", declaration, importPath(spec))
					if path.sugg != "" {
						msg += fmt.Sprintf(", suggested: %q", path.sugg)
					}
					for _, pos := range positions {
						pass.Reportf(pos, msg)
					}
				}
			}
		}
	}

	return nil, nil
}

// importUsages reports all exported declaration used for a given import.
func importUsages(pass *analysis.Pass, commentMap ast.CommentMap, f *ast.File, spec *ast.ImportSpec) map[string][]token.Pos {
	importRef := spec.Name.String()
	switch importRef {
	case "<nil>":
		importRef, _ = strconv.Unquote(spec.Path.Value)
		// If the package importRef is not explicitly specified,
		// make an educated guess. This is not guaranteed to be correct.
		lastSlash := strings.LastIndex(importRef, "/")
		if lastSlash != -1 {
			importRef = importRef[lastSlash+1:]
		}
	case "_", ".":
		// Not sure if this import is used - on the side of caution, report special "unspecified" usage.
		return map[string][]token.Pos{unspecifiedUsage: nil}
	}
	usages := map[string][]token.Pos{}

	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if isTopName(sel.X, importRef) {
			if usageHasDirective(pass, commentMap, n, sel.Sel.NamePos, ignoreKey) {
				return true
			}
			usages[sel.Sel.Name] = append(usages[sel.Sel.Name], sel.Sel.NamePos)
		}
		return true
	})
	return usages
}

// importSpecs returns all import specs for f import statements importing path.
func importSpec(f *ast.File, path string, recursive bool) (imports []*ast.ImportSpec) {
	for _, s := range f.Imports {
		impPath := importPath(s)
		if impPath == path {
			imports = append(imports, s)
		} else if recursive && strings.HasPrefix(impPath, path+"/") {
			// match all subpaths as well, i.e:
			//   impPath = golang.org/x/net/context
			//      path = golang.org/x/net
			// We add the "/" so we don't match packages with dashes, such as
			// `golang.org/x/net-
			imports = append(imports, s)
		}
	}
	return imports
}

// importPath returns the unquoted import path of s,
// or "" if the path is not properly quoted.
func importPath(s *ast.ImportSpec) string {
	t, err := strconv.Unquote(s.Path.Value)
	if err == nil {
		return t
	}
	return ""
}

// isTopName returns true if n is a top-level unresolved identifier with the given name.
func isTopName(n ast.Expr, name string) bool {
	id, ok := n.(*ast.Ident)
	return ok && id.Name == name && id.Obj == nil
}

func parseDirective(pass *analysis.Pass, c *ast.Comment) (option string) {
	s := c.Text
	if !strings.HasPrefix(s, "//lint:") {
		return ""
	}
	s = strings.TrimPrefix(s, "//lint:")
	fields := strings.SplitN(s, " ", 3)

	if len(fields) < 2 {
		return ""
	}

	if fields[1] != "faillint" {
		return ""
	}

	if fields[0] != ignoreKey && fields[0] != fileIgnoreKey {
		pass.Reportf(c.Pos(), unrecognizedOptionTemplate, fields[0])
		return ""
	}

	if len(fields) < 3 {
		pass.Reportf(c.Pos(), missingReasonTemplate, fields[0])
		return ""
	}

	return fields[0]
}

func anyHasDirective(pass *analysis.Pass, cgs []*ast.CommentGroup, option string) bool {
	for _, cg := range cgs {
		if hasDirective(pass, cg, option) {
			return true
		}
	}
	return false
}

func hasDirective(pass *analysis.Pass, cg *ast.CommentGroup, option string) bool {
	if cg == nil {
		return false
	}
	for _, c := range cg.List {
		if parseDirective(pass, c) == option {
			return true
		}
	}
	return false
}

func usageHasDirective(pass *analysis.Pass, cm ast.CommentMap, n ast.Node, p token.Pos, option string) bool {
	for _, cg := range cm[n] {
		if hasDirective(pass, cg, ignoreKey) {
			return true
		}
	}
	// Try to find an "enclosing" node which the ast.CommentMap will
	// thus have associated comments to this field selector.
	for node := range cm {
		if p >= node.Pos() && p <= node.End() {
			for _, cg := range cm[node] {
				if hasDirective(pass, cg, option) {
					return true
				}
			}
		}
	}
	return false
}

func parsePaths(paths string) []path {
	pathGroups := pathsRegexp.FindAllStringSubmatch(trimAllWhitespaces(paths), -1)

	parsed := make([]path, 0, len(pathGroups))
	for _, group := range pathGroups {
		p := path{}
		for i, name := range pathsRegexp.SubexpNames() {
			switch name {
			case "import":
				p.imp = group[i]
			case "recursive":
				p.recursive = group[i] != ""
			case "suggestion":
				p.sugg = group[i]
			case "declarations":
				if group[i] == "" {
					break
				}
				p.decls = strings.Split(group[i], ",")
			}
		}
		parsed = append(parsed, p)
	}
	return parsed
}

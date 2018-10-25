// Package lint provides the foundation for tools like staticcheck
package lint // import "honnef.co/go/tools/lint"

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/tools/go/packages"
	"honnef.co/go/tools/config"
	"honnef.co/go/tools/ssa"
	"honnef.co/go/tools/ssa/ssautil"
)

type Job struct {
	Program *Program

	checker  string
	check    Check
	problems []Problem

	duration time.Duration
}

type Ignore interface {
	Match(p Problem) bool
}

type LineIgnore struct {
	File    string
	Line    int
	Checks  []string
	matched bool
	pos     token.Pos
}

func (li *LineIgnore) Match(p Problem) bool {
	if p.Position.Filename != li.File || p.Position.Line != li.Line {
		return false
	}
	for _, c := range li.Checks {
		if m, _ := filepath.Match(c, p.Check); m {
			li.matched = true
			return true
		}
	}
	return false
}

func (li *LineIgnore) String() string {
	matched := "not matched"
	if li.matched {
		matched = "matched"
	}
	return fmt.Sprintf("%s:%d %s (%s)", li.File, li.Line, strings.Join(li.Checks, ", "), matched)
}

type FileIgnore struct {
	File   string
	Checks []string
}

func (fi *FileIgnore) Match(p Problem) bool {
	if p.Position.Filename != fi.File {
		return false
	}
	for _, c := range fi.Checks {
		if m, _ := filepath.Match(c, p.Check); m {
			return true
		}
	}
	return false
}

type GlobIgnore struct {
	Pattern string
	Checks  []string
}

func (gi *GlobIgnore) Match(p Problem) bool {
	if gi.Pattern != "*" {
		pkgpath := p.Package.Types.Path()
		if strings.HasSuffix(pkgpath, "_test") {
			pkgpath = pkgpath[:len(pkgpath)-len("_test")]
		}
		name := filepath.Join(pkgpath, filepath.Base(p.Position.Filename))
		if m, _ := filepath.Match(gi.Pattern, name); !m {
			return false
		}
	}
	for _, c := range gi.Checks {
		if m, _ := filepath.Match(c, p.Check); m {
			return true
		}
	}
	return false
}

type Program struct {
	SSA              *ssa.Program
	InitialPackages  []*Pkg
	InitialFunctions []*ssa.Function
	AllPackages      []*packages.Package
	AllFunctions     []*ssa.Function
	Files            []*ast.File
	GoVersion        int

	tokenFileMap map[*token.File]*ast.File
	astFileMap   map[*ast.File]*Pkg
	packagesMap  map[string]*packages.Package

	genMu        sync.RWMutex
	generatedMap map[string]bool
}

func (prog *Program) Fset() *token.FileSet {
	return prog.InitialPackages[0].Fset
}

type Func func(*Job)

type Severity uint8

const (
	Error Severity = iota
	Warning
	Ignored
)

// Problem represents a problem in some source code.
type Problem struct {
	Position token.Position // position in source file
	Text     string         // the prose that describes the problem
	Check    string
	Checker  string
	Package  *Pkg
	Severity Severity
}

func (p *Problem) String() string {
	if p.Check == "" {
		return p.Text
	}
	return fmt.Sprintf("%s (%s)", p.Text, p.Check)
}

type Checker interface {
	Name() string
	Prefix() string
	Init(*Program)
	Checks() []Check
}

type Check struct {
	Fn              Func
	ID              string
	FilterGenerated bool
}

// A Linter lints Go source code.
type Linter struct {
	Checkers      []Checker
	Ignores       []Ignore
	GoVersion     int
	ReturnIgnored bool
	Config        config.Config

	MaxConcurrentJobs int
	PrintStats        bool

	automaticIgnores []Ignore
}

func (l *Linter) ignore(p Problem) bool {
	ignored := false
	for _, ig := range l.automaticIgnores {
		// We cannot short-circuit these, as we want to record, for
		// each ignore, whether it matched or not.
		if ig.Match(p) {
			ignored = true
		}
	}
	if ignored {
		// no need to execute other ignores if we've already had a
		// match.
		return true
	}
	for _, ig := range l.Ignores {
		// We can short-circuit here, as we aren't tracking any
		// information.
		if ig.Match(p) {
			return true
		}
	}

	return false
}

func (prog *Program) File(node Positioner) *ast.File {
	return prog.tokenFileMap[prog.SSA.Fset.File(node.Pos())]
}

func (j *Job) File(node Positioner) *ast.File {
	return j.Program.File(node)
}

func parseDirective(s string) (cmd string, args []string) {
	if !strings.HasPrefix(s, "//lint:") {
		return "", nil
	}
	s = strings.TrimPrefix(s, "//lint:")
	fields := strings.Split(s, " ")
	return fields[0], fields[1:]
}

type PerfStats struct {
	PackageLoading time.Duration
	SSABuild       time.Duration
	OtherInitWork  time.Duration
	CheckerInits   map[string]time.Duration
	Jobs           []JobStat
}

type JobStat struct {
	Job      string
	Duration time.Duration
}

func (stats *PerfStats) Print(w io.Writer) {
	fmt.Fprintln(w, "Package loading:", stats.PackageLoading)
	fmt.Fprintln(w, "SSA build:", stats.SSABuild)
	fmt.Fprintln(w, "Other init work:", stats.OtherInitWork)

	fmt.Fprintln(w, "Checker inits:")
	for checker, d := range stats.CheckerInits {
		fmt.Fprintf(w, "\t%s: %s\n", checker, d)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Jobs:")
	sort.Slice(stats.Jobs, func(i, j int) bool {
		return stats.Jobs[i].Duration < stats.Jobs[j].Duration
	})
	var total time.Duration
	for _, job := range stats.Jobs {
		fmt.Fprintf(w, "\t%s: %s\n", job.Job, job.Duration)
		total += job.Duration
	}
	fmt.Fprintf(w, "\tTotal: %s\n", total)
}

func (l *Linter) Lint(initial []*packages.Package, stats *PerfStats) []Problem {
	allPkgs := allPackages(initial)
	t := time.Now()
	ssaprog, _ := ssautil.Packages(allPkgs, ssa.GlobalDebug)
	ssaprog.Build()
	if stats != nil {
		stats.SSABuild = time.Since(t)
	}

	t = time.Now()
	pkgMap := map[*ssa.Package]*Pkg{}
	var pkgs []*Pkg
	for _, pkg := range initial {
		ssapkg := ssaprog.Package(pkg.Types)
		var cfg config.Config
		if len(pkg.GoFiles) != 0 {
			path := pkg.GoFiles[0]
			dir := filepath.Dir(path)
			var err error
			// OPT(dh): we're rebuilding the entire config tree for
			// each package. for example, if we check a/b/c and
			// a/b/c/d, we'll process a, a/b, a/b/c, a, a/b, a/b/c,
			// a/b/c/d – we should cache configs per package and only
			// load the new levels.
			cfg, err = config.Load(dir)
			if err != nil {
				// FIXME(dh): we couldn't load the config, what are we
				// supposed to do? probably tell the user somehow
			}
			cfg = cfg.Merge(l.Config)
		}

		pkg := &Pkg{
			SSA:     ssapkg,
			Package: pkg,
			Config:  cfg,
		}
		pkgMap[ssapkg] = pkg
		pkgs = append(pkgs, pkg)
	}

	prog := &Program{
		SSA:             ssaprog,
		InitialPackages: pkgs,
		AllPackages:     allPkgs,
		GoVersion:       l.GoVersion,
		tokenFileMap:    map[*token.File]*ast.File{},
		astFileMap:      map[*ast.File]*Pkg{},
		generatedMap:    map[string]bool{},
	}
	prog.packagesMap = map[string]*packages.Package{}
	for _, pkg := range allPkgs {
		prog.packagesMap[pkg.Types.Path()] = pkg
	}

	isInitial := map[*types.Package]struct{}{}
	for _, pkg := range pkgs {
		isInitial[pkg.Types] = struct{}{}
	}
	for fn := range ssautil.AllFunctions(ssaprog) {
		if fn.Pkg == nil {
			continue
		}
		prog.AllFunctions = append(prog.AllFunctions, fn)
		if _, ok := isInitial[fn.Pkg.Pkg]; ok {
			prog.InitialFunctions = append(prog.InitialFunctions, fn)
		}
	}
	for _, pkg := range pkgs {
		prog.Files = append(prog.Files, pkg.Syntax...)

		ssapkg := ssaprog.Package(pkg.Types)
		for _, f := range pkg.Syntax {
			prog.astFileMap[f] = pkgMap[ssapkg]
		}
	}

	for _, pkg := range allPkgs {
		for _, f := range pkg.Syntax {
			tf := pkg.Fset.File(f.Pos())
			prog.tokenFileMap[tf] = f
		}
	}

	var out []Problem
	l.automaticIgnores = nil
	for _, pkg := range initial {
		for _, f := range pkg.Syntax {
			cm := ast.NewCommentMap(pkg.Fset, f, f.Comments)
			for node, cgs := range cm {
				for _, cg := range cgs {
					for _, c := range cg.List {
						if !strings.HasPrefix(c.Text, "//lint:") {
							continue
						}
						cmd, args := parseDirective(c.Text)
						switch cmd {
						case "ignore", "file-ignore":
							if len(args) < 2 {
								// FIXME(dh): this causes duplicated warnings when using megacheck
								p := Problem{
									Position: prog.DisplayPosition(c.Pos()),
									Text:     "malformed linter directive; missing the required reason field?",
									Check:    "",
									Checker:  "lint",
									Package:  nil,
								}
								out = append(out, p)
								continue
							}
						default:
							// unknown directive, ignore
							continue
						}
						checks := strings.Split(args[0], ",")
						pos := prog.DisplayPosition(node.Pos())
						var ig Ignore
						switch cmd {
						case "ignore":
							ig = &LineIgnore{
								File:   pos.Filename,
								Line:   pos.Line,
								Checks: checks,
								pos:    c.Pos(),
							}
						case "file-ignore":
							ig = &FileIgnore{
								File:   pos.Filename,
								Checks: checks,
							}
						}
						l.automaticIgnores = append(l.automaticIgnores, ig)
					}
				}
			}
		}
	}

	sizes := struct {
		types      int
		defs       int
		uses       int
		implicits  int
		selections int
		scopes     int
	}{}
	for _, pkg := range pkgs {
		sizes.types += len(pkg.TypesInfo.Types)
		sizes.defs += len(pkg.TypesInfo.Defs)
		sizes.uses += len(pkg.TypesInfo.Uses)
		sizes.implicits += len(pkg.TypesInfo.Implicits)
		sizes.selections += len(pkg.TypesInfo.Selections)
		sizes.scopes += len(pkg.TypesInfo.Scopes)
	}

	if stats != nil {
		stats.OtherInitWork = time.Since(t)
	}

	for _, checker := range l.Checkers {
		t := time.Now()
		checker.Init(prog)
		if stats != nil {
			stats.CheckerInits[checker.Name()] = time.Since(t)
		}
	}

	var jobs []*Job
	var allChecks []string

	for _, checker := range l.Checkers {
		checks := checker.Checks()
		for _, check := range checks {
			allChecks = append(allChecks, check.ID)
			j := &Job{
				Program: prog,
				checker: checker.Name(),
				check:   check,
			}
			jobs = append(jobs, j)
		}
	}

	max := len(jobs)
	if l.MaxConcurrentJobs > 0 {
		max = l.MaxConcurrentJobs
	}

	sem := make(chan struct{}, max)
	wg := &sync.WaitGroup{}
	for _, j := range jobs {
		wg.Add(1)
		go func(j *Job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			fn := j.check.Fn
			if fn == nil {
				return
			}
			t := time.Now()
			fn(j)
			j.duration = time.Since(t)
		}(j)
	}
	wg.Wait()

	for _, j := range jobs {
		if stats != nil {
			stats.Jobs = append(stats.Jobs, JobStat{j.check.ID, j.duration})
		}
		for _, p := range j.problems {
			allowedChecks := FilterChecks(allChecks, p.Package.Config.Checks)

			if l.ignore(p) {
				p.Severity = Ignored
			}
			// TODO(dh): support globs in check white/blacklist
			// OPT(dh): this approach doesn't actually disable checks,
			// it just discards their results. For the moment, that's
			// fine. None of our checks are super expensive. In the
			// future, we may want to provide opt-in expensive
			// analysis, which shouldn't run at all. It may be easiest
			// to implement this in the individual checks.
			if (l.ReturnIgnored || p.Severity != Ignored) && allowedChecks[p.Check] {
				out = append(out, p)
			}
		}
	}

	for _, ig := range l.automaticIgnores {
		ig, ok := ig.(*LineIgnore)
		if !ok {
			continue
		}
		if ig.matched {
			continue
		}

		couldveMatched := false
		for f, pkg := range prog.astFileMap {
			if prog.Fset().Position(f.Pos()).Filename != ig.File {
				continue
			}
			allowedChecks := FilterChecks(allChecks, pkg.Config.Checks)
			for _, c := range ig.Checks {
				if !allowedChecks[c] {
					continue
				}
				couldveMatched = true
				break
			}
			break
		}

		if !couldveMatched {
			// The ignored checks were disabled for the containing package.
			// Don't flag the ignore for not having matched.
			continue
		}
		p := Problem{
			Position: prog.DisplayPosition(ig.pos),
			Text:     "this linter directive didn't match anything; should it be removed?",
			Check:    "",
			Checker:  "lint",
			Package:  nil,
		}
		out = append(out, p)
	}

	sort.Slice(out, func(i int, j int) bool {
		pi, pj := out[i].Position, out[j].Position

		if pi.Filename != pj.Filename {
			return pi.Filename < pj.Filename
		}
		if pi.Line != pj.Line {
			return pi.Line < pj.Line
		}
		if pi.Column != pj.Column {
			return pi.Column < pj.Column
		}

		return out[i].Text < out[j].Text
	})

	if len(out) < 2 {
		return out
	}

	uniq := make([]Problem, 0, len(out))
	uniq = append(uniq, out[0])
	prev := out[0]
	for _, p := range out[1:] {
		if prev.Position == p.Position && prev.Text == p.Text {
			continue
		}
		prev = p
		uniq = append(uniq, p)
	}

	if l.PrintStats && stats != nil {
		stats.Print(os.Stderr)
	}
	return uniq
}

func FilterChecks(allChecks []string, checks []string) map[string]bool {
	// OPT(dh): this entire computation could be cached per package
	allowedChecks := map[string]bool{}

	for _, check := range checks {
		b := true
		if len(check) > 1 && check[0] == '-' {
			b = false
			check = check[1:]
		}
		if check == "*" || check == "all" {
			// Match all
			for _, c := range allChecks {
				allowedChecks[c] = b
			}
		} else if strings.HasSuffix(check, "*") {
			// Glob
			prefix := check[:len(check)-1]
			isCat := strings.IndexFunc(prefix, func(r rune) bool { return unicode.IsNumber(r) }) == -1

			for _, c := range allChecks {
				idx := strings.IndexFunc(c, func(r rune) bool { return unicode.IsNumber(r) })
				if isCat {
					// Glob is S*, which should match S1000 but not SA1000
					cat := c[:idx]
					if prefix == cat {
						allowedChecks[c] = b
					}
				} else {
					// Glob is S1*
					if strings.HasPrefix(c, prefix) {
						allowedChecks[c] = b
					}
				}
			}
		} else {
			// Literal check name
			allowedChecks[check] = b
		}
	}
	return allowedChecks
}

func (prog *Program) Package(path string) *packages.Package {
	return prog.packagesMap[path]
}

// Pkg represents a package being linted.
type Pkg struct {
	SSA *ssa.Package
	*packages.Package
	Config config.Config
}

type Positioner interface {
	Pos() token.Pos
}

func (prog *Program) DisplayPosition(p token.Pos) token.Position {
	// Only use the adjusted position if it points to another Go file.
	// This means we'll point to the original file for cgo files, but
	// we won't point to a YACC grammar file.

	pos := prog.Fset().PositionFor(p, false)
	adjPos := prog.Fset().PositionFor(p, true)

	if filepath.Ext(adjPos.Filename) == ".go" {
		return adjPos
	}
	return pos
}

func (prog *Program) isGenerated(path string) bool {
	// This function isn't very efficient in terms of lock contention
	// and lack of parallelism, but it really shouldn't matter.
	// Projects consists of thousands of files, and have hundreds of
	// errors. That's not a lot of calls to isGenerated.

	prog.genMu.RLock()
	if b, ok := prog.generatedMap[path]; ok {
		prog.genMu.RUnlock()
		return b
	}
	prog.genMu.RUnlock()
	prog.genMu.Lock()
	defer prog.genMu.Unlock()
	// recheck to avoid doing extra work in case of race
	if b, ok := prog.generatedMap[path]; ok {
		return b
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b := isGenerated(f)
	prog.generatedMap[path] = b
	return b
}

func (j *Job) Errorf(n Positioner, format string, args ...interface{}) *Problem {
	tf := j.Program.SSA.Fset.File(n.Pos())
	f := j.Program.tokenFileMap[tf]
	pkg := j.Program.astFileMap[f]

	pos := j.Program.DisplayPosition(n.Pos())
	if j.Program.isGenerated(pos.Filename) && j.check.FilterGenerated {
		return nil
	}
	problem := Problem{
		Position: pos,
		Text:     fmt.Sprintf(format, args...),
		Check:    j.check.ID,
		Checker:  j.checker,
		Package:  pkg,
	}
	j.problems = append(j.problems, problem)
	return &j.problems[len(j.problems)-1]
}

func (j *Job) NodePackage(node Positioner) *Pkg {
	f := j.File(node)
	return j.Program.astFileMap[f]
}

// TODO(dh): replace with packages.Visit
func allPackages(pkgs []*packages.Package) []*packages.Package {
	all := map[*packages.Package]bool{}
	var wl []*packages.Package
	wl = append(wl, pkgs...)
	for len(wl) > 0 {
		pkg := wl[len(wl)-1]
		wl = wl[:len(wl)-1]
		if all[pkg] {
			continue
		}
		all[pkg] = true
		for _, imp := range pkg.Imports {
			wl = append(wl, imp)
		}
	}

	out := make([]*packages.Package, 0, len(all))
	for pkg := range all {
		out = append(out, pkg)
	}
	return out
}

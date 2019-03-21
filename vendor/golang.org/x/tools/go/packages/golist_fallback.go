// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package packages

import (
	"encoding/json"
	"fmt"
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/internal/cgo"
)

// TODO(matloob): Delete this file once Go 1.12 is released.

// This file provides backwards compatibility support for
// loading for versions of Go earlier than 1.11. This support is meant to
// assist with migration to the Package API until there's
// widespread adoption of these newer Go versions.
// This support will be removed once Go 1.12 is released
// in Q1 2019.

func golistDriverFallback(cfg *Config, words ...string) (*driverResponse, error) {
	// Turn absolute paths into GOROOT and GOPATH-relative paths to provide to go list.
	// This will have surprising behavior if GOROOT or GOPATH contain multiple packages with the same
	// path and a user provides an absolute path to a directory that's shadowed by an earlier
	// directory in GOROOT or GOPATH with the same package path.
	words = cleanAbsPaths(cfg, words)

	original, deps, err := getDeps(cfg, words...)
	if err != nil {
		return nil, err
	}

	var tmpdir string // used for generated cgo files
	var needsTestVariant []struct {
		pkg, xtestPkg *Package
	}

	var response driverResponse
	allPkgs := make(map[string]bool)
	addPackage := func(p *jsonPackage, isRoot bool) {
		id := p.ImportPath

		if allPkgs[id] {
			return
		}
		allPkgs[id] = true

		pkgpath := id

		if pkgpath == "unsafe" {
			p.GoFiles = nil // ignore fake unsafe.go file
		}

		importMap := func(importlist []string) map[string]*Package {
			importMap := make(map[string]*Package)
			for _, id := range importlist {

				if id == "C" {
					for _, path := range []string{"unsafe", "syscall", "runtime/cgo"} {
						if pkgpath != path && importMap[path] == nil {
							importMap[path] = &Package{ID: path}
						}
					}
					continue
				}
				importMap[vendorlessPath(id)] = &Package{ID: id}
			}
			return importMap
		}
		compiledGoFiles := absJoin(p.Dir, p.GoFiles)
		// Use a function to simplify control flow. It's just a bunch of gotos.
		var cgoErrors []error
		var outdir string
		getOutdir := func() (string, error) {
			if outdir != "" {
				return outdir, nil
			}
			if tmpdir == "" {
				if tmpdir, err = ioutil.TempDir("", "gopackages"); err != nil {
					return "", err
				}
			}
			// Add a "go-build" component to the path to make the tests think the files are in the cache.
			// This allows the same test to test the pre- and post-Go 1.11 go list logic because the Go 1.11
			// go list generates test mains in the cache, and the test code knows not to rely on paths in the
			// cache to stay stable.
			outdir = filepath.Join(tmpdir, "go-build", strings.Replace(p.ImportPath, "/", "_", -1))
			if err := os.MkdirAll(outdir, 0755); err != nil {
				outdir = ""
				return "", err
			}
			return outdir, nil
		}
		processCgo := func() bool {
			// Suppress any cgo errors. Any relevant errors will show up in typechecking.
			// TODO(matloob): Skip running cgo if Mode < LoadTypes.
			outdir, err := getOutdir()
			if err != nil {
				cgoErrors = append(cgoErrors, err)
				return false
			}
			files, _, err := runCgo(p.Dir, outdir, cfg.Env)
			if err != nil {
				cgoErrors = append(cgoErrors, err)
				return false
			}
			compiledGoFiles = append(compiledGoFiles, files...)
			return true
		}
		if len(p.CgoFiles) == 0 || !processCgo() {
			compiledGoFiles = append(compiledGoFiles, absJoin(p.Dir, p.CgoFiles)...) // Punt to typechecker.
		}
		if isRoot {
			response.Roots = append(response.Roots, id)
		}
		pkg := &Package{
			ID:              id,
			Name:            p.Name,
			GoFiles:         absJoin(p.Dir, p.GoFiles, p.CgoFiles),
			CompiledGoFiles: compiledGoFiles,
			OtherFiles:      absJoin(p.Dir, otherFiles(p)...),
			PkgPath:         pkgpath,
			Imports:         importMap(p.Imports),
			// TODO(matloob): set errors on the Package to cgoErrors
		}
		if p.Error != nil {
			pkg.Errors = append(pkg.Errors, Error{
				Pos: p.Error.Pos,
				Msg: p.Error.Err,
			})
		}
		response.Packages = append(response.Packages, pkg)
		if cfg.Tests && isRoot {
			testID := fmt.Sprintf("%s [%s.test]", id, id)
			if len(p.TestGoFiles) > 0 || len(p.XTestGoFiles) > 0 {
				response.Roots = append(response.Roots, testID)
				testPkg := &Package{
					ID:              testID,
					Name:            p.Name,
					GoFiles:         absJoin(p.Dir, p.GoFiles, p.CgoFiles, p.TestGoFiles),
					CompiledGoFiles: append(compiledGoFiles, absJoin(p.Dir, p.TestGoFiles)...),
					OtherFiles:      absJoin(p.Dir, otherFiles(p)...),
					PkgPath:         pkgpath,
					Imports:         importMap(append(p.Imports, p.TestImports...)),
					// TODO(matloob): set errors on the Package to cgoErrors
				}
				response.Packages = append(response.Packages, testPkg)
				var xtestPkg *Package
				if len(p.XTestGoFiles) > 0 {
					xtestID := fmt.Sprintf("%s_test [%s.test]", id, id)
					response.Roots = append(response.Roots, xtestID)
					// Generate test variants for all packages q where a path exists
					// such that xtestPkg -> ... -> q -> ... -> p (where p is the package under test)
					// and rewrite all import map entries of p to point to testPkg (the test variant of
					// p), and of each q  to point to the test variant of that q.
					xtestPkg = &Package{
						ID:              xtestID,
						Name:            p.Name + "_test",
						GoFiles:         absJoin(p.Dir, p.XTestGoFiles),
						CompiledGoFiles: absJoin(p.Dir, p.XTestGoFiles),
						PkgPath:         pkgpath + "_test",
						Imports:         importMap(p.XTestImports),
					}
					// Add to list of packages we need to rewrite imports for to refer to test variants.
					// We may need to create a test variant of a package that hasn't been loaded yet, so
					// the test variants need to be created later.
					needsTestVariant = append(needsTestVariant, struct{ pkg, xtestPkg *Package }{pkg, xtestPkg})
					response.Packages = append(response.Packages, xtestPkg)
				}
				// testmain package
				testmainID := id + ".test"
				response.Roots = append(response.Roots, testmainID)
				imports := map[string]*Package{}
				imports[testPkg.PkgPath] = &Package{ID: testPkg.ID}
				if xtestPkg != nil {
					imports[xtestPkg.PkgPath] = &Package{ID: xtestPkg.ID}
				}
				testmainPkg := &Package{
					ID:      testmainID,
					Name:    "main",
					PkgPath: testmainID,
					Imports: imports,
				}
				response.Packages = append(response.Packages, testmainPkg)
				outdir, err := getOutdir()
				if err != nil {
					testmainPkg.Errors = append(testmainPkg.Errors, Error{
						Pos:  "-",
						Msg:  fmt.Sprintf("failed to generate testmain: %v", err),
						Kind: ListError,
					})
					return
				}
				testmain := filepath.Join(outdir, "testmain.go")
				extraimports, extradeps, err := generateTestmain(testmain, testPkg, xtestPkg)
				if err != nil {
					testmainPkg.Errors = append(testmainPkg.Errors, Error{
						Pos:  "-",
						Msg:  fmt.Sprintf("failed to generate testmain: %v", err),
						Kind: ListError,
					})
				}
				deps = append(deps, extradeps...)
				for _, imp := range extraimports { // testing, testing/internal/testdeps, and maybe os
					imports[imp] = &Package{ID: imp}
				}
				testmainPkg.GoFiles = []string{testmain}
				testmainPkg.CompiledGoFiles = []string{testmain}
			}
		}
	}

	for _, pkg := range original {
		addPackage(pkg, true)
	}
	if cfg.Mode < LoadImports || len(deps) == 0 {
		return &response, nil
	}

	buf, err := invokeGo(cfg, golistArgsFallback(cfg, deps)...)
	if err != nil {
		return nil, err
	}

	// Decode the JSON and convert it to Package form.
	for dec := json.NewDecoder(buf); dec.More(); {
		p := new(jsonPackage)
		if err := dec.Decode(p); err != nil {
			return nil, fmt.Errorf("JSON decoding failed: %v", err)
		}

		addPackage(p, false)
	}

	for _, v := range needsTestVariant {
		createTestVariants(&response, v.pkg, v.xtestPkg)
	}

	return &response, nil
}

func createTestVariants(response *driverResponse, pkgUnderTest, xtestPkg *Package) {
	allPkgs := make(map[string]*Package)
	for _, pkg := range response.Packages {
		allPkgs[pkg.ID] = pkg
	}
	needsTestVariant := make(map[string]bool)
	needsTestVariant[pkgUnderTest.ID] = true
	var needsVariantRec func(p *Package) bool
	needsVariantRec = func(p *Package) bool {
		if needsTestVariant[p.ID] {
			return true
		}
		for _, imp := range p.Imports {
			if needsVariantRec(allPkgs[imp.ID]) {
				// Don't break because we want to make sure all dependencies
				// have been processed, and all required test variants of our dependencies
				// exist.
				needsTestVariant[p.ID] = true
			}
		}
		if !needsTestVariant[p.ID] {
			return false
		}
		// Create a clone of the package. It will share the same strings and lists of source files,
		// but that's okay. It's only necessary for the Imports map to have a separate identity.
		testVariant := *p
		testVariant.ID = fmt.Sprintf("%s [%s.test]", p.ID, pkgUnderTest.ID)
		testVariant.Imports = make(map[string]*Package)
		for imp, pkg := range p.Imports {
			testVariant.Imports[imp] = pkg
			if needsTestVariant[pkg.ID] {
				testVariant.Imports[imp] = &Package{ID: fmt.Sprintf("%s [%s.test]", pkg.ID, pkgUnderTest.ID)}
			}
		}
		response.Packages = append(response.Packages, &testVariant)
		return needsTestVariant[p.ID]
	}
	// finally, update the xtest package's imports
	for imp, pkg := range xtestPkg.Imports {
		if allPkgs[pkg.ID] == nil {
			fmt.Printf("for %s: package %s doesn't exist\n", xtestPkg.ID, pkg.ID)
		}
		if needsVariantRec(allPkgs[pkg.ID]) {
			xtestPkg.Imports[imp] = &Package{ID: fmt.Sprintf("%s [%s.test]", pkg.ID, pkgUnderTest.ID)}
		}
	}
}

// cleanAbsPaths replaces all absolute paths with GOPATH- and GOROOT-relative
// paths. If an absolute path is not GOPATH- or GOROOT- relative, it is left as an
// absolute path so an error can be returned later.
func cleanAbsPaths(cfg *Config, words []string) []string {
	var searchpaths []string
	var cleaned = make([]string, len(words))
	for i := range cleaned {
		cleaned[i] = words[i]
		// Ignore relative directory paths (they must already be goroot-relative) and Go source files
		// (absolute source files are already allowed for ad-hoc packages).
		// TODO(matloob): Can there be non-.go files in ad-hoc packages.
		if !filepath.IsAbs(cleaned[i]) || strings.HasSuffix(cleaned[i], ".go") {
			continue
		}
		// otherwise, it's an absolute path. Search GOPATH and GOROOT to find it.
		if searchpaths == nil {
			cmd := exec.Command("go", "env", "GOPATH", "GOROOT")
			cmd.Env = cfg.Env
			out, err := cmd.Output()
			if err != nil {
				searchpaths = []string{}
				continue // suppress the error, it will show up again when running go list
			}
			lines := strings.Split(string(out), "\n")
			if len(lines) != 3 || lines[0] == "" || lines[1] == "" || lines[2] != "" {
				continue // suppress error
			}
			// first line is GOPATH
			for _, path := range filepath.SplitList(lines[0]) {
				searchpaths = append(searchpaths, filepath.Join(path, "src"))
			}
			// second line is GOROOT
			searchpaths = append(searchpaths, filepath.Join(lines[1], "src"))
		}
		for _, sp := range searchpaths {
			if strings.HasPrefix(cleaned[i], sp) {
				cleaned[i] = strings.TrimPrefix(cleaned[i], sp)
				cleaned[i] = strings.TrimLeft(cleaned[i], string(filepath.Separator))
			}
		}
	}
	return cleaned
}

// vendorlessPath returns the devendorized version of the import path ipath.
// For example, VendorlessPath("foo/bar/vendor/a/b") returns "a/b".
// Copied from golang.org/x/tools/imports/fix.go.
func vendorlessPath(ipath string) string {
	// Devendorize for use in import statement.
	if i := strings.LastIndex(ipath, "/vendor/"); i >= 0 {
		return ipath[i+len("/vendor/"):]
	}
	if strings.HasPrefix(ipath, "vendor/") {
		return ipath[len("vendor/"):]
	}
	return ipath
}

// getDeps runs an initial go list to determine all the dependency packages.
func getDeps(cfg *Config, words ...string) (initial []*jsonPackage, deps []string, err error) {
	buf, err := invokeGo(cfg, golistArgsFallback(cfg, words)...)
	if err != nil {
		return nil, nil, err
	}

	depsSet := make(map[string]bool)
	var testImports []string

	// Extract deps from the JSON.
	for dec := json.NewDecoder(buf); dec.More(); {
		p := new(jsonPackage)
		if err := dec.Decode(p); err != nil {
			return nil, nil, fmt.Errorf("JSON decoding failed: %v", err)
		}

		initial = append(initial, p)
		for _, dep := range p.Deps {
			depsSet[dep] = true
		}
		if cfg.Tests {
			// collect the additional imports of the test packages.
			pkgTestImports := append(p.TestImports, p.XTestImports...)
			for _, imp := range pkgTestImports {
				if depsSet[imp] {
					continue
				}
				depsSet[imp] = true
				testImports = append(testImports, imp)
			}
		}
	}
	// Get the deps of the packages imported by tests.
	if len(testImports) > 0 {
		buf, err = invokeGo(cfg, golistArgsFallback(cfg, testImports)...)
		if err != nil {
			return nil, nil, err
		}
		// Extract deps from the JSON.
		for dec := json.NewDecoder(buf); dec.More(); {
			p := new(jsonPackage)
			if err := dec.Decode(p); err != nil {
				return nil, nil, fmt.Errorf("JSON decoding failed: %v", err)
			}
			for _, dep := range p.Deps {
				depsSet[dep] = true
			}
		}
	}

	for _, orig := range initial {
		delete(depsSet, orig.ImportPath)
	}

	deps = make([]string, 0, len(depsSet))
	for dep := range depsSet {
		deps = append(deps, dep)
	}
	sort.Strings(deps) // ensure output is deterministic
	return initial, deps, nil
}

func golistArgsFallback(cfg *Config, words []string) []string {
	fullargs := []string{"list", "-e", "-json"}
	fullargs = append(fullargs, cfg.BuildFlags...)
	fullargs = append(fullargs, "--")
	fullargs = append(fullargs, words...)
	return fullargs
}

func runCgo(pkgdir, tmpdir string, env []string) (files, displayfiles []string, err error) {
	// Use go/build to open cgo files and determine the cgo flags, etc, from them.
	// This is tricky so it's best to avoid reimplementing as much as we can, and
	// we plan to delete this support once Go 1.12 is released anyways.
	// TODO(matloob): This isn't completely correct because we're using the Default
	// context. Perhaps we should more accurately fill in the context.
	bp, err := build.ImportDir(pkgdir, build.ImportMode(0))
	if err != nil {
		return nil, nil, err
	}
	for _, ev := range env {
		if v := strings.TrimPrefix(ev, "CGO_CPPFLAGS"); v != ev {
			bp.CgoCPPFLAGS = append(bp.CgoCPPFLAGS, strings.Fields(v)...)
		} else if v := strings.TrimPrefix(ev, "CGO_CFLAGS"); v != ev {
			bp.CgoCFLAGS = append(bp.CgoCFLAGS, strings.Fields(v)...)
		} else if v := strings.TrimPrefix(ev, "CGO_CXXFLAGS"); v != ev {
			bp.CgoCXXFLAGS = append(bp.CgoCXXFLAGS, strings.Fields(v)...)
		} else if v := strings.TrimPrefix(ev, "CGO_LDFLAGS"); v != ev {
			bp.CgoLDFLAGS = append(bp.CgoLDFLAGS, strings.Fields(v)...)
		}
	}
	return cgo.Run(bp, pkgdir, tmpdir, true)
}

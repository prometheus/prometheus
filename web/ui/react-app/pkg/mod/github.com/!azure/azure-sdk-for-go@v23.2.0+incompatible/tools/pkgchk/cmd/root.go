// Copyright 2018 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var excepFileFlag string

var rootCmd = &cobra.Command{
	Use:   "pkgchk <dir>",
	Short: "Performs package validation tasks against all packages found under the specified directory.",
	Long: `This tool will perform various package validation checks against all of the packages
found under the specified directory.  Failures can be baselined and thus ignored by
copying the failure text verbatim, pasting it into a text file then specifying that
file via the optional exceptions flag.
`,
	Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(cmd, args); err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		return theCommand(args)
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&excepFileFlag, "exceptions", "e", "", "text file containing the list of exceptions")
}

// Execute executes the specified command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func theCommand(args []string) error {
	rootDir := args[0]
	if !filepath.IsAbs(rootDir) {
		asAbs, err := filepath.Abs(rootDir)
		if err != nil {
			return errors.Wrap(err, "failed to get absolute path")
		}
		rootDir = asAbs
	}

	pkgs, err := getPkgs(rootDir)
	if err != nil {
		return errors.Wrap(err, "failed to get packages")
	}

	var exceptions []string
	if excepFileFlag != "" {
		exceptions, err = loadExceptions(excepFileFlag)
		if err != nil {
			return errors.Wrap(err, "failed to load exceptions")
		}
	}
	verifiers := getVerifiers()
	count := 0
	for _, pkg := range pkgs {
		for _, v := range verifiers {
			if err = v(pkg); err != nil && !contains(exceptions, err.Error()) {
				fmt.Fprintln(os.Stderr, err)
				count++
			}
		}
	}

	var res error
	if count > 0 {
		res = fmt.Errorf("found %d errors", count)
	}
	return res
}

func contains(items []string, item string) bool {
	if items == nil {
		return false
	}
	for _, i := range items {
		if i == item {
			return true
		}
	}
	return false
}

func loadExceptions(excepFile string) ([]string, error) {
	f, err := os.Open(excepFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	exceps := []string{}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		exceps = append(exceps, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		return nil, err
	}

	return exceps, nil
}

type pkg struct {
	// the directory where the package resides relative to the root dir
	d string

	// the AST of the package
	p *ast.Package
}

// returns true if the package directory corresponds to an ARM package
func (p pkg) isARMPkg() bool {
	return strings.Index(p.d, "/mgmt/") > -1
}

// walks the directory hierarchy from the specified root returning a slice of all the packages found
func getPkgs(rootDir string) ([]pkg, error) {
	pkgs := []pkg{}
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// check if leaf dir
			fi, err := ioutil.ReadDir(path)
			if err != nil {
				return err
			}
			hasSubDirs := false
			interfacesDir := false
			for _, f := range fi {
				if f.IsDir() {
					hasSubDirs = true
					break
				}
				if f.Name() == "interfaces.go" {
					interfacesDir = true
				}
			}
			if !hasSubDirs {
				fs := token.NewFileSet()
				// with interfaces codegen the majority of leaf directories are now the
				// *api packages. when this is the case parse from the parent directory.
				if interfacesDir {
					path = filepath.Dir(path)
				}
				packages, err := parser.ParseDir(fs, path, func(fi os.FileInfo) bool {
					return fi.Name() != "interfaces.go"
				}, parser.PackageClauseOnly)
				if err != nil {
					return err
				}
				if len(packages) < 1 {
					return errors.New("didn't find any packages which is unexpected")
				}
				if len(packages) > 1 {
					return errors.New("found more than one package which is unexpected")
				}
				var p *ast.Package
				for _, pkgs := range packages {
					p = pkgs
				}
				// normalize directory separator to '/' character
				pkgs = append(pkgs, pkg{
					d: strings.Replace(path[len(rootDir):], "\\", "/", -1),
					p: p,
				})
			}
		}
		return nil
	})
	return pkgs, err
}

type verifier func(p pkg) error

// returns a list of verifiers to execute
func getVerifiers() []verifier {
	return []verifier{
		verifyPkgMatchesDir,
		verifyLowerCase,
		verifyDirectorySturcture,
	}
}

// ensures that the leaf directory name matches the package name
func verifyPkgMatchesDir(p pkg) error {
	leaf := p.d[strings.LastIndex(p.d, "/")+1:]
	if strings.Compare(leaf, p.p.Name) != 0 {
		return fmt.Errorf("leaf directory of '%s' doesn't match package name '%s'", p.d, p.p.Name)
	}
	return nil
}

// ensures that there are no upper-case letters in a package's directory
func verifyLowerCase(p pkg) error {
	// walk the package directory looking for upper-case characters
	for _, r := range p.d {
		if r == '/' {
			continue
		}
		if unicode.IsUpper(r) {
			return fmt.Errorf("found upper-case character in directory '%s'", p.d)
		}
	}
	return nil
}

// ensures that the package's directory hierarchy is properly formed
func verifyDirectorySturcture(p pkg) error {
	// for ARM the package directory structure is highly deterministic:
	// /redis/mgmt/2015-08-01/redis
	// /resources/mgmt/2017-06-01-preview/policy
	// /preview/signalr/mgmt/2018-03-01-preview/signalr
	if !p.isARMPkg() {
		return nil
	}
	regexStr := strings.Join([]string{
		`^(?:/preview)?`,
		`[a-z0-9\-]+`,
		`mgmt`,
		`\d{4}-\d{2}-\d{2}(?:-preview)?`,
		`[a-z0-9]+`,
	}, "/")
	regex := regexp.MustCompile(regexStr)
	if !regex.MatchString(p.d) {
		return fmt.Errorf("bad directory structure '%s'", p.d)
	}
	return nil
}

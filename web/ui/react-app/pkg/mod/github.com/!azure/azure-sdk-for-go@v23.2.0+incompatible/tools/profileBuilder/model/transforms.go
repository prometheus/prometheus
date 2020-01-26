// +build go1.9

// Copyright 2018 Microsoft Corporation and contributors
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

// Package model holds the business logic for the operations made available by
// profileBuilder.
//
// This package is not governed by the SemVer associated with the rest of the
// Azure-SDK-for-Go.
package model

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/marstr/collection"
	goalias "github.com/marstr/goalias/model"
	"golang.org/x/tools/imports"
)

type Alias struct {
	*goalias.AliasPackage
	TargetPath string
}

const armPathModifier = "mgmt"

var packageName = regexp.MustCompile(`services[/\\](?P<provider>[\w\-\.\d_\\/]+)[/\\](?:(?P<arm>` + armPathModifier + `)[/\\])?(?P<version>v?\d{4}-\d{2}-\d{2}[\w\d\.\-]*|v?\d+\.\d+[\.\d\w\-]*)[/\\](?P<group>[/\\\w\d\-\._]+)`)

// BuildProfile takes a list of packages and creates a profile
func BuildProfile(packageList collection.Enumerable, name, outputLocation string, outputLog, errLog *log.Logger) {
	var packages collection.Enumerator

	// Find the names of all of the packages for inclusion in this profile.
	packages = packageList.Enumerate(nil).Select(func(x interface{}) interface{} {
		if cast, ok := x.(string); ok {
			return cast
		}
		return nil
	})

	// Parse the packages that were selected for inclusion in this profile.
	packages = packages.SelectMany(func(x interface{}) collection.Enumerator {
		results := make(chan interface{})

		go func() {
			defer close(results)

			cast, ok := x.(string)
			if !ok {
				return
			}
			files := token.NewFileSet()
			parsed, err := parser.ParseDir(files, cast, nil, 0)
			if err != nil {
				errLog.Printf("Couldn't open %q because: %v", cast, err)
				return
			}

			for _, entry := range parsed {
				results <- entry
			}
		}()

		return results
	})

	packages = GenerateAliasPackages(packages, name, outputLog, errLog)
	products := packages.ParallelSelect(GetAliasWriter(outputLocation, outputLog, errLog))

	generated := 0

	// Write each aliased package that was found
	for entry := range products {
		if entry.(bool) {
			generated++
		}
	}
	outputLog.Print(generated, " packages generated.")
}

// GetAliasPath takes an existing API Version path and a package name, and converts the path
// to a path which uses the new profile layout.
func GetAliasPath(subject, profile string) (transformed string, err error) {
	subject = strings.TrimSuffix(subject, "/")
	subject = TrimGoPath(subject)

	matches := packageName.FindAllStringSubmatch(subject, -1)
	if matches == nil {
		err = fmt.Errorf("path '%s' does not resemble a known package path", subject)
		return
	}

	output := []string{
		profile,
		matches[0][1],
	}

	if matches[0][2] == armPathModifier {
		output = append(output, armPathModifier)
	}

	output = append(output, matches[0][4])

	transformed = strings.Join(output, "/")
	return
}

// TrimGoPath removes the prefix defined in the environment variabe GOPATH if it is present in the string provided.
func TrimGoPath(subject string) string {
	splitGo := strings.Split(os.Getenv("GOPATH"), string(os.PathSeparator))
	splitGo = append(splitGo, "src")
	splitPath := strings.Split(subject, string(os.PathSeparator))
	for i, dir := range splitGo {
		if splitPath[i] != dir {
			return subject
		}
	}
	packageIdentifier := splitPath[len(splitGo):]
	return path.Join(packageIdentifier...)
}

// GenerateAliasPackages creates an enumerator, which when called, will create Alias packages ready to be
// written to disk.
func GenerateAliasPackages(packages collection.Enumerator, profileName string, outputLog, errLog *log.Logger) collection.Enumerator {
	packages = packages.ParallelSelect(GetAliasMaker(profileName, outputLog, errLog))
	packages = packages.Select(GetUserAgentUpdater(profileName))
	return packages
}

// GetUserAgentUpdater creates a lambda which will transform alias packages into
// professing the Profile that the package is a part of.
func GetUserAgentUpdater(profileName string) collection.Transform {
	return func(x interface{}) interface{} {
		return updateAliasPackageUserAgent(x, profileName)
	}
}

//updateAliasPackageUserAgent updates the "UserAgent" function in the generated profile, if it is present.
func updateAliasPackageUserAgent(x interface{}, profileName string) interface{} {
	cast, ok := x.(*Alias)

	if !ok {
		return nil
	}

	var userAgent *ast.FuncDecl

	// Grab all functions in the alias package named "UserAgent"
	userAgentCandidates := collection.Where(collection.AsEnumerable(cast.Files["models.go"].Decls), func(x interface{}) bool {
		cast, ok := x.(*ast.FuncDecl)
		return ok && cast.Name.Name == "UserAgent"
	})

	// There should really only be one of them, otherwise bailout because we don't understand the world anymore.
	candidate, err := collection.Single(userAgentCandidates)
	if err != nil {
		return x
	}
	userAgent, ok = candidate.(*ast.FuncDecl)
	if !ok {
		return x
	}

	// Grab the expression being returned.
	retResults := &userAgent.Body.List[0].(*ast.ReturnStmt).Results[0]

	// Append a string literal to the result
	updated := &ast.BinaryExpr{
		Op: token.ADD,
		X:  *retResults,
		Y: &ast.BasicLit{
			Value: fmt.Sprintf(`" profiles/%s"`, profileName),
		},
	}

	*retResults = updated
	return x
}

// GetAliasMaker creates a lambda which fetches Alias packages when given a pointer to
// an existing
func GetAliasMaker(profileName string, outputLog, errLog *log.Logger) collection.Transform {
	return func(x interface{}) interface{} {
		return generateIntermediateAliasPackage(x, profileName, outputLog, errLog)
	}
}

// generateIntermediateAliasPackage generates the alias package from the originally parsed one.
func generateIntermediateAliasPackage(x interface{}, profileName string, outputLog, errLog *log.Logger) interface{} {
	var err error
	var subject *goalias.AliasPackage
	cast, ok := x.(*ast.Package)
	if !ok {
		return nil
	}

	var bundle Alias
	for filename := range cast.Files {
		bundle.TargetPath = filepath.Dir(filename)
		bundle.TargetPath = TrimGoPath(bundle.TargetPath)
		subject, err = goalias.NewAliasPackage(cast, bundle.TargetPath)
		if err != nil {
			errLog.Print(err)
			return nil
		}
		bundle.TargetPath, err = GetAliasPath(bundle.TargetPath, profileName)
		if err != nil {
			errLog.Print(err)
			return nil
		}
		break
	}

	bundle.AliasPackage = subject
	return &bundle
}

// GetAliasWriter gets a `collection.Transform` which writes a package to
// disk at the specified location.
func GetAliasWriter(outputLocation string, outputLog, errLog *log.Logger) collection.Transform {
	return func(x interface{}) interface{} {
		return writeAliasPackage(x, outputLocation, outputLog, errLog)
	}
}

// writeAliasPackage adds the MSFT Copyright Header, then writes the alias package to disk.
func writeAliasPackage(x interface{}, outputLocation string, outputLog, errLog *log.Logger) interface{} {
	cast, ok := x.(*Alias)
	if !ok {
		return false
	}

	files := token.NewFileSet()

	outputPath := filepath.Join(outputLocation, cast.TargetPath, "models.go")
	outputPath = strings.Replace(outputPath, `\`, `/`, -1)
	err := os.MkdirAll(path.Dir(outputPath), os.ModePerm|os.ModeDir)
	if err != nil {
		errLog.Print("error creating directory:", err)
		return false
	}

	outputFile, err := os.Create(outputPath)
	if err != nil {
		errLog.Print("error creating file: ", err)
		return false
	}

	// TODO: This should really be added by the `goalias` package itself. Doing it here is a work around
	fmt.Fprintln(outputFile, "// +build go1.9")
	fmt.Fprintln(outputFile)

	generatorStampBuilder := new(bytes.Buffer)

	fmt.Fprintf(generatorStampBuilder, "// Copyright %4d Microsoft Corporation\n", time.Now().Year())
	fmt.Fprintln(generatorStampBuilder, `//
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
// limitations under the License.`)

	fmt.Fprintln(outputFile, generatorStampBuilder.String())

	generatorStampBuilder.Reset()

	fmt.Fprintln(generatorStampBuilder, "// This code was auto-generated by:")
	fmt.Fprintln(generatorStampBuilder, "// github.com/Azure/azure-sdk-for-go/tools/profileBuilder")

	fmt.Fprintln(generatorStampBuilder)
	fmt.Fprint(outputFile, generatorStampBuilder.String())

	outputLog.Printf("Writing File: %s", outputPath)

	file := cast.ModelFile()

	var b bytes.Buffer
	printer.Fprint(&b, files, file)
	res, _ := imports.Process(outputPath, b.Bytes(), nil)
	fmt.Fprintf(outputFile, "%s", res)
	outputFile.Close()

	if err := exec.Command("gofmt", "-w", outputPath).Run(); err == nil {
		outputLog.Print("Success formatting profile.")
	} else {
		errLog.Print("Trouble formatting profile: ", err)
	}
	return true
}

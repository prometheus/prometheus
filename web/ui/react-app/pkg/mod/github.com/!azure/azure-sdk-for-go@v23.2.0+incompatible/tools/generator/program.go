// Copyright 2017 Microsoft Corporation and contributors.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//
// See the License for the specific language governing permissions and
// limitations under the License.

// generator scours the github.com/Azure/azure-rest-api-specs repository in search for
// REST functionality that has Go configuration, then generates packages accordingly.
// This tool was developed with the intention that it be an internal tool for the
// Azure-SDK-for-Go team, but the usage of this package for public scenarios is in no
// way prohibited. For example, have a fork of https://github.com/Azure/autorest.go?
// Generate out and SDK with the same shape as our SDK, but using your patterns using
// this tool.
//
// Given that this code was developed as an internal tool, troubles with it that do not
// pertain to our usage of it may be slow to be fixed. Pull Requests are welcome though,
// feel free to contribute.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/marstr/collection"
)

const (
	defaultGenVer = "@microsoft.azure/autorest.go@v2.1.62"
)

var (
	targetFiles    collection.Enumerable
	packageVersion string
	outputBase     string
	logDirBase     string
	autorestVer    string
	errLog         *log.Logger
	statusLog      *log.Logger
	dryRun         bool
)

// version should be set by the linker when compiling this program by providing the following arguments:
// -X main.version=<version>
//
// If installing the generator in your machine, that means running the following the command:
//   go install -ldflags "-X main.version=<version>"
//
// The reason this is not controlled in source, is to allow for the git commit SHA1 to be used as the
// version of the string. To retrieve the currently checked-out git commit SHA1, you can use the command:
//   git rev-parse HEAD
//
// If this value is set, it is recommended that it be the value of the SHA1 git commit identifier for the
// source that was used at build time.
var version string

func main() {
	start := time.Now()

	type generationTuple struct {
		fileName     string
		packageName  string
		outputFolder string
	}

	literateFiles := collection.Where(targetFiles, func(subject interface{}) bool {
		return strings.EqualFold(path.Base(subject.(string)), "README.md")
	})

	stableAPIPaths := []string{}

	tuples := collection.SelectMany(literateFiles,
		// The following function compiles the regexp which finds Go related package tags in a literate file, and creates a collection.Unfolder.
		// This function has been declared this way so that the relatively expensive act of compiling a regexp is only done once.
		func() collection.Unfolder {
			const goSettingsPatternText = "```" + `\s+yaml\s+\$\(\s*tag\s*\)\s*==\s*'([\d\w\-\.]+)'\s*&&\s*\$\(go\)[\w\d\-\s:]*\s+output-folder:\s+\$\(go-sdk-folder\)([\w\d\-_\\/\.]+)\s+[\w\d\-\s:]*` + "```"
			goConfigPattern := regexp.MustCompile(goSettingsPatternText)

			const packagePatternText = `(?ms)\s+yaml\s+\$\(\s*tag\s*\)\s*==\s*'([\d\w\-\.]+)'\s*^input-file:(\s+.*?)\x60`
			packageConfigPattern := regexp.MustCompile(packagePatternText)

			// This function is a collection.Unfolder which takes a literate file as a path, and retrieves all configuration which applies to a package tag and Go.
			return func(subject interface{}) collection.Enumerator {
				results := make(chan interface{})

				go func() {
					defer close(results)

					targetContents, err := ioutil.ReadFile(subject.(string))
					if err != nil {
						errLog.Printf("Skipping %q because: %v", subject.(string), err)
						return
					}

					matches := goConfigPattern.Copy().FindAllStringSubmatch(string(targetContents), -1)

					if len(matches) == 0 {
						statusLog.Printf("Skipping %q because there were no package tags with go configuration found.", subject.(string))
					} else {
						packageMatches := packageConfigPattern.Copy().FindAllStringSubmatch(string(targetContents), -1)
						outputAPIFolders := map[string]string{}

						for _, submatch := range matches {

							packageName := normalizePath(submatch[1])
							outputFolder := normalizePath(submatch[2])
							leafFolder := getLeafOutputFolder(outputFolder)
							isStableAPI := isStableAPI(packageName, packageMatches)
							_, ok := outputAPIFolders[leafFolder]

							if !ok && isStableAPI {
								outputAPIFolders[leafFolder] = packageName
								stableAPIPaths = append(stableAPIPaths, path.Join(getRelativeOutputBase(), strings.Trim(outputFolder, "/")))
							}

							results <- generationTuple{
								fileName:     subject.(string),
								packageName:  packageName,
								outputFolder: outputFolder,
							}
						}
					}
				}()

				return results
			}
		}())

	if dryRun {
		for entry := range tuples.Enumerate(nil) {
			tuple := entry.(generationTuple)
			fmt.Printf("%q in %q to %q\n", tuple.packageName, tuple.fileName, tuple.outputFolder)
		}
	} else {
		var generatedCount, formattedCount, builtCount, vettedCount uint

		done := make(chan struct{})
		// Call AutoRest for each of the tags in each of the literate files, generating a Go package for calling that service.
		generated := tuples.Enumerate(done).ParallelSelect(func(subject interface{}) interface{} {
			tuple := subject.(generationTuple)
			args := []string{
				tuple.fileName,
				"--go",
				fmt.Sprintf("--go-sdk-folder='%s'", outputBase),
				"--verbose",
				"--tag=" + tuple.packageName,
				"--use=" + autorestVer,
				"--use-onever",
			}

			if packageVersion != "" {
				args = append(args, fmt.Sprintf("--package-version='%s'", packageVersion))
			}

			logFileLoc := filepath.Join(logDirBase, tuple.outputFolder)
			err := os.MkdirAll(logFileLoc, os.ModePerm)
			if err != nil {
				errLog.Printf("Could not create log directory %q", logFileLoc)
				return nil
			}

			logFile, err := os.Create(filepath.Join(logFileLoc, "autorestLog.txt"))
			if err != nil {
				errLog.Printf("Could not create log file %q for AutoRest generating from: %q in %q", logFileLoc, tuple.packageName, tuple.fileName)
				return nil
			}

			commandText := new(bytes.Buffer)
			fmt.Fprint(commandText, "Executing Command: \"")
			fmt.Fprint(commandText, "autorest ")
			for _, a := range args {
				fmt.Fprint(commandText, a)
				fmt.Fprint(commandText, " ")
			}
			commandText.Truncate(commandText.Len() - 1)
			fmt.Fprint(commandText, `"`)

			fmt.Fprintln(logFile, commandText.String())

			genProc := exec.Command("autorest", args...)
			genProc.Stdout = logFile
			genProc.Stderr = logFile

			err = genProc.Run()
			if err != nil {
				fmt.Fprintln(logFile, "Autorest Exectution Error: ", err)
				return nil
			}
			generatedCount++
			return path.Join(outputBase, tuple.outputFolder)
		}).Where(isntNil)

		// Take all of the generated packages and reformat them to adhere to Go's guidelines.
		formatted := generated.Select(func(subject interface{}) (result interface{}) {
			err := exec.Command("gofmt", "-w", subject.(string)).Run()
			if err == nil {
				formattedCount++
				result = subject
			} else {
				errLog.Printf("Failed to format: %q", subject.(string))
			}
			return
		}).Where(isntNil)

		// Build all of the packages as a sanity check.
		built := formatted.Select(func(subject interface{}) (result interface{}) {
			pkgName := strings.TrimPrefix(trimGoPath(subject.(string)), "/src/")
			err := exec.Command("go", "build", pkgName).Run()
			if err == nil {
				builtCount++
				return pkgName
			}
			errLog.Printf("Failed to build: %q", pkgName)
			return nil
		}).Where(isntNil)

		vetted := built.Select(func(subject interface{}) interface{} {
			err := exec.Command("go", "vet", subject.(string)).Run()
			if err == nil {
				vettedCount++
				return subject
			}
			errLog.Printf("Failed to vet: %q", subject.(string))
			return nil
		}).Where(isntNil)

		// Turn the crank. This loop forces evaluation of the chain of enumerators that were built up
		// in the code above.
		for range vetted {
			// Intenionally Left Blank
		}

		// Write list of stable apis.
		writeListToFile(path.Join(outputBase, "profiles", "latest", "stableApis.txt"), stableAPIPaths)

		fmt.Println("Execution Time: ", time.Now().Sub(start))
		fmt.Println("Generated: ", generatedCount)
		fmt.Println("Formatted: ", formattedCount)
		fmt.Println("Built: ", builtCount)
		fmt.Println("Vetted: ", vettedCount)
		fmt.Println("Stable APIs count: ", len(stableAPIPaths))
		close(done)
	}
}

func init() {
	var useRecursive, useStatus bool

	const automaticLogDirValue = "temp"
	const logDirUsage = "The root directory where all logs can be found. If the value `" + automaticLogDirValue + "` is supplied, a randomly named directory will be generated in the $TEMP directory."

	flag.BoolVar(&useRecursive, "r", false, "Recursively traverses the directories specified looking for literate files.")
	flag.StringVar(&outputBase, "o", getDefaultOutputBase(), "The root directory to use for the output of generated files. i.e. The value to be treated as the go-sdk-folder when AutoRest is called.")
	flag.BoolVar(&useStatus, "v", false, "Print status messages as generation takes place.")
	flag.BoolVar(&dryRun, "p", false, "Preview which packages would be generated instead of actaully calling autorest.")
	flag.StringVar(&packageVersion, "version", "", "The version that should be stamped on this SDK. This should be a semver.")
	flag.StringVar(&logDirBase, "l", getDefaultOutputBase(), logDirUsage)
	flag.StringVar(&autorestVer, "a", defaultGenVer, "The version of the AutoRest Go code generator to use, defaults to `"+defaultGenVer+"`.")

	// Override the default usage message, printing to stderr as the default one would.
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "generator [options] [target Literate files]...")
		flag.PrintDefaults()
	}

	flag.Parse()

	statusWriter := ioutil.Discard
	if useStatus {
		statusWriter = os.Stdout
	}
	statusLog = log.New(statusWriter, "[STATUS] ", 0)
	errLog = log.New(os.Stderr, "[ERROR] ", 0)

	if logDirBase == automaticLogDirValue {
		var err error
		logDirBase, err = ioutil.TempDir("", "az-go-sdk-logs")
		logDirBase = normalizePath(logDirBase)
		if err == nil {
			statusLog.Print("Generation logs can be found at: ", logDirBase)
		} else {
			errLog.Print("Logging disabled. Could not create directory: ", logDirBase)
		}
	}

	targetFiles = collection.AsEnumerable(flag.Args())
	targetFiles = collection.SelectMany(targetFiles, func(subject interface{}) collection.Enumerator {
		cast, ok := subject.(string)

		if !ok {
			return collection.Empty.Enumerate(nil)
		}
		pathInfo, err := os.Stat(cast)
		if err != nil {
			return collection.Empty.Enumerate(nil)
		}

		if pathInfo.IsDir() {
			traverser := collection.Directory{
				Location: cast,
				Options:  collection.DirectoryOptionsExcludeDirectories,
			}

			if useRecursive {
				traverser.Options |= collection.DirectoryOptionsRecursive
			}

			return traverser.Enumerate(nil)
		}

		return collection.AsEnumerable(cast).Enumerate(nil)
	})

	targetFiles = collection.Select(targetFiles, func(subject interface{}) interface{} {
		return normalizePath(subject.(string))
	})
}

// goPath returns the value of the environement variable GOPATH at the beginning of this
// program's execution. The actual enviroment variable is only queried once, before any calls
// are made to this function.
var goPath = func() func() string {
	val := normalizePath(os.Getenv("GOPATH"))
	return func() string {
		return val
	}
}()

// getDefaultOutputBase returns the default location of the Azure-SDK-for-Go on your filesystem.
func getDefaultOutputBase() string {
	return normalizePath(path.Join(goPath(), "src", "github.com", "Azure", "azure-sdk-for-go"))
}

// getDefaultOutputBase returns the default location of the Azure-SDK-for-Go on your filesystem relative to the 'GOPATH/src'.
func getRelativeOutputBase() string {
	return normalizePath(path.Join("github.com", "Azure", "azure-sdk-for-go"))
}

// trimGoPath operates like strings.TrimPrefix, where the prefix is always the value of GOPATH in the
// environment in which this program is being executed.
func trimGoPath(location string) string {
	return strings.TrimPrefix(normalizePath(location), goPath())
}

// normalizePath ensures that a path is expressed using forward slashes for sanity when working
// with Go Standard Library functions that seem to expect this.
func normalizePath(location string) (result string) {
	result = strings.Replace(location, `\`, "/", -1)
	return
}

//getLeafOutputFolder returns the leaf of a given path.
func getLeafOutputFolder(location string) string {
	_, leaf := filepath.Split(location)
	return leaf
}

//isStableAPI returns true if for the given package name there are no preview input files.
func isStableAPI(packageName string, packageInputFiles [][]string) bool {
	for _, packageDefinition := range packageInputFiles {
		if !strings.Contains(normalizePath(packageDefinition[2]), "/preview/") && packageName == packageDefinition[1] {
			return true
		}
	}
	return false
}

// isntNil is a simple `collection.Predicate` which filters out `nil` objects from an Enumerator.
func isntNil(subject interface{}) bool {
	return subject != nil
}

// writeListToFile prints a list of strings to the specified filepath.
func writeListToFile(filepath string, list []string) error {
	outputFile, err := os.Create(filepath)
	if err != nil {
		return err
	}
	for _, el := range list {
		fmt.Fprintln(outputFile, el)
	}
	return nil
}

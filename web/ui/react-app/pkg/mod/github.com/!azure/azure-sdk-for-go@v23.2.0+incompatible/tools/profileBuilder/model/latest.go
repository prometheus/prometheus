// +build go1.9

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

package model

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/marstr/collection"
)

// LatestStrategy evaluates the Azure-SDK-for-Go repository for the latest available API versions of each service.
type LatestStrategy struct {
	Root          string
	Predicate     func(packageName string) bool
	VerboseOutput *log.Logger
}

// AcceptAll is a predefined value for `LatestStrategy.Predicate` which always returns true.
func AcceptAll(name string) bool {
	return true
}

// IgnorePreview searches a packages "API Version" to see if it contains the word "preview". It only returns true when a package is not a preview.
func IgnorePreview(name string) (result bool) {
	matches := packageName.FindStringSubmatch(name)
	version := matches[3]
	result = !strings.Contains(version, "-preview") && !strings.Contains(version, "-beta") // matches[2] is the `version` group
	return
}

// Enumerate scans through the known Azure SDK for Go packages and finds each
func (latest LatestStrategy) Enumerate(cancel <-chan struct{}) collection.Enumerator {
	results := make(chan interface{})

	go func() {
		defer close(results)

		type operationGroup struct {
			provider string
			arm      string
			group    string
		}

		type operInfo struct {
			version string
			rawpath string
		}

		if latest.VerboseOutput == nil {
			latest.VerboseOutput = log.New(ioutil.Discard, "", 0)
		}

		maxFound := make(map[operationGroup]operInfo)

		filepath.Walk(latest.Root, func(currentPath string, info os.FileInfo, openErr error) (err error) {
			pathMatches := packageName.FindStringSubmatch(currentPath)

			if latest.Predicate == nil {
				latest.Predicate = AcceptAll
			}

			if len(pathMatches) == 0 || !info.IsDir() {
				return
			} else if !latest.Predicate(currentPath) {
				latest.VerboseOutput.Printf("%q rejected by Predicate", currentPath)
				return
			}

			version := pathMatches[3]
			currentGroup := operationGroup{
				provider: pathMatches[1],
				arm:      pathMatches[2],
				group:    pathMatches[4],
			}

			prev, ok := maxFound[currentGroup]
			if !ok {
				maxFound[currentGroup] = operInfo{version, currentPath}
				latest.VerboseOutput.Printf("New group found %q using version %q", currentGroup, version)
				return
			}

			if le, _ := VersionLE(prev.version, version); le {
				maxFound[currentGroup] = operInfo{version, currentPath}
				latest.VerboseOutput.Printf("Updating group %q from version %q to %q", currentGroup, prev.version, version)
			} else {
				latest.VerboseOutput.Printf("Evaluated group %q version %q decided to stay with %q", currentGroup, version, prev.version)
			}

			return
		})

		for _, entry := range maxFound {
			absolute, err := filepath.Abs(entry.rawpath)
			if err != nil {
				continue
			}

			select {
			case results <- absolute:
				// Intionally Left Blank
			case <-cancel:
				return
			}
		}
	}()

	return results
}

// ErrNotVersionString is instantiated when a string not conforming to known Azure API Version patterns is parsed is if it did.
type ErrNotVersionString string

func (err ErrNotVersionString) Error() string {
	return fmt.Sprintf("`%s` is not a recognized Azure version string", string(err))
}

// VersionLE takes two version strings that share a format and returns true if the one on the
// left is less than or equal to the one on the right. If the two do not match in format, or
// are not in a well known format, this will return false and an error.
var VersionLE = func() func(string, string) (bool, error) {
	wellKnownStrategies := map[*regexp.Regexp]func([]string, []string) (bool, error){
		// The strategy below handles Azure API Versions which have a date optionally followed by some tag.
		regexp.MustCompile(`^(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})(?:[\.\-](?P<tag>.+))?$`): func(leftMatch, rightMatch []string) (bool, error) {
			var err error
			for i := 1; i <= 3; i++ { // Start with index 1 because the element 0 is the entire match, not a group. End at 3 because there are three numeric groups.
				if leftMatch[i] == rightMatch[i] {
					continue
				}

				var leftNum, rightNum int
				leftNum, err = strconv.Atoi(leftMatch[i])
				if err != nil {
					return false, err
				}

				rightNum, err = strconv.Atoi(rightMatch[i])
				if err != nil {
					return false, err
				}

				if leftNum < rightNum {
					return true, nil
				}
				return false, nil
			}

			if leftTag, rightTag := leftMatch[4], rightMatch[4]; leftTag == "" && rightTag != "" { // match[4] is the tag portion of a date based API Version label
				return false, nil
			} else if leftTag != "" && rightTag != "" {
				return leftTag <= rightTag, nil
			}
			return true, nil
		},
		// The strategy below compares two semvers.
		regexp.MustCompile(`(?P<major>\d+)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?-?(?P<tag>.*)`): func(leftMatch, rightMatch []string) (bool, error) {
			for i := 1; i <= 3; i++ {
				if len(leftMatch[i]) == 0 || len(rightMatch[i]) == 0 {
					return leftMatch[i] <= rightMatch[i], nil
				}
				numLeft, err := strconv.Atoi(leftMatch[i])
				if err != nil {
					return false, err
				}
				numRight, err := strconv.Atoi(rightMatch[i])
				if err != nil {
					return false, err
				}

				if numLeft < numRight {
					return true, nil
				}

				if numLeft > numRight {
					return false, nil
				}
			}

			return leftMatch[4] <= rightMatch[4], nil
		},
	}

	// This function finds a strategy which recognizes the versions passed to it, then applies that strategy.
	return func(left, right string) (bool, error) {
		if left == right {
			return true, nil
		}

		for versionStrategy, handler := range wellKnownStrategies {
			if leftMatch, rightMatch := versionStrategy.FindAllStringSubmatch(left, 1), versionStrategy.FindAllStringSubmatch(right, 1); len(leftMatch) > 0 && len(rightMatch) > 0 {
				return handler(leftMatch[0], rightMatch[0])
			}
		}
		return false, fmt.Errorf("Unable to find versioning strategy that could compare %q and %q", left, right)
	}
}()

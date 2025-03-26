// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/mod/semver"
)

func main() {
	// Load the current version as string from the first argument.
	// This is the version that will be tagged as latest.
	if len(os.Args) < 2 {
		panic("Missing current version argument")
	}
	version := os.Args[1]

	if !semver.IsValid(version) {
		panic("Invalid current version format, not a semantic version")
	}

	/* Load the existings tags from the standard input, one per line. */
	existingVersions := []string{}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		v := scanner.Text()
		if !semver.IsValid(v) {
			continue
		}
		if semver.Prerelease(v) != "" {
			// Skip prerelease versions.
			continue
		}
		existingVersions = append(existingVersions, v)
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	if len(existingVersions) == 0 {
		panic("No existing versions found")
	}

	extraTags := getExtraTags(version, existingVersions)
	fmt.Printf("%s\n", strings.Join(extraTags, "\n"))
}

func getExtraTags(version string, existingVersions []string) (extraTags []string) {
	extraTags = []string{}
	if semver.Prerelease(version) != "" {
		// No extra tags for prerelease versions.
		return
	}

	semver.Sort(existingVersions)

	// Check if the current version is the latest version.
	lastVersion := existingVersions[len(existingVersions)-1]
	if semver.Compare(lastVersion, version) <= 0 {
		extraTags = append(extraTags, semver.Major(version))
		extraTags = append(extraTags, semver.MajorMinor(version))
		extraTags = append(extraTags, "latest")
		return
	}

	// Check if it is the latest in the major version.
	matchingMajor := []string{}
	for _, v := range existingVersions {
		if semver.Major(v) == semver.Major(version) {
			// Does not affect ordering.
			matchingMajor = append(matchingMajor, v)
		}
	}
	if len(matchingMajor) == 0 || semver.Compare(matchingMajor[len(matchingMajor)-1], version) <= 0 {
		extraTags = append(extraTags, semver.Major(version))
	}

	// Check if it is the latest in the minor version.
	matchingMinor := []string{}
	for _, v := range matchingMajor {
		if semver.MajorMinor(v) == semver.MajorMinor(version) {
			// Does not affect ordering.
			matchingMinor = append(matchingMinor, v)
		}
	}
	if len(matchingMinor) == 0 || semver.Compare(matchingMinor[len(matchingMinor)-1], version) <= 0 {
		extraTags = append(extraTags, semver.MajorMinor(version))
	}

	return
}

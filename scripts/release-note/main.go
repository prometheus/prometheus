// Copyright 2024 The Prometheus Authors
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
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	gogithub "github.com/google/go-github/v66/github"
)

const (
	ghOrg  = "prometheus"
	ghRepo = "prometheus"

	ghTokenEnvVar = "PROM_GRN_GITHUB_TOKEN"

	generateSubCmd = "generate"
	checkSubCmd    = "check"

	releaseBlockName = "```release-note ```"
	notApplicable    = "NONE"

	perPageLimit = 10000
)

var (
	releaseNoteRegex = regexp.MustCompile("(?s)```release-note\\s+(?U)(.*)\\s*```")

	errPRAlreadySeen       = fmt.Errorf("pull request was already processed")
	errReleaseNoteNotFound = fmt.Errorf("release note couldn't be retrieved. If the release note "+
		"is not applicable, you can put %s in the %s block", notApplicable, releaseBlockName)
)

type releaseNotesBuilder struct {
	ghClient *gogithub.Client

	baseRef string
	headRef string

	seenPrs map[int64]struct{}
}

func NewReleaseNotesBuilder(baseRef, headRef string) *releaseNotesBuilder {
	ghClient := gogithub.NewClient(nil)
	token, set := os.LookupEnv(ghTokenEnvVar)
	if set {
		ghClient = ghClient.WithAuthToken(token)
	}

	return &releaseNotesBuilder{
		ghClient: ghClient,

		baseRef: baseRef,
		headRef: headRef,

		seenPrs: make(map[int64]struct{}),
	}
}

func (builder releaseNotesBuilder) fetchReleaseNote(pr *gogithub.PullRequest) (string, error) {
	defer func() {
		// pr.ID isn't supposed to be nil.
		builder.seenPrs[*pr.ID] = struct{}{}
	}()

	if _, seen := builder.seenPrs[*pr.ID]; seen {
		return "", errPRAlreadySeen
	}
	if !isEmptyString(pr.Body) {
		note, err := extractReleaseNote(*pr.Body)
		if err == nil {
			return note, nil
		}
		if !errors.Is(err, errReleaseNoteNotFound) {
			return "", err
		}
	}

	// Use the PR's title as a fallback.
	if isEmptyString(pr.Title) {
		return "", fmt.Errorf("pull request body and title are empty")
	}
	return fmt.Sprintf("WARNING: PR TITLE AS RELEASE NOTE: %s", *pr.Title), nil
}

// TODO: maybe this will need to be grouped.
func (builder releaseNotesBuilder) printReleaseNotesDraft(compare *gogithub.CommitsComparison) {
	for _, commit := range compare.Commits {
		prs, _, err := builder.ghClient.PullRequests.ListPullRequestsWithCommit(
			context.Background(),
			ghOrg,
			ghRepo,
			*commit.SHA,
			nil,
		)
		if err != nil {
			log.Fatalf("error getting the commits between %s and %s: %s", builder.baseRef, builder.headRef, err.Error())
		}
		for _, pr := range prs {
			releaseNote, err := builder.fetchReleaseNote(pr)
			if err != nil {
				if errors.Is(err, errPRAlreadySeen) {
					continue
				}
				fmt.Printf("- ERROR: COULDN'T GET RELEASE NOTE: %s (%s).\n", err.Error(), *pr.HTMLURL)
				continue
			}
			if releaseNote == notApplicable {
				continue
			}
			fmt.Printf("- %s (%s).\n", releaseNote, *pr.HTMLURL)
		}
	}
	fmt.Printf("\nFull Changelog: %s\n", *compare.HTMLURL)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Available subcommands are: %s, %s.\n", generateSubCmd, checkSubCmd)
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		log.Fatal("Please specify a subcommand.")
	}
	cmd, args := args[0], args[1:]
	switch cmd {
	case checkSubCmd:
		check(args)
	case generateSubCmd:
		generate(args)
	default:
		log.Fatalf("Unrecognized subcommand: %q.", cmd)
	}
}

func check(args []string) {
	flag := flag.NewFlagSet("check", flag.ExitOnError)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "The command reads input from stdin (typically a GitHub pull request body) and "+
			"extracts the note from the %s block. Fails if the release note could not be extracted.\n", releaseBlockName)
	}

	flag.Parse(args)
	args = flag.Args()
	if len(args) != 0 {
		flag.Usage()

		log.Fatalf("Unknown args: %q", args)
	}

	var sb strings.Builder
	buf := bufio.NewScanner(os.Stdin)
	for buf.Scan() {
		sb.WriteString(buf.Text())
		sb.WriteString("\n")
	}

	if err := buf.Err(); err != nil {
		log.Fatalf("Error reading input: %s", err)
	}
	note, err := extractReleaseNote(sb.String())
	if err != nil {
		log.Fatal(err.Error())
	}
	if note == notApplicable {
		fmt.Fprint(os.Stderr, "Release note is not applicable.\n")
		return
	}
	fmt.Fprintf(os.Stderr, "Release note was extracted:\n%q\n", note)
}

func generate(args []string) {
	flag := flag.NewFlagSet("generate", flag.ExitOnError)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "The command fetches release note entries from pull requests "+
			"associated with all commits between two references and assembles them into a draft "+
			"release notes printed to stdout.\nUses %s env var for GitHub API requests if set.\n", ghTokenEnvVar)
		flag.PrintDefaults()
	}
	baseRef := flag.String("base", "", "base reference")
	headRef := flag.String("head", "", "head reference")

	flag.Parse(args)
	args = flag.Args()
	if len(args) != 0 {
		flag.Usage()
		log.Fatalf("Unknown args: %q", args)
	}

	if isEmptyString(baseRef) {
		flag.Usage()
		log.Fatalf("base reference not set")
	}
	if isEmptyString(headRef) {
		flag.Usage()
		log.Fatalf("head reference not set")
	}

	builder := NewReleaseNotesBuilder(*baseRef, *headRef)
	compare, _, err := builder.ghClient.Repositories.CompareCommits(
		context.Background(),
		ghOrg,
		ghRepo,
		builder.baseRef,
		builder.headRef,
		&gogithub.ListOptions{
			PerPage: perPageLimit,
		},
	)
	if err != nil {
		log.Fatalf("error getting the commits between %s and %s: %s", builder.baseRef, builder.headRef, err.Error())
	}
	if len(compare.Commits) == perPageLimit {
		log.Fatal("commits could be missed, pagination should be set up.")
	}

	builder.printReleaseNotesDraft(compare)
}

func isEmptyString(s *string) bool {
	return s == nil || *s == ""
}

func extractReleaseNote(s string) (string, error) {
	match := releaseNoteRegex.FindStringSubmatch(s)
	if len(match) != 2 {
		return "", fmt.Errorf("%s block not found: %w", releaseBlockName, errReleaseNoteNotFound)
	}
	note := match[1]
	switch {
	case note == "":
		return "", fmt.Errorf("release note is empty: %w", errReleaseNoteNotFound)
	case note == notApplicable:
		return notApplicable, nil
	case strings.Contains(note, "\n"):
		return "", fmt.Errorf("%s block should contain one line: %q", releaseBlockName, note)
	}
	return note, nil
}

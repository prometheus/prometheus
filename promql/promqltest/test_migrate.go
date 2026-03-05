// Copyright The Prometheus Authors
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

package promqltest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/grafana/regexp"
)

const defaultTestDataDir = "promql/promqltest/testdata"

var (
	evalRegex   = regexp.MustCompile(`^(eval |eval_fail |eval_warn |eval_info |eval_ordered )(.*)$`)
	indentRegex = regexp.MustCompile(`^([ \t]+)\S`)
)

type MigrateMode int

const (
	MigrateStrict MigrateMode = iota
	MigrateBasic
	MigrateTolerant
)

func ParseMigrateMode(s string) (MigrateMode, error) {
	switch s {
	case "strict":
		return MigrateStrict, nil
	case "basic":
		return MigrateBasic, nil
	case "tolerant":
		return MigrateTolerant, nil
	default:
		return MigrateStrict, fmt.Errorf("invalid mode: %s", s)
	}
}

// MigrateTestData migrates all PromQL test files to the new syntax format.
// It applies annotation rules based on the provided migration mode ("strict", "basic", or "tolerant").
// The function parses each .test file, converts it to the new syntax and overwrites the file.
func MigrateTestData(mode, dir string) error {
	if dir == "" {
		dir = defaultTestDataDir
	}

	migrationMode, err := ParseMigrateMode(mode)
	if err != nil {
		return fmt.Errorf("failed to parse mode: %w", err)
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read testdata directory: %w", err)
	}

	annotationMap := map[MigrateMode]map[string][]string{
		MigrateStrict: {
			"eval_fail":    {"expect fail", "expect no_warn", "expect no_info"},
			"eval_warn":    {"expect warn", "expect no_info"},
			"eval_info":    {"expect info", "expect no_warn"},
			"eval_ordered": {"expect ordered", "expect no_warn", "expect no_info"},
			"eval":         {"expect no_warn", "expect no_info"},
		},
		MigrateBasic: {
			"eval_fail":    {"expect fail"},
			"eval_warn":    {"expect warn"},
			"eval_info":    {"expect info"},
			"eval_ordered": {"expect ordered"},
		},
		MigrateTolerant: {
			"eval_fail":    {"expect fail"},
			"eval_ordered": {"expect ordered"},
		},
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".test") {
			continue
		}

		path := filepath.Join(dir, file.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", path, err)
		}

		lines := strings.Split(string(content), "\n")
		processedLines, err := processTestFileLines(lines, annotationMap[migrationMode], evalRegex)
		if err != nil {
			return fmt.Errorf("error processing file %s: %w", path, err)
		}

		if err := os.WriteFile(path, []byte(strings.Join(processedLines, "\n")), 0o644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", path, err)
		}
	}
	return nil
}

func processTestFileLines(
	lines []string,
	annotationMap map[string][]string,
	evalRegex *regexp.Regexp,
) (result []string, err error) {
	for i := 0; i < len(lines); i++ {
		startLine := lines[i]
		matches := evalRegex.FindStringSubmatch(strings.TrimSpace(startLine))
		if matches == nil {
			result = append(result, startLine)
			continue
		}

		var inputBlock []string
		var outputBlock []string
		skipBlock := false
		i++
		for i < len(lines) {
			inputBlock = append(inputBlock, lines[i])
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "expect ") {
				skipBlock = true
			}
			if i+1 < len(lines) && evalRegex.MatchString(strings.TrimSpace(lines[i+1])) {
				break
			}
			i++
		}

		if skipBlock {
			result = append(result, startLine)
			result = append(result, inputBlock...)
			continue
		}

		// Get leading whitespace from startLine using indentRegex.
		leadingWS := ""
		if indentMatch := indentRegex.FindStringSubmatch(startLine); indentMatch != nil {
			leadingWS = indentMatch[1]
		}

		command := strings.TrimSpace(matches[1])
		expression := matches[2]
		var annotations []string
		result = append(result, leadingWS+fmt.Sprintf("eval %s", expression))

		// Detecting indentation style (tab or space) from the first non-empty, indented line.
		indent := "  "
		for _, line := range inputBlock {
			if indentMatch := indentRegex.FindStringSubmatch(line); indentMatch != nil {
				indent = indentMatch[1]
				break
			}
		}

		for _, annotation := range annotationMap[command] {
			annotations = append(annotations, indent+annotation)
		}

		for _, line := range inputBlock {
			trimmedLine := strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(trimmedLine, "expected_fail_message"):
				msg := strings.TrimPrefix(trimmedLine, "expected_fail_message ")
				for j, s := range annotations {
					if strings.Contains(s, "expect fail") {
						annotations[j] = indent + fmt.Sprintf("expect fail msg:%s", msg)
					}
				}
			case strings.HasPrefix(trimmedLine, "expected_fail_regexp"):
				regex := strings.TrimPrefix(trimmedLine, "expected_fail_regexp ")
				for j, s := range annotations {
					if strings.Contains(s, "expect fail") {
						annotations[j] = indent + fmt.Sprintf("expect fail regex:%s", regex)
					}
				}
			default:
				outputBlock = append(outputBlock, line)
			}
		}

		result = append(result, annotations...)
		result = append(result, outputBlock...)
	}

	return result, nil
}

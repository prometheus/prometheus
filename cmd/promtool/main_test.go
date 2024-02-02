// Copyright 2018 The Prometheus Authors
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
)

var promtoolPath = os.Args[0]

func TestMain(m *testing.M) {
	for i, arg := range os.Args {
		if arg == "-test.main" {
			os.Args = append(os.Args[:i], os.Args[i+1:]...)
			main()
			return
		}
	}

	exitCode := m.Run()
	os.Exit(exitCode)
}

func TestQueryRange(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "matrix", "result": []}}`)
	defer s.Close()

	urlObject, err := url.Parse(s.URL)
	require.NoError(t, err)

	p := &promqlPrinter{}
	exitCode := QueryRange(urlObject, http.DefaultTransport, map[string]string{}, "up", "0", "300", 0, p)
	require.Equal(t, "/api/v1/query_range", getRequest().URL.Path)
	form := getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "1", form.Get("step"))
	require.Equal(t, 0, exitCode)

	exitCode = QueryRange(urlObject, http.DefaultTransport, map[string]string{}, "up", "0", "300", 10*time.Millisecond, p)
	require.Equal(t, "/api/v1/query_range", getRequest().URL.Path)
	form = getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "0.01", form.Get("step"))
	require.Equal(t, 0, exitCode)
}

func TestQueryInstant(t *testing.T) {
	s, getRequest := mockServer(200, `{"status": "success", "data": {"resultType": "vector", "result": []}}`)
	defer s.Close()

	urlObject, err := url.Parse(s.URL)
	require.NoError(t, err)

	p := &promqlPrinter{}
	exitCode := QueryInstant(urlObject, http.DefaultTransport, "up", "300", p)
	require.Equal(t, "/api/v1/query", getRequest().URL.Path)
	form := getRequest().Form
	require.Equal(t, "up", form.Get("query"))
	require.Equal(t, "300", form.Get("time"))
	require.Equal(t, 0, exitCode)
}

func mockServer(code int, body string) (*httptest.Server, func() *http.Request) {
	var req *http.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		req = r
		w.WriteHeader(code)
		fmt.Fprintln(w, body)
	}))

	f := func() *http.Request {
		return req
	}
	return server, f
}

func TestCheckSDFile(t *testing.T) {
	cases := []struct {
		name string
		file string
		err  string
	}{
		{
			name: "good .yml",
			file: "./testdata/good-sd-file.yml",
		},
		{
			name: "good .yaml",
			file: "./testdata/good-sd-file.yaml",
		},
		{
			name: "good .json",
			file: "./testdata/good-sd-file.json",
		},
		{
			name: "bad file extension",
			file: "./testdata/bad-sd-file-extension.nonexistant",
			err:  "invalid file extension: \".nonexistant\"",
		},
		{
			name: "bad format",
			file: "./testdata/bad-sd-file-format.yml",
			err:  "yaml: unmarshal errors:\n  line 1: field targats not found in type struct { Targets []string \"yaml:\\\"targets\\\"\"; Labels model.LabelSet \"yaml:\\\"labels\\\"\" }",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkSDFile(test.file)
			if test.err != "" {
				require.Equalf(t, test.err, err.Error(), "Expected error %q, got %q", test.err, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckDuplicates(t *testing.T) {
	cases := []struct {
		name         string
		ruleFile     string
		expectedDups []compareRuleType
	}{
		{
			name:     "no duplicates",
			ruleFile: "./testdata/rules.yml",
		},
		{
			name:     "duplicate in other group",
			ruleFile: "./testdata/rules_duplicates.yml",
			expectedDups: []compareRuleType{
				{
					metric: "job:test:count_over_time1m",
					label:  labels.New(),
				},
			},
		},
	}

	for _, test := range cases {
		c := test
		t.Run(c.name, func(t *testing.T) {
			rgs, err := rulefmt.ParseFile(c.ruleFile)
			require.Empty(t, err)
			dups := checkDuplicates(rgs.Groups)
			require.Equal(t, c.expectedDups, dups)
		})
	}
}

func BenchmarkCheckDuplicates(b *testing.B) {
	rgs, err := rulefmt.ParseFile("./testdata/rules_large.yml")
	require.Empty(b, err)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		checkDuplicates(rgs.Groups)
	}
}

func TestCheckTargetConfig(t *testing.T) {
	cases := []struct {
		name string
		file string
		err  string
	}{
		{
			name: "url_in_scrape_targetgroup_with_relabel_config.good",
			file: "url_in_scrape_targetgroup_with_relabel_config.good.yml",
			err:  "",
		},
		{
			name: "url_in_alert_targetgroup_with_relabel_config.good",
			file: "url_in_alert_targetgroup_with_relabel_config.good.yml",
			err:  "",
		},
		{
			name: "url_in_scrape_targetgroup_with_relabel_config.bad",
			file: "url_in_scrape_targetgroup_with_relabel_config.bad.yml",
			err:  "instance 0 in group 0: \"http://bad\" is not a valid hostname",
		},
		{
			name: "url_in_alert_targetgroup_with_relabel_config.bad",
			file: "url_in_alert_targetgroup_with_relabel_config.bad.yml",
			err:  "\"http://bad\" is not a valid hostname",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkConfig(false, "testdata/"+test.file, false)
			if test.err != "" {
				require.Equalf(t, test.err, err.Error(), "Expected error %q, got %q", test.err, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckConfigSyntax(t *testing.T) {
	cases := []struct {
		name       string
		file       string
		syntaxOnly bool
		err        string
		errWindows string
	}{
		{
			name:       "check with syntax only succeeds with nonexistent rule files",
			file:       "config_with_rule_files.yml",
			syntaxOnly: true,
			err:        "",
			errWindows: "",
		},
		{
			name:       "check without syntax only fails with nonexistent rule files",
			file:       "config_with_rule_files.yml",
			syntaxOnly: false,
			err:        "\"testdata/non-existent-file.yml\" does not point to an existing file",
			errWindows: "\"testdata\\\\non-existent-file.yml\" does not point to an existing file",
		},
		{
			name:       "check with syntax only succeeds with nonexistent service discovery files",
			file:       "config_with_service_discovery_files.yml",
			syntaxOnly: true,
			err:        "",
			errWindows: "",
		},
		// The test below doesn't fail because the file verification for ServiceDiscoveryConfigs doesn't fail the check if
		// file isn't found; it only outputs a warning message.
		{
			name:       "check without syntax only succeeds with nonexistent service discovery files",
			file:       "config_with_service_discovery_files.yml",
			syntaxOnly: false,
			err:        "",
			errWindows: "",
		},
		{
			name:       "check with syntax only succeeds with nonexistent TLS files",
			file:       "config_with_tls_files.yml",
			syntaxOnly: true,
			err:        "",
			errWindows: "",
		},
		{
			name:       "check without syntax only fails with nonexistent TLS files",
			file:       "config_with_tls_files.yml",
			syntaxOnly: false,
			err: "error checking client cert file \"testdata/nonexistent_cert_file.yml\": " +
				"stat testdata/nonexistent_cert_file.yml: no such file or directory",
			errWindows: "error checking client cert file \"testdata\\\\nonexistent_cert_file.yml\": " +
				"CreateFile testdata\\nonexistent_cert_file.yml: The system cannot find the file specified.",
		},
		{
			name:       "check with syntax only succeeds with nonexistent credentials file",
			file:       "authorization_credentials_file.bad.yml",
			syntaxOnly: true,
			err:        "",
			errWindows: "",
		},
		{
			name:       "check without syntax only fails with nonexistent credentials file",
			file:       "authorization_credentials_file.bad.yml",
			syntaxOnly: false,
			err: "error checking authorization credentials or bearer token file \"/random/file/which/does/not/exist.yml\": " +
				"stat /random/file/which/does/not/exist.yml: no such file or directory",
			errWindows: "error checking authorization credentials or bearer token file \"testdata\\\\random\\\\file\\\\which\\\\does\\\\not\\\\exist.yml\": " +
				"CreateFile testdata\\random\\file\\which\\does\\not\\exist.yml: The system cannot find the path specified.",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkConfig(false, "testdata/"+test.file, test.syntaxOnly)
			expectedErrMsg := test.err
			if strings.Contains(runtime.GOOS, "windows") {
				expectedErrMsg = test.errWindows
			}
			if expectedErrMsg != "" {
				require.Equalf(t, expectedErrMsg, err.Error(), "Expected error %q, got %q", test.err, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAuthorizationConfig(t *testing.T) {
	cases := []struct {
		name string
		file string
		err  string
	}{
		{
			name: "authorization_credentials_file.bad",
			file: "authorization_credentials_file.bad.yml",
			err:  "error checking authorization credentials or bearer token file",
		},
		{
			name: "authorization_credentials_file.good",
			file: "authorization_credentials_file.good.yml",
			err:  "",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := checkConfig(false, "testdata/"+test.file, false)
			if test.err != "" {
				require.Contains(t, err.Error(), test.err, "Expected error to contain %q, got %q", test.err, err.Error())
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCheckMetricsExtended(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	f, err := os.Open("testdata/metrics-test.prom")
	require.NoError(t, err)
	defer f.Close()

	stats, total, err := checkMetricsExtended(f)
	require.NoError(t, err)
	require.Equal(t, 27, total)
	require.Equal(t, []metricStat{
		{
			name:        "prometheus_tsdb_compaction_chunk_size_bytes",
			cardinality: 15,
			percentage:  float64(15) / float64(27),
		},
		{
			name:        "go_gc_duration_seconds",
			cardinality: 7,
			percentage:  float64(7) / float64(27),
		},
		{
			name:        "net_conntrack_dialer_conn_attempted_total",
			cardinality: 4,
			percentage:  float64(4) / float64(27),
		},
		{
			name:        "go_info",
			cardinality: 1,
			percentage:  float64(1) / float64(27),
		},
	}, stats)
}

func TestExitCodes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	for _, c := range []struct {
		file      string
		exitCode  int
		lintIssue bool
	}{
		{
			file: "prometheus-config.good.yml",
		},
		{
			file:     "prometheus-config.bad.yml",
			exitCode: 1,
		},
		{
			file:     "prometheus-config.nonexistent.yml",
			exitCode: 1,
		},
		{
			file:      "prometheus-config.lint.yml",
			lintIssue: true,
			exitCode:  3,
		},
	} {
		t.Run(c.file, func(t *testing.T) {
			for _, lintFatal := range []bool{true, false} {
				t.Run(fmt.Sprintf("%t", lintFatal), func(t *testing.T) {
					args := []string{"-test.main", "check", "config", "testdata/" + c.file}
					if lintFatal {
						args = append(args, "--lint-fatal")
					}
					tool := exec.Command(promtoolPath, args...)
					err := tool.Run()
					if c.exitCode == 0 || (c.lintIssue && !lintFatal) {
						require.NoError(t, err)
						return
					}

					require.Error(t, err)

					var exitError *exec.ExitError
					if errors.As(err, &exitError) {
						status := exitError.Sys().(syscall.WaitStatus)
						require.Equal(t, c.exitCode, status.ExitStatus())
					} else {
						t.Errorf("unable to retrieve the exit status for promtool: %v", err)
					}
				})
			}
		})
	}
}

func TestDocumentation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, promtoolPath, "-test.main", "write-documentation")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() != 0 {
			fmt.Println("Command failed with non-zero exit code")
		}
	}

	generatedContent := strings.ReplaceAll(stdout.String(), filepath.Base(promtoolPath), strings.TrimSuffix(filepath.Base(promtoolPath), ".test"))

	expectedContent, err := os.ReadFile(filepath.Join("..", "..", "docs", "command-line", "promtool.md"))
	require.NoError(t, err)

	require.Equal(t, string(expectedContent), generatedContent, "Generated content does not match documentation. Hint: run `make cli-documentation`.")
}

func TestCheckRules(t *testing.T) {
	t.Run("rules-good", func(t *testing.T) {
		data, err := os.ReadFile("./testdata/rules.yml")
		require.NoError(t, err)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}

		_, err = w.Write(data)
		if err != nil {
			t.Error(err)
		}
		w.Close()

		// Restore stdin right after the test.
		defer func(v *os.File) { os.Stdin = v }(os.Stdin)
		os.Stdin = r

		exitCode := CheckRules(newLintConfig(lintOptionDuplicateRules, false))
		require.Equal(t, successExitCode, exitCode, "")
	})

	t.Run("rules-bad", func(t *testing.T) {
		data, err := os.ReadFile("./testdata/rules-bad.yml")
		require.NoError(t, err)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}

		_, err = w.Write(data)
		if err != nil {
			t.Error(err)
		}
		w.Close()

		// Restore stdin right after the test.
		defer func(v *os.File) { os.Stdin = v }(os.Stdin)
		os.Stdin = r

		exitCode := CheckRules(newLintConfig(lintOptionDuplicateRules, false))
		require.Equal(t, failureExitCode, exitCode, "")
	})

	t.Run("rules-lint-fatal", func(t *testing.T) {
		data, err := os.ReadFile("./testdata/prometheus-rules.lint.yml")
		require.NoError(t, err)
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}

		_, err = w.Write(data)
		if err != nil {
			t.Error(err)
		}
		w.Close()

		// Restore stdin right after the test.
		defer func(v *os.File) { os.Stdin = v }(os.Stdin)
		os.Stdin = r

		exitCode := CheckRules(newLintConfig(lintOptionDuplicateRules, true))
		require.Equal(t, lintErrExitCode, exitCode, "")
	})
}

func TestCheckRulesWithRuleFiles(t *testing.T) {
	t.Run("rules-good", func(t *testing.T) {
		exitCode := CheckRules(newLintConfig(lintOptionDuplicateRules, false), "./testdata/rules.yml")
		require.Equal(t, successExitCode, exitCode, "")
	})

	t.Run("rules-bad", func(t *testing.T) {
		exitCode := CheckRules(newLintConfig(lintOptionDuplicateRules, false), "./testdata/rules-bad.yml")
		require.Equal(t, failureExitCode, exitCode, "")
	})

	t.Run("rules-lint-fatal", func(t *testing.T) {
		exitCode := CheckRules(newLintConfig(lintOptionDuplicateRules, true), "./testdata/prometheus-rules.lint.yml")
		require.Equal(t, lintErrExitCode, exitCode, "")
	})
}

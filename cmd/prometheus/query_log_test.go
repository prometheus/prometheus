// Copyright 2020 The Prometheus Authors
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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/util/testutil"
)

type origin int

const (
	apiOrigin origin = iota
	consoleOrigin
	ruleOrigin
)

// queryLogTest defines a query log test.
type queryLogTest struct {
	origin         origin   // Kind of queries tested: api, console, rules.
	prefix         string   // Set as --web.route-prefix.
	host           string   // Used in --web.listen-address. Used with 127.0.0.1 and ::1.
	port           int      // Used in --web.listen-address.
	cwd            string   // Directory where the test is running. Required to find the rules in testdata.
	configFile     *os.File // The configuration file.
	enabledAtStart bool     // Whether query log is enabled at startup.
}

// skip checks if the test is needed and the prerequisites are met.
func (p *queryLogTest) skip(t *testing.T) {
	if p.prefix != "" && p.origin == ruleOrigin {
		t.Skip("changing prefix has no effect on rules")
	}
	// Some systems don't support IPv4 or IPv6.
	l, err := net.Listen("tcp", fmt.Sprintf("%s:0", p.host))
	if err != nil {
		t.Skip("ip version not supported")
	}
	l.Close()
}

// waitForPrometheus waits for Prometheus to be ready.
func (p *queryLogTest) waitForPrometheus() error {
	var err error
	for x := 0; x < 20; x++ {
		var r *http.Response
		if r, err = http.Get(fmt.Sprintf("http://%s:%d%s/-/ready", p.host, p.port, p.prefix)); err == nil && r.StatusCode == 200 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return err
}

// setQueryLog alters the configuration file to enable or disable the query log,
// then reloads the configuration if needed.
func (p *queryLogTest) setQueryLog(t *testing.T, queryLogFile string) {
	err := p.configFile.Truncate(0)
	testutil.Ok(t, err)
	_, err = p.configFile.Seek(0, 0)
	testutil.Ok(t, err)
	if queryLogFile != "" {
		_, err = p.configFile.Write([]byte(fmt.Sprintf("global:\n  query_log_file: %s\n", queryLogFile)))
		testutil.Ok(t, err)
	}
	_, err = p.configFile.Write([]byte(p.configuration()))
	testutil.Ok(t, err)
}

// reloadConfig reloads the configuration using POST.
func (p *queryLogTest) reloadConfig(t *testing.T) {
	r, err := http.Post(fmt.Sprintf("http://%s:%d%s/-/reload", p.host, p.port, p.prefix), "text/plain", nil)
	testutil.Ok(t, err)
	testutil.Equals(t, 200, r.StatusCode)
}

// query runs a query according to the test origin.
func (p *queryLogTest) query(t *testing.T) {
	switch p.origin {
	case apiOrigin:
		r, err := http.Get(fmt.Sprintf(
			"http://%s:%d%s/api/v1/query_range?step=5&start=0&end=3600&query=%s",
			p.host,
			p.port,
			p.prefix,
			url.QueryEscape("query_with_api"),
		))
		testutil.Ok(t, err)
		testutil.Equals(t, 200, r.StatusCode)
	case consoleOrigin:
		r, err := http.Get(fmt.Sprintf(
			"http://%s:%d%s/consoles/test.html",
			p.host,
			p.port,
			p.prefix,
		))
		testutil.Ok(t, err)
		testutil.Equals(t, 200, r.StatusCode)
	case ruleOrigin:
		time.Sleep(2 * time.Second)
	default:
		panic("can't query this origin")
	}
}

// queryString returns the expected queryString of a this test.
func (p *queryLogTest) queryString() string {
	switch p.origin {
	case apiOrigin:
		return "query_with_api"
	case ruleOrigin:
		return "query_in_rule"
	case consoleOrigin:
		return "query_in_console"
	default:
		panic("unknown origin")
	}
}

// validateLastQuery checks that the last query in the query log matches the
// test parameters.
func (p *queryLogTest) validateLastQuery(t *testing.T, ql []queryLogLine) {
	q := ql[len(ql)-1]
	testutil.Equals(t, p.queryString(), q.Params.Query)

	switch p.origin {
	case apiOrigin:
		testutil.Equals(t, 5, q.Params.Step)
		testutil.Equals(t, "1970-01-01T00:00:00.000Z", q.Params.Start)
		testutil.Equals(t, "1970-01-01T01:00:00.000Z", q.Params.End)
	default:
		testutil.Equals(t, 0, q.Params.Step)
	}

	if p.origin != ruleOrigin {
		host := p.host
		if host == "[::1]" {
			host = "::1"
		}
		testutil.Equals(t, host, q.Request.ClientIP)
	}

	switch p.origin {
	case apiOrigin:
		testutil.Equals(t, p.prefix+"/api/v1/query_range", q.Request.Path)
	case consoleOrigin:
		testutil.Equals(t, p.prefix+"/consoles/test.html", q.Request.Path)
	case ruleOrigin:
		testutil.Equals(t, "querylogtest", q.RuleGroup.Name)
		testutil.Equals(t, filepath.Join(p.cwd, "testdata", "rules", "test.yml"), q.RuleGroup.File)
	default:
		panic("unknown origin")
	}
}

func (p *queryLogTest) String() string {
	var name string
	switch p.origin {
	case apiOrigin:
		name = "api queries"
	case consoleOrigin:
		name = "console queries"
	case ruleOrigin:
		name = "rule queries"
	}
	name = name + ", " + p.host + ":" + strconv.Itoa(p.port)
	if p.enabledAtStart {
		name = name + ", enabled at start"
	}
	if p.prefix != "" {
		name = name + ", with prefix " + p.prefix
	}
	return name
}

// params returns the specific command line parameters of this test.
func (p *queryLogTest) params() []string {
	s := []string{}
	if p.prefix != "" {
		s = append(s, "--web.route-prefix="+p.prefix)
	}
	if p.origin == consoleOrigin {
		s = append(s, "--web.console.templates="+filepath.Join("testdata", "consoles"))
	}
	return s
}

// configuration returns the specific configuration lines required for this
// test.
func (p *queryLogTest) configuration() string {
	switch p.origin {
	case ruleOrigin:
		return "\nrule_files:\n- " + filepath.Join(p.cwd, "testdata", "rules", "test.yml") + "\n"
	default:
		return "\n"
	}
}

// exactQueryCount returns whether we can match an exact query count. False on
// recording rules are they are regular time intervals.
func (p *queryLogTest) exactQueryCount() bool {
	return p.origin != ruleOrigin
}

// run launches the scenario of this query log test.
func (p *queryLogTest) run(t *testing.T) {
	p.skip(t)

	// Setup temporary files for this test.
	queryLogFile, err := ioutil.TempFile("", "query")
	testutil.Ok(t, err)
	defer os.Remove(queryLogFile.Name())
	p.configFile, err = ioutil.TempFile("", "config")
	testutil.Ok(t, err)
	defer os.Remove(p.configFile.Name())

	if p.enabledAtStart {
		p.setQueryLog(t, queryLogFile.Name())
	} else {
		p.setQueryLog(t, "")
	}

	dir, err := ioutil.TempDir("", "query_log_test")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	params := append([]string{
		"-test.main",
		"--config.file=" + p.configFile.Name(),
		"--web.enable-lifecycle",
		fmt.Sprintf("--web.listen-address=%s:%d", p.host, p.port),
		"--storage.tsdb.path=" + dir,
	}, p.params()...)

	prom := exec.Command(promPath, params...)

	// Log stderr in case of failure.
	stderr, err := prom.StderrPipe()
	testutil.Ok(t, err)

	// We use a WaitGroup to avoid calling t.Log after the test is done.
	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	go func() {
		slurp, _ := ioutil.ReadAll(stderr)
		t.Log(string(slurp))
		wg.Done()
	}()

	testutil.Ok(t, prom.Start())

	defer func() {
		prom.Process.Kill()
		prom.Wait()
	}()
	testutil.Ok(t, p.waitForPrometheus())

	if !p.enabledAtStart {
		p.query(t)
		testutil.Equals(t, 0, len(readQueryLog(t, queryLogFile.Name())))
		p.setQueryLog(t, queryLogFile.Name())
		p.reloadConfig(t)
	}

	p.query(t)

	ql := readQueryLog(t, queryLogFile.Name())
	qc := len(ql)
	if p.exactQueryCount() {
		testutil.Equals(t, 1, qc)
	} else {
		testutil.Assert(t, qc > 0, "no queries logged")
	}
	p.validateLastQuery(t, ql)

	p.setQueryLog(t, "")
	p.reloadConfig(t)
	if !p.exactQueryCount() {
		qc = len(readQueryLog(t, queryLogFile.Name()))
	}

	p.query(t)

	ql = readQueryLog(t, queryLogFile.Name())
	testutil.Equals(t, qc, len(ql))

	qc = len(ql)
	p.setQueryLog(t, queryLogFile.Name())
	p.reloadConfig(t)

	p.query(t)
	qc++

	ql = readQueryLog(t, queryLogFile.Name())
	if p.exactQueryCount() {
		testutil.Equals(t, qc, len(ql))
	} else {
		testutil.Assert(t, len(ql) > qc, "no queries logged")
	}
	p.validateLastQuery(t, ql)
	qc = len(ql)

	// The last part of the test can not succeed on Windows because you can't
	// rename files used by other processes.
	if runtime.GOOS == "windows" {
		return
	}
	// Move the file, Prometheus should still write to the old file.
	newFile, err := ioutil.TempFile("", "newLoc")
	testutil.Ok(t, err)
	testutil.Ok(t, newFile.Close())
	defer os.Remove(newFile.Name())
	testutil.Ok(t, os.Rename(queryLogFile.Name(), newFile.Name()))
	ql = readQueryLog(t, newFile.Name())
	if p.exactQueryCount() {
		testutil.Equals(t, qc, len(ql))
	}
	p.validateLastQuery(t, ql)
	qc = len(ql)

	p.query(t)

	qc++

	ql = readQueryLog(t, newFile.Name())
	if p.exactQueryCount() {
		testutil.Equals(t, qc, len(ql))
	} else {
		testutil.Assert(t, len(ql) > qc, "no queries logged")
	}
	p.validateLastQuery(t, ql)

	p.reloadConfig(t)

	p.query(t)

	ql = readQueryLog(t, queryLogFile.Name())
	qc = len(ql)
	if p.exactQueryCount() {
		testutil.Equals(t, 1, qc)
	} else {
		testutil.Assert(t, qc > 0, "no queries logged")
	}
}

type queryLogLine struct {
	Params struct {
		Query string `json:"query"`
		Step  int    `json:"step"`
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"params"`
	Request struct {
		Path     string `json:"path"`
		ClientIP string `json:"clientIP"`
	} `json:"httpRequest"`
	RuleGroup struct {
		File string `json:"file"`
		Name string `json:"name"`
	} `json:"ruleGroup"`
}

// readQueryLog unmarshal a json-formatted query log into query log lines.
func readQueryLog(t *testing.T, path string) []queryLogLine {
	ql := []queryLogLine{}
	file, err := os.Open(path)
	testutil.Ok(t, err)
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var q queryLogLine
		testutil.Ok(t, json.Unmarshal(scanner.Bytes(), &q))
		ql = append(ql, q)
	}
	return ql
}

func TestQueryLog(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	cwd, err := os.Getwd()
	testutil.Ok(t, err)

	port := 15000
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		for _, prefix := range []string{"", "/foobar"} {
			for _, enabledAtStart := range []bool{true, false} {
				for _, origin := range []origin{apiOrigin, consoleOrigin, ruleOrigin} {
					p := &queryLogTest{
						origin:         origin,
						host:           host,
						enabledAtStart: enabledAtStart,
						prefix:         prefix,
						port:           port,
						cwd:            cwd,
					}

					t.Run(p.String(), func(t *testing.T) {
						p.run(t)
					})
				}
			}
		}
	}
}

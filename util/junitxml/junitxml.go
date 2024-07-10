package junitxml

import (
	"encoding/xml"
	"io"
)

type JUnitXML struct {
	XMLName xml.Name     `xml:"testsuites"`
	Suites  []*TestSuite `xml:"testsuite"`
}

type TestSuite struct {
	Name         string      `xml:"name,attr"`
	TestCount    int         `xml:"tests,attr"`
	FailureCount int         `xml:"failures,attr"`
	ErrorCount   int         `xml:"errors,attr"`
	SkippedCount int         `xml:"skipped,attr"`
	Timestamp    string      `xml:"timestamp,attr"`
	Cases        []*TestCase `xml:"testcase"`
}
type TestCase struct {
	Name     string   `xml:"name,attr"`
	Failures []string `xml:"failure,omitempty"`
	Error    string   `xml:"error,omitempty"`
}

func (j *JUnitXML) WriteXML(h io.Writer) error {
	return xml.NewEncoder(h).Encode(j)
}

func (j *JUnitXML) Suite(name string) *TestSuite {
	ts := &TestSuite{Name: name}
	j.Suites = append(j.Suites, ts)
	return ts
}
func (ts *TestSuite) Fail(f string) {
	ts.FailureCount++
	curt := ts.lastCase()
	curt.Failures = append(curt.Failures, f)
}
func (ts *TestSuite) lastCase() *TestCase {
	if len(ts.Cases) == 0 {
		ts.Case("unknown")
	}
	return ts.Cases[len(ts.Cases)-1]
}
func (ts *TestSuite) Case(name string) *TestSuite {
	j := &TestCase{
		Name: name,
	}
	ts.Cases = append(ts.Cases, j)
	ts.TestCount++
	return ts

}

func (ts *TestSuite) Settime(name string) {
	ts.Timestamp = name
}

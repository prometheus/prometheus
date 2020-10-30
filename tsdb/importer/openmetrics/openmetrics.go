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

package openmetrics

import (
	"bufio"
	"io"

	"github.com/prometheus/prometheus/pkg/textparse"
)

// Content Type for the Open Metrics Parser.
const contentType = "application/openmetrics-text;"

type Parser struct {
	textparse.Parser
	s *bufio.Scanner
}

// NewParser is a tiny layer between textparse.Parser and importer.Parser which works on io.Reader.
func NewParser(r io.Reader) textparse.Parser {
	return &Parser{s: bufio.NewScanner(r)}
}

// Next advances the parser to the next sample. It returns io.EOF if no
// more samples were read.
func (p *Parser) Next() (textparse.Entry, error) {
	for p.s.Scan() {
		line := p.s.Bytes()
		line = append(line, '\n')
		// fmt.Println(string(line))
		p.Parser = textparse.New(line, contentType)
		if et, err := p.Parser.Next(); err != io.EOF {
			return et, err
		}
	}
	if err := p.s.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}

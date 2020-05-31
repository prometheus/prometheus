// Copyright 2019 Tobias Guggenmos
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

package cache

import (
	"go/token"

	promql "github.com/prometheus/prometheus/promql/parser"
)

// CompiledQuery stores the results of compiling one query.
type CompiledQuery struct {
	Pos     token.Pos
	Ast     promql.Node
	Err     promql.ParseErrors
	Content string
	Record  string
}

// compile asynchronously parses the provided document.
func (d *DocumentHandle) compile() error {
	defer d.doc.compilers.Done()

	switch d.GetLanguageID() {
	case "promql":
		d.doc.compilers.Add(1)
		return d.compileQuery(true, 0, 0, "")
	case "yaml":
		err := d.parseYamls()
		if err != nil {
			return err
		}

		d.doc.compilers.Add(1)

		err = d.scanYamlTree()
		if err != nil {
			return err
		}
	default:
	}

	return nil
}

// compileQuery compiles the query at the position given by the last two arguments.
//
// If fullFile is set, the last two arguments are ignored and the full file is assumed
// to be one query.
//
// d.compilers.Add(1) must be called before calling this.
func (d *DocumentHandle) compileQuery(fullFile bool, pos token.Pos, endPos token.Pos, record string) error {
	defer d.doc.compilers.Done()

	var content string

	var expired error

	if fullFile {
		content, expired = d.GetContent()
		pos = token.Pos(d.doc.posData.Base())
	} else {
		content, expired = d.GetSubstring(pos, endPos)
	}

	if expired != nil {
		return expired
	}

	ast, err := promql.ParseExpr(content)

	var parseErr promql.ParseErrors

	var ok bool

	if parseErr, ok = err.(promql.ParseErrors); !ok {
		parseErr = nil
	}

	err = d.addCompileResult(pos, ast, parseErr, record, content)
	if err != nil {
		return err
	}

	for _, e := range parseErr {
		diagnostic, err := d.promQLErrToProtocolDiagnostic(pos, &e) //nolint:scopelint
		if err != nil {
			return err
		}

		err = d.addDiagnostic(diagnostic)
		if err != nil {
			return err
		}
	}

	return nil
}

// addCompileResult adds a compiled query compilation results of a Document.
//
// If the DocumentHandle is expired, the result is discarded.
func (d *DocumentHandle) addCompileResult(pos token.Pos, ast promql.Node, err promql.ParseErrors, record string, content string) error {
	d.doc.mu.Lock()
	defer d.doc.mu.Unlock()

	select {
	case <-d.ctx.Done():
		return d.ctx.Err()
	default:
		d.doc.queries = append(d.doc.queries, &CompiledQuery{pos, ast, err, content, record})
		return nil
	}
}

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

package template

import (
	"bytes"
	"context"
	"fmt"
	html_template "html/template"
	"maps"
	"net/url"
	text_template "text/template"
	"time"

	"github.com/prometheus/common/model"

	internaltemplate "github.com/prometheus/prometheus/internal/template"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/features"
)

// QueryFunc executes a PromQL query at the given time.
type QueryFunc func(context.Context, string, time.Time) (promql.Vector, error)

// Expander executes templates in text or HTML mode with a common set of Prometheus template functions.
type Expander struct {
	text    string
	name    string
	data    any
	funcMap text_template.FuncMap
	options []string
}

// NewTemplateExpander returns a template expander ready to use.
func NewTemplateExpander(
	ctx context.Context,
	text string,
	name string,
	data any,
	timestamp model.Time,
	queryFunc QueryFunc,
	externalURL *url.URL,
	options []string,
) *Expander {
	if options == nil {
		options = []string{"missingkey=zero"}
	}
	return &Expander{
		text:    text,
		name:    name,
		data:    data,
		funcMap: internaltemplate.NewFuncMap(ctx, timestamp, queryFunc, externalURL),
		options: options,
	}
}

// AlertTemplateData returns the interface to be used in expanding the template.
func AlertTemplateData(labels, externalLabels map[string]string, externalURL string, smpl promql.Sample) any {
	res := struct {
		Labels         map[string]string
		ExternalLabels map[string]string
		ExternalURL    string
		Value          any
	}{
		Labels:         labels,
		ExternalLabels: externalLabels,
		ExternalURL:    externalURL,
		Value:          smpl.F,
	}

	if smpl.H != nil {
		res.Value = smpl.H
	}

	return res
}

// Funcs adds the functions in fm to the Expander's function map.
// Existing functions will be overwritten in case of conflict.
func (te Expander) Funcs(fm text_template.FuncMap) {
	maps.Copy(te.funcMap, fm)
}

// Expand expands a template in text (non-HTML) mode.
func (te Expander) Expand() (string, error) {
	return internaltemplate.ExpandText(te.name, te.text, te.data, te.funcMap, te.options)
}

// ExpandHTML expands a template with HTML escaping, with templates read from the given files.
func (te Expander) ExpandHTML(templateFiles []string) (result string, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			resultErr, ok = r.(error)
			if !ok {
				resultErr = fmt.Errorf("panic expanding template %s: %v", te.name, r)
			}
		}
	}()
	//nolint:unconvert // Before Go 1.19 conversion from text_template to html_template is mandatory
	tmpl := html_template.New(te.name).Funcs(html_template.FuncMap(te.funcMap))
	tmpl.Option(te.options...)
	tmpl.Funcs(html_template.FuncMap{
		"tmpl": func(name string, data any) (html_template.HTML, error) {
			var buffer bytes.Buffer
			err := tmpl.ExecuteTemplate(&buffer, name, data)
			return html_template.HTML(buffer.String()), err
		},
	})
	tmpl, err := tmpl.Parse(te.text)
	if err != nil {
		return "", fmt.Errorf("error parsing template %v: %w", te.name, err)
	}
	if len(templateFiles) > 0 {
		_, err = tmpl.ParseFiles(templateFiles...)
		if err != nil {
			return "", fmt.Errorf("error parsing template files for %v: %w", te.name, err)
		}
	}
	var buffer bytes.Buffer
	err = tmpl.Execute(&buffer, te.data)
	if err != nil {
		return "", fmt.Errorf("error executing template %v: %w", te.name, err)
	}
	return buffer.String(), nil
}

// ParseTest parses the templates and returns the error if any.
func (te Expander) ParseTest() error {
	_, err := text_template.New(te.name).Funcs(te.funcMap).Option("missingkey=zero").Parse(te.text)
	if err != nil {
		return err
	}
	return nil
}

// templateFunctions returns a representative funcMap with all available template functions.
// This is used to discover which functions are available for feature registration.
func templateFunctions() text_template.FuncMap {
	// Create a dummy expander to get the function map.
	expander := NewTemplateExpander(
		context.Background(),
		"",
		"",
		nil,
		0,
		nil,
		&url.URL{},
		nil,
	)
	return expander.funcMap
}

// RegisterFeatures registers all template functions with the feature registry.
func RegisterFeatures(r features.Collector) {
	// Get all function names from the template function map.
	funcMap := templateFunctions()
	for name := range funcMap {
		r.Enable(features.TemplatingFunctions, name)
	}
}

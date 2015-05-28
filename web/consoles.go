// Copyright 2014 The Prometheus Authors
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

package web

import (
	"flag"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/template"
)

var (
	consoleTemplatesPath = flag.String("web.console.templates", "consoles", "Path to the console template directory, available at /console.")
	consoleLibrariesPath = flag.String("web.console.libraries", "console_libraries", "Path to the console library directory.")
)

// ConsolesHandler implements http.Handler.
type ConsolesHandler struct {
	QueryEngine *promql.Engine
	PathPrefix  string
}

func (h *ConsolesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	file, err := http.Dir(*consoleTemplatesPath).Open(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	text, err := ioutil.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Provide URL parameters as a map for easy use. Advanced users may have need for
	// parameters beyond the first, so provide RawParams.
	rawParams, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := map[string]string{}
	for k, v := range rawParams {
		params[k] = v[0]
	}
	data := struct {
		RawParams url.Values
		Params    map[string]string
		Path      string
	}{
		RawParams: rawParams,
		Params:    params,
		Path:      r.URL.Path,
	}

	tmpl := template.NewTemplateExpander(string(text), "__console_"+r.URL.Path, data, clientmodel.Now(), h.QueryEngine, h.PathPrefix)
	filenames, err := filepath.Glob(*consoleLibrariesPath + "/*.lib")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := tmpl.ExpandHTML(filenames)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, result)
}

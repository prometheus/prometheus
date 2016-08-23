// Copyright 2016 The Prometheus Authors
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

package frankenstein

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/prometheus/common/log"

	template "html/template"

	"github.com/prometheus/prometheus/web/ui"
)

func getTemplate(name string) (string, error) {
	baseTmpl, err := ui.Asset("web/ui/templates/_frankenstein_base.html")
	if err != nil {
		return "", fmt.Errorf("error reading base template: %s", err)
	}
	pageTmpl, err := ui.Asset(filepath.Join("web/ui/templates", name))
	if err != nil {
		return "", fmt.Errorf("error reading page template %s: %s", name, err)
	}
	return string(baseTmpl) + string(pageTmpl), nil
}

func GraphHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		text, err := getTemplate("graph.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		tmpl := template.New("graph.html")
		tmpl.Funcs(template.FuncMap{
			"pathPrefix": func() string { return "." },
		})
		tmpl, err = tmpl.Parse(text)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

		var result bytes.Buffer
		err = tmpl.Execute(&result, nil)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		io.WriteString(w, result.String())
	})
}

func StaticAssetsHandler(stripPrefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fp := strings.TrimPrefix(req.URL.Path, stripPrefix)
		fp = filepath.Join("web/ui/static", fp)

		info, err := ui.AssetInfo(fp)
		if err != nil {
			log.With("file", fp).Warn("Could not get file info: ", err)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		file, err := ui.Asset(fp)
		if err != nil {
			if err != io.EOF {
				log.With("file", fp).Warn("Could not get file: ", err)
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		http.ServeContent(w, req, info.Name(), info.ModTime(), bytes.NewReader(file))
	})
}

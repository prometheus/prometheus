// Copyright 2020 Tobias Guggenmos
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.  // You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
Package rest provides a REST API to access the features of the PromQL language server with a stateless API.

The returned datastructures are taken from the Language Server Protocol Specification (https://microsoft.github.io/language-server-protocol/specifications/specification-current/)


Supported endpoints:

	/diagnostics
	/completion
	/hover
	/signatureHelp

URL Query Parameters:

	expr  : A PromQL expression.
	limit : (optional, only for /diagnostics and /completion endpoints) The maximum number of diagnostic messages returned.
	line  : (only for /signatureHelp, /hover and /completion endpoints) The line (0 based) for which the metadata is queried.
	char  : (only for /signatureHelp, /hover and /completion endpoints) The column (0 based) for which the metadata is queried. Characters are counted as UTF16 Codepoints.


Examples:

  $ curl 'localhost:8080/diagnostics?expr=some_metric()&limit=100'|jq
  [
    {
      "range": {
        "start": {
          "line": 0,
          "character": 0
        },
        "end": {
          "line": 0,
          "character": 11
        }
      },
      "severity": 1,
      "source": "promql-lsp",
      "message": "unknown function with name \"some_metric\""
    }
  ]


  $  curl 'localhost:8080/completion?expr=sum(go)&line=0&char=6&limit=2'|jq
  [
    {
      "label": "go_gc_duration_seconds",
      "kind": 12,
      "sortText": "__3__go_gc_duration_seconds",
      "textEdit": {
        "range": {
          "start": {
            "line": 0,
            "character": 4
          },
          "end": {
            "line": 0,
            "character": 6
          }
        },
        "newText": "go_gc_duration_seconds"
      }
    },
    {
      "label": "go_gc_duration_seconds_count",
      "kind": 12,
      "sortText": "__3__go_gc_duration_seconds_count",
      "textEdit": {
        "range": {
          "start": {
            "line": 0,
            "character": 4
          },
          "end": {
            "line": 0,
            "character": 6
          }
        },
        "newText": "go_gc_duration_seconds_count"
      }
    }
  ]


Try out the API:

Use the PromQL language server with a configuration file like this:

  rest_api_port: 8080
  prometheus_url: http://localhost:9090

Run it with:

  $ promql-langserver --config-file config.yaml
  REST API: Listening on port  8080
  Prometheus: http://localhost:9090

*/
package rest

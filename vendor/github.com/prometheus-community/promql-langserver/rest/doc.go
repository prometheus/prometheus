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

All endpoints are only available through the HTTP method POST. For each request, you have to provide the following JSON:

{
  "expr": "a PromQL expression" # Mandatory for all available endpoints
  "limit": 45  # Optional. It will be used only for the endpoints /diagnostics and /completion. It's the maximum number of results returned.
  "positionLine": 0 # Mandatory for the endpoints /signatureHelp, /hover and /completion. The line (0 based) for which the metadata is queried.
  "positionChar": 2 # Mandatory for the endpoints /signatureHelp, /hover and /completion. The column (0 based) for which the metadata is queried. Characters are counted as UTF-16 code points.
}


Examples:

  $ curl -XPOST 'localhost:8080/diagnostics' -H "Content-Type: application/json" --data '{"expr": "some_metric()", "limit":100}'|jq
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


  $  curl -XPOST 'localhost:8080/completion' -H "Content-Type: application/json" --data '{"expr": "sum(go)", "limit":2, "positionLine":0, "positionChar":6}'|jq
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

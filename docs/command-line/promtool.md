---
title: promtool
---

Tooling for the Prometheus monitoring system.


## Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">-h</code>, <code class="text-nowrap">--help</code> | Show context-sensitive help (also try --help-long and --help-man). |
| <code class="text-nowrap">--version</code> | Show application version. |
| <code class="text-nowrap">--experimental</code> | Enable experimental commands. |
| <code class="text-nowrap">--enable-feature</code> <code class="text-nowrap">...<code class="text-nowrap"> | Comma separated feature names to enable. Valid options: promql-experimental-functions, promql-delayed-name-removal. See https://prometheus.io/docs/prometheus/latest/feature_flags/ for more details |




## Commands

| Command | Description |
| --- | --- |
| help | Show help. |
| check | Check the resources for validity. |
| query | Run query against a Prometheus server. |
| debug | Fetch debug information. |
| push | Push to a Prometheus server. |
| test | Unit testing. |
| tsdb | Run tsdb commands. |
| promql | PromQL formatting and editing. Requires the --experimental flag. |




### `promtool help`

Show help.



#### Arguments

| Argument | Description |
| --- | --- |
| command | Show help on command. |




### `promtool check`

Check the resources for validity.



#### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--query.lookback-delta</code> | The server's maximum query lookback duration. | `5m` |




##### `promtool check service-discovery`

Perform service discovery for the given job name and report the results, including relabeling.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--timeout</code> | The time to wait for discovery results. | `30s` |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| config-file | The prometheus config file. | Yes |
| job | The job to run service discovery for. | Yes |




##### `promtool check config`

Check if the config files are valid or not.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--syntax-only</code> | Only check the config file syntax, ignoring file and content validation referenced in the config |  |
| <code class="text-nowrap">--lint</code> | Linting checks to apply to the rules/scrape configs specified in the config. Available options are: all, duplicate-rules, none, too-long-scrape-interval. Use --lint=none to disable linting | `duplicate-rules` |
| <code class="text-nowrap">--lint-fatal</code> | Make lint errors exit with exit code 3. | `false` |
| <code class="text-nowrap">--ignore-unknown-fields</code> | Ignore unknown fields in the rule groups read by the config files. This is useful when you want to extend rule files with custom metadata. Ensure that those fields are removed before loading them into the Prometheus server as it performs strict checks by default. | `false` |
| <code class="text-nowrap">--agent</code> | Check config file for Prometheus in Agent mode. |  |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| config-files | The config files to check. | Yes |




##### `promtool check web-config`

Check if the web config files are valid or not.



###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| web-config-files | The config files to check. | Yes |




##### `promtool check healthy`

Check if the Prometheus server is healthy.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--http.config.file</code> | HTTP client configuration file, see details at https://prometheus.io/docs/prometheus/latest/configuration/promtool |  |
| <code class="text-nowrap">--url</code> | The URL for the Prometheus server. | `http://localhost:9090` |




##### `promtool check ready`

Check if the Prometheus server is ready.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--http.config.file</code> | HTTP client configuration file, see details at https://prometheus.io/docs/prometheus/latest/configuration/promtool |  |
| <code class="text-nowrap">--url</code> | The URL for the Prometheus server. | `http://localhost:9090` |




##### `promtool check rules`

Check if the rule files are valid or not.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--lint</code> | Linting checks to apply. Available options are: all, duplicate-rules, none. Use --lint=none to disable linting | `duplicate-rules` |
| <code class="text-nowrap">--lint-fatal</code> | Make lint errors exit with exit code 3. | `false` |
| <code class="text-nowrap">--ignore-unknown-fields</code> | Ignore unknown fields in the rule files. This is useful when you want to extend rule files with custom metadata. Ensure that those fields are removed before loading them into the Prometheus server as it performs strict checks by default. | `false` |




###### Arguments

| Argument | Description |
| --- | --- |
| rule-files | The rule files to check, default is read from standard input. |




##### `promtool check metrics`

Pass Prometheus metrics over stdin to lint them for consistency and correctness, and optionally perform cardinality analysis.

examples:

$ cat metrics.prom | promtool check metrics

$ curl -s http://localhost:9090/metrics | promtool check metrics `--extended`

$ curl -s http://localhost:9100/metrics | promtool check metrics `--extended` `--lint`=none



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--extended</code> | Print extended information related to the cardinality of the metrics. |  |
| <code class="text-nowrap">--lint</code> | Linting checks to apply for metrics. Available options are: all, none. Use --lint=none to disable metrics linting. | `all` |




### `promtool query`

Run query against a Prometheus server.



#### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">-o</code>, <code class="text-nowrap">--format</code> | Output format of the query. | `promql` |
| <code class="text-nowrap">--http.config.file</code> | HTTP client configuration file, see details at https://prometheus.io/docs/prometheus/latest/configuration/promtool |  |




##### `promtool query instant`

Run instant query.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--time</code> | Query evaluation time (RFC3339 or Unix timestamp). |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to query. | Yes |
| expr | PromQL query expression. | Yes |




##### `promtool query range`

Run range query.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--header</code> | Extra headers to send to server. |
| <code class="text-nowrap">--start</code> | Query range start time (RFC3339 or Unix timestamp). |
| <code class="text-nowrap">--end</code> | Query range end time (RFC3339 or Unix timestamp). |
| <code class="text-nowrap">--step</code> | Query step size (duration). |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to query. | Yes |
| expr | PromQL query expression. | Yes |




##### `promtool query series`

Run series query.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--match</code> <code class="text-nowrap">...<code class="text-nowrap"> | Series selector. Can be specified multiple times. |
| <code class="text-nowrap">--start</code> | Start time (RFC3339 or Unix timestamp). |
| <code class="text-nowrap">--end</code> | End time (RFC3339 or Unix timestamp). |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to query. | Yes |




##### `promtool query labels`

Run labels query.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--start</code> | Start time (RFC3339 or Unix timestamp). |
| <code class="text-nowrap">--end</code> | End time (RFC3339 or Unix timestamp). |
| <code class="text-nowrap">--match</code> <code class="text-nowrap">...<code class="text-nowrap"> | Series selector. Can be specified multiple times. |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to query. | Yes |
| name | Label name to provide label values for. | Yes |




##### `promtool query analyze`

Run queries against your Prometheus to analyze the usage pattern of certain metrics.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--server</code> | Prometheus server to query. |  |
| <code class="text-nowrap">--type</code> | Type of metric: histogram. |  |
| <code class="text-nowrap">--duration</code> | Time frame to analyze. | `1h` |
| <code class="text-nowrap">--time</code> | Query time (RFC3339 or Unix timestamp), defaults to now. |  |
| <code class="text-nowrap">--match</code> <code class="text-nowrap">...<code class="text-nowrap"> | Series selector. Can be specified multiple times. |  |




### `promtool debug`

Fetch debug information.



##### `promtool debug pprof`

Fetch profiling debug information.



###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to get pprof files from. | Yes |




##### `promtool debug metrics`

Fetch metrics debug information.



###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to get metrics from. | Yes |




##### `promtool debug all`

Fetch all debug information.



###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| server | Prometheus server to get all debug information from. | Yes |




### `promtool push`

Push to a Prometheus server.



#### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--http.config.file</code> | HTTP client configuration file, see details at https://prometheus.io/docs/prometheus/latest/configuration/promtool |




##### `promtool push metrics`

Push metrics to a prometheus remote write (for testing purpose only).



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--label</code> | Label to attach to metrics. Can be specified multiple times. | `job=promtool` |
| <code class="text-nowrap">--timeout</code> | The time to wait for pushing metrics. | `30s` |
| <code class="text-nowrap">--header</code> | Prometheus remote write header. |  |
| <code class="text-nowrap">--protobuf_message</code> | Protobuf message to use when writing (prometheus.WriteRequest or io.prometheus.write.v2.Request). | `prometheus.WriteRequest` |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| remote-write-url | Prometheus remote write url to push metrics. | Yes |
| metric-files | The metric files to push, default is read from standard input. |  |




### `promtool test`

Unit testing.



#### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--junit</code> | File path to store JUnit XML test results. |




##### `promtool test rules`

Unit tests for rules.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--run</code> <code class="text-nowrap">...<code class="text-nowrap"> | If set, will only run test groups whose names match the regular expression. Can be specified multiple times. |  |
| <code class="text-nowrap">--debug</code> | Enable unit test debugging. | `false` |
| <code class="text-nowrap">--diff</code> | [Experimental] Print colored differential output between expected & received output. | `false` |
| <code class="text-nowrap">--ignore-unknown-fields</code> | Ignore unknown fields in the test files. This is useful when you want to extend rule files with custom metadata. Ensure that those fields are removed before loading them into the Prometheus server as it performs strict checks by default. | `false` |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| test-rule-file | The unit test file. | Yes |




### `promtool tsdb`

Run tsdb commands.



##### `promtool tsdb bench`

Run benchmarks.



##### `promtool tsdb bench write`

Run a write performance benchmark.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--out</code> | Set the output path. | `benchout` |
| <code class="text-nowrap">--metrics</code> | Number of metrics to read. | `10000` |
| <code class="text-nowrap">--scrapes</code> | Number of scrapes to simulate. | `3000` |




###### Arguments

| Argument | Description | Default |
| --- | --- | --- |
| file | Input file with samples data, default is (../../tsdb/testdata/20kseries.json). | `../../tsdb/testdata/20kseries.json` |




##### `promtool tsdb analyze`

Analyze churn, label pair cardinality and compaction efficiency.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--limit</code> | How many items to show in each list. | `20` |
| <code class="text-nowrap">--extended</code> | Run extended analysis. |  |
| <code class="text-nowrap">--match</code> | Series selector to analyze. Only 1 set of matchers is supported now. |  |




###### Arguments

| Argument | Description | Default |
| --- | --- | --- |
| db path | Database path (default is data/). | `data/` |
| block id | Block to analyze (default is the last block). |  |




##### `promtool tsdb list`

List tsdb blocks.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">-r</code>, <code class="text-nowrap">--human-readable</code> | Print human readable values. |




###### Arguments

| Argument | Description | Default |
| --- | --- | --- |
| db path | Database path (default is data/). | `data/` |




##### `promtool tsdb dump`

Dump data (series+samples or optionally just series) from a TSDB.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--sandbox-dir-root</code> | Root directory where a sandbox directory will be created, this sandbox is used in case WAL replay generates chunks (default is the database path). The sandbox is cleaned up at the end. |  |
| <code class="text-nowrap">--min-time</code> | Minimum timestamp to dump, in milliseconds since the Unix epoch. | `-9223372036854775808` |
| <code class="text-nowrap">--max-time</code> | Maximum timestamp to dump, in milliseconds since the Unix epoch. | `9223372036854775807` |
| <code class="text-nowrap">--match</code> <code class="text-nowrap">...<code class="text-nowrap"> | Series selector. Can be specified multiple times. | `{__name__=~'(?s:.*)'}` |
| <code class="text-nowrap">--format</code> | Output format of the dump (prom (default) or seriesjson). | `prom` |




###### Arguments

| Argument | Description | Default |
| --- | --- | --- |
| db path | Database path (default is data/). | `data/` |




##### `promtool tsdb dump-openmetrics`

[Experimental] Dump samples from a TSDB into OpenMetrics text format, excluding native histograms and staleness markers, which are not representable in OpenMetrics.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--sandbox-dir-root</code> | Root directory where a sandbox directory will be created, this sandbox is used in case WAL replay generates chunks (default is the database path). The sandbox is cleaned up at the end. |  |
| <code class="text-nowrap">--min-time</code> | Minimum timestamp to dump, in milliseconds since the Unix epoch. | `-9223372036854775808` |
| <code class="text-nowrap">--max-time</code> | Maximum timestamp to dump, in milliseconds since the Unix epoch. | `9223372036854775807` |
| <code class="text-nowrap">--match</code> <code class="text-nowrap">...<code class="text-nowrap"> | Series selector. Can be specified multiple times. | `{__name__=~'(?s:.*)'}` |




###### Arguments

| Argument | Description | Default |
| --- | --- | --- |
| db path | Database path (default is data/). | `data/` |




##### `promtool tsdb create-blocks-from`

[Experimental] Import samples from input and produce TSDB blocks. Please refer to the storage docs for more details.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">-r</code>, <code class="text-nowrap">--human-readable</code> | Print human readable values. |
| <code class="text-nowrap">-q</code>, <code class="text-nowrap">--quiet</code> | Do not print created blocks. |




##### `promtool tsdb create-blocks-from openmetrics`

Import samples from OpenMetrics input and produce TSDB blocks. Please refer to the storage docs for more details.



###### Flags

| Flag | Description |
| --- | --- |
| <code class="text-nowrap">--label</code> | Label to attach to metrics. Can be specified multiple times. Example --label=label_name=label_value |




###### Arguments

| Argument | Description | Default | Required |
| --- | --- | --- | --- |
| input file | OpenMetrics file to read samples from. |  | Yes |
| output directory | Output directory for generated blocks. | `data/` |  |




##### `promtool tsdb create-blocks-from rules`

Create blocks of data for new recording rules.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">--http.config.file</code> | HTTP client configuration file, see details at https://prometheus.io/docs/prometheus/latest/configuration/promtool |  |
| <code class="text-nowrap">--url</code> | The URL for the Prometheus API with the data where the rule will be backfilled from. | `http://localhost:9090` |
| <code class="text-nowrap">--start</code> | The time to start backfilling the new rule from. Must be a RFC3339 formatted date or Unix timestamp. Required. |  |
| <code class="text-nowrap">--end</code> | If an end time is provided, all recording rules in the rule files provided will be backfilled to the end time. Default will backfill up to 3 hours ago. Must be a RFC3339 formatted date or Unix timestamp. |  |
| <code class="text-nowrap">--output-dir</code> | Output directory for generated blocks. | `data/` |
| <code class="text-nowrap">--eval-interval</code> | How frequently to evaluate rules when backfilling if a value is not set in the recording rule files. | `60s` |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| rule-files | A list of one or more files containing recording rules to be backfilled. All recording rules listed in the files will be backfilled. Alerting rules are not evaluated. | Yes |




### `promtool promql`

PromQL formatting and editing. Requires the `--experimental` flag.



##### `promtool promql format`

Format PromQL query to pretty printed form.



###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| query | PromQL query. | Yes |




##### `promtool promql label-matchers`

Edit label matchers contained within an existing PromQL query.



##### `promtool promql label-matchers set`

Set a label matcher in the query.



###### Flags

| Flag | Description | Default |
| --- | --- | --- |
| <code class="text-nowrap">-t</code>, <code class="text-nowrap">--type</code> | Type of the label matcher to set. | `=` |




###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| query | PromQL query. | Yes |
| name | Name of the label matcher to set. | Yes |
| value | Value of the label matcher to set. | Yes |




##### `promtool promql label-matchers delete`

Delete a label from the query.



###### Arguments

| Argument | Description | Required |
| --- | --- | --- |
| query | PromQL query. | Yes |
| name | Name of the label to delete. | Yes |



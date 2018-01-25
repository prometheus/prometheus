## 2.1.0 / 2018-1-19

* [FEATURE] New Service Discovery UI showing labels before and after relabelling.
* [FEATURE] New Admin APIs added to v1 to delete, snapshot and remove tombstones.
* [ENHANCEMENT] The graph UI autcomplete now includes your previous queries.
* [ENHANCEMENT] Federation is now much faster for large numbers of series.
* [ENHANCEMENT] Added new metrics to measure rule timings.
* [ENHANCEMENT] Rule evaluation times added to the rules UI.
* [ENHANCEMENT] Added metrics to measure modified time of file SD files.
* [ENHANCEMENT] Kubernetes SD now includes POD UID in discovery metadata.
* [ENHANCEMENT] The Query APIs now return optional stats on query execution times.
* [ENHANCEMENT] The index now no longer has the 4GiB size limit and is also smaller.
* [BUGFIX] Remote read `read_recent` option is now false by default.
* [BUGFIX] Pass the right configuration to each Alertmanager (AM) when using multiple AM configs.
* [BUGFIX] Fix not-matchers not selecting series with labels unset.
* [BUGFIX] tsdb: Fix occasional panic in head block.
* [BUGFIX] tsdb: Close files before deletion to fix retention issues on Windows and NFS.
* [BUGFIX] tsdb: Cleanup and do not retry failing compactions.
* [BUGFIX] tsdb: Close WAL while shutting down.


## 2.0.0 / 2017-11-08

This release includes a completely rewritten storage, huge performance
improvements, but also many backwards incompatible changes. For more
information, read the announcement blog post and migration guide.

https://prometheus.io/blog/2017/11/08/announcing-prometheus-2-0/
https://prometheus.io/docs/prometheus/2.0/migration/

* [CHANGE] Completely rewritten storage layer, with WAL. This is not backwards compatible with 1.x storage, and many flags have changed/disappeared.
* [CHANGE] New staleness behavior. Series now marked stale after target scrapes no longer return them, and soon after targets disappear from service discovery.
* [CHANGE] Rules files use YAML syntax now. Conversion tool added to promtool.
* [CHANGE] Removed `count_scalar`, `drop_common_labels` functions and `keep_common` modifier from PromQL.
* [CHANGE] Rewritten exposition format parser with much higher performance. The Protobuf exposition format is no longer supported.
* [CHANGE] Example console templates updated for new storage and metrics names. Examples other than node exporter and Prometheus removed.
* [CHANGE] Admin and lifecycle APIs now disabled by default, can be reenabled via flags
* [CHANGE] Flags switched to using Kingpin, all flags are now --flagname rather than -flagname.
* [FEATURE/CHANGE] Remote read can be configured to not read data which is available locally. This is enabled by default.
* [FEATURE] Rules can be grouped now. Rules within a rule group are executed sequentially.
* [FEATURE] Added experimental GRPC apis
* [FEATURE] Add timestamp() function to PromQL.
* [ENHANCEMENT] Remove remote read from the query path if no remote storage is configured.
* [ENHANCEMENT] Bump Consul HTTP client timeout to not match the Consul SD watch timeout.
* [ENHANCEMENT] Go-conntrack added to provide HTTP connection metrics.
* [BUGFIX] Fix connection leak in Consul SD.

## 1.8.2 / 2017-11-04

* [BUGFIX] EC2 service discovery: Do not crash if tags are empty.

## 1.8.1 / 2017-10-19

* [BUGFIX] Correctly handle external labels on remote read endpoint

## 1.8.0 / 2017-10-06

* [CHANGE] Rule links link to the _Console_ tab rather than the _Graph_ tab to
  not trigger expensive range queries by default.
* [FEATURE] Ability to act as a remote read endpoint for other Prometheus
  servers.
* [FEATURE] K8s SD: Support discovery of ingresses.
* [FEATURE] Consul SD: Support for node metadata.
* [FEATURE] Openstack SD: Support discovery of hypervisors.
* [FEATURE] Expose current Prometheus config via `/status/config`.
* [FEATURE] Allow to collapse jobs on `/targets` page.
* [FEATURE] Add `/-/healthy` and `/-/ready` endpoints.
* [FEATURE] Add color scheme support to console templates.
* [ENHANCEMENT] Remote storage connections use HTTP keep-alive.
* [ENHANCEMENT] Improved logging about remote storage.
* [ENHANCEMENT] Relaxed URL validation.
* [ENHANCEMENT] Openstack SD: Handle instances without IP.
* [ENHANCEMENT] Make remote storage queue manager configurable.
* [ENHANCEMENT] Validate metrics returned from remote read.
* [ENHANCEMENT] EC2 SD: Set a default region.
* [ENHANCEMENT] Changed help link to `https://prometheus.io/docs`.
* [BUGFIX] Fix floating-point precision issue in `deriv` function.
* [BUGFIX] Fix pprof endpoints when -web.route-prefix or -web.external-url is
  used.
* [BUGFIX] Fix handling of `null` target groups in file-based SD.
* [BUGFIX] Set the sample timestamp in date-related PromQL functions.
* [BUGFIX] Apply path prefix to redirect from deprecated graph URL.
* [BUGFIX] Fixed tests on MS Windows.
* [BUGFIX] Check for invalid UTF-8 in label values after relabeling.

## 1.7.2 / 2017-09-26

* [BUGFIX] Correctly remove all targets from DNS service discovery if the
  corresponding DNS query succeeds and returns an empty result.
* [BUGFIX] Correctly parse resolution input in expression browser.
* [BUGFIX] Consistently use UTC in the date picker of the expression browser.
* [BUGFIX] Correctly handle multiple ports in Marathon service discovery.
* [BUGFIX] Fix HTML escaping so that HTML templates compile with Go1.9.
* [BUGFIX] Prevent number of remote write shards from going negative.
* [BUGFIX] In the graphs created by the expression browser, render very large
  and small numbers in a readable way.
* [BUGFIX] Fix a rarely occurring iterator issue in varbit encoded chunks.

## 1.7.1 / 2017-06-12

* [BUGFIX] Fix double prefix redirect.

## 1.7.0 / 2017-06-06

* [CHANGE] Compress remote storage requests and responses with unframed/raw snappy.
* [CHANGE] Properly ellide secrets in config.
* [FEATURE] Add OpenStack service discovery.
* [FEATURE] Add ability to limit Kubernetes service discovery to certain namespaces.
* [FEATURE] Add metric for discovered number of Alertmanagers.
* [ENHANCEMENT] Print system information (uname) on start up.
* [ENHANCEMENT] Show gaps in graphs on expression browser.
* [ENHANCEMENT] Promtool linter checks counter naming and more reserved labels.
* [BUGFIX] Fix broken Mesos discovery.
* [BUGFIX] Fix redirect when external URL is set.
* [BUGFIX] Fix mutation of active alert elements by notifier.
* [BUGFIX] Fix HTTP error handling for remote write.
* [BUGFIX] Fix builds for Solaris/Illumos.
* [BUGFIX] Fix overflow checking in global config.
* [BUGFIX] Fix log level reporting issue.
* [BUGFIX] Fix ZooKeeper serverset discovery can become out-of-sync.

## 1.6.3 / 2017-05-18

* [BUGFIX] Fix disappearing Alertmanger targets in Alertmanager discovery.
* [BUGFIX] Fix panic with remote_write on ARMv7.
* [BUGFIX] Fix stacked graphs to adapt min/max values.

## 1.6.2 / 2017-05-11

* [BUGFIX] Fix potential memory leak in Kubernetes service discovery

## 1.6.1 / 2017-04-19

* [BUGFIX] Don't panic if storage has no FPs even after initial wait

## 1.6.0 / 2017-04-14

* [CHANGE] Replaced the remote write implementations for various backends by a
  generic write interface with example adapter implementation for various
  backends. Note that both the previous and the current remote write
  implementations are **experimental**.
* [FEATURE] New flag `-storage.local.target-heap-size` to tell Prometheus about
  the desired heap size. This deprecates the flags
  `-storage.local.memory-chunks` and `-storage.local.max-chunks-to-persist`,
  which are kept for backward compatibility.
* [FEATURE] Add `check-metrics` to `promtool` to lint metric names.
* [FEATURE] Add Joyent Triton discovery.
* [FEATURE] `X-Prometheus-Scrape-Timeout-Seconds` header in HTTP scrape
  requests.
* [FEATURE] Remote read interface, including example for InfluxDB. **Experimental.**
* [FEATURE] Enable Consul SD to connect via TLS.
* [FEATURE] Marathon SD supports multiple ports.
* [FEATURE] Marathon SD supports bearer token for authentication.
* [FEATURE] Custom timeout for queries.
* [FEATURE] Expose `buildQueryUrl` in `graph.js`.
* [FEATURE] Add `rickshawGraph` property to the graph object in console
  templates.
* [FEATURE] New metrics exported by Prometheus itself:
  * Summary `prometheus_engine_query_duration_seconds`
  * Counter `prometheus_evaluator_iterations_missed_total`
  * Counter `prometheus_evaluator_iterations_total`
  * Gauge `prometheus_local_storage_open_head_chunks`
  * Gauge `prometheus_local_storage_target_heap_size`
* [ENHANCEMENT] Reduce shut-down time by interrupting an ongoing checkpoint
  before starting the final checkpoint.
* [ENHANCEMENT] Auto-tweak times between checkpoints to limit time spent in
  checkpointing to 50%.
* [ENHANCEMENT] Improved crash recovery deals better with certain index
  corruptions.
* [ENHANCEMENT] Graphing deals better with constant time series.
* [ENHANCEMENT] Retry remote writes on recoverable errors.
* [ENHANCEMENT] Evict unused chunk descriptors during crash recovery to limit
  memory usage.
* [ENHANCEMENT] Smoother disk usage during series maintenance.
* [ENHANCEMENT] Targets on targets page sorted by instance within a job.
* [ENHANCEMENT] Sort labels in federation.
* [ENHANCEMENT] Set `GOGC=40` by default, which results in much better memory
  utilization at the price of slightly higher CPU usage. If `GOGC` is set by
  the user, it is still honored as usual.
* [ENHANCEMENT] Close head chunks after being idle for the duration of the
  configured staleness delta. This helps to persist and evict head chunk of
  stale series more quickly.
* [ENHANCEMENT] Stricter checking of relabel config.
* [ENHANCEMENT] Cache busters for static web content.
* [ENHANCEMENT] Send Prometheus-specific user-agent header during scrapes.
* [ENHANCEMENT] Improved performance of series retention cut-off.
* [ENHANCEMENT] Mitigate impact of non-atomic sample ingestion on
  `histogram_quantile` by enforcing buckets to be monotonic.
* [ENHANCEMENT] Released binaries built with Go 1.8.1.
* [BUGFIX] Send `instance=""` with federation if `instance` not set.
* [BUGFIX] Update to new `client_golang` to get rid of unwanted quantile
  metrics in summaries.
* [BUGFIX] Introduce several additional guards against data corruption.
* [BUGFIX] Mark storage dirty and increment
  `prometheus_local_storage_persist_errors_total` on all relevant errors.
* [BUGFIX] Propagate storage errors as 500 in the HTTP API.
* [BUGFIX] Fix int64 overflow in timestamps in the HTTP API.
* [BUGFIX] Fix deadlock in Zookeeper SD.
* [BUGFIX] Fix fuzzy search problems in the web-UI auto-completion.

## 1.5.3 / 2017-05-11

* [BUGFIX] Fix potential memory leak in Kubernetes service discovery

## 1.5.2 / 2017-02-10

* [BUGFIX] Fix series corruption in a special case of series maintenance where
  the minimum series-file-shrink-ratio kicks in.
* [BUGFIX] Fix two panic conditions both related to processing a series
  scheduled to be quarantined.
* [ENHANCEMENT] Binaries built with Go1.7.5.

## 1.5.1 / 2017-02-07

* [BUGFIX] Don't lose fully persisted memory series during checkpointing.
* [BUGFIX] Fix intermittently failing relabeling.
* [BUGFIX] Make `-storage.local.series-file-shrink-ratio` work.
* [BUGFIX] Remove race condition from TestLoop.

## 1.5.0 / 2017-01-23

* [CHANGE] Use lexicographic order to sort alerts by name.
* [FEATURE] Add Joyent Triton discovery.
* [FEATURE] Add scrape targets and alertmanager targets API.
* [FEATURE] Add various persistence related metrics.
* [FEATURE] Add various query engine related metrics.
* [FEATURE] Add ability to limit scrape samples, and related metrics.
* [FEATURE] Add labeldrop and labelkeep relabelling actions.
* [FEATURE] Display current working directory on status-page.
* [ENHANCEMENT] Strictly use ServiceAccount for in cluster configuration on Kubernetes.
* [ENHANCEMENT] Various performance and memory-management improvements.
* [BUGFIX] Fix basic auth for alertmanagers configured via flag.
* [BUGFIX] Don't panic on decoding corrupt data.
* [BUGFIX] Ignore dotfiles in data directory.
* [BUGFIX] Abort on intermediate federation errors.

## 1.4.1 / 2016-11-28

* [BUGFIX] Fix Consul service discovery

## 1.4.0 / 2016-11-25

* [FEATURE] Allow configuring Alertmanagers via service discovery
* [FEATURE] Display used Alertmanagers on runtime page in the UI
* [FEATURE] Support profiles in AWS EC2 service discovery configuration
* [ENHANCEMENT] Remove duplicated logging of Kubernetes client errors
* [ENHANCEMENT] Add metrics about Kubernetes service discovery
* [BUGFIX] Update alert annotations on re-evaluation
* [BUGFIX] Fix export of group modifier in PromQL queries
* [BUGFIX] Remove potential deadlocks in several service discovery implementations
* [BUGFIX] Use proper float64 modulo in PromQL `%` binary operations
* [BUGFIX] Fix crash bug in Kubernetes service discovery

## 1.3.1 / 2016-11-04

This bug-fix release pulls in the fixes from the 1.2.3 release.

* [BUGFIX] Correctly handle empty Regex entry in relabel config.
* [BUGFIX] MOD (`%`) operator doesn't panic with small floating point numbers.
* [BUGFIX] Updated miekg/dns vendoring to pick up upstream bug fixes.
* [ENHANCEMENT] Improved DNS error reporting.

## 1.2.3 / 2016-11-04

Note that this release is chronologically after 1.3.0.

* [BUGFIX] Correctly handle end time before start time in range queries.
* [BUGFIX] Error on negative `-storage.staleness-delta`
* [BUGFIX] Correctly handle empty Regex entry in relabel config.
* [BUGFIX] MOD (`%`) operator doesn't panic with small floating point numbers.
* [BUGFIX] Updated miekg/dns vendoring to pick up upstream bug fixes.
* [ENHANCEMENT] Improved DNS error reporting.

## 1.3.0 / 2016-11-01

This is a breaking change to the Kubernetes service discovery.

* [CHANGE] Rework Kubernetes SD.
* [FEATURE] Add support for interpolating `target_label`.
* [FEATURE] Add GCE metadata as Prometheus meta labels.
* [ENHANCEMENT] Add EC2 SD metrics.
* [ENHANCEMENT] Add Azure SD metrics.
* [ENHANCEMENT] Add fuzzy search to `/graph` textarea.
* [ENHANCEMENT] Always show instance labels on target page.
* [BUGFIX] Validate query end time is not before start time.
* [BUGFIX] Error on negative `-storage.staleness-delta`

## 1.2.2 / 2016-10-30

* [BUGFIX] Correctly handle on() in alerts.
* [BUGFIX] UI: Deal properly with aborted requests.
* [BUGFIX] UI: Decode URL query parameters properly.
* [BUGFIX] Storage: Deal better with data corruption (non-monotonic timestamps).
* [BUGFIX] Remote storage: Re-add accidentally removed timeout flag.
* [BUGFIX] Updated a number of vendored packages to pick up upstream bug fixes.

## 1.2.1 / 2016-10-10

* [BUGFIX] Count chunk evictions properly so that the server doesn't
  assume it runs out of memory and subsequencly throttles ingestion.
* [BUGFIX] Use Go1.7.1 for prebuilt binaries to fix issues on MacOS Sierra.

## 1.2.0 / 2016-10-07

* [FEATURE] Cleaner encoding of query parameters in `/graph` URLs.
* [FEATURE] PromQL: Add `minute()` function.
* [FEATURE] Add GCE service discovery.
* [FEATURE] Allow any valid UTF-8 string as job name.
* [FEATURE] Allow disabling local storage.
* [FEATURE] EC2 service discovery: Expose `ec2_instance_state`.
* [ENHANCEMENT] Various performance improvements in local storage.
* [BUGFIX] Zookeeper service discovery: Remove deleted nodes.
* [BUGFIX] Zookeeper service discovery: Resync state after Zookeeper failure.
* [BUGFIX] Remove JSON from HTTP Accept header.
* [BUGFIX] Fix flag validation of Alertmanager URL.
* [BUGFIX] Fix race condition on shutdown.
* [BUGFIX] Do not fail Consul discovery on Prometheus startup when Consul
  is down.
* [BUGFIX] Handle NaN in `changes()` correctly.
* [CHANGE] **Experimental** remote write path: Remove use of gRPC.
* [CHANGE] **Experimental** remote write path: Configuration via config file
  rather than command line flags.
* [FEATURE] **Experimental** remote write path: Add HTTP basic auth and TLS.
* [FEATURE] **Experimental** remote write path: Support for relabelling.

## 1.1.3 / 2016-09-16

* [ENHANCEMENT] Use golang-builder base image for tests in CircleCI.
* [ENHANCEMENT] Added unit tests for federation.
* [BUGFIX] Correctly de-dup metric families in federation output.

## 1.1.2 / 2016-09-08

* [BUGFIX] Allow label names that coincide with keywords.

## 1.1.1 / 2016-09-07

* [BUGFIX] Fix IPv6 escaping in service discovery integrations
* [BUGFIX] Fix default scrape port assignment for IPv6

## 1.1.0 / 2016-09-03

* [FEATURE] Add `quantile` and `quantile_over_time`.
* [FEATURE] Add `stddev_over_time` and `stdvar_over_time`.
* [FEATURE] Add various time and date functions.
* [FEATURE] Added `toUpper` and `toLower` formatting to templates.
* [FEATURE] Allow relabeling of alerts.
* [FEATURE] Allow URLs in targets defined via a JSON file.
* [FEATURE] Add idelta function.
* [FEATURE] 'Remove graph' button on the /graph page.
* [FEATURE] Kubernetes SD: Add node name and host IP to pod discovery.
* [FEATURE] New remote storage write path. EXPERIMENTAL!
* [ENHANCEMENT] Improve time-series index lookups.
* [ENHANCEMENT] Forbid invalid relabel configurations.
* [ENHANCEMENT] Improved various tests.
* [ENHANCEMENT] Add crash recovery metric 'started_dirty'.
* [ENHANCEMENT] Fix (and simplify) populating series iterators.
* [ENHANCEMENT] Add job link on target page.
* [ENHANCEMENT] Message on empty Alerts page.
* [ENHANCEMENT] Various internal code refactorings and clean-ups.
* [ENHANCEMENT] Various improvements in the build system.
* [BUGFIX] Catch errors when unmarshalling delta/doubleDelta encoded chunks.
* [BUGFIX] Fix data race in lexer and lexer test.
* [BUGFIX] Trim stray whitespace from bearer token file.
* [BUGFIX] Avoid divide-by-zero panic on query_range?step=0.
* [BUGFIX] Detect invalid rule files at startup.
* [BUGFIX] Fix counter reset treatment in PromQL.
* [BUGFIX] Fix rule HTML escaping issues.
* [BUGFIX] Remove internal labels from alerts sent to AM.

## 1.0.2 / 2016-08-24

* [BUGFIX] Clean up old targets after config reload.

## 1.0.1 / 2016-07-21

* [BUGFIX] Exit with error on non-flag command-line arguments.
* [BUGFIX] Update example console templates to new HTTP API.
* [BUGFIX] Re-add logging flags.

## 1.0.0 / 2016-07-18

* [CHANGE] Remove deprecated query language keywords
* [CHANGE] Change Kubernetes SD to require specifying Kubernetes role
* [CHANGE] Use service address in Consul SD if available
* [CHANGE] Standardize all Prometheus internal metrics to second units
* [CHANGE] Remove unversioned legacy HTTP API
* [CHANGE] Remove legacy ingestion of JSON metric format
* [CHANGE] Remove deprecated `target_groups` configuration
* [FEATURE] Add binary power operation to PromQL
* [FEATURE] Add `count_values` aggregator
* [FEATURE] Add `-web.route-prefix` flag
* [FEATURE] Allow `on()`, `by()`, `without()` in PromQL with empty label sets
* [ENHANCEMENT] Make `topk/bottomk` query functions aggregators
* [BUGFIX] Fix annotations in alert rule printing
* [BUGFIX] Expand alert templating at evaluation time
* [BUGFIX] Fix edge case handling in crash recovery
* [BUGFIX] Hide testing package flags from help output

## 0.20.0 / 2016-06-15

This release contains multiple breaking changes to the configuration schema.

* [FEATURE] Allow configuring multiple Alertmanagers
* [FEATURE] Add server name to TLS configuration
* [FEATURE] Add labels for all node addresses and discover node port if available in Kubernetes SD
* [ENHANCEMENT] More meaningful configuration errors
* [ENHANCEMENT] Round scraping timestamps to milliseconds in web UI
* [ENHANCEMENT] Make number of storage fingerprint locks configurable
* [BUGFIX] Fix date parsing in console template graphs
* [BUGFIX] Fix static console files in Docker images
* [BUGFIX] Fix console JS XHR requests for IE11
* [BUGFIX] Add missing path prefix in new status page
* [CHANGE] Rename `target_groups` to `static_configs` in config files
* [CHANGE] Rename `names` to `files` in file SD configuration
* [CHANGE] Remove kubelet port config option in Kubernetes SD configuration

## 0.19.3 / 2016-06-14

* [BUGFIX] Handle Marathon apps with zero ports
* [BUGFIX] Fix startup panic in retrieval layer

## 0.19.2 / 2016-05-29

* [BUGFIX] Correctly handle `GROUP_LEFT` and `GROUP_RIGHT` without labels in
  string representation of expressions and in rules.
* [BUGFIX] Use `-web.external-url` for new status endpoints.

## 0.19.1 / 2016-05-25

* [BUGFIX] Handle service discovery panic affecting Kubernetes SD
* [BUGFIX] Fix web UI display issue in some browsers

## 0.19.0 / 2016-05-24

This version contains a breaking change to the query language. Please read
the documentation on the grouping behavior of vector matching:

https://prometheus.io/docs/querying/operators/#vector-matching

* [FEATURE] Add experimental Microsoft Azure service discovery
* [FEATURE] Add `ignoring` modifier for binary operations
* [FEATURE] Add pod discovery to Kubernetes service discovery
* [CHANGE] Vector matching takes grouping labels from one-side
* [ENHANCEMENT] Support time range on /api/v1/series endpoint
* [ENHANCEMENT] Partition status page into individual pages
* [BUGFIX] Fix issue of hanging target scrapes

## 0.18.0 / 2016-04-18

* [BUGFIX] Fix operator precedence in PromQL
* [BUGFIX] Never drop still open head chunk
* [BUGFIX] Fix missing 'keep_common' when printing AST node
* [CHANGE/BUGFIX] Target identity considers path and parameters additionally to host and port
* [CHANGE] Rename metric `prometheus_local_storage_invalid_preload_requests_total` to `prometheus_local_storage_non_existent_series_matches_total`
* [CHANGE] Support for old alerting rule syntax dropped
* [FEATURE] Deduplicate targets within the same scrape job
* [FEATURE] Add varbit chunk encoding (higher compression, more CPU usage – disabled by default)
* [FEATURE] Add `holt_winters` query function
* [FEATURE] Add relative complement `unless` operator to PromQL
* [ENHANCEMENT] Quarantine series file if data corruption is encountered (instead of crashing)
* [ENHANCEMENT] Validate Alertmanager URL
* [ENHANCEMENT] Use UTC for build timestamp
* [ENHANCEMENT] Improve index query performance (especially for active time series)
* [ENHANCEMENT] Instrument configuration reload duration
* [ENHANCEMENT] Instrument retrieval layer
* [ENHANCEMENT] Add Go version to `prometheus_build_info` metric

## 0.17.0 / 2016-03-02

This version no longer works with Alertmanager 0.0.4 and earlier!
The alerting rule syntax has changed as well but the old syntax is supported
up until version 0.18.

All regular expressions in PromQL are anchored now, matching the behavior of
regular expressions in config files.

* [CHANGE] Integrate with Alertmanager 0.1.0 and higher
* [CHANGE] Degraded storage mode renamed to rushed mode
* [CHANGE] New alerting rule syntax
* [CHANGE] Add label validation on ingestion
* [CHANGE] Regular expression matchers in PromQL are anchored
* [FEATURE] Add `without` aggregation modifier
* [FEATURE] Send alert resolved notifications to Alertmanager
* [FEATURE] Allow millisecond precision in configuration file
* [FEATURE] Support AirBnB's Smartstack Nerve for service discovery
* [ENHANCEMENT] Storage switches less often between regular and rushed mode.
* [ENHANCEMENT] Storage switches into rushed mode if there are too many memory chunks.
* [ENHANCEMENT] Added more storage instrumentation
* [ENHANCEMENT] Improved instrumentation of notification handler
* [BUGFIX] Do not count head chunks as chunks waiting for persistence
* [BUGFIX] Handle OPTIONS HTTP requests to the API correctly
* [BUGFIX] Parsing of ranges in PromQL fixed
* [BUGFIX] Correctly validate URL flag parameters
* [BUGFIX] Log argument parse errors
* [BUGFIX] Properly handle creation of target with bad TLS config
* [BUGFIX] Fix of checkpoint timing issue

## 0.16.2 / 2016-01-18

* [FEATURE] Multiple authentication options for EC2 discovery added
* [FEATURE] Several meta labels for EC2 discovery added
* [FEATURE] Allow full URLs in static target groups (used e.g. by the `blackbox_exporter`)
* [FEATURE] Add Graphite remote-storage integration
* [FEATURE] Create separate Kubernetes targets for services and their endpoints
* [FEATURE] Add `clamp_{min,max}` functions to PromQL
* [FEATURE] Omitted time parameter in API query defaults to now
* [ENHANCEMENT] Less frequent time series file truncation
* [ENHANCEMENT] Instrument number of  manually deleted time series
* [ENHANCEMENT] Ignore lost+found directory during storage version detection
* [CHANGE] Kubernetes `masters` renamed to `api_servers`
* [CHANGE] "Healthy" and "unhealthy" targets are now called "up" and "down" in the web UI
* [CHANGE] Remove undocumented 2nd argument of the `delta` function.
  (This is a BREAKING CHANGE for users of the undocumented 2nd argument.)
* [BUGFIX] Return proper HTTP status codes on API errors
* [BUGFIX] Fix Kubernetes authentication configuration
* [BUGFIX] Fix stripped OFFSET from in rule evaluation and display
* [BUGFIX] Do not crash on failing Consul SD initialization
* [BUGFIX] Revert changes to metric auto-completion
* [BUGFIX] Add config overflow validation for TLS configuration
* [BUGFIX] Skip already watched Zookeeper nodes in serverset SD
* [BUGFIX] Don't federate stale samples
* [BUGFIX] Move NaN to end of result for `topk/bottomk/sort/sort_desc/min/max`
* [BUGFIX] Limit extrapolation of `delta/rate/increase`
* [BUGFIX] Fix unhandled error in rule evaluation

Some changes to the Kubernetes service discovery were integration since
it was released as a beta feature.

## 0.16.1 / 2015-10-16

* [FEATURE] Add `irate()` function.
* [ENHANCEMENT] Improved auto-completion in expression browser.
* [CHANGE] Kubernetes SD moves node label to instance label.
* [BUGFIX] Escape regexes in console templates.

## 0.16.0 / 2015-10-09

BREAKING CHANGES:

* Release tarballs now contain the built binaries in a nested directory.
* The `hash_mod` relabeling action now uses MD5 hashes instead of FNV hashes to
  achieve a better distribution.
* The DNS-SD meta label `__meta_dns_srv_name` was renamed to `__meta_dns_name`
  to reflect support for DNS record types other than `SRV`.
* The default full refresh interval for the file-based service discovery has been
  increased from 30 seconds to 5 minutes.
* In relabeling, parts of a source label that weren't matched by
  the specified regular expression are no longer included in the replacement
  output.
* Queries no longer interpolate between two data points. Instead, the resulting
  value will always be the latest value before the evaluation query timestamp.
* Regular expressions supplied via the configuration are now anchored to match
  full strings instead of substrings.
* Global labels are not appended upon storing time series anymore. Instead,
  they are only appended when communicating with external systems
  (Alertmanager, remote storages, federation). They have thus also been renamed
  from `global.labels` to `global.external_labels`.
* The names and units of metrics related to remote storage sample appends have
  been changed.
* The experimental support for writing to InfluxDB has been updated to work
  with InfluxDB 0.9.x. 0.8.x versions of InfluxDB are not supported anymore.
* Escape sequences in double- and single-quoted string literals in rules or query
  expressions are now interpreted like escape sequences in Go string literals
  (https://golang.org/ref/spec#String_literals).

Future breaking changes / deprecated features:

* The `delta()` function had an undocumented optional second boolean argument
  to make it behave like `increase()`. This second argument will be removed in
  the future. Migrate any occurrences of `delta(x, 1)` to use `increase(x)`
  instead.
* Support for filter operators between two scalar values (like `2 > 1`) will be
  removed in the future. These will require a `bool` modifier on the operator,
  e.g.  `2 > bool 1`.

All changes:

* [CHANGE] Renamed `global.labels` to `global.external_labels`.
* [CHANGE] Vendoring is now done via govendor instead of godep.
* [CHANGE] Change web UI root page to show the graphing interface instead of
  the server status page.
* [CHANGE] Append global labels only when communicating with external systems
  instead of storing them locally.
* [CHANGE] Change all regexes in the configuration to do full-string matches
  instead of substring matches.
* [CHANGE] Remove interpolation of vector values in queries.
* [CHANGE] For alert `SUMMARY`/`DESCRIPTION` template fields, cast the alert
  value to `float64` to work with common templating functions.
* [CHANGE] In relabeling, don't include unmatched source label parts in the
  replacement.
* [CHANGE] Change default full refresh interval for the file-based service
  discovery from 30 seconds to 5 minutes.
* [CHANGE] Rename the DNS-SD meta label `__meta_dns_srv_name` to
  `__meta_dns_name` to reflect support for other record types than `SRV`.
* [CHANGE] Release tarballs now contain the binaries in a nested directory.
* [CHANGE] Update InfluxDB write support to work with InfluxDB 0.9.x.
* [FEATURE] Support full "Go-style" escape sequences in strings and add raw
  string literals.
* [FEATURE] Add EC2 service discovery support.
* [FEATURE] Allow configuring TLS options in scrape configurations.
* [FEATURE] Add instrumentation around configuration reloads.
* [FEATURE] Add `bool` modifier to comparison operators to enable boolean
  (`0`/`1`) output instead of filtering.
* [FEATURE] In Zookeeper serverset discovery, provide `__meta_serverset_shard`
  label with the serverset shard number.
* [FEATURE] Provide `__meta_consul_service_id` meta label in Consul service
  discovery.
* [FEATURE] Allow scalar expressions in recording rules to enable use cases
  such as building constant metrics.
* [FEATURE] Add `label_replace()` and `vector()` query language functions.
* [FEATURE] In Consul service discovery, fill in the `__meta_consul_dc`
  datacenter label from the Consul agent when it's not set in the Consul SD
  config.
* [FEATURE] Scrape all services upon empty services list in Consul service
  discovery.
* [FEATURE] Add `labelmap` relabeling action to map a set of input labels to a
  set of output labels using regular expressions.
* [FEATURE] Introduce `__tmp` as a relabeling label prefix that is guaranteed
  to not be used by Prometheus internally.
* [FEATURE] Kubernetes-based service discovery.
* [FEATURE] Marathon-based service discovery.
* [FEATURE] Support multiple series names in console graphs JavaScript library.
* [FEATURE] Allow reloading configuration via web handler at `/-/reload`.
* [FEATURE] Updates to promtool to reflect new Prometheus configuration
  features.
* [FEATURE] Add `proxy_url` parameter to scrape configurations to enable use of
  proxy servers.
* [FEATURE] Add console templates for Prometheus itself.
* [FEATURE] Allow relabeling the protocol scheme of targets.
* [FEATURE] Add `predict_linear()` query language function.
* [FEATURE] Support for authentication using bearer tokens, client certs, and
  CA certs.
* [FEATURE] Implement unary expressions for vector types (`-foo`, `+foo`).
* [FEATURE] Add console templates for the SNMP exporter.
* [FEATURE] Make it possible to relabel target scrape query parameters.
* [FEATURE] Add support for `A` and `AAAA` records in DNS service discovery.
* [ENHANCEMENT] Fix several flaky tests.
* [ENHANCEMENT] Switch to common routing package.
* [ENHANCEMENT] Use more resilient metric decoder.
* [ENHANCEMENT] Update vendored dependencies.
* [ENHANCEMENT] Add compression to more HTTP handlers.
* [ENHANCEMENT] Make -web.external-url flag help string more verbose.
* [ENHANCEMENT] Improve metrics around remote storage queues.
* [ENHANCEMENT] Use Go 1.5.1 instead of Go 1.4.2 in builds.
* [ENHANCEMENT] Update the architecture diagram in the `README.md`.
* [ENHANCEMENT] Time out sample appends in retrieval layer if the storage is
  backlogging.
* [ENHANCEMENT] Make `hash_mod` relabeling action use MD5 instead of FNV to
  enable better hash distribution.
* [ENHANCEMENT] Better tracking of targets between same service discovery
  mechanisms in one scrape configuration.
* [ENHANCEMENT] Handle parser and query evaluation runtime panics more
  gracefully.
* [ENHANCEMENT] Add IDs to H2 tags on status page to allow anchored linking.
* [BUGFIX] Fix watching multiple paths with Zookeeper serverset discovery.
* [BUGFIX] Fix high CPU usage on configuration reload.
* [BUGFIX] Fix disappearing `__params` on configuration reload.
* [BUGFIX] Make `labelmap` action available through configuration.
* [BUGFIX] Fix direct access of protobuf fields.
* [BUGFIX] Fix panic on Consul request error.
* [BUGFIX] Redirect of graph endpoint for prefixed setups.
* [BUGFIX] Fix series file deletion behavior when purging archived series.
* [BUGFIX] Fix error checking and logging around checkpointing.
* [BUGFIX] Fix map initialization in target manager.
* [BUGFIX] Fix draining of file watcher events in file-based service discovery.
* [BUGFIX] Add `POST` handler for `/debug` endpoints to fix CPU profiling.
* [BUGFIX] Fix several flaky tests.
* [BUGFIX] Fix busylooping in case a scrape configuration has no target
  providers defined.
* [BUGFIX] Fix exit behavior of static target provider.
* [BUGFIX] Fix configuration reloading loop upon shutdown.
* [BUGFIX] Add missing check for nil expression in expression parser.
* [BUGFIX] Fix error handling bug in test code.
* [BUGFIX] Fix Consul port meta label.
* [BUGFIX] Fix lexer bug that treated non-Latin Unicode digits as digits.
* [CLEANUP] Remove obsolete federation example from console templates.
* [CLEANUP] Remove duplicated Bootstrap JS inclusion on graph page.
* [CLEANUP] Switch to common log package.
* [CLEANUP] Update build environment scripts and Makefiles to work better with
  native Go build mechanisms and new Go 1.5 experimental vendoring support.
* [CLEANUP] Remove logged notice about 0.14.x configuration file format change.
* [CLEANUP] Move scrape-time metric label modification into SampleAppenders.
* [CLEANUP] Switch from `github.com/client_golang/model` to
  `github.com/common/model` and related type cleanups.
* [CLEANUP] Switch from `github.com/client_golang/extraction` to
  `github.com/common/expfmt` and related type cleanups.
* [CLEANUP] Exit Prometheus when the web server encounters a startup error.
* [CLEANUP] Remove non-functional alert-silencing links on alerting page.
* [CLEANUP] General cleanups to comments and code, derived from `golint`,
  `go vet`, or otherwise.
* [CLEANUP] When entering crash recovery, tell users how to cleanly shut down
  Prometheus.
* [CLEANUP] Remove internal support for multi-statement queries in query engine.
* [CLEANUP] Update AUTHORS.md.
* [CLEANUP] Don't warn/increment metric upon encountering equal timestamps for
  the same series upon append.
* [CLEANUP] Resolve relative paths during configuration loading.

## 0.15.1 / 2015-07-27
* [BUGFIX] Fix vector matching behavior when there is a mix of equality and
  non-equality matchers in a vector selector and one matcher matches no series.
* [ENHANCEMENT] Allow overriding `GOARCH` and `GOOS` in Makefile.INCLUDE.
* [ENHANCEMENT] Update vendored dependencies.

## 0.15.0 / 2015-07-21

BREAKING CHANGES:

* Relative paths for rule files are now evaluated relative to the config file.
* External reachability flags (`-web.*`) consolidated.
* The default storage directory has been changed from `/tmp/metrics`
  to `data` in the local directory.
* The `rule_checker` tool has been replaced by `promtool` with
  different flags and more functionality.
* Empty labels are now removed upon ingestion into the
  storage. Matching empty labels is now equivalent to matching unset
  labels (`mymetric{label=""}` now matches series that don't have
  `label` set at all).
* The special `__meta_consul_tags` label in Consul service discovery
  now starts and ends with tag separators to enable easier regex
  matching.
* The default scrape interval has been changed back from 1 minute to
  10 seconds.

All changes:

* [CHANGE] Change default storage directory to `data` in the current
  working directory.
* [CHANGE] Consolidate external reachability flags (`-web.*`)into one.
* [CHANGE] Deprecate `keeping_extra` modifier keyword, rename it to
  `keep_common`.
* [CHANGE] Improve label matching performance and treat unset labels
  like empty labels in label matchers.
* [CHANGE] Remove `rule_checker` tool and add generic `promtool` CLI
  tool which allows checking rules and configuration files.
* [CHANGE] Resolve rule files relative to config file.
* [CHANGE] Restore default ScrapeInterval of 1 minute instead of 10 seconds.
* [CHANGE] Surround `__meta_consul_tags` value with tag separators.
* [CHANGE] Update node disk console for new filesystem labels.
* [FEATURE] Add Consul's `ServiceAddress`, `Address`, and `ServicePort` as
  meta labels to enable setting a custom scrape address if needed.
* [FEATURE] Add `hashmod` relabel action to allow for horizontal
  sharding of Prometheus servers.
* [FEATURE] Add `honor_labels` scrape configuration option to not
  overwrite any labels exposed by the target.
* [FEATURE] Add basic federation support on `/federate`.
* [FEATURE] Add optional `RUNBOOK` field to alert statements.
* [FEATURE] Add pre-relabel target labels to status page.
* [FEATURE] Add version information endpoint under `/version`.
* [FEATURE] Added initial stable API version 1 under `/api/v1`,
  including ability to delete series and query more metadata.
* [FEATURE] Allow configuring query parameters when scraping metrics endpoints.
* [FEATURE] Allow deleting time series via the new v1 API.
* [FEATURE] Allow individual ingested metrics to be relabeled.
* [FEATURE] Allow loading rule files from an entire directory.
* [FEATURE] Allow scalar expressions in range queries, improve error messages.
* [FEATURE] Support Zookeeper Serversets as a service discovery mechanism.
* [ENHANCEMENT] Add circleci yaml for Dockerfile test build.
* [ENHANCEMENT] Always show selected graph range, regardless of available data.
* [ENHANCEMENT] Change expression input field to multi-line textarea.
* [ENHANCEMENT] Enforce strict monotonicity of time stamps within a series.
* [ENHANCEMENT] Export build information as metric.
* [ENHANCEMENT] Improve UI of `/alerts` page.
* [ENHANCEMENT] Improve display of target labels on status page.
* [ENHANCEMENT] Improve initialization and routing functionality of web service.
* [ENHANCEMENT] Improve target URL handling and display.
* [ENHANCEMENT] New dockerfile using alpine-glibc base image and make.
* [ENHANCEMENT] Other minor fixes.
* [ENHANCEMENT] Preserve alert state across reloads.
* [ENHANCEMENT] Prettify flag help output even more.
* [ENHANCEMENT] README.md updates.
* [ENHANCEMENT] Raise error on unknown config parameters.
* [ENHANCEMENT] Refine v1 HTTP API output.
* [ENHANCEMENT] Show original configuration file contents on status
  page instead of serialized YAML.
* [ENHANCEMENT] Start HUP signal handler earlier to not exit upon HUP
  during startup.
* [ENHANCEMENT] Updated vendored dependencies.
* [BUGFIX] Do not panic in `StringToDuration()` on wrong duration unit.
* [BUGFIX] Exit on invalid rule files on startup.
* [BUGFIX] Fix a regression in the `.Path` console template variable.
* [BUGFIX] Fix chunk descriptor loading.
* [BUGFIX] Fix consoles "Prometheus" link to point to /
* [BUGFIX] Fix empty configuration file cases
* [BUGFIX] Fix float to int conversions in chunk encoding, which were
  broken for some architectures.
* [BUGFIX] Fix overflow detection for serverset config.
* [BUGFIX] Fix race conditions in retrieval layer.
* [BUGFIX] Fix shutdown deadlock in Consul SD code.
* [BUGFIX] Fix the race condition targets in the Makefile.
* [BUGFIX] Fix value display error in web console.
* [BUGFIX] Hide authentication credentials in config `String()` output.
* [BUGFIX] Increment dirty counter metric in storage only if
  `setDirty(true)` is called.
* [BUGFIX] Periodically refresh services in Consul to recover from
  missing events.
* [BUGFIX] Prevent overwrite of default global config when loading a
  configuration.
* [BUGFIX] Properly lex `\r` as whitespace in expression language.
* [BUGFIX] Validate label names in JSON target groups.
* [BUGFIX] Validate presence of regex field in relabeling configurations.
* [CLEANUP] Clean up initialization of remote storage queues.
* [CLEANUP] Fix `go vet` and `golint` violations.
* [CLEANUP] General cleanup of rules and query language code.
* [CLEANUP] Improve and simplify Dockerfile build steps.
* [CLEANUP] Improve and simplify build infrastructure, use go-bindata
  for web assets. Allow building without git.
* [CLEANUP] Move all utility packages into common `util` subdirectory.
* [CLEANUP] Refactor main, flag handling, and web package.
* [CLEANUP] Remove unused methods from `Rule` interface.
* [CLEANUP] Simplify default config handling.
* [CLEANUP] Switch human-readable times on web UI to UTC.
* [CLEANUP] Use `templates.TemplateExpander` for all page templates.
* [CLEANUP] Use new v1 HTTP API for querying and graphing.

## 0.14.0 / 2015-06-01
* [CHANGE] Configuration format changed and switched to YAML.
  (See the provided [migration tool](https://github.com/prometheus/migrate/releases).)
* [ENHANCEMENT] Redesign of state-preserving target discovery.
* [ENHANCEMENT] Allow specifying scrape URL scheme and basic HTTP auth for non-static targets.
* [FEATURE] Allow attaching meaningful labels to targets via relabeling.
* [FEATURE] Configuration/rule reloading at runtime.
* [FEATURE] Target discovery via file watches.
* [FEATURE] Target discovery via Consul.
* [ENHANCEMENT] Simplified binary operation evaluation.
* [ENHANCEMENT] More stable component initialization.
* [ENHANCEMENT] Added internal expression testing language.
* [BUGFIX] Fix graph links with path prefix.
* [ENHANCEMENT] Allow building from source without git.
* [ENHANCEMENT] Improve storage iterator performance.
* [ENHANCEMENT] Change logging output format and flags.
* [BUGFIX] Fix memory alignment bug for 32bit systems.
* [ENHANCEMENT] Improve web redirection behavior.
* [ENHANCEMENT] Allow overriding default hostname for Prometheus URLs.
* [BUGFIX] Fix double slash in URL sent to alertmanager.
* [FEATURE] Add resets() query function to count counter resets.
* [FEATURE] Add changes() query function to count the number of times a gauge changed.
* [FEATURE] Add increase() query function to calculate a counter's increase.
* [ENHANCEMENT] Limit retrievable samples to the storage's retention window.

## 0.13.4 / 2015-05-23
* [BUGFIX] Fix a race while checkpointing fingerprint mappings.

## 0.13.3 / 2015-05-11
* [BUGFIX] Handle fingerprint collisions properly.
* [CHANGE] Comments in rules file must start with `#`. (The undocumented `//`
  and `/*...*/` comment styles are no longer supported.)
* [ENHANCEMENT] Switch to custom expression language parser and evaluation
  engine, which generates better error messages, fixes some parsing edge-cases,
  and enables other future enhancements (like the ones below).
* [ENHANCEMENT] Limit maximum number of concurrent queries.
* [ENHANCEMENT] Terminate running queries during shutdown.

## 0.13.2 / 2015-05-05
* [MAINTENANCE] Updated vendored dependencies to their newest versions.
* [MAINTENANCE] Include rule_checker and console templates in release tarball.
* [BUGFIX] Sort NaN as the lowest value.
* [ENHANCEMENT] Add square root, stddev and stdvar functions.
* [BUGFIX] Use scrape_timeout for scrape timeout, not scrape_interval.
* [ENHANCEMENT] Improve chunk and chunkDesc loading, increase performance when
  reading from disk.
* [BUGFIX] Show correct error on wrong DNS response.

## 0.13.1 / 2015-04-09
* [BUGFIX] Treat memory series with zero chunks correctly in series maintenance.
* [ENHANCEMENT] Improve readability of usage text even more.

## 0.13.0 / 2015-04-08
* [ENHANCEMENT] Double-delta encoding for chunks, saving typically 40% of
  space, both in RAM and on disk.
* [ENHANCEMENT] Redesign of chunk persistence queuing, increasing performance
  on spinning disks significantly.
* [ENHANCEMENT] Redesign of sample ingestion, increasing ingestion performance.
* [FEATURE] Added ln, log2, log10 and exp functions to the query language.
* [FEATURE] Experimental write support to InfluxDB.
* [FEATURE] Allow custom timestamps in instant query API.
* [FEATURE] Configurable path prefix for URLs to support proxies.
* [ENHANCEMENT] Increase of rule_checker CLI usability.
* [CHANGE] Show special float values as gaps.
* [ENHANCEMENT] Made usage output more readable.
* [ENHANCEMENT] Increased resilience of the storage against data corruption.
* [ENHANCEMENT] Various improvements around chunk encoding.
* [ENHANCEMENT] Nicer formatting of target health table on /status.
* [CHANGE] Rename UNREACHABLE to UNHEALTHY, ALIVE to HEALTHY.
* [BUGFIX] Strip trailing slash in alertmanager URL.
* [BUGFIX] Avoid +InfYs and similar, just display +Inf.
* [BUGFIX] Fixed HTML-escaping at various places.
* [BUGFIX] Fixed special value handling in division and modulo of the query
  language.
* [BUGFIX] Fix embed-static.sh.
* [CLEANUP] Added intial HTTP API tests.
* [CLEANUP] Misc. other code cleanups.
* [MAINTENANCE] Updated vendored dependcies to their newest versions.

## 0.12.0 / 2015-03-04
* [CHANGE] Use client_golang v0.3.1. THIS CHANGES FINGERPRINTING AND INVALIDATES
  ALL PERSISTED FINGERPRINTS. You have to wipe your storage to use this or
  later versions. There is a version guard in place that will prevent you to
  run Prometheus with the stored data of an older Prometheus.
* [BUGFIX] The change above fixes a weakness in the fingerprinting algorithm.
* [ENHANCEMENT] The change above makes fingerprinting faster and less allocation
  intensive.
* [FEATURE] OR operator and vector matching options. See docs for details.
* [ENHANCEMENT] Scientific notation and special float values (Inf, NaN) now
  supported by the expression language.
* [CHANGE] Dockerfile makes Prometheus use the Docker volume to store data
  (rather than /tmp/metrics).
* [CHANGE] Makefile uses Go 1.4.2.

## 0.11.1 / 2015-02-27
* [BUGFIX] Make series maintenance complete again. (Ever since 0.9.0rc4,
  or commit 0851945, series would not be archived, chunk descriptors would
  not be evicted, and stale head chunks would never be closed. This happened
  due to accidental deletion of a line calling a (well tested :) function.
* [BUGFIX] Do not double count head chunks read from checkpoint on startup.
  Also fix a related but less severe bug in counting chunk descriptors.
* [BUGFIX] Check last time in head chunk for head chunk timeout, not first.
* [CHANGE] Update vendoring due to vendoring changes in client_golang.
* [CLEANUP] Code cleanups.
* [ENHANCEMENT] Limit the number of 'dirty' series counted during checkpointing.

## 0.11.0 / 2015-02-23
* [FEATURE] Introduce new metric type Histogram with server-side aggregation.
* [FEATURE] Add offset operator.
* [FEATURE] Add floor, ceil and round functions.
* [CHANGE] Change instance identifiers to be host:port.
* [CHANGE] Dependency management and vendoring changed/improved.
* [CHANGE] Flag name changes to create consistency between various Prometheus
  binaries.
* [CHANGE] Show unlimited number of metrics in autocomplete.
* [CHANGE] Add query timeout.
* [CHANGE] Remove labels on persist error counter.
* [ENHANCEMENT] Various performance improvements for sample ingestion.
* [ENHANCEMENT] Various Makefile improvements.
* [ENHANCEMENT] Various console template improvements, including
  proof-of-concept for federation via console templates.
* [ENHANCEMENT] Fix graph JS glitches and simplify graphing code.
* [ENHANCEMENT] Dramatically decrease resources for file embedding.
* [ENHANCEMENT] Crash recovery saves lost series data in 'orphaned' directory.
* [BUGFIX] Fix aggregation grouping key calculation.
* [BUGFIX] Fix Go download path for various architectures.
* [BUGFIX] Fixed the link of the Travis build status image.
* [BUGFIX] Fix Rickshaw/D3 version mismatch.
* [CLEANUP] Various code cleanups.

## 0.10.0 / 2015-01-26
* [CHANGE] More efficient JSON result format in query API. This requires
  up-to-date versions of PromDash and prometheus_cli, too.
* [ENHANCEMENT] Excluded non-minified Bootstrap assets and the Bootstrap maps
  from embedding into the binary. Those files are only used for debugging,
  and then you can use -web.use-local-assets. By including fewer files, the
  RAM usage during compilation is much more manageable.
* [ENHANCEMENT] Help link points to http://prometheus.github.io now.
* [FEATURE] Consoles for haproxy and cloudwatch.
* [BUGFIX] Several fixes to graphs in consoles.
* [CLEANUP] Removed a file size check that did not check anything.

## 0.9.0 / 2015-01-23
* [CHANGE] Reworked command line flags, now more consistent and taking into
  account needs of the new storage backend (see below).
* [CHANGE] Metric names are dropped after certain transformations.
* [CHANGE] Changed partitioning of summary metrics exported by Prometheus.
* [CHANGE] Got rid of Gerrit as a review tool.
* [CHANGE] 'Tabular' view now the default (rather than 'Graph') to avoid
  running very expensive queries accidentally.
* [CHANGE] On-disk format for stored samples changed. For upgrading, you have
  to nuke your old files completely. See "Complete rewrite of the storage
* [CHANGE] Removed 2nd argument from `delta`.
* [FEATURE] Added a `deriv` function.
* [FEATURE] Console templates.
* [FEATURE] Added `absent` function.
* [FEATURE] Allow omitting the metric name in queries.
* [BUGFIX] Removed all known race conditions.
* [BUGFIX] Metric mutations now handled correctly in all cases.
* [ENHANCEMENT] Proper double-start protection.
* [ENHANCEMENT] Complete rewrite of the storage layer. Benefits include:
  * Better query performance.
  * More samples in less RAM.
  * Better memory management.
  * Scales up to millions of time series and thousands of samples ingested
    per second.
  * Purging of obsolete samples much cleaner now, up to completely
    "forgetting" obsolete time series.
  * Proper instrumentation to diagnose the storage layer with... well...
    Prometheus.
  * Pure Go implementation, no need for cgo and shared C libraries anymore.
  * Better concurrency.
* [ENHANCEMENT] Copy-on-write semantics in the AST layer.
* [ENHANCEMENT] Switched from Go 1.3 to Go 1.4.
* [ENHANCEMENT] Vendored external dependencies with godeps.
* [ENHANCEMENT] Numerous Web UI improvements, moved to Bootstrap3 and
  Rickshaw 1.5.1.
* [ENHANCEMENT] Improved Docker integration.
* [ENHANCEMENT] Simplified the Makefile contraption.
* [CLEANUP] Put meta-data files into proper shape (LICENSE, README.md etc.)
* [CLEANUP] Removed all legitimate 'go vet' and 'golint' warnings.
* [CLEANUP] Removed dead code.

## 0.8.0 / 2014-09-04
* [ENHANCEMENT] Stagger scrapes to spread out load.
* [BUGFIX] Correctly quote HTTP Accept header.

## 0.7.0 / 2014-08-06
* [FEATURE] Added new functions: abs(), topk(), bottomk(), drop_common_labels().
* [FEATURE] Let console templates get graph links from expressions.
* [FEATURE] Allow console templates to dynamically include other templates.
* [FEATURE] Template consoles now have access to their URL.
* [BUGFIX] Fixed time() function to return evaluation time, not wallclock time.
* [BUGFIX] Fixed HTTP connection leak when targets returned a non-200 status.
* [BUGFIX] Fixed link to console templates in UI.
* [PERFORMANCE] Removed extra memory copies while scraping targets.
* [ENHANCEMENT] Switched from Go 1.2.1 to Go 1.3.
* [ENHANCEMENT] Made metrics exported by Prometheus itself more consistent.
* [ENHANCEMENT] Removed incremental backoffs for unhealthy targets.
* [ENHANCEMENT] Dockerfile also builds Prometheus support tools now.

## 0.6.0 / 2014-06-30
* [FEATURE] Added console and alert templates support, along with various template functions.
* [PERFORMANCE] Much faster and more memory-efficient flushing to disk.
* [ENHANCEMENT] Query results are now only logged when debugging.
* [ENHANCEMENT] Upgraded to new Prometheus client library for exposing metrics.
* [BUGFIX] Samples are now kept in memory until fully flushed to disk.
* [BUGFIX] Non-200 target scrapes are now treated as an error.
* [BUGFIX] Added installation step for missing dependency to Dockerfile.
* [BUGFIX] Removed broken and unused "User Dashboard" link.

## 0.5.0 / 2014-05-28

* [BUGFIX] Fixed next retrieval time display on status page.
* [BUGFIX] Updated some variable references in tools subdir.
* [FEATURE] Added support for scraping metrics via the new text format.
* [PERFORMANCE] Improved label matcher performance.
* [PERFORMANCE] Removed JSON indentation in query API, leading to smaller response sizes.
* [ENHANCEMENT] Added internal check to verify temporal order of streams.
* [ENHANCEMENT] Some internal refactorings.

## 0.4.0 / 2014-04-17

* [FEATURE] Vectors and scalars may now be reversed in binary operations (`<scalar> <binop> <vector>`).
* [FEATURE] It's possible to shutdown Prometheus via a `/-/quit` web endpoint now.
* [BUGFIX] Fix for a deadlock race condition in the memory storage.
* [BUGFIX] Mac OS X build fixed.
* [BUGFIX] Built from Go 1.2.1, which has internal fixes to race conditions in garbage collection handling.
* [ENHANCEMENT] Internal storage interface refactoring that allows building e.g. the `rule_checker` tool without LevelDB dynamic library dependencies.
* [ENHANCEMENT] Cleanups around shutdown handling.
* [PERFORMANCE] Preparations for better memory reuse during marshalling / unmarshalling.

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

ALL CHANGES:

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
* [MAINTENANCE] Updated vendored dependcies to their newest versions.
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

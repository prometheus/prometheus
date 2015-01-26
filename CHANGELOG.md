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

## 2.13.0 / 2019-10-04

* [SECURITY/BUGFIX] UI: Fix a Stored DOM XSS vulnerability with query history [CVE-2019-10215](http://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2019-10215). #6098
* [CHANGE] Metrics: renamed prometheus_sd_configs_failed_total to prometheus_sd_failed_configs and changed to Gauge #5254
* [ENHANCEMENT] Include the tsdb tool in builds. #6089
* [ENHANCEMENT] Service discovery: add new node address types for kubernetes. #5902
* [ENHANCEMENT] UI: show warnings if query have returned some warnings. #5964
* [ENHANCEMENT] Remote write: reduce memory usage of the series cache. #5849
* [ENHANCEMENT] Remote read: use remote read streaming to reduce memory usage. #5703
* [ENHANCEMENT] Metrics: added metrics for remote write max/min/desired shards to queue manager. #5787
* [ENHANCEMENT] Promtool: show the warnings during label query. #5924
* [ENHANCEMENT] Promtool: improve error messages when parsing bad rules. #5965
* [ENHANCEMENT] Promtool: more promlint rules. #5515
* [BUGFIX] Promtool: fix recording inconsistency due to duplicate labels. #6026
* [BUGFIX] UI: fixes service-discovery view when accessed from unhealthy targets. #5915
* [BUGFIX] Metrics format: OpenMetrics parser crashes on short input. #5939
* [BUGFIX] UI: avoid truncated Y-axis values. #6014

## 2.12.0 / 2019-08-17

* [FEATURE] Track currently active PromQL queries in a log file. #5794
* [FEATURE] Enable and provide binaries for `mips64` / `mips64le` architectures. #5792
* [ENHANCEMENT] Improve responsiveness of targets web UI and API endpoint. #5740
* [ENHANCEMENT] Improve remote write desired shards calculation. #5763
* [ENHANCEMENT] Flush TSDB pages more precisely. tsdb#660
* [ENHANCEMENT] Add `prometheus_tsdb_retention_limit_bytes` metric. tsdb#667
* [ENHANCEMENT] Add logging during TSDB WAL replay on startup. tsdb#662
* [ENHANCEMENT] Improve TSDB memory usage. tsdb#653, tsdb#643, tsdb#654, tsdb#642, tsdb#627
* [BUGFIX] Check for duplicate label names in remote read. #5829
* [BUGFIX] Mark deleted rules' series as stale on next evaluation. #5759
* [BUGFIX] Fix JavaScript error when showing warning about out-of-sync server time. #5833
* [BUGFIX] Fix `promtool test rules` panic when providing empty `exp_labels`. #5774
* [BUGFIX] Only check last directory when discovering checkpoint number. #5756
* [BUGFIX] Fix error propagation in WAL watcher helper functions. #5741
* [BUGFIX] Correctly handle empty labels from alert templates. #5845

## 2.11.1 / 2019-07-10

* [BUGFIX] Fix potential panic when prometheus is watching multiple zookeeper paths. #5749

## 2.11.0 / 2019-07-09

* [CHANGE] Remove `max_retries` from queue_config (it has been unused since rewriting remote-write to utilize the write-ahead-log). #5649
* [CHANGE] The meta file `BlockStats` no longer holds size information. This is now dynamically calculated and kept in memory. It also includes the meta file size which was not included before. tsdb#637
* [CHANGE] Renamed metric from `prometheus_tsdb_wal_reader_corruption_errors` to `prometheus_tsdb_wal_reader_corruption_errors_total`. tsdb#622
* [FEATURE] Add option to use Alertmanager API v2. #5482
* [FEATURE] Added `humanizePercentage` function for templates. #5670
* [FEATURE] Include InitContainers in Kubernetes Service Discovery. #5598
* [FEATURE] Provide option to compress WAL records using Snappy. [#609](https://github.com/prometheus/tsdb/pull/609)
* [ENHANCEMENT] Create new clean segment when starting the WAL. tsdb#608
* [ENHANCEMENT] Reduce allocations in PromQL aggregations. #5641
* [ENHANCEMENT] Add storage warnings to LabelValues and LabelNames API results. #5673
* [ENHANCEMENT] Add `prometheus_http_requests_total` metric. #5640
* [ENHANCEMENT] Enable openbsd/arm build. #5696
* [ENHANCEMENT] Remote-write allocation improvements. #5614
* [ENHANCEMENT] Query performance improvement: Efficient iteration and search in HashForLabels and HashWithoutLabels. #5707
* [ENHANCEMENT] Allow injection of arbitrary headers in promtool. #4389
* [ENHANCEMENT] Allow passing `external_labels` in alert unit tests groups. #5608
* [ENHANCEMENT] Allows globs for rules when unit testing. #5595
* [ENHANCEMENT] Improved postings intersection matching. tsdb#616
* [ENHANCEMENT] Reduced disk usage for WAL for small setups. tsdb#605
* [ENHANCEMENT] Optimize queries using regexp for set lookups. tsdb#602
* [BUGFIX] resolve race condition in maxGauge. #5647
* [BUGFIX] Fix ZooKeeper connection leak. #5675
* [BUGFIX] Improved atomicity of .tmp block replacement during compaction for usual case. tsdb#636
* [BUGFIX] Fix "unknown series references" after clean shutdown. tsdb#623
* [BUGFIX] Re-calculate block size when calling `block.Delete`. tsdb#637
* [BUGFIX] Fix unsafe snapshots with head block. tsdb#641
* [BUGFIX] `prometheus_tsdb_compactions_failed_total` is now incremented on any compaction failure. tsdb#613

## 2.10.0 / 2019-05-25

* [CHANGE/BUGFIX] API: Encode alert values as string to correctly represent Inf/NaN. #5582
* [FEATURE] Template expansion: Make external labels available as `$externalLabels` in alert and console template expansion. #5463
* [FEATURE] TSDB: Add `prometheus_tsdb_wal_segment_current` metric for the WAL segment index that TSDB is currently writing to. tsdb#601
* [FEATURE] Scrape: Add `scrape_series_added` per-scrape metric. #5546
* [ENHANCEMENT] Discovery/kubernetes: Add labels `__meta_kubernetes_endpoint_node_name` and `__meta_kubernetes_endpoint_hostname`. #5571
* [ENHANCEMENT] Discovery/azure: Add label `__meta_azure_machine_public_ip`. #5475
* [ENHANCEMENT] TSDB: Simplify mergedPostings.Seek, resulting in better performance if there are many posting lists. tsdb#595
* [ENHANCEMENT] Log filesystem type on startup. #5558
* [ENHANCEMENT] Cmd/promtool: Use POST requests for Query and QueryRange. client_golang#557
* [ENHANCEMENT] Web: Sort alerts by group name. #5448
* [ENHANCEMENT] Console templates: Add convenience variables `$rawParams`, `$params`, `$path`. #5463
* [BUGFIX] TSDB: Don't panic when running out of disk space and recover nicely from the condition. tsdb#582
* [BUGFIX] TSDB: Correctly handle empty labels. tsdb#594
* [BUGFIX] TSDB: Don't crash on an unknown tombstone reference. tsdb#604
* [BUGFIX] Storage/remote: Remove queue-manager specific metrics if queue no longer exists. #5445 #5485 #5555
* [BUGFIX] PromQL: Correctly display `{__name__="a"}`. #5552
* [BUGFIX] Discovery/kubernetes: Use `service` rather than `ingress` as the name for the service workqueue. #5520
* [BUGFIX] Discovery/azure: Don't panic on a VM with a public IP. #5587
* [BUGFIX] Discovery/triton: Always read HTTP body to completion. #5596
* [BUGFIX] Web: Fixed Content-Type for js and css instead of using `/etc/mime.types`. #5551

## 2.9.2 / 2019-04-24

* [BUGFIX] Make sure subquery range is taken into account for selection #5467
* [BUGFIX] Exhaust every request body before closing it #5166
* [BUGFIX] Cmd/promtool: return errors from rule evaluations #5483
* [BUGFIX] Remote Storage: string interner should not panic in release #5487
* [BUGFIX] Fix memory allocation regression in mergedPostings.Seek tsdb#586

## 2.9.1 / 2019-04-16

* [BUGFIX] Discovery/kubernetes: fix missing label sanitization #5462
* [BUGFIX] Remote_write: Prevent reshard concurrent with calling stop #5460

## 2.9.0 / 2019-04-15

This releases uses Go 1.12, which includes a change in how memory is released
to Linux. This will cause RSS to be reported as higher, however this is harmless
and the memory is available to the kernel when it needs it.

* [CHANGE/ENHANCEMENT] Update Consul to support catalog.ServiceMultipleTags. #5151
* [FEATURE] Add honor_timestamps scrape option. #5304
* [ENHANCEMENT] Discovery/kubernetes: add present labels for labels/annotations. #5443
* [ENHANCEMENT] OpenStack SD: Add ProjectID and UserID meta labels. #5431
* [ENHANCEMENT] Add GODEBUG and retention to the runtime page. #5324 #5322
* [ENHANCEMENT] Add support for POSTing to /series endpoint. #5422
* [ENHANCEMENT] Support PUT methods for Lifecycle and Admin APIs. #5376
* [ENHANCEMENT] Scrape: Add global jitter for HA server. #5181
* [ENHANCEMENT] Check for cancellation on every step of a range evaluation. #5131
* [ENHANCEMENT] String interning for labels & values in the remote_write path. #5316
* [ENHANCEMENT] Don't lose the scrape cache on a failed scrape. #5414
* [ENHANCEMENT] Reload cert files from disk automatically. common#173
* [ENHANCEMENT] Use fixed length millisecond timestamp format for logs. common#172
* [ENHANCEMENT] Performance improvements for postings. tsdb#509 tsdb#572
* [BUGFIX] Remote Write: fix checkpoint reading. #5429
* [BUGFIX] Check if label value is valid when unmarshaling external labels from YAML. #5316
* [BUGFIX] Promparse: sort all labels when parsing. #5372
* [BUGFIX] Reload rules: copy state on both name and labels. #5368
* [BUGFIX] Exponentiation operator to drop metric name in result of operation. #5329
* [BUGFIX] Config: resolve more file paths. #5284
* [BUGFIX] Promtool: resolve relative paths in alert test files. #5336
* [BUGFIX] Set TLSHandshakeTimeout in HTTP transport. common#179
* [BUGFIX] Use fsync to be more resilient to machine crashes. tsdb#573 tsdb#578
* [BUGFIX] Keep series that are still in WAL in checkpoints. tsdb#577
* [BUGFIX] Fix output sample values for scalar-to-vector comparison operations. #5454

## 2.8.1 / 2019-03-28

* [BUGFIX] Display the job labels in `/targets` which was removed accidentally. #5406

## 2.8.0 / 2019-03-12

This release uses Write-Ahead Logging (WAL) for the remote_write API. This currently causes a slight increase in memory usage, which will be addressed in future releases.

* [CHANGE] Default time retention is used only when no size based retention is specified. These are flags where time retention is specified by the flag `--storage.tsdb.retention` and size retention by `--storage.tsdb.retention.size`. #5216
* [CHANGE] `prometheus_tsdb_storage_blocks_bytes_total` is now `prometheus_tsdb_storage_blocks_bytes`. prometheus/tsdb#506
* [FEATURE] [EXPERIMENTAL] Time overlapping blocks are now allowed; vertical compaction and vertical query merge. It is an optional feature which is controlled by the `--storage.tsdb.allow-overlapping-blocks` flag, disabled by default. prometheus/tsdb#370
* [ENHANCEMENT] Use the WAL for remote_write API. #4588
* [ENHANCEMENT] Query performance improvements. prometheus/tsdb#531
* [ENHANCEMENT] UI enhancements with upgrade to Bootstrap 4. #5226
* [ENHANCEMENT] Reduce time that Alertmanagers are in flux when reloaded. #5126
* [ENHANCEMENT] Limit number of metrics displayed on UI to 10000. #5139
* [ENHANCEMENT] (1) Remember All/Unhealthy choice on target-overview when reloading page. (2) Resize text-input area on Graph page on mouseclick. #5201
* [ENHANCEMENT] In `histogram_quantile` merge buckets with equivalent le values. #5158.
* [ENHANCEMENT] Show list of offending labels in the error message in many-to-many scenarios. #5189
* [ENHANCEMENT] Show `Storage Retention` criteria in effect on `/status` page. #5322
* [BUGFIX] Fix sorting of rule groups. #5260
* [BUGFIX] Fix support for password_file and bearer_token_file in Kubernetes SD. #5211
* [BUGFIX] Scrape: catch errors when creating HTTP clients #5182. Adds new metrics:
  * `prometheus_target_scrape_pools_total`
  * `prometheus_target_scrape_pools_failed_total`
  * `prometheus_target_scrape_pool_reloads_total`
  * `prometheus_target_scrape_pool_reloads_failed_total`
* [BUGFIX] Fix panic when aggregator param is not a literal. #5290

## 2.7.2 / 2019-03-02

* [BUGFIX] `prometheus_rule_group_last_evaluation_timestamp_seconds` is now a unix timestamp. #5186

## 2.7.1 / 2019-01-31

This release has a fix for a Stored DOM XSS vulnerability that can be triggered when using the query history functionality. Thanks to Dor Tumarkin from Checkmarx for reporting it.

* [BUGFIX/SECURITY] Fix a Stored DOM XSS vulnerability with query history. #5163
* [BUGFIX] `prometheus_rule_group_last_duration_seconds` now reports seconds instead of nanoseconds. #5153
* [BUGFIX] Make sure the targets are consistently sorted in the targets page. #5161

## 2.7.0 / 2019-01-28

We're rolling back the Dockerfile changes introduced in 2.6.0. If you made changes to your docker deployment in 2.6.0, you will need to roll them back. This release also adds experimental support for disk size based retention. To accommodate that we are deprecating the flag `storage.tsdb.retention` in favour of `storage.tsdb.retention.time`. We print a warning if the flag is in use, but it will function without breaking until Prometheus 3.0.

* [CHANGE] Rollback Dockerfile to version at 2.5.0. Rollback of the breaking change introduced in 2.6.0. #5122
* [FEATURE] Add subqueries to PromQL. #4831
* [FEATURE] [EXPERIMENTAL] Add support for disk size based retention. Note that we don't consider the WAL size which could be significant and the time based retention policy also applies. #5109 prometheus/tsdb#343
* [FEATURE] Add CORS origin flag. #5011
* [ENHANCEMENT] Consul SD: Add tagged address to the discovery metadata. #5001
* [ENHANCEMENT] Kubernetes SD: Add service external IP and external name to the discovery metadata. #4940
* [ENHANCEMENT] Azure SD: Add support for Managed Identity authentication. #4590
* [ENHANCEMENT] Azure SD: Add tenant and subscription IDs to the discovery metadata. #4969
* [ENHANCEMENT] OpenStack SD: Add support for application credentials based authentication. #4968
* [ENHANCEMENT] Add metric for number of rule groups loaded. #5090
* [BUGFIX] Avoid duplicate tests for alert unit tests. #4964
* [BUGFIX] Don't depend on given order when comparing samples in alert unit testing. #5049
* [BUGFIX] Make sure the retention period doesn't overflow. #5112
* [BUGFIX] Make sure the blocks don't get very large. #5112
* [BUGFIX] Don't generate blocks with no samples. prometheus/tsdb#374
* [BUGFIX] Reintroduce metric for WAL corruptions. prometheus/tsdb#473

## 2.6.1 / 2019-01-15

* [BUGFIX] Azure SD: Fix discovery getting stuck sometimes. #5088
* [BUGFIX] Marathon SD: Use `Tasks.Ports` when `RequirePorts` is `false`. #5026
* [BUGFIX] Promtool: Fix "out-of-order sample" errors when testing rules. #5069

## 2.6.0 / 2018-12-17

* [CHANGE] Remove default flags from the container's entrypoint, run Prometheus from `/etc/prometheus` and symlink the storage directory to `/etc/prometheus/data`. #4976
* [CHANGE] Promtool: Remove the `update` command. #3839
* [FEATURE] Add JSON log format via the `--log.format` flag. #4876
* [FEATURE] API: Add /api/v1/labels endpoint to get all label names. #4835
* [FEATURE] Web: Allow setting the page's title via the `--web.ui-title` flag. #4841
* [ENHANCEMENT] Add `prometheus_tsdb_lowest_timestamp_seconds`, `prometheus_tsdb_head_min_time_seconds` and `prometheus_tsdb_head_max_time_seconds` metrics. #4888
* [ENHANCEMENT] Add `rule_group_last_evaluation_timestamp_seconds` metric. #4852
* [ENHANCEMENT] Add `prometheus_template_text_expansion_failures_total` and `prometheus_template_text_expansions_total` metrics. #4747
* [ENHANCEMENT] Set consistent User-Agent header in outgoing requests. #4891
* [ENHANCEMENT] Azure SD: Error out at load time when authentication parameters are missing. #4907
* [ENHANCEMENT] EC2 SD: Add the machine's private DNS name to the discovery metadata. #4693
* [ENHANCEMENT] EC2 SD: Add the operating system's platform to the discovery metadata. #4663
* [ENHANCEMENT] Kubernetes SD: Add the pod's phase to the discovery metadata. #4824
* [ENHANCEMENT] Kubernetes SD: Log Kubernetes messages. #4931
* [ENHANCEMENT] Promtool: Collect CPU and trace profiles. #4897
* [ENHANCEMENT] Promtool: Support writing output as JSON. #4848
* [ENHANCEMENT] Remote Read: Return available data if remote read fails partially. #4832
* [ENHANCEMENT] Remote Write: Improve queue performance. #4772
* [ENHANCEMENT] Remote Write: Add min_shards parameter to set the minimum number of shards. #4924
* [ENHANCEMENT] TSDB: Improve WAL reading. #4953
* [ENHANCEMENT] TSDB: Memory improvements. #4953
* [ENHANCEMENT] Web: Log stack traces on panic. #4221
* [ENHANCEMENT] Web UI: Add copy to clipboard button for configuration. #4410
* [ENHANCEMENT] Web UI: Support console queries at specific times. #4764
* [ENHANCEMENT] Web UI: group targets by job then instance. #4898 #4806
* [BUGFIX] Deduplicate handler labels for HTTP metrics. #4732
* [BUGFIX] Fix leaked queriers causing shutdowns to hang. #4922
* [BUGFIX] Fix configuration loading panics on nil pointer slice elements. #4942
* [BUGFIX] API: Correctly skip mismatching targets on /api/v1/targets/metadata. #4905
* [BUGFIX] API: Better rounding for incoming query timestamps. #4941
* [BUGFIX] Azure SD: Fix panic. #4867
* [BUGFIX] Console templates: Fix hover when the metric has a null value. #4906
* [BUGFIX] Discovery: Remove all targets when the scrape configuration gets empty. #4819
* [BUGFIX] Marathon SD: Fix leaked connections. #4915
* [BUGFIX] Marathon SD: Use 'hostPort' member of portMapping to construct target endpoints. #4887
* [BUGFIX] PromQL: Fix a goroutine leak in the lexer/parser. #4858
* [BUGFIX] Scrape: Pass through content-type for non-compressed output. #4912
* [BUGFIX] Scrape: Fix deadlock in the scrape's manager. #4894
* [BUGFIX] Scrape: Scrape targets at fixed intervals even after Prometheus restarts. #4926
* [BUGFIX] TSDB: Support restored snapshots including the head properly. #4953
* [BUGFIX] TSDB: Repair WAL when the last record in a segment is torn. #4953
* [BUGFIX] TSDB: Fix unclosed file readers on Windows systems. #4997
* [BUGFIX] Web: Avoid proxy to connect to the local gRPC server. #4572

## 2.5.0 / 2018-11-06

* [CHANGE] Group targets by scrape config instead of job name. #4806 #4526
* [CHANGE] Marathon SD: Various changes to adapt to Marathon 1.5+. #4499
* [CHANGE] Discovery: Split `prometheus_sd_discovered_targets` metric by scrape and notify (Alertmanager SD) as well as by section in the respective configuration. #4753
* [FEATURE] Add OpenMetrics support for scraping (EXPERIMENTAL). #4700
* [FEATURE] Add unit testing for rules. #4350
* [FEATURE] Make maximum number of samples per query configurable via `--query.max-samples` flag. #4513
* [FEATURE] Make maximum number of concurrent remote reads configurable via `--storage.remote.read-concurrent-limit` flag. #4656
* [ENHANCEMENT] Support s390x platform for Linux. #4605
* [ENHANCEMENT] API: Add `prometheus_api_remote_read_queries` metric tracking currently executed or waiting remote read API requests. #4699
* [ENHANCEMENT] Remote Read: Add `prometheus_remote_storage_remote_read_queries` metric tracking currently in-flight remote read queries. #4677
* [ENHANCEMENT] Remote Read: Reduced memory usage. #4655
* [ENHANCEMENT] Discovery: Add `prometheus_sd_discovered_targets`, `prometheus_sd_received_updates_total`, `prometheus_sd_updates_delayed_total`, and `prometheus_sd_updates_total` metrics for discovery subsystem. #4667
* [ENHANCEMENT] Discovery: Improve performance of previously slow updates of changes of targets. #4526
* [ENHANCEMENT] Kubernetes SD: Add extended metrics. #4458
* [ENHANCEMENT] OpenStack SD: Support discovering instances from all projects. #4682
* [ENHANCEMENT] OpenStack SD: Discover all interfaces. #4649
* [ENHANCEMENT] OpenStack SD: Support `tls_config` for the used HTTP client. #4654
* [ENHANCEMENT] Triton SD: Add ability to filter triton_sd targets by pre-defined groups. #4701
* [ENHANCEMENT] Web UI: Avoid browser spell-checking in expression field. #4728
* [ENHANCEMENT] Web UI: Add scrape duration and last evaluation time in targets and rules pages. #4722
* [ENHANCEMENT] Web UI: Improve rule view by wrapping lines. #4702
* [ENHANCEMENT] Rules: Error out at load time for invalid templates, rather than at evaluation time. #4537
* [ENHANCEMENT] TSDB: Add metrics for WAL operations. #4692
* [BUGFIX] Change max/min over_time to handle NaNs properly. #4386
* [BUGFIX] Check label name for `count_values` PromQL function. #4585
* [BUGFIX] Ensure that vectors and matrices do not contain identical label-sets. #4589

## 2.4.3 / 2018-10-04

* [BUGFIX] Fix panic when using custom EC2 API for SD #4672
* [BUGFIX] Fix panic when Zookeeper SD cannot connect to servers #4669
* [BUGFIX] Make the skip_head an optional parameter for snapshot API #4674

## 2.4.2 / 2018-09-21

 The last release didn't have bugfix included due to a vendoring error.

 * [BUGFIX] Handle WAL corruptions properly prometheus/tsdb#389
 * [BUGFIX] Handle WAL migrations correctly on Windows prometheus/tsdb#392

## 2.4.1 / 2018-09-19

* [ENHANCEMENT] New TSDB metrics prometheus/tsdb#375 prometheus/tsdb#363
* [BUGFIX] Render UI correctly for Windows #4616

## 2.4.0 / 2018-09-11

This release includes multiple bugfixes and features. Further, the WAL implementation has been re-written so the storage is not forward compatible. Prometheus 2.3 storage will work on 2.4 but not vice-versa.

* [CHANGE] Reduce remote write default retries #4279
* [CHANGE] Remove /heap endpoint #4460
* [FEATURE] Persist alert 'for' state across restarts #4061
* [FEATURE] Add API providing per target metric metadata #4183
* [FEATURE] Add API providing recording and alerting rules #4318 #4501
* [ENHANCEMENT] Brand new WAL implementation for TSDB. Forwards incompatible with previous WAL.
* [ENHANCEMENT] Show rule evaluation errors in UI #4457
* [ENHANCEMENT] Throttle resends of alerts to Alertmanager #4538
* [ENHANCEMENT] Send EndsAt along with the alert to Alertmanager #4550
* [ENHANCEMENT] Limit the samples returned by remote read endpoint #4532
* [ENHANCEMENT] Limit the data read in through remote read #4239
* [ENHANCEMENT] Coalesce identical SD configurations #3912
* [ENHANCEMENT] `promtool`: Add new commands for debugging and querying #4247 #4308 #4346 #4454
* [ENHANCEMENT] Update console examples for node_exporter v0.16.0 #4208
* [ENHANCEMENT] Optimize PromQL aggregations #4248
* [ENHANCEMENT] Remote read: Add Offset to hints #4226
* [ENHANCEMENT] `consul_sd`: Add support for ServiceMeta field #4280
* [ENHANCEMENT] `ec2_sd`: Maintain order of subnet_id label #4405
* [ENHANCEMENT] `ec2_sd`: Add support for custom endpoint to support EC2 compliant APIs #4333
* [ENHANCEMENT] `ec2_sd`: Add instance_owner label #4514
* [ENHANCEMENT] `azure_sd`: Add support for VMSS discovery and multiple environments #4202 #4569
* [ENHANCEMENT] `gce_sd`: Add instance_id label #4488
* [ENHANCEMENT] Forbid rule-abiding robots from indexing #4266
* [ENHANCEMENT] Log virtual memory limits on startup #4418
* [BUGFIX] Wait for service discovery to stop before exiting #4508
* [BUGFIX] Render SD configs properly #4338
* [BUGFIX] Only add LookbackDelta to vector selectors #4399
* [BUGFIX] `ec2_sd`: Handle panic-ing nil pointer #4469
* [BUGFIX] `consul_sd`: Stop leaking connections #4443
* [BUGFIX] Use templated labels also to identify alerts #4500
* [BUGFIX] Reduce floating point errors in stddev and related functions #4533
* [BUGFIX] Log errors while encoding responses #4359

## 2.3.2 / 2018-07-12

* [BUGFIX] Fix various tsdb bugs #4369
* [BUGFIX] Reorder startup and shutdown to prevent panics. #4321
* [BUGFIX] Exit with non-zero code on error #4296
* [BUGFIX] discovery/kubernetes/ingress: fix scheme discovery #4329
* [BUGFIX] Fix race in zookeeper sd #4355
* [BUGFIX] Better timeout handling in promql #4291 #4300
* [BUGFIX] Propagate errors when selecting series from the tsdb #4136

## 2.3.1 / 2018-06-19

* [BUGFIX] Avoid infinite loop on duplicate NaN values. #4275
* [BUGFIX] Fix nil pointer deference when using various API endpoints #4282
* [BUGFIX] config: set target group source index during unmarshaling #4245
* [BUGFIX] discovery/file: fix logging #4178
* [BUGFIX] kubernetes_sd: fix namespace filtering #4285
* [BUGFIX] web: restore old path prefix behavior #4273
* [BUGFIX] web: remove security headers added in 2.3.0 #4259

## 2.3.0 / 2018-06-05

* [CHANGE] `marathon_sd`: use `auth_token` and `auth_token_file` for token-based authentication instead of `bearer_token` and `bearer_token_file` respectively.
* [CHANGE] Metric names for HTTP server metrics changed
* [FEATURE] Add query commands to promtool
* [FEATURE] Add security headers to HTTP server responses
* [FEATURE] Pass query hints via remote read API
* [FEATURE] Basic auth passwords can now be configured via file across all configuration
* [ENHANCEMENT] Optimize PromQL and API serialization for memory usage and allocations
* [ENHANCEMENT] Limit number of dropped targets in web UI
* [ENHANCEMENT] Consul and EC2 service discovery allow using server-side filtering for performance improvement
* [ENHANCEMENT] Add advanced filtering configuration to EC2 service discovery
* [ENHANCEMENT] `marathon_sd`: adds support for basic and bearer authentication, plus all other common HTTP client options (TLS config, proxy URL, etc.)
* [ENHANCEMENT] Provide machine type metadata and labels in GCE service discovery
* [ENHANCEMENT] Add pod controller kind and name to Kubernetes service discovery data
* [ENHANCEMENT] Move TSDB to flock-based log file that works with Docker containers
* [BUGFIX] Properly propagate storage errors in PromQL
* [BUGFIX] Fix path prefix for web pages
* [BUGFIX] Fix goroutine leak in Consul service discovery
* [BUGFIX] Fix races in scrape manager
* [BUGFIX] Fix OOM for very large k in PromQL topk() queries
* [BUGFIX] Make remote write more resilient to unavailable receivers
* [BUGFIX] Make remote write shutdown cleanly
* [BUGFIX] Don't leak files on errors in TSDB's tombstone cleanup
* [BUGFIX] Unary minus expressions now removes the metric name from results
* [BUGFIX] Fix bug that lead to wrong amount of samples considered for time range expressions

## 2.2.1 / 2018-03-13

* [BUGFIX] Fix data loss in TSDB on compaction
* [BUGFIX] Correctly stop timer in remote-write path
* [BUGFIX] Fix deadlock triggered by loading targets page
* [BUGFIX] Fix incorrect buffering of samples on range selection queries
* [BUGFIX] Handle large index files on windows properly

## 2.2.0 / 2018-03-08

* [CHANGE] Rename file SD mtime metric.
* [CHANGE] Send target update on empty pod IP in Kubernetes SD.
* [FEATURE] Add API endpoint for flags.
* [FEATURE] Add API endpoint for dropped targets.
* [FEATURE] Display annotations on alerts page.
* [FEATURE] Add option to skip head data when taking snapshots.
* [ENHANCEMENT] Federation performance improvement.
* [ENHANCEMENT] Read bearer token file on every scrape.
* [ENHANCEMENT] Improve typeahead on `/graph` page.
* [ENHANCEMENT] Change rule file formatting.
* [ENHANCEMENT] Set consul server default to `localhost:8500`.
* [ENHANCEMENT] Add dropped Alertmanagers to API info endpoint.
* [ENHANCEMENT] Add OS type meta label to Azure SD.
* [ENHANCEMENT] Validate required fields in SD configuration.
* [BUGFIX] Prevent stack overflow on deep recursion in TSDB.
* [BUGFIX] Correctly read offsets in index files that are greater than 4GB.
* [BUGFIX] Fix scraping behavior for empty labels.
* [BUGFIX] Drop metric name for bool modifier.
* [BUGFIX] Fix races in discovery.
* [BUGFIX] Fix Kubernetes endpoints SD for empty subsets.
* [BUGFIX] Throttle updates from SD providers, which caused increased CPU usage and allocations.
* [BUGFIX] Fix TSDB block reload issue.
* [BUGFIX] Fix PromQL printing of empty `without()`.
* [BUGFIX] Don't reset FiredAt for inactive alerts.
* [BUGFIX] Fix erroneous file version changes and repair existing data.

## 2.1.0 / 2018-01-19

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
* [CHANGE] Admin and lifecycle APIs now disabled by default, can be re-enabled via flags
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
* [BUGFIX] Catch errors when unmarshaling delta/doubleDelta encoded chunks.
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
* [CLEANUP] Added initial HTTP API tests.
* [CLEANUP] Misc. other code cleanups.
* [MAINTENANCE] Updated vendored dependencies to their newest versions.

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
* [ENHANCEMENT] Help link points to https://prometheus.github.io now.
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
* [PERFORMANCE] Preparations for better memory reuse during marshaling / unmarshaling.

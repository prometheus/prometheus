# Changelog

## unreleased

* [CHANGE] TSDB: Fix the predicate checking for blocks which are beyond the retention period to include the ones right at the retention boundary. #9633
* [CHANGE] Rules: Execute 1 query instead of N (where N is the number of alerts within alert rule) when restoring alerts. #13980
* [ENHANCEMENT] Rules: Add `rule_group_last_restore_duration_seconds` to measure the time it takes to restore a rule group. #13974
* [ENHANCEMENT] OTLP: Improve remote write format translation performance by using label set hashes for metric identifiers instead of string based ones. #14006 #13991
* [BUGFIX] OTLP: Don't generate target_info unless at least one identifying label is defined. #13991
* [BUGFIX] OTLP: Don't generate target_info unless there are metrics. #13991

## 2.51.2 / 2024-04-09

Bugfix release.

[BUGFIX] Notifier: could hang when using relabeling on alerts #13861

## 2.51.1 / 2024-03-27

Bugfix release.

* [BUGFIX] PromQL: Re-instate validation of label_join destination label #13803
* [BUGFIX] Scraping (experimental native histograms): Fix handling of the min bucket factor on sync of targets #13846
* [BUGFIX] PromQL: Some queries could return the same series twice (library use only) #13845

## 2.51.0 / 2024-03-18

This version is built with Go 1.22.1.

There is a new optional build tag "dedupelabels", which should reduce memory consumption (#12304).
It is off by default; there will be an optional alternative image to try it out.

* [CHANGE] Scraping: Do experimental timestamp alignment even if tolerance is bigger than 1% of scrape interval #13624, #13737
* [FEATURE] Alerting: Relabel rules for AlertManagerConfig; allows routing alerts to different alertmanagers #12551, #13735
* [FEATURE] API: add limit param to series, label-names and label-values APIs #13396
* [FEATURE] UI (experimental native histograms): Add native histogram chart to Table view #13658
* [FEATURE] Promtool: Add a "tsdb dump-openmetrics" to dump in OpenMetrics format. #13194
* [FEATURE] PromQL (experimental native histograms): Add histogram_avg function #13467
* [ENHANCEMENT] Rules: Evaluate independent rules concurrently #12946, #13527
* [ENHANCEMENT] Scraping (experimental native histograms): Support exemplars #13488
* [ENHANCEMENT] Remote Write: Disable resharding during active retry backoffs #13562
* [ENHANCEMENT] Observability: Add native histograms to latency/duration metrics #13681
* [ENHANCEMENT] Observability: Add 'type' label to prometheus_tsdb_head_out_of_order_samples_appended_total #13607
* [ENHANCEMENT] API: Faster generation of targets into JSON #13469, #13484
* [ENHANCEMENT] Scraping, API: Use faster compression library #10782
* [ENHANCEMENT] OpenTelemetry: Performance improvements in OTLP parsing #13627
* [ENHANCEMENT] PromQL: Optimisations to reduce CPU and memory #13448, #13536
* [BUGFIX] PromQL: Constrain extrapolation in rate() to half of sample interval #13725
* [BUGFIX] Remote Write: Stop slowing down when a new WAL segment is created #13583, #13628
* [BUGFIX] PromQL: Fix wrongly scoped range vectors with @ modifier #13559
* [BUGFIX] Kubernetes SD: Pod status changes were not discovered by Endpoints service discovery #13337
* [BUGFIX] Azure SD: Fix 'error: parameter virtualMachineScaleSetName cannot be empty' (#13702)
* [BUGFIX] Remote Write: Fix signing for AWS sigv4 transport #13497
* [BUGFIX] Observability: Exemplars emitted by Prometheus use "trace_id" not "traceID" #13589

## 2.50.1 / 2024-02-26

* [BUGFIX] API: Fix metadata API using wrong field names. #13633

## 2.50.0 / 2024-02-22

* [CHANGE] Remote Write: Error `storage.ErrTooOldSample` is now generating HTTP error 400 instead of HTTP error 500. #13335
* [FEATURE] Remote Write: Drop old inmemory samples. Activated using the config entry `sample_age_limit`. #13002
* [FEATURE] **Experimental**: Add support for ingesting zeros as created timestamps. (enabled under the feature-flag `created-timestamp-zero-ingestion`). #12733 #13279
* [FEATURE] Promtool: Add `analyze` histograms command. #12331
* [FEATURE] TSDB/compaction: Add a way to enable overlapping compaction. #13282 #13393 #13398
* [FEATURE] Add automatic memory limit handling. Activated using the feature flag. `auto-gomemlimit` #13395
* [ENHANCEMENT] Promtool: allow specifying multiple matchers in `promtool tsdb dump`. #13296
* [ENHANCEMENT] PromQL: Restore more efficient version of `NewPossibleNonCounterInfo` annotation. #13022
* [ENHANCEMENT] Kuma SD: Extend configuration to allow users to specify client ID. #13278
* [ENHANCEMENT] PromQL: Use natural sort in `sort_by_label` and `sort_by_label_desc`. This is **experimental**. #13411
* [ENHANCEMENT] Native Histograms: support `native_histogram_min_bucket_factor` in scrape_config. #13222
* [ENHANCEMENT] Native Histograms: Issue warning if histogramRate is applied to the wrong kind of histogram. #13392
* [ENHANCEMENT] TSDB: Make transaction isolation data structures smaller. #13015
* [ENHANCEMENT] TSDB/postings: Optimize merge using Loser Tree. #12878
* [ENHANCEMENT] TSDB: Simplify internal series delete function. #13261
* [ENHANCEMENT] Agent: Performance improvement by making the global hash lookup table smaller. #13262
* [ENHANCEMENT] PromQL: faster execution of metric functions, e.g. abs(), rate() #13446
* [ENHANCEMENT] TSDB: Optimize label values with matchers by taking shortcuts. #13426
* [ENHANCEMENT] Kubernetes SD: Check preconditions earlier and avoid unnecessary checks or iterations in kube_sd. #13408
* [ENHANCEMENT] Promtool: Improve visibility for `promtool test rules` with JSON colored formatting. #13342
* [ENHANCEMENT] Consoles: Exclude iowait and steal from CPU Utilisation. #9593
* [ENHANCEMENT] Various improvements and optimizations on Native Histograms. #13267, #13215, #13276 #13289, #13340
* [BUGFIX] Scraping: Fix quality value in HTTP Accept header. #13313
* [BUGFIX] UI: Fix usage of the function `time()` that was crashing. #13371
* [BUGFIX] Azure SD: Fix SD crashing when it finds a VM scale set. #13578

## 2.49.1 / 2024-01-15

* [BUGFIX] TSDB: Fixed a wrong `q=` value in scrape accept header #13313

## 2.49.0 / 2024-01-15

* [FEATURE] Promtool: Add `--run` flag promtool test rules command. #12206
* [FEATURE] SD: Add support for `NS` records to DNS SD. #13219
* [FEATURE] UI: Add heatmap visualization setting in the Graph tab, useful histograms. #13096 #13371
* [FEATURE] Scraping: Add `scrape_config.enable_compression` (default true) to disable gzip compression when scraping the target. #13166
* [FEATURE] PromQL: Add a `promql-experimental-functions` feature flag containing some new experimental PromQL functions. #13103 NOTE: More experimental functions might be added behind the same feature flag in the future. Added functions:
  * Experimental `mad_over_time` (median absolute deviation around the median) function. #13059
  * Experimental `sort_by_label` and `sort_by_label_desc` functions allowing sorting returned series by labels. #11299
* [FEATURE] SD: Add `__meta_linode_gpus` label to Linode SD. #13097
* [FEATURE] API: Add `exclude_alerts` query parameter to `/api/v1/rules` to only return recording rules. #12999
* [FEATURE] TSDB: --storage.tsdb.retention.time flag value is now exposed as a `prometheus_tsdb_retention_limit_seconds` metric. #12986
* [FEATURE] Scraping: Add ability to specify priority of scrape protocols to accept during scrape (e.g. to scrape Prometheus proto format for certain jobs). This can be changed by setting `global.scrape_protocols` and `scrape_config.scrape_protocols`. #12738
* [ENHANCEMENT] Scraping: Automated handling of scraping histograms that violate `scrape_config.native_histogram_bucket_limit` setting. #13129
* [ENHANCEMENT] Scraping: Optimized memory allocations when scraping. #12992
* [ENHANCEMENT] SD: Added cache for Azure SD to avoid rate-limits. #12622
* [ENHANCEMENT] TSDB: Various improvements to OOO exemplar scraping. E.g. allowing ingestion of exemplars with the same timestamp, but with different labels. #13021
* [ENHANCEMENT] API: Optimize `/api/v1/labels` and `/api/v1/label/<label_name>/values` when 1 set of matchers are used. #12888
* [ENHANCEMENT] TSDB: Various optimizations for TSDB block index, head mmap chunks and WAL, reducing latency and memory allocations (improving API calls, compaction queries etc). #12997 #13058 #13056 #13040
* [ENHANCEMENT] PromQL: Optimize memory allocations and latency when querying float histograms. #12954
* [ENHANCEMENT] Rules: Instrument TraceID in log lines for rule evaluations. #13034
* [ENHANCEMENT] PromQL: Optimize memory allocations in query_range calls. #13043
* [ENHANCEMENT] Promtool: unittest interval now defaults to evaluation_intervals when not set. #12729
* [BUGFIX] SD: Fixed Azure SD public IP reporting #13241
* [BUGFIX] API: Fix inaccuracies in posting cardinality statistics. #12653
* [BUGFIX] PromQL: Fix inaccuracies of `histogram_quantile` with classic histograms. #13153
* [BUGFIX] TSDB: Fix rare fails or inaccurate queries with OOO samples. #13115
* [BUGFIX] TSDB: Fix rare panics on append commit when exemplars are used. #13092
* [BUGFIX] TSDB: Fix exemplar WAL storage, so remote write can send/receive samples before exemplars. #13113
* [BUGFIX] Mixins: Fix `url` filter on remote write dashboards. #10721
* [BUGFIX] PromQL/TSDB: Various fixes to float histogram operations. #12891 #12977 #12609 #13190 #13189 #13191 #13201 #13212 #13208
* [BUGFIX] Promtool: Fix int32 overflow issues for 32-bit architectures. #12978
* [BUGFIX] SD: Fix Azure VM Scale Set NIC issue. #13283

## 2.48.1 / 2023-12-07

* [BUGFIX] TSDB: Make the wlog watcher read segments synchronously when not tailing. #13224
* [BUGFIX] Agent: Participate in notify calls (fixes slow down in remote write handling introduced in 2.45). #13223

## 2.48.0 / 2023-11-16

* [CHANGE] Remote-write: respect Retry-After header on 5xx errors. #12677
* [FEATURE] Alerting: Add AWS SigV4 authentication support for Alertmanager endpoints. #12774
* [FEATURE] Promtool: Add support for histograms in the TSDB dump command. #12775
* [FEATURE] PromQL: Add warnings (and annotations) to PromQL query results. #12152 #12982 #12988 #13012
* [FEATURE] Remote-write: Add Azure AD OAuth authentication support for remote write requests. #12572
* [ENHANCEMENT] Remote-write: Add a header to count retried remote write requests. #12729
* [ENHANCEMENT] TSDB: Improve query performance by re-using iterator when moving between series. #12757
* [ENHANCEMENT] UI: Move /targets page discovered labels to expandable section #12824
* [ENHANCEMENT] TSDB: Optimize WBL loading by not sending empty buffers over channel. #12808
* [ENHANCEMENT] TSDB: Reply WBL mmap markers concurrently. #12801
* [ENHANCEMENT] Promtool: Add support for specifying series matchers in the TSDB analyze command. #12842
* [ENHANCEMENT] PromQL: Prevent Prometheus from overallocating memory on subquery with large amount of steps. #12734
* [ENHANCEMENT] PromQL: Add warning when monotonicity is forced in the input to histogram_quantile. #12931
* [ENHANCEMENT] Scraping: Optimize sample appending by reducing garbage. #12939
* [ENHANCEMENT] Storage: Reduce memory allocations in queries that merge series sets. #12938
* [ENHANCEMENT] UI: Show group interval in rules display. #12943
* [ENHANCEMENT] Scraping: Save memory when scraping by delaying creation of buffer. #12953
* [ENHANCEMENT] Agent: Allow ingestion of out-of-order samples. #12897
* [ENHANCEMENT] Promtool: Improve support for native histograms in TSDB analyze command. #12869
* [ENHANCEMENT] Scraping: Add configuration option for tracking staleness of scraped timestamps. #13060
* [BUGFIX] SD: Ensure that discovery managers are properly canceled. #10569
* [BUGFIX] TSDB: Fix PostingsForMatchers race with creating new series. #12558
* [BUGFIX] TSDB: Fix handling of explicit counter reset header in histograms. #12772
* [BUGFIX] SD: Validate HTTP client configuration in HTTP, EC2, Azure, Uyuni, PuppetDB, and Lightsail SDs. #12762 #12811 #12812 #12815 #12814 #12816
* [BUGFIX] TSDB: Fix counter reset edgecases causing native histogram panics. #12838
* [BUGFIX] TSDB: Fix duplicate sample detection at chunk size limit. #12874
* [BUGFIX] Promtool: Fix errors not being reported in check rules command. #12715
* [BUGFIX] TSDB: Avoid panics reported in logs when head initialization takes a long time. #12876
* [BUGFIX] TSDB: Ensure that WBL is repaired when possible. #12406
* [BUGFIX] Storage: Fix crash caused by incorrect mixed samples handling. #13055
* [BUGFIX] TSDB: Fix compactor failures by adding min time to histogram chunks. #13062

## 2.47.1 / 2023-10-04

* [BUGFIX] Fix duplicate sample detection at chunk size limit #12874

## 2.47.0 / 2023-09-06

This release adds an experimental OpenTelemetry (OTLP) Ingestion feature,
and also new setting `keep_dropped_targets` to limit the amount of dropped
targets held in memory. This defaults to 0 meaning 'no limit', so we encourage
users with large Prometheus to try setting a limit such as 100.

* [FEATURE] Web: Add OpenTelemetry (OTLP) Ingestion endpoint. #12571 #12643
* [FEATURE] Scraping: Optionally limit detail on dropped targets, to save memory. #12647
* [ENHANCEMENT] TSDB: Write head chunks to disk in the background to reduce blocking. #11818
* [ENHANCEMENT] PromQL: Speed up aggregate and function queries. #12682
* [ENHANCEMENT] PromQL: More efficient evaluation of query with `timestamp()`. #12579
* [ENHANCEMENT] API: Faster streaming of Labels to JSON. #12598
* [ENHANCEMENT] Agent: Memory pooling optimisation. #12651
* [ENHANCEMENT] TSDB: Prevent storage space leaks due to terminated snapshots on shutdown. #12664
* [ENHANCEMENT] Histograms: Refactoring and optimisations. #12352 #12584 #12596 #12711 #12054
* [ENHANCEMENT] Histograms: Add `histogram_stdvar` and `histogram_stddev` functions. #12614
* [ENHANCEMENT] Remote-write: add http.resend_count tracing attribute. #12676
* [ENHANCEMENT] TSDB: Support native histograms in snapshot on shutdown. #12722
* [BUGFIX] TSDB/Agent: ensure that new series get written to WAL on rollback. #12592
* [BUGFIX] Scraping: fix infinite loop on exemplar in protobuf format. #12737

## 2.46.0 / 2023-07-25

* [FEATURE] Promtool: Add PromQL format and label matcher set/delete commands to promtool. #11411
* [FEATURE] Promtool: Add push metrics command. #12299
* [ENHANCEMENT] Promtool: Read from stdin if no filenames are provided in check rules. #12225
* [ENHANCEMENT] Hetzner SD: Support larger ID's that will be used by Hetzner in September. #12569
* [ENHANCEMENT] Kubernetes SD: Add more labels for endpointslice and endpoints role. #10914
* [ENHANCEMENT] Kubernetes SD: Do not add pods to target group if the PodIP status is not set. #11642
* [ENHANCEMENT] OpenStack SD: Include instance image ID in labels. #12502
* [ENHANCEMENT] Remote Write receiver: Validate the metric names and labels. #11688
* [ENHANCEMENT] Web: Initialize `prometheus_http_requests_total` metrics with `code` label set to `200`. #12472
* [ENHANCEMENT] TSDB: Add Zstandard compression option for wlog. #11666
* [ENHANCEMENT] TSDB: Support native histograms in snapshot on shutdown. #12258
* [ENHANCEMENT] Labels: Avoid compiling regexes that are literal. #12434
* [BUGFIX] Histograms: Fix parsing of float histograms without zero bucket. #12577
* [BUGFIX] Histograms: Fix scraping native and classic histograms missing some histograms. #12554
* [BUGFIX] Histograms: Enable ingestion of multiple exemplars per sample. 12557
* [BUGFIX] File SD: Fix path handling in File-SD watcher to allow directory monitoring on Windows. #12488
* [BUGFIX] Linode SD: Cast `InstanceSpec` values to `int64` to avoid overflows on 386 architecture. #12568
* [BUGFIX] PromQL Engine: Include query parsing in active-query tracking. #12418
* [BUGFIX] TSDB: Handle TOC parsing failures. #10623

## 2.45.0 / 2023-06-23

This release is a LTS (Long-Term Support) release of Prometheus and will
receive security, documentation and bugfix patches for at least 12 months.
Please read more about our LTS release cycle at
<https://prometheus.io/docs/introduction/release-cycle/>.

* [FEATURE] API: New limit parameter to limit the number of items returned by `/api/v1/status/tsdb` endpoint. #12336
* [FEATURE] Config: Add limits to global config. #12126
* [FEATURE] Consul SD: Added support for `path_prefix`. #12372
* [FEATURE] Native histograms: Add option to scrape both classic and native histograms. #12350
* [FEATURE] Native histograms: Added support for two more arithmetic operators `avg_over_time` and `sum_over_time`. #12262
* [FEATURE] Promtool: When providing the block id, only one block will be loaded and analyzed. #12031
* [FEATURE] Remote-write: New Azure ad configuration to support remote writing directly to Azure Monitor workspace. #11944
* [FEATURE] TSDB: Samples per chunk are now configurable with flag `storage.tsdb.samples-per-chunk`. By default set to its former value 120. #12055
* [ENHANCEMENT] Native histograms: bucket size can now be limited to avoid scrape fails. #12254
* [ENHANCEMENT] TSDB: Dropped series are now deleted from the WAL sooner. #12297
* [BUGFIX] Native histograms: ChunkSeries iterator now checks if a new sample can be appended to the open chunk. #12185
* [BUGFIX] Native histograms: Fix Histogram Appender `Appendable()` segfault. #12357
* [BUGFIX] Native histograms: Fix setting reset header to gauge histograms in seriesToChunkEncoder. #12329
* [BUGFIX] TSDB: Tombstone intervals are not modified after Get() call. #12245
* [BUGFIX] TSDB: Use path/filepath to set the WAL directory. #12349

## 2.44.0 / 2023-05-13

This version is built with Go tag `stringlabels`, to use the smaller data
structure for Labels that was optional in the previous release. For more
details about this code change see #10991.

* [CHANGE] Remote-write: Raise default samples per send to 2,000. #12203
* [FEATURE] Remote-read: Handle native histograms. #12085, #12192
* [FEATURE] Promtool: Health and readiness check of prometheus server in CLI. #12096
* [FEATURE] PromQL: Add `query_samples_total` metric, the total number of samples loaded by all queries. #12251
* [ENHANCEMENT] Storage: Optimise buffer used to iterate through samples. #12326
* [ENHANCEMENT] Scrape: Reduce memory allocations on target labels. #12084
* [ENHANCEMENT] PromQL: Use faster heap method for `topk()` / `bottomk()`. #12190
* [ENHANCEMENT] Rules API: Allow filtering by rule name. #12270
* [ENHANCEMENT] Native Histograms: Various fixes and improvements. #11687, #12264, #12272
* [ENHANCEMENT] UI: Search of scraping pools is now case-insensitive. #12207
* [ENHANCEMENT] TSDB: Add an affirmative log message for successful WAL repair. #12135
* [BUGFIX] TSDB: Block compaction failed when shutting down. #12179
* [BUGFIX] TSDB: Out-of-order chunks could be ignored if the write-behind log was deleted. #12127

## 2.43.1 / 2023-05-03

* [BUGFIX] Labels: `Set()` after `Del()` would be ignored, which broke some relabeling rules. #12322

## 2.43.0 / 2023-03-21

We are working on some performance improvements in Prometheus, which are only
built into Prometheus when compiling it using the Go tag `stringlabels`
(therefore they are not shipped in the default binaries). It uses a data
structure for labels that uses a single string to hold all the label/values,
resulting in a smaller heap size and some speedups in most cases. We would like
to encourage users who are interested in these improvements to help us measure
the gains on their production architecture. We are providing release artefacts
`2.43.0+stringlabels` and Docker images tagged `v2.43.0-stringlabels` with those
improvements for testing. #10991

* [FEATURE] Promtool: Add HTTP client configuration to query commands. #11487
* [FEATURE] Scrape: Add `scrape_config_files` to include scrape configs from different files. #12019
* [FEATURE] HTTP client: Add `no_proxy` to exclude URLs from proxied requests. #12098
* [FEATURE] HTTP client: Add `proxy_from_environment` to read proxies from env variables. #12098
* [ENHANCEMENT] API: Add support for setting lookback delta per query via the API. #12088
* [ENHANCEMENT] API: Change HTTP status code from 503/422 to 499 if a request is canceled. #11897
* [ENHANCEMENT] Scrape: Allow exemplars for all metric types. #11984
* [ENHANCEMENT] TSDB: Add metrics for head chunks and WAL folders size. #12013
* [ENHANCEMENT] TSDB: Automatically remove incorrect snapshot with index that is ahead of WAL. #11859
* [ENHANCEMENT] TSDB: Improve Prometheus parser error outputs to be more comprehensible. #11682
* [ENHANCEMENT] UI: Scope `group by` labels to metric in autocompletion. #11914
* [BUGFIX] Scrape: Fix `prometheus_target_scrape_pool_target_limit` metric not set before reloading. #12002
* [BUGFIX] TSDB: Correctly update `prometheus_tsdb_head_chunks_removed_total` and `prometheus_tsdb_head_chunks` metrics when reading WAL. #11858
* [BUGFIX] TSDB: Use the correct unit (seconds) when recording out-of-order append deltas in the `prometheus_tsdb_sample_ooo_delta` metric. #12004

## 2.42.0 / 2023-01-31

This release comes with a bunch of feature coverage for native histograms and breaking changes.

If you are trying native histograms already, we recommend you remove the `wal` directory when upgrading.
Because the old WAL record for native histograms is not backward compatible in v2.42.0, this will lead to some data loss for the latest data.

Additionally, if you scrape "float histograms" or use recording rules on native histograms in v2.42.0 (which writes float histograms),
it is a one-way street since older versions do not support float histograms.

* [CHANGE] **breaking** TSDB: Changed WAL record format for the experimental native histograms. #11783
* [FEATURE] Add 'keep_firing_for' field to alerting rules. #11827
* [FEATURE] Promtool: Add support of selecting timeseries for TSDB dump. #11872
* [ENHANCEMENT] Agent: Native histogram support. #11842
* [ENHANCEMENT] Rules: Support native histograms in recording rules. #11838
* [ENHANCEMENT] SD: Add container ID as a meta label for pod targets for Kubernetes. #11844
* [ENHANCEMENT] SD: Add VM size label to azure service discovery. #11650
* [ENHANCEMENT] Support native histograms in federation. #11830
* [ENHANCEMENT] TSDB: Add gauge histogram support. #11783 #11840 #11814
* [ENHANCEMENT] TSDB/Scrape: Support FloatHistogram that represents buckets as float64 values. #11522 #11817 #11716
* [ENHANCEMENT] UI: Show individual scrape pools on /targets page. #11142

## 2.41.0 / 2022-12-20

* [FEATURE] Relabeling: Add `keepequal` and `dropequal` relabel actions. #11564
* [FEATURE] Add support for HTTP proxy headers. #11712
* [ENHANCEMENT] Reload private certificates when changed on disk. #11685
* [ENHANCEMENT] Add `max_version` to specify maximum TLS version in `tls_config`. #11685
* [ENHANCEMENT] Add `goos` and `goarch` labels to `prometheus_build_info`. #11685
* [ENHANCEMENT] SD: Add proxy support for EC2 and LightSail SDs #11611
* [ENHANCEMENT] SD: Add new metric `prometheus_sd_file_watcher_errors_total`. #11066
* [ENHANCEMENT] Remote Read: Use a pool to speed up marshalling. #11357
* [ENHANCEMENT] TSDB: Improve handling of tombstoned chunks in iterators. #11632
* [ENHANCEMENT] TSDB: Optimize postings offset table reading. #11535
* [BUGFIX] Scrape: Validate the metric name, label names, and label values after relabeling. #11074
* [BUGFIX] Remote Write receiver and rule manager: Fix error handling. #11727

## 2.40.7 / 2022-12-14

* [BUGFIX] Use Windows native DNS resolver. #11704
* [BUGFIX] TSDB: Fix queries involving negative buckets of native histograms. #11699

## 2.40.6 / 2022-12-09

* [SECURITY] Security upgrade from go and upstream dependencies that include
  security fixes to the net/http and os packages. #11691

## 2.40.5 / 2022-12-01

* [BUGFIX] TSDB: Fix queries involving native histograms due to improper reset of iterators. #11643

## 2.40.4 / 2022-11-29

* [SECURITY] Fix basic authentication bypass vulnerability (CVE-2022-46146). GHSA-4v48-4q5m-8vx4

## 2.40.3 / 2022-11-23

* [BUGFIX] TSDB: Fix compaction after a deletion is called. #11623

## 2.40.2 / 2022-11-16

* [BUGFIX] UI: Fix black-on-black metric name color in dark mode. #11572

## 2.40.1 / 2022-11-09

* [BUGFIX] TSDB: Fix alignment for atomic int64 for 32 bit architecture. #11547
* [BUGFIX] Scrape: Fix accept headers. #11552

## 2.40.0 / 2022-11-08

This release introduces an experimental, native way of representing and storing histograms.

It can be enabled in Prometheus via `--enable-feature=native-histograms` to accept native histograms.
Enabling native histograms will also switch the preferred exposition format to protobuf.

To instrument your application with native histograms, use the `main` branch of `client_golang` (this will change for the final release when v1.14.0 of client_golang will be out), and set the `NativeHistogramBucketFactor` in your `HistogramOpts` (`1.1` is a good starting point).
Your existing histograms won't switch to native histograms until `NativeHistogramBucketFactor` is set.

* [FEATURE] Add **experimental** support for native histograms. Enable with the flag `--enable-feature=native-histograms`. #11447
* [FEATURE] SD: Add service discovery for OVHcloud. #10802
* [ENHANCEMENT] Kubernetes SD: Use protobuf encoding. #11353
* [ENHANCEMENT] TSDB: Use golang.org/x/exp/slices for improved sorting speed. #11054 #11318 #11380
* [ENHANCEMENT] Consul SD: Add enterprise admin partitions. Adds `__meta_consul_partition` label. Adds `partition` config in `consul_sd_config`. #11482
* [BUGFIX] API: Fix API error codes for `/api/v1/labels` and `/api/v1/series`. #11356

## 2.39.2 / 2022-11-09

* [BUGFIX] TSDB: Fix alignment for atomic int64 for 32 bit architecture. #11547

## 2.39.1 / 2022-10-07

* [BUGFIX] Rules: Fix notifier relabel changing the labels on active alerts. #11427

## 2.39.0 / 2022-10-05

* [FEATURE] **experimental** TSDB: Add support for ingesting out-of-order samples. This is configured via `out_of_order_time_window` field in the config file; check config file docs for more info. #11075
* [ENHANCEMENT] API: `/-/healthy` and `/-/ready` API calls now also respond to a `HEAD` request on top of existing `GET` support. #11160
* [ENHANCEMENT] PuppetDB SD: Add `__meta_puppetdb_query` label. #11238
* [ENHANCEMENT] AWS EC2 SD: Add `__meta_ec2_region` label. #11326
* [ENHANCEMENT] AWS Lightsail SD: Add `__meta_lightsail_region` label. #11326
* [ENHANCEMENT] Scrape: Optimise relabeling by re-using memory. #11147
* [ENHANCEMENT] TSDB: Improve WAL replay timings. #10973 #11307 #11319
* [ENHANCEMENT] TSDB: Optimise memory by not storing unnecessary data in the memory. #11280 #11288 #11296
* [ENHANCEMENT] TSDB: Allow overlapping blocks by default. `--storage.tsdb.allow-overlapping-blocks` now has no effect. #11331
* [ENHANCEMENT] UI: Click to copy label-value pair from query result to clipboard. #11229
* [BUGFIX] TSDB: Turn off isolation for Head compaction to fix a memory leak. #11317
* [BUGFIX] TSDB: Fix 'invalid magic number 0' error on Prometheus startup. #11338
* [BUGFIX] PromQL: Properly close file descriptor when logging unfinished queries. #11148
* [BUGFIX] Agent: Fix validation of flag options and prevent WAL from growing more than desired. #9876

## 2.38.0 / 2022-08-16

* [FEATURE]: Web: Add a `/api/v1/format_query` HTTP API endpoint that allows pretty-formatting PromQL expressions. #11036 #10544 #11005
* [FEATURE]: UI: Add support for formatting PromQL expressions in the UI. #11039
* [FEATURE]: DNS SD: Support MX records for discovering targets. #10099
* [FEATURE]: Templates: Add `toTime()` template function that allows converting sample timestamps to Go `time.Time` values. #10993
* [ENHANCEMENT]: Kubernetes SD: Add `__meta_kubernetes_service_port_number` meta label indicating the service port number. #11002 #11053
* [ENHANCEMENT]: Kubernetes SD: Add `__meta_kubernetes_pod_container_image` meta label indicating the container image. #11034 #11146
* [ENHANCEMENT]: PromQL: When a query panics, also log the query itself alongside the panic message. #10995
* [ENHANCEMENT]: UI: Tweak colors in the dark theme to improve the contrast ratio. #11068
* [ENHANCEMENT]: Web: Speed up calls to `/api/v1/rules` by avoiding locks and using atomic types instead. #10858
* [ENHANCEMENT]: Scrape: Add a `no-default-scrape-port` feature flag, which omits or removes any default HTTP (`:80`) or HTTPS (`:443`) ports in the target's scrape address. #9523
* [BUGFIX]: TSDB: In the WAL watcher metrics, expose the `type="exemplar"` label instead of `type="unknown"` for exemplar records. #11008
* [BUGFIX]: TSDB: Fix race condition around allocating series IDs during chunk snapshot loading. #11099

## 2.37.0 / 2022-07-14

This release is a LTS (Long-Term Support) release of Prometheus and will
receive security, documentation and bugfix patches for at least 6 months.
Please read more about our LTS release cycle at
<https://prometheus.io/docs/introduction/release-cycle/>.

Following data loss by users due to lack of unified buffer cache in OpenBSD, we
will no longer release Prometheus upstream for OpenBSD until a proper solution is
found. #8799

* [FEATURE] Nomad SD: New service discovery for Nomad built-in service discovery. #10915
* [ENHANCEMENT] Kubernetes SD: Allow attaching node labels for endpoint role. #10759
* [ENHANCEMENT] PromQL: Optimise creation of signature with/without labels. #10667
* [ENHANCEMENT] TSDB: Memory optimizations. #10873 #10874
* [ENHANCEMENT] TSDB: Reduce sleep time when reading WAL. #10859 #10878
* [ENHANCEMENT] OAuth2: Add appropriate timeouts and User-Agent header. #11020
* [BUGFIX] Alerting: Fix Alertmanager targets not being updated when alerts were queued. #10948
* [BUGFIX] Hetzner SD: Make authentication files relative to Prometheus config file. #10813
* [BUGFIX] Promtool: Fix `promtool check config` not erroring properly on failures. #10952
* [BUGFIX] Scrape: Keep relabeled scrape interval and timeout on reloads. #10916
* [BUGFIX] TSDB: Don't increment `prometheus_tsdb_compactions_failed_total` when context is canceled. #10772
* [BUGFIX] TSDB: Fix panic if series is not found when deleting series. #10907
* [BUGFIX] TSDB: Increase `prometheus_tsdb_mmap_chunk_corruptions_total` on out of sequence errors. #10406
* [BUGFIX] Uyuni SD: Make authentication files relative to Prometheus configuration file and fix default configuration values. #10813

## 2.36.2 / 2022-06-20

* [BUGFIX] Fix serving of static assets like fonts and favicon. #10888

## 2.36.1 / 2022-06-09

* [BUGFIX] promtool: Add --lint-fatal option. #10840

## 2.36.0 / 2022-05-30

* [FEATURE] Add lowercase and uppercase relabel action. #10641
* [FEATURE] SD: Add IONOS Cloud integration. #10514
* [FEATURE] SD: Add Vultr integration. #10714
* [FEATURE] SD: Add Linode SD failure count metric. #10673
* [FEATURE] Add prometheus_ready metric. #10682
* [ENHANCEMENT] Add stripDomain to template function. #10475
* [ENHANCEMENT] UI: Enable active search through dropped targets. #10668
* [ENHANCEMENT] promtool: support matchers when querying label values. #10727
* [ENHANCEMENT] Add agent mode identifier. #9638
* [BUGFIX] Changing TotalQueryableSamples from int to int64. #10549
* [BUGFIX] tsdb/agent: Ignore duplicate exemplars. #10595
* [BUGFIX] TSDB: Fix chunk overflow appending samples at a variable rate. #10607
* [BUGFIX] Stop rule manager before TSDB is stopped. #10680

## 2.35.0 / 2022-04-21

This Prometheus release is built with go1.18, which contains two noticeable changes related to TLS:

1. [TLS 1.0 and 1.1 disabled by default client-side](https://go.dev/doc/go1.18#tls10).
Prometheus users can override this with the `min_version` parameter of [tls_config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#tls_config).
2. [Certificates signed with the SHA-1 hash function are rejected](https://go.dev/doc/go1.18#sha1). This doesn't apply to self-signed root certificates.

* [CHANGE] TSDB: Delete `*.tmp` WAL files when Prometheus starts. #10317
* [CHANGE] promtool: Add new flag `--lint` (enabled by default) for the commands `check rules` and `check config`, resulting in a new exit code (`3`) for linter errors. #10435
* [FEATURE] Support for automatically setting the variable `GOMAXPROCS` to the container CPU limit. Enable with the flag `--enable-feature=auto-gomaxprocs`. #10498
* [FEATURE] PromQL: Extend statistics with total and peak number of samples in a query. Additionally, per-step statistics are available with  --enable-feature=promql-per-step-stats and using `stats=all` in the query API.
Enable with the flag `--enable-feature=per-step-stats`. #10369
* [ENHANCEMENT] Prometheus is built with Go 1.18. #10501
* [ENHANCEMENT] TSDB: more efficient sorting of postings read from WAL at startup. #10500
* [ENHANCEMENT] Azure SD: Add metric to track Azure SD failures. #10476
* [ENHANCEMENT] Azure SD: Add an optional `resource_group` configuration. #10365
* [ENHANCEMENT] Kubernetes SD: Support `discovery.k8s.io/v1` `EndpointSlice` (previously only `discovery.k8s.io/v1beta1` `EndpointSlice` was supported). #9570
* [ENHANCEMENT] Kubernetes SD: Allow attaching node metadata to discovered pods. #10080
* [ENHANCEMENT] OAuth2: Support for using a proxy URL to fetch OAuth2 tokens. #10492
* [ENHANCEMENT] Configuration: Add the ability to disable HTTP2. #10492
* [ENHANCEMENT] Config: Support overriding minimum TLS version. #10610
* [BUGFIX] Kubernetes SD: Explicitly include gcp auth from k8s.io. #10516
* [BUGFIX] Fix OpenMetrics parser to sort uppercase labels correctly. #10510
* [BUGFIX] UI: Fix scrape interval and duration tooltip not showing on target page. #10545
* [BUGFIX] Tracing/GRPC: Set TLS credentials only when insecure is false. #10592
* [BUGFIX] Agent: Fix ID collision when loading a WAL with multiple segments. #10587
* [BUGFIX] Remote-write: Fix a deadlock between Batch and flushing the queue. #10608

## 2.34.0 / 2022-03-15

* [CHANGE] UI: Classic UI removed. #10208
* [CHANGE] Tracing: Migrate from Jaeger to OpenTelemetry based tracing. #9724, #10203, #10276
* [ENHANCEMENT] TSDB: Disable the chunk write queue by default and allow configuration with the experimental flag `--storage.tsdb.head-chunks-write-queue-size`. #10425
* [ENHANCEMENT] HTTP SD: Add a failure counter. #10372
* [ENHANCEMENT] Azure SD: Set Prometheus User-Agent on requests. #10209
* [ENHANCEMENT] Uyuni SD: Reduce the number of logins to Uyuni. #10072
* [ENHANCEMENT] Scrape: Log when an invalid media type is encountered during a scrape. #10186
* [ENHANCEMENT] Scrape: Accept application/openmetrics-text;version=1.0.0 in addition to version=0.0.1. #9431
* [ENHANCEMENT] Remote-read: Add an option to not use external labels as selectors for remote read. #10254
* [ENHANCEMENT] UI: Optimize the alerts page and add a search bar. #10142
* [ENHANCEMENT] UI: Improve graph colors that were hard to see. #10179
* [ENHANCEMENT] Config: Allow escaping of `$` with `$$` when using environment variables with external labels. #10129
* [BUGFIX] PromQL: Properly return an error from histogram_quantile when metrics have the same labelset. #10140
* [BUGFIX] UI: Fix bug that sets the range input to the resolution. #10227
* [BUGFIX] TSDB: Fix a query panic when `memory-snapshot-on-shutdown` is enabled. #10348
* [BUGFIX] Parser: Specify type in metadata parser errors. #10269
* [BUGFIX] Scrape: Fix label limit changes not applying. #10370

## 2.33.5 / 2022-03-08

The binaries published with this release are built with Go1.17.8 to avoid [CVE-2022-24921](https://cve.mitre.org/cgi-bin/cvename.cgi?name=CVE-2022-24921).

* [BUGFIX] Remote-write: Fix deadlock between adding to queue and getting batch. #10395

## 2.33.4 / 2022-02-22

* [BUGFIX] TSDB: Fix panic when m-mapping head chunks onto the disk. #10316

## 2.33.3 / 2022-02-11

* [BUGFIX] Azure SD: Fix a regression when public IP Address isn't set. #10289

## 2.33.2 / 2022-02-11

* [BUGFIX] Azure SD: Fix panic when public IP Address isn't set. #10280
* [BUGFIX] Remote-write: Fix deadlock when stopping a shard. #10279

## 2.33.1 / 2022-02-02

* [BUGFIX] SD: Fix _no such file or directory_ in K8s SD when not running inside K8s. #10235

## 2.33.0 / 2022-01-29

* [CHANGE] PromQL: Promote negative offset and `@` modifer to stable features. #10121
* [CHANGE] Web: Promote remote-write-receiver to stable. #10119
* [FEATURE] Config: Add `stripPort` template function. #10002
* [FEATURE] Promtool: Add cardinality analysis to `check metrics`, enabled by flag `--extended`. #10045
* [FEATURE] SD: Enable target discovery in own K8s namespace. #9881
* [FEATURE] SD: Add provider ID label in K8s SD. #9603
* [FEATURE] Web: Add limit field to the rules API. #10152
* [ENHANCEMENT] Remote-write: Avoid allocations by buffering concrete structs instead of interfaces. #9934
* [ENHANCEMENT] Remote-write: Log time series details for out-of-order samples in remote write receiver. #9894
* [ENHANCEMENT] Remote-write: Shard up more when backlogged. #9274
* [ENHANCEMENT] TSDB: Use simpler map key to improve exemplar ingest performance. #10111
* [ENHANCEMENT] TSDB: Avoid allocations when popping from the intersected postings heap. #10092
* [ENHANCEMENT] TSDB: Make chunk writing non-blocking, avoiding latency spikes in remote-write. #10051
* [ENHANCEMENT] TSDB: Improve label matching performance. #9907
* [ENHANCEMENT] UI: Optimize the service discovery page and add a search bar. #10131
* [ENHANCEMENT] UI: Optimize the target page and add a search bar. #10103
* [BUGFIX] Promtool: Make exit codes more consistent. #9861
* [BUGFIX] Promtool: Fix flakiness of rule testing. #8818
* [BUGFIX] Remote-write: Update `prometheus_remote_storage_queue_highest_sent_timestamp_seconds` metric when write irrecoverably fails. #10102
* [BUGFIX] Storage: Avoid panic in `BufferedSeriesIterator`. #9945
* [BUGFIX] TSDB: CompactBlockMetas should produce correct mint/maxt for overlapping blocks. #10108
* [BUGFIX] TSDB: Fix logging of exemplar storage size. #9938
* [BUGFIX] UI: Fix overlapping click targets for the alert state checkboxes. #10136
* [BUGFIX] UI: Fix _Unhealthy_ filter on target page to actually display only _Unhealthy_ targets. #10103
* [BUGFIX] UI: Fix autocompletion when expression is empty. #10053
* [BUGFIX] TSDB: Fix deadlock from simultaneous GC and write. #10166

## 2.32.1 / 2021-12-17

* [BUGFIX] Scrape: Fix reporting metrics when sample limit is reached during the report. #9996
* [BUGFIX] Scrape: Ensure that scrape interval and scrape timeout are always set. #10023
* [BUGFIX] TSDB: Expose and fix bug in iterators' `Seek()` method. #10030

## 2.32.0 / 2021-12-09

This release introduces the Prometheus Agent, a new mode of operation for
Prometheus optimized for remote-write only scenarios. In this mode, Prometheus
does not generate blocks on the local filesystem and is not queryable locally.
Enable with `--enable-feature=agent`.

Learn more about the Prometheus Agent in our [blog post](https://prometheus.io/blog/2021/11/16/agent/).

* [CHANGE] Remote-write: Change default max retry time from 100ms to 5 seconds. #9634
* [FEATURE] Agent: New mode of operation optimized for remote-write only scenarios, without local storage. Enable with `--enable-feature=agent`. #8785 #9851 #9664 #9939 #9941 #9943
* [FEATURE] Promtool: Add `promtool check service-discovery` command. #8970
* [FEATURE] UI: Add search in metrics dropdown. #9629
* [FEATURE] Templates: Add parseDuration to template functions. #8817
* [ENHANCEMENT] Promtool: Improve test output. #8064
* [ENHANCEMENT] PromQL: Use kahan summation for better numerical stability. #9588
* [ENHANCEMENT] Remote-write: Reuse memory for marshalling. #9412
* [ENHANCEMENT] Scrape: Add `scrape_body_size_bytes` scrape metric behind the `--enable-feature=extra-scrape-metrics` flag. #9569
* [ENHANCEMENT] TSDB: Add windows arm64 support. #9703
* [ENHANCEMENT] TSDB: Optimize query by skipping unneeded sorting in TSDB. #9673
* [ENHANCEMENT] Templates: Support int and uint as datatypes for template formatting. #9680
* [ENHANCEMENT] UI: Prefer `rate` over `rad`, `delta` over `deg`, and `count` over `cos` in autocomplete. #9688
* [ENHANCEMENT] Linode SD: Tune API request page sizes. #9779
* [BUGFIX] TSDB: Add more size checks when writing individual sections in the index. #9710
* [BUGFIX] PromQL: Make `deriv()` return zero values for constant series. #9728
* [BUGFIX] TSDB: Fix panic when checkpoint directory is empty. #9687
* [BUGFIX] TSDB: Fix panic, out of order chunks, and race warning during WAL replay. #9856
* [BUGFIX] UI: Correctly render links for targets with IPv6 addresses that contain a Zone ID. #9853
* [BUGFIX] Promtool: Fix checking of `authorization.credentials_file` and `bearer_token_file` fields. #9883
* [BUGFIX] Uyuni SD: Fix null pointer exception during initialization. #9924 #9950
* [BUGFIX] TSDB: Fix queries after a failed snapshot replay. #9980

## 2.31.2 / 2021-12-09

* [BUGFIX] TSDB: Fix queries after a failed snapshot replay. #9980

## 2.31.1 / 2021-11-05

* [BUGFIX] SD: Fix a panic when the experimental discovery manager receives
  targets during a reload. #9656

## 2.31.0 / 2021-11-02

* [CHANGE] UI: Remove standard PromQL editor in favour of the codemirror-based editor. #9452
* [FEATURE] PromQL: Add trigonometric functions and `atan2` binary operator. #9239 #9248 #9515
* [FEATURE] Remote: Add support for exemplar in the remote write receiver endpoint. #9319 #9414
* [FEATURE] SD: Add PuppetDB service discovery. #8883
* [FEATURE] SD: Add Uyuni service discovery. #8190
* [FEATURE] Web: Add support for security-related HTTP headers. #9546
* [ENHANCEMENT] Azure SD: Add `proxy_url`, `follow_redirects`, `tls_config`. #9267
* [ENHANCEMENT] Backfill: Add `--max-block-duration` in `promtool create-blocks-from rules`. #9511
* [ENHANCEMENT] Config: Print human-readable sizes with unit instead of raw numbers. #9361
* [ENHANCEMENT] HTTP: Re-enable HTTP/2. #9398
* [ENHANCEMENT] Kubernetes SD: Warn user if number of endpoints exceeds limit. #9467
* [ENHANCEMENT] OAuth2: Add TLS configuration to token requests. #9550
* [ENHANCEMENT] PromQL: Several optimizations. #9365 #9360 #9362 #9552
* [ENHANCEMENT] PromQL: Make aggregations deterministic in instant queries. #9459
* [ENHANCEMENT] Rules: Add the ability to limit number of alerts or series. #9260 #9541
* [ENHANCEMENT] SD: Experimental discovery manager to avoid restarts upon reload. Disabled by default, enable with flag `--enable-feature=new-service-discovery-manager`. #9349 #9537
* [ENHANCEMENT] UI: Debounce timerange setting changes. #9359
* [BUGFIX] Backfill: Apply rule labels after query labels. #9421
* [BUGFIX] Scrape: Resolve conflicts between multiple exported label prefixes. #9479 #9518
* [BUGFIX] Scrape: Restart scrape loops when `__scrape_interval__` is changed. #9551
* [BUGFIX] TSDB: Fix memory leak in samples deletion. #9151
* [BUGFIX] UI: Use consistent margin-bottom for all alert kinds. #9318

## 2.30.4 / 2021-12-09

* [BUGFIX] TSDB: Fix queries after a failed snapshot replay. #9980

## 2.30.3 / 2021-10-05

* [BUGFIX] TSDB: Fix panic on failed snapshot replay. #9438
* [BUGFIX] TSDB: Don't fail snapshot replay with exemplar storage disabled when the snapshot contains exemplars. #9438

## 2.30.2 / 2021-10-01

* [BUGFIX] TSDB: Don't error on overlapping m-mapped chunks during WAL replay. #9381

## 2.30.1 / 2021-09-28

* [ENHANCEMENT] Remote Write: Redact remote write URL when used for metric label. #9383
* [ENHANCEMENT] UI: Redact remote write URL and proxy URL passwords in the `/config` page. #9408
* [BUGFIX] promtool rules backfill: Prevent creation of data before the start time. #9339
* [BUGFIX] promtool rules backfill: Do not query after the end time. #9340
* [BUGFIX] Azure SD: Fix panic when no computername is set. #9387

## 2.30.0 / 2021-09-14

* [FEATURE] **experimental** TSDB: Snapshot in-memory chunks on shutdown for faster restarts. Behind `--enable-feature=memory-snapshot-on-shutdown` flag. #7229
* [FEATURE] **experimental** Scrape: Configure scrape interval and scrape timeout via relabeling using `__scrape_interval__` and `__scrape_timeout__` labels respectively. #8911
* [FEATURE] Scrape: Add `scrape_timeout_seconds` and `scrape_sample_limit` metric. Behind `--enable-feature=extra-scrape-metrics` flag to avoid additional cardinality by default. #9247 #9295
* [ENHANCEMENT] Scrape: Add `--scrape.timestamp-tolerance` flag to adjust scrape timestamp tolerance when enabled via `--scrape.adjust-timestamps`. #9283
* [ENHANCEMENT] Remote Write: Improve throughput when sending exemplars. #8921
* [ENHANCEMENT] TSDB: Optimise WAL loading by removing extra map and caching min-time #9160
* [ENHANCEMENT] promtool: Speed up checking for duplicate rules. #9262/#9306
* [ENHANCEMENT] Scrape: Reduce allocations when parsing the metrics. #9299
* [ENHANCEMENT] docker_sd: Support host network mode #9125
* [BUGFIX] Exemplars: Fix panic when resizing exemplar storage from 0 to a non-zero size. #9286
* [BUGFIX] TSDB: Correctly decrement `prometheus_tsdb_head_active_appenders` when the append has no samples. #9230
* [BUGFIX] promtool rules backfill: Return 1 if backfill was unsuccessful. #9303
* [BUGFIX] promtool rules backfill: Avoid creation of overlapping blocks. #9324
* [BUGFIX] config: Fix a panic when reloading configuration with a `null` relabel action. #9224

## 2.29.2 / 2021-08-27

* [BUGFIX] Fix Kubernetes SD failing to discover Ingress in Kubernetes v1.22. #9205
* [BUGFIX] Fix data race in loading write-ahead-log (WAL). #9259

## 2.29.1 / 2021-08-11

* [BUGFIX] tsdb: align atomically accessed int64 to prevent panic in 32-bit
  archs. #9192

## 2.29.0 / 2021-08-11

Note for macOS users: Due to [changes in the upcoming Go 1.17](https://tip.golang.org/doc/go1.17#darwin),
this is the last Prometheus release that supports macOS 10.12 Sierra.

* [CHANGE] Promote `--storage.tsdb.allow-overlapping-blocks` flag to stable. #9117
* [CHANGE] Promote `--storage.tsdb.retention.size` flag to stable. #9004
* [FEATURE] Add Kuma service discovery. #8844
* [FEATURE] Add `present_over_time` PromQL function. #9097
* [FEATURE] Allow configuring exemplar storage via file and make it reloadable. #8974
* [FEATURE] UI: Allow selecting time range with mouse drag. #8977
* [FEATURE] promtool: Add feature flags flag `--enable-feature`. #8958
* [FEATURE] promtool: Add file_sd file validation. #8950
* [ENHANCEMENT] Reduce blocking of outgoing remote write requests from series garbage collection. #9109
* [ENHANCEMENT] Improve write-ahead-log decoding performance. #9106
* [ENHANCEMENT] Improve append performance in TSDB by reducing mutexes usage. #9061
* [ENHANCEMENT] Allow configuring `max_samples_per_send` for remote write metadata. #8959
* [ENHANCEMENT] Add `__meta_gce_interface_ipv4_<name>` meta label to GCE discovery. #8978
* [ENHANCEMENT] Add `__meta_ec2_availability_zone_id` meta label to EC2 discovery. #8896
* [ENHANCEMENT] Add `__meta_azure_machine_computer_name` meta label to Azure discovery. #9112
* [ENHANCEMENT] Add `__meta_hetzner_hcloud_labelpresent_<labelname>` meta label to Hetzner discovery. #9028
* [ENHANCEMENT] promtool: Add compaction efficiency to `promtool tsdb analyze` reports. #8940
* [ENHANCEMENT] promtool: Allow configuring max block duration for backfilling via `--max-block-duration` flag. #8919
* [ENHANCEMENT] UI: Add sorting and filtering to flags page. #8988
* [ENHANCEMENT] UI: Improve alerts page rendering performance. #9005
* [BUGFIX] Log when total symbol size exceeds 2^32 bytes, causing compaction to fail, and skip compaction. #9104
* [BUGFIX] Fix incorrect `target_limit` reloading of zero value. #9120
* [BUGFIX] Fix head GC and pending readers race condition. #9081
* [BUGFIX] Fix timestamp handling in OpenMetrics parser. #9008
* [BUGFIX] Fix potential duplicate metrics in `/federate` endpoint when specifying multiple matchers. #8885
* [BUGFIX] Fix server configuration and validation for authentication via client cert. #9123
* [BUGFIX] Allow `start` and `end` again as label names in PromQL queries. They were disallowed since the introduction of @ timestamp feature. #9119

## 2.28.1 / 2021-07-01

* [BUGFIX]: HTTP SD: Allow `charset` specification in `Content-Type` header. #8981
* [BUGFIX]: HTTP SD: Fix handling of disappeared target groups. #9019
* [BUGFIX]: Fix incorrect log-level handling after moving to go-kit/log. #9021

## 2.28.0 / 2021-06-21

* [CHANGE] UI: Make the new experimental PromQL editor the default. #8925
* [FEATURE] Linode SD: Add Linode service discovery. #8846
* [FEATURE] HTTP SD: Add generic HTTP-based service discovery. #8839
* [FEATURE] Kubernetes SD: Allow configuring API Server access via a kubeconfig file. #8811
* [FEATURE] UI: Add exemplar display support to the graphing interface. #8832 #8945 #8929
* [FEATURE] Consul SD: Add namespace support for Consul Enterprise. #8900
* [ENHANCEMENT] Promtool: Allow silencing output when importing / backfilling data. #8917
* [ENHANCEMENT] Consul SD: Support reading tokens from file. #8926
* [ENHANCEMENT] Rules: Add a new `.ExternalURL` alert field templating variable, containing the external URL of the Prometheus server. #8878
* [ENHANCEMENT] Scrape: Add experimental `body_size_limit` scrape configuration setting to limit the allowed response body size for target scrapes. #8833 #8886
* [ENHANCEMENT] Kubernetes SD: Add ingress class name label for ingress discovery. #8916
* [ENHANCEMENT] UI: Show a startup screen with progress bar when the TSDB is not ready yet. #8662 #8908 #8909 #8946
* [ENHANCEMENT] SD: Add a target creation failure counter `prometheus_target_sync_failed_total` and improve target creation failure handling. #8786
* [ENHANCEMENT] TSDB: Improve validation of exemplar label set length. #8816
* [ENHANCEMENT] TSDB: Add a `prometheus_tsdb_clean_start` metric that indicates whether a TSDB lockfile from a previous run still existed upon startup. #8824
* [BUGFIX] UI: In the experimental PromQL editor, fix autocompletion and parsing for special float values and improve series metadata fetching. #8856
* [BUGFIX] TSDB: When merging chunks, split resulting chunks if they would contain more than the maximum of 120 samples. #8582
* [BUGFIX] SD: Fix the computation of the `prometheus_sd_discovered_targets` metric when using multiple service discoveries. #8828

## 2.27.1 / 2021-05-18

This release contains a bug fix for a security issue in the API endpoint. An
attacker can craft a special URL that redirects a user to any endpoint via an
HTTP 302 response. See the [security advisory][GHSA-vx57-7f4q-fpc7] for more details.

[GHSA-vx57-7f4q-fpc7]:https://github.com/prometheus/prometheus/security/advisories/GHSA-vx57-7f4q-fpc7

This vulnerability has been reported by Aaron Devaney from MDSec.

* [BUGFIX] SECURITY: Fix arbitrary redirects under the /new endpoint (CVE-2021-29622)

## 2.27.0 / 2021-05-12

* [CHANGE] Remote write: Metric `prometheus_remote_storage_samples_bytes_total` renamed to `prometheus_remote_storage_bytes_total`. #8296
* [FEATURE] Promtool: Retroactive rule evaluation functionality. #7675
* [FEATURE] Configuration: Environment variable expansion for external labels. Behind `--enable-feature=expand-external-labels` flag. #8649
* [FEATURE] TSDB: Add a flag(`--storage.tsdb.max-block-chunk-segment-size`) to control the max chunks file size of the blocks for small Prometheus instances. #8478
* [FEATURE] UI: Add a dark theme. #8604
* [FEATURE] AWS Lightsail Discovery: Add AWS Lightsail Discovery. #8693
* [FEATURE] Docker Discovery: Add Docker Service Discovery. #8629
* [FEATURE] OAuth: Allow OAuth 2.0 to be used anywhere an HTTP client is used. #8761
* [FEATURE] Remote Write: Send exemplars via remote write. Experimental and disabled by default. #8296
* [ENHANCEMENT] Digital Ocean Discovery: Add `__meta_digitalocean_vpc` label. #8642
* [ENHANCEMENT] Scaleway Discovery: Read Scaleway secret from a file. #8643
* [ENHANCEMENT] Scrape: Add configurable limits for label size and count. #8777
* [ENHANCEMENT] UI: Add 16w and 26w time range steps. #8656
* [ENHANCEMENT] Templating: Enable parsing strings in `humanize` functions. #8682
* [BUGFIX] UI: Provide errors instead of blank page on TSDB Status Page. #8654 #8659
* [BUGFIX] TSDB: Do not panic when writing very large records to the WAL. #8790
* [BUGFIX] TSDB: Avoid panic when mmaped memory is referenced after the file is closed. #8723
* [BUGFIX] Scaleway Discovery: Fix nil pointer dereference. #8737
* [BUGFIX] Consul Discovery: Restart no longer required after config update with no targets. #8766

## 2.26.0 / 2021-03-31

Prometheus is now built and supporting Go 1.16 (#8544). This reverts the memory release pattern added in Go 1.12. This makes common RSS usage metrics showing more accurate number for actual memory used by Prometheus. You can read more details [here](https://www.bwplotka.dev/2019/golang-memory-monitoring/).

Note that from this release Prometheus is using Alertmanager v2 by default.

* [CHANGE] Alerting: Using Alertmanager v2 API by default. #8626
* [CHANGE] Prometheus/Promtool: As agreed on dev summit, binaries are now printing help and usage to stdout instead of stderr. #8542
* [FEATURE] Remote: Add support for AWS SigV4 auth method for remote_write. #8509
* [FEATURE] Scaleway Discovery: Add Scaleway Service Discovery. #8555
* [FEATURE] PromQL: Allow negative offsets. Behind `--enable-feature=promql-negative-offset` flag. #8487
* [FEATURE] **experimental** Exemplars: Add in-memory storage for exemplars. Behind `--enable-feature=exemplar-storage` flag. #6635
* [FEATURE] UI: Add advanced auto-completion, syntax highlighting and linting to graph page query input. #8634
* [ENHANCEMENT] Digital Ocean Discovery: Add `__meta_digitalocean_image` label. #8497
* [ENHANCEMENT] PromQL: Add `last_over_time`, `sgn`, `clamp` functions. #8457
* [ENHANCEMENT] Scrape: Add support for specifying type of Authorization header credentials with Bearer by default. #8512
* [ENHANCEMENT] Scrape: Add `follow_redirects` option to scrape configuration. #8546
* [ENHANCEMENT] Remote: Allow retries on HTTP 429 response code for remote_write. Disabled by default. See [configuration docs](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write) for details. #8237 #8477
* [ENHANCEMENT] Remote: Allow configuring custom headers for remote_read. See [configuration docs](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_read) for details. #8516
* [ENHANCEMENT] UI: Hitting Enter now triggers new query. #8581
* [ENHANCEMENT] UI: Better handling of long rule and names on the `/rules` and `/targets` pages. #8608 #8609
* [ENHANCEMENT] UI: Add collapse/expand all button on the `/targets` page. #8486
* [BUGFIX] TSDB: Eager deletion of removable blocks on every compaction, saving disk peak space usage. #8007
* [BUGFIX] PromQL: Fix parser support for special characters like`炬`. #8517
* [BUGFIX] Rules: Update rule health for append/commit fails. #8619

## 2.25.2 / 2021-03-16

* [BUGFIX] Fix the ingestion of scrapes when the wall clock changes, e.g. on suspend. #8601

## 2.25.1 / 2021-03-14

* [BUGFIX] Fix a crash in `promtool` when a subquery with default resolution is used. #8569
* [BUGFIX] Fix a bug that could return duplicate datapoints in queries. #8591
* [BUGFIX] Fix crashes with arm64 when compiled with go1.16. #8593

## 2.25.0 / 2021-02-17

This release includes a new `--enable-feature=` flag that enables
experimental features. Such features might be changed or removed in the future.

In the next minor release (2.26), Prometheus will use the Alertmanager API v2.
It will be done by defaulting `alertmanager_config.api_version` to `v2`.
Alertmanager API v2 was released in Alertmanager v0.16.0 (released in January
2019).

* [FEATURE] **experimental** API: Accept remote_write requests. Behind the --enable-feature=remote-write-receiver flag. #8424
* [FEATURE] **experimental** PromQL: Add `@ <timestamp>` modifier. Behind the --enable-feature=promql-at-modifier flag. #8121 #8436 #8425
* [ENHANCEMENT] Add optional name property to testgroup for better test failure output. #8440
* [ENHANCEMENT] Add warnings into React Panel on the Graph page. #8427
* [ENHANCEMENT] TSDB: Increase the number of buckets for the compaction duration metric. #8342
* [ENHANCEMENT] Remote: Allow passing along custom remote_write HTTP headers. #8416
* [ENHANCEMENT] Mixins: Scope grafana configuration. #8332
* [ENHANCEMENT] Kubernetes SD: Add endpoint labels metadata. #8273
* [ENHANCEMENT] UI: Expose total number of label pairs in head in TSDB stats page. #8343
* [ENHANCEMENT] TSDB: Reload blocks every minute, to detect new blocks and enforce retention more often. #8340
* [BUGFIX] API: Fix global URL when external address has no port. #8359
* [BUGFIX] Backfill: Fix error message handling. #8432
* [BUGFIX] Backfill: Fix "add sample: out of bounds" error when series span an entire block. #8476
* [BUGFIX] Deprecate unused flag --alertmanager.timeout. #8407
* [BUGFIX] Mixins: Support remote-write metrics renamed in v2.23 in alerts. #8423
* [BUGFIX] Remote: Fix garbage collection of dropped series in remote write. #8387
* [BUGFIX] Remote: Log recoverable remote write errors as warnings. #8412
* [BUGFIX] TSDB: Remove pre-2.21 temporary blocks on start. #8353.
* [BUGFIX] UI: Fix duplicated keys on /targets page. #8456
* [BUGFIX] UI: Fix label name leak into class name. #8459

## 2.24.1 / 2021-01-20

* [ENHANCEMENT] Cache basic authentication results to significantly improve performance of HTTP endpoints (via an update of prometheus/exporter-toolkit).
* [BUGFIX] Prevent user enumeration by timing requests sent to authenticated HTTP endpoints (via an update of prometheus/exporter-toolkit).

## 2.24.0 / 2021-01-06

* [FEATURE] Add TLS and basic authentication to HTTP endpoints. #8316
* [FEATURE] promtool: Add `check web-config` subcommand to check web config files. #8319
* [FEATURE] promtool: Add `tsdb create-blocks-from openmetrics` subcommand to backfill metrics data from an OpenMetrics file. #8084
* [ENHANCEMENT] HTTP API: Fast-fail queries with only empty matchers. #8288
* [ENHANCEMENT] HTTP API: Support matchers for labels API. #8301
* [ENHANCEMENT] promtool: Improve checking of URLs passed on the command line. #7956
* [ENHANCEMENT] SD: Expose IPv6 as a label in EC2 SD. #7086
* [ENHANCEMENT] SD: Reuse EC2 client, reducing frequency of requesting credentials. #8311
* [ENHANCEMENT] TSDB: Add logging when compaction takes more than the block time range. #8151
* [ENHANCEMENT] TSDB: Avoid unnecessary GC runs after compaction. #8276
* [BUGFIX] HTTP API: Avoid double-closing of channel when quitting multiple times via HTTP. #8242
* [BUGFIX] SD: Ignore CNAME records in DNS SD to avoid spurious `Invalid SRV record` warnings. #8216
* [BUGFIX] SD: Avoid config error triggered by valid label selectors in Kubernetes SD. #8285

## 2.23.0 / 2020-11-26

* [CHANGE] UI: Make the React UI default. #8142
* [CHANGE] Remote write: The following metrics were removed/renamed in remote write. #6815
  * `prometheus_remote_storage_succeeded_samples_total` was removed and `prometheus_remote_storage_samples_total` was introduced for all the samples attempted to send.
  * `prometheus_remote_storage_sent_bytes_total` was removed and replaced with `prometheus_remote_storage_samples_bytes_total` and `prometheus_remote_storage_metadata_bytes_total`.
  * `prometheus_remote_storage_failed_samples_total` -> `prometheus_remote_storage_samples_failed_total` .
  * `prometheus_remote_storage_retried_samples_total` -> `prometheus_remote_storage_samples_retried_total`.
  * `prometheus_remote_storage_dropped_samples_total` -> `prometheus_remote_storage_samples_dropped_total`.
  * `prometheus_remote_storage_pending_samples` -> `prometheus_remote_storage_samples_pending`.
* [CHANGE] Remote: Do not collect non-initialized timestamp metrics. #8060
* [FEATURE] [EXPERIMENTAL] Remote write: Allow metric metadata to be propagated via remote write. The following new metrics were introduced: `prometheus_remote_storage_metadata_total`, `prometheus_remote_storage_metadata_failed_total`, `prometheus_remote_storage_metadata_retried_total`, `prometheus_remote_storage_metadata_bytes_total`. #6815
* [ENHANCEMENT] Remote write: Added a metric `prometheus_remote_storage_max_samples_per_send` for remote write. #8102
* [ENHANCEMENT] TSDB: Make the snapshot directory name always the same length. #8138
* [ENHANCEMENT] TSDB: Create a checkpoint only once at the end of all head compactions. #8067
* [ENHANCEMENT] TSDB: Avoid Series API from hitting the chunks. #8050
* [ENHANCEMENT] TSDB: Cache label name and last value when adding series during compactions making compactions faster. #8192
* [ENHANCEMENT] PromQL: Improved performance of Hash method making queries a bit faster. #8025
* [ENHANCEMENT] promtool: `tsdb list` now prints block sizes. #7993
* [ENHANCEMENT] promtool: Calculate mint and maxt per test avoiding unnecessary calculations. #8096
* [ENHANCEMENT] SD: Add filtering of services to Docker Swarm SD. #8074
* [BUGFIX] React UI: Fix button display when there are no panels. #8155
* [BUGFIX] PromQL: Fix timestamp() method for vector selector inside parenthesis. #8164
* [BUGFIX] PromQL: Don't include rendered expression on PromQL parse errors. #8177
* [BUGFIX] web: Fix panic with double close() of channel on calling `/-/quit/`. #8166
* [BUGFIX] TSDB: Fixed WAL corruption on partial writes within a page causing `invalid checksum` error on WAL replay. #8125
* [BUGFIX] Update config metrics `prometheus_config_last_reload_successful` and `prometheus_config_last_reload_success_timestamp_seconds` right after initial validation before starting TSDB.
* [BUGFIX] promtool: Correctly detect duplicate label names in exposition.

## 2.22.2 / 2020-11-16

* [BUGFIX] Fix race condition in syncing/stopping/reloading scrapers. #8176

## 2.22.1 / 2020-11-03

* [BUGFIX] Fix potential "mmap: invalid argument" errors in loading the head chunks, after an unclean shutdown, by performing read repairs. #8061
* [BUGFIX] Fix serving metrics and API when reloading scrape config. #8104
* [BUGFIX] Fix head chunk size calculation for size based retention. #8139

## 2.22.0 / 2020-10-07

As announced in the 2.21.0 release notes, the experimental gRPC API v2 has been
removed.

* [CHANGE] web: Remove APIv2. #7935
* [ENHANCEMENT] React UI: Implement missing TSDB head stats section. #7876
* [ENHANCEMENT] UI: Add Collapse all button to targets page. #6957
* [ENHANCEMENT] UI: Clarify alert state toggle via checkbox icon. #7936
* [ENHANCEMENT] Add `rule_group_last_evaluation_samples` and `prometheus_tsdb_data_replay_duration_seconds` metrics. #7737 #7977
* [ENHANCEMENT] Gracefully handle unknown WAL record types. #8004
* [ENHANCEMENT] Issue a warning for 64 bit systems running 32 bit binaries. #8012
* [BUGFIX] Adjust scrape timestamps to align them to the intended schedule, effectively reducing block size. Workaround for a regression in go1.14+. #7976
* [BUGFIX] promtool: Ensure alert rules are marked as restored in unit tests. #7661
* [BUGFIX] Eureka: Fix service discovery when compiled in 32-bit. #7964
* [BUGFIX] Don't do literal regex matching optimisation when case insensitive. #8013
* [BUGFIX] Fix classic UI sometimes running queries for instant query when in range query mode. #7984

## 2.21.0 / 2020-09-11

This release is built with Go 1.15, which deprecates [X.509 CommonName](https://golang.org/doc/go1.15#commonname)
in TLS certificates validation.

In the unlikely case that you use the gRPC API v2 (which is limited to TSDB
admin commands), please note that we will remove this experimental API in the
next minor release 2.22.

* [CHANGE] Disable HTTP/2 because of concerns with the Go HTTP/2 client. #7588 #7701
* [CHANGE] PromQL: `query_log_file` path is now relative to the config file. #7701
* [CHANGE] Promtool: Replace the tsdb command line tool by a promtool tsdb subcommand. #6088
* [CHANGE] Rules: Label `rule_group_iterations` metric with group name. #7823
* [FEATURE] Eureka SD: New service discovery. #3369
* [FEATURE] Hetzner SD: New service discovery. #7822
* [FEATURE] Kubernetes SD: Support Kubernetes EndpointSlices. #6838
* [FEATURE] Scrape: Add per scrape-config targets limit. #7554
* [ENHANCEMENT] Support composite durations in PromQL, config and UI, e.g. 1h30m. #7713 #7833
* [ENHANCEMENT] DNS SD: Add SRV record target and port meta labels. #7678
* [ENHANCEMENT] Docker Swarm SD: Support tasks and service without published ports. #7686
* [ENHANCEMENT] PromQL: Reduce the amount of data queried by remote read when a subquery has an offset. #7667
* [ENHANCEMENT] Promtool: Add `--time` option to query instant command. #7829
* [ENHANCEMENT] UI: Respect the `--web.page-title` parameter in the React UI. #7607
* [ENHANCEMENT] UI: Add duration, labels, annotations to alerts page in the React UI. #7605
* [ENHANCEMENT] UI: Add duration on the React UI rules page, hide annotation and labels if empty. #7606
* [BUGFIX] API: Deduplicate series in /api/v1/series. #7862
* [BUGFIX] PromQL: Drop metric name in bool comparison between two instant vectors. #7819
* [BUGFIX] PromQL: Exit with an error when time parameters can't be parsed. #7505
* [BUGFIX] Remote read: Re-add accidentally removed tracing for remote-read requests. #7916
* [BUGFIX] Rules: Detect extra fields in rule files. #7767
* [BUGFIX] Rules: Disallow overwriting the metric name in the `labels` section of recording rules. #7787
* [BUGFIX] Rules: Keep evaluation timestamp across reloads. #7775
* [BUGFIX] Scrape: Do not stop scrapes in progress during reload. #7752
* [BUGFIX] TSDB: Fix `chunks.HeadReadWriter: maxt of the files are not set` error. #7856
* [BUGFIX] TSDB: Delete blocks atomically to prevent corruption when there is a panic/crash during deletion. #7772
* [BUGFIX] Triton SD: Fix a panic when triton_sd_config is nil. #7671
* [BUGFIX] UI: Fix react UI bug with series going on and off. #7804
* [BUGFIX] UI: Fix styling bug for target labels with special names in React UI. #7902
* [BUGFIX] Web: Stop CMUX and GRPC servers even with stale connections, preventing the server to stop on SIGTERM. #7810

## 2.20.1 / 2020-08-05

* [BUGFIX] SD: Reduce the Consul watch timeout to 2m and adjust the request timeout accordingly. #7724

## 2.20.0 / 2020-07-22

This release changes WAL compression from opt-in to default. WAL compression will prevent a downgrade to v2.10 or earlier without deleting the WAL. Disable WAL compression explicitly by setting the command line flag `--no-storage.tsdb.wal-compression` if you require downgrading to v2.10 or earlier.

* [CHANGE] promtool: Changed rule numbering from 0-based to 1-based when reporting rule errors. #7495
* [CHANGE] Remote read: Added `prometheus_remote_storage_read_queries_total` counter and `prometheus_remote_storage_read_request_duration_seconds` histogram, removed `prometheus_remote_storage_remote_read_queries_total` counter.
* [CHANGE] Remote write: Added buckets for longer durations to `prometheus_remote_storage_sent_batch_duration_seconds` histogram.
* [CHANGE] TSDB: WAL compression is enabled by default. #7410
* [FEATURE] PromQL: Added `group()` aggregator. #7480
* [FEATURE] SD: Added Docker Swarm SD. #7420
* [FEATURE] SD: Added DigitalOcean SD. #7407
* [FEATURE] SD: Added Openstack config option to query alternative endpoints. #7494
* [ENHANCEMENT] Configuration: Exit early on invalid config file and signal it with exit code 2. #7399
* [ENHANCEMENT] PromQL: `without` is now a valid metric identifier. #7533
* [ENHANCEMENT] PromQL: Optimized regex label matching for literals within the pattern or as prefix/suffix. #7453 #7503
* [ENHANCEMENT] promtool: Added time range parameters for labels API in promtool. #7463
* [ENHANCEMENT] Remote write: Include samples waiting in channel in pending samples metric. Log number of dropped samples on hard shutdown. #7335
* [ENHANCEMENT] Scrape: Ingest synthetic scrape report metrics atomically with the corresponding scraped metrics. #7562
* [ENHANCEMENT] SD: Reduce timeouts for Openstack SD. #7507
* [ENHANCEMENT] SD: Use 10m timeout for Consul watches. #7423
* [ENHANCEMENT] SD: Added AMI meta label for EC2 SD. #7386
* [ENHANCEMENT] TSDB: Increment WAL corruption metric also on WAL corruption during checkpointing. #7491
* [ENHANCEMENT] TSDB: Improved query performance for high-cardinality labels. #7448
* [ENHANCEMENT] UI: Display dates as well as timestamps in status page. #7544
* [ENHANCEMENT] UI: Improved scrolling when following hash-fragment links. #7456
* [ENHANCEMENT] UI: React UI renders numbers in alerts in a more human-readable way. #7426
* [BUGFIX] API: Fixed error status code in the query API. #7435
* [BUGFIX] PromQL: Fixed `avg` and `avg_over_time` for NaN, Inf, and float64 overflows. #7346
* [BUGFIX] PromQL: Fixed off-by-one error in `histogram_quantile`. #7393
* [BUGFIX] promtool: Support extended durations in rules unit tests. #6297
* [BUGFIX] Scrape: Fix undercounting for `scrape_samples_post_metric_relabeling` in case of errors. #7342
* [BUGFIX] TSDB: Don't panic on WAL corruptions. #7550
* [BUGFIX] TSDB: Avoid leaving behind empty files in `chunks_head`, causing startup failures. #7573
* [BUGFIX] TSDB: Fixed race between compact (gc, populate) and head append causing unknown symbol error. #7560
* [BUGFIX] TSDB: Fixed unknown symbol error during head compaction. #7526
* [BUGFIX] TSDB: Fixed panic during TSDB metric registration. #7501
* [BUGFIX] TSDB: Fixed `--limit` command line flag in `tsdb` tool. #7430

## 2.19.3 / 2020-07-24

* [BUGFIX] TSDB: Don't panic on WAL corruptions. #7550
* [BUGFIX] TSDB: Avoid leaving behind empty files in chunks_head, causing startup failures. #7573

## 2.19.2 / 2020-06-26

* [BUGFIX] Remote Write: Fix panic when reloading config with modified queue parameters. #7452

## 2.19.1 / 2020-06-18

* [BUGFIX] TSDB: Fix m-map file truncation leading to unsequential files. #7414

## 2.19.0 / 2020-06-09

* [FEATURE] TSDB: Memory-map full chunks of Head (in-memory) block from disk. This reduces memory footprint and makes restarts faster. #6679
* [ENHANCEMENT] Discovery: Added discovery support for Triton global zones. #7250
* [ENHANCEMENT] Increased alert resend delay to be more tolerant towards failures. #7228
* [ENHANCEMENT] Remote Read: Added `prometheus_remote_storage_remote_read_queries_total` counter to count total number of remote read queries. #7328
* [ENHANCEMEMT] Added time range parameters for label names and label values API. #7288
* [ENHANCEMENT] TSDB: Reduced contention in isolation for high load. #7332
* [BUGFIX] PromQL: Eliminated collision while checking for duplicate labels. #7058
* [BUGFIX] React UI: Don't null out data when clicking on the current tab. #7243
* [BUGFIX] PromQL: Correctly track number of samples for a query. #7307
* [BUGFIX] PromQL: Return NaN when histogram buckets have 0 observations. #7318

## 2.18.2 / 2020-06-09

* [BUGFIX] TSDB: Fix incorrect query results when using Prometheus with remote reads configured #7361

## 2.18.1 / 2020-05-07

* [BUGFIX] TSDB: Fixed snapshot API. #7217

## 2.18.0 / 2020-05-05

* [CHANGE] Federation: Only use local TSDB for federation (ignore remote read). #7096
* [CHANGE] Rules: `rule_evaluations_total` and `rule_evaluation_failures_total` have a `rule_group` label now. #7094
* [FEATURE] Tracing: Added experimental Jaeger support #7148
* [ENHANCEMENT] TSDB: Significantly reduce WAL size kept around after a block cut. #7098
* [ENHANCEMENT] Discovery: Add `architecture` meta label for EC2. #7000
* [BUGFIX] UI: Fixed wrong MinTime reported by /status. #7182
* [BUGFIX] React UI: Fixed multiselect legend on OSX. #6880
* [BUGFIX] Remote Write: Fixed blocked resharding edge case. #7122
* [BUGFIX] Remote Write: Fixed remote write not updating on relabel configs change. #7073

## 2.17.2 / 2020-04-20

* [BUGFIX] Federation: Register federation metrics #7081
* [BUGFIX] PromQL: Fix panic in parser error handling #7132
* [BUGFIX] Rules: Fix reloads hanging when deleting a rule group that is being evaluated #7138
* [BUGFIX] TSDB: Fix a memory leak when prometheus starts with an empty TSDB WAL #7135
* [BUGFIX] TSDB: Make isolation more robust to panics in web handlers #7129 #7136

## 2.17.1 / 2020-03-26

* [BUGFIX] TSDB: Fix query performance regression that increased memory and CPU usage #7051

## 2.17.0 / 2020-03-24

This release implements isolation in TSDB. API queries and recording rules are
guaranteed to only see full scrapes and full recording rules. This comes with a
certain overhead in resource usage. Depending on the situation, there might be
some increase in memory usage, CPU usage, or query latency.

* [FEATURE] TSDB: Support isolation #6841
* [ENHANCEMENT] PromQL: Allow more keywords as metric names #6933
* [ENHANCEMENT] React UI: Add normalization of localhost URLs in targets page #6794
* [ENHANCEMENT] Remote read: Read from remote storage concurrently #6770
* [ENHANCEMENT] Rules: Mark deleted rule series as stale after a reload #6745
* [ENHANCEMENT] Scrape: Log scrape append failures as debug rather than warn #6852
* [ENHANCEMENT] TSDB: Improve query performance for queries that partially hit the head #6676
* [ENHANCEMENT] Consul SD: Expose service health as meta label #5313
* [ENHANCEMENT] EC2 SD: Expose EC2 instance lifecycle as meta label #6914
* [ENHANCEMENT] Kubernetes SD: Expose service type as meta label for K8s service role #6684
* [ENHANCEMENT] Kubernetes SD: Expose label_selector and field_selector #6807
* [ENHANCEMENT] Openstack SD: Expose hypervisor id as meta label #6962
* [BUGFIX] PromQL: Do not escape HTML-like chars in query log #6834 #6795
* [BUGFIX] React UI: Fix data table matrix values #6896
* [BUGFIX] React UI: Fix new targets page not loading when using non-ASCII characters #6892
* [BUGFIX] Remote read: Fix duplication of metrics read from remote storage with external labels #6967 #7018
* [BUGFIX] Remote write: Register WAL watcher and live reader metrics for all remotes, not just the first one #6998
* [BUGFIX] Scrape: Prevent removal of metric names upon relabeling #6891
* [BUGFIX] Scrape: Fix 'superfluous response.WriteHeader call' errors when scrape fails under some circonstances #6986
* [BUGFIX] Scrape: Fix crash when reloads are separated by two scrape intervals #7011

## 2.16.0 / 2020-02-13

* [FEATURE] React UI: Support local timezone on /graph #6692
* [FEATURE] PromQL: add absent_over_time query function #6490
* [FEATURE] Adding optional logging of queries to their own file #6520
* [ENHANCEMENT] React UI: Add support for rules page and "Xs ago" duration displays #6503
* [ENHANCEMENT] React UI: alerts page, replace filtering togglers tabs with checkboxes #6543
* [ENHANCEMENT] TSDB: Export metric for WAL write errors #6647
* [ENHANCEMENT] TSDB: Improve query performance for queries that only touch the most recent 2h of data. #6651
* [ENHANCEMENT] PromQL: Refactoring in parser errors to improve error messages #6634
* [ENHANCEMENT] PromQL: Support trailing commas in grouping opts #6480
* [ENHANCEMENT] Scrape: Reduce memory usage on reloads by reusing scrape cache #6670
* [ENHANCEMENT] Scrape: Add metrics to track bytes and entries in the metadata cache #6675
* [ENHANCEMENT] promtool: Add support for line-column numbers for invalid rules output #6533
* [ENHANCEMENT] Avoid restarting rule groups when it is unnecessary #6450
* [BUGFIX] React UI: Send cookies on fetch() on older browsers #6553
* [BUGFIX] React UI: adopt grafana flot fix for stacked graphs #6603
* [BUFGIX] React UI: broken graph page browser history so that back button works as expected #6659
* [BUGFIX] TSDB: ensure compactionsSkipped metric is registered, and log proper error if one is returned from head.Init #6616
* [BUGFIX] TSDB: return an error on ingesting series with duplicate labels #6664
* [BUGFIX] PromQL: Fix unary operator precedence #6579
* [BUGFIX] PromQL: Respect query.timeout even when we reach query.max-concurrency #6712
* [BUGFIX] PromQL: Fix string and parentheses handling in engine, which affected React UI #6612
* [BUGFIX] PromQL: Remove output labels returned by absent() if they are produced by multiple identical label matchers #6493
* [BUGFIX] Scrape: Validate that OpenMetrics input ends with `# EOF` #6505
* [BUGFIX] Remote read: return the correct error if configs can't be marshal'd to JSON #6622
* [BUGFIX] Remote write: Make remote client `Store` use passed context, which can affect shutdown timing #6673
* [BUGFIX] Remote write: Improve sharding calculation in cases where we would always be consistently behind by tracking pendingSamples #6511
* [BUGFIX] Ensure prometheus_rule_group metrics are deleted when a rule group is removed #6693

## 2.15.2 / 2020-01-06

* [BUGFIX] TSDB: Fixed support for TSDB blocks built with Prometheus before 2.1.0. #6564
* [BUGFIX] TSDB: Fixed block compaction issues on Windows. #6547

## 2.15.1 / 2019-12-25

* [BUGFIX] TSDB: Fixed race on concurrent queries against same data. #6512

## 2.15.0 / 2019-12-23

* [CHANGE] Discovery: Removed `prometheus_sd_kubernetes_cache_*` metrics. Additionally `prometheus_sd_kubernetes_workqueue_latency_seconds` and `prometheus_sd_kubernetes_workqueue_work_duration_seconds` metrics now show correct values in seconds. #6393
* [CHANGE] Remote write: Changed `query` label on `prometheus_remote_storage_*` metrics to `remote_name` and `url`. #6043
* [FEATURE] API: Added new endpoint for exposing per metric metadata `/metadata`. #6420 #6442
* [ENHANCEMENT] TSDB: Significantly reduced memory footprint of loaded TSDB blocks. #6418 #6461
* [ENHANCEMENT] TSDB: Significantly optimized what we buffer during compaction which should result in lower memory footprint during compaction. #6422 #6452 #6468 #6475
* [ENHANCEMENT] TSDB: Improve replay latency. #6230
* [ENHANCEMENT] TSDB: WAL size is now used for size based retention calculation. #5886
* [ENHANCEMENT] Remote read: Added query grouping and range hints to the remote read request #6401
* [ENHANCEMENT] Remote write: Added `prometheus_remote_storage_sent_bytes_total` counter per queue. #6344
* [ENHANCEMENT] promql: Improved PromQL parser performance. #6356
* [ENHANCEMENT] React UI: Implemented missing pages like `/targets` #6276, TSDB status page #6281 #6267 and many other fixes and performance improvements.
* [ENHANCEMENT] promql: Prometheus now accepts spaces between time range and square bracket. e.g `[ 5m]` #6065
* [BUGFIX] Config: Fixed alertmanager configuration to not miss targets when configurations are similar. #6455
* [BUGFIX] Remote write: Value of `prometheus_remote_storage_shards_desired` gauge shows raw value of desired shards and it's updated correctly. #6378
* [BUGFIX] Rules: Prometheus now fails the evaluation of rules and alerts where metric results collide with labels specified in `labels` field. #6469
* [BUGFIX] API: Targets Metadata API `/targets/metadata` now accepts empty `match_targets` parameter as in the spec. #6303

## 2.14.0 / 2019-11-11

* [SECURITY/BUGFIX] UI: Ensure warnings from the API are escaped. #6279
* [FEATURE] API: `/api/v1/status/runtimeinfo` and `/api/v1/status/buildinfo` endpoints added for use by the React UI. #6243
* [FEATURE] React UI: implement the new experimental React based UI. #5694 and many more
  * Can be found by under `/new`.
  * Not all pages are implemented yet.
* [FEATURE] Status: Cardinality statistics added to the Runtime & Build Information page. #6125
* [ENHANCEMENT/BUGFIX] Remote write: fix delays in remote write after a compaction. #6021
* [ENHANCEMENT] UI: Alerts can be filtered by state. #5758
* [BUGFIX] API: lifecycle endpoints return 403 when not enabled. #6057
* [BUGFIX] Build: Fix Solaris build. #6149
* [BUGFIX] Promtool: Remove false duplicate rule warnings when checking rule files with alerts. #6270
* [BUGFIX] Remote write: restore use of deduplicating logger in remote write. #6113
* [BUGFIX] Remote write: do not reshard when unable to send samples. #6111
* [BUGFIX] Service discovery: errors are no longer logged on context cancellation. #6116, #6133
* [BUGFIX] UI: handle null response from API properly. #6071

## 2.13.1 / 2019-10-16

* [BUGFIX] Fix panic in ARM builds of Prometheus. #6110
* [BUGFIX] promql: fix potential panic in the query logger. #6094
* [BUGFIX] Multiple errors of http: superfluous response.WriteHeader call in the logs. #6145

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

## 2.11.2 / 2019-08-14

* [BUGFIX/SECURITY] Fix a Stored DOM XSS vulnerability with query history. #5888

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
* [ENHANCEMENT] The graph UI autocomplete now includes your previous queries.
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

<https://prometheus.io/blog/2017/11/08/announcing-prometheus-2-0/>
<https://prometheus.io/docs/prometheus/2.0/migration/>

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

* [BUGFIX] Fix disappearing Alertmanager targets in Alertmanager discovery.
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
  assume it runs out of memory and subsequently throttles ingestion.
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

<https://prometheus.io/docs/querying/operators/#vector-matching>

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
  (<https://golang.org/ref/spec#String_literals>).

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
* [ENHANCEMENT] Help link points to <https://prometheus.github.io> now.
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

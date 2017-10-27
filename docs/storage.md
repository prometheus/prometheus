---
title: Storage
sort_rank: 5
---

# Storage

Prometheus has a sophisticated local storage subsystem. For indexes,
it uses [LevelDB](https://github.com/google/leveldb). For the bulk
sample data, it has its own custom storage layer, which organizes
sample data in chunks of constant size (1024 bytes payload). These
chunks are then stored on disk in one file per time series.

This sections deals with the various configuration settings and issues you
might run into. To dive deeper into the topic, check out the following talks:

* [The Prometheus Time Series Database](https://www.youtube.com/watch?v=HbnGSNEjhUc).
* [Configuring Prometheus for High Performance](https://www.youtube.com/watch?v=hPC60ldCGm8).

## Memory usage

Prometheus keeps all the currently used chunks in memory. In addition, it keeps
as many most recently used chunks in memory as possible. You have to tell
Prometheus how much memory it may use for this caching. The flag
`storage.local.target-heap-size` allows you to set the heap size (in bytes)
Prometheus aims not to exceed. Note that the amount of physical memory the
Prometheus server will use is the result of complex interactions of the Go
runtime and the operating system and very hard to predict precisely. As a rule
of thumb, you should have at least 50% headroom in physical memory over the
configured heap size. (Or, in other words, set `storage.local.target-heap-size`
to a value of two thirds of the physical memory limit Prometheus should not
exceed.)

The default value of `storage.local.target-heap-size` is 2GiB and thus tailored
to 3GiB of physical memory usage. If you have less physical memory available,
you have to lower the flag value. If you have more memory available, you should
raise the value accordingly. Otherwise, Prometheus will not make use of the
memory and thus will perform much worse than it could.

Because Prometheus uses most of its heap for long-lived allocations of memory
chunks, the
[garbage collection target percentage](https://golang.org/pkg/runtime/debug/#SetGCPercent)
is set to 40 by default. You can still override this setting via the `GOGC`
environment variable as usual. If you need to conserve CPU capacity and can
accept running with fewer memory chunks, try higher values.

For high-performance set-ups, you might need to adjust more flags. Please read
through the sections below for details.

NOTE: Prior to v1.6, there was no flag `storage.local.target-heap-size`.
Instead, the number of chunks kept in memory had to be configured using the
flags `storage.local.memory-chunks` and `storage.local.max-chunks-to-persist`.
These flags still exist for compatibility reasons. However,
`storage.local.max-chunks-to-persist` has no effect anymore, and if
`storage.local.memory-chunks` is set to a non-zero value _x_, it is used to
override the value for `storage.local.target-heap-size` to 3072*_x_.

## Disk usage

Prometheus stores its on-disk time series data under the directory specified by
the flag `storage.local.path`. The default path is `./data` (relative to the
working directory), which is good to try something out quickly but most likely
not what you want for actual operations. The flag `storage.local.retention`
allows you to configure the retention time for samples. Adjust it to your needs
and your available disk space.

## Chunk encoding

Prometheus currently offers three different types of chunk encodings. The chunk
encoding for newly created chunks is determined by the
`-storage.local.chunk-encoding-version` flag. The valid values are 0, 1,
or 2.

Type 0 is the simple delta encoding implemented for Prometheus's first chunked
storage layer. Type 1 is the current default encoding, a double-delta encoding
with much better compression behavior than type 0. Both encodings feature a
fixed byte width per sample over the whole chunk, which allows fast random
access. While type 0 is the fastest encoding, the difference in encoding cost
compared to encoding 1 is tiny. Due to the better compression behavior of type
1, there is really no reason to select type 0 except compatibility with very
old Prometheus versions.

Type 2 is a variable bit-width encoding, i.e. each sample in the chunk can use
a different number of bits. Timestamps are double-delta encoded, too, but with
a slightly different algorithm. A number of different encoding schemes are
available for sample values. The choice is made per chunk based on the nature
of the sample values (constant, integer, regularly increasing, random…). Major
parts of the type 2 encoding are inspired by a paper published by Facebook
engineers:
[_Gorilla: A Fast, Scalable, In-Memory Time Series Database_](http://www.vldb.org/pvldb/vol8/p1816-teller.pdf).

With type 2, access within a chunk has to happen sequentially, and the encoding
and decoding cost is a bit higher. Overall, type 2 will cause more CPU usage
and increased query latency compared to type 1 but offers a much improved
compression ratio. The exact numbers depend heavily on the data set and the
kind of queries. Below are results from a typical production server with a
fairly expensive set of recording rules.

Chunk type | bytes per sample | cores | rule evaluation duration
:------:|:-----:|:----:|:----:
1 | 3.3 | 1.6 | 2.9s
2 | 1.3 | 2.4 | 4.9s

You can change the chunk encoding each time you start the server, so
experimenting with your own use case is encouraged. Take into account, however,
that only newly created chunks will use the newly selected chunk encoding, so
it will take a while until you see the effects.

For more details about the trade-off between the chunk encodings, see
[this blog post](https://prometheus.io/blog/2016/05/08/when-to-use-varbit-chunks/).

## Settings for high numbers of time series

Prometheus can handle millions of time series. However, with the above
mentioned default setting for `storage.local.target-heap-size`, you will be
limited to about 200,000 time series simultaneously present in memory. For more
series, you need more memory, and you need to configure Prometheus to make use
of it as described above.

Each of the aforementioned chunks contains samples of a single time series. A
time series is thus represented as a series of chunks, which ultimately end up
in a time series file (one file per time series) on disk.

A series that has recently received new samples will have an open incomplete
_head chunk_. Once that chunk is completely filled, or the series hasn't
received samples in a while, the head chunk is closed and becomes a chunk
waiting to be appended to its corresponding series file, i.e. it is _waiting
for persistence_. After the chunk has been persisted to disk, it becomes
_evictable_, provided it is not currently used by a query. Prometheus will
evict evictable chunks from memory to satisfy the configured target heap
size. A series with an open head chunk is called an _active series_. This is
different from a _memory series_, which also includes series without an open
head chunk but still other chunks in memory (whether waiting for persistence,
used in a query, or evictable). A series without any chunks in memory may be
_archived_, upon which it ceases to have any mandatory memory footprint.

The amount of chunks Prometheus can keep in memory depends on the flag value
for `storage.local.target-heap-size` and on the amount of memory used by
everything else. If there are not enough chunks evictable to satisfy the target
heap size, Prometheus will throttle ingestion of more samples (by skipping
scrapes and rule evaluations) until the heap has shrunk enough. _Throttled
ingestion is really bad for various reasons. You really do not want to be in
that situation._

Open head chunks, chunks still waiting for persistence, and chunks being used
in a query are not evictable. Thus, the reasons for the inability to evict
enough chunks include the following:

1. Queries that use too many chunks.
2. Chunks are piling up waiting for persistence because the storage layer
   cannot keep up writing chunks.
3. There are too many active time series, which results in too many open head
   chunks.

Currently, Prometheus has no defence against case (1). Abusive queries will
essentially OOM the server.

To defend against case (2), there is a concept of persistence urgency explained
in the next section.

Case (3) depends on the targets you monitor. To mitigate an unplanned explosion
of the number of series, you can limit the number of samples per individual
scrape (see `sample_limit` in the [scrape config](configuration/configuration.md#scrape_config)).
If the number of active time series exceeds the number of memory chunks the
Prometheus server can afford, the server will quickly throttle ingestion as
described above. The only way out of this is to give Prometheus more RAM or
reduce the number of time series to ingest.

In fact, you want many more memory chunks than you have series in
memory. Prometheus tries to batch up disk writes as much as possible as it
helps for both HDD (write as much as possible after each seek) and SSD (tiny
writes create write amplification, which limits the effective throughput and
burns much more quickly through the lifetime of the device). The more
Prometheus can batch up writes, the more efficient is the process of persisting
chunks to disk. which helps case (2).

In conclusion, to keep the Prometheus server healthy, make sure it has plenty
of headroom of memory chunks available for the number of memory series. A
factor of three is a good starting point. Refer to the
[section about helpful metrics](#helpful-metrics) to find out what to look
for. A very broad rule of thumb for an upper limit of memory series is the
total available physical memory divided by 10,000, e.g. About 6M memory series
on a 64GiB server.

If you combine a high number of time series with very fast and/or large
scrapes, the number of pre-allocated mutexes for series locking might not be
sufficient. If you see scrape hiccups while Prometheus is writing a checkpoint
or processing expensive queries, try increasing the value of the
`storage.local.num-fingerprint-mutexes` flag. Sometimes tens of thousands or
even more are required.

PromQL queries that involve a high number of time series will make heavy use of
the LevelDB-backed indexes. If you need to run queries of that kind, tweaking
the index cache sizes might be required. The following flags are relevant:

* `-storage.local.index-cache-size.label-name-to-label-values`: For regular
  expression matching.
* `-storage.local.index-cache-size.label-pair-to-fingerprints`: Increase the
  size if a large number of time series share the same label pair or name.
* `-storage.local.index-cache-size.fingerprint-to-metric` and
  `-storage.local.index-cache-size.fingerprint-to-timerange`: Increase the size
  if you have a large number of archived time series, i.e. series that have not
  received samples in a while but are still not old enough to be purged
  completely.

You have to experiment with the flag values to find out what helps. If a query
touches 100,000+ time series, hundreds of MiB might be reasonable. If you have
plenty of memory available, using more of it for LevelDB cannot harm. More
memory for LevelDB will effectively reduce the number of memory chunks
Prometheus can afford.

## Persistence urgency and “rushed mode”

Naively, Prometheus would all the time try to persist completed chunk to disk
as soon as possible. Such a strategy would lead to many tiny write operations,
using up most of the I/O bandwidth and keeping the server quite busy. Spinning
disks will appear to be very slow because of the many slow seeks required, and
SSDs will suffer from write amplification. Prometheus tries instead to batch up
write operations as much as possible, which works better if it is allowed to
use more memory.

Prometheus will also sync series files after each write (with
`storage.local.series-sync-strategy=adaptive`, which is the default) and use
the disk bandwidth for more frequent checkpoints (based on the count of “dirty
series”, see [below](#crash-recovery)), both attempting to minimize data loss
in case of a crash.

But what to do if the number of chunks waiting for persistence grows too much?
Prometheus calculates a score for urgency to persist chunks. The score is
between 0 and 1, where 1 corresponds to the highest urgency. Depending on the
score, Prometheus will write to disk more frequently. Should the score ever
pass the threshold of 0.8, Prometheus enters “rushed mode” (which you can see
in the logs). In rushed mode, the following strategies are applied to speed up
persisting chunks:

* Series files are not synced after write operations anymore (making better use
  of the OS's page cache at the price of an increased risk of losing data in
  case of a server crash – this behavior can be overridden with the flag
  `storage.local.series-sync-strategy`).
* Checkpoints are only created as often as configured via the
  `storage.local.checkpoint-interval` flag (freeing more disk bandwidth for
  persisting chunks at the price of more data loss in case of a crash and an
  increased time to run the subsequent crash recovery).
* Write operations to persist chunks are not throttled anymore and performed as
  fast as possible.

Prometheus leaves rushed mode once the score has dropped below 0.7.

Throttling of ingestion happens if the urgency score reaches 1. Thus, the
rushed mode is not _per se_ something to be avoided. It is, on the contrary, a
measure the Prometheus server takes to avoid the really bad situation of
throttled ingestion. Occasionally entering rushed mode is OK, if it helps and
ultimately leads to leaving rushed mode again. _If rushed mode is entered but
the urgency score still goes up, the server has a real problem._

## Settings for very long retention time

If you have set a very long retention time via the `storage.local.retention`
flag (more than a month), you might want to increase the flag value
`storage.local.series-file-shrink-ratio`.

Whenever Prometheus needs to cut off some chunks from the beginning of a series
file, it will simply rewrite the whole file. (Some file systems support “head
truncation”, which Prometheus currently does not use for several reasons.) To
not rewrite a very large series file to get rid of very few chunks, the rewrite
only happens if at least 10% of the chunks in the series file are removed. This
value can be changed via the mentioned `storage.local.series-file-shrink-ratio`
flag. If you have a lot of disk space but want to minimize rewrites (at the
cost of wasted disk space), increase the flag value to higher values, e.g. 0.3
for 30% of required chunk removal.

## Crash recovery

Prometheus saves chunks to disk as soon as possible after they are
complete. Incomplete chunks are saved to disk during regular
checkpoints. You can configure the checkpoint interval with the flag
`storage.local.checkpoint-interval`. Prometheus creates checkpoints
more frequently than that if too many time series are in a “dirty”
state, i.e. their current incomplete head chunk is not the one that is
contained in the most recent checkpoint. This limit is configurable
via the `storage.local.checkpoint-dirty-series-limit` flag.

More active time series to cycle through lead in general to more chunks waiting
for persistence, which in turns leads to larger checkpoints and ultimately more
time needed for checkpointing. There is a clear trade-off between limiting the
loss of data in case of a crash and the ability to scale to high number of
active time series. To not spend the majority of the disk throughput for
checkpointing, you have to increase the checkpoint interval. Prometheus itself
limits the time spent in checkpointing to 50% by waiting after each
checkpoint's completion for at least as long as the previous checkpoint took.

Nevertheless, should your server crash, you might still lose data, and
your storage might be left in an inconsistent state. Therefore,
Prometheus performs a crash recovery after an unclean shutdown,
similar to an `fsck` run for a file system. Details about the crash
recovery are logged, so you can use it for forensics if required. Data
that cannot be recovered is moved to a directory called `orphaned`
(located under `storage.local.path`). Remember to delete that data if
you do not need it anymore.

The crash recovery usually takes less than a minute. Should it take much
longer, consult the log to find out what is going on. With increasing number of
time series in the storage (archived or not), the re-indexing tends to dominate
the recovery time and can take tens of minutes in extreme cases.

## Data corruption

If you suspect problems caused by corruption in the database, you can
enforce a crash recovery by starting the server with the flag
`storage.local.dirty`.

If that does not help, or if you simply want to erase the existing
database, you can easily start fresh by deleting the contents of the
storage directory:

   1. Stop Prometheus.
   1. `rm -r <storage path>/*`
   1. Start Prometheus.

## Helpful metrics

Out of the metrics that Prometheus exposes about itself, the following are
particularly useful to tweak flags and find out about the required
resources. They also help to create alerts to find out in time if a Prometheus
server has problems or is out of capacity.

* `prometheus_local_storage_memory_series`: The current number of series held
  in memory.
* `prometheus_local_storage_open_head_chunks`: The number of open head chunks.
* `prometheus_local_storage_chunks_to_persist`: The number of memory chunks
  that still need to be persisted to disk.
* `prometheus_local_storage_memory_chunks`: The current number of chunks held
  in memory. If you substract the previous two, you get the number of persisted
  chunks (which are evictable if not currently in use by a query).
* `prometheus_local_storage_series_chunks_persisted`: A histogram of the number
  of chunks persisted per batch.
* `prometheus_local_storage_persistence_urgency_score`: The urgency score as
  discussed [above](#persistence-urgency-and-rushed-mode).
* `prometheus_local_storage_rushed_mode` is 1 if Prometheus is in “rushed
  mode”, 0 otherwise. Can be used to calculate the percentage of time
  Prometheus is in rushed mode.
* `prometheus_local_storage_checkpoint_last_duration_seconds`: How long the
  last checkpoint took.
* `prometheus_local_storage_checkpoint_last_size_bytes`: Size of the last
  checkpoint in bytes.
* `prometheus_local_storage_checkpointing` is 1 while Prometheus is
  checkpointing, 0 otherwise. Can be used to calculate the percentage of time
  Prometheus is checkpointing.
* `prometheus_local_storage_inconsistencies_total`: Counter for storage
  inconsistencies found. If this is greater than 0, restart the server for
  recovery.
* `prometheus_local_storage_persist_errors_total`: Counter for persist errors.
* `prometheus_local_storage_memory_dirty_series`: Current number of dirty series.
* `process_resident_memory_bytes`: Broadly speaking the physical memory
  occupied by the Prometheus process.
* `go_memstats_alloc_bytes`: Go heap size (allocated objects in use plus allocated
  objects not in use anymore but not yet garbage-collected).

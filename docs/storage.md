---
title: Storage
sort_rank: 5
---

# Storage

Prometheus includes a local on-disk time series database, but also optionally integrates with remote storage systems.

## Local storage

Prometheus's local time series database stores time series data in a custom format on disk.

### On-disk layout

Ingested samples are grouped into blocks of two hours. Each two-hour block consists of a directory containing one or more chunk files that contain all time series samples for that window of time, as well as a metadata file and index file (which indexes metric names and labels to time series in the chunk files). When series are deleted via the API, deletion records are stored in separate tombstone files (instead of deleting the data immediately from the chunk files).

The block for currently incoming samples is kept in memory and not fully persisted yet. It is secured against crashes by a write-ahead-log (WAL) that can be replayed when the Prometheus server restarts after a crash. Write-ahead log files are stored in the `wal` directory in 128MB segments. These files contain raw data that has not been compacted yet, so they are significantly larger than regular block files. Prometheus will keep a minimum of 3 write-ahead log files, however high-traffic servers may see more than three WAL files since it needs to keep at least two hours worth of raw data.

The directory structure of a Prometheus server's data directory will look something like this:

```
./data
├── 01BKGV7JBM69T2G1BGBGM6KB12
│   └── meta.json
├── 01BKGTZQ1SYQJTR4PB43C8PD98
│   ├── chunks
│   │   └── 000001
│   ├── tombstones
│   ├── index
│   └── meta.json
├── 01BKGTZQ1HHWHV8FBJXW1Y3W0K
│   └── meta.json
├── 01BKGV7JC0RY8A6MACW02A2PJD
│   ├── chunks
│   │   └── 000001
│   ├── tombstones
│   ├── index
│   └── meta.json
├── chunks_head
│   └── 000001
└── wal
    ├── 000000002
    └── checkpoint.00000001
        └── 00000000
```


Note that a limitation of the local storage is that it is not clustered or replicated. Thus, it is not arbitrarily scalable or durable in the face of disk or node outages and should be treated as you would any other kind of single node database. Using RAID for disk availability, [snapshots](querying/api.md#snapshot) for backups, capacity planning, etc, is recommended for improved durability. With proper storage durability and planning storing years of data in the local storage is possible.

Alternatively, external storage may be used via the [remote read/write APIs](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage). Careful evaluation is required for these systems as they vary greatly in durability, performance, and efficiency.

For further details on file format, see [TSDB format](/tsdb/docs/format/README.md).

## Compaction

The initial two-hour blocks are eventually compacted into longer blocks in the background.

Compaction will create larger blocks up to 10% of the retention time, or 31 days, whichever is smaller.

## Operational aspects

Prometheus has several flags that allow configuring the local storage. The most important ones are:

* `--storage.tsdb.path`: This determines where Prometheus writes its database. Defaults to `data/`.
* `--storage.tsdb.retention.time`: This determines when to remove old data. Defaults to `15d`. Overrides `storage.tsdb.retention` if this flag is set to anything other than default.
* `--storage.tsdb.retention.size`: [EXPERIMENTAL] This determines the maximum number of bytes that storage blocks can use (note that this does not include the WAL size, which can be substantial). The oldest data will be removed first. Defaults to `0` or disabled. This flag is experimental and can be changed in future releases. Units supported: B, KB, MB, GB, TB, PB, EB. Ex: "512MB"
* `--storage.tsdb.retention`: This flag has been deprecated in favour of `storage.tsdb.retention.time`.
* `--storage.tsdb.wal-compression`: This flag enables compression of the write-ahead log (WAL). Depending on your data, you can expect the WAL size to be halved with little extra cpu load. This flag was introduced in 2.11.0 and enabled by default in 2.20.0. Note that once enabled, downgrading Prometheus to a version below 2.11.0 will require deleting the WAL.

On average, Prometheus uses only around 1-2 bytes per sample. Thus, to plan the capacity of a Prometheus server, you can use the rough formula:

```
needed_disk_space = retention_time_seconds * ingested_samples_per_second * bytes_per_sample
```

To tune the rate of ingested samples per second, you can either reduce the number of time series you scrape (fewer targets or fewer series per target), or you can increase the scrape interval. However, reducing the number of series is likely more effective, due to compression of samples within a series.

If your local storage becomes corrupted for whatever reason, your best bet is to shut down Prometheus and remove the entire storage directory. You can try removing individual block directories, or WAL directory, to resolve the problem, this means losing a time window of around two hours worth of data per block directory. Again, Prometheus's local storage is not meant as durable long-term storage.

CAUTION: Non-POSIX compliant filesystems are not supported by Prometheus' local storage as unrecoverable corruptions may happen. NFS filesystems (including AWS's EFS) are not supported. NFS could be POSIX-compliant, but most implementations are not. It is strongly recommended to use a local filesystem for reliability.

If both time and size retention policies are specified, whichever policy triggers first will be used at that instant.

Expired block cleanup happens on a background schedule. It may take up to two hours to remove expired blocks. Expired blocks must be fully expired before they are cleaned up.

## Remote storage integrations

Prometheus's local storage is limited by single nodes in its scalability and durability. Instead of trying to solve clustered storage in Prometheus itself, Prometheus has a set of interfaces that allow integrating with remote storage systems.

### Overview

Prometheus integrates with remote storage systems in two ways:

* Prometheus can write samples that it ingests to a remote URL in a standardized format.
* Prometheus can read (back) sample data from a remote URL in a standardized format.

![Remote read and write architecture](images/remote_integrations.png)

The read and write protocols both use a snappy-compressed protocol buffer encoding over HTTP. The protocols are not considered as stable APIs yet and may change to use gRPC over HTTP/2 in the future, when all hops between Prometheus and the remote storage can safely be assumed to support HTTP/2.

For details on configuring remote storage integrations in Prometheus, see the [remote write](configuration/configuration.md#remote_write) and [remote read](configuration/configuration.md#remote_read) sections of the Prometheus configuration documentation.

For details on the request and response messages, see the [remote storage protocol buffer definitions](https://github.com/prometheus/prometheus/blob/master/prompb/remote.proto).

Note that on the read path, Prometheus only fetches raw series data for a set of label selectors and time ranges from the remote end. All PromQL evaluation on the raw data still happens in Prometheus itself. This means that remote read queries have some scalability limit, since all necessary data needs to be loaded into the querying Prometheus server first and then processed there. However, supporting fully distributed evaluation of PromQL was deemed infeasible for the time being.

### Existing integrations

To learn more about existing integrations with remote storage systems, see the [Integrations documentation](https://prometheus.io/docs/operating/integrations/#remote-endpoints-and-storage).

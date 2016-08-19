# Project Frankenstein

Design Doc: https://docs.google.com/document/d/1C7yhMnb1x2sfeoe45f4mnnKConvroWhJ8KQZwIHJOuw/edit#heading=h.f7lkb8wswewc

    GOOS=linux DOCKER_IMAGE_TAG=latest make build docker docker-frank

TODO list
- chunk id generation
- parallelise chunk puts
- commit log in the ingester
- separate query service?

## Retrieval

Use existing prometheus binary; add a --retrieval-only flag to existing prometheus?  and use one of the remote storage protocols or add a new one.

- Brian's generic write PR https://github.com/prometheus/prometheus/pull/1487

## Distribution

Use a consistent hasing library to distribute timeseries to collectors

## Ingester

Use existing prometheus binary with addind push interface. Adapt memorySeriesStorage with support for flushing chunks to something else.

Ingestion Chunk Builder

On disk:
    Index for fingerprint -> metric, with in-memory cache (use existing leveldb / FingerprintMetricIndex)
    Log of (fingerprint, datapoint) (skip for now, add later)

In memory:
    Map of fingerprint -> timeseries (use existing seriesMap)

    Timeseries:
         - has 1 active/open chunk
         - and a queue of chunks being flushed

    A goroutine which:
        - move active chunks to the flushing queue
        - flushes chunks in batches
        - deletes empty timeseries from the map & index

    On append:
        - work out fingerprint (consult mapper)
        - write (fingerprint, datapoint) entry to log
        - find / create timeseries
        - add entry to "open" chunk

    On query:
        - For now, iterate through all metrics & return matching fingerprints
        - Later, consider in-memory inverted index too, although only needs
          to be built from existing DS, doesn't need to be persisted.

    On startup:
        - load index
        - replay log

Pros:
    - simplicity: only concerned with maintaining and building open chunks
    - no caching to worry about etc

Cons:
    - have to rewrite lots
    - user could start virtually infinite number of timeseries and cause us to OOM

## Query

Use existing prometheus binary with flags to point it at distribution?

## Storage

Hash key = [userid, hour bucket, metric name]
Range key = [label, value, chunk id]

What queries don't use the metric name?
- number of metrics
- timeseries per job

Would adding a layer of indirection for metric name hurt?

We need to know what labels an existing chunk has, to return to the user.
- each chunk could contain these value
- or the chunk id could be the finger print, and have a separate lookup table

How to assign ids?
- hashes have collisions (fnv1a)
- unique prefixes and locally

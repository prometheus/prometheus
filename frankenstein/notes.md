# Project Frankenstein

Design Doc: https://docs.google.com/document/d/1C7yhMnb1x2sfeoe45f4mnnKConvroWhJ8KQZwIHJOuw/edit#heading=h.f7lkb8wswewc

## Retrieval

Use existing prometheus binary; add a --retrieval-only flag to existing prometheus?  and use one of the remote storage protocols or add a new one.

- Brian's generic write PR https://github.com/prometheus/prometheus/pull/1487

## Distribution

Use a consistent hasing library to distribute timeseries to collectors

## Collection

Use existing prometheus binary with addind push interface. Adapt memorySeriesStorage with support for flushing chunks to something else.

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

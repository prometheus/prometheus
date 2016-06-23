# Project Frankenstein

Design Doc: https://docs.google.com/document/d/1C7yhMnb1x2sfeoe45f4mnnKConvroWhJ8KQZwIHJOuw/edit#heading=h.f7lkb8wswewc

    # Build frankenstein
    make frank

    # Start Consul:
    consul agent -ui -data-dir=consul/ -server -advertise=127.0.0.1 -bootstrap

    # Start frank distributor:
    ./frank -web.listen-address=:9094

    # Start a ingestor
    ./prometheus -config.file=empty.yml

    # Add a token into consul for it:
    curl  -X PUT -d '{"hostname": "http://localhost:9090/push", "tokens": [0]}' http://localhost:8500/v1/kv/collectors/localhost

    # Start retrieval scraping the ingestor, push to distributor
    ./prometheus -config.file=frankenstein/retrieval.yml -web.listen-address=:9091 -retrieval-only -storage.remote.generic-url=http://localhost:9094/push

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

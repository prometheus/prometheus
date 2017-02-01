# Remote storage bridge

This is a bridge that receives samples in Prometheus's remote storage
format and forwards them to Graphite, InfluxDB, or OpenTSDB. It is meant
as a replacement for the built-in specific remote storage implementations
that have been removed from Prometheus.

## Building

```
go build
```

## Running

Example:

```
./remote_storage_bridge -graphite-address=localhost:8080 -opentsdb-url=http://localhost:8081/
```

To show all flags:

```
./remote_storage_bridge -h
```

## Configuring Prometheus

To configure Prometheus to send samples to this bridge, add the following to your `prometheus.yml`:

```yaml
remote_write:
  url: "http://localhost:9201/receive"
```
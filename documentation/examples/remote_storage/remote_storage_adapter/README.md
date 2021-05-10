# Remote storage adapter

This is a write adapter that receives samples via Prometheus's remote write
protocol and stores them in Graphite, InfluxDB, or OpenTSDB. It is meant as a
replacement for the built-in specific remote storage implementations that have
been removed from Prometheus.

For InfluxDB, this binary is also a read adapter that supports reading back
data through Prometheus via Prometheus's remote read protocol.

## Building

```
go build
```

## Running

Graphite example:

```
./remote_storage_adapter --graphite-address=localhost:8080
```

OpenTSDB example:

```
./remote_storage_adapter --opentsdb-url=http://localhost:8081/
```

InfluxDB example:

```
./remote_storage_adapter --influxdb-url=http://localhost:8086/ --influxdb.database=prometheus --influxdb.retention-policy=autogen
```

Clickhouse example:
```
./remote_storage_adapter --clickhouse.url=localhost:9000
```

sql for clickhouse
``` sql
# note: replace {shard} and {replica} and run on each server

CREATE DATABASE prometheus ON CLUSTER you_cluster

DROP TABLE IF EXISTS prometheus.metrics;
CREATE TABLE IF NOT EXISTS prometheus.metrics ON CLUSTER you_cluster
(
     date Date DEFAULT toDate(0),
     name String,
     tags Array(String),
     val Float64,
     ts DateTime,
     updated DateTime DEFAULT now()
)
ENGINE = GraphiteMergeTree(
     '/clickhouse/tables/{shard}/prometheus.metrics',
     '{replica}', date, (name, tags, ts), 8192, 'graphite_rollup'
)

```

xml example  for clickhouse server config
``` xml
 <graphite_rollup>
        <path_column_name>tags</path_column_name>
        <time_column_name>ts</time_column_name>
        <value_column_name>val</value_column_name>
        <version_column_name>updated</version_column_name>
        <default>
                <function>avg</function>
                <retention>
                        <age>0</age>
                        <precision>10</precision>
                </retention>
                <retention>
                        <age>86400</age>
                        <precision>30</precision>
                </retention>
                <retention>
                        <age>172800</age>
                        <precision>300</precision>
                </retention>
        </default>
  </graphite_rollup>
```


To show all flags:

```
./remote_storage_adapter -h
```

## Configuring Prometheus

To configure Prometheus to send samples to this binary, add the following to your `prometheus.yml`:

```yaml
# Remote write configuration (for Graphite, OpenTSDB, or InfluxDB).
remote_write:
  - url: "http://localhost:9201/write"

# Remote read configuration (for InfluxDB only at the moment).
remote_read:
  - url: "http://localhost:9201/read"
```

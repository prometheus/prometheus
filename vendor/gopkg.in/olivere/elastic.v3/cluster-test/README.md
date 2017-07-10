# Cluster Test

This directory contains a program you can use to test a cluster.

Here's how:

First, install a cluster of Elasticsearch nodes. You can install them on
different computers, or start several nodes on a single machine.

Build cluster-test by `go build cluster-test.go` (or build with `make`).

Run `./cluster-test -h` to get a list of flags:

```sh
$ ./cluster-test -h
Usage of ./cluster-test:
  -errorlog="": error log file
  -healthcheck=true: enable or disable healthchecks
  -healthchecker=1m0s: healthcheck interval
  -index="twitter": name of ES index to use
  -infolog="": info log file
  -n=5: number of goroutines that run searches
  -nodes="": comma-separated list of ES URLs (e.g. 'http://192.168.2.10:9200,http://192.168.2.11:9200')
  -retries=0: number of retries
  -sniff=true: enable or disable sniffer
  -sniffer=15m0s: sniffer interval
  -tracelog="": trace log file
```

Example:

```sh
$ ./cluster-test -nodes=http://127.0.0.1:9200,http://127.0.0.1:9201,http://127.0.0.1:9202 -n=5 -index=twitter -retries=5 -sniff=true -sniffer=10s -healthcheck=true -healthchecker=5s -errorlog=error.log
```

The above example will create an index and start some search jobs on the
cluster defined by http://127.0.0.1:9200, http://127.0.0.1:9201,
and http://127.0.0.1:9202.

* It will create an index called `twitter` on the cluster (`-index=twitter`)
* It will run 5 search jobs in parallel (`-n=5`).
* It will retry failed requests 5 times (`-retries=5`).
* It will sniff the cluster periodically (`-sniff=true`).
* It will sniff the cluster every 10 seconds (`-sniffer=10s`).
* It will perform health checks periodically (`-healthcheck=true`).
* It will perform health checks on the nodes every 5 seconds (`-healthchecker=5s`).
* It will write an error log file (`-errorlog=error.log`).

If you want to test Elastic with nodes going up and down, you can use a
chaos monkey script like this and run it on the nodes of your cluster:

```sh
#!/bin/bash
while true
do
	echo "Starting ES node"
	elasticsearch -d -Xmx4g -Xms1g -Des.config=elasticsearch.yml -p es.pid
	sleep `jot -r 1 10 300` # wait for 10-300s
	echo "Stopping ES node"
	kill -TERM `cat es.pid`
	sleep `jot -r 1 10 60`  # wait for 10-60s
done
```

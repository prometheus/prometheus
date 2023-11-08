# e2e Remote Write benchmark

This benchmark's purpose is to compare different versions of the remote write
protocol. It runs multiple pairs of sender=>receiver prometheus instances and
has the senders scrape an entire port-forwarded remote kubernetes namepsace.


## Run

1. Set envvars to port forward pods:

    ```
    export CONTEXT=my-k8-context
    export NAMESPACE=my-namespace

    ```
    If desired, tweak the INSTANCES variable in the `run.sh` script.

2. Run

```
./run.sh
```

## Profiles

```
go tool pprof -seconds=240 http://localhost:9095/debug/pprof/profile
go tool pprof -seconds=240 http://localhost:9094/debug/pprof/profile
```

## Stats

# Grafana instance with provisiones datasource and dashboard
```
docker run --network host -v ${PWD}/local_grafana/:/etc/grafana/provisioning  --env GF_AUTH_ANONYMOUS_ENABLED=true --env GF_AUTH_ANONYMOUS_ORG_ROLE=Admin  --env GF_AUTH_BASIC_ENABLED=false --env ORG_ID=123 grafana/grafana:latest
```


```
http://localhost:9095/graph?g0.expr=sum%20by%20(job)%20(process_cpu_seconds_total%7Bjob%3D~%22(sender%7Creceiver).%2B%22%2C%7D)&g0.tab=0&g0.stacked=0&g0.show_exemplars=0&g0.range_input=15m

http://localhost:9095/graph?g0.expr=sum%20by%20(job)%20(prometheus_remote_storage_bytes_total%7Bjob%3D~%22(sender%7Creceiver).%2B%22%2C%7D)&g0.tab=0&g0.stacked=0&g0.show_exemplars=0&g0.range_input=15m

http://localhost:9095/graph?g0.expr=sum%20by%20(job)%20(go_memstats_alloc_bytes_total%7Bjob%3D~%22(sender%7Creceiver).%2B%22%2C%7D)&g0.tab=0&g0.stacked=0&g0.show_exemplars=0&g0.range_input=15m
```
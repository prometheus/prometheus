---
title: Management API
sort_rank: 7
---

# Management API

Prometheus provides a set of management API to ease automation and integrations.


### Health check

```
GET /-/healthy
```

This endpoint always returns 200 and should be used to check Prometheus health.


### Readiness check

```
GET /-/ready
```

This endpoint returns 200 when Prometheus is ready to serve traffic (i.e. respond to queries).


### Reload

```
PUT  /-/reload
POST /-/reload
```

This endpoint triggers a reload of the Prometheus configuration and rule files. It's disabled by default and can be enabled via the `--web.enable-lifecycle` flag.

Alternatively, a configuration reload can be triggered by sending a `SIGHUP` to the Prometheus process.


### Quit

```
PUT  /-/quit
POST /-/quit
```

This endpoint triggers a graceful shutdown of Prometheus. It's disabled by default and can be enabled via the `--web.enable-lifecycle` flag.

Alternatively, a graceful shutdown can be triggered by sending a `SIGTERM` to the Prometheus process.

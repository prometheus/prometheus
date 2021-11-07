---
title: Installation
sort_rank: 2
---

# Installation

## Using pre-compiled binaries

We provide precompiled binaries for most official Prometheus components. Check
out the [download section](https://prometheus.io/download) for a list of all
available versions.

## From source

For building Prometheus components from source, see the `Makefile` targets in
the respective repository.

## Using Docker

All Prometheus services are available as Docker images on
[Quay.io](https://quay.io/repository/prometheus/prometheus) or
[Docker Hub](https://hub.docker.com/r/prom/prometheus/).

Running Prometheus on Docker is as simple as `docker run -p 9090:9090
prom/prometheus`. This starts Prometheus with a sample
configuration and exposes it on port 9090.

The Prometheus image uses a volume to store the actual metrics. For
production deployments it is highly recommended to use a
[named volume](https://docs.docker.com/storage/volumes/)
to ease managing the data on Prometheus upgrades.

To provide your own configuration, there are several options. Here are
two examples.

### Volumes & bind-mount

Bind-mount your `prometheus.yml` from the host by running:

```bash
docker run \
    -p 9090:9090 \
    -v /path/to/prometheus.yml:/etc/prometheus/prometheus.yml \
    prom/prometheus
```

Or bind-mount the directory containing `prometheus.yml` onto
`/etc/prometheus` by running:

```bash
docker run \
    -p 9090:9090 \
    -v /path/to/config:/etc/prometheus \
    prom/prometheus
```

### Custom image

To avoid managing a file on the host and bind-mount it, the
configuration can be baked into the image. This works well if the
configuration itself is rather static and the same across all
environments.

For this, create a new directory with a Prometheus configuration and a
`Dockerfile` like this:

```Dockerfile
FROM prom/prometheus
ADD prometheus.yml /etc/prometheus/
```

Now build and run it:

```bash
docker build -t my-prometheus .
docker run -p 9090:9090 my-prometheus
```

A more advanced option is to render the configuration dynamically on start
with some tooling or even have a daemon update it periodically.

## Quick-start using Docker and node-exporter

When running Prometheus via Docker, you often want to report system metrics using node_exporter, 
but are constrained by the node running inside a Docker container. Here is an example docker-compose configuration:

```yml
version: "3.7"

services:
  node_exporter:
    image: prom/node-exporter
    container_name: prometheus_node
    volumes:
      - /proc:/host/proc:ro
      - /sys:/host/sys:ro
      - /:/rootfs:ro
    command:
      - "--path.procfs=/host/proc"
      - "--path.rootfs=/rootfs"
      - "--path.sysfs=/host/sys"
      - "--collector.filesystem.ignored-mount-points=^/(sys|proc|dev|host|etc)($$|/)"
    restart: always

  prometheus:
    image: my-prometheus
    container_name: prometheus
    restart: always
    ports:
      - 9090:9090
```

You also need to ensure that the configuration in your `prometheus.yml` contains something like this:

```yml
scrape_configs:
  - job_name: node
    static_configs:
      - targets: ['node_exporter:9100']
```

Note that the address of the node_exporter is NOT `localhost` but `node_exporter` - which will be properly resolved by Docker to the correct container, 
and there is no need to expose any extra ports to the host.


## Using configuration management systems

If you prefer using configuration management systems you might be interested in
the following third-party contributions:

### Ansible

* [Cloud Alchemy/ansible-prometheus](https://github.com/cloudalchemy/ansible-prometheus)

### Chef

* [rayrod2030/chef-prometheus](https://github.com/rayrod2030/chef-prometheus)

### Puppet

* [puppet/prometheus](https://forge.puppet.com/puppet/prometheus)

### SaltStack

* [saltstack-formulas/prometheus-formula](https://github.com/saltstack-formulas/prometheus-formula)

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
[Docker Hub](https://hub.docker.com/u/prom/).

Running Prometheus on Docker is as simple as `docker run -p 9090:9090
prom/prometheus`. This starts Prometheus with a sample
configuration and exposes it on port 9090.

The Prometheus image uses a volume to store the actual metrics. For
production deployments it is highly recommended to use the
[Data Volume Container](https://docs.docker.com/engine/admin/volumes/volumes/)
pattern to ease managing the data on Prometheus upgrades.

To provide your own configuration, there are several options. Here are
two examples.

### Volumes & bind-mount

Bind-mount your `prometheus.yml` from the host by running:

```bash
docker run -p 9090:9090 -v /tmp/prometheus.yml:/etc/prometheus/prometheus.yml \
       prom/prometheus
```

Or use an additional volume for the config:

```bash
docker run -p 9090:9090 -v /prometheus-data \
       prom/prometheus --config.file=/prometheus-data/prometheus.yml
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

* [bechtoldt/saltstack-prometheus-formula](https://github.com/bechtoldt/saltstack-prometheus-formula)

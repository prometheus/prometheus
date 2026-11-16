---
title: Installation
sort_rank: 2
---

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

The default Docker image declares `/prometheus` as a volume. If you do not mount
storage there, Docker creates an anonymous volume. For
production deployments it is highly recommended to use a
[named volume](https://docs.docker.com/storage/volumes/)
to ease managing the data on Prometheus upgrades.

### Setting command line parameters

The Docker image is started with a number of default command line parameters, which
can be found in the [Dockerfile](https://github.com/prometheus/prometheus/blob/main/Dockerfile) (adjust the link to correspond with the version in use).

If you want to add extra command line parameters to the `docker run` command,
you will need to re-add these yourself as they will be overwritten.

### Volumes & bind-mount

To provide your own configuration, there are several options. Here are
two examples.

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

### Save your Prometheus data

Prometheus stores its data in `/prometheus` inside the container. Stopping or
restarting the same container preserves that data. Starting a replacement with
`docker run` does not automatically reuse the previous anonymous volume, which
may still exist after the old container is removed.

To keep data available across container replacements and image upgrades, mount
a named volume or bind mount at `/prometheus`, and reuse the same storage in
each replacement container.

Run Prometheus container with persistent storage:

```bash
# Create persistent volume for your data
docker volume create prometheus-data
# Start Prometheus container
docker run \
    -p 9090:9090 \
    -v /path/to/prometheus.yml:/etc/prometheus/prometheus.yml \
    -v prometheus-data:/prometheus \
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

## Using configuration management systems

If you prefer using configuration management systems you might be interested in
the following third-party contributions:

### Ansible

* [prometheus-community/ansible](https://github.com/prometheus-community/ansible)

### Chef

* [rayrod2030/chef-prometheus](https://github.com/rayrod2030/chef-prometheus)

### Puppet

* [puppet/prometheus](https://forge.puppet.com/puppet/prometheus)

### SaltStack

* [saltstack-formulas/prometheus-formula](https://github.com/saltstack-formulas/prometheus-formula)

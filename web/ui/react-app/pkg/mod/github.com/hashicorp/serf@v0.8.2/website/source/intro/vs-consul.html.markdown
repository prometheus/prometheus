---
layout: "intro"
page_title: "Serf vs. Consul"
sidebar_current: "vs-other-consul"
description: |-
  Consul is a tool for service discovery and configuration. It provides high level features such as service discovery, health checking and key/value storage. It makes use of a group of strongly consistent servers to manage the datacenter.
---

# Serf vs. Consul

[Consul](https://www.consul.io) is a tool for service discovery and configuration.
It provides high level features such as service discovery, health checking
and key/value storage. It makes use of a group of strongly consistent servers
to manage the datacenter.

Serf can also be used for service discovery and orchestration, but it is built on
an eventually consistent gossip model, with no centralized servers. It provides a
number of features, including group membership, failure detection, event broadcasts
and a query mechanism. However, Serf does not provide any of the high-level features
of Consul.

In fact, the internal gossip protocol used within Consul, is powered by the Serf library.
Consul leverages the membership and failure detection features, and builds upon them.

The health checking provided by Serf is very low level, and only indicates if the
agent is alive. Consul extends this to provide a rich health checking system,
that handles liveness, in addition to arbitrary host and service-level checks.
Health checks are integrated with a central catalog that operators can easily
query to gain insight into the cluster.

The membership provided by Serf is at a node level, while Consul focuses
on the service level abstraction, with a single node to multiple service model.
This can be simulated in Serf using tags, but it is much more limited, and does
not provide rich query interfaces. Consul also makes use of a strongly consistent
Catalog, while Serf is only eventually consistent.

In addition to the service level abstraction and improved health checking,
Consul provides a key/value store and support for multiple datacenters.
Serf can run across the WAN but with degraded performance. Consul makes use
of [multiple gossip pools](https://www.consul.io/docs/internals/architecture.html),
so that the performance of Serf over a LAN can be retained while still using it over
a WAN for linking together multiple datacenters.

Consul is opinionated in its usage, while Serf is a more flexible and
general purpose tool. Consul uses a CP architecture, favoring consistency over
availability. Serf is a AP system, and sacrifices consistency for availability.
This means Consul cannot operate if the central servers cannot form a quorum,
while Serf will continue to function under almost all circumstances.

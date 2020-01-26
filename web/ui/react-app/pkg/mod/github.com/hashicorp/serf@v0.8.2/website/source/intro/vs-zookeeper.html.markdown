---
layout: "intro"
page_title: "Serf vs. ZooKeeper, doozerd, etcd"
sidebar_current: "vs-other-zk"
description: |-
  ZooKeeper, doozerd and etcd are all similar in their client/server architecture. All three have server nodes that require a quorum of nodes to operate (usually a simple majority). They are strongly consistent, and expose various primitives that can be used through client libraries within applications to build complex distributed systems.
---

# Serf vs. ZooKeeper, doozerd, etcd

ZooKeeper, doozerd and etcd are all similar in their client/server
architecture. All three have server nodes that require a quorum of
nodes to operate (usually a simple majority). They are strongly consistent,
and expose various primitives that can be used through client libraries within
applications to build complex distributed systems.

Serf has a radically different architecture based on gossip and provides a
smaller feature set. Serf only provides membership, failure detection,
and user events. Serf is designed to operate under network partitions
and embraces eventual consistency. Designed as a tool, it is friendly
for both system administrators and application developers.

ZooKeeper et al. by contrast are much more complex, and cannot be used directly
as a tool. Application developers must use libraries to build the features
they need, although some libraries exist for common patterns. Most failure
detection schemes built on these systems also have intrinsic scalability issues.
Most naive failure detection schemes depend on heartbeating, which use
periodic updates and timeouts. These schemes require work linear to
the number of nodes and place the demand on a fixed number of servers.
Additionally, the failure detection window is at least as long as the timeout,
meaning that in many cases failures may not be detected for a long time.
Additionally, ZooKeeper ephemeral nodes require that many active connections
be maintained to a few nodes.

The strong consistency provided by these systems is essential for building leader
election and other types of coordination for distributed systems, but it limits
their ability to operate under network partitions. At a minimum, if a majority of
nodes are not available, writes are disallowed. Since a failure is indistinguishable
from a slow response, the performance of these systems may rapidly degrade
under certain network conditions. All of these issues can be highly
problematic when partition tolerance is needed, for example in a service
discovery layer.

Additionally, Serf is not mutually exclusive with any of these strongly
consistent systems. Instead, they can be used in combination to create systems
that are more scalable and fault tolerant, without sacrificing features.

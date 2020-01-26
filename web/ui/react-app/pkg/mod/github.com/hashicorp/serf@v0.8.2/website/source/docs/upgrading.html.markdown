---
layout: "docs"
page_title: "Upgrading Serf"
sidebar_current: "docs-upgrading-upgrading"
description: |-
  Serf is meant to be a long-running agent on any nodes participating in a Serf cluster. These nodes consistently communicate with each other. As such, protocol level compatibility and ease of upgrades is an important thing to keep in mind when using Serf.
---

# Upgrading Serf

Serf is meant to be a long-running agent on any nodes participating in a
Serf cluster. These nodes consistently communicate with each other. As such,
protocol level compatibility and ease of upgrades is an important thing to
keep in mind when using Serf.

This page documents how to upgrade Serf when a new version is released.

## Upgrading Serf

In short, upgrading Serf is a short series of easy steps. For the steps
below, assume you're running version A of Serf, and then version B comes out.

1. On each node, install version B of Serf.

2. Shut down version A, and start version B with the `-protocol=PREVIOUS`
   flag, where "PREVIOUS" is the protocol version of version A (which can
   be discovered by running `serf -v` or `serf members -detailed`).

3. Once all nodes are running version B, go through every node and restart
   the version B agent _without_ the `-protocol` flag.

4. Done! You're now running the latest Serf agent speaking the latest protocol.
   You can verify this is the case by running `serf members -detailed` to
   make sure all members are speaking the same, latest protocol version.

The key to making this work is the [protocol compatibility](/docs/compatibility.html)
of Serf. The protocol version system is discussed below.

## Protocol Versions

By default, Serf agents speak the latest protocol they can. However, each
new version of Serf is also able to speak the previous protocol, if there
were any protocol changes.

You can see what protocol versions your version of Serf understands by
running `serf -v`. You'll see output similar to that below:

```
$ serf -v
Serf v0.2.0
Agent Protocol: 1 (Understands back to: 0)
```

This says the version of Serf as well as the latest protocol version (1,
in this case). It also says the earliest protocol version that this Serf
agent can understand (0, in this case).

By specifying the `-protocol` flag on `serf agent`, you can tell the
Serf agent to speak any protocol version that it can understand. This
only specifies the protocol version to _speak_. Every Serf agent can
always understand the entire range of protocol versions it claims to
on `serf -v`.

~> **By running a previous protocol version**, some features
of Serf, especially newer features, may not be available. If this is the
case, Serf will typically warn you. In general, you should always upgrade
your cluster so that you can run the latest protocol version.

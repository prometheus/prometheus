---
layout: "intro"
page_title: "Introduction"
sidebar_current: "what"
description: |-
  Welcome to the intro guide to Serf! This guide will show you what Serf is, explain the problems Serf solves, compare Serf versus other similar software, and show how easy it is to actually use Serf.
---

# Introduction to Serf

Welcome to the intro guide to Serf! This guide will show you what Serf is,
explain the problems Serf solves, compare Serf versus other similar
software, and show how easy it is to actually use Serf. If you're already familiar
with the basics of Serf, the [documentation](/docs/index.html) provides more
of a reference for all available features.

## What is Serf?

Serf is a tool for cluster membership, failure detection,
and orchestration that is decentralized, fault-tolerant and
highly available. Serf runs on every major platform: Linux, Mac OS X, and Windows. It is
extremely lightweight: it uses 5 to 10 MB of resident memory and primarily
communicates using infrequent UDP messages.

Serf uses an efficient [gossip protocol](/docs/internals/gossip.html)
to solve three major problems:

* **Membership**: Serf maintains cluster membership lists and is able to
  execute custom handler scripts when that membership changes. For example,
  Serf can maintain the list of web servers for a load balancer and notify
  that load balancer whenever a node comes online or goes offline.

* **Failure detection and recovery**: Serf automatically detects failed nodes within
  seconds, notifies the rest of the cluster,
  and executes handler scripts allowing you to handle these events.
  Serf will attempt to recover failed nodes by reconnecting to them
  periodically.

* **Custom event propagation**: Serf can broadcast custom events and queries
  to the cluster. These can be used to trigger deploys, propagate configuration, etc.
  Events are simply fire-and-forget broadcast, and Serf makes a best effort to
  deliver messages in the face of offline nodes or network partitions. Queries
  provide a simple realtime request/response mechanism.

See the [use cases page](/intro/use-cases.html) for a list of concrete use
cases built on top of the features Serf provides. See the page on
[how Serf compares to other software](/intro/vs-other-sw.html) to see just
how it fits into your existing infrastructure. Or continue onwards with
the [getting started guide](/intro/getting-started/install.html) to get
Serf up and running and see how it works.

---
layout: "docs"
page_title: "Internals"
sidebar_current: "docs-internals"
description: |-
  This section goes over some of the internals of Serf, such as the gossip protocol, ordering of messages via Lamport clocks, etc. This section also contains a useful convergence simulator that can be used to see how fast a Serf cluster will converge under various conditions with specific configurations.
---

# Serf Internals

This section goes over some of the internals of Serf, such as the gossip
protocol, ordering of messages via Lamport clocks, etc. This section
also contains a useful [convergence simulator](/docs/internals/simulator.html)
that can be used to see how fast a Serf cluster will converge under
various conditions with specific configurations.

-> **Note:** Knowing about the internals of Serf is not necessary to
successfully use it, but we document it here to be completely transparent
about how the "magic" of Serf works.

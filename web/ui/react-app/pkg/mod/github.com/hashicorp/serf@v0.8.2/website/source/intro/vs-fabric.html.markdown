---
layout: "intro"
page_title: "Serf vs. Fabric"
sidebar_current: "vs-other-fabric"
description: |-
  Fabric is a widely used tool for system administration over SSH. Broadly, it is used to SSH into a group of nodes and execute commands. Both Fabric and Serf can be used for service management in different ways.
---

# Serf vs. Fabric

Fabric is a widely used tool for system administration over SSH. Broadly,
it is used to SSH into a group of nodes and execute commands. Both Fabric
and Serf can be used for service management in different ways.

While Fabric sends commands from a single box, Serf instead rapidly broadcasts a message
to the entire cluster in a distributed fashion. Fabric has a number of advantages
in that it can collect unlimited output of commands and stop execution if an
error is encountered. Serf queries can collect a limited amount of output, but lacks
the more complex control flow of Fabric. However, Fabric must be provided with a list
of nodes to contact, whereas membership is built directly into Serf. Additionally, Serf
is able to propagate a message within seconds to an entire cluster, allowing for much higher
parallelism and scalability.

Fabric is much more capable than Serf at system administration, but it is
limited by its execution speed and lack of node discovery. Combined together,
Fabric can query Serf for nodes and make use of message broadcasts where
appropriate, using direct SSH execution when and where output is needed.

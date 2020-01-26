---
layout: "docs"
page_title: "Gossip Protocol"
sidebar_current: "docs-internals-gossip"
description: |-
Serf uses a gossip protocol to broadcast messages to the cluster. This page documents the details of this internal protocol. The gossip protocol is based on SWIM: Scalable Weakly-consistent Infection-style Process Group Membership Protocol, with a few minor adaptations, mostly to increase propagation speed and convergence rate.
---

# Gossip Protocol

Serf uses a [gossip protocol](https://en.wikipedia.org/wiki/Gossip_protocol)
to broadcast messages to the cluster. This page documents the details of
this internal protocol. The gossip protocol is based on
["SWIM: Scalable Weakly-consistent Infection-style Process Group Membership Protocol"](http://www.cs.cornell.edu/info/projects/spinglass/public_pdfs/swim.pdf),
with a few minor adaptations, mostly to increase propagation speed
and convergence rate.

~> **Advanced Topic!** This page covers the technical details of
the internals of Serf. You don't need to know these details to effectively
operate and use Serf. These details are documented here for those who wish
to learn about them without having to go spelunking through the source code.

## SWIM Protocol Overview

Serf begins by joining an existing cluster or starting a new
cluster. If starting a new cluster, additional nodes are expected to join
it. New nodes in an existing cluster must be given the address of at
least one existing member in order to join the cluster. The new member
does a full state sync with the existing member over TCP and begins gossiping its
existence to the cluster.

Gossip is done over UDP with a configurable but fixed fanout and interval.
This ensures that network usage is constant with regards to number of nodes.
Complete state exchanges with a random node are done periodically over
TCP, but much less often than gossip messages. This increases the likelihood
that the membership list converges properly since the full state is exchanged
and merged. The interval between full state exchanges is configurable or can
be disabled entirely.

Failure detection is done by periodic random probing using a configurable interval.
If the node fails to ack within a reasonable time (typically some multiple
of RTT), then an indirect probe is attempted. An indirect probe asks a
configurable number of random nodes to probe the same node, in case there
are network issues causing our own node to fail the probe. If both our
probe and the indirect probes fail within a reasonable time, then the
node is marked "suspicious" and this knowledge is gossiped to the cluster.
A suspicious node is still considered a member of cluster. If the suspect member
of the cluster does not dispute the suspicion within a configurable period of
time, the node is finally considered dead, and this state is then gossiped
to the cluster.

This is a brief and incomplete description of the protocol. For a better idea,
please read the
[SWIM paper](https://www.cs.cornell.edu/~asdas/research/dsn02-swim.pdf)
in its entirety, along with the Serf source code.

## SWIM Modifications

As mentioned earlier, the gossip protocol is based on SWIM but includes
minor changes, mostly to increase propagation speed and convergence rates.

The changes from SWIM are noted here:

* Serf does a full state sync over TCP periodically. SWIM only propagates
  changes over gossip. While both are eventually consistent, Serf is able to
  more quickly reach convergence, as well as gracefully recover from network
  partitions.

* Serf has a dedicated gossip layer separate from the failure detection
  protocol. SWIM only piggybacks gossip messages on top of probe/ack messages.
  Serf uses piggybacking along with dedicated gossip messages. This
  feature lets you have a higher gossip rate (for example once per 200ms)
  and a slower failure detection rate (such as once per second), resulting
  in overall faster convergence rates and data propagation speeds.

* Serf keeps the state of dead nodes around for a set amount of time,
  so that when full syncs are requested, the requester also receives information
  about dead nodes. Because SWIM doesn't do full syncs, SWIM deletes dead node
  state immediately upon learning that the node is dead. This change again helps
  the cluster converge more quickly.

<a name="lifeguard"></a>
## Lifeguard Enhancements

SWIM makes the assumption that the local node is healthy in the sense
that soft real-time processing of packets is possible. However, in cases
where the local node is experiencing CPU or network exhaustion this assumption
can be violated. The result is that the node health can occassionally flap,
resulting in false monitoring alarms, adding noise to telemetry, and simply
causing the overall cluster to waste CPU and network resources diagnosing a
failure that may not truly exist.

Serf 0.8 added Lifeguard, which completely resolves this issue with novel
enhancements to SWIM.

The first extension introduces a "nack" message to probe queries. If the
probing node realizes it is missing "nack" messages then it becomes aware
that it may be degraded and slows down its failure detector. As nack messages
begin arriving, the failure detector is sped back up.

The second change introduces a dynamically changing suspicion timeout
before declaring another node as failured. The probing node will initially
start with a very long suspicion timeout. As other nodes in the cluster confirm
a node is suspect, the timer accelerates. During normal operations the
detection time is actually the same as in previous versions of Serf. However,
if a node is degraded and doesn't get confirmations, there is a long timeout
which allows the suspected node to refute its status and remain healthy.

These two mechanisms combine to make Serf much more robust to degraded nodes in a
cluster, while keeping failure detection performance unchanged. There is no
additional configuration for Lifeguard, it tunes itself automatically.

For more details about Lifeguard, please see the
[Making Gossip More Robust with Lifeguard](https://www.hashicorp.com/blog/making-gossip-more-robust-with-lifeguard/)
blog post, which provides a high level overview of the HashiCorp Research paper
[Lifeguard : SWIM-ing with Situational Awareness](https://arxiv.org/abs/1707.00788).

## Serf-Specific Messages

On top of the SWIM-based gossip layer, Serf sends some custom message types.

Serf makes heavy use of [Lamport clocks](https://en.wikipedia.org/wiki/Lamport_timestamps)
to maintain some notion of message ordering despite being eventually
consistent. Every message sent by Serf contains a Lamport clock time.

When a node gracefully leaves the cluster, Serf sends a _leave intent_ through
the gossip layer. Because the underlying gossip layer makes no differentiation
between a node leaving the cluster and a node being detected as failed, this
allows the higher level Serf layer to detect a failure versus a graceful
leave.

When a node joins the cluster, Serf sends a _join intent_. The purpose
of this intent is solely to attach a Lamport clock time to a join so that
it can be ordered properly in case a leave comes out of order.

For custom events and queries, Serf sends either a _user event_,
or _user query_ message. This message contains a Lamport time, event name, and event payload.
Because user events are sent along the gossip layer, which uses UDP, the payload and entire message framing
must fit within a single UDP packet.

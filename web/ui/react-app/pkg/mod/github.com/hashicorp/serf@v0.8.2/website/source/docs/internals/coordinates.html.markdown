---
layout: "docs"
page_title: "Network Coordinates"
sidebar_current: "docs-internals-coordinates"
description: |-
Serf uses a network tomography system to compute network coordinates for nodes in the cluster. These coordinates are useful for easily calculating the estimated network round trip time between any two nodes in the cluster. This page documents the details of this system. The core of the network tomography system us based on Vivaldi: A Decentralized Network Coordinate System, with several improvements based on several follow-on papers.
---

# Network Coordinates

Serf uses a [network tomography](https://en.wikipedia.org/wiki/Network_tomography)
system to compute network coordinates for nodes in the cluster. These coordinates
allow the network round trip time to be estimated between any two nodes using a
very simple calculation.

Coordinates are obtained by adding a very small amount of data to the probe
messages that Serf already sends as part of its [gossip protocol](/docs/internals/gossip.html),
so this system scales well and incurs very little overhead.

Serf's network tomography is based on ["Vivaldi: A Decentralized Network Coordinate System"](https://www.cs.ucsb.edu/~ravenben/classes/276/papers/vivaldi-sigcomm04.pdf), with some additions from ["Network Coordinates in the Wild"](https://www.usenix.org/legacy/events/nsdi07/tech/full_papers/ledlie/ledlie_html/index_save.html) and ["On Suitability of Euclidean Embedding for Host-Based Network Coordinate Systems"](http://domino.research.ibm.com/library/cyberdig.nsf/papers/492D147FCCEA752C8525768F00535D8A).

~> **Advanced Topic!** This page covers the technical details of
the internals of Serf. You don't need to know these details to effectively
operate and use Serf. These details are documented here for those who wish
to learn about them without having to go spelunking through the source code.

## Vivaldi Overview

The Vivaldi algorithm works in a manner that's similar to a simulation of a
system of nodes connected by springs. Nodes all start out bunched up at the origin,
and as they learn information about the distances to their peers over time,
they adjust their positions in order to minimize the energy stored in the
springs. After running a few iterations of this algorithm, the nodes will start
to find positions that will accurately predict their distances to their peers.
As things change in the network, the nodes will adjust their positions to
compensate.

Taking this into Serf, the distances are not physical distances but the round
trip times the agents observe when probing other nodes as a regular part of the
gossip protocol. It's important to note that in Serf the resulting raw coordinates
don't really model any particular physical layout. For example, they won't help
you figure out which rack a server is in. Instead, they form a more abstract
space that is only useful for calculating round trip times.

Here's an example showing how network tomography works, with the node `foo`
probing the node `bar`:

1. Node `foo` sends node `bar` a probe message, and tracks the time at which the
   probe was sent.

1. Node `bar` responds to the probe with its own current network coordinate.

1. Node `foo` tracks the time the probe response was received which gives it a
   round trip time estimate to node `bar`, comparing it to the time above.

1. Node `foo` also calculates a round trip time to node `bar` using only its
   current network coordinate and the one it got from node `bar`. If there's an
   error, node `foo` will update its own network coordinate to move to a place that
   makes this error smaller, as if it was being pushed or pulled by the other
   node in proportion to the amount of error.

So mapping back to the physical analogy, the physical distance corresponds to
network round trip time and the lowest energy spring configuration corresponds
to the set of coordinates that most closely estimate the observations.

Vivaldi is a natural fit to Serf because, as the example shows, it's based only
on observations between peers and doesn't rely on any centralized system to
manage coordinates. Also, it builds a useful set of coordinates with only a
limited number of interactions between peers so it scales to many, many nodes in
a way that direct pings between all possible pairs of nodes never would.

## Additional Enhancements

Guided by the follow-on papers cited above, there were several enhancements
added to Serf's implementation on top of the core Vivaldi algorithm:

* A non-Euclidean "height" term was added to the network coordinates in order to
  to help model real-world situations. In particular, height helps model access
  links that nodes have to the rest of the network, such as the fixed latency
  overhead of a virtual machine through a hypervisor to a network card.

* Another non-Euclidean "adjustment" term was added to help the system perform
  better with hosts that are near each other in terms of network round trip time.

* The last known coordinate is saved and serves as the starting point when a
  Serf agent restarts. This helps reduce churn in the coordinate system versus
  always starting nodes at the origin.

* A "gravity" effect was added to gently pull the cluster's coordinates back
  into a system that's roughly centered around the origin. Without this, over
  long periods of time, the nodes might all drift which is undesirable for
  accuracy. For example, the components of the vectors could take on large
  values, and the default position of new nodes at the origin would be far
  outside the rest of the space.

* A latency filter was added to smooth the round trip time observations as they
  feed into the Vivaldi algorithm. This prevents occasional, real-world "pops"
  in these measurements from introducing noise and instability into the system,
  at the expense of a slightly longer response time to actual changes in round
  trip times.

With all these in place, a network coordinate consists of an eight-dimensional
Euclidean vector, plus single values for adjustment, error, and height. Eight
dimensions were chosen based on a survey of the literature as well as some
experimental testing.

The following section shows how to perform calculations with these coordinates.

## Working with Coordinates

Computing the estimated network round trip time between any two nodes is simple
once you have their coordinates. Here's a sample coordinate, as returned from the
`get-coordinate` [RPC call](/docs/agent/rpc.html):

```
    "Coord": {
        "Adjustment": 0.1,
        "Error": 1.5,
        "Height": 0.02,
        "Vec": [0.34,0.68,0.003,0.01,0.05,0.1,0.34,0.06]
    }
```

All values are floating point numbers in units of seconds, except for the error
term which isn't used for distance calculations.

Here's a complete example in Go showing how to compute the distance between two
coordinates:

```
import (
    "github.com/hashicorp/serf/coordinate"
    "math"
    "time"
)

func dist(a *coordinate.Coordinate, b *coordinate.Coordinate) time.Duration {
    // Coordinates will always have the same dimensionality, so this is
    // just a sanity check.
    if len(a.Vec) != len(b.Vec) {
        panic("dimensions aren't compatible")
    }

    // Calculate the Euclidean distance plus the heights.
    sumsq := 0.0
    for i := 0; i < len(a.Vec); i++ {
        diff := a.Vec[i] - b.Vec[i]
        sumsq += diff * diff
    }
    rtt := math.Sqrt(sumsq) + a.Height + b.Height

    // Apply the adjustment components, guarding against negatives.
    adjusted := rtt + a.Adjustment + b.Adjustment
    if adjusted > 0.0 {
        rtt = adjusted
    }

    // Go's times are natively nanoseconds, so we convert from seconds.
    const secondsToNanoseconds = 1.0e9
    return time.Duration(rtt * secondsToNanoseconds)
}
```

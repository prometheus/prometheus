---
layout: "docs"
page_title: "Commands: Query"
sidebar_current: "docs-commands-query"
description: |-
  The `serf query` command dispatches a custom user query into a Serf cluster, efficiently broadcasting the query to all nodes, and gathering responses.
---

# Serf Query

Command: `serf query`

The `serf query` command dispatches a custom user query into a Serf cluster,
efficiently broadcasting the query to all nodes, and gathering responses.

Nodes in the cluster can listen for queries and respond to them.
Example use cases of queries are to request load average, ask for the
version of an app that is deployed, or to trigger deploys across web nodes
by sending a "deploy" query, possibly with a commit payload.

The command will wait until the query finishes (by reaching a timeout) and
will report all acknowledgements and responses that are received.

The open ended nature of `serf query` allows you to send and respond to
queries in _any way_ you want.

The main distinction between a Serf query and an event is that events
are fire-and-forget. The Serf client will send the event immediately and
will broadcast to the entire cluster. Events have no filtering mechanism
and cannot reply or acknowledge receipt. Serf also tries harder to deliver
events, by performing anti-entropy over TCP as well as message replay.

Queries are intended to be a real-time request and response mechanism.
Since they are intended to be time sensitive, Serf will not do message
replay or anti-entropy, as a response to a very old query is not useful.
Queries have more advanced filtering mechanisms and can be used to build
more complex control flow. For example, a code deploy could check that at
least 90% of nodes successfully deployed before continuing.

## Usage

Usage: `serf query [options] name [payload]`

The command-line flags are all optional. The list of available flags are:

* `-format` - Controls the output format. Supports `text` and `json`.
  The default format is `text`.

* `-no-ack` - If provided, the query will not request that nodes acknowledge
  receipt of the query. By default, any nodes that pass the `-node` and `-tag` filters
  will acknowledge receipt of a query and potentially respond if they have a configured
  event handler.

* `-relay-factor` - Available in Serf 0.8.1 and later, if provided, nodes responding to
  the query will relay their response through the specified number of other nodes for
  redundancy. Must be between 0 and 255.

* `-node node` - If provided, output is filtered to only nodes with the given
  node name. `-node` can be specified multiple times to allow multiple nodes.

* `-tag key=value` - If provided, output is filtered to only nodes with the specified
  tag if its value matches the regular expression. tag can be specified
  multiple times to filter on multiple keys.

* `-timeout=15s` - When provided, the given timeout overrides the default query timeout.
  By default, a query has a timeout that is designed to give the cluster enough time
  to gossip the message out and to respond. This is a computed multiple of the
  GossipInterval, a QueryTimeoutMultipler and a logarithmic scale based on cluster size.
  This time may be low for long running queries, so this flag can be used to specify
  an alternative timeout.

* `-rpc-addr` - Address to the RPC server of the agent you want to contact
  to send this command. If this isn't specified, the command will contact
  "127.0.0.1:7373" which is the default RPC address of a Serf agent. This option
  can also be controlled using the `SERF_RPC_ADDR` environment variable.

* `-rpc-auth` - Optional RPC auth token. If the agent is configured to use
  an auth token, then this must be provided or the agent will refuse the
  command.  This option can also be controlled using the `SERF_RPC_AUTH`
  environment variable.

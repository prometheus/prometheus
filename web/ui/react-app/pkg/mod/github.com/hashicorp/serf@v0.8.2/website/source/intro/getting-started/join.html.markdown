---
layout: "intro"
page_title: "Join a Cluster"
sidebar_current: "gettingstarted-join"
description: |-
  In the previous page, we started our first agent. While it showed how easy it is to run Serf, it wasn't very exciting since we simply made a cluster of one member. In this page, we'll create a real cluster with multiple members.
---

# Join a Cluster

In the previous page, we started our first agent. While it showed how easy
it is to run Serf, it wasn't very exciting since we simply made a cluster of
one member. In this page, we'll create a real cluster with multiple members.

When starting a Serf agent, it begins without knowledge of any other node, and is
an isolated cluster of one.  To learn about other cluster members, the agent must
_join_ an existing cluster.  To join an existing cluster, Serf only needs to know
about a _single_ existing member. After it joins, the agent will gossip with this
member and quickly discover the other members in the cluster.

## Starting the Agents

To simulate a more realistic cluster, we are using a two node cluster in
Vagrant. The Vagrantfile can be found in the demo section of the repo
[here](https://github.com/hashicorp/serf/tree/master/demo/vagrant-cluster).

We start the first agent on our first node and also
specify a node name. The node name must be unique and is how a machine
is uniquely identified. By default it is the hostname of the machine, but
we'll manually override it. We are also providing a bind address. This is the
address that Serf listens on, and it *must* be accessible by all other nodes
in the cluster.

```
$ serf agent -node=agent-one -bind=172.20.20.10
...
```

Then, in another terminal, start the second agent on the new node.
This time, we set the bind address to match the IP of the second node
as specified in the Vagrantfile. In production, you will generally want
to provide a bind address or interface as well.

```
$ serf agent -node=agent-two -bind=172.20.20.11
...
```

At this point, you have two Serf agents running. The two Serf agents
still don't know anything about each other, and are each part of their own
clusters (of one member). You can verify this by running `serf members`
against each agent and noting that only one member is a part of each.

## Joining a Cluster

Now, let's tell the first agent to join the second agent by running
the following command in a new terminal:

```
$ serf join 172.20.20.11
Successfully joined cluster by contacting 1 nodes.
```

You should see some log output in each of the agent logs. If you read
carefully, you'll see that they received join information. If you
run `serf members` against each agent, you'll see that both agents now
know about each other:

```
$ serf members
agent-one     172.20.20.10:7946    alive
agent-two     172.20.20.11:7946    alive
```

-> **Note:** To join a cluster, a Serf agent needs to only
learn about <em>one existing member</em>. After joining the cluster, the
agents gossip with each other to propagate full membership information.

In addition to using `serf join` you can use the `-join` flag on
`serf agent` to join a cluster as part of starting up the agent.

## Leaving a Cluster

To leave the cluster, you can either gracefully quit an agent (using
`Ctrl-C`) or force kill one of the agents. Gracefully leaving allows
the node to transition into the _left_ state, otherwise other nodes
will detect it as having _failed_. The difference is covered
in more detail [here](/intro/getting-started/agent.html#toc_3).

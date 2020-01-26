---
layout: "intro"
page_title: "Run the Agent"
sidebar_current: "gettingstarted-agent"
description: |-
  After Serf is installed, the agent must be run. The agent is a lightweight process that runs until told to quit and maintains cluster membership and communication. The agent must be run for every node that will be part of the cluster.
---

# Run the Serf Agent

After Serf is installed, the agent must be run. The agent is a lightweight
process that runs until told to quit and maintains cluster membership
and communication. The agent must be run for every node that will be part of
the cluster.

It is possible to run multiple agents, and thus participate in multiple Serf
clusters. For example, you may want to run a separate Serf cluster to
maintain web server membership info for a load balancer from another Serf
cluster that manages membership of Memcached nodes, but perhaps the web
servers need to be part of the Memcached cluster too so they can be notified
when Memcached nodes come online or go offline. Other examples include a Serf
cluster within a datacenter, and a separate cluster used for cross WAN gossip
which has more relaxed timing.

## Starting the Agent

For simplicity, we'll run a single Serf agent right now:

```
$ serf agent
==> Starting Serf agent...
==> Serf agent running!
    Node name: 'foobar'
    Bind addr: '0.0.0.0:7946'
     RPC addr: '127.0.0.1:7373'

==> Log data will now stream in as it occurs:

2013/10/21 18:57:15 [INFO] Serf agent starting
2013/10/21 18:57:15 [INFO] serf: EventMemberJoin: mitchellh.local 10.0.1.60
2013/10/21 18:57:15 [INFO] Serf agent started
2013/10/21 18:57:15 [INFO] agent: Received event: member-join
```

As you can see, the Serf agent has started and has output some log
data. From the log data, you can see that a member has joined the cluster.
This member is yourself.

## Cluster Members

If you run `serf members` in another terminal, you can see the members of
the Serf cluster. You should only see one member (yourself). We'll cover
joining clusters in the next section.

```
$ serf members
mitchellh.local    10.0.1.60    alive
```

This command, along with many others, communicates with a running Serf
agent via an internal RPC protocol. When starting the Serf agent, you
may have noticed that it tells you the "RPC addr". This is the address
that commands such as `serf members` use to communicate with the agent.

By default, RPC listens only on loopback, so it is inaccessible outside
of your machine for security reasons.

If you're running multiple Serf agents, you'll have to specify
an `-rpc-addr` to both the agent and any commands so that it doesn't
collide with other agents.

## Stopping the Agent

You can use `Ctrl-C` (the interrupt signal) to gracefully halt the agent.
After interrupting the agent, you should see it leave the cluster gracefully
and shut down.

By gracefully leaving, Serf notifies other cluster members that the
node _left_. If you had forcibly killed the agent process, other members
of the cluster would have detected that the node _failed_. This can be a
crucial difference depending on what your use case of Serf is. Serf will
automatically try to reconnect to _failed_ nodes, which allows it to recover
from certain network conditions, while _left_ nodes are no longer contacted.


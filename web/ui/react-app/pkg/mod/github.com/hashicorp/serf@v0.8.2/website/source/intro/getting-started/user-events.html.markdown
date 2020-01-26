---
layout: "intro"
page_title: "Custom User Events"
sidebar_current: "gettingstarted-userevents"
description: |-
  In addition to the standard membership-related events that Serf fires, Serf is able to propagate custom events across the cluster. Events are fire-and-forget. There is no originating node, an event has no response, and Serf works hard to ensure it is delivered to all nodes. Custom events are useful for tasks such as: triggering deploys, telling the cluster to restart, etc.
---

# Custom User Events

In addition to the standard membership-related events that Serf fires,
Serf is able to propagate custom events across the cluster. Events are
"fire-and-forget". There is no originating node, an event has no response,
and Serf works hard to ensure it is delivered to all nodes. Custom events
are useful for tasks such as: triggering deploys, telling the cluster to
restart, etc.

## Sending Custom Events

First, start a Serf agent so we can see the event being sent. Since we'll
just be running a single agent, running `serf agent` by itself is fine.
Then, to send a custom event, use the `serf event` command:

```
$ serf event hello-there
```

If you look at the output of `serf agent`, you should see that it received
the user event:

```
...
2013/10/22 07:06:32 [INFO] agent: Received event: user-event: hello-there
```

If the cluster were made up of multiple members, all of the members
would have received this event, eventually.

Just like normal Serf events, event handlers can respond to user events.
For example, if we had a "restart" custom event, we might create an
event handler that restarts some server when it receives that event.

## Event Payloads

Events are not limited to just an event name. The event can also contain
a payload: arbitrary data associated with the event. With our same agent
running, let's deliver an event with a payload: `serf event my-name-is Mitchell`

In practice, event payloads can contain information such as the git commit
to deploy if you're using Serf as a deployment tool. Or perhaps it contains
some updated configuration to modify on the nodes. It can contain anything
you'd like; it is up to the event handler to use it in some meaningful way.

## Custom Event Limitations

Custom events are delivered using the Serf gossip layer. The benefits of
this approach is that you get completely decentralized messaging across
your entire cluster that is fault tolerant. Even if a node is down, it will
eventually receive that event message.

Due to the mechanics of gossip, custom
events are highly scalable: Serf doesn't need to connect to each and every
node to send the message, it only needs to connect to a handful, regardless
of cluster size.

Custom events come with some trade-offs, however:

* Events are eventually consistent: Because events are delivered over
  gossip, the messages _eventually_ arrive at every node. In theory
  (and anecdotally in practice), the state of the cluster
  [converges rapidly](/docs/internals/simulator.html).

* Payload size is limited: Serf gossips via UDP, so the payload must fit
  within a single UDP packet (alongside any other data Serf sends). This
  limits the potential size of a payload to less than 1 KB. In practice,
  Serf limits the payload to a much smaller size.


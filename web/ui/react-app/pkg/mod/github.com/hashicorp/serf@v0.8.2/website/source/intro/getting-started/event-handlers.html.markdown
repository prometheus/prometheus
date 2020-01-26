---
layout: "intro"
page_title: "Event Handlers"
sidebar_current: "gettingstarted-eventhandlers"
description: |-
  We've now seen how to start Serf agents and join them into a cluster. While this is cool to see on its own, the true power and utility of Serf is being able to react to membership changes and other events that Serf invokes. By specifying event handlers, Serf will invoke custom scripts whenever an event is received.
---

# Event Handlers

We've now seen how to start Serf agents and join them into a cluster.
While this is cool to see on its own, the true power and utility of Serf
is being able to react to membership changes and other events that Serf
invokes. By specifying _event handlers_, Serf will invoke custom scripts
whenever an event is received.

## Our First Event Handler

To start, let's create our first event handler. Create a shell script
named `handler.sh` with the following contents and make sure that it is
set to be executable (`chmod +x handler.sh`).

```bash
#!/bin/bash

echo
echo "New event: ${SERF_EVENT}. Data follows..."
while read line; do
    printf "${line}\n"
done
```

This will be the script that we'll tell Serf to invoke for any event.
The script outputs the event, which Serf puts into the `SERF_EVENT`
environment variable. The data for a Serf event always comes in via
stdin, so the script then reads stdin and outputs any data it received.

By sending data to stdin, Serf works extremely well with standard Unix
tools such as `grep`, `sed`, `awk`, etc. Shell commands work when specifying
event handlers, so by using standard Unix methodologies, complex event
handlers can often be built up without resorting to custom scripts.

## Specifying an Event Handler

With the event handler written, let's start an agent with that event handler.
By setting the log-level to "debug", Serf will output the stdout/stderr
of the event handlers, so we can see them being run:

```
$ serf agent -log-level=debug -event-handler=handler.sh
==> Starting Serf agent...
==> Serf agent running!
    Node name: 'foobar'
    Bind addr: '0.0.0.0:7946'
     RPC addr: '127.0.0.1:7373'

==> Log data will now stream in as it occurs:

2013/10/22 06:54:04 [INFO] Serf agent starting
2013/10/22 06:54:04 [INFO] serf: EventMemberJoin: mitchellh 127.0.0.1
2013/10/22 06:54:04 [INFO] Serf agent started
2013/10/22 06:54:04 [INFO] agent: Received event: member-join
2013/10/22 06:54:04 [DEBUG] Event 'member-join' script output:
New event: member-join. Data follows...
mitchellh.local    127.0.0.1
```

As you can see from the tail end of the output, the event script was
executed and displayed the event that was run along with the data
of that event.
In this case, the event was a "member-join" event and the data was
a single member, ourself.

In practice, an event script would do something like adding a web
server to a load balancer, monitoring that node with Nagios, etc.

## Types of Events

There are currently seven types of events that Serf invokes:

* `member-join` - One or more members have joined the cluster.
* `member-leave` - One or more members have gracefully left the cluster.
* `member-failed` - One or more members have failed, meaning that they
  didn't properly respond to ping requests.
* `member-update` - One or more members have updated, likely to update the
  associated tags
* `member-reap` - Serf has removed one or more members from its list of members.
  This means a failed node exceeded the `reconnect_timeout`, or a left node reached
  the `tombstone_timeout`.
* `user` - A custom user event, covered later in this guide.
* `query` - A query event, covered later in this guide

## Multiple Event Scripts, Filtering, And More

For the purposes of introduction, we showed how to use a single event
handler with Serf. This event handler responded to all events. However,
the event handling system is actually far more robust: Serf is able
to invoke multiple event handlers as well as invoke certain event handlers
for only certain Serf events.

To learn more about these features, see the full documentation section
of [event handlers](/docs/agent/event-handlers.html).


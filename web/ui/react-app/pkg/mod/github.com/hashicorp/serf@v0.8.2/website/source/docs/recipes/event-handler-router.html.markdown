---
layout: "docs"
page_title: "Event Handler Router Recipe"
sidebar_current: "docs-recipes"
description: |-
  Typically you must configure a handler for each type of event you expect to encounter. To change handler configuration, one must update the Serf configuration and reload the agent with a SIGHUP or a restart.
---

# Recipe: Event handler router

Typically you must configure a handler for each type of event you expect to
encounter (more about events and handlers [here](/docs/agent/event-handlers.html)).
To change handler configuration, one must update the Serf configuration and reload
the agent with a SIGHUP or a restart.

Thanks to the flexibility and open-endedness Serf offers for configuring and
executing handlers, it is possible to achieve similar functionality by
configuring only a single "router" handler, which never needs to be updated.
This removes the orchestration work of reloading the agents by allowing one to
simply drop new executables with predictable names into a directory.

Handler executables must be named by event type for this recipe to work
(e.g. `member-join`). User events get prefixed with "user-", and queries with
"query-" (`user-deploy`, `query-uptime`, etc.).

What you will end up with is a directory structure that looks like this:

```
$ tree /etc/serf
/etc/serf
└── handlers
    ├── member-failed
    ├── member-join
    ├── member-leave
    ├── member-update
    ├── user-deploy
    └── query-uptime
```

## Handler code

The following code must be configured as the only handler for this recipe to
work. It will act as a catch-all this way and be able to make decisions on what
script handler to invoke based on the Serf environment variables.

```
#!/bin/sh
HANDLER_DIR="/etc/serf/handlers"

if [ "$SERF_EVENT" = "user" ]; then
    EVENT="user-$SERF_USER_EVENT"
elif [ "$SERF_EVENT" = "query" ]; then
    EVENT="query-$SERF_QUERY_NAME"
else
    EVENT=$SERF_EVENT
fi

HANDLER="$HANDLER_DIR/$EVENT"
[ -f "$HANDLER" -a -x "$HANDLER" ] && exec "$HANDLER" || :
```

## pluginhook

The [pluginhook](https://github.com/progrium/pluginhook) project offers
an alternative way that enables multiple plugins to handle a hook, instead
of only a single handler.

Using this enables a tree structure that looks like:

```
$ tree /etc/serf
/etc/serf
└── handlers
    ├── a-1st-plugin
    |   ├── member-failed
    |   ├── member-join
    |   ├── member-leave
    |   └── member-update
    ├── a-2nd-plugin
    |   ├── member-update
    |   ├── user-deploy
    |   └── query-uptime
    └── an-nth-plugin
        ├── member-join
        └── member-leave
```

Using this requires only requires minor modifications to our handler script
above:

```
#!/bin/sh
HANDLER_DIR="/etc/serf/handlers"

if [ "$SERF_EVENT" = "user" ]; then
    EVENT="user-$SERF_USER_EVENT"
elif [ "$SERF_EVENT" = "query" ]; then
    EVENT="query-$SERF_QUERY_NAME"
else
    EVENT=$SERF_EVENT
fi

cd $HANDLER_DIR
pluginhook $EVENT
```

# serf-master

The [serf-master](https://github.com/garethr/serf-master) project provides the same functionality but allows you to write handlers strictly in Python. Using serf-master might be a better approach if you are more comfortable with Python than Bash.

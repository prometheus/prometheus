---
layout: "docs"
page_title: "Agent Uptime Recipe"
sidebar_current: "docs-recipes"
description: |-
  On occasion users have asked for the ability to identify the uptime of a Serf agent. While Serf does not expose this directly in the RPC layer, it is quite easy to utilize tags to accomplish this.
---

# Recipe: Agent Uptime

On occasion users have asked for the ability to identify the uptime of a Serf
agent. While Serf does not expose this directly in the RPC layer, it is quite
easy to utilize tags to accomplish this.

The solution is simple. While starting a Serf agent, add a tag to indicate the
time at which it was started:

```
serf agent -tag start_time=`date +%s`
```

While the agent is running, you can retrieve its uptime in seconds by getting
the current UNIX timestamp and doing simple subtraction.

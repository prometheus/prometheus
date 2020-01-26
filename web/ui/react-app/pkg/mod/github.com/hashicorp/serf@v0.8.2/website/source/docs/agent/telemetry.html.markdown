---
layout: "docs"
page_title: "Telemetry"
sidebar_current: "docs-agent-telemetry"
description: |-
  The Serf agent collects various metrics data at runtime about the performance of different libraries and sub-systems. These metrics are aggregated on a ten second interval and are retained for one minute.
---

# Telemetry

The Serf agent collects various metrics data at runtime about the performance
of different libraries and sub-systems. These metrics are aggregated on a ten second
interval and are retained for one minute.

To view the telemetry information, you must send a `USR1` signal to the Serf
process. Windows users must use the `BREAK` signal instead.
Once Serf receives the signal, it will dump the current telemetry
information to the stderr of the agent.

In general, the telemetry information is useful for debugging or otherwise
getting a better view into what Serf is doing.

Below is an example output:

```
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.num_goroutines': 19.000
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.alloc_bytes': 755960.000
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.malloc_count': 7550.000
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.free_count': 4387.000
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.heap_objects': 3163.000
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.total_gc_pause_ns': 1151002.000
[2014-01-29 10:56:50 -0800 PST][G] 'serf-agent.runtime.total_gc_runs': 4.000
[2014-01-29 10:56:50 -0800 PST][C] 'serf-agent.agent.ipc.accept': Count: 5 Sum: 5.000
[2014-01-29 10:56:50 -0800 PST][C] 'serf-agent.agent.ipc.command': Count: 10 Sum: 10.000
[2014-01-29 10:56:50 -0800 PST][C] 'serf-agent.serf.events': Count: 5 Sum: 5.000
[2014-01-29 10:56:50 -0800 PST][C] 'serf-agent.serf.events.foo': Count: 4 Sum: 4.000
[2014-01-29 10:56:50 -0800 PST][C] 'serf-agent.serf.events.baz': Count: 1 Sum: 1.000
[2014-01-29 10:56:50 -0800 PST][S] 'serf-agent.memberlist.gossip': Count: 50 Min: 0.007 Mean: 0.020 Max: 0.041 Stddev: 0.007 Sum: 0.989
[2014-01-29 10:56:50 -0800 PST][S] 'serf-agent.serf.queue.Intent': Count: 10 Sum: 0.000
[2014-01-29 10:56:50 -0800 PST][S] 'serf-agent.serf.queue.Event': Count: 10 Min: 0.000 Mean: 2.500 Max: 5.000 Stddev: 2.121 Sum: 25.000
```


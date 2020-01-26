---
layout: "intro"
page_title: "Serf vs. Chef, Puppet, etc."
sidebar_current: "vs-other-chef"
description: |-
  It may seem strange to compare Serf to configuration management tools, but most of them provide mechanisms to incorporate global state into the configuration of a node. For example, Puppet provides exported resources and Chef has node searching. As an example, if you generate a config file for a load balancer to include the web servers, the config management tool is being used to manage membership.
---

# Serf vs. Chef, Puppet, etc.

It may seem strange to compare Serf to configuration management tools,
but most of them provide mechanisms to incorporate global state into the
configuration of a node. For example, Puppet provides exported resources
and Chef has node searching. As an example, if you generate a config file
for a load balancer to include the web servers, the config management
tool is being used to manage membership.

However, none of these config management tools are designed to perform
this task. They are not designed to propagate information quickly,
handle failure detection, or tolerate network partitions. Generally,
they rely on very infrequent convergence runs to bring things up to date.
Lastly, these tools are not friendly for immutable infrastructure as they
require constant operation to keep nodes up to date.

That said, Serf is designed to be used alongside config management tools.
Once configured, Serf can be used to handle changes to the cluster and
update configuration files nearly instantly instead of relying on convergence
runs. This way, a web server can join a cluster in seconds instead of hours.
The separation of configuration management and cluster management also has
a number of advantageous side effects: Chef recipes and Puppet manifests become
simpler without global state, periodic runs are no longer required for
membership updates, and the infrastructure can become immutable since
config management runs require no global state.

# Serf Demo: Web Servers + Load Balancer

In this demo, Serf is used to coordinate web servers entering a load
balancer. The nodes in this example can come up in any order and they'll
eventually converge to a running cluster. When visiting the load balancer,
you'll see each of the nodes visited in round-robin.

After telling the cluster to launch, no further human interaction is
required. The cluster will launch and orchestrate itself into a load
balanced web server setup.

The nodes are launched on AWS using CloudFormation and therefore will
incur a real cost. You can control this cost based on how many nodes
you decide to launch.

## Running the Demo

To run the demo, log into your AWS console, go to the CloudFormation
tab, and create a new stack using the "cloudformation.json" file in this
directory.

Configure how many web nodes you'd like to launch then create the stack.
When the stack is finished creating, the load balancer IP will be in
the "outputs" tab of that stack.

Visit `http://IP:9999/` to see the HAProxy stats page and list of
configured backends. Or visit the IP directly in your browser to see
it round robin between the web nodes.

It will take some time for the load balancer and web servers to install
their software and configure themselves. If the load balancer stats page
is not up yet, give it a few seconds and try again. Once it is up, it
will automatically refresh so you can watch as nodes eventually come
online.

## How it Works

### Node Setup

All the nodes (webs and load balancer) come up at the same time, with no
explicit ordering defined. If there is an order, it doesn't actually matter
for the function of the demo. The web servers can all come up before the load
balancer, the load balancer can come up in the middle of some web servers, etc.

Each of the nodes does an initial setup using a shell script. In a real
environment, this initial setup would probably be done with a real
configuration management system. For the purposes of this demo, we use
shell scripts for ease of understanding.

The load balancer runs `setup_load_balancer.sh` which installs HAProxy.
HAProxy is configured with no backend servers. Serf will handle this later.

The web server runs `setup_web_server.sh` which installs Apache and
sets up an index.html to just say what host it is.

At this stage, the load balancer is running HAProxy, and the web servers
are running Apache, but they're not aware of each other in any way.

Next, all nodes run `setup_serf.sh` which installs Serf and configures it
to run. This script also starts the Serf agent. For web servers, it also
starts a task that continuosly tries to join the Serf cluster at address
`10.0.0.5`, which happens to be the static IP assigned to the load balancer.
Note that Serf can join _any_ node in the cluster. We just set a static IP
on the load balancer for ease of automation, but in a real production
environment you can use any existing node, potentially just using DNS.

Web server roles also make use of a "serf-query" upstart task to dynamically
update the index page they serve. By using a "load" query, each node asks
the rest of the cluster for their current load. This information is used
to dynamically generate the index page.

### Serf Configuration

The Serf configuration is very simple. Each node runs a Serf agent. The
web servers run the agent with a role of "web" and the load balancer runs
the agent with a role of "lb".

#### Event: member-join

On member-join, the following shell script is run. We'll talk about the
function of the shell script after the code sample.

```sh
if [ "x${SERF_TAG_ROLE}" != "xlb" ]; then
    echo "Not an lb. Ignoring member join."
    exit 0
fi

while read line; do
    ROLE=`echo $line | awk '{print \$3 }'`
    if [ "x${ROLE}" != "xweb" ]; then
        continue
    fi

    echo $line | \
        awk '{ printf "    server %s %s check\n", $1, $2 }' >>/etc/haproxy/haproxy.cfg
done

/etc/init.d/haproxy reload
```

The script first checks if this node is a load balancer. If it isn't a load
balancer, we don't do anything on the "member-join" event. Web servers simply
don't care about other members.

Otherwise, if this is a load balancer, then it handles the "member-join" event
and reads the event data and updates the configuration of HAProxy. In a
production environment, you might use something like a template processor to do
this rather than appending to the configuration. For the purposes of this demo,
this works as well.

Finally, HAProxy is reloaded so the configuration is read.

The result of this is that as members join, they're automatically added into
rotation on the load balancer.

#### Event: member-leave, member-failed

On member-leave and member-failed evets, the following shell script is run.
This demo doesn't differentiate between the two events, treating both
the same way and simply removing the node from the HAProxy configuration.

```sh
if [ "x${SERF_TAG_ROLE}" != "xlb" ]; then
    echo "Not an lb. Ignoring member leave"
    exit 0
fi

while read line; do
    NAME=`echo $line | awk '{print \$1 }'`
    sed -i'' "/${NAME} /d" /etc/haproxy/haproxy.cfg
done

/etc/init.d/haproxy reload
```

This script also does nothing if this node is not a load balanacer. Otherwise,
it uses `sed` to remove each of the nodes from the HAProxy configuration and
reloads it.

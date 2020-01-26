#!/bin/sh
#
# This script installs and configures the Serf agent that runs on
# every node. As with the other scripts, this should probably be done with
# formal configuration management, but a shell script is simple as well.
#
# The SERF_ROLE environmental variable must be passed into this script
# in order to set the role of the machine. This should be either "lb" or
# "web".
#
set -e

sudo apt-get install -y unzip

# Download and install Serf
cd /tmp
until wget -O serf.zip https://dl.bintray.com/mitchellh/serf/0.6.4_linux_amd64.zip; do
    sleep 1
done
unzip serf.zip
sudo mv serf /usr/local/bin/serf

# The member join script is invoked when a member joins the Serf cluster.
# Our join script simply adds the node to the load balancer.
cat <<EOF >/tmp/join.sh
if [ "x\${SERF_TAG_ROLE}" != "xlb" ]; then
    echo "Not an lb. Ignoring member join."
    exit 0
fi

while read line; do
    ROLE=\`echo \$line | awk '{print \\\$3 }'\`
    if [ "x\${ROLE}" != "xweb" ]; then
        continue
    fi

    echo \$line | \\
        awk '{ printf "    server %s %s check\\n", \$1, \$2 }' >>/etc/haproxy/haproxy.cfg
done

/etc/init.d/haproxy reload
EOF
sudo mv /tmp/join.sh /usr/local/bin/serf_member_join.sh
chmod +x /usr/local/bin/serf_member_join.sh

# The member leave script is invoked when a member leaves or fails out
# of the serf cluster. Our script removes the node from the load balancer.
cat <<EOF >/tmp/leave.sh
if [ "x\${SERF_TAG_ROLE}" != "xlb" ]; then
    echo "Not an lb. Ignoring member leave"
    exit 0
fi

while read line; do
    NAME=\`echo \$line | awk '{print \\\$1 }'\`
    sed -i'' "/\${NAME} /d" /etc/haproxy/haproxy.cfg
done

/etc/init.d/haproxy reload
EOF
sudo mv /tmp/leave.sh /usr/local/bin/serf_member_left.sh
chmod +x /usr/local/bin/serf_member_left.sh

# Configure the agent
cat <<EOF >/tmp/agent.conf
description "Serf agent"

start on runlevel [2345]
stop on runlevel [!2345]

exec /usr/local/bin/serf agent \\
    -event-handler "member-join=/usr/local/bin/serf_member_join.sh" \\
    -event-handler "member-leave,member-failed=/usr/local/bin/serf_member_left.sh" \\
    -event-handler "query:load=uptime" \\
    -tag role=${SERF_ROLE} >>/var/log/serf.log 2>&1
EOF
sudo mv /tmp/agent.conf /etc/init/serf.conf

# Start the agent!
sudo start serf

# If we're the web node, then we need to configure the join retry
if [ "x${SERF_ROLE}" != "xweb" ]; then
    exit 0
fi

cat <<EOF >/tmp/join.conf
description "Join the serf cluster"

start on runlevel [2345]
stop on runlevel [!2345]

task
respawn

script
    sleep 5
    exec /usr/local/bin/serf join 10.0.0.5
end script
EOF
sudo mv /tmp/join.conf /etc/init/serf-join.conf
sudo start serf-join

cat <<EOF >/tmp/query.conf
description "Query the serf cluster load"

start on runlevel [2345]
stop on runlevel [!2345]

respawn

script
    echo `date` I am "${HOSTNAME}<br>" > /var/www/index.html.1
    serf query -no-ack load | sed 's|$|<br>|' >> /var/www/index.html.1
    mv /var/www/index.html.1 /var/www/index.html
    sleep 10
end script
EOF
sudo mv /tmp/query.conf /etc/init/serf-query.conf
sudo start serf-query

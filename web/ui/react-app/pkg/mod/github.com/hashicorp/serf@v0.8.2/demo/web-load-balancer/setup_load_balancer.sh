#!/bin/sh
#
# This script sets up the HAProxy load balancer initially, configured with
# no working backend servers. Presumably in a real environment you would
# do this sort of setup with a real configuration management system. For
# this demo, however, this shell script will suffice.
#
set -e

# Install HAProxy
sudo apt-get update
sudo apt-get install -y haproxy

# Configure it in a jank way
cat <<EOF >/tmp/haproxy.cfg
global
    daemon
    maxconn 256

defaults
    mode http
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

listen stats
    bind *:9999
    mode http
    stats enable
    stats uri /
    stats refresh 2s

listen http-in
    bind *:80
    balance roundrobin
    option http-server-close
EOF
sudo mv /tmp/haproxy.cfg /etc/haproxy/haproxy.cfg

# Enable HAProxy
cat <<EOF >/tmp/haproxy
ENABLED=1
EOF
sudo mv /tmp/haproxy /etc/default/haproxy

# Start it
sudo /etc/init.d/haproxy start

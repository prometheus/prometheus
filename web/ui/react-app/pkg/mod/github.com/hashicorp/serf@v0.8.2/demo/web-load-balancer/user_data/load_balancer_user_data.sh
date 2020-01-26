#!/bin/bash
NODE_SETUP_URL="https://raw.github.com/hashicorp/serf/master/demo/web-load-balancer/setup_load_balancer.sh"

SERF_SETUP_URL="https://raw.github.com/hashicorp/serf/master/demo/web-load-balancer/setup_serf.sh"

# Setup the node itself
wget -O - $NODE_SETUP_URL | bash

# Setup the serf agent
export SERF_ROLE="lb"
wget -O - $SERF_SETUP_URL | bash

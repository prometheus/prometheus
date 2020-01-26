#!/bin/sh
#
# This script sets up the web servers. The web servers just output who
# they are initially. Serf will do some additional configuration. Presumably
# in a real environment you would use a real configuration management system
# to do this. But for this demo a shell script is used for simplicity.
#
set -e

# Install apache2
sudo apt-get update
sudo apt-get install -y apache2

HOSTNAME=`hostname`
cat <<EOF >/tmp/index.html
I am "${HOSTNAME}"
EOF
sudo mv /tmp/index.html /var/www/index.html

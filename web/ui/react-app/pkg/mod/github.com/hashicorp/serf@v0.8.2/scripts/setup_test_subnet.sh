#!/bin/bash
#
# This script makes sure that 127.0.0.x is routable. On Darwin, there
# is a bug that it isn't routable and this causes errors.
#

# Fail if any commands fail (unchecked), namely sudo and ifconfig.
set -e

# Check if loopback is setup
if ping -c 1 -W 10 127.0.0.2 > /dev/null 2>&1
then
    exit
fi

# If we're not on OS X, then error
case $OSTYPE in
    darwin*)
        ;;
    *)
        echo "Can't setup interfaces on non-Mac. Error!"
        exit 1
        ;;
esac

# Setup loopback
echo "Using sudo to setup lo0 interface aliases for testing."
sudo -v
for ((i=2;i<256;i++))
do
    sudo -n ifconfig lo0 alias 127.0.0.$i up
done

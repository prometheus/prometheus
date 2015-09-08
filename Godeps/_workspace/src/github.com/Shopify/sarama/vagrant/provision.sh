#!/bin/sh

set -ex

apt-get update
yes | apt-get install default-jre

export KAFKA_INSTALL_ROOT=/opt
export KAFKA_HOSTNAME=192.168.100.67
export KAFKA_VERSION=0.8.2.1
export REPOSITORY_ROOT=/vagrant

sh /vagrant/vagrant/install_cluster.sh
sh /vagrant/vagrant/setup_services.sh
sh /vagrant/vagrant/create_topics.sh

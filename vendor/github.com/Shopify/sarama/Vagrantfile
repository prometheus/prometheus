# -*- mode: ruby -*-
# vi: set ft=ruby :

# Vagrantfile API/syntax version. Don't touch unless you know what you're doing!
VAGRANTFILE_API_VERSION = "2"

# We have 5 * 192MB ZK processes and 5 * 320MB Kafka processes => 2560MB
MEMORY = 3072

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.box = "ubuntu/trusty64"

  config.vm.provision :shell, path: "vagrant/provision.sh"

  config.vm.network "private_network", ip: "192.168.100.67"

  config.vm.provider "virtualbox" do |v|
    v.memory = MEMORY
  end
end

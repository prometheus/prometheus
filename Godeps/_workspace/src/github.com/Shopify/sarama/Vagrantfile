# -*- mode: ruby -*-
# vi: set ft=ruby :

# Vagrantfile API/syntax version. Don't touch unless you know what you're doing!
VAGRANTFILE_API_VERSION = "2"

MEMORY = 3072

Vagrant.configure(VAGRANTFILE_API_VERSION) do |config|
  config.vm.box = "hashicorp/precise64"

  config.vm.provision :shell, path: "vagrant/provision.sh"

  config.vm.network "private_network", ip: "192.168.100.67"

  config.vm.provider "vmware_fusion" do |v|
    v.vmx["memsize"] = MEMORY.to_s
  end
  config.vm.provider "virtualbox" do |v|
    v.memory = MEMORY
  end
end

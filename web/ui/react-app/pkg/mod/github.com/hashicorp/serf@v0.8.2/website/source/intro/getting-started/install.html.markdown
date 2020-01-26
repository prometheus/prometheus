---
layout: "intro"
page_title: "Installing Serf"
sidebar_current: "gettingstarted-install"
description: |-
  Serf must first be installed on every node that will be a member of a Serf cluster. To make installation easy, Serf is distributed as a binary package for all supported platforms and architectures. This page will not cover how to compile Serf from source.
---

# Install Serf

Serf must first be installed on every node that will be a member of a Serf
cluster. To make installation easy, Serf is distributed as a
[binary package](/downloads.html) for all supported platforms and architectures.

After downloading Serf, unzip the package. Copy the `serf` binary to
somewhere on the PATH so that it can be executed. On Unix systems,
`~/bin` and `/usr/local/bin` are common installation directories,
depending on if you want to restrict the install to a single user or
expose it to the entire system. On Windows systems, you can put it wherever
you would like.

It is also possible to build and install the `serf` binary through the standard
`go` command-line utility (`go get -u github.com/hashicorp/serf/cmd/serf` which
installs `serf` as `$GOPATH/bin/serf`), however we recommend running an official
release.

### OS X

If you are using [homebrew](http://brew.sh/#install) as a package manager,
than you can install serf as simple as:
```
brew install serf
```

## Verifying the Installation

After installing Serf, verify the installation worked by opening a new
terminal session and checking that `serf` is available. By executing
`serf` you should see help output similar to that below:

```
$ serf
usage: serf [--version] [--help] <command> [<args>]

Available commands are:
    agent           Runs a Serf agent
    event           Send a custom event through the Serf cluster
    force-leave     Forces a member of the cluster to enter the "left" state
    info            Provides debugging information for operators
    join            Tell Serf agent to join cluster
    keygen          Generates a new encryption key
    keys            Manipulate the internal encryption keyring used by Serf
    leave           Gracefully leaves the Serf cluster and shuts down
    members         Lists the members of a Serf cluster
    monitor         Stream logs from a Serf agent
    query           Send a query to the Serf cluster
    reachability    Test network reachability
    tags            Modify tags of a running Serf agent
    version         Prints the Serf version
```

If you get an error that `serf` could not be found, then your PATH
environmental variable was not setup properly. Please go back and ensure
that your PATH variable contains the directory where Serf was installed.

Otherwise, Serf is installed and ready to go!

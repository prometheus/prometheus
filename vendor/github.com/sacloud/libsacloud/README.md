libsacloud
===

[![GoDoc](https://godoc.org/github.com/sacloud/libsacloud?status.svg)](https://godoc.org/github.com/sacloud/libsacloud)
[![Build Status](https://travis-ci.org/sacloud/libsacloud.svg?branch=master)](https://travis-ci.org/sacloud/libsacloud)
[![Go Report Card](https://goreportcard.com/badge/github.com/sacloud/libsacloud)](https://goreportcard.com/report/github.com/sacloud/libsacloud)

This project provides various Go packages to perform operations
on [`SAKURA CLOUD APIs`](http://developer.sakura.ad.jp/cloud/api/1.1/).

See list of implemented API clients under this.

  - [High-level API(builder package)](https://godoc.org/github.com/sacloud/libsacloud/builder)
  - [Low-level API(api package)](https://godoc.org/github.com/sacloud/libsacloud/api)
  - [Model defines(sacloud package)](https://godoc.org/github.com/sacloud/libsacloud/sacloud)



**Notice:** This is the library formerly known as
`github.com/yamamoto-febc/libsacloud` -- Github will automatically redirect requests
to this repository, but we recommend updating your references for clarity.



# Installation

    go get -d github.com/sacloud/libsacloud

# Sample (High-level API)

This sample will create a server by using a High-level API.

High-level API's document is [here](https://godoc.org/github.com/sacloud/libsacloud/builder).

##  Create a server

```go
package main

import (
	"fmt"
	"github.com/sacloud/libsacloud/api"
	"github.com/sacloud/libsacloud/builder"
	"github.com/sacloud/libsacloud/sacloud/ostype"
)

var (
	token      = "PUT-YOUR-TOKEN"    // API token
	secret     = "PUT-YOUR-SECRET"   // API secret
	zone       = "tk1a"              // target zone [tk1a or is1b]
	serverName = "example-server"    // server name
	password   = "PUT-YOUR-PASSWORD" // password
	core       = 2                   // cpu core
	memory     = 4                   // memory size(GB)
	diskSize   = 100                 // disk size(GB)

	// public key
	sshKey = "ssh-rsa AAAA..."

	// startup script
	script = `#!/bin/bash
yum -y update || exit 1
exit 0`
)

func main() {

	// create SakuraCloud API client
	client := api.NewClient(token, secret, zone)

	// Create server using CentOS public archive
	result, err := builder.ServerPublicArchiveUnix(client, ostype.CentOS, serverName, password).
		AddPublicNWConnectedNIC(). // connect shared segment
		SetCore(core).             // set cpu core
		SetMemory(memory).         // set memory size
		SetDiskSize(diskSize).     // set disk size
		AddSSHKey(sshKey).         // regist ssh public key
		SetDisablePWAuth(true).    // disable password auth
		AddNote(script).           // regist startup script
		Build()                    // build server

	if err != nil {
		panic(err)
	}
	fmt.Printf("%#v", result.Server)
}
```


# Sample (Low-level API)

This sample is a translation of the examples of [saklient](http://sakura-internet.github.io/saklient.doc/) to golang.

Original(saklient) sample codes is [here](http://sakura-internet.github.io/saklient.doc/).

Low-level API's document is [here](https://godoc.org/github.com/sacloud/libsacloud/api).

##  Create a server

```go

package main

import (
	"fmt"
	"github.com/sacloud/libsacloud/api"
	"os"
	"time"
)

func main() {

	// settings
	var (
		token        = os.Args[1]
		secret       = os.Args[2]
		zone         = os.Args[3]
		name         = "libsacloud demo"
		description  = "libsacloud demo description"
		tag          = "libsacloud-test"
		cpu          = 1
		mem          = 2
		hostName     = "libsacloud-test"
		password     = "C8#mf92mp!*s"
		sshPublicKey = "ssh-rsa AAAA..."
	)

	// authorize
	client := api.NewClient(token, secret, zone)

	//search archives
	fmt.Println("searching archives")
	archive, _ := client.Archive.FindLatestStableCentOS()

	// search scripts
	fmt.Println("searching scripts")
	res, _ := client.Note.
		WithNameLike("WordPress").
		WithSharedScope().
		Limit(1).
		Find()
	script := res.Notes[0]

	// create a disk
	fmt.Println("creating a disk")
	disk := client.Disk.New()
	disk.Name = name
	disk.Description = description
	disk.Tags = []string{tag}
	disk.SetDiskPlanToSSD()
	disk.SetSourceArchive(archive.ID)

	disk, _ = client.Disk.Create(disk)

	// create a server
	fmt.Println("creating a server")
	server := client.Server.New()
	server.Name = name
	server.Description = description
	server.Tags = []string{tag}

	// set ServerPlan
	plan, _ := client.Product.Server.GetBySpec(cpu, mem)
	server.SetServerPlanByID(plan.GetStrID())

	server, _ = client.Server.Create(server)

	// connect to shared segment

	fmt.Println("connecting the server to shared segment")
	iface, _ := client.Interface.CreateAndConnectToServer(server.ID)
	client.Interface.ConnectToSharedSegment(iface.ID)

	// wait disk copy
	err := client.Disk.SleepWhileCopying(disk.ID, 120*time.Second)
	if err != nil {
		fmt.Println("failed")
		os.Exit(1)
	}

	// config the disk
	diskConf := client.Disk.NewCondig()
	diskConf.SetHostName(hostName)
	diskConf.SetPassword(password)
	diskConf.AddSSHKeyByString(sshPublicKey)
	diskConf.AddNote(script.GetStrID())
	client.Disk.Config(disk.ID, diskConf)

	// connect to server
	client.Disk.ConnectToServer(disk.ID, server.ID)

	// boot
	fmt.Println("booting the server")
	client.Server.Boot(server.ID)

	// stop
	time.Sleep(3 * time.Second)
	fmt.Println("stopping the server")
	client.Server.Stop(server.ID)

	// wait for server to down
	err = client.Server.SleepUntilDown(server.ID, 120*time.Second)
	if err != nil {
		fmt.Println("failed")
		os.Exit(1)
	}

	// disconnect the disk from the server
	fmt.Println("disconnecting the disk")
	client.Disk.DisconnectFromServer(disk.ID)

	// delete the server
	fmt.Println("deleting the server")
	client.Server.Delete(server.ID)

	// delete the disk
	fmt.Println("deleting the disk")
	client.Disk.Delete(disk.ID)
}

```

## Download a disk image

**Pre requirements**
  * install ftps libs. please run `go get github.com/webguerilla/ftps`
  * create a disk named "GitLab"

```go

package main

import (
	"fmt"
	"github.com/webguerilla/ftps"
	API "github.com/sacloud/libsacloud/api"
	"os"
	"time"
)

func main() {

	// settings
	var (
		token   = os.Args[1]
		secret  = os.Args[2]
		zone    = os.Args[3]
		srcName = "GitLab"
	)

	// authorize
	api := API.NewClient(token, secret, zone)

	// search the source disk
	res, _ := api.Disk.
		WithNameLike(srcName).
		Limit(1).
		Find()
	if res.Count == 0 {
		panic("Disk `GitLab` not found")
	}

	disk := res.Disks[0]

	// copy the disk to a new archive
	fmt.Println("copying the disk to a new archive")

	archive := api.Archive.New()
	archive.Name = fmt.Sprintf("Copy:%s", disk.Name)
	archive.SetSourceDisk(disk.ID)
	archive, _ = api.Archive.Create(archive)
	api.Archive.SleepWhileCopying(archive.ID, 180*time.Second)

	// get FTP information
	ftp, _ := api.Archive.OpenFTP(archive.ID, false)
	fmt.Println("FTP information:")
	fmt.Println("  user: " + ftp.User)
	fmt.Println("  pass: " + ftp.Password)
	fmt.Println("  host: " + ftp.HostName)

	// download the archive via FTPS
	ftpsClient := &ftps.FTPS{}
	ftpsClient.TLSConfig.InsecureSkipVerify = true
	ftpsClient.Connect(ftp.HostName, 21)
	ftpsClient.Login(ftp.User, ftp.Password)
	err := ftpsClient.RetrieveFile("archive.img", "archive.img")
	if err != nil {
		panic(err)
	}
	ftpsClient.Quit()

	// delete the archive after download
	fmt.Println("deleting the archive")
	api.Archive.CloseFTP(archive.ID)
	api.Archive.Delete(archive.ID)

}

```

# License

  `libsacloud` Copyright (C) 2016 Kazumichi Yamamoto.

  This project is published under [Apache 2.0 License](LICENSE).

# Author

* Kazumichi Yamamoto ([@yamamoto-febc](https://github.com/yamamoto-febc))

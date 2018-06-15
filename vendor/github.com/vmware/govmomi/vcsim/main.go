/*
Copyright (c) 2017 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"crypto/tls"
	"expvar"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/google/uuid"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/simulator/esx"
)

func main() {
	model := simulator.VPX()

	flag.IntVar(&model.Datacenter, "dc", model.Datacenter, "Number of datacenters")
	flag.IntVar(&model.Cluster, "cluster", model.Cluster, "Number of clusters")
	flag.IntVar(&model.ClusterHost, "host", model.ClusterHost, "Number of hosts per cluster")
	flag.IntVar(&model.Host, "standalone-host", model.Host, "Number of standalone hosts")
	flag.IntVar(&model.Datastore, "ds", model.Datastore, "Number of local datastores")
	flag.IntVar(&model.Machine, "vm", model.Machine, "Number of virtual machines per resource pool")
	flag.IntVar(&model.Pool, "pool", model.Pool, "Number of resource pools per compute resource")
	flag.IntVar(&model.App, "app", model.App, "Number of virtual apps per compute resource")
	flag.IntVar(&model.Pod, "pod", model.Pod, "Number of storage pods per datacenter")
	flag.IntVar(&model.Portgroup, "pg", model.Portgroup, "Number of port groups")
	flag.IntVar(&model.Folder, "folder", model.Folder, "Number of folders")
	flag.BoolVar(&model.Autostart, "autostart", model.Autostart, "Autostart model created VMs")

	isESX := flag.Bool("esx", false, "Simulate standalone ESX")
	isTLS := flag.Bool("tls", true, "Enable TLS")
	cert := flag.String("tlscert", "", "Path to TLS certificate file")
	key := flag.String("tlskey", "", "Path to TLS key file")
	env := flag.String("E", "-", "Output vcsim variables to the given fifo or stdout")
	flag.BoolVar(&simulator.Trace, "trace", simulator.Trace, "Trace SOAP to stderr")

	flag.Parse()

	switch flag.Arg(0) {
	case "uuidgen": // util-linux not installed on Travis CI
		fmt.Println(uuid.New().String())
		return
	}

	var err error
	out := os.Stdout

	if *env != "-" {
		out, err = os.OpenFile(*env, os.O_WRONLY, 0)
		if err != nil {
			log.Fatal(err)
		}
	}

	f := flag.Lookup("httptest.serve")
	if f.Value.String() == "" {
		// #nosec: Errors unhandled
		_ = f.Value.Set("127.0.0.1:8989")
	}

	if *isESX {
		opts := model
		model = simulator.ESX()
		// Preserve options that also apply to ESX
		model.Datastore = opts.Datastore
		model.Machine = opts.Machine
		model.Autostart = opts.Autostart
	}

	tag := " (govmomi simulator)"
	model.ServiceContent.About.Name += tag
	model.ServiceContent.About.OsType = runtime.GOOS + "-" + runtime.GOARCH

	esx.HostSystem.Summary.Hardware.Vendor += tag

	err = model.Create()
	if err != nil {
		log.Fatal(err)
	}

	if *isTLS {
		model.Service.TLS = new(tls.Config)
		if *cert != "" {
			c, err := tls.LoadX509KeyPair(*cert, *key)
			if err != nil {
				log.Fatal(err)
			}

			model.Service.TLS.Certificates = []tls.Certificate{c}
		}
	}

	expvar.Publish("vcsim", expvar.Func(func() interface{} {
		count := model.Count()

		return struct {
			Registry *simulator.Registry
			Model    *simulator.Model
		}{
			simulator.Map,
			&count,
		}
	}))

	model.Service.ServeMux = http.DefaultServeMux // expvar.init registers "/debug/vars" with the DefaultServeMux

	s := model.Service.NewServer()

	fmt.Fprintf(out, "export GOVC_URL=%s GOVC_SIM_PID=%d\n", s.URL, os.Getpid())
	if out != os.Stdout {
		_ = out.Close()
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	<-sig

	model.Remove()
}

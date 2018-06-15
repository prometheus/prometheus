// Copyright 2016 VMware, Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/vmware/vmw-guestinfo/rpcvmx"
	"github.com/vmware/vmw-guestinfo/vmcheck"
)

var (
	set bool
	get bool
)

func init() {

	flag.BoolVar(&set, "set", false, "Sets the guestinfo.KEY with the string VALUE")
	flag.BoolVar(&get, "get", false, "Returns the config string in the guestinfo.* namespace")

	flag.Parse()
}

func main() {

	isVM, err := vmcheck.IsVirtualWorld()
	if err != nil {
		log.Fatalf("Error: %s", err)
	}

	if !isVM {
		log.Fatalf("ERROR: not in a virtual world.")
	}

	if !set && !get {
		flag.Usage()
	}

	config := rpcvmx.NewConfig()
	if set {
		if flag.NArg() != 2 {
			log.Fatalf("ERROR: Please provide guestinfo key / value pair (eg; -set foo bar")
		}
		if err := config.SetString(flag.Arg(0), flag.Arg(1)); err != nil {
			log.Fatalf("ERROR: SetString failed with %s", err)
		}
	}

	if get {
		if flag.NArg() != 1 {
			log.Fatalf("ERROR: Please provide guestinfo key (eg; -get foo)")
		}
		if out, err := config.String(flag.Arg(0), ""); err != nil {
			log.Fatalf("ERROR: String failed with %s", err)
		} else {
			fmt.Printf("%s\n", out)
		}
	}

}

// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strutil

import (
	"fmt"
	"net"
	"os"
	"strings"
)

var (
	buildinFunctionMap map[string]func(string) (string, bool)
)

func init() {
	buildinFunctionMap = make(map[string]func(string) (string, bool))
	buildinFunctionMap["ip"] = func(string) (string, bool) {
		addrs, err := net.InterfaceAddrs()
		if err != nil {
			fmt.Println(err)
			return "", false
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String(), true
				}
			}
		}
		return "", false
	}
}

func ReplacePlaceholders(input string) string {
	var output string
	for i := 0; i < len(input); i++ {
		if input[i] == '\\' && i+1 < len(input) && input[i+1] == '$' {
			output += "$"
			i++
			continue
		}
		if input[i] == '$' && i+2 < len(input) && input[i+1] == '{' {
			j := i + 2
			for ; j < len(input) && input[j] != '}'; j++ {
			}
			if j == len(input) {
				output += input[i:j]
				break
			}
			key := input[i+2 : j]
			if strings.HasPrefix(key, "os_") {
				value, ok := os.LookupEnv(key[3:])
				if ok {
					output += value
				} else {
					output += input[i : j+1]
				}
			} else if strings.HasPrefix(key, "buildin_") {
				if fun := buildinFunctionMap[key[8:]]; fun != nil {
					if realValue, ok := fun(key[8:]); ok {
						output += realValue
					} else {
						output += input[i : j+1]
					}
				} else {
					output += input[i : j+1]
				}
			} else {
				output += input[i : j+1]
			}
			i = j
		} else {
			output += string(input[i])
		}
	}
	return output
}

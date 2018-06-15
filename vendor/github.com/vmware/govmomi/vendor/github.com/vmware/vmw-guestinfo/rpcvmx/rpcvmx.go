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

package rpcvmx

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/vmware/vmw-guestinfo/rpcout"
)

const (
	prefix = "guestinfo"
)

// Config gives access to the vmx config through the VMware backdoor
type Config struct{}

// NewConfig creates a new Config object
func NewConfig() *Config {
	return &Config{}
}

// String returns the config string in the guestinfo.* namespace
func (c *Config) String(key string, defaultValue string) (string, error) {
	// add "guestinfo." prefix if missing
	if !strings.HasPrefix(key, prefix) {
		key = fmt.Sprintf("%s.%s", prefix, key)
	}

	out, ok, err := rpcout.SendOne("info-get %s", key)
	if err != nil {
		return "", err
	} else if !ok {
		return defaultValue, nil
	}
	return string(out), nil
}

// Bool returns the config boolean in the guestinfo.* namespace
func (c *Config) Bool(key string, defaultValue bool) (bool, error) {
	val, err := c.String(key, fmt.Sprintf("%t", defaultValue))
	if err != nil {
		return false, err
	}
	res, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue, nil
	}
	return res, nil
}

// Int returns the config integer in the guestinfo.* namespace
func (c *Config) Int(key string, defaultValue int) (int, error) {
	val, err := c.String(key, "")
	if err != nil {
		return 0, err
	}
	res, err := strconv.Atoi(val)
	if err != nil {
		return defaultValue, nil
	}
	return res, nil
}

// SetString sets the guestinfo.KEY with the string VALUE
func (c *Config) SetString(key string, value string) error {
	// add "guestinfo." prefix if missing
	if !strings.HasPrefix(key, prefix) {
		key = fmt.Sprintf("%s.%s", prefix, key)
	}

	_, _, err := rpcout.SendOne("info-set %s %s", key, value)
	if err != nil {
		return err
	}
	return nil
}

// SetString sets the guestinfo.KEY with the bool VALUE
func (c *Config) SetBool(key string, value bool) error {
	return c.SetString(key, strconv.FormatBool(value))
}

// SetString sets the guestinfo.KEY with the int VALUE
func (c *Config) SetInt(key string, value int) error {
	return c.SetString(key, strconv.Itoa(value))
}

/*
 *
 * Copyright 2020 gRPC authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

// Package serviceconfig contains utility functions to parse service config.
package serviceconfig

import (
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/balancer"
	"google.golang.org/grpc/grpclog"
	externalserviceconfig "google.golang.org/grpc/serviceconfig"
)

var logger = grpclog.Component("core")

// BalancerConfig wraps the name and config associated with one load balancing
// policy. It corresponds to a single entry of the loadBalancingConfig field
// from ServiceConfig.
//
// It implements the json.Unmarshaler interface.
//
// https://github.com/grpc/grpc-proto/blob/54713b1e8bc6ed2d4f25fb4dff527842150b91b2/grpc/service_config/service_config.proto#L247
type BalancerConfig struct {
	Name   string
	Config externalserviceconfig.LoadBalancingConfig
}

type intermediateBalancerConfig []map[string]json.RawMessage

// UnmarshalJSON implements the json.Unmarshaler interface.
//
// ServiceConfig contains a list of loadBalancingConfigs, each with a name and
// config. This method iterates through that list in order, and stops at the
// first policy that is supported.
// - If the config for the first supported policy is invalid, the whole service
//   config is invalid.
// - If the list doesn't contain any supported policy, the whole service config
//   is invalid.
func (bc *BalancerConfig) UnmarshalJSON(b []byte) error {
	var ir intermediateBalancerConfig
	err := json.Unmarshal(b, &ir)
	if err != nil {
		return err
	}

	for i, lbcfg := range ir {
		if len(lbcfg) != 1 {
			return fmt.Errorf("invalid loadBalancingConfig: entry %v does not contain exactly 1 policy/config pair: %q", i, lbcfg)
		}

		var (
			name    string
			jsonCfg json.RawMessage
		)
		// Get the key:value pair from the map. We have already made sure that
		// the map contains a single entry.
		for name, jsonCfg = range lbcfg {
		}

		builder := balancer.Get(name)
		if builder == nil {
			// If the balancer is not registered, move on to the next config.
			// This is not an error.
			continue
		}
		bc.Name = name

		parser, ok := builder.(balancer.ConfigParser)
		if !ok {
			if string(jsonCfg) != "{}" {
				logger.Warningf("non-empty balancer configuration %q, but balancer does not implement ParseConfig", string(jsonCfg))
			}
			// Stop at this, though the builder doesn't support parsing config.
			return nil
		}

		cfg, err := parser.ParseConfig(jsonCfg)
		if err != nil {
			return fmt.Errorf("error parsing loadBalancingConfig for policy %q: %v", name, err)
		}
		bc.Config = cfg
		return nil
	}
	// This is reached when the for loop iterates over all entries, but didn't
	// return. This means we had a loadBalancingConfig slice but did not
	// encounter a registered policy. The config is considered invalid in this
	// case.
	return fmt.Errorf("invalid loadBalancingConfig: no supported policies found")
}

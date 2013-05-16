// Copyright 2013 Prometheus Team
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

package config

import (
	"code.google.com/p/goprotobuf/proto"
	pb "github.com/prometheus/prometheus/config/generated"
	"io/ioutil"
)

func LoadFromString(configStr string) (Config, error) {
	configProto := pb.PrometheusConfig{}
	if err := proto.UnmarshalText(configStr, &configProto); err != nil {
		return Config{}, err
	}
	if configProto.Global == nil {
		configProto.Global = &pb.GlobalConfig{}
	}
	for _, job := range configProto.Job {
		if job.ScrapeInterval == nil {
			job.ScrapeInterval = proto.String(configProto.Global.GetScrapeInterval())
		}
	}

	config := Config{configProto}
	err := config.Validate()

	return config, err
}

func LoadFromFile(fileName string) (Config, error) {
	configStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return Config{}, err
	}

	return LoadFromString(string(configStr))
}

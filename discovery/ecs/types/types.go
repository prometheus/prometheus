// Copyright 2016 The Prometheus Authors
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

package types

// ServiceInstance represents an ECS service instance.
type ServiceInstance struct {
	// Addr is the address of the target instance.
	Addr string
	// cluster is the cluster name where the target instance resides.
	Cluster string
	// Service is the service that owns the target instance.
	Service string
	// Contaner is the container name of th target instance.
	Container string
	// Tags ar the EC2 instance tags of the target instance.
	Tags map[string]string
	// Labels are the Docker container instance labels of the target instance.
	Labels map[string]string
	// Image is the docker image that has been used to run the container.
	Image string
	// ContainerPort is the port exposed inside the container.
	ContainerPort string
	// ContainerPortProto is the port protocol exposed inside the container.
	ContainerPortProto string
}

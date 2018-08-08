/*
Copyright 2018 The Kubernetes Authors.

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

package v1

import (
	authenticationv1 "k8s.io/api/authentication/v1"
)

// The ServiceAccountExpansion interface allows manually adding extra methods
// to the ServiceAccountInterface.
type ServiceAccountExpansion interface {
	CreateToken(name string, tr *authenticationv1.TokenRequest) (*authenticationv1.TokenRequest, error)
}

// CreateToken creates a new token for a serviceaccount.
func (c *serviceAccounts) CreateToken(name string, tr *authenticationv1.TokenRequest) (*authenticationv1.TokenRequest, error) {
	result := &authenticationv1.TokenRequest{}
	err := c.client.Post().
		Namespace(c.ns).
		Resource("serviceaccounts").
		SubResource("token").
		Name(name).
		Body(tr).
		Do().
		Into(result)
	return result, err
}

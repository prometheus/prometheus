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

package clientauthentication

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ExecCredentials is used by exec-based plugins to communicate credentials to
// HTTP transports.
type ExecCredential struct {
	metav1.TypeMeta

	// Spec holds information passed to the plugin by the transport. This contains
	// request and runtime specific information, such as if the session is interactive.
	Spec ExecCredentialSpec

	// Status is filled in by the plugin and holds the credentials that the transport
	// should use to contact the API.
	// +optional
	Status *ExecCredentialStatus
}

// ExecCredenitalSpec holds request and runtime specific information provided by
// the transport.
type ExecCredentialSpec struct {
	// Response is populated when the transport encounters HTTP status codes, such as 401,
	// suggesting previous credentials were invalid.
	// +optional
	Response *Response

	// Interactive is true when the transport detects the command is being called from an
	// interactive prompt.
	// +optional
	Interactive bool
}

// ExecCredentialStatus holds credentials for the transport to use.
type ExecCredentialStatus struct {
	// ExpirationTimestamp indicates a time when the provided credentials expire.
	// +optional
	ExpirationTimestamp *metav1.Time
	// Token is a bearer token used by the client for request authentication.
	Token string
}

// Response defines metadata about a failed request, including HTTP status code and
// response headers.
type Response struct {
	// Headers holds HTTP headers returned by the server.
	Header map[string][]string
	// Code is the HTTP status code returned by the server.
	Code int32
}

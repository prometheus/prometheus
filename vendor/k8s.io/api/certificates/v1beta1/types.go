/*
Copyright 2016 The Kubernetes Authors.

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

package v1beta1

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:prerelease-lifecycle-gen:introduced=1.12
// +k8s:prerelease-lifecycle-gen:deprecated=1.19
// +k8s:prerelease-lifecycle-gen:replacement=certificates.k8s.io,v1,CertificateSigningRequest

// Describes a certificate signing request
type CertificateSigningRequest struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// The certificate request itself and any additional information.
	// +optional
	Spec CertificateSigningRequestSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Derived information about the request.
	// +optional
	Status CertificateSigningRequestStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// This information is immutable after the request is created. Only the Request
// and Usages fields can be set on creation, other fields are derived by
// Kubernetes and cannot be modified by users.
type CertificateSigningRequestSpec struct {
	// Base64-encoded PKCS#10 CSR data
	// +listType=atomic
	Request []byte `json:"request" protobuf:"bytes,1,opt,name=request"`

	// Requested signer for the request. It is a qualified name in the form:
	// `scope-hostname.io/name`.
	// If empty, it will be defaulted:
	//  1. If it's a kubelet client certificate, it is assigned
	//     "kubernetes.io/kube-apiserver-client-kubelet".
	//  2. If it's a kubelet serving certificate, it is assigned
	//     "kubernetes.io/kubelet-serving".
	//  3. Otherwise, it is assigned "kubernetes.io/legacy-unknown".
	// Distribution of trust for signers happens out of band.
	// You can select on this field using `spec.signerName`.
	// +optional
	SignerName *string `json:"signerName,omitempty" protobuf:"bytes,7,opt,name=signerName"`

	// allowedUsages specifies a set of usage contexts the key will be
	// valid for.
	// See: https://tools.ietf.org/html/rfc5280#section-4.2.1.3
	//      https://tools.ietf.org/html/rfc5280#section-4.2.1.12
	// Valid values are:
	//  "signing",
	//  "digital signature",
	//  "content commitment",
	//  "key encipherment",
	//  "key agreement",
	//  "data encipherment",
	//  "cert sign",
	//  "crl sign",
	//  "encipher only",
	//  "decipher only",
	//  "any",
	//  "server auth",
	//  "client auth",
	//  "code signing",
	//  "email protection",
	//  "s/mime",
	//  "ipsec end system",
	//  "ipsec tunnel",
	//  "ipsec user",
	//  "timestamping",
	//  "ocsp signing",
	//  "microsoft sgc",
	//  "netscape sgc"
	// +listType=atomic
	Usages []KeyUsage `json:"usages,omitempty" protobuf:"bytes,5,opt,name=usages"`

	// Information about the requesting user.
	// See user.Info interface for details.
	// +optional
	Username string `json:"username,omitempty" protobuf:"bytes,2,opt,name=username"`
	// UID information about the requesting user.
	// See user.Info interface for details.
	// +optional
	UID string `json:"uid,omitempty" protobuf:"bytes,3,opt,name=uid"`
	// Group information about the requesting user.
	// See user.Info interface for details.
	// +listType=atomic
	// +optional
	Groups []string `json:"groups,omitempty" protobuf:"bytes,4,rep,name=groups"`
	// Extra information about the requesting user.
	// See user.Info interface for details.
	// +optional
	Extra map[string]ExtraValue `json:"extra,omitempty" protobuf:"bytes,6,rep,name=extra"`
}

// Built in signerName values that are honoured by kube-controller-manager.
// None of these usages are related to ServiceAccount token secrets
// `.data[ca.crt]` in any way.
const (
	// Signs certificates that will be honored as client-certs by the
	// kube-apiserver. Never auto-approved by kube-controller-manager.
	KubeAPIServerClientSignerName = "kubernetes.io/kube-apiserver-client"

	// Signs client certificates that will be honored as client-certs by the
	// kube-apiserver for a kubelet.
	// May be auto-approved by kube-controller-manager.
	KubeAPIServerClientKubeletSignerName = "kubernetes.io/kube-apiserver-client-kubelet"

	// Signs serving certificates that are honored as a valid kubelet serving
	// certificate by the kube-apiserver, but has no other guarantees.
	KubeletServingSignerName = "kubernetes.io/kubelet-serving"

	// Has no guarantees for trust at all. Some distributions may honor these
	// as client certs, but that behavior is not standard kubernetes behavior.
	LegacyUnknownSignerName = "kubernetes.io/legacy-unknown"
)

// ExtraValue masks the value so protobuf can generate
// +protobuf.nullable=true
// +protobuf.options.(gogoproto.goproto_stringer)=false
type ExtraValue []string

func (t ExtraValue) String() string {
	return fmt.Sprintf("%v", []string(t))
}

type CertificateSigningRequestStatus struct {
	// Conditions applied to the request, such as approval or denial.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []CertificateSigningRequestCondition `json:"conditions,omitempty" protobuf:"bytes,1,rep,name=conditions"`

	// If request was approved, the controller will place the issued certificate here.
	// +listType=atomic
	// +optional
	Certificate []byte `json:"certificate,omitempty" protobuf:"bytes,2,opt,name=certificate"`
}

type RequestConditionType string

// These are the possible conditions for a certificate request.
const (
	CertificateApproved RequestConditionType = "Approved"
	CertificateDenied   RequestConditionType = "Denied"
	CertificateFailed   RequestConditionType = "Failed"
)

type CertificateSigningRequestCondition struct {
	// type of the condition. Known conditions include "Approved", "Denied", and "Failed".
	Type RequestConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=RequestConditionType"`
	// Status of the condition, one of True, False, Unknown.
	// Approved, Denied, and Failed conditions may not be "False" or "Unknown".
	// Defaults to "True".
	// If unset, should be treated as "True".
	// +optional
	Status v1.ConditionStatus `json:"status" protobuf:"bytes,6,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// brief reason for the request state
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,2,opt,name=reason"`
	// human readable message with details about the request state
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,3,opt,name=message"`
	// timestamp for the last update to this condition
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty" protobuf:"bytes,4,opt,name=lastUpdateTime"`
	// lastTransitionTime is the time the condition last transitioned from one status to another.
	// If unset, when a new condition type is added or an existing condition's status is changed,
	// the server defaults this to the current time.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,5,opt,name=lastTransitionTime"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:prerelease-lifecycle-gen:introduced=1.12
// +k8s:prerelease-lifecycle-gen:deprecated=1.19
// +k8s:prerelease-lifecycle-gen:replacement=certificates.k8s.io,v1,CertificateSigningRequestList

type CertificateSigningRequestList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []CertificateSigningRequest `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// KeyUsages specifies valid usage contexts for keys.
// See: https://tools.ietf.org/html/rfc5280#section-4.2.1.3
//      https://tools.ietf.org/html/rfc5280#section-4.2.1.12
type KeyUsage string

const (
	UsageSigning           KeyUsage = "signing"
	UsageDigitalSignature  KeyUsage = "digital signature"
	UsageContentCommitment KeyUsage = "content commitment"
	UsageKeyEncipherment   KeyUsage = "key encipherment"
	UsageKeyAgreement      KeyUsage = "key agreement"
	UsageDataEncipherment  KeyUsage = "data encipherment"
	UsageCertSign          KeyUsage = "cert sign"
	UsageCRLSign           KeyUsage = "crl sign"
	UsageEncipherOnly      KeyUsage = "encipher only"
	UsageDecipherOnly      KeyUsage = "decipher only"
	UsageAny               KeyUsage = "any"
	UsageServerAuth        KeyUsage = "server auth"
	UsageClientAuth        KeyUsage = "client auth"
	UsageCodeSigning       KeyUsage = "code signing"
	UsageEmailProtection   KeyUsage = "email protection"
	UsageSMIME             KeyUsage = "s/mime"
	UsageIPsecEndSystem    KeyUsage = "ipsec end system"
	UsageIPsecTunnel       KeyUsage = "ipsec tunnel"
	UsageIPsecUser         KeyUsage = "ipsec user"
	UsageTimestamping      KeyUsage = "timestamping"
	UsageOCSPSigning       KeyUsage = "ocsp signing"
	UsageMicrosoftSGC      KeyUsage = "microsoft sgc"
	UsageNetscapeSGC       KeyUsage = "netscape sgc"
)

/*
Copyright 2015 The Kubernetes Authors.

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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// NamespaceDefault means the object is in the default namespace which is applied when not specified by clients
	NamespaceDefault string = "default"
	// NamespaceAll is the default argument to specify on a context when you want to list or filter resources across all namespaces
	NamespaceAll string = ""
	// NamespaceNodeLease is the namespace where we place node lease objects (used for node heartbeats)
	NamespaceNodeLease string = "kube-node-lease"
	// TopologyKeyAny is the service topology key that matches any node
	TopologyKeyAny string = "*"
)

// Volume represents a named volume in a pod that may be accessed by any container in the pod.
type Volume struct {
	// Volume's name.
	// Must be a DNS_LABEL and unique within the pod.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// VolumeSource represents the location and type of the mounted volume.
	// If not specified, the Volume is implied to be an EmptyDir.
	// This implied behavior is deprecated and will be removed in a future version.
	VolumeSource `json:",inline" protobuf:"bytes,2,opt,name=volumeSource"`
}

// Represents the source of a volume to mount.
// Only one of its members may be specified.
type VolumeSource struct {
	// HostPath represents a pre-existing file or directory on the host
	// machine that is directly exposed to the container. This is generally
	// used for system agents or other privileged things that are allowed
	// to see the host machine. Most containers will NOT need this.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath
	// ---
	// TODO(jonesdl) We need to restrict who can use host directory mounts and who can/can not
	// mount host directories as read/write.
	// +optional
	HostPath *HostPathVolumeSource `json:"hostPath,omitempty" protobuf:"bytes,1,opt,name=hostPath"`
	// EmptyDir represents a temporary directory that shares a pod's lifetime.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir
	// +optional
	EmptyDir *EmptyDirVolumeSource `json:"emptyDir,omitempty" protobuf:"bytes,2,opt,name=emptyDir"`
	// GCEPersistentDisk represents a GCE Disk resource that is attached to a
	// kubelet's host machine and then exposed to the pod.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
	// +optional
	GCEPersistentDisk *GCEPersistentDiskVolumeSource `json:"gcePersistentDisk,omitempty" protobuf:"bytes,3,opt,name=gcePersistentDisk"`
	// AWSElasticBlockStore represents an AWS Disk resource that is attached to a
	// kubelet's host machine and then exposed to the pod.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore
	// +optional
	AWSElasticBlockStore *AWSElasticBlockStoreVolumeSource `json:"awsElasticBlockStore,omitempty" protobuf:"bytes,4,opt,name=awsElasticBlockStore"`
	// GitRepo represents a git repository at a particular revision.
	// DEPRECATED: GitRepo is deprecated. To provision a container with a git repo, mount an
	// EmptyDir into an InitContainer that clones the repo using git, then mount the EmptyDir
	// into the Pod's container.
	// +optional
	GitRepo *GitRepoVolumeSource `json:"gitRepo,omitempty" protobuf:"bytes,5,opt,name=gitRepo"`
	// Secret represents a secret that should populate this volume.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#secret
	// +optional
	Secret *SecretVolumeSource `json:"secret,omitempty" protobuf:"bytes,6,opt,name=secret"`
	// NFS represents an NFS mount on the host that shares a pod's lifetime
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs
	// +optional
	NFS *NFSVolumeSource `json:"nfs,omitempty" protobuf:"bytes,7,opt,name=nfs"`
	// ISCSI represents an ISCSI Disk resource that is attached to a
	// kubelet's host machine and then exposed to the pod.
	// More info: https://examples.k8s.io/volumes/iscsi/README.md
	// +optional
	ISCSI *ISCSIVolumeSource `json:"iscsi,omitempty" protobuf:"bytes,8,opt,name=iscsi"`
	// Glusterfs represents a Glusterfs mount on the host that shares a pod's lifetime.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md
	// +optional
	Glusterfs *GlusterfsVolumeSource `json:"glusterfs,omitempty" protobuf:"bytes,9,opt,name=glusterfs"`
	// PersistentVolumeClaimVolumeSource represents a reference to a
	// PersistentVolumeClaim in the same namespace.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	PersistentVolumeClaim *PersistentVolumeClaimVolumeSource `json:"persistentVolumeClaim,omitempty" protobuf:"bytes,10,opt,name=persistentVolumeClaim"`
	// RBD represents a Rados Block Device mount on the host that shares a pod's lifetime.
	// More info: https://examples.k8s.io/volumes/rbd/README.md
	// +optional
	RBD *RBDVolumeSource `json:"rbd,omitempty" protobuf:"bytes,11,opt,name=rbd"`
	// FlexVolume represents a generic volume resource that is
	// provisioned/attached using an exec based plugin.
	// +optional
	FlexVolume *FlexVolumeSource `json:"flexVolume,omitempty" protobuf:"bytes,12,opt,name=flexVolume"`
	// Cinder represents a cinder volume attached and mounted on kubelets host machine.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	// +optional
	Cinder *CinderVolumeSource `json:"cinder,omitempty" protobuf:"bytes,13,opt,name=cinder"`
	// CephFS represents a Ceph FS mount on the host that shares a pod's lifetime
	// +optional
	CephFS *CephFSVolumeSource `json:"cephfs,omitempty" protobuf:"bytes,14,opt,name=cephfs"`
	// Flocker represents a Flocker volume attached to a kubelet's host machine. This depends on the Flocker control service being running
	// +optional
	Flocker *FlockerVolumeSource `json:"flocker,omitempty" protobuf:"bytes,15,opt,name=flocker"`
	// DownwardAPI represents downward API about the pod that should populate this volume
	// +optional
	DownwardAPI *DownwardAPIVolumeSource `json:"downwardAPI,omitempty" protobuf:"bytes,16,opt,name=downwardAPI"`
	// FC represents a Fibre Channel resource that is attached to a kubelet's host machine and then exposed to the pod.
	// +optional
	FC *FCVolumeSource `json:"fc,omitempty" protobuf:"bytes,17,opt,name=fc"`
	// AzureFile represents an Azure File Service mount on the host and bind mount to the pod.
	// +optional
	AzureFile *AzureFileVolumeSource `json:"azureFile,omitempty" protobuf:"bytes,18,opt,name=azureFile"`
	// ConfigMap represents a configMap that should populate this volume
	// +optional
	ConfigMap *ConfigMapVolumeSource `json:"configMap,omitempty" protobuf:"bytes,19,opt,name=configMap"`
	// VsphereVolume represents a vSphere volume attached and mounted on kubelets host machine
	// +optional
	VsphereVolume *VsphereVirtualDiskVolumeSource `json:"vsphereVolume,omitempty" protobuf:"bytes,20,opt,name=vsphereVolume"`
	// Quobyte represents a Quobyte mount on the host that shares a pod's lifetime
	// +optional
	Quobyte *QuobyteVolumeSource `json:"quobyte,omitempty" protobuf:"bytes,21,opt,name=quobyte"`
	// AzureDisk represents an Azure Data Disk mount on the host and bind mount to the pod.
	// +optional
	AzureDisk *AzureDiskVolumeSource `json:"azureDisk,omitempty" protobuf:"bytes,22,opt,name=azureDisk"`
	// PhotonPersistentDisk represents a PhotonController persistent disk attached and mounted on kubelets host machine
	PhotonPersistentDisk *PhotonPersistentDiskVolumeSource `json:"photonPersistentDisk,omitempty" protobuf:"bytes,23,opt,name=photonPersistentDisk"`
	// Items for all in one resources secrets, configmaps, and downward API
	Projected *ProjectedVolumeSource `json:"projected,omitempty" protobuf:"bytes,26,opt,name=projected"`
	// PortworxVolume represents a portworx volume attached and mounted on kubelets host machine
	// +optional
	PortworxVolume *PortworxVolumeSource `json:"portworxVolume,omitempty" protobuf:"bytes,24,opt,name=portworxVolume"`
	// ScaleIO represents a ScaleIO persistent volume attached and mounted on Kubernetes nodes.
	// +optional
	ScaleIO *ScaleIOVolumeSource `json:"scaleIO,omitempty" protobuf:"bytes,25,opt,name=scaleIO"`
	// StorageOS represents a StorageOS volume attached and mounted on Kubernetes nodes.
	// +optional
	StorageOS *StorageOSVolumeSource `json:"storageos,omitempty" protobuf:"bytes,27,opt,name=storageos"`
	// CSI (Container Storage Interface) represents storage that is handled by an external CSI driver (Alpha feature).
	// +optional
	CSI *CSIVolumeSource `json:"csi,omitempty" protobuf:"bytes,28,opt,name=csi"`
}

// PersistentVolumeClaimVolumeSource references the user's PVC in the same namespace.
// This volume finds the bound PV and mounts that volume for the pod. A
// PersistentVolumeClaimVolumeSource is, essentially, a wrapper around another
// type of volume that is owned by someone else (the system).
type PersistentVolumeClaimVolumeSource struct {
	// ClaimName is the name of a PersistentVolumeClaim in the same namespace as the pod using this volume.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	ClaimName string `json:"claimName" protobuf:"bytes,1,opt,name=claimName"`
	// Will force the ReadOnly setting in VolumeMounts.
	// Default false.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,2,opt,name=readOnly"`
}

// PersistentVolumeSource is similar to VolumeSource but meant for the
// administrator who creates PVs. Exactly one of its members must be set.
type PersistentVolumeSource struct {
	// GCEPersistentDisk represents a GCE Disk resource that is attached to a
	// kubelet's host machine and then exposed to the pod. Provisioned by an admin.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
	// +optional
	GCEPersistentDisk *GCEPersistentDiskVolumeSource `json:"gcePersistentDisk,omitempty" protobuf:"bytes,1,opt,name=gcePersistentDisk"`
	// AWSElasticBlockStore represents an AWS Disk resource that is attached to a
	// kubelet's host machine and then exposed to the pod.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore
	// +optional
	AWSElasticBlockStore *AWSElasticBlockStoreVolumeSource `json:"awsElasticBlockStore,omitempty" protobuf:"bytes,2,opt,name=awsElasticBlockStore"`
	// HostPath represents a directory on the host.
	// Provisioned by a developer or tester.
	// This is useful for single-node development and testing only!
	// On-host storage is not supported in any way and WILL NOT WORK in a multi-node cluster.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath
	// +optional
	HostPath *HostPathVolumeSource `json:"hostPath,omitempty" protobuf:"bytes,3,opt,name=hostPath"`
	// Glusterfs represents a Glusterfs volume that is attached to a host and
	// exposed to the pod. Provisioned by an admin.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md
	// +optional
	Glusterfs *GlusterfsPersistentVolumeSource `json:"glusterfs,omitempty" protobuf:"bytes,4,opt,name=glusterfs"`
	// NFS represents an NFS mount on the host. Provisioned by an admin.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs
	// +optional
	NFS *NFSVolumeSource `json:"nfs,omitempty" protobuf:"bytes,5,opt,name=nfs"`
	// RBD represents a Rados Block Device mount on the host that shares a pod's lifetime.
	// More info: https://examples.k8s.io/volumes/rbd/README.md
	// +optional
	RBD *RBDPersistentVolumeSource `json:"rbd,omitempty" protobuf:"bytes,6,opt,name=rbd"`
	// ISCSI represents an ISCSI Disk resource that is attached to a
	// kubelet's host machine and then exposed to the pod. Provisioned by an admin.
	// +optional
	ISCSI *ISCSIPersistentVolumeSource `json:"iscsi,omitempty" protobuf:"bytes,7,opt,name=iscsi"`
	// Cinder represents a cinder volume attached and mounted on kubelets host machine.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	// +optional
	Cinder *CinderPersistentVolumeSource `json:"cinder,omitempty" protobuf:"bytes,8,opt,name=cinder"`
	// CephFS represents a Ceph FS mount on the host that shares a pod's lifetime
	// +optional
	CephFS *CephFSPersistentVolumeSource `json:"cephfs,omitempty" protobuf:"bytes,9,opt,name=cephfs"`
	// FC represents a Fibre Channel resource that is attached to a kubelet's host machine and then exposed to the pod.
	// +optional
	FC *FCVolumeSource `json:"fc,omitempty" protobuf:"bytes,10,opt,name=fc"`
	// Flocker represents a Flocker volume attached to a kubelet's host machine and exposed to the pod for its usage. This depends on the Flocker control service being running
	// +optional
	Flocker *FlockerVolumeSource `json:"flocker,omitempty" protobuf:"bytes,11,opt,name=flocker"`
	// FlexVolume represents a generic volume resource that is
	// provisioned/attached using an exec based plugin.
	// +optional
	FlexVolume *FlexPersistentVolumeSource `json:"flexVolume,omitempty" protobuf:"bytes,12,opt,name=flexVolume"`
	// AzureFile represents an Azure File Service mount on the host and bind mount to the pod.
	// +optional
	AzureFile *AzureFilePersistentVolumeSource `json:"azureFile,omitempty" protobuf:"bytes,13,opt,name=azureFile"`
	// VsphereVolume represents a vSphere volume attached and mounted on kubelets host machine
	// +optional
	VsphereVolume *VsphereVirtualDiskVolumeSource `json:"vsphereVolume,omitempty" protobuf:"bytes,14,opt,name=vsphereVolume"`
	// Quobyte represents a Quobyte mount on the host that shares a pod's lifetime
	// +optional
	Quobyte *QuobyteVolumeSource `json:"quobyte,omitempty" protobuf:"bytes,15,opt,name=quobyte"`
	// AzureDisk represents an Azure Data Disk mount on the host and bind mount to the pod.
	// +optional
	AzureDisk *AzureDiskVolumeSource `json:"azureDisk,omitempty" protobuf:"bytes,16,opt,name=azureDisk"`
	// PhotonPersistentDisk represents a PhotonController persistent disk attached and mounted on kubelets host machine
	PhotonPersistentDisk *PhotonPersistentDiskVolumeSource `json:"photonPersistentDisk,omitempty" protobuf:"bytes,17,opt,name=photonPersistentDisk"`
	// PortworxVolume represents a portworx volume attached and mounted on kubelets host machine
	// +optional
	PortworxVolume *PortworxVolumeSource `json:"portworxVolume,omitempty" protobuf:"bytes,18,opt,name=portworxVolume"`
	// ScaleIO represents a ScaleIO persistent volume attached and mounted on Kubernetes nodes.
	// +optional
	ScaleIO *ScaleIOPersistentVolumeSource `json:"scaleIO,omitempty" protobuf:"bytes,19,opt,name=scaleIO"`
	// Local represents directly-attached storage with node affinity
	// +optional
	Local *LocalVolumeSource `json:"local,omitempty" protobuf:"bytes,20,opt,name=local"`
	// StorageOS represents a StorageOS volume that is attached to the kubelet's host machine and mounted into the pod
	// More info: https://examples.k8s.io/volumes/storageos/README.md
	// +optional
	StorageOS *StorageOSPersistentVolumeSource `json:"storageos,omitempty" protobuf:"bytes,21,opt,name=storageos"`
	// CSI represents storage that is handled by an external CSI driver (Beta feature).
	// +optional
	CSI *CSIPersistentVolumeSource `json:"csi,omitempty" protobuf:"bytes,22,opt,name=csi"`
}

const (
	// BetaStorageClassAnnotation represents the beta/previous StorageClass annotation.
	// It's currently still used and will be held for backwards compatibility
	BetaStorageClassAnnotation = "volume.beta.kubernetes.io/storage-class"

	// MountOptionAnnotation defines mount option annotation used in PVs
	MountOptionAnnotation = "volume.beta.kubernetes.io/mount-options"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PersistentVolume (PV) is a storage resource provisioned by an administrator.
// It is analogous to a node.
// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes
type PersistentVolume struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines a specification of a persistent volume owned by the cluster.
	// Provisioned by an administrator.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistent-volumes
	// +optional
	Spec PersistentVolumeSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status represents the current information/status for the persistent volume.
	// Populated by the system.
	// Read-only.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistent-volumes
	// +optional
	Status PersistentVolumeStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// PersistentVolumeSpec is the specification of a persistent volume.
type PersistentVolumeSpec struct {
	// A description of the persistent volume's resources and capacity.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#capacity
	// +optional
	Capacity ResourceList `json:"capacity,omitempty" protobuf:"bytes,1,rep,name=capacity,casttype=ResourceList,castkey=ResourceName"`
	// The actual volume backing the persistent volume.
	PersistentVolumeSource `json:",inline" protobuf:"bytes,2,opt,name=persistentVolumeSource"`
	// AccessModes contains all ways the volume can be mounted.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#access-modes
	// +optional
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty" protobuf:"bytes,3,rep,name=accessModes,casttype=PersistentVolumeAccessMode"`
	// ClaimRef is part of a bi-directional binding between PersistentVolume and PersistentVolumeClaim.
	// Expected to be non-nil when bound.
	// claim.VolumeName is the authoritative bind between PV and PVC.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#binding
	// +optional
	ClaimRef *ObjectReference `json:"claimRef,omitempty" protobuf:"bytes,4,opt,name=claimRef"`
	// What happens to a persistent volume when released from its claim.
	// Valid options are Retain (default for manually created PersistentVolumes), Delete (default
	// for dynamically provisioned PersistentVolumes), and Recycle (deprecated).
	// Recycle must be supported by the volume plugin underlying this PersistentVolume.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#reclaiming
	// +optional
	PersistentVolumeReclaimPolicy PersistentVolumeReclaimPolicy `json:"persistentVolumeReclaimPolicy,omitempty" protobuf:"bytes,5,opt,name=persistentVolumeReclaimPolicy,casttype=PersistentVolumeReclaimPolicy"`
	// Name of StorageClass to which this persistent volume belongs. Empty value
	// means that this volume does not belong to any StorageClass.
	// +optional
	StorageClassName string `json:"storageClassName,omitempty" protobuf:"bytes,6,opt,name=storageClassName"`
	// A list of mount options, e.g. ["ro", "soft"]. Not validated - mount will
	// simply fail if one is invalid.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes/#mount-options
	// +optional
	MountOptions []string `json:"mountOptions,omitempty" protobuf:"bytes,7,opt,name=mountOptions"`
	// volumeMode defines if a volume is intended to be used with a formatted filesystem
	// or to remain in raw block state. Value of Filesystem is implied when not included in spec.
	// +optional
	VolumeMode *PersistentVolumeMode `json:"volumeMode,omitempty" protobuf:"bytes,8,opt,name=volumeMode,casttype=PersistentVolumeMode"`
	// NodeAffinity defines constraints that limit what nodes this volume can be accessed from.
	// This field influences the scheduling of pods that use this volume.
	// +optional
	NodeAffinity *VolumeNodeAffinity `json:"nodeAffinity,omitempty" protobuf:"bytes,9,opt,name=nodeAffinity"`
}

// VolumeNodeAffinity defines constraints that limit what nodes this volume can be accessed from.
type VolumeNodeAffinity struct {
	// Required specifies hard node constraints that must be met.
	Required *NodeSelector `json:"required,omitempty" protobuf:"bytes,1,opt,name=required"`
}

// PersistentVolumeReclaimPolicy describes a policy for end-of-life maintenance of persistent volumes.
type PersistentVolumeReclaimPolicy string

const (
	// PersistentVolumeReclaimRecycle means the volume will be recycled back into the pool of unbound persistent volumes on release from its claim.
	// The volume plugin must support Recycling.
	PersistentVolumeReclaimRecycle PersistentVolumeReclaimPolicy = "Recycle"
	// PersistentVolumeReclaimDelete means the volume will be deleted from Kubernetes on release from its claim.
	// The volume plugin must support Deletion.
	PersistentVolumeReclaimDelete PersistentVolumeReclaimPolicy = "Delete"
	// PersistentVolumeReclaimRetain means the volume will be left in its current phase (Released) for manual reclamation by the administrator.
	// The default policy is Retain.
	PersistentVolumeReclaimRetain PersistentVolumeReclaimPolicy = "Retain"
)

// PersistentVolumeMode describes how a volume is intended to be consumed, either Block or Filesystem.
type PersistentVolumeMode string

const (
	// PersistentVolumeBlock means the volume will not be formatted with a filesystem and will remain a raw block device.
	PersistentVolumeBlock PersistentVolumeMode = "Block"
	// PersistentVolumeFilesystem means the volume will be or is formatted with a filesystem.
	PersistentVolumeFilesystem PersistentVolumeMode = "Filesystem"
)

// PersistentVolumeStatus is the current status of a persistent volume.
type PersistentVolumeStatus struct {
	// Phase indicates if a volume is available, bound to a claim, or released by a claim.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#phase
	// +optional
	Phase PersistentVolumePhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=PersistentVolumePhase"`
	// A human-readable message indicating details about why the volume is in this state.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
	// Reason is a brief CamelCase string that describes any failure and is meant
	// for machine parsing and tidy display in the CLI.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PersistentVolumeList is a list of PersistentVolume items.
type PersistentVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// List of persistent volumes.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes
	Items []PersistentVolume `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PersistentVolumeClaim is a user's request for and claim to a persistent volume
type PersistentVolumeClaim struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the desired characteristics of a volume requested by a pod author.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	Spec PersistentVolumeClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status represents the current information/status of a persistent volume claim.
	// Read-only.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	// +optional
	Status PersistentVolumeClaimStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PersistentVolumeClaimList is a list of PersistentVolumeClaim items.
type PersistentVolumeClaimList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// A list of persistent volume claims.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims
	Items []PersistentVolumeClaim `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// PersistentVolumeClaimSpec describes the common attributes of storage devices
// and allows a Source for provider-specific attributes
type PersistentVolumeClaimSpec struct {
	// AccessModes contains the desired access modes the volume should have.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#access-modes-1
	// +optional
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty" protobuf:"bytes,1,rep,name=accessModes,casttype=PersistentVolumeAccessMode"`
	// A label query over volumes to consider for binding.
	// +optional
	Selector *metav1.LabelSelector `json:"selector,omitempty" protobuf:"bytes,4,opt,name=selector"`
	// Resources represents the minimum resources the volume should have.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#resources
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,2,opt,name=resources"`
	// VolumeName is the binding reference to the PersistentVolume backing this claim.
	// +optional
	VolumeName string `json:"volumeName,omitempty" protobuf:"bytes,3,opt,name=volumeName"`
	// Name of the StorageClass required by the claim.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty" protobuf:"bytes,5,opt,name=storageClassName"`
	// volumeMode defines what type of volume is required by the claim.
	// Value of Filesystem is implied when not included in claim spec.
	// +optional
	VolumeMode *PersistentVolumeMode `json:"volumeMode,omitempty" protobuf:"bytes,6,opt,name=volumeMode,casttype=PersistentVolumeMode"`
	// This field can be used to specify either:
	// * An existing VolumeSnapshot object (snapshot.storage.k8s.io/VolumeSnapshot - Beta)
	// * An existing PVC (PersistentVolumeClaim)
	// * An existing custom resource/object that implements data population (Alpha)
	// In order to use VolumeSnapshot object types, the appropriate feature gate
	// must be enabled (VolumeSnapshotDataSource or AnyVolumeDataSource)
	// If the provisioner or an external controller can support the specified data source,
	// it will create a new volume based on the contents of the specified data source.
	// If the specified data source is not supported, the volume will
	// not be created and the failure will be reported as an event.
	// In the future, we plan to support more data source types and the behavior
	// of the provisioner may change.
	// +optional
	DataSource *TypedLocalObjectReference `json:"dataSource,omitempty" protobuf:"bytes,7,opt,name=dataSource"`
}

// PersistentVolumeClaimConditionType is a valid value of PersistentVolumeClaimCondition.Type
type PersistentVolumeClaimConditionType string

const (
	// PersistentVolumeClaimResizing - a user trigger resize of pvc has been started
	PersistentVolumeClaimResizing PersistentVolumeClaimConditionType = "Resizing"
	// PersistentVolumeClaimFileSystemResizePending - controller resize is finished and a file system resize is pending on node
	PersistentVolumeClaimFileSystemResizePending PersistentVolumeClaimConditionType = "FileSystemResizePending"
)

// PersistentVolumeClaimCondition contails details about state of pvc
type PersistentVolumeClaimCondition struct {
	Type   PersistentVolumeClaimConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=PersistentVolumeClaimConditionType"`
	Status ConditionStatus                    `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// Last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty" protobuf:"bytes,3,opt,name=lastProbeTime"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// Unique, this should be a short, machine understandable string that gives the reason
	// for condition's last transition. If it reports "ResizeStarted" that means the underlying
	// persistent volume is being resized.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
}

// PersistentVolumeClaimStatus is the current status of a persistent volume claim.
type PersistentVolumeClaimStatus struct {
	// Phase represents the current phase of PersistentVolumeClaim.
	// +optional
	Phase PersistentVolumeClaimPhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=PersistentVolumeClaimPhase"`
	// AccessModes contains the actual access modes the volume backing the PVC has.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#access-modes-1
	// +optional
	AccessModes []PersistentVolumeAccessMode `json:"accessModes,omitempty" protobuf:"bytes,2,rep,name=accessModes,casttype=PersistentVolumeAccessMode"`
	// Represents the actual resources of the underlying volume.
	// +optional
	Capacity ResourceList `json:"capacity,omitempty" protobuf:"bytes,3,rep,name=capacity,casttype=ResourceList,castkey=ResourceName"`
	// Current Condition of persistent volume claim. If underlying persistent volume is being
	// resized then the Condition will be set to 'ResizeStarted'.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []PersistentVolumeClaimCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,4,rep,name=conditions"`
}

type PersistentVolumeAccessMode string

const (
	// can be mounted in read/write mode to exactly 1 host
	ReadWriteOnce PersistentVolumeAccessMode = "ReadWriteOnce"
	// can be mounted in read-only mode to many hosts
	ReadOnlyMany PersistentVolumeAccessMode = "ReadOnlyMany"
	// can be mounted in read/write mode to many hosts
	ReadWriteMany PersistentVolumeAccessMode = "ReadWriteMany"
)

type PersistentVolumePhase string

const (
	// used for PersistentVolumes that are not available
	VolumePending PersistentVolumePhase = "Pending"
	// used for PersistentVolumes that are not yet bound
	// Available volumes are held by the binder and matched to PersistentVolumeClaims
	VolumeAvailable PersistentVolumePhase = "Available"
	// used for PersistentVolumes that are bound
	VolumeBound PersistentVolumePhase = "Bound"
	// used for PersistentVolumes where the bound PersistentVolumeClaim was deleted
	// released volumes must be recycled before becoming available again
	// this phase is used by the persistent volume claim binder to signal to another process to reclaim the resource
	VolumeReleased PersistentVolumePhase = "Released"
	// used for PersistentVolumes that failed to be correctly recycled or deleted after being released from a claim
	VolumeFailed PersistentVolumePhase = "Failed"
)

type PersistentVolumeClaimPhase string

const (
	// used for PersistentVolumeClaims that are not yet bound
	ClaimPending PersistentVolumeClaimPhase = "Pending"
	// used for PersistentVolumeClaims that are bound
	ClaimBound PersistentVolumeClaimPhase = "Bound"
	// used for PersistentVolumeClaims that lost their underlying
	// PersistentVolume. The claim was bound to a PersistentVolume and this
	// volume does not exist any longer and all data on it was lost.
	ClaimLost PersistentVolumeClaimPhase = "Lost"
)

type HostPathType string

const (
	// For backwards compatible, leave it empty if unset
	HostPathUnset HostPathType = ""
	// If nothing exists at the given path, an empty directory will be created there
	// as needed with file mode 0755, having the same group and ownership with Kubelet.
	HostPathDirectoryOrCreate HostPathType = "DirectoryOrCreate"
	// A directory must exist at the given path
	HostPathDirectory HostPathType = "Directory"
	// If nothing exists at the given path, an empty file will be created there
	// as needed with file mode 0644, having the same group and ownership with Kubelet.
	HostPathFileOrCreate HostPathType = "FileOrCreate"
	// A file must exist at the given path
	HostPathFile HostPathType = "File"
	// A UNIX socket must exist at the given path
	HostPathSocket HostPathType = "Socket"
	// A character device must exist at the given path
	HostPathCharDev HostPathType = "CharDevice"
	// A block device must exist at the given path
	HostPathBlockDev HostPathType = "BlockDevice"
)

// Represents a host path mapped into a pod.
// Host path volumes do not support ownership management or SELinux relabeling.
type HostPathVolumeSource struct {
	// Path of the directory on the host.
	// If the path is a symlink, it will follow the link to the real path.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath
	Path string `json:"path" protobuf:"bytes,1,opt,name=path"`
	// Type for HostPath Volume
	// Defaults to ""
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath
	// +optional
	Type *HostPathType `json:"type,omitempty" protobuf:"bytes,2,opt,name=type"`
}

// Represents an empty directory for a pod.
// Empty directory volumes support ownership management and SELinux relabeling.
type EmptyDirVolumeSource struct {
	// What type of storage medium should back this directory.
	// The default is "" which means to use the node's default medium.
	// Must be an empty string (default) or Memory.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir
	// +optional
	Medium StorageMedium `json:"medium,omitempty" protobuf:"bytes,1,opt,name=medium,casttype=StorageMedium"`
	// Total amount of local storage required for this EmptyDir volume.
	// The size limit is also applicable for memory medium.
	// The maximum usage on memory medium EmptyDir would be the minimum value between
	// the SizeLimit specified here and the sum of memory limits of all containers in a pod.
	// The default is nil which means that the limit is undefined.
	// More info: http://kubernetes.io/docs/user-guide/volumes#emptydir
	// +optional
	SizeLimit *resource.Quantity `json:"sizeLimit,omitempty" protobuf:"bytes,2,opt,name=sizeLimit"`
}

// Represents a Glusterfs mount that lasts the lifetime of a pod.
// Glusterfs volumes do not support ownership management or SELinux relabeling.
type GlusterfsVolumeSource struct {
	// EndpointsName is the endpoint name that details Glusterfs topology.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	EndpointsName string `json:"endpoints" protobuf:"bytes,1,opt,name=endpoints"`

	// Path is the Glusterfs volume path.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	Path string `json:"path" protobuf:"bytes,2,opt,name=path"`

	// ReadOnly here will force the Glusterfs volume to be mounted with read-only permissions.
	// Defaults to false.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
}

// Represents a Glusterfs mount that lasts the lifetime of a pod.
// Glusterfs volumes do not support ownership management or SELinux relabeling.
type GlusterfsPersistentVolumeSource struct {
	// EndpointsName is the endpoint name that details Glusterfs topology.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	EndpointsName string `json:"endpoints" protobuf:"bytes,1,opt,name=endpoints"`

	// Path is the Glusterfs volume path.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	Path string `json:"path" protobuf:"bytes,2,opt,name=path"`

	// ReadOnly here will force the Glusterfs volume to be mounted with read-only permissions.
	// Defaults to false.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`

	// EndpointsNamespace is the namespace that contains Glusterfs endpoint.
	// If this field is empty, the EndpointNamespace defaults to the same namespace as the bound PVC.
	// More info: https://examples.k8s.io/volumes/glusterfs/README.md#create-a-pod
	// +optional
	EndpointsNamespace *string `json:"endpointsNamespace,omitempty" protobuf:"bytes,4,opt,name=endpointsNamespace"`
}

// Represents a Rados Block Device mount that lasts the lifetime of a pod.
// RBD volumes support ownership management and SELinux relabeling.
type RBDVolumeSource struct {
	// A collection of Ceph monitors.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	CephMonitors []string `json:"monitors" protobuf:"bytes,1,rep,name=monitors"`
	// The rados image name.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	RBDImage string `json:"image" protobuf:"bytes,2,opt,name=image"`
	// Filesystem type of the volume that you want to mount.
	// Tip: Ensure that the filesystem type is supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#rbd
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,3,opt,name=fsType"`
	// The rados pool name.
	// Default is rbd.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	RBDPool string `json:"pool,omitempty" protobuf:"bytes,4,opt,name=pool"`
	// The rados user name.
	// Default is admin.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	RadosUser string `json:"user,omitempty" protobuf:"bytes,5,opt,name=user"`
	// Keyring is the path to key ring for RBDUser.
	// Default is /etc/ceph/keyring.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	Keyring string `json:"keyring,omitempty" protobuf:"bytes,6,opt,name=keyring"`
	// SecretRef is name of the authentication secret for RBDUser. If provided
	// overrides keyring.
	// Default is nil.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty" protobuf:"bytes,7,opt,name=secretRef"`
	// ReadOnly here will force the ReadOnly setting in VolumeMounts.
	// Defaults to false.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,8,opt,name=readOnly"`
}

// Represents a Rados Block Device mount that lasts the lifetime of a pod.
// RBD volumes support ownership management and SELinux relabeling.
type RBDPersistentVolumeSource struct {
	// A collection of Ceph monitors.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	CephMonitors []string `json:"monitors" protobuf:"bytes,1,rep,name=monitors"`
	// The rados image name.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	RBDImage string `json:"image" protobuf:"bytes,2,opt,name=image"`
	// Filesystem type of the volume that you want to mount.
	// Tip: Ensure that the filesystem type is supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#rbd
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,3,opt,name=fsType"`
	// The rados pool name.
	// Default is rbd.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	RBDPool string `json:"pool,omitempty" protobuf:"bytes,4,opt,name=pool"`
	// The rados user name.
	// Default is admin.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	RadosUser string `json:"user,omitempty" protobuf:"bytes,5,opt,name=user"`
	// Keyring is the path to key ring for RBDUser.
	// Default is /etc/ceph/keyring.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	Keyring string `json:"keyring,omitempty" protobuf:"bytes,6,opt,name=keyring"`
	// SecretRef is name of the authentication secret for RBDUser. If provided
	// overrides keyring.
	// Default is nil.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty" protobuf:"bytes,7,opt,name=secretRef"`
	// ReadOnly here will force the ReadOnly setting in VolumeMounts.
	// Defaults to false.
	// More info: https://examples.k8s.io/volumes/rbd/README.md#how-to-use-it
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,8,opt,name=readOnly"`
}

// Represents a cinder volume resource in Openstack.
// A Cinder volume must exist before mounting to a container.
// The volume must also be in the same region as the kubelet.
// Cinder volumes support ownership management and SELinux relabeling.
type CinderVolumeSource struct {
	// volume id used to identify the volume in cinder.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	VolumeID string `json:"volumeID" protobuf:"bytes,1,opt,name=volumeID"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
	// Optional: points to a secret object containing parameters used to connect
	// to OpenStack.
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty" protobuf:"bytes,4,opt,name=secretRef"`
}

// Represents a cinder volume resource in Openstack.
// A Cinder volume must exist before mounting to a container.
// The volume must also be in the same region as the kubelet.
// Cinder volumes support ownership management and SELinux relabeling.
type CinderPersistentVolumeSource struct {
	// volume id used to identify the volume in cinder.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	VolumeID string `json:"volumeID" protobuf:"bytes,1,opt,name=volumeID"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// More info: https://examples.k8s.io/mysql-cinder-pd/README.md
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
	// Optional: points to a secret object containing parameters used to connect
	// to OpenStack.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty" protobuf:"bytes,4,opt,name=secretRef"`
}

// Represents a Ceph Filesystem mount that lasts the lifetime of a pod
// Cephfs volumes do not support ownership management or SELinux relabeling.
type CephFSVolumeSource struct {
	// Required: Monitors is a collection of Ceph monitors
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	Monitors []string `json:"monitors" protobuf:"bytes,1,rep,name=monitors"`
	// Optional: Used as the mounted root, rather than the full Ceph tree, default is /
	// +optional
	Path string `json:"path,omitempty" protobuf:"bytes,2,opt,name=path"`
	// Optional: User is the rados user name, default is admin
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	User string `json:"user,omitempty" protobuf:"bytes,3,opt,name=user"`
	// Optional: SecretFile is the path to key ring for User, default is /etc/ceph/user.secret
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	SecretFile string `json:"secretFile,omitempty" protobuf:"bytes,4,opt,name=secretFile"`
	// Optional: SecretRef is reference to the authentication secret for User, default is empty.
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty" protobuf:"bytes,5,opt,name=secretRef"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,6,opt,name=readOnly"`
}

// SecretReference represents a Secret Reference. It has enough information to retrieve secret
// in any namespace
type SecretReference struct {
	// Name is unique within a namespace to reference a secret resource.
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	// Namespace defines the space within which the secret name must be unique.
	// +optional
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
}

// Represents a Ceph Filesystem mount that lasts the lifetime of a pod
// Cephfs volumes do not support ownership management or SELinux relabeling.
type CephFSPersistentVolumeSource struct {
	// Required: Monitors is a collection of Ceph monitors
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	Monitors []string `json:"monitors" protobuf:"bytes,1,rep,name=monitors"`
	// Optional: Used as the mounted root, rather than the full Ceph tree, default is /
	// +optional
	Path string `json:"path,omitempty" protobuf:"bytes,2,opt,name=path"`
	// Optional: User is the rados user name, default is admin
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	User string `json:"user,omitempty" protobuf:"bytes,3,opt,name=user"`
	// Optional: SecretFile is the path to key ring for User, default is /etc/ceph/user.secret
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	SecretFile string `json:"secretFile,omitempty" protobuf:"bytes,4,opt,name=secretFile"`
	// Optional: SecretRef is reference to the authentication secret for User, default is empty.
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty" protobuf:"bytes,5,opt,name=secretRef"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// More info: https://examples.k8s.io/volumes/cephfs/README.md#how-to-use-it
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,6,opt,name=readOnly"`
}

// Represents a Flocker volume mounted by the Flocker agent.
// One and only one of datasetName and datasetUUID should be set.
// Flocker volumes do not support ownership management or SELinux relabeling.
type FlockerVolumeSource struct {
	// Name of the dataset stored as metadata -> name on the dataset for Flocker
	// should be considered as deprecated
	// +optional
	DatasetName string `json:"datasetName,omitempty" protobuf:"bytes,1,opt,name=datasetName"`
	// UUID of the dataset. This is unique identifier of a Flocker dataset
	// +optional
	DatasetUUID string `json:"datasetUUID,omitempty" protobuf:"bytes,2,opt,name=datasetUUID"`
}

// StorageMedium defines ways that storage can be allocated to a volume.
type StorageMedium string

const (
	StorageMediumDefault         StorageMedium = ""           // use whatever the default is for the node, assume anything we don't explicitly handle is this
	StorageMediumMemory          StorageMedium = "Memory"     // use memory (e.g. tmpfs on linux)
	StorageMediumHugePages       StorageMedium = "HugePages"  // use hugepages
	StorageMediumHugePagesPrefix StorageMedium = "HugePages-" // prefix for full medium notation HugePages-<size>
)

// Protocol defines network protocols supported for things like container ports.
type Protocol string

const (
	// ProtocolTCP is the TCP protocol.
	ProtocolTCP Protocol = "TCP"
	// ProtocolUDP is the UDP protocol.
	ProtocolUDP Protocol = "UDP"
	// ProtocolSCTP is the SCTP protocol.
	ProtocolSCTP Protocol = "SCTP"
)

// Represents a Persistent Disk resource in Google Compute Engine.
//
// A GCE PD must exist before mounting to a container. The disk must
// also be in the same GCE project and zone as the kubelet. A GCE PD
// can only be mounted as read/write once or read-only many times. GCE
// PDs support ownership management and SELinux relabeling.
type GCEPersistentDiskVolumeSource struct {
	// Unique name of the PD resource in GCE. Used to identify the disk in GCE.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
	PDName string `json:"pdName" protobuf:"bytes,1,opt,name=pdName"`
	// Filesystem type of the volume that you want to mount.
	// Tip: Ensure that the filesystem type is supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// The partition in the volume that you want to mount.
	// If omitted, the default is to mount by volume name.
	// Examples: For volume /dev/sda1, you specify the partition as "1".
	// Similarly, the volume partition for /dev/sda is "0" (or you can leave the property empty).
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
	// +optional
	Partition int32 `json:"partition,omitempty" protobuf:"varint,3,opt,name=partition"`
	// ReadOnly here will force the ReadOnly setting in VolumeMounts.
	// Defaults to false.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
}

// Represents a Quobyte mount that lasts the lifetime of a pod.
// Quobyte volumes do not support ownership management or SELinux relabeling.
type QuobyteVolumeSource struct {
	// Registry represents a single or multiple Quobyte Registry services
	// specified as a string as host:port pair (multiple entries are separated with commas)
	// which acts as the central registry for volumes
	Registry string `json:"registry" protobuf:"bytes,1,opt,name=registry"`

	// Volume is a string that references an already created Quobyte volume by name.
	Volume string `json:"volume" protobuf:"bytes,2,opt,name=volume"`

	// ReadOnly here will force the Quobyte volume to be mounted with read-only permissions.
	// Defaults to false.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`

	// User to map volume access to
	// Defaults to serivceaccount user
	// +optional
	User string `json:"user,omitempty" protobuf:"bytes,4,opt,name=user"`

	// Group to map volume access to
	// Default is no group
	// +optional
	Group string `json:"group,omitempty" protobuf:"bytes,5,opt,name=group"`

	// Tenant owning the given Quobyte volume in the Backend
	// Used with dynamically provisioned Quobyte volumes, value is set by the plugin
	// +optional
	Tenant string `json:"tenant,omitempty" protobuf:"bytes,6,opt,name=tenant"`
}

// FlexPersistentVolumeSource represents a generic persistent volume resource that is
// provisioned/attached using an exec based plugin.
type FlexPersistentVolumeSource struct {
	// Driver is the name of the driver to use for this volume.
	Driver string `json:"driver" protobuf:"bytes,1,opt,name=driver"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". The default filesystem depends on FlexVolume script.
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// Optional: SecretRef is reference to the secret object containing
	// sensitive information to pass to the plugin scripts. This may be
	// empty if no secret object is specified. If the secret object
	// contains more than one secret, all secrets are passed to the plugin
	// scripts.
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty" protobuf:"bytes,3,opt,name=secretRef"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
	// Optional: Extra command options if any.
	// +optional
	Options map[string]string `json:"options,omitempty" protobuf:"bytes,5,rep,name=options"`
}

// FlexVolume represents a generic volume resource that is
// provisioned/attached using an exec based plugin.
type FlexVolumeSource struct {
	// Driver is the name of the driver to use for this volume.
	Driver string `json:"driver" protobuf:"bytes,1,opt,name=driver"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". The default filesystem depends on FlexVolume script.
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// Optional: SecretRef is reference to the secret object containing
	// sensitive information to pass to the plugin scripts. This may be
	// empty if no secret object is specified. If the secret object
	// contains more than one secret, all secrets are passed to the plugin
	// scripts.
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty" protobuf:"bytes,3,opt,name=secretRef"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
	// Optional: Extra command options if any.
	// +optional
	Options map[string]string `json:"options,omitempty" protobuf:"bytes,5,rep,name=options"`
}

// Represents a Persistent Disk resource in AWS.
//
// An AWS EBS disk must exist before mounting to a container. The disk
// must also be in the same AWS zone as the kubelet. An AWS EBS disk
// can only be mounted as read/write once. AWS EBS volumes support
// ownership management and SELinux relabeling.
type AWSElasticBlockStoreVolumeSource struct {
	// Unique ID of the persistent disk resource in AWS (Amazon EBS volume).
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore
	VolumeID string `json:"volumeID" protobuf:"bytes,1,opt,name=volumeID"`
	// Filesystem type of the volume that you want to mount.
	// Tip: Ensure that the filesystem type is supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// The partition in the volume that you want to mount.
	// If omitted, the default is to mount by volume name.
	// Examples: For volume /dev/sda1, you specify the partition as "1".
	// Similarly, the volume partition for /dev/sda is "0" (or you can leave the property empty).
	// +optional
	Partition int32 `json:"partition,omitempty" protobuf:"varint,3,opt,name=partition"`
	// Specify "true" to force and set the ReadOnly property in VolumeMounts to "true".
	// If omitted, the default is "false".
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
}

// Represents a volume that is populated with the contents of a git repository.
// Git repo volumes do not support ownership management.
// Git repo volumes support SELinux relabeling.
//
// DEPRECATED: GitRepo is deprecated. To provision a container with a git repo, mount an
// EmptyDir into an InitContainer that clones the repo using git, then mount the EmptyDir
// into the Pod's container.
type GitRepoVolumeSource struct {
	// Repository URL
	Repository string `json:"repository" protobuf:"bytes,1,opt,name=repository"`
	// Commit hash for the specified revision.
	// +optional
	Revision string `json:"revision,omitempty" protobuf:"bytes,2,opt,name=revision"`
	// Target directory name.
	// Must not contain or start with '..'.  If '.' is supplied, the volume directory will be the
	// git repository.  Otherwise, if specified, the volume will contain the git repository in
	// the subdirectory with the given name.
	// +optional
	Directory string `json:"directory,omitempty" protobuf:"bytes,3,opt,name=directory"`
}

// Adapts a Secret into a volume.
//
// The contents of the target Secret's Data field will be presented in a volume
// as files using the keys in the Data field as the file names.
// Secret volumes support ownership management and SELinux relabeling.
type SecretVolumeSource struct {
	// Name of the secret in the pod's namespace to use.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#secret
	// +optional
	SecretName string `json:"secretName,omitempty" protobuf:"bytes,1,opt,name=secretName"`
	// If unspecified, each key-value pair in the Data field of the referenced
	// Secret will be projected into the volume as a file whose name is the
	// key and content is the value. If specified, the listed keys will be
	// projected into the specified paths, and unlisted keys will not be
	// present. If a key is specified which is not present in the Secret,
	// the volume setup will error unless it is marked optional. Paths must be
	// relative and may not contain the '..' path or start with '..'.
	// +optional
	Items []KeyToPath `json:"items,omitempty" protobuf:"bytes,2,rep,name=items"`
	// Optional: mode bits to use on created files by default. Must be a
	// value between 0 and 0777. Defaults to 0644.
	// Directories within the path are not affected by this setting.
	// This might be in conflict with other options that affect the file
	// mode, like fsGroup, and the result can be other mode bits set.
	// +optional
	DefaultMode *int32 `json:"defaultMode,omitempty" protobuf:"bytes,3,opt,name=defaultMode"`
	// Specify whether the Secret or its keys must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,4,opt,name=optional"`
}

const (
	SecretVolumeSourceDefaultMode int32 = 0644
)

// Adapts a secret into a projected volume.
//
// The contents of the target Secret's Data field will be presented in a
// projected volume as files using the keys in the Data field as the file names.
// Note that this is identical to a secret volume source without the default
// mode.
type SecretProjection struct {
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// If unspecified, each key-value pair in the Data field of the referenced
	// Secret will be projected into the volume as a file whose name is the
	// key and content is the value. If specified, the listed keys will be
	// projected into the specified paths, and unlisted keys will not be
	// present. If a key is specified which is not present in the Secret,
	// the volume setup will error unless it is marked optional. Paths must be
	// relative and may not contain the '..' path or start with '..'.
	// +optional
	Items []KeyToPath `json:"items,omitempty" protobuf:"bytes,2,rep,name=items"`
	// Specify whether the Secret or its key must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,4,opt,name=optional"`
}

// Represents an NFS mount that lasts the lifetime of a pod.
// NFS volumes do not support ownership management or SELinux relabeling.
type NFSVolumeSource struct {
	// Server is the hostname or IP address of the NFS server.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs
	Server string `json:"server" protobuf:"bytes,1,opt,name=server"`

	// Path that is exported by the NFS server.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs
	Path string `json:"path" protobuf:"bytes,2,opt,name=path"`

	// ReadOnly here will force
	// the NFS export to be mounted with read-only permissions.
	// Defaults to false.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
}

// Represents an ISCSI disk.
// ISCSI volumes can only be mounted as read/write once.
// ISCSI volumes support ownership management and SELinux relabeling.
type ISCSIVolumeSource struct {
	// iSCSI Target Portal. The Portal is either an IP or ip_addr:port if the port
	// is other than default (typically TCP ports 860 and 3260).
	TargetPortal string `json:"targetPortal" protobuf:"bytes,1,opt,name=targetPortal"`
	// Target iSCSI Qualified Name.
	IQN string `json:"iqn" protobuf:"bytes,2,opt,name=iqn"`
	// iSCSI Target Lun number.
	Lun int32 `json:"lun" protobuf:"varint,3,opt,name=lun"`
	// iSCSI Interface Name that uses an iSCSI transport.
	// Defaults to 'default' (tcp).
	// +optional
	ISCSIInterface string `json:"iscsiInterface,omitempty" protobuf:"bytes,4,opt,name=iscsiInterface"`
	// Filesystem type of the volume that you want to mount.
	// Tip: Ensure that the filesystem type is supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#iscsi
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,5,opt,name=fsType"`
	// ReadOnly here will force the ReadOnly setting in VolumeMounts.
	// Defaults to false.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,6,opt,name=readOnly"`
	// iSCSI Target Portal List. The portal is either an IP or ip_addr:port if the port
	// is other than default (typically TCP ports 860 and 3260).
	// +optional
	Portals []string `json:"portals,omitempty" protobuf:"bytes,7,opt,name=portals"`
	// whether support iSCSI Discovery CHAP authentication
	// +optional
	DiscoveryCHAPAuth bool `json:"chapAuthDiscovery,omitempty" protobuf:"varint,8,opt,name=chapAuthDiscovery"`
	// whether support iSCSI Session CHAP authentication
	// +optional
	SessionCHAPAuth bool `json:"chapAuthSession,omitempty" protobuf:"varint,11,opt,name=chapAuthSession"`
	// CHAP Secret for iSCSI target and initiator authentication
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty" protobuf:"bytes,10,opt,name=secretRef"`
	// Custom iSCSI Initiator Name.
	// If initiatorName is specified with iscsiInterface simultaneously, new iSCSI interface
	// <target portal>:<volume name> will be created for the connection.
	// +optional
	InitiatorName *string `json:"initiatorName,omitempty" protobuf:"bytes,12,opt,name=initiatorName"`
}

// ISCSIPersistentVolumeSource represents an ISCSI disk.
// ISCSI volumes can only be mounted as read/write once.
// ISCSI volumes support ownership management and SELinux relabeling.
type ISCSIPersistentVolumeSource struct {
	// iSCSI Target Portal. The Portal is either an IP or ip_addr:port if the port
	// is other than default (typically TCP ports 860 and 3260).
	TargetPortal string `json:"targetPortal" protobuf:"bytes,1,opt,name=targetPortal"`
	// Target iSCSI Qualified Name.
	IQN string `json:"iqn" protobuf:"bytes,2,opt,name=iqn"`
	// iSCSI Target Lun number.
	Lun int32 `json:"lun" protobuf:"varint,3,opt,name=lun"`
	// iSCSI Interface Name that uses an iSCSI transport.
	// Defaults to 'default' (tcp).
	// +optional
	ISCSIInterface string `json:"iscsiInterface,omitempty" protobuf:"bytes,4,opt,name=iscsiInterface"`
	// Filesystem type of the volume that you want to mount.
	// Tip: Ensure that the filesystem type is supported by the host operating system.
	// Examples: "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes#iscsi
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,5,opt,name=fsType"`
	// ReadOnly here will force the ReadOnly setting in VolumeMounts.
	// Defaults to false.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,6,opt,name=readOnly"`
	// iSCSI Target Portal List. The Portal is either an IP or ip_addr:port if the port
	// is other than default (typically TCP ports 860 and 3260).
	// +optional
	Portals []string `json:"portals,omitempty" protobuf:"bytes,7,opt,name=portals"`
	// whether support iSCSI Discovery CHAP authentication
	// +optional
	DiscoveryCHAPAuth bool `json:"chapAuthDiscovery,omitempty" protobuf:"varint,8,opt,name=chapAuthDiscovery"`
	// whether support iSCSI Session CHAP authentication
	// +optional
	SessionCHAPAuth bool `json:"chapAuthSession,omitempty" protobuf:"varint,11,opt,name=chapAuthSession"`
	// CHAP Secret for iSCSI target and initiator authentication
	// +optional
	SecretRef *SecretReference `json:"secretRef,omitempty" protobuf:"bytes,10,opt,name=secretRef"`
	// Custom iSCSI Initiator Name.
	// If initiatorName is specified with iscsiInterface simultaneously, new iSCSI interface
	// <target portal>:<volume name> will be created for the connection.
	// +optional
	InitiatorName *string `json:"initiatorName,omitempty" protobuf:"bytes,12,opt,name=initiatorName"`
}

// Represents a Fibre Channel volume.
// Fibre Channel volumes can only be mounted as read/write once.
// Fibre Channel volumes support ownership management and SELinux relabeling.
type FCVolumeSource struct {
	// Optional: FC target worldwide names (WWNs)
	// +optional
	TargetWWNs []string `json:"targetWWNs,omitempty" protobuf:"bytes,1,rep,name=targetWWNs"`
	// Optional: FC target lun number
	// +optional
	Lun *int32 `json:"lun,omitempty" protobuf:"varint,2,opt,name=lun"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// TODO: how do we prevent errors in the filesystem from compromising the machine
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,3,opt,name=fsType"`
	// Optional: Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
	// Optional: FC volume world wide identifiers (wwids)
	// Either wwids or combination of targetWWNs and lun must be set, but not both simultaneously.
	// +optional
	WWIDs []string `json:"wwids,omitempty" protobuf:"bytes,5,rep,name=wwids"`
}

// AzureFile represents an Azure File Service mount on the host and bind mount to the pod.
type AzureFileVolumeSource struct {
	// the name of secret that contains Azure Storage Account Name and Key
	SecretName string `json:"secretName" protobuf:"bytes,1,opt,name=secretName"`
	// Share Name
	ShareName string `json:"shareName" protobuf:"bytes,2,opt,name=shareName"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
}

// AzureFile represents an Azure File Service mount on the host and bind mount to the pod.
type AzureFilePersistentVolumeSource struct {
	// the name of secret that contains Azure Storage Account Name and Key
	SecretName string `json:"secretName" protobuf:"bytes,1,opt,name=secretName"`
	// Share Name
	ShareName string `json:"shareName" protobuf:"bytes,2,opt,name=shareName"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
	// the namespace of the secret that contains Azure Storage Account Name and Key
	// default is the same as the Pod
	// +optional
	SecretNamespace *string `json:"secretNamespace" protobuf:"bytes,4,opt,name=secretNamespace"`
}

// Represents a vSphere volume resource.
type VsphereVirtualDiskVolumeSource struct {
	// Path that identifies vSphere volume vmdk
	VolumePath string `json:"volumePath" protobuf:"bytes,1,opt,name=volumePath"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// Storage Policy Based Management (SPBM) profile name.
	// +optional
	StoragePolicyName string `json:"storagePolicyName,omitempty" protobuf:"bytes,3,opt,name=storagePolicyName"`
	// Storage Policy Based Management (SPBM) profile ID associated with the StoragePolicyName.
	// +optional
	StoragePolicyID string `json:"storagePolicyID,omitempty" protobuf:"bytes,4,opt,name=storagePolicyID"`
}

// Represents a Photon Controller persistent disk resource.
type PhotonPersistentDiskVolumeSource struct {
	// ID that identifies Photon Controller persistent disk
	PdID string `json:"pdID" protobuf:"bytes,1,opt,name=pdID"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
}

type AzureDataDiskCachingMode string
type AzureDataDiskKind string

const (
	AzureDataDiskCachingNone      AzureDataDiskCachingMode = "None"
	AzureDataDiskCachingReadOnly  AzureDataDiskCachingMode = "ReadOnly"
	AzureDataDiskCachingReadWrite AzureDataDiskCachingMode = "ReadWrite"

	AzureSharedBlobDisk    AzureDataDiskKind = "Shared"
	AzureDedicatedBlobDisk AzureDataDiskKind = "Dedicated"
	AzureManagedDisk       AzureDataDiskKind = "Managed"
)

// AzureDisk represents an Azure Data Disk mount on the host and bind mount to the pod.
type AzureDiskVolumeSource struct {
	// The Name of the data disk in the blob storage
	DiskName string `json:"diskName" protobuf:"bytes,1,opt,name=diskName"`
	// The URI the data disk in the blob storage
	DataDiskURI string `json:"diskURI" protobuf:"bytes,2,opt,name=diskURI"`
	// Host Caching mode: None, Read Only, Read Write.
	// +optional
	CachingMode *AzureDataDiskCachingMode `json:"cachingMode,omitempty" protobuf:"bytes,3,opt,name=cachingMode,casttype=AzureDataDiskCachingMode"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// +optional
	FSType *string `json:"fsType,omitempty" protobuf:"bytes,4,opt,name=fsType"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly *bool `json:"readOnly,omitempty" protobuf:"varint,5,opt,name=readOnly"`
	// Expected values Shared: multiple blob disks per storage account  Dedicated: single blob disk per storage account  Managed: azure managed data disk (only in managed availability set). defaults to shared
	Kind *AzureDataDiskKind `json:"kind,omitempty" protobuf:"bytes,6,opt,name=kind,casttype=AzureDataDiskKind"`
}

// PortworxVolumeSource represents a Portworx volume resource.
type PortworxVolumeSource struct {
	// VolumeID uniquely identifies a Portworx volume
	VolumeID string `json:"volumeID" protobuf:"bytes,1,opt,name=volumeID"`
	// FSType represents the filesystem type to mount
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs". Implicitly inferred to be "ext4" if unspecified.
	FSType string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`
}

// ScaleIOVolumeSource represents a persistent ScaleIO volume
type ScaleIOVolumeSource struct {
	// The host address of the ScaleIO API Gateway.
	Gateway string `json:"gateway" protobuf:"bytes,1,opt,name=gateway"`
	// The name of the storage system as configured in ScaleIO.
	System string `json:"system" protobuf:"bytes,2,opt,name=system"`
	// SecretRef references to the secret for ScaleIO user and other
	// sensitive information. If this is not provided, Login operation will fail.
	SecretRef *LocalObjectReference `json:"secretRef" protobuf:"bytes,3,opt,name=secretRef"`
	// Flag to enable/disable SSL communication with Gateway, default false
	// +optional
	SSLEnabled bool `json:"sslEnabled,omitempty" protobuf:"varint,4,opt,name=sslEnabled"`
	// The name of the ScaleIO Protection Domain for the configured storage.
	// +optional
	ProtectionDomain string `json:"protectionDomain,omitempty" protobuf:"bytes,5,opt,name=protectionDomain"`
	// The ScaleIO Storage Pool associated with the protection domain.
	// +optional
	StoragePool string `json:"storagePool,omitempty" protobuf:"bytes,6,opt,name=storagePool"`
	// Indicates whether the storage for a volume should be ThickProvisioned or ThinProvisioned.
	// Default is ThinProvisioned.
	// +optional
	StorageMode string `json:"storageMode,omitempty" protobuf:"bytes,7,opt,name=storageMode"`
	// The name of a volume already created in the ScaleIO system
	// that is associated with this volume source.
	VolumeName string `json:"volumeName,omitempty" protobuf:"bytes,8,opt,name=volumeName"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs".
	// Default is "xfs".
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,9,opt,name=fsType"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,10,opt,name=readOnly"`
}

// ScaleIOPersistentVolumeSource represents a persistent ScaleIO volume
type ScaleIOPersistentVolumeSource struct {
	// The host address of the ScaleIO API Gateway.
	Gateway string `json:"gateway" protobuf:"bytes,1,opt,name=gateway"`
	// The name of the storage system as configured in ScaleIO.
	System string `json:"system" protobuf:"bytes,2,opt,name=system"`
	// SecretRef references to the secret for ScaleIO user and other
	// sensitive information. If this is not provided, Login operation will fail.
	SecretRef *SecretReference `json:"secretRef" protobuf:"bytes,3,opt,name=secretRef"`
	// Flag to enable/disable SSL communication with Gateway, default false
	// +optional
	SSLEnabled bool `json:"sslEnabled,omitempty" protobuf:"varint,4,opt,name=sslEnabled"`
	// The name of the ScaleIO Protection Domain for the configured storage.
	// +optional
	ProtectionDomain string `json:"protectionDomain,omitempty" protobuf:"bytes,5,opt,name=protectionDomain"`
	// The ScaleIO Storage Pool associated with the protection domain.
	// +optional
	StoragePool string `json:"storagePool,omitempty" protobuf:"bytes,6,opt,name=storagePool"`
	// Indicates whether the storage for a volume should be ThickProvisioned or ThinProvisioned.
	// Default is ThinProvisioned.
	// +optional
	StorageMode string `json:"storageMode,omitempty" protobuf:"bytes,7,opt,name=storageMode"`
	// The name of a volume already created in the ScaleIO system
	// that is associated with this volume source.
	VolumeName string `json:"volumeName,omitempty" protobuf:"bytes,8,opt,name=volumeName"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs".
	// Default is "xfs"
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,9,opt,name=fsType"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,10,opt,name=readOnly"`
}

// Represents a StorageOS persistent volume resource.
type StorageOSVolumeSource struct {
	// VolumeName is the human-readable name of the StorageOS volume.  Volume
	// names are only unique within a namespace.
	VolumeName string `json:"volumeName,omitempty" protobuf:"bytes,1,opt,name=volumeName"`
	// VolumeNamespace specifies the scope of the volume within StorageOS.  If no
	// namespace is specified then the Pod's namespace will be used.  This allows the
	// Kubernetes name scoping to be mirrored within StorageOS for tighter integration.
	// Set VolumeName to any name to override the default behaviour.
	// Set to "default" if you are not using namespaces within StorageOS.
	// Namespaces that do not pre-exist within StorageOS will be created.
	// +optional
	VolumeNamespace string `json:"volumeNamespace,omitempty" protobuf:"bytes,2,opt,name=volumeNamespace"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,3,opt,name=fsType"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
	// SecretRef specifies the secret to use for obtaining the StorageOS API
	// credentials.  If not specified, default values will be attempted.
	// +optional
	SecretRef *LocalObjectReference `json:"secretRef,omitempty" protobuf:"bytes,5,opt,name=secretRef"`
}

// Represents a StorageOS persistent volume resource.
type StorageOSPersistentVolumeSource struct {
	// VolumeName is the human-readable name of the StorageOS volume.  Volume
	// names are only unique within a namespace.
	VolumeName string `json:"volumeName,omitempty" protobuf:"bytes,1,opt,name=volumeName"`
	// VolumeNamespace specifies the scope of the volume within StorageOS.  If no
	// namespace is specified then the Pod's namespace will be used.  This allows the
	// Kubernetes name scoping to be mirrored within StorageOS for tighter integration.
	// Set VolumeName to any name to override the default behaviour.
	// Set to "default" if you are not using namespaces within StorageOS.
	// Namespaces that do not pre-exist within StorageOS will be created.
	// +optional
	VolumeNamespace string `json:"volumeNamespace,omitempty" protobuf:"bytes,2,opt,name=volumeNamespace"`
	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". Implicitly inferred to be "ext4" if unspecified.
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,3,opt,name=fsType"`
	// Defaults to false (read/write). ReadOnly here will force
	// the ReadOnly setting in VolumeMounts.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,4,opt,name=readOnly"`
	// SecretRef specifies the secret to use for obtaining the StorageOS API
	// credentials.  If not specified, default values will be attempted.
	// +optional
	SecretRef *ObjectReference `json:"secretRef,omitempty" protobuf:"bytes,5,opt,name=secretRef"`
}

// Adapts a ConfigMap into a volume.
//
// The contents of the target ConfigMap's Data field will be presented in a
// volume as files using the keys in the Data field as the file names, unless
// the items element is populated with specific mappings of keys to paths.
// ConfigMap volumes support ownership management and SELinux relabeling.
type ConfigMapVolumeSource struct {
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// If unspecified, each key-value pair in the Data field of the referenced
	// ConfigMap will be projected into the volume as a file whose name is the
	// key and content is the value. If specified, the listed keys will be
	// projected into the specified paths, and unlisted keys will not be
	// present. If a key is specified which is not present in the ConfigMap,
	// the volume setup will error unless it is marked optional. Paths must be
	// relative and may not contain the '..' path or start with '..'.
	// +optional
	Items []KeyToPath `json:"items,omitempty" protobuf:"bytes,2,rep,name=items"`
	// Optional: mode bits to use on created files by default. Must be a
	// value between 0 and 0777. Defaults to 0644.
	// Directories within the path are not affected by this setting.
	// This might be in conflict with other options that affect the file
	// mode, like fsGroup, and the result can be other mode bits set.
	// +optional
	DefaultMode *int32 `json:"defaultMode,omitempty" protobuf:"varint,3,opt,name=defaultMode"`
	// Specify whether the ConfigMap or its keys must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,4,opt,name=optional"`
}

const (
	ConfigMapVolumeSourceDefaultMode int32 = 0644
)

// Adapts a ConfigMap into a projected volume.
//
// The contents of the target ConfigMap's Data field will be presented in a
// projected volume as files using the keys in the Data field as the file names,
// unless the items element is populated with specific mappings of keys to paths.
// Note that this is identical to a configmap volume source without the default
// mode.
type ConfigMapProjection struct {
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// If unspecified, each key-value pair in the Data field of the referenced
	// ConfigMap will be projected into the volume as a file whose name is the
	// key and content is the value. If specified, the listed keys will be
	// projected into the specified paths, and unlisted keys will not be
	// present. If a key is specified which is not present in the ConfigMap,
	// the volume setup will error unless it is marked optional. Paths must be
	// relative and may not contain the '..' path or start with '..'.
	// +optional
	Items []KeyToPath `json:"items,omitempty" protobuf:"bytes,2,rep,name=items"`
	// Specify whether the ConfigMap or its keys must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,4,opt,name=optional"`
}

// ServiceAccountTokenProjection represents a projected service account token
// volume. This projection can be used to insert a service account token into
// the pods runtime filesystem for use against APIs (Kubernetes API Server or
// otherwise).
type ServiceAccountTokenProjection struct {
	// Audience is the intended audience of the token. A recipient of a token
	// must identify itself with an identifier specified in the audience of the
	// token, and otherwise should reject the token. The audience defaults to the
	// identifier of the apiserver.
	//+optional
	Audience string `json:"audience,omitempty" protobuf:"bytes,1,rep,name=audience"`
	// ExpirationSeconds is the requested duration of validity of the service
	// account token. As the token approaches expiration, the kubelet volume
	// plugin will proactively rotate the service account token. The kubelet will
	// start trying to rotate the token if the token is older than 80 percent of
	// its time to live or if the token is older than 24 hours.Defaults to 1 hour
	// and must be at least 10 minutes.
	//+optional
	ExpirationSeconds *int64 `json:"expirationSeconds,omitempty" protobuf:"varint,2,opt,name=expirationSeconds"`
	// Path is the path relative to the mount point of the file to project the
	// token into.
	Path string `json:"path" protobuf:"bytes,3,opt,name=path"`
}

// Represents a projected volume source
type ProjectedVolumeSource struct {
	// list of volume projections
	Sources []VolumeProjection `json:"sources" protobuf:"bytes,1,rep,name=sources"`
	// Mode bits to use on created files by default. Must be a value between
	// 0 and 0777.
	// Directories within the path are not affected by this setting.
	// This might be in conflict with other options that affect the file
	// mode, like fsGroup, and the result can be other mode bits set.
	// +optional
	DefaultMode *int32 `json:"defaultMode,omitempty" protobuf:"varint,2,opt,name=defaultMode"`
}

// Projection that may be projected along with other supported volume types
type VolumeProjection struct {
	// all types below are the supported types for projection into the same volume

	// information about the secret data to project
	// +optional
	Secret *SecretProjection `json:"secret,omitempty" protobuf:"bytes,1,opt,name=secret"`
	// information about the downwardAPI data to project
	// +optional
	DownwardAPI *DownwardAPIProjection `json:"downwardAPI,omitempty" protobuf:"bytes,2,opt,name=downwardAPI"`
	// information about the configMap data to project
	// +optional
	ConfigMap *ConfigMapProjection `json:"configMap,omitempty" protobuf:"bytes,3,opt,name=configMap"`
	// information about the serviceAccountToken data to project
	// +optional
	ServiceAccountToken *ServiceAccountTokenProjection `json:"serviceAccountToken,omitempty" protobuf:"bytes,4,opt,name=serviceAccountToken"`
}

const (
	ProjectedVolumeSourceDefaultMode int32 = 0644
)

// Maps a string key to a path within a volume.
type KeyToPath struct {
	// The key to project.
	Key string `json:"key" protobuf:"bytes,1,opt,name=key"`

	// The relative path of the file to map the key to.
	// May not be an absolute path.
	// May not contain the path element '..'.
	// May not start with the string '..'.
	Path string `json:"path" protobuf:"bytes,2,opt,name=path"`
	// Optional: mode bits to use on this file, must be a value between 0
	// and 0777. If not specified, the volume defaultMode will be used.
	// This might be in conflict with other options that affect the file
	// mode, like fsGroup, and the result can be other mode bits set.
	// +optional
	Mode *int32 `json:"mode,omitempty" protobuf:"varint,3,opt,name=mode"`
}

// Local represents directly-attached storage with node affinity (Beta feature)
type LocalVolumeSource struct {
	// The full path to the volume on the node.
	// It can be either a directory or block device (disk, partition, ...).
	Path string `json:"path" protobuf:"bytes,1,opt,name=path"`

	// Filesystem type to mount.
	// It applies only when the Path is a block device.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs". The default value is to auto-select a fileystem if unspecified.
	// +optional
	FSType *string `json:"fsType,omitempty" protobuf:"bytes,2,opt,name=fsType"`
}

// Represents storage that is managed by an external CSI volume driver (Beta feature)
type CSIPersistentVolumeSource struct {
	// Driver is the name of the driver to use for this volume.
	// Required.
	Driver string `json:"driver" protobuf:"bytes,1,opt,name=driver"`

	// VolumeHandle is the unique volume name returned by the CSI volume
	// plugin’s CreateVolume to refer to the volume on all subsequent calls.
	// Required.
	VolumeHandle string `json:"volumeHandle" protobuf:"bytes,2,opt,name=volumeHandle"`

	// Optional: The value to pass to ControllerPublishVolumeRequest.
	// Defaults to false (read/write).
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,3,opt,name=readOnly"`

	// Filesystem type to mount.
	// Must be a filesystem type supported by the host operating system.
	// Ex. "ext4", "xfs", "ntfs".
	// +optional
	FSType string `json:"fsType,omitempty" protobuf:"bytes,4,opt,name=fsType"`

	// Attributes of the volume to publish.
	// +optional
	VolumeAttributes map[string]string `json:"volumeAttributes,omitempty" protobuf:"bytes,5,rep,name=volumeAttributes"`

	// ControllerPublishSecretRef is a reference to the secret object containing
	// sensitive information to pass to the CSI driver to complete the CSI
	// ControllerPublishVolume and ControllerUnpublishVolume calls.
	// This field is optional, and may be empty if no secret is required. If the
	// secret object contains more than one secret, all secrets are passed.
	// +optional
	ControllerPublishSecretRef *SecretReference `json:"controllerPublishSecretRef,omitempty" protobuf:"bytes,6,opt,name=controllerPublishSecretRef"`

	// NodeStageSecretRef is a reference to the secret object containing sensitive
	// information to pass to the CSI driver to complete the CSI NodeStageVolume
	// and NodeStageVolume and NodeUnstageVolume calls.
	// This field is optional, and may be empty if no secret is required. If the
	// secret object contains more than one secret, all secrets are passed.
	// +optional
	NodeStageSecretRef *SecretReference `json:"nodeStageSecretRef,omitempty" protobuf:"bytes,7,opt,name=nodeStageSecretRef"`

	// NodePublishSecretRef is a reference to the secret object containing
	// sensitive information to pass to the CSI driver to complete the CSI
	// NodePublishVolume and NodeUnpublishVolume calls.
	// This field is optional, and may be empty if no secret is required. If the
	// secret object contains more than one secret, all secrets are passed.
	// +optional
	NodePublishSecretRef *SecretReference `json:"nodePublishSecretRef,omitempty" protobuf:"bytes,8,opt,name=nodePublishSecretRef"`

	// ControllerExpandSecretRef is a reference to the secret object containing
	// sensitive information to pass to the CSI driver to complete the CSI
	// ControllerExpandVolume call.
	// This is an alpha field and requires enabling ExpandCSIVolumes feature gate.
	// This field is optional, and may be empty if no secret is required. If the
	// secret object contains more than one secret, all secrets are passed.
	// +optional
	ControllerExpandSecretRef *SecretReference `json:"controllerExpandSecretRef,omitempty" protobuf:"bytes,9,opt,name=controllerExpandSecretRef"`
}

// Represents a source location of a volume to mount, managed by an external CSI driver
type CSIVolumeSource struct {
	// Driver is the name of the CSI driver that handles this volume.
	// Consult with your admin for the correct name as registered in the cluster.
	Driver string `json:"driver" protobuf:"bytes,1,opt,name=driver"`

	// Specifies a read-only configuration for the volume.
	// Defaults to false (read/write).
	// +optional
	ReadOnly *bool `json:"readOnly,omitempty" protobuf:"varint,2,opt,name=readOnly"`

	// Filesystem type to mount. Ex. "ext4", "xfs", "ntfs".
	// If not provided, the empty value is passed to the associated CSI driver
	// which will determine the default filesystem to apply.
	// +optional
	FSType *string `json:"fsType,omitempty" protobuf:"bytes,3,opt,name=fsType"`

	// VolumeAttributes stores driver-specific properties that are passed to the CSI
	// driver. Consult your driver's documentation for supported values.
	// +optional
	VolumeAttributes map[string]string `json:"volumeAttributes,omitempty" protobuf:"bytes,4,rep,name=volumeAttributes"`

	// NodePublishSecretRef is a reference to the secret object containing
	// sensitive information to pass to the CSI driver to complete the CSI
	// NodePublishVolume and NodeUnpublishVolume calls.
	// This field is optional, and  may be empty if no secret is required. If the
	// secret object contains more than one secret, all secret references are passed.
	// +optional
	NodePublishSecretRef *LocalObjectReference `json:"nodePublishSecretRef,omitempty" protobuf:"bytes,5,opt,name=nodePublishSecretRef"`
}

// ContainerPort represents a network port in a single container.
type ContainerPort struct {
	// If specified, this must be an IANA_SVC_NAME and unique within the pod. Each
	// named port in a pod must have a unique name. Name for the port that can be
	// referred to by services.
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	// Number of port to expose on the host.
	// If specified, this must be a valid port number, 0 < x < 65536.
	// If HostNetwork is specified, this must match ContainerPort.
	// Most containers do not need this.
	// +optional
	HostPort int32 `json:"hostPort,omitempty" protobuf:"varint,2,opt,name=hostPort"`
	// Number of port to expose on the pod's IP address.
	// This must be a valid port number, 0 < x < 65536.
	ContainerPort int32 `json:"containerPort" protobuf:"varint,3,opt,name=containerPort"`
	// Protocol for port. Must be UDP, TCP, or SCTP.
	// Defaults to "TCP".
	// +optional
	Protocol Protocol `json:"protocol,omitempty" protobuf:"bytes,4,opt,name=protocol,casttype=Protocol"`
	// What host IP to bind the external port to.
	// +optional
	HostIP string `json:"hostIP,omitempty" protobuf:"bytes,5,opt,name=hostIP"`
}

// VolumeMount describes a mounting of a Volume within a container.
type VolumeMount struct {
	// This must match the Name of a Volume.
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// Mounted read-only if true, read-write otherwise (false or unspecified).
	// Defaults to false.
	// +optional
	ReadOnly bool `json:"readOnly,omitempty" protobuf:"varint,2,opt,name=readOnly"`
	// Path within the container at which the volume should be mounted.  Must
	// not contain ':'.
	MountPath string `json:"mountPath" protobuf:"bytes,3,opt,name=mountPath"`
	// Path within the volume from which the container's volume should be mounted.
	// Defaults to "" (volume's root).
	// +optional
	SubPath string `json:"subPath,omitempty" protobuf:"bytes,4,opt,name=subPath"`
	// mountPropagation determines how mounts are propagated from the host
	// to container and the other way around.
	// When not set, MountPropagationNone is used.
	// This field is beta in 1.10.
	// +optional
	MountPropagation *MountPropagationMode `json:"mountPropagation,omitempty" protobuf:"bytes,5,opt,name=mountPropagation,casttype=MountPropagationMode"`
	// Expanded path within the volume from which the container's volume should be mounted.
	// Behaves similarly to SubPath but environment variable references $(VAR_NAME) are expanded using the container's environment.
	// Defaults to "" (volume's root).
	// SubPathExpr and SubPath are mutually exclusive.
	// +optional
	SubPathExpr string `json:"subPathExpr,omitempty" protobuf:"bytes,6,opt,name=subPathExpr"`
}

// MountPropagationMode describes mount propagation.
type MountPropagationMode string

const (
	// MountPropagationNone means that the volume in a container will
	// not receive new mounts from the host or other containers, and filesystems
	// mounted inside the container won't be propagated to the host or other
	// containers.
	// Note that this mode corresponds to "private" in Linux terminology.
	MountPropagationNone MountPropagationMode = "None"
	// MountPropagationHostToContainer means that the volume in a container will
	// receive new mounts from the host or other containers, but filesystems
	// mounted inside the container won't be propagated to the host or other
	// containers.
	// Note that this mode is recursively applied to all mounts in the volume
	// ("rslave" in Linux terminology).
	MountPropagationHostToContainer MountPropagationMode = "HostToContainer"
	// MountPropagationBidirectional means that the volume in a container will
	// receive new mounts from the host or other containers, and its own mounts
	// will be propagated from the container to the host or other containers.
	// Note that this mode is recursively applied to all mounts in the volume
	// ("rshared" in Linux terminology).
	MountPropagationBidirectional MountPropagationMode = "Bidirectional"
)

// volumeDevice describes a mapping of a raw block device within a container.
type VolumeDevice struct {
	// name must match the name of a persistentVolumeClaim in the pod
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// devicePath is the path inside of the container that the device will be mapped to.
	DevicePath string `json:"devicePath" protobuf:"bytes,2,opt,name=devicePath"`
}

// EnvVar represents an environment variable present in a Container.
type EnvVar struct {
	// Name of the environment variable. Must be a C_IDENTIFIER.
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`

	// Optional: no more than one of the following may be specified.

	// Variable references $(VAR_NAME) are expanded
	// using the previous defined environment variables in the container and
	// any service environment variables. If a variable cannot be resolved,
	// the reference in the input string will be unchanged. The $(VAR_NAME)
	// syntax can be escaped with a double $$, ie: $$(VAR_NAME). Escaped
	// references will never be expanded, regardless of whether the variable
	// exists or not.
	// Defaults to "".
	// +optional
	Value string `json:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
	// Source for the environment variable's value. Cannot be used if value is not empty.
	// +optional
	ValueFrom *EnvVarSource `json:"valueFrom,omitempty" protobuf:"bytes,3,opt,name=valueFrom"`
}

// EnvVarSource represents a source for the value of an EnvVar.
type EnvVarSource struct {
	// Selects a field of the pod: supports metadata.name, metadata.namespace, metadata.labels, metadata.annotations,
	// spec.nodeName, spec.serviceAccountName, status.hostIP, status.podIP, status.podIPs.
	// +optional
	FieldRef *ObjectFieldSelector `json:"fieldRef,omitempty" protobuf:"bytes,1,opt,name=fieldRef"`
	// Selects a resource of the container: only resources limits and requests
	// (limits.cpu, limits.memory, limits.ephemeral-storage, requests.cpu, requests.memory and requests.ephemeral-storage) are currently supported.
	// +optional
	ResourceFieldRef *ResourceFieldSelector `json:"resourceFieldRef,omitempty" protobuf:"bytes,2,opt,name=resourceFieldRef"`
	// Selects a key of a ConfigMap.
	// +optional
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty" protobuf:"bytes,3,opt,name=configMapKeyRef"`
	// Selects a key of a secret in the pod's namespace
	// +optional
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty" protobuf:"bytes,4,opt,name=secretKeyRef"`
}

// ObjectFieldSelector selects an APIVersioned field of an object.
type ObjectFieldSelector struct {
	// Version of the schema the FieldPath is written in terms of, defaults to "v1".
	// +optional
	APIVersion string `json:"apiVersion,omitempty" protobuf:"bytes,1,opt,name=apiVersion"`
	// Path of the field to select in the specified API version.
	FieldPath string `json:"fieldPath" protobuf:"bytes,2,opt,name=fieldPath"`
}

// ResourceFieldSelector represents container resources (cpu, memory) and their output format
type ResourceFieldSelector struct {
	// Container name: required for volumes, optional for env vars
	// +optional
	ContainerName string `json:"containerName,omitempty" protobuf:"bytes,1,opt,name=containerName"`
	// Required: resource to select
	Resource string `json:"resource" protobuf:"bytes,2,opt,name=resource"`
	// Specifies the output format of the exposed resources, defaults to "1"
	// +optional
	Divisor resource.Quantity `json:"divisor,omitempty" protobuf:"bytes,3,opt,name=divisor"`
}

// Selects a key from a ConfigMap.
type ConfigMapKeySelector struct {
	// The ConfigMap to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key to select.
	Key string `json:"key" protobuf:"bytes,2,opt,name=key"`
	// Specify whether the ConfigMap or its key must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// SecretKeySelector selects a key of a Secret.
type SecretKeySelector struct {
	// The name of the secret in the pod's namespace to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// The key of the secret to select from.  Must be a valid secret key.
	Key string `json:"key" protobuf:"bytes,2,opt,name=key"`
	// Specify whether the Secret or its key must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,3,opt,name=optional"`
}

// EnvFromSource represents the source of a set of ConfigMaps
type EnvFromSource struct {
	// An optional identifier to prepend to each key in the ConfigMap. Must be a C_IDENTIFIER.
	// +optional
	Prefix string `json:"prefix,omitempty" protobuf:"bytes,1,opt,name=prefix"`
	// The ConfigMap to select from
	// +optional
	ConfigMapRef *ConfigMapEnvSource `json:"configMapRef,omitempty" protobuf:"bytes,2,opt,name=configMapRef"`
	// The Secret to select from
	// +optional
	SecretRef *SecretEnvSource `json:"secretRef,omitempty" protobuf:"bytes,3,opt,name=secretRef"`
}

// ConfigMapEnvSource selects a ConfigMap to populate the environment
// variables with.
//
// The contents of the target ConfigMap's Data field will represent the
// key-value pairs as environment variables.
type ConfigMapEnvSource struct {
	// The ConfigMap to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// Specify whether the ConfigMap must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,2,opt,name=optional"`
}

// SecretEnvSource selects a Secret to populate the environment
// variables with.
//
// The contents of the target Secret's Data field will represent the
// key-value pairs as environment variables.
type SecretEnvSource struct {
	// The Secret to select from.
	LocalObjectReference `json:",inline" protobuf:"bytes,1,opt,name=localObjectReference"`
	// Specify whether the Secret must be defined
	// +optional
	Optional *bool `json:"optional,omitempty" protobuf:"varint,2,opt,name=optional"`
}

// HTTPHeader describes a custom header to be used in HTTP probes
type HTTPHeader struct {
	// The header field name
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// The header field value
	Value string `json:"value" protobuf:"bytes,2,opt,name=value"`
}

// HTTPGetAction describes an action based on HTTP Get requests.
type HTTPGetAction struct {
	// Path to access on the HTTP server.
	// +optional
	Path string `json:"path,omitempty" protobuf:"bytes,1,opt,name=path"`
	// Name or number of the port to access on the container.
	// Number must be in the range 1 to 65535.
	// Name must be an IANA_SVC_NAME.
	Port intstr.IntOrString `json:"port" protobuf:"bytes,2,opt,name=port"`
	// Host name to connect to, defaults to the pod IP. You probably want to set
	// "Host" in httpHeaders instead.
	// +optional
	Host string `json:"host,omitempty" protobuf:"bytes,3,opt,name=host"`
	// Scheme to use for connecting to the host.
	// Defaults to HTTP.
	// +optional
	Scheme URIScheme `json:"scheme,omitempty" protobuf:"bytes,4,opt,name=scheme,casttype=URIScheme"`
	// Custom headers to set in the request. HTTP allows repeated headers.
	// +optional
	HTTPHeaders []HTTPHeader `json:"httpHeaders,omitempty" protobuf:"bytes,5,rep,name=httpHeaders"`
}

// URIScheme identifies the scheme used for connection to a host for Get actions
type URIScheme string

const (
	// URISchemeHTTP means that the scheme used will be http://
	URISchemeHTTP URIScheme = "HTTP"
	// URISchemeHTTPS means that the scheme used will be https://
	URISchemeHTTPS URIScheme = "HTTPS"
)

// TCPSocketAction describes an action based on opening a socket
type TCPSocketAction struct {
	// Number or name of the port to access on the container.
	// Number must be in the range 1 to 65535.
	// Name must be an IANA_SVC_NAME.
	Port intstr.IntOrString `json:"port" protobuf:"bytes,1,opt,name=port"`
	// Optional: Host name to connect to, defaults to the pod IP.
	// +optional
	Host string `json:"host,omitempty" protobuf:"bytes,2,opt,name=host"`
}

// ExecAction describes a "run in container" action.
type ExecAction struct {
	// Command is the command line to execute inside the container, the working directory for the
	// command  is root ('/') in the container's filesystem. The command is simply exec'd, it is
	// not run inside a shell, so traditional shell instructions ('|', etc) won't work. To use
	// a shell, you need to explicitly call out to that shell.
	// Exit status of 0 is treated as live/healthy and non-zero is unhealthy.
	// +optional
	Command []string `json:"command,omitempty" protobuf:"bytes,1,rep,name=command"`
}

// Probe describes a health check to be performed against a container to determine whether it is
// alive or ready to receive traffic.
type Probe struct {
	// The action taken to determine the health of a container
	Handler `json:",inline" protobuf:"bytes,1,opt,name=handler"`
	// Number of seconds after the container has started before liveness probes are initiated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty" protobuf:"varint,2,opt,name=initialDelaySeconds"`
	// Number of seconds after which the probe times out.
	// Defaults to 1 second. Minimum value is 1.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty" protobuf:"varint,3,opt,name=timeoutSeconds"`
	// How often (in seconds) to perform the probe.
	// Default to 10 seconds. Minimum value is 1.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty" protobuf:"varint,4,opt,name=periodSeconds"`
	// Minimum consecutive successes for the probe to be considered successful after having failed.
	// Defaults to 1. Must be 1 for liveness and startup. Minimum value is 1.
	// +optional
	SuccessThreshold int32 `json:"successThreshold,omitempty" protobuf:"varint,5,opt,name=successThreshold"`
	// Minimum consecutive failures for the probe to be considered failed after having succeeded.
	// Defaults to 3. Minimum value is 1.
	// +optional
	FailureThreshold int32 `json:"failureThreshold,omitempty" protobuf:"varint,6,opt,name=failureThreshold"`
}

// PullPolicy describes a policy for if/when to pull a container image
type PullPolicy string

const (
	// PullAlways means that kubelet always attempts to pull the latest image. Container will fail If the pull fails.
	PullAlways PullPolicy = "Always"
	// PullNever means that kubelet never pulls an image, but only uses a local image. Container will fail if the image isn't present
	PullNever PullPolicy = "Never"
	// PullIfNotPresent means that kubelet pulls if the image isn't present on disk. Container will fail if the image isn't present and the pull fails.
	PullIfNotPresent PullPolicy = "IfNotPresent"
)

// PreemptionPolicy describes a policy for if/when to preempt a pod.
type PreemptionPolicy string

const (
	// PreemptLowerPriority means that pod can preempt other pods with lower priority.
	PreemptLowerPriority PreemptionPolicy = "PreemptLowerPriority"
	// PreemptNever means that pod never preempts other pods with lower priority.
	PreemptNever PreemptionPolicy = "Never"
)

// TerminationMessagePolicy describes how termination messages are retrieved from a container.
type TerminationMessagePolicy string

const (
	// TerminationMessageReadFile is the default behavior and will set the container status message to
	// the contents of the container's terminationMessagePath when the container exits.
	TerminationMessageReadFile TerminationMessagePolicy = "File"
	// TerminationMessageFallbackToLogsOnError will read the most recent contents of the container logs
	// for the container status message when the container exits with an error and the
	// terminationMessagePath has no contents.
	TerminationMessageFallbackToLogsOnError TerminationMessagePolicy = "FallbackToLogsOnError"
)

// Capability represent POSIX capabilities type
type Capability string

// Adds and removes POSIX capabilities from running containers.
type Capabilities struct {
	// Added capabilities
	// +optional
	Add []Capability `json:"add,omitempty" protobuf:"bytes,1,rep,name=add,casttype=Capability"`
	// Removed capabilities
	// +optional
	Drop []Capability `json:"drop,omitempty" protobuf:"bytes,2,rep,name=drop,casttype=Capability"`
}

// ResourceRequirements describes the compute resource requirements.
type ResourceRequirements struct {
	// Limits describes the maximum amount of compute resources allowed.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Limits ResourceList `json:"limits,omitempty" protobuf:"bytes,1,rep,name=limits,casttype=ResourceList,castkey=ResourceName"`
	// Requests describes the minimum amount of compute resources required.
	// If Requests is omitted for a container, it defaults to Limits if that is explicitly specified,
	// otherwise to an implementation-defined value.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Requests ResourceList `json:"requests,omitempty" protobuf:"bytes,2,rep,name=requests,casttype=ResourceList,castkey=ResourceName"`
}

const (
	// TerminationMessagePathDefault means the default path to capture the application termination message running in a container
	TerminationMessagePathDefault string = "/dev/termination-log"
)

// A single application container that you want to run within a pod.
type Container struct {
	// Name of the container specified as a DNS_LABEL.
	// Each container in a pod must have a unique name (DNS_LABEL).
	// Cannot be updated.
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// Docker image name.
	// More info: https://kubernetes.io/docs/concepts/containers/images
	// This field is optional to allow higher level config management to default or override
	// container images in workload controllers like Deployments and StatefulSets.
	// +optional
	Image string `json:"image,omitempty" protobuf:"bytes,2,opt,name=image"`
	// Entrypoint array. Not executed within a shell.
	// The docker image's ENTRYPOINT is used if this is not provided.
	// Variable references $(VAR_NAME) are expanded using the container's environment. If a variable
	// cannot be resolved, the reference in the input string will be unchanged. The $(VAR_NAME) syntax
	// can be escaped with a double $$, ie: $$(VAR_NAME). Escaped references will never be expanded,
	// regardless of whether the variable exists or not.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// +optional
	Command []string `json:"command,omitempty" protobuf:"bytes,3,rep,name=command"`
	// Arguments to the entrypoint.
	// The docker image's CMD is used if this is not provided.
	// Variable references $(VAR_NAME) are expanded using the container's environment. If a variable
	// cannot be resolved, the reference in the input string will be unchanged. The $(VAR_NAME) syntax
	// can be escaped with a double $$, ie: $$(VAR_NAME). Escaped references will never be expanded,
	// regardless of whether the variable exists or not.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// +optional
	Args []string `json:"args,omitempty" protobuf:"bytes,4,rep,name=args"`
	// Container's working directory.
	// If not specified, the container runtime's default will be used, which
	// might be configured in the container image.
	// Cannot be updated.
	// +optional
	WorkingDir string `json:"workingDir,omitempty" protobuf:"bytes,5,opt,name=workingDir"`
	// List of ports to expose from the container. Exposing a port here gives
	// the system additional information about the network connections a
	// container uses, but is primarily informational. Not specifying a port here
	// DOES NOT prevent that port from being exposed. Any port which is
	// listening on the default "0.0.0.0" address inside a container will be
	// accessible from the network.
	// Cannot be updated.
	// +optional
	// +patchMergeKey=containerPort
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=containerPort
	// +listMapKey=protocol
	Ports []ContainerPort `json:"ports,omitempty" patchStrategy:"merge" patchMergeKey:"containerPort" protobuf:"bytes,6,rep,name=ports"`
	// List of sources to populate environment variables in the container.
	// The keys defined within a source must be a C_IDENTIFIER. All invalid keys
	// will be reported as an event when the container is starting. When a key exists in multiple
	// sources, the value associated with the last source will take precedence.
	// Values defined by an Env with a duplicate key will take precedence.
	// Cannot be updated.
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty" protobuf:"bytes,19,rep,name=envFrom"`
	// List of environment variables to set in the container.
	// Cannot be updated.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Env []EnvVar `json:"env,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,7,rep,name=env"`
	// Compute Resources required by this container.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,8,opt,name=resources"`
	// Pod volumes to mount into the container's filesystem.
	// Cannot be updated.
	// +optional
	// +patchMergeKey=mountPath
	// +patchStrategy=merge
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty" patchStrategy:"merge" patchMergeKey:"mountPath" protobuf:"bytes,9,rep,name=volumeMounts"`
	// volumeDevices is the list of block devices to be used by the container.
	// +patchMergeKey=devicePath
	// +patchStrategy=merge
	// +optional
	VolumeDevices []VolumeDevice `json:"volumeDevices,omitempty" patchStrategy:"merge" patchMergeKey:"devicePath" protobuf:"bytes,21,rep,name=volumeDevices"`
	// Periodic probe of container liveness.
	// Container will be restarted if the probe fails.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	LivenessProbe *Probe `json:"livenessProbe,omitempty" protobuf:"bytes,10,opt,name=livenessProbe"`
	// Periodic probe of container service readiness.
	// Container will be removed from service endpoints if the probe fails.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	ReadinessProbe *Probe `json:"readinessProbe,omitempty" protobuf:"bytes,11,opt,name=readinessProbe"`
	// StartupProbe indicates that the Pod has successfully initialized.
	// If specified, no other probes are executed until this completes successfully.
	// If this probe fails, the Pod will be restarted, just as if the livenessProbe failed.
	// This can be used to provide different probe parameters at the beginning of a Pod's lifecycle,
	// when it might take a long time to load data or warm a cache, than during steady-state operation.
	// This cannot be updated.
	// This is a beta feature enabled by the StartupProbe feature flag.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#container-probes
	// +optional
	StartupProbe *Probe `json:"startupProbe,omitempty" protobuf:"bytes,22,opt,name=startupProbe"`
	// Actions that the management system should take in response to container lifecycle events.
	// Cannot be updated.
	// +optional
	Lifecycle *Lifecycle `json:"lifecycle,omitempty" protobuf:"bytes,12,opt,name=lifecycle"`
	// Optional: Path at which the file to which the container's termination message
	// will be written is mounted into the container's filesystem.
	// Message written is intended to be brief final status, such as an assertion failure message.
	// Will be truncated by the node if greater than 4096 bytes. The total message length across
	// all containers will be limited to 12kb.
	// Defaults to /dev/termination-log.
	// Cannot be updated.
	// +optional
	TerminationMessagePath string `json:"terminationMessagePath,omitempty" protobuf:"bytes,13,opt,name=terminationMessagePath"`
	// Indicate how the termination message should be populated. File will use the contents of
	// terminationMessagePath to populate the container status message on both success and failure.
	// FallbackToLogsOnError will use the last chunk of container log output if the termination
	// message file is empty and the container exited with an error.
	// The log output is limited to 2048 bytes or 80 lines, whichever is smaller.
	// Defaults to File.
	// Cannot be updated.
	// +optional
	TerminationMessagePolicy TerminationMessagePolicy `json:"terminationMessagePolicy,omitempty" protobuf:"bytes,20,opt,name=terminationMessagePolicy,casttype=TerminationMessagePolicy"`
	// Image pull policy.
	// One of Always, Never, IfNotPresent.
	// Defaults to Always if :latest tag is specified, or IfNotPresent otherwise.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/containers/images#updating-images
	// +optional
	ImagePullPolicy PullPolicy `json:"imagePullPolicy,omitempty" protobuf:"bytes,14,opt,name=imagePullPolicy,casttype=PullPolicy"`
	// Security options the pod should run with.
	// More info: https://kubernetes.io/docs/concepts/policy/security-context/
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/security-context/
	// +optional
	SecurityContext *SecurityContext `json:"securityContext,omitempty" protobuf:"bytes,15,opt,name=securityContext"`

	// Variables for interactive containers, these have very specialized use-cases (e.g. debugging)
	// and shouldn't be used for general purpose containers.

	// Whether this container should allocate a buffer for stdin in the container runtime. If this
	// is not set, reads from stdin in the container will always result in EOF.
	// Default is false.
	// +optional
	Stdin bool `json:"stdin,omitempty" protobuf:"varint,16,opt,name=stdin"`
	// Whether the container runtime should close the stdin channel after it has been opened by
	// a single attach. When stdin is true the stdin stream will remain open across multiple attach
	// sessions. If stdinOnce is set to true, stdin is opened on container start, is empty until the
	// first client attaches to stdin, and then remains open and accepts data until the client disconnects,
	// at which time stdin is closed and remains closed until the container is restarted. If this
	// flag is false, a container processes that reads from stdin will never receive an EOF.
	// Default is false
	// +optional
	StdinOnce bool `json:"stdinOnce,omitempty" protobuf:"varint,17,opt,name=stdinOnce"`
	// Whether this container should allocate a TTY for itself, also requires 'stdin' to be true.
	// Default is false.
	// +optional
	TTY bool `json:"tty,omitempty" protobuf:"varint,18,opt,name=tty"`
}

// Handler defines a specific action that should be taken
// TODO: pass structured data to these actions, and document that data here.
type Handler struct {
	// One and only one of the following should be specified.
	// Exec specifies the action to take.
	// +optional
	Exec *ExecAction `json:"exec,omitempty" protobuf:"bytes,1,opt,name=exec"`
	// HTTPGet specifies the http request to perform.
	// +optional
	HTTPGet *HTTPGetAction `json:"httpGet,omitempty" protobuf:"bytes,2,opt,name=httpGet"`
	// TCPSocket specifies an action involving a TCP port.
	// TCP hooks not yet supported
	// TODO: implement a realistic TCP lifecycle hook
	// +optional
	TCPSocket *TCPSocketAction `json:"tcpSocket,omitempty" protobuf:"bytes,3,opt,name=tcpSocket"`
}

// Lifecycle describes actions that the management system should take in response to container lifecycle
// events. For the PostStart and PreStop lifecycle handlers, management of the container blocks
// until the action is complete, unless the container process fails, in which case the handler is aborted.
type Lifecycle struct {
	// PostStart is called immediately after a container is created. If the handler fails,
	// the container is terminated and restarted according to its restart policy.
	// Other management of the container blocks until the hook completes.
	// More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks
	// +optional
	PostStart *Handler `json:"postStart,omitempty" protobuf:"bytes,1,opt,name=postStart"`
	// PreStop is called immediately before a container is terminated due to an
	// API request or management event such as liveness/startup probe failure,
	// preemption, resource contention, etc. The handler is not called if the
	// container crashes or exits. The reason for termination is passed to the
	// handler. The Pod's termination grace period countdown begins before the
	// PreStop hooked is executed. Regardless of the outcome of the handler, the
	// container will eventually terminate within the Pod's termination grace
	// period. Other management of the container blocks until the hook completes
	// or until the termination grace period is reached.
	// More info: https://kubernetes.io/docs/concepts/containers/container-lifecycle-hooks/#container-hooks
	// +optional
	PreStop *Handler `json:"preStop,omitempty" protobuf:"bytes,2,opt,name=preStop"`
}

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition.
// "ConditionFalse" means a resource is not in the condition. "ConditionUnknown" means kubernetes
// can't decide if a resource is in the condition or not. In the future, we could add other
// intermediate conditions, e.g. ConditionDegraded.
const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

// ContainerStateWaiting is a waiting state of a container.
type ContainerStateWaiting struct {
	// (brief) reason the container is not yet running.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,1,opt,name=reason"`
	// Message regarding why the container is not yet running.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,2,opt,name=message"`
}

// ContainerStateRunning is a running state of a container.
type ContainerStateRunning struct {
	// Time at which the container was last (re-)started
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty" protobuf:"bytes,1,opt,name=startedAt"`
}

// ContainerStateTerminated is a terminated state of a container.
type ContainerStateTerminated struct {
	// Exit status from the last termination of the container
	ExitCode int32 `json:"exitCode" protobuf:"varint,1,opt,name=exitCode"`
	// Signal from the last termination of the container
	// +optional
	Signal int32 `json:"signal,omitempty" protobuf:"varint,2,opt,name=signal"`
	// (brief) reason from the last termination of the container
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
	// Message regarding the last termination of the container
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,4,opt,name=message"`
	// Time at which previous execution of the container started
	// +optional
	StartedAt metav1.Time `json:"startedAt,omitempty" protobuf:"bytes,5,opt,name=startedAt"`
	// Time at which the container last terminated
	// +optional
	FinishedAt metav1.Time `json:"finishedAt,omitempty" protobuf:"bytes,6,opt,name=finishedAt"`
	// Container's ID in the format 'docker://<container_id>'
	// +optional
	ContainerID string `json:"containerID,omitempty" protobuf:"bytes,7,opt,name=containerID"`
}

// ContainerState holds a possible state of container.
// Only one of its members may be specified.
// If none of them is specified, the default one is ContainerStateWaiting.
type ContainerState struct {
	// Details about a waiting container
	// +optional
	Waiting *ContainerStateWaiting `json:"waiting,omitempty" protobuf:"bytes,1,opt,name=waiting"`
	// Details about a running container
	// +optional
	Running *ContainerStateRunning `json:"running,omitempty" protobuf:"bytes,2,opt,name=running"`
	// Details about a terminated container
	// +optional
	Terminated *ContainerStateTerminated `json:"terminated,omitempty" protobuf:"bytes,3,opt,name=terminated"`
}

// ContainerStatus contains details for the current status of this container.
type ContainerStatus struct {
	// This must be a DNS_LABEL. Each container in a pod must have a unique name.
	// Cannot be updated.
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// Details about the container's current condition.
	// +optional
	State ContainerState `json:"state,omitempty" protobuf:"bytes,2,opt,name=state"`
	// Details about the container's last termination condition.
	// +optional
	LastTerminationState ContainerState `json:"lastState,omitempty" protobuf:"bytes,3,opt,name=lastState"`
	// Specifies whether the container has passed its readiness probe.
	Ready bool `json:"ready" protobuf:"varint,4,opt,name=ready"`
	// The number of times the container has been restarted, currently based on
	// the number of dead containers that have not yet been removed.
	// Note that this is calculated from dead containers. But those containers are subject to
	// garbage collection. This value will get capped at 5 by GC.
	RestartCount int32 `json:"restartCount" protobuf:"varint,5,opt,name=restartCount"`
	// The image the container is running.
	// More info: https://kubernetes.io/docs/concepts/containers/images
	// TODO(dchen1107): Which image the container is running with?
	Image string `json:"image" protobuf:"bytes,6,opt,name=image"`
	// ImageID of the container's image.
	ImageID string `json:"imageID" protobuf:"bytes,7,opt,name=imageID"`
	// Container's ID in the format 'docker://<container_id>'.
	// +optional
	ContainerID string `json:"containerID,omitempty" protobuf:"bytes,8,opt,name=containerID"`
	// Specifies whether the container has passed its startup probe.
	// Initialized as false, becomes true after startupProbe is considered successful.
	// Resets to false when the container is restarted, or if kubelet loses state temporarily.
	// Is always true when no startupProbe is defined.
	// +optional
	Started *bool `json:"started,omitempty" protobuf:"varint,9,opt,name=started"`
}

// PodPhase is a label for the condition of a pod at the current time.
type PodPhase string

// These are the valid statuses of pods.
const (
	// PodPending means the pod has been accepted by the system, but one or more of the containers
	// has not been started. This includes time before being bound to a node, as well as time spent
	// pulling images onto the host.
	PodPending PodPhase = "Pending"
	// PodRunning means the pod has been bound to a node and all of the containers have been started.
	// At least one container is still running or is in the process of being restarted.
	PodRunning PodPhase = "Running"
	// PodSucceeded means that all containers in the pod have voluntarily terminated
	// with a container exit code of 0, and the system is not going to restart any of these containers.
	PodSucceeded PodPhase = "Succeeded"
	// PodFailed means that all containers in the pod have terminated, and at least one container has
	// terminated in a failure (exited with a non-zero exit code or was stopped by the system).
	PodFailed PodPhase = "Failed"
	// PodUnknown means that for some reason the state of the pod could not be obtained, typically due
	// to an error in communicating with the host of the pod.
	PodUnknown PodPhase = "Unknown"
)

// PodConditionType is a valid value for PodCondition.Type
type PodConditionType string

// These are valid conditions of pod.
const (
	// ContainersReady indicates whether all containers in the pod are ready.
	ContainersReady PodConditionType = "ContainersReady"
	// PodInitialized means that all init containers in the pod have started successfully.
	PodInitialized PodConditionType = "Initialized"
	// PodReady means the pod is able to service requests and should be added to the
	// load balancing pools of all matching services.
	PodReady PodConditionType = "Ready"
	// PodScheduled represents status of the scheduling process for this pod.
	PodScheduled PodConditionType = "PodScheduled"
)

// These are reasons for a pod's transition to a condition.
const (
	// PodReasonUnschedulable reason in PodScheduled PodCondition means that the scheduler
	// can't schedule the pod right now, for example due to insufficient resources in the cluster.
	PodReasonUnschedulable = "Unschedulable"
)

// PodCondition contains details for the current condition of this pod.
type PodCondition struct {
	// Type is the type of the condition.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-conditions
	Type PodConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=PodConditionType"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-conditions
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// Last time we probed the condition.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty" protobuf:"bytes,3,opt,name=lastProbeTime"`
	// Last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// Unique, one-word, CamelCase reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// Human-readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
}

// RestartPolicy describes how the container should be restarted.
// Only one of the following restart policies may be specified.
// If none of the following policies is specified, the default one
// is RestartPolicyAlways.
type RestartPolicy string

const (
	RestartPolicyAlways    RestartPolicy = "Always"
	RestartPolicyOnFailure RestartPolicy = "OnFailure"
	RestartPolicyNever     RestartPolicy = "Never"
)

// DNSPolicy defines how a pod's DNS will be configured.
type DNSPolicy string

const (
	// DNSClusterFirstWithHostNet indicates that the pod should use cluster DNS
	// first, if it is available, then fall back on the default
	// (as determined by kubelet) DNS settings.
	DNSClusterFirstWithHostNet DNSPolicy = "ClusterFirstWithHostNet"

	// DNSClusterFirst indicates that the pod should use cluster DNS
	// first unless hostNetwork is true, if it is available, then
	// fall back on the default (as determined by kubelet) DNS settings.
	DNSClusterFirst DNSPolicy = "ClusterFirst"

	// DNSDefault indicates that the pod should use the default (as
	// determined by kubelet) DNS settings.
	DNSDefault DNSPolicy = "Default"

	// DNSNone indicates that the pod should use empty DNS settings. DNS
	// parameters such as nameservers and search paths should be defined via
	// DNSConfig.
	DNSNone DNSPolicy = "None"
)

const (
	// DefaultTerminationGracePeriodSeconds indicates the default duration in
	// seconds a pod needs to terminate gracefully.
	DefaultTerminationGracePeriodSeconds = 30
)

// A node selector represents the union of the results of one or more label queries
// over a set of nodes; that is, it represents the OR of the selectors represented
// by the node selector terms.
type NodeSelector struct {
	//Required. A list of node selector terms. The terms are ORed.
	NodeSelectorTerms []NodeSelectorTerm `json:"nodeSelectorTerms" protobuf:"bytes,1,rep,name=nodeSelectorTerms"`
}

// A null or empty node selector term matches no objects. The requirements of
// them are ANDed.
// The TopologySelectorTerm type implements a subset of the NodeSelectorTerm.
type NodeSelectorTerm struct {
	// A list of node selector requirements by node's labels.
	// +optional
	MatchExpressions []NodeSelectorRequirement `json:"matchExpressions,omitempty" protobuf:"bytes,1,rep,name=matchExpressions"`
	// A list of node selector requirements by node's fields.
	// +optional
	MatchFields []NodeSelectorRequirement `json:"matchFields,omitempty" protobuf:"bytes,2,rep,name=matchFields"`
}

// A node selector requirement is a selector that contains values, a key, and an operator
// that relates the key and values.
type NodeSelectorRequirement struct {
	// The label key that the selector applies to.
	Key string `json:"key" protobuf:"bytes,1,opt,name=key"`
	// Represents a key's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist. Gt, and Lt.
	Operator NodeSelectorOperator `json:"operator" protobuf:"bytes,2,opt,name=operator,casttype=NodeSelectorOperator"`
	// An array of string values. If the operator is In or NotIn,
	// the values array must be non-empty. If the operator is Exists or DoesNotExist,
	// the values array must be empty. If the operator is Gt or Lt, the values
	// array must have a single element, which will be interpreted as an integer.
	// This array is replaced during a strategic merge patch.
	// +optional
	Values []string `json:"values,omitempty" protobuf:"bytes,3,rep,name=values"`
}

// A node selector operator is the set of operators that can be used in
// a node selector requirement.
type NodeSelectorOperator string

const (
	NodeSelectorOpIn           NodeSelectorOperator = "In"
	NodeSelectorOpNotIn        NodeSelectorOperator = "NotIn"
	NodeSelectorOpExists       NodeSelectorOperator = "Exists"
	NodeSelectorOpDoesNotExist NodeSelectorOperator = "DoesNotExist"
	NodeSelectorOpGt           NodeSelectorOperator = "Gt"
	NodeSelectorOpLt           NodeSelectorOperator = "Lt"
)

// A topology selector term represents the result of label queries.
// A null or empty topology selector term matches no objects.
// The requirements of them are ANDed.
// It provides a subset of functionality as NodeSelectorTerm.
// This is an alpha feature and may change in the future.
type TopologySelectorTerm struct {
	// A list of topology selector requirements by labels.
	// +optional
	MatchLabelExpressions []TopologySelectorLabelRequirement `json:"matchLabelExpressions,omitempty" protobuf:"bytes,1,rep,name=matchLabelExpressions"`
}

// A topology selector requirement is a selector that matches given label.
// This is an alpha feature and may change in the future.
type TopologySelectorLabelRequirement struct {
	// The label key that the selector applies to.
	Key string `json:"key" protobuf:"bytes,1,opt,name=key"`
	// An array of string values. One value must match the label to be selected.
	// Each entry in Values is ORed.
	Values []string `json:"values" protobuf:"bytes,2,rep,name=values"`
}

// Affinity is a group of affinity scheduling rules.
type Affinity struct {
	// Describes node affinity scheduling rules for the pod.
	// +optional
	NodeAffinity *NodeAffinity `json:"nodeAffinity,omitempty" protobuf:"bytes,1,opt,name=nodeAffinity"`
	// Describes pod affinity scheduling rules (e.g. co-locate this pod in the same node, zone, etc. as some other pod(s)).
	// +optional
	PodAffinity *PodAffinity `json:"podAffinity,omitempty" protobuf:"bytes,2,opt,name=podAffinity"`
	// Describes pod anti-affinity scheduling rules (e.g. avoid putting this pod in the same node, zone, etc. as some other pod(s)).
	// +optional
	PodAntiAffinity *PodAntiAffinity `json:"podAntiAffinity,omitempty" protobuf:"bytes,3,opt,name=podAntiAffinity"`
}

// Pod affinity is a group of inter pod affinity scheduling rules.
type PodAffinity struct {
	// NOT YET IMPLEMENTED. TODO: Uncomment field once it is implemented.
	// If the affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to a pod label update), the
	// system will try to eventually evict the pod from its node.
	// When there are multiple elements, the lists of nodes corresponding to each
	// podAffinityTerm are intersected, i.e. all terms must be satisfied.
	// +optional
	// RequiredDuringSchedulingRequiredDuringExecution []PodAffinityTerm  `json:"requiredDuringSchedulingRequiredDuringExecution,omitempty"`

	// If the affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to a pod label update), the
	// system may or may not try to eventually evict the pod from its node.
	// When there are multiple elements, the lists of nodes corresponding to each
	// podAffinityTerm are intersected, i.e. all terms must be satisfied.
	// +optional
	RequiredDuringSchedulingIgnoredDuringExecution []PodAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" protobuf:"bytes,1,rep,name=requiredDuringSchedulingIgnoredDuringExecution"`
	// The scheduler will prefer to schedule pods to nodes that satisfy
	// the affinity expressions specified by this field, but it may choose
	// a node that violates one or more of the expressions. The node that is
	// most preferred is the one with the greatest sum of weights, i.e.
	// for each node that meets all of the scheduling requirements (resource
	// request, requiredDuringScheduling affinity expressions, etc.),
	// compute a sum by iterating through the elements of this field and adding
	// "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the
	// node(s) with the highest sum are the most preferred.
	// +optional
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" protobuf:"bytes,2,rep,name=preferredDuringSchedulingIgnoredDuringExecution"`
}

// Pod anti affinity is a group of inter pod anti affinity scheduling rules.
type PodAntiAffinity struct {
	// NOT YET IMPLEMENTED. TODO: Uncomment field once it is implemented.
	// If the anti-affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the anti-affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to a pod label update), the
	// system will try to eventually evict the pod from its node.
	// When there are multiple elements, the lists of nodes corresponding to each
	// podAffinityTerm are intersected, i.e. all terms must be satisfied.
	// +optional
	// RequiredDuringSchedulingRequiredDuringExecution []PodAffinityTerm  `json:"requiredDuringSchedulingRequiredDuringExecution,omitempty"`

	// If the anti-affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the anti-affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to a pod label update), the
	// system may or may not try to eventually evict the pod from its node.
	// When there are multiple elements, the lists of nodes corresponding to each
	// podAffinityTerm are intersected, i.e. all terms must be satisfied.
	// +optional
	RequiredDuringSchedulingIgnoredDuringExecution []PodAffinityTerm `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" protobuf:"bytes,1,rep,name=requiredDuringSchedulingIgnoredDuringExecution"`
	// The scheduler will prefer to schedule pods to nodes that satisfy
	// the anti-affinity expressions specified by this field, but it may choose
	// a node that violates one or more of the expressions. The node that is
	// most preferred is the one with the greatest sum of weights, i.e.
	// for each node that meets all of the scheduling requirements (resource
	// request, requiredDuringScheduling anti-affinity expressions, etc.),
	// compute a sum by iterating through the elements of this field and adding
	// "weight" to the sum if the node has pods which matches the corresponding podAffinityTerm; the
	// node(s) with the highest sum are the most preferred.
	// +optional
	PreferredDuringSchedulingIgnoredDuringExecution []WeightedPodAffinityTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" protobuf:"bytes,2,rep,name=preferredDuringSchedulingIgnoredDuringExecution"`
}

// The weights of all of the matched WeightedPodAffinityTerm fields are added per-node to find the most preferred node(s)
type WeightedPodAffinityTerm struct {
	// weight associated with matching the corresponding podAffinityTerm,
	// in the range 1-100.
	Weight int32 `json:"weight" protobuf:"varint,1,opt,name=weight"`
	// Required. A pod affinity term, associated with the corresponding weight.
	PodAffinityTerm PodAffinityTerm `json:"podAffinityTerm" protobuf:"bytes,2,opt,name=podAffinityTerm"`
}

// Defines a set of pods (namely those matching the labelSelector
// relative to the given namespace(s)) that this pod should be
// co-located (affinity) or not co-located (anti-affinity) with,
// where co-located is defined as running on a node whose value of
// the label with key <topologyKey> matches that of any node on which
// a pod of the set of pods is running
type PodAffinityTerm struct {
	// A label query over a set of resources, in this case pods.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty" protobuf:"bytes,1,opt,name=labelSelector"`
	// namespaces specifies which namespaces the labelSelector applies to (matches against);
	// null or empty list means "this pod's namespace"
	// +optional
	Namespaces []string `json:"namespaces,omitempty" protobuf:"bytes,2,rep,name=namespaces"`
	// This pod should be co-located (affinity) or not co-located (anti-affinity) with the pods matching
	// the labelSelector in the specified namespaces, where co-located is defined as running on a node
	// whose value of the label with key topologyKey matches that of any node on which any of the
	// selected pods is running.
	// Empty topologyKey is not allowed.
	TopologyKey string `json:"topologyKey" protobuf:"bytes,3,opt,name=topologyKey"`
}

// Node affinity is a group of node affinity scheduling rules.
type NodeAffinity struct {
	// NOT YET IMPLEMENTED. TODO: Uncomment field once it is implemented.
	// If the affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to an update), the system
	// will try to eventually evict the pod from its node.
	// +optional
	// RequiredDuringSchedulingRequiredDuringExecution *NodeSelector `json:"requiredDuringSchedulingRequiredDuringExecution,omitempty"`

	// If the affinity requirements specified by this field are not met at
	// scheduling time, the pod will not be scheduled onto the node.
	// If the affinity requirements specified by this field cease to be met
	// at some point during pod execution (e.g. due to an update), the system
	// may or may not try to eventually evict the pod from its node.
	// +optional
	RequiredDuringSchedulingIgnoredDuringExecution *NodeSelector `json:"requiredDuringSchedulingIgnoredDuringExecution,omitempty" protobuf:"bytes,1,opt,name=requiredDuringSchedulingIgnoredDuringExecution"`
	// The scheduler will prefer to schedule pods to nodes that satisfy
	// the affinity expressions specified by this field, but it may choose
	// a node that violates one or more of the expressions. The node that is
	// most preferred is the one with the greatest sum of weights, i.e.
	// for each node that meets all of the scheduling requirements (resource
	// request, requiredDuringScheduling affinity expressions, etc.),
	// compute a sum by iterating through the elements of this field and adding
	// "weight" to the sum if the node matches the corresponding matchExpressions; the
	// node(s) with the highest sum are the most preferred.
	// +optional
	PreferredDuringSchedulingIgnoredDuringExecution []PreferredSchedulingTerm `json:"preferredDuringSchedulingIgnoredDuringExecution,omitempty" protobuf:"bytes,2,rep,name=preferredDuringSchedulingIgnoredDuringExecution"`
}

// An empty preferred scheduling term matches all objects with implicit weight 0
// (i.e. it's a no-op). A null preferred scheduling term matches no objects (i.e. is also a no-op).
type PreferredSchedulingTerm struct {
	// Weight associated with matching the corresponding nodeSelectorTerm, in the range 1-100.
	Weight int32 `json:"weight" protobuf:"varint,1,opt,name=weight"`
	// A node selector term, associated with the corresponding weight.
	Preference NodeSelectorTerm `json:"preference" protobuf:"bytes,2,opt,name=preference"`
}

// The node this Taint is attached to has the "effect" on
// any pod that does not tolerate the Taint.
type Taint struct {
	// Required. The taint key to be applied to a node.
	Key string `json:"key" protobuf:"bytes,1,opt,name=key"`
	// The taint value corresponding to the taint key.
	// +optional
	Value string `json:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
	// Required. The effect of the taint on pods
	// that do not tolerate the taint.
	// Valid effects are NoSchedule, PreferNoSchedule and NoExecute.
	Effect TaintEffect `json:"effect" protobuf:"bytes,3,opt,name=effect,casttype=TaintEffect"`
	// TimeAdded represents the time at which the taint was added.
	// It is only written for NoExecute taints.
	// +optional
	TimeAdded *metav1.Time `json:"timeAdded,omitempty" protobuf:"bytes,4,opt,name=timeAdded"`
}

type TaintEffect string

const (
	// Do not allow new pods to schedule onto the node unless they tolerate the taint,
	// but allow all pods submitted to Kubelet without going through the scheduler
	// to start, and allow all already-running pods to continue running.
	// Enforced by the scheduler.
	TaintEffectNoSchedule TaintEffect = "NoSchedule"
	// Like TaintEffectNoSchedule, but the scheduler tries not to schedule
	// new pods onto the node, rather than prohibiting new pods from scheduling
	// onto the node entirely. Enforced by the scheduler.
	TaintEffectPreferNoSchedule TaintEffect = "PreferNoSchedule"
	// NOT YET IMPLEMENTED. TODO: Uncomment field once it is implemented.
	// Like TaintEffectNoSchedule, but additionally do not allow pods submitted to
	// Kubelet without going through the scheduler to start.
	// Enforced by Kubelet and the scheduler.
	// TaintEffectNoScheduleNoAdmit TaintEffect = "NoScheduleNoAdmit"

	// Evict any already-running pods that do not tolerate the taint.
	// Currently enforced by NodeController.
	TaintEffectNoExecute TaintEffect = "NoExecute"
)

// The pod this Toleration is attached to tolerates any taint that matches
// the triple <key,value,effect> using the matching operator <operator>.
type Toleration struct {
	// Key is the taint key that the toleration applies to. Empty means match all taint keys.
	// If the key is empty, operator must be Exists; this combination means to match all values and all keys.
	// +optional
	Key string `json:"key,omitempty" protobuf:"bytes,1,opt,name=key"`
	// Operator represents a key's relationship to the value.
	// Valid operators are Exists and Equal. Defaults to Equal.
	// Exists is equivalent to wildcard for value, so that a pod can
	// tolerate all taints of a particular category.
	// +optional
	Operator TolerationOperator `json:"operator,omitempty" protobuf:"bytes,2,opt,name=operator,casttype=TolerationOperator"`
	// Value is the taint value the toleration matches to.
	// If the operator is Exists, the value should be empty, otherwise just a regular string.
	// +optional
	Value string `json:"value,omitempty" protobuf:"bytes,3,opt,name=value"`
	// Effect indicates the taint effect to match. Empty means match all taint effects.
	// When specified, allowed values are NoSchedule, PreferNoSchedule and NoExecute.
	// +optional
	Effect TaintEffect `json:"effect,omitempty" protobuf:"bytes,4,opt,name=effect,casttype=TaintEffect"`
	// TolerationSeconds represents the period of time the toleration (which must be
	// of effect NoExecute, otherwise this field is ignored) tolerates the taint. By default,
	// it is not set, which means tolerate the taint forever (do not evict). Zero and
	// negative values will be treated as 0 (evict immediately) by the system.
	// +optional
	TolerationSeconds *int64 `json:"tolerationSeconds,omitempty" protobuf:"varint,5,opt,name=tolerationSeconds"`
}

// A toleration operator is the set of operators that can be used in a toleration.
type TolerationOperator string

const (
	TolerationOpExists TolerationOperator = "Exists"
	TolerationOpEqual  TolerationOperator = "Equal"
)

// PodReadinessGate contains the reference to a pod condition
type PodReadinessGate struct {
	// ConditionType refers to a condition in the pod's condition list with matching type.
	ConditionType PodConditionType `json:"conditionType" protobuf:"bytes,1,opt,name=conditionType,casttype=PodConditionType"`
}

// PodSpec is a description of a pod.
type PodSpec struct {
	// List of volumes that can be mounted by containers belonging to the pod.
	// More info: https://kubernetes.io/docs/concepts/storage/volumes
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge,retainKeys
	Volumes []Volume `json:"volumes,omitempty" patchStrategy:"merge,retainKeys" patchMergeKey:"name" protobuf:"bytes,1,rep,name=volumes"`
	// List of initialization containers belonging to the pod.
	// Init containers are executed in order prior to containers being started. If any
	// init container fails, the pod is considered to have failed and is handled according
	// to its restartPolicy. The name for an init container or normal container must be
	// unique among all containers.
	// Init containers may not have Lifecycle actions, Readiness probes, Liveness probes, or Startup probes.
	// The resourceRequirements of an init container are taken into account during scheduling
	// by finding the highest request/limit for each resource type, and then using the max of
	// of that value or the sum of the normal containers. Limits are applied to init containers
	// in a similar fashion.
	// Init containers cannot currently be added or removed.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/init-containers/
	// +patchMergeKey=name
	// +patchStrategy=merge
	InitContainers []Container `json:"initContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,20,rep,name=initContainers"`
	// List of containers belonging to the pod.
	// Containers cannot currently be added or removed.
	// There must be at least one container in a Pod.
	// Cannot be updated.
	// +patchMergeKey=name
	// +patchStrategy=merge
	Containers []Container `json:"containers" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=containers"`
	// List of ephemeral containers run in this pod. Ephemeral containers may be run in an existing
	// pod to perform user-initiated actions such as debugging. This list cannot be specified when
	// creating a pod, and it cannot be modified by updating the pod spec. In order to add an
	// ephemeral container to an existing pod, use the pod's ephemeralcontainers subresource.
	// This field is alpha-level and is only honored by servers that enable the EphemeralContainers feature.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	EphemeralContainers []EphemeralContainer `json:"ephemeralContainers,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,34,rep,name=ephemeralContainers"`
	// Restart policy for all containers within the pod.
	// One of Always, OnFailure, Never.
	// Default to Always.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#restart-policy
	// +optional
	RestartPolicy RestartPolicy `json:"restartPolicy,omitempty" protobuf:"bytes,3,opt,name=restartPolicy,casttype=RestartPolicy"`
	// Optional duration in seconds the pod needs to terminate gracefully. May be decreased in delete request.
	// Value must be non-negative integer. The value zero indicates delete immediately.
	// If this value is nil, the default grace period will be used instead.
	// The grace period is the duration in seconds after the processes running in the pod are sent
	// a termination signal and the time when the processes are forcibly halted with a kill signal.
	// Set this value longer than the expected cleanup time for your process.
	// Defaults to 30 seconds.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty" protobuf:"varint,4,opt,name=terminationGracePeriodSeconds"`
	// Optional duration in seconds the pod may be active on the node relative to
	// StartTime before the system will actively try to mark it failed and kill associated containers.
	// Value must be a positive integer.
	// +optional
	ActiveDeadlineSeconds *int64 `json:"activeDeadlineSeconds,omitempty" protobuf:"varint,5,opt,name=activeDeadlineSeconds"`
	// Set DNS policy for the pod.
	// Defaults to "ClusterFirst".
	// Valid values are 'ClusterFirstWithHostNet', 'ClusterFirst', 'Default' or 'None'.
	// DNS parameters given in DNSConfig will be merged with the policy selected with DNSPolicy.
	// To have DNS options set along with hostNetwork, you have to specify DNS policy
	// explicitly to 'ClusterFirstWithHostNet'.
	// +optional
	DNSPolicy DNSPolicy `json:"dnsPolicy,omitempty" protobuf:"bytes,6,opt,name=dnsPolicy,casttype=DNSPolicy"`
	// NodeSelector is a selector which must be true for the pod to fit on a node.
	// Selector which must match a node's labels for the pod to be scheduled on that node.
	// More info: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty" protobuf:"bytes,7,rep,name=nodeSelector"`

	// ServiceAccountName is the name of the ServiceAccount to use to run this pod.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty" protobuf:"bytes,8,opt,name=serviceAccountName"`
	// DeprecatedServiceAccount is a depreciated alias for ServiceAccountName.
	// Deprecated: Use serviceAccountName instead.
	// +k8s:conversion-gen=false
	// +optional
	DeprecatedServiceAccount string `json:"serviceAccount,omitempty" protobuf:"bytes,9,opt,name=serviceAccount"`
	// AutomountServiceAccountToken indicates whether a service account token should be automatically mounted.
	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty" protobuf:"varint,21,opt,name=automountServiceAccountToken"`

	// NodeName is a request to schedule this pod onto a specific node. If it is non-empty,
	// the scheduler simply schedules this pod onto that node, assuming that it fits resource
	// requirements.
	// +optional
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
	// Host networking requested for this pod. Use the host's network namespace.
	// If this option is set, the ports that will be used must be specified.
	// Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostNetwork bool `json:"hostNetwork,omitempty" protobuf:"varint,11,opt,name=hostNetwork"`
	// Use the host's pid namespace.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostPID bool `json:"hostPID,omitempty" protobuf:"varint,12,opt,name=hostPID"`
	// Use the host's ipc namespace.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	HostIPC bool `json:"hostIPC,omitempty" protobuf:"varint,13,opt,name=hostIPC"`
	// Share a single process namespace between all of the containers in a pod.
	// When this is set containers will be able to view and signal processes from other containers
	// in the same pod, and the first process in each container will not be assigned PID 1.
	// HostPID and ShareProcessNamespace cannot both be set.
	// Optional: Default to false.
	// +k8s:conversion-gen=false
	// +optional
	ShareProcessNamespace *bool `json:"shareProcessNamespace,omitempty" protobuf:"varint,27,opt,name=shareProcessNamespace"`
	// SecurityContext holds pod-level security attributes and common container settings.
	// Optional: Defaults to empty.  See type description for default values of each field.
	// +optional
	SecurityContext *PodSecurityContext `json:"securityContext,omitempty" protobuf:"bytes,14,opt,name=securityContext"`
	// ImagePullSecrets is an optional list of references to secrets in the same namespace to use for pulling any of the images used by this PodSpec.
	// If specified, these secrets will be passed to individual puller implementations for them to use. For example,
	// in the case of docker, only DockerConfig type secrets are honored.
	// More info: https://kubernetes.io/docs/concepts/containers/images#specifying-imagepullsecrets-on-a-pod
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	ImagePullSecrets []LocalObjectReference `json:"imagePullSecrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,15,rep,name=imagePullSecrets"`
	// Specifies the hostname of the Pod
	// If not specified, the pod's hostname will be set to a system-defined value.
	// +optional
	Hostname string `json:"hostname,omitempty" protobuf:"bytes,16,opt,name=hostname"`
	// If specified, the fully qualified Pod hostname will be "<hostname>.<subdomain>.<pod namespace>.svc.<cluster domain>".
	// If not specified, the pod will not have a domainname at all.
	// +optional
	Subdomain string `json:"subdomain,omitempty" protobuf:"bytes,17,opt,name=subdomain"`
	// If specified, the pod's scheduling constraints
	// +optional
	Affinity *Affinity `json:"affinity,omitempty" protobuf:"bytes,18,opt,name=affinity"`
	// If specified, the pod will be dispatched by specified scheduler.
	// If not specified, the pod will be dispatched by default scheduler.
	// +optional
	SchedulerName string `json:"schedulerName,omitempty" protobuf:"bytes,19,opt,name=schedulerName"`
	// If specified, the pod's tolerations.
	// +optional
	Tolerations []Toleration `json:"tolerations,omitempty" protobuf:"bytes,22,opt,name=tolerations"`
	// HostAliases is an optional list of hosts and IPs that will be injected into the pod's hosts
	// file if specified. This is only valid for non-hostNetwork pods.
	// +optional
	// +patchMergeKey=ip
	// +patchStrategy=merge
	HostAliases []HostAlias `json:"hostAliases,omitempty" patchStrategy:"merge" patchMergeKey:"ip" protobuf:"bytes,23,rep,name=hostAliases"`
	// If specified, indicates the pod's priority. "system-node-critical" and
	// "system-cluster-critical" are two special keywords which indicate the
	// highest priorities with the former being the highest priority. Any other
	// name must be defined by creating a PriorityClass object with that name.
	// If not specified, the pod priority will be default or zero if there is no
	// default.
	// +optional
	PriorityClassName string `json:"priorityClassName,omitempty" protobuf:"bytes,24,opt,name=priorityClassName"`
	// The priority value. Various system components use this field to find the
	// priority of the pod. When Priority Admission Controller is enabled, it
	// prevents users from setting this field. The admission controller populates
	// this field from PriorityClassName.
	// The higher the value, the higher the priority.
	// +optional
	Priority *int32 `json:"priority,omitempty" protobuf:"bytes,25,opt,name=priority"`
	// Specifies the DNS parameters of a pod.
	// Parameters specified here will be merged to the generated DNS
	// configuration based on DNSPolicy.
	// +optional
	DNSConfig *PodDNSConfig `json:"dnsConfig,omitempty" protobuf:"bytes,26,opt,name=dnsConfig"`
	// If specified, all readiness gates will be evaluated for pod readiness.
	// A pod is ready when all its containers are ready AND
	// all conditions specified in the readiness gates have status equal to "True"
	// More info: https://git.k8s.io/enhancements/keps/sig-network/0007-pod-ready%2B%2B.md
	// +optional
	ReadinessGates []PodReadinessGate `json:"readinessGates,omitempty" protobuf:"bytes,28,opt,name=readinessGates"`
	// RuntimeClassName refers to a RuntimeClass object in the node.k8s.io group, which should be used
	// to run this pod.  If no RuntimeClass resource matches the named class, the pod will not be run.
	// If unset or empty, the "legacy" RuntimeClass will be used, which is an implicit class with an
	// empty definition that uses the default runtime handler.
	// More info: https://git.k8s.io/enhancements/keps/sig-node/runtime-class.md
	// This is a beta feature as of Kubernetes v1.14.
	// +optional
	RuntimeClassName *string `json:"runtimeClassName,omitempty" protobuf:"bytes,29,opt,name=runtimeClassName"`
	// EnableServiceLinks indicates whether information about services should be injected into pod's
	// environment variables, matching the syntax of Docker links.
	// Optional: Defaults to true.
	// +optional
	EnableServiceLinks *bool `json:"enableServiceLinks,omitempty" protobuf:"varint,30,opt,name=enableServiceLinks"`
	// PreemptionPolicy is the Policy for preempting pods with lower priority.
	// One of Never, PreemptLowerPriority.
	// Defaults to PreemptLowerPriority if unset.
	// This field is alpha-level and is only honored by servers that enable the NonPreemptingPriority feature.
	// +optional
	PreemptionPolicy *PreemptionPolicy `json:"preemptionPolicy,omitempty" protobuf:"bytes,31,opt,name=preemptionPolicy"`
	// Overhead represents the resource overhead associated with running a pod for a given RuntimeClass.
	// This field will be autopopulated at admission time by the RuntimeClass admission controller. If
	// the RuntimeClass admission controller is enabled, overhead must not be set in Pod create requests.
	// The RuntimeClass admission controller will reject Pod create requests which have the overhead already
	// set. If RuntimeClass is configured and selected in the PodSpec, Overhead will be set to the value
	// defined in the corresponding RuntimeClass, otherwise it will remain unset and treated as zero.
	// More info: https://git.k8s.io/enhancements/keps/sig-node/20190226-pod-overhead.md
	// This field is alpha-level as of Kubernetes v1.16, and is only honored by servers that enable the PodOverhead feature.
	// +optional
	Overhead ResourceList `json:"overhead,omitempty" protobuf:"bytes,32,opt,name=overhead"`
	// TopologySpreadConstraints describes how a group of pods ought to spread across topology
	// domains. Scheduler will schedule pods in a way which abides by the constraints.
	// This field is only honored by clusters that enable the EvenPodsSpread feature.
	// All topologySpreadConstraints are ANDed.
	// +optional
	// +patchMergeKey=topologyKey
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty" patchStrategy:"merge" patchMergeKey:"topologyKey" protobuf:"bytes,33,opt,name=topologySpreadConstraints"`
}

type UnsatisfiableConstraintAction string

const (
	// DoNotSchedule instructs the scheduler not to schedule the pod
	// when constraints are not satisfied.
	DoNotSchedule UnsatisfiableConstraintAction = "DoNotSchedule"
	// ScheduleAnyway instructs the scheduler to schedule the pod
	// even if constraints are not satisfied.
	ScheduleAnyway UnsatisfiableConstraintAction = "ScheduleAnyway"
)

// TopologySpreadConstraint specifies how to spread matching pods among the given topology.
type TopologySpreadConstraint struct {
	// MaxSkew describes the degree to which pods may be unevenly distributed.
	// It's the maximum permitted difference between the number of matching pods in
	// any two topology domains of a given topology type.
	// For example, in a 3-zone cluster, MaxSkew is set to 1, and pods with the same
	// labelSelector spread as 1/1/0:
	// +-------+-------+-------+
	// | zone1 | zone2 | zone3 |
	// +-------+-------+-------+
	// |   P   |   P   |       |
	// +-------+-------+-------+
	// - if MaxSkew is 1, incoming pod can only be scheduled to zone3 to become 1/1/1;
	// scheduling it onto zone1(zone2) would make the ActualSkew(2-0) on zone1(zone2)
	// violate MaxSkew(1).
	// - if MaxSkew is 2, incoming pod can be scheduled onto any zone.
	// It's a required field. Default value is 1 and 0 is not allowed.
	MaxSkew int32 `json:"maxSkew" protobuf:"varint,1,opt,name=maxSkew"`
	// TopologyKey is the key of node labels. Nodes that have a label with this key
	// and identical values are considered to be in the same topology.
	// We consider each <key, value> as a "bucket", and try to put balanced number
	// of pods into each bucket.
	// It's a required field.
	TopologyKey string `json:"topologyKey" protobuf:"bytes,2,opt,name=topologyKey"`
	// WhenUnsatisfiable indicates how to deal with a pod if it doesn't satisfy
	// the spread constraint.
	// - DoNotSchedule (default) tells the scheduler not to schedule it
	// - ScheduleAnyway tells the scheduler to still schedule it
	// It's considered as "Unsatisfiable" if and only if placing incoming pod on any
	// topology violates "MaxSkew".
	// For example, in a 3-zone cluster, MaxSkew is set to 1, and pods with the same
	// labelSelector spread as 3/1/1:
	// +-------+-------+-------+
	// | zone1 | zone2 | zone3 |
	// +-------+-------+-------+
	// | P P P |   P   |   P   |
	// +-------+-------+-------+
	// If WhenUnsatisfiable is set to DoNotSchedule, incoming pod can only be scheduled
	// to zone2(zone3) to become 3/2/1(3/1/2) as ActualSkew(2-1) on zone2(zone3) satisfies
	// MaxSkew(1). In other words, the cluster can still be imbalanced, but scheduler
	// won't make it *more* imbalanced.
	// It's a required field.
	WhenUnsatisfiable UnsatisfiableConstraintAction `json:"whenUnsatisfiable" protobuf:"bytes,3,opt,name=whenUnsatisfiable,casttype=UnsatisfiableConstraintAction"`
	// LabelSelector is used to find matching pods.
	// Pods that match this label selector are counted to determine the number of pods
	// in their corresponding topology domain.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty" protobuf:"bytes,4,opt,name=labelSelector"`
}

const (
	// The default value for enableServiceLinks attribute.
	DefaultEnableServiceLinks = true
)

// HostAlias holds the mapping between IP and hostnames that will be injected as an entry in the
// pod's hosts file.
type HostAlias struct {
	// IP address of the host file entry.
	IP string `json:"ip,omitempty" protobuf:"bytes,1,opt,name=ip"`
	// Hostnames for the above IP address.
	Hostnames []string `json:"hostnames,omitempty" protobuf:"bytes,2,rep,name=hostnames"`
}

// PodFSGroupChangePolicy holds policies that will be used for applying fsGroup to a volume
// when volume is mounted.
type PodFSGroupChangePolicy string

const (
	// FSGroupChangeOnRootMismatch indicates that volume's ownership and permissions will be changed
	// only when permission and ownership of root directory does not match with expected
	// permissions on the volume. This can help shorten the time it takes to change
	// ownership and permissions of a volume.
	FSGroupChangeOnRootMismatch PodFSGroupChangePolicy = "OnRootMismatch"
	// FSGroupChangeAlways indicates that volume's ownership and permissions
	// should always be changed whenever volume is mounted inside a Pod. This the default
	// behavior.
	FSGroupChangeAlways PodFSGroupChangePolicy = "Always"
)

// PodSecurityContext holds pod-level security attributes and common container settings.
// Some fields are also present in container.securityContext.  Field values of
// container.securityContext take precedence over field values of PodSecurityContext.
type PodSecurityContext struct {
	// The SELinux context to be applied to all containers.
	// If unspecified, the container runtime will allocate a random SELinux context for each
	// container.  May also be set in SecurityContext.  If set in
	// both SecurityContext and PodSecurityContext, the value specified in SecurityContext
	// takes precedence for that container.
	// +optional
	SELinuxOptions *SELinuxOptions `json:"seLinuxOptions,omitempty" protobuf:"bytes,1,opt,name=seLinuxOptions"`
	// The Windows specific settings applied to all containers.
	// If unspecified, the options within a container's SecurityContext will be used.
	// If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	WindowsOptions *WindowsSecurityContextOptions `json:"windowsOptions,omitempty" protobuf:"bytes,8,opt,name=windowsOptions"`
	// The UID to run the entrypoint of the container process.
	// Defaults to user specified in image metadata if unspecified.
	// May also be set in SecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence
	// for that container.
	// +optional
	RunAsUser *int64 `json:"runAsUser,omitempty" protobuf:"varint,2,opt,name=runAsUser"`
	// The GID to run the entrypoint of the container process.
	// Uses runtime default if unset.
	// May also be set in SecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence
	// for that container.
	// +optional
	RunAsGroup *int64 `json:"runAsGroup,omitempty" protobuf:"varint,6,opt,name=runAsGroup"`
	// Indicates that the container must run as a non-root user.
	// If true, the Kubelet will validate the image at runtime to ensure that it
	// does not run as UID 0 (root) and fail to start the container if it does.
	// If unset or false, no such validation will be performed.
	// May also be set in SecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsNonRoot *bool `json:"runAsNonRoot,omitempty" protobuf:"varint,3,opt,name=runAsNonRoot"`
	// A list of groups applied to the first process run in each container, in addition
	// to the container's primary GID.  If unspecified, no groups will be added to
	// any container.
	// +optional
	SupplementalGroups []int64 `json:"supplementalGroups,omitempty" protobuf:"varint,4,rep,name=supplementalGroups"`
	// A special supplemental group that applies to all containers in a pod.
	// Some volume types allow the Kubelet to change the ownership of that volume
	// to be owned by the pod:
	//
	// 1. The owning GID will be the FSGroup
	// 2. The setgid bit is set (new files created in the volume will be owned by FSGroup)
	// 3. The permission bits are OR'd with rw-rw----
	//
	// If unset, the Kubelet will not modify the ownership and permissions of any volume.
	// +optional
	FSGroup *int64 `json:"fsGroup,omitempty" protobuf:"varint,5,opt,name=fsGroup"`
	// Sysctls hold a list of namespaced sysctls used for the pod. Pods with unsupported
	// sysctls (by the container runtime) might fail to launch.
	// +optional
	Sysctls []Sysctl `json:"sysctls,omitempty" protobuf:"bytes,7,rep,name=sysctls"`
	// fsGroupChangePolicy defines behavior of changing ownership and permission of the volume
	// before being exposed inside Pod. This field will only apply to
	// volume types which support fsGroup based ownership(and permissions).
	// It will have no effect on ephemeral volume types such as: secret, configmaps
	// and emptydir.
	// Valid values are "OnRootMismatch" and "Always". If not specified defaults to "Always".
	// +optional
	FSGroupChangePolicy *PodFSGroupChangePolicy `json:"fsGroupChangePolicy,omitempty" protobuf:"bytes,9,opt,name=fsGroupChangePolicy"`
}

// PodQOSClass defines the supported qos classes of Pods.
type PodQOSClass string

const (
	// PodQOSGuaranteed is the Guaranteed qos class.
	PodQOSGuaranteed PodQOSClass = "Guaranteed"
	// PodQOSBurstable is the Burstable qos class.
	PodQOSBurstable PodQOSClass = "Burstable"
	// PodQOSBestEffort is the BestEffort qos class.
	PodQOSBestEffort PodQOSClass = "BestEffort"
)

// PodDNSConfig defines the DNS parameters of a pod in addition to
// those generated from DNSPolicy.
type PodDNSConfig struct {
	// A list of DNS name server IP addresses.
	// This will be appended to the base nameservers generated from DNSPolicy.
	// Duplicated nameservers will be removed.
	// +optional
	Nameservers []string `json:"nameservers,omitempty" protobuf:"bytes,1,rep,name=nameservers"`
	// A list of DNS search domains for host-name lookup.
	// This will be appended to the base search paths generated from DNSPolicy.
	// Duplicated search paths will be removed.
	// +optional
	Searches []string `json:"searches,omitempty" protobuf:"bytes,2,rep,name=searches"`
	// A list of DNS resolver options.
	// This will be merged with the base options generated from DNSPolicy.
	// Duplicated entries will be removed. Resolution options given in Options
	// will override those that appear in the base DNSPolicy.
	// +optional
	Options []PodDNSConfigOption `json:"options,omitempty" protobuf:"bytes,3,rep,name=options"`
}

// PodDNSConfigOption defines DNS resolver options of a pod.
type PodDNSConfigOption struct {
	// Required.
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	// +optional
	Value *string `json:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
}

// IP address information for entries in the (plural) PodIPs field.
// Each entry includes:
//    IP: An IP address allocated to the pod. Routable at least within the cluster.
type PodIP struct {
	// ip is an IP address (IPv4 or IPv6) assigned to the pod
	IP string `json:"ip,omitempty" protobuf:"bytes,1,opt,name=ip"`
}

// EphemeralContainerCommon is a copy of all fields in Container to be inlined in
// EphemeralContainer. This separate type allows easy conversion from EphemeralContainer
// to Container and allows separate documentation for the fields of EphemeralContainer.
// When a new field is added to Container it must be added here as well.
type EphemeralContainerCommon struct {
	// Name of the ephemeral container specified as a DNS_LABEL.
	// This name must be unique among all containers, init containers and ephemeral containers.
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// Docker image name.
	// More info: https://kubernetes.io/docs/concepts/containers/images
	Image string `json:"image,omitempty" protobuf:"bytes,2,opt,name=image"`
	// Entrypoint array. Not executed within a shell.
	// The docker image's ENTRYPOINT is used if this is not provided.
	// Variable references $(VAR_NAME) are expanded using the container's environment. If a variable
	// cannot be resolved, the reference in the input string will be unchanged. The $(VAR_NAME) syntax
	// can be escaped with a double $$, ie: $$(VAR_NAME). Escaped references will never be expanded,
	// regardless of whether the variable exists or not.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// +optional
	Command []string `json:"command,omitempty" protobuf:"bytes,3,rep,name=command"`
	// Arguments to the entrypoint.
	// The docker image's CMD is used if this is not provided.
	// Variable references $(VAR_NAME) are expanded using the container's environment. If a variable
	// cannot be resolved, the reference in the input string will be unchanged. The $(VAR_NAME) syntax
	// can be escaped with a double $$, ie: $$(VAR_NAME). Escaped references will never be expanded,
	// regardless of whether the variable exists or not.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/tasks/inject-data-application/define-command-argument-container/#running-a-command-in-a-shell
	// +optional
	Args []string `json:"args,omitempty" protobuf:"bytes,4,rep,name=args"`
	// Container's working directory.
	// If not specified, the container runtime's default will be used, which
	// might be configured in the container image.
	// Cannot be updated.
	// +optional
	WorkingDir string `json:"workingDir,omitempty" protobuf:"bytes,5,opt,name=workingDir"`
	// Ports are not allowed for ephemeral containers.
	Ports []ContainerPort `json:"ports,omitempty" protobuf:"bytes,6,rep,name=ports"`
	// List of sources to populate environment variables in the container.
	// The keys defined within a source must be a C_IDENTIFIER. All invalid keys
	// will be reported as an event when the container is starting. When a key exists in multiple
	// sources, the value associated with the last source will take precedence.
	// Values defined by an Env with a duplicate key will take precedence.
	// Cannot be updated.
	// +optional
	EnvFrom []EnvFromSource `json:"envFrom,omitempty" protobuf:"bytes,19,rep,name=envFrom"`
	// List of environment variables to set in the container.
	// Cannot be updated.
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Env []EnvVar `json:"env,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,7,rep,name=env"`
	// Resources are not allowed for ephemeral containers. Ephemeral containers use spare resources
	// already allocated to the pod.
	// +optional
	Resources ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,8,opt,name=resources"`
	// Pod volumes to mount into the container's filesystem.
	// Cannot be updated.
	// +optional
	// +patchMergeKey=mountPath
	// +patchStrategy=merge
	VolumeMounts []VolumeMount `json:"volumeMounts,omitempty" patchStrategy:"merge" patchMergeKey:"mountPath" protobuf:"bytes,9,rep,name=volumeMounts"`
	// volumeDevices is the list of block devices to be used by the container.
	// +patchMergeKey=devicePath
	// +patchStrategy=merge
	// +optional
	VolumeDevices []VolumeDevice `json:"volumeDevices,omitempty" patchStrategy:"merge" patchMergeKey:"devicePath" protobuf:"bytes,21,rep,name=volumeDevices"`
	// Probes are not allowed for ephemeral containers.
	// +optional
	LivenessProbe *Probe `json:"livenessProbe,omitempty" protobuf:"bytes,10,opt,name=livenessProbe"`
	// Probes are not allowed for ephemeral containers.
	// +optional
	ReadinessProbe *Probe `json:"readinessProbe,omitempty" protobuf:"bytes,11,opt,name=readinessProbe"`
	// Probes are not allowed for ephemeral containers.
	// +optional
	StartupProbe *Probe `json:"startupProbe,omitempty" protobuf:"bytes,22,opt,name=startupProbe"`
	// Lifecycle is not allowed for ephemeral containers.
	// +optional
	Lifecycle *Lifecycle `json:"lifecycle,omitempty" protobuf:"bytes,12,opt,name=lifecycle"`
	// Optional: Path at which the file to which the container's termination message
	// will be written is mounted into the container's filesystem.
	// Message written is intended to be brief final status, such as an assertion failure message.
	// Will be truncated by the node if greater than 4096 bytes. The total message length across
	// all containers will be limited to 12kb.
	// Defaults to /dev/termination-log.
	// Cannot be updated.
	// +optional
	TerminationMessagePath string `json:"terminationMessagePath,omitempty" protobuf:"bytes,13,opt,name=terminationMessagePath"`
	// Indicate how the termination message should be populated. File will use the contents of
	// terminationMessagePath to populate the container status message on both success and failure.
	// FallbackToLogsOnError will use the last chunk of container log output if the termination
	// message file is empty and the container exited with an error.
	// The log output is limited to 2048 bytes or 80 lines, whichever is smaller.
	// Defaults to File.
	// Cannot be updated.
	// +optional
	TerminationMessagePolicy TerminationMessagePolicy `json:"terminationMessagePolicy,omitempty" protobuf:"bytes,20,opt,name=terminationMessagePolicy,casttype=TerminationMessagePolicy"`
	// Image pull policy.
	// One of Always, Never, IfNotPresent.
	// Defaults to Always if :latest tag is specified, or IfNotPresent otherwise.
	// Cannot be updated.
	// More info: https://kubernetes.io/docs/concepts/containers/images#updating-images
	// +optional
	ImagePullPolicy PullPolicy `json:"imagePullPolicy,omitempty" protobuf:"bytes,14,opt,name=imagePullPolicy,casttype=PullPolicy"`
	// SecurityContext is not allowed for ephemeral containers.
	// +optional
	SecurityContext *SecurityContext `json:"securityContext,omitempty" protobuf:"bytes,15,opt,name=securityContext"`

	// Variables for interactive containers, these have very specialized use-cases (e.g. debugging)
	// and shouldn't be used for general purpose containers.

	// Whether this container should allocate a buffer for stdin in the container runtime. If this
	// is not set, reads from stdin in the container will always result in EOF.
	// Default is false.
	// +optional
	Stdin bool `json:"stdin,omitempty" protobuf:"varint,16,opt,name=stdin"`
	// Whether the container runtime should close the stdin channel after it has been opened by
	// a single attach. When stdin is true the stdin stream will remain open across multiple attach
	// sessions. If stdinOnce is set to true, stdin is opened on container start, is empty until the
	// first client attaches to stdin, and then remains open and accepts data until the client disconnects,
	// at which time stdin is closed and remains closed until the container is restarted. If this
	// flag is false, a container processes that reads from stdin will never receive an EOF.
	// Default is false
	// +optional
	StdinOnce bool `json:"stdinOnce,omitempty" protobuf:"varint,17,opt,name=stdinOnce"`
	// Whether this container should allocate a TTY for itself, also requires 'stdin' to be true.
	// Default is false.
	// +optional
	TTY bool `json:"tty,omitempty" protobuf:"varint,18,opt,name=tty"`
}

// EphemeralContainerCommon converts to Container. All fields must be kept in sync between
// these two types.
var _ = Container(EphemeralContainerCommon{})

// An EphemeralContainer is a container that may be added temporarily to an existing pod for
// user-initiated activities such as debugging. Ephemeral containers have no resource or
// scheduling guarantees, and they will not be restarted when they exit or when a pod is
// removed or restarted. If an ephemeral container causes a pod to exceed its resource
// allocation, the pod may be evicted.
// Ephemeral containers may not be added by directly updating the pod spec. They must be added
// via the pod's ephemeralcontainers subresource, and they will appear in the pod spec
// once added.
// This is an alpha feature enabled by the EphemeralContainers feature flag.
type EphemeralContainer struct {
	// Ephemeral containers have all of the fields of Container, plus additional fields
	// specific to ephemeral containers. Fields in common with Container are in the
	// following inlined struct so than an EphemeralContainer may easily be converted
	// to a Container.
	EphemeralContainerCommon `json:",inline" protobuf:"bytes,1,req"`

	// If set, the name of the container from PodSpec that this ephemeral container targets.
	// The ephemeral container will be run in the namespaces (IPC, PID, etc) of this container.
	// If not set then the ephemeral container is run in whatever namespaces are shared
	// for the pod. Note that the container runtime must support this feature.
	// +optional
	TargetContainerName string `json:"targetContainerName,omitempty" protobuf:"bytes,2,opt,name=targetContainerName"`
}

// PodStatus represents information about the status of a pod. Status may trail the actual
// state of a system, especially if the node that hosts the pod cannot contact the control
// plane.
type PodStatus struct {
	// The phase of a Pod is a simple, high-level summary of where the Pod is in its lifecycle.
	// The conditions array, the reason and message fields, and the individual container status
	// arrays contain more detail about the pod's status.
	// There are five possible phase values:
	//
	// Pending: The pod has been accepted by the Kubernetes system, but one or more of the
	// container images has not been created. This includes time before being scheduled as
	// well as time spent downloading images over the network, which could take a while.
	// Running: The pod has been bound to a node, and all of the containers have been created.
	// At least one container is still running, or is in the process of starting or restarting.
	// Succeeded: All containers in the pod have terminated in success, and will not be restarted.
	// Failed: All containers in the pod have terminated, and at least one container has
	// terminated in failure. The container either exited with non-zero status or was terminated
	// by the system.
	// Unknown: For some reason the state of the pod could not be obtained, typically due to an
	// error in communicating with the host of the pod.
	//
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-phase
	// +optional
	Phase PodPhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=PodPhase"`
	// Current service state of pod.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-conditions
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []PodCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`
	// A human readable message indicating details about why the pod is in this condition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,3,opt,name=message"`
	// A brief CamelCase message indicating details about why the pod is in this state.
	// e.g. 'Evicted'
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// nominatedNodeName is set only when this pod preempts other pods on the node, but it cannot be
	// scheduled right away as preemption victims receive their graceful termination periods.
	// This field does not guarantee that the pod will be scheduled on this node. Scheduler may decide
	// to place the pod elsewhere if other nodes become available sooner. Scheduler may also decide to
	// give the resources on this node to a higher priority pod that is created after preemption.
	// As a result, this field may be different than PodSpec.nodeName when the pod is
	// scheduled.
	// +optional
	NominatedNodeName string `json:"nominatedNodeName,omitempty" protobuf:"bytes,11,opt,name=nominatedNodeName"`

	// IP address of the host to which the pod is assigned. Empty if not yet scheduled.
	// +optional
	HostIP string `json:"hostIP,omitempty" protobuf:"bytes,5,opt,name=hostIP"`
	// IP address allocated to the pod. Routable at least within the cluster.
	// Empty if not yet allocated.
	// +optional
	PodIP string `json:"podIP,omitempty" protobuf:"bytes,6,opt,name=podIP"`

	// podIPs holds the IP addresses allocated to the pod. If this field is specified, the 0th entry must
	// match the podIP field. Pods may be allocated at most 1 value for each of IPv4 and IPv6. This list
	// is empty if no IPs have been allocated yet.
	// +optional
	// +patchStrategy=merge
	// +patchMergeKey=ip
	PodIPs []PodIP `json:"podIPs,omitempty" protobuf:"bytes,12,rep,name=podIPs" patchStrategy:"merge" patchMergeKey:"ip"`

	// RFC 3339 date and time at which the object was acknowledged by the Kubelet.
	// This is before the Kubelet pulled the container image(s) for the pod.
	// +optional
	StartTime *metav1.Time `json:"startTime,omitempty" protobuf:"bytes,7,opt,name=startTime"`

	// The list has one entry per init container in the manifest. The most recent successful
	// init container will have ready = true, the most recently started container will have
	// startTime set.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-and-container-status
	InitContainerStatuses []ContainerStatus `json:"initContainerStatuses,omitempty" protobuf:"bytes,10,rep,name=initContainerStatuses"`

	// The list has one entry per container in the manifest. Each entry is currently the output
	// of `docker inspect`.
	// More info: https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle#pod-and-container-status
	// +optional
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty" protobuf:"bytes,8,rep,name=containerStatuses"`
	// The Quality of Service (QOS) classification assigned to the pod based on resource requirements
	// See PodQOSClass type for available QOS classes
	// More info: https://git.k8s.io/community/contributors/design-proposals/node/resource-qos.md
	// +optional
	QOSClass PodQOSClass `json:"qosClass,omitempty" protobuf:"bytes,9,rep,name=qosClass"`
	// Status for any ephemeral containers that have run in this pod.
	// This field is alpha-level and is only populated by servers that enable the EphemeralContainers feature.
	// +optional
	EphemeralContainerStatuses []ContainerStatus `json:"ephemeralContainerStatuses,omitempty" protobuf:"bytes,13,rep,name=ephemeralContainerStatuses"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodStatusResult is a wrapper for PodStatus returned by kubelet that can be encode/decoded
type PodStatusResult struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	// Most recently observed status of the pod.
	// This data may not be up to date.
	// Populated by the system.
	// Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status PodStatus `json:"status,omitempty" protobuf:"bytes,2,opt,name=status"`
}

// +genclient
// +genclient:method=GetEphemeralContainers,verb=get,subresource=ephemeralcontainers,result=EphemeralContainers
// +genclient:method=UpdateEphemeralContainers,verb=update,subresource=ephemeralcontainers,input=EphemeralContainers,result=EphemeralContainers
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Pod is a collection of containers that can run on a host. This resource is created
// by clients and scheduled onto hosts.
type Pod struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the desired behavior of the pod.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec PodSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Most recently observed status of the pod.
	// This data may not be up to date.
	// Populated by the system.
	// Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status PodStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodList is a list of Pods.
type PodList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of pods.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md
	Items []Pod `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// PodTemplateSpec describes the data a pod should have when created from a template
type PodTemplateSpec struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Specification of the desired behavior of the pod.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec PodSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodTemplate describes a template for creating copies of a predefined pod.
type PodTemplate struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Template defines the pods that will be created from this pod template.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Template PodTemplateSpec `json:"template,omitempty" protobuf:"bytes,2,opt,name=template"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodTemplateList is a list of PodTemplates.
type PodTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of pod templates
	Items []PodTemplate `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// ReplicationControllerSpec is the specification of a replication controller.
type ReplicationControllerSpec struct {
	// Replicas is the number of desired replicas.
	// This is a pointer to distinguish between explicit zero and unspecified.
	// Defaults to 1.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#what-is-a-replicationcontroller
	// +optional
	Replicas *int32 `json:"replicas,omitempty" protobuf:"varint,1,opt,name=replicas"`

	// Minimum number of seconds for which a newly created pod should be ready
	// without any of its container crashing, for it to be considered available.
	// Defaults to 0 (pod will be considered available as soon as it is ready)
	// +optional
	MinReadySeconds int32 `json:"minReadySeconds,omitempty" protobuf:"varint,4,opt,name=minReadySeconds"`

	// Selector is a label query over pods that should match the Replicas count.
	// If Selector is empty, it is defaulted to the labels present on the Pod template.
	// Label keys and values that must match in order to be controlled by this replication
	// controller, if empty defaulted to labels on Pod template.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	// +optional
	Selector map[string]string `json:"selector,omitempty" protobuf:"bytes,2,rep,name=selector"`

	// TemplateRef is a reference to an object that describes the pod that will be created if
	// insufficient replicas are detected.
	// Reference to an object that describes the pod that will be created if insufficient replicas are detected.
	// +optional
	// TemplateRef *ObjectReference `json:"templateRef,omitempty"`

	// Template is the object that describes the pod that will be created if
	// insufficient replicas are detected. This takes precedence over a TemplateRef.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#pod-template
	// +optional
	Template *PodTemplateSpec `json:"template,omitempty" protobuf:"bytes,3,opt,name=template"`
}

// ReplicationControllerStatus represents the current status of a replication
// controller.
type ReplicationControllerStatus struct {
	// Replicas is the most recently oberved number of replicas.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller#what-is-a-replicationcontroller
	Replicas int32 `json:"replicas" protobuf:"varint,1,opt,name=replicas"`

	// The number of pods that have labels matching the labels of the pod template of the replication controller.
	// +optional
	FullyLabeledReplicas int32 `json:"fullyLabeledReplicas,omitempty" protobuf:"varint,2,opt,name=fullyLabeledReplicas"`

	// The number of ready replicas for this replication controller.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty" protobuf:"varint,4,opt,name=readyReplicas"`

	// The number of available replicas (ready for at least minReadySeconds) for this replication controller.
	// +optional
	AvailableReplicas int32 `json:"availableReplicas,omitempty" protobuf:"varint,5,opt,name=availableReplicas"`

	// ObservedGeneration reflects the generation of the most recently observed replication controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,3,opt,name=observedGeneration"`

	// Represents the latest available observations of a replication controller's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []ReplicationControllerCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,6,rep,name=conditions"`
}

type ReplicationControllerConditionType string

// These are valid conditions of a replication controller.
const (
	// ReplicationControllerReplicaFailure is added in a replication controller when one of its pods
	// fails to be created due to insufficient quota, limit ranges, pod security policy, node selectors,
	// etc. or deleted due to kubelet being down or finalizers are failing.
	ReplicationControllerReplicaFailure ReplicationControllerConditionType = "ReplicaFailure"
)

// ReplicationControllerCondition describes the state of a replication controller at a certain point.
type ReplicationControllerCondition struct {
	// Type of replication controller condition.
	Type ReplicationControllerConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=ReplicationControllerConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// The last time the condition transitioned from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,3,opt,name=lastTransitionTime"`
	// The reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,4,opt,name=reason"`
	// A human readable message indicating details about the transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,5,opt,name=message"`
}

// +genclient
// +genclient:method=GetScale,verb=get,subresource=scale,result=k8s.io/api/autoscaling/v1.Scale
// +genclient:method=UpdateScale,verb=update,subresource=scale,input=k8s.io/api/autoscaling/v1.Scale,result=k8s.io/api/autoscaling/v1.Scale
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReplicationController represents the configuration of a replication controller.
type ReplicationController struct {
	metav1.TypeMeta `json:",inline"`

	// If the Labels of a ReplicationController are empty, they are defaulted to
	// be the same as the Pod(s) that the replication controller manages.
	// Standard object's metadata. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the specification of the desired behavior of the replication controller.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec ReplicationControllerSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status is the most recently observed status of the replication controller.
	// This data may be out of date by some window of time.
	// Populated by the system.
	// Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status ReplicationControllerStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ReplicationControllerList is a collection of replication controllers.
type ReplicationControllerList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of replication controllers.
	// More info: https://kubernetes.io/docs/concepts/workloads/controllers/replicationcontroller
	Items []ReplicationController `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// Session Affinity Type string
type ServiceAffinity string

const (
	// ServiceAffinityClientIP is the Client IP based.
	ServiceAffinityClientIP ServiceAffinity = "ClientIP"

	// ServiceAffinityNone - no session affinity.
	ServiceAffinityNone ServiceAffinity = "None"
)

const DefaultClientIPServiceAffinitySeconds int32 = 10800

// SessionAffinityConfig represents the configurations of session affinity.
type SessionAffinityConfig struct {
	// clientIP contains the configurations of Client IP based session affinity.
	// +optional
	ClientIP *ClientIPConfig `json:"clientIP,omitempty" protobuf:"bytes,1,opt,name=clientIP"`
}

// ClientIPConfig represents the configurations of Client IP based session affinity.
type ClientIPConfig struct {
	// timeoutSeconds specifies the seconds of ClientIP type session sticky time.
	// The value must be >0 && <=86400(for 1 day) if ServiceAffinity == "ClientIP".
	// Default value is 10800(for 3 hours).
	// +optional
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty" protobuf:"varint,1,opt,name=timeoutSeconds"`
}

// Service Type string describes ingress methods for a service
type ServiceType string

const (
	// ServiceTypeClusterIP means a service will only be accessible inside the
	// cluster, via the cluster IP.
	ServiceTypeClusterIP ServiceType = "ClusterIP"

	// ServiceTypeNodePort means a service will be exposed on one port of
	// every node, in addition to 'ClusterIP' type.
	ServiceTypeNodePort ServiceType = "NodePort"

	// ServiceTypeLoadBalancer means a service will be exposed via an
	// external load balancer (if the cloud provider supports it), in addition
	// to 'NodePort' type.
	ServiceTypeLoadBalancer ServiceType = "LoadBalancer"

	// ServiceTypeExternalName means a service consists of only a reference to
	// an external name that kubedns or equivalent will return as a CNAME
	// record, with no exposing or proxying of any pods involved.
	ServiceTypeExternalName ServiceType = "ExternalName"
)

// Service External Traffic Policy Type string
type ServiceExternalTrafficPolicyType string

const (
	// ServiceExternalTrafficPolicyTypeLocal specifies node-local endpoints behavior.
	ServiceExternalTrafficPolicyTypeLocal ServiceExternalTrafficPolicyType = "Local"
	// ServiceExternalTrafficPolicyTypeCluster specifies node-global (legacy) behavior.
	ServiceExternalTrafficPolicyTypeCluster ServiceExternalTrafficPolicyType = "Cluster"
)

// ServiceStatus represents the current status of a service.
type ServiceStatus struct {
	// LoadBalancer contains the current status of the load-balancer,
	// if one is present.
	// +optional
	LoadBalancer LoadBalancerStatus `json:"loadBalancer,omitempty" protobuf:"bytes,1,opt,name=loadBalancer"`
}

// LoadBalancerStatus represents the status of a load-balancer.
type LoadBalancerStatus struct {
	// Ingress is a list containing ingress points for the load-balancer.
	// Traffic intended for the service should be sent to these ingress points.
	// +optional
	Ingress []LoadBalancerIngress `json:"ingress,omitempty" protobuf:"bytes,1,rep,name=ingress"`
}

// LoadBalancerIngress represents the status of a load-balancer ingress point:
// traffic intended for the service should be sent to an ingress point.
type LoadBalancerIngress struct {
	// IP is set for load-balancer ingress points that are IP based
	// (typically GCE or OpenStack load-balancers)
	// +optional
	IP string `json:"ip,omitempty" protobuf:"bytes,1,opt,name=ip"`

	// Hostname is set for load-balancer ingress points that are DNS based
	// (typically AWS load-balancers)
	// +optional
	Hostname string `json:"hostname,omitempty" protobuf:"bytes,2,opt,name=hostname"`
}

// IPFamily represents the IP Family (IPv4 or IPv6). This type is used
// to express the family of an IP expressed by a type (i.e. service.Spec.IPFamily)
type IPFamily string

const (
	// IPv4Protocol indicates that this IP is IPv4 protocol
	IPv4Protocol IPFamily = "IPv4"
	// IPv6Protocol indicates that this IP is IPv6 protocol
	IPv6Protocol IPFamily = "IPv6"
	// MaxServiceTopologyKeys is the largest number of topology keys allowed on a service
	MaxServiceTopologyKeys = 16
)

// ServiceSpec describes the attributes that a user creates on a service.
type ServiceSpec struct {
	// The list of ports that are exposed by this service.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies
	// +patchMergeKey=port
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=port
	// +listMapKey=protocol
	Ports []ServicePort `json:"ports,omitempty" patchStrategy:"merge" patchMergeKey:"port" protobuf:"bytes,1,rep,name=ports"`

	// Route service traffic to pods with label keys and values matching this
	// selector. If empty or not present, the service is assumed to have an
	// external process managing its endpoints, which Kubernetes will not
	// modify. Only applies to types ClusterIP, NodePort, and LoadBalancer.
	// Ignored if type is ExternalName.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/
	// +optional
	Selector map[string]string `json:"selector,omitempty" protobuf:"bytes,2,rep,name=selector"`

	// clusterIP is the IP address of the service and is usually assigned
	// randomly by the master. If an address is specified manually and is not in
	// use by others, it will be allocated to the service; otherwise, creation
	// of the service will fail. This field can not be changed through updates.
	// Valid values are "None", empty string (""), or a valid IP address. "None"
	// can be specified for headless services when proxying is not required.
	// Only applies to types ClusterIP, NodePort, and LoadBalancer. Ignored if
	// type is ExternalName.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies
	// +optional
	ClusterIP string `json:"clusterIP,omitempty" protobuf:"bytes,3,opt,name=clusterIP"`

	// type determines how the Service is exposed. Defaults to ClusterIP. Valid
	// options are ExternalName, ClusterIP, NodePort, and LoadBalancer.
	// "ExternalName" maps to the specified externalName.
	// "ClusterIP" allocates a cluster-internal IP address for load-balancing to
	// endpoints. Endpoints are determined by the selector or if that is not
	// specified, by manual construction of an Endpoints object. If clusterIP is
	// "None", no virtual IP is allocated and the endpoints are published as a
	// set of endpoints rather than a stable IP.
	// "NodePort" builds on ClusterIP and allocates a port on every node which
	// routes to the clusterIP.
	// "LoadBalancer" builds on NodePort and creates an
	// external load-balancer (if supported in the current cloud) which routes
	// to the clusterIP.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types
	// +optional
	Type ServiceType `json:"type,omitempty" protobuf:"bytes,4,opt,name=type,casttype=ServiceType"`

	// externalIPs is a list of IP addresses for which nodes in the cluster
	// will also accept traffic for this service.  These IPs are not managed by
	// Kubernetes.  The user is responsible for ensuring that traffic arrives
	// at a node with this IP.  A common example is external load-balancers
	// that are not part of the Kubernetes system.
	// +optional
	ExternalIPs []string `json:"externalIPs,omitempty" protobuf:"bytes,5,rep,name=externalIPs"`

	// Supports "ClientIP" and "None". Used to maintain session affinity.
	// Enable client IP based session affinity.
	// Must be ClientIP or None.
	// Defaults to None.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#virtual-ips-and-service-proxies
	// +optional
	SessionAffinity ServiceAffinity `json:"sessionAffinity,omitempty" protobuf:"bytes,7,opt,name=sessionAffinity,casttype=ServiceAffinity"`

	// Only applies to Service Type: LoadBalancer
	// LoadBalancer will get created with the IP specified in this field.
	// This feature depends on whether the underlying cloud-provider supports specifying
	// the loadBalancerIP when a load balancer is created.
	// This field will be ignored if the cloud-provider does not support the feature.
	// +optional
	LoadBalancerIP string `json:"loadBalancerIP,omitempty" protobuf:"bytes,8,opt,name=loadBalancerIP"`

	// If specified and supported by the platform, this will restrict traffic through the cloud-provider
	// load-balancer will be restricted to the specified client IPs. This field will be ignored if the
	// cloud-provider does not support the feature."
	// More info: https://kubernetes.io/docs/tasks/access-application-cluster/configure-cloud-provider-firewall/
	// +optional
	LoadBalancerSourceRanges []string `json:"loadBalancerSourceRanges,omitempty" protobuf:"bytes,9,opt,name=loadBalancerSourceRanges"`

	// externalName is the external reference that kubedns or equivalent will
	// return as a CNAME record for this service. No proxying will be involved.
	// Must be a valid RFC-1123 hostname (https://tools.ietf.org/html/rfc1123)
	// and requires Type to be ExternalName.
	// +optional
	ExternalName string `json:"externalName,omitempty" protobuf:"bytes,10,opt,name=externalName"`

	// externalTrafficPolicy denotes if this Service desires to route external
	// traffic to node-local or cluster-wide endpoints. "Local" preserves the
	// client source IP and avoids a second hop for LoadBalancer and Nodeport
	// type services, but risks potentially imbalanced traffic spreading.
	// "Cluster" obscures the client source IP and may cause a second hop to
	// another node, but should have good overall load-spreading.
	// +optional
	ExternalTrafficPolicy ServiceExternalTrafficPolicyType `json:"externalTrafficPolicy,omitempty" protobuf:"bytes,11,opt,name=externalTrafficPolicy"`

	// healthCheckNodePort specifies the healthcheck nodePort for the service.
	// If not specified, HealthCheckNodePort is created by the service api
	// backend with the allocated nodePort. Will use user-specified nodePort value
	// if specified by the client. Only effects when Type is set to LoadBalancer
	// and ExternalTrafficPolicy is set to Local.
	// +optional
	HealthCheckNodePort int32 `json:"healthCheckNodePort,omitempty" protobuf:"bytes,12,opt,name=healthCheckNodePort"`

	// publishNotReadyAddresses, when set to true, indicates that DNS implementations
	// must publish the notReadyAddresses of subsets for the Endpoints associated with
	// the Service. The default value is false.
	// The primary use case for setting this field is to use a StatefulSet's Headless Service
	// to propagate SRV records for its Pods without respect to their readiness for purpose
	// of peer discovery.
	// +optional
	PublishNotReadyAddresses bool `json:"publishNotReadyAddresses,omitempty" protobuf:"varint,13,opt,name=publishNotReadyAddresses"`

	// sessionAffinityConfig contains the configurations of session affinity.
	// +optional
	SessionAffinityConfig *SessionAffinityConfig `json:"sessionAffinityConfig,omitempty" protobuf:"bytes,14,opt,name=sessionAffinityConfig"`

	// ipFamily specifies whether this Service has a preference for a particular IP family (e.g. IPv4 vs.
	// IPv6).  If a specific IP family is requested, the clusterIP field will be allocated from that family, if it is
	// available in the cluster.  If no IP family is requested, the cluster's primary IP family will be used.
	// Other IP fields (loadBalancerIP, loadBalancerSourceRanges, externalIPs) and controllers which
	// allocate external load-balancers should use the same IP family.  Endpoints for this Service will be of
	// this family.  This field is immutable after creation. Assigning a ServiceIPFamily not available in the
	// cluster (e.g. IPv6 in IPv4 only cluster) is an error condition and will fail during clusterIP assignment.
	// +optional
	IPFamily *IPFamily `json:"ipFamily,omitempty" protobuf:"bytes,15,opt,name=ipFamily,Configcasttype=IPFamily"`

	// topologyKeys is a preference-order list of topology keys which
	// implementations of services should use to preferentially sort endpoints
	// when accessing this Service, it can not be used at the same time as
	// externalTrafficPolicy=Local.
	// Topology keys must be valid label keys and at most 16 keys may be specified.
	// Endpoints are chosen based on the first topology key with available backends.
	// If this field is specified and all entries have no backends that match
	// the topology of the client, the service has no backends for that client
	// and connections should fail.
	// The special value "*" may be used to mean "any topology". This catch-all
	// value, if used, only makes sense as the last value in the list.
	// If this is not specified or empty, no topology constraints will be applied.
	// +optional
	TopologyKeys []string `json:"topologyKeys,omitempty" protobuf:"bytes,16,opt,name=topologyKeys"`
}

// ServicePort contains information on service's port.
type ServicePort struct {
	// The name of this port within the service. This must be a DNS_LABEL.
	// All ports within a ServiceSpec must have unique names. When considering
	// the endpoints for a Service, this must match the 'name' field in the
	// EndpointPort.
	// Optional if only one ServicePort is defined on this service.
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// The IP protocol for this port. Supports "TCP", "UDP", and "SCTP".
	// Default is TCP.
	// +optional
	Protocol Protocol `json:"protocol,omitempty" protobuf:"bytes,2,opt,name=protocol,casttype=Protocol"`

	// The application protocol for this port.
	// This field follows standard Kubernetes label syntax.
	// Un-prefixed names are reserved for IANA standard service names (as per
	// RFC-6335 and http://www.iana.org/assignments/service-names).
	// Non-standard protocols should use prefixed names such as
	// mycompany.com/my-custom-protocol.
	// Field can be enabled with ServiceAppProtocol feature gate.
	// +optional
	AppProtocol *string `json:"appProtocol,omitempty" protobuf:"bytes,6,opt,name=appProtocol"`

	// The port that will be exposed by this service.
	Port int32 `json:"port" protobuf:"varint,3,opt,name=port"`

	// Number or name of the port to access on the pods targeted by the service.
	// Number must be in the range 1 to 65535. Name must be an IANA_SVC_NAME.
	// If this is a string, it will be looked up as a named port in the
	// target Pod's container ports. If this is not specified, the value
	// of the 'port' field is used (an identity map).
	// This field is ignored for services with clusterIP=None, and should be
	// omitted or set equal to the 'port' field.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#defining-a-service
	// +optional
	TargetPort intstr.IntOrString `json:"targetPort,omitempty" protobuf:"bytes,4,opt,name=targetPort"`

	// The port on each node on which this service is exposed when type=NodePort or LoadBalancer.
	// Usually assigned by the system. If specified, it will be allocated to the service
	// if unused or else creation of the service will fail.
	// Default is to auto-allocate a port if the ServiceType of this Service requires one.
	// More info: https://kubernetes.io/docs/concepts/services-networking/service/#type-nodeport
	// +optional
	NodePort int32 `json:"nodePort,omitempty" protobuf:"varint,5,opt,name=nodePort"`
}

// +genclient
// +genclient:skipVerbs=deleteCollection
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Service is a named abstraction of software service (for example, mysql) consisting of local port
// (for example 3306) that the proxy listens on, and the selector that determines which pods
// will answer requests sent through the proxy.
type Service struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the behavior of a service.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec ServiceSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Most recently observed status of the service.
	// Populated by the system.
	// Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status ServiceStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

const (
	// ClusterIPNone - do not assign a cluster IP
	// no proxying required and no environment variables should be created for pods
	ClusterIPNone = "None"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceList holds a list of services.
type ServiceList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of services
	Items []Service `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +genclient:method=CreateToken,verb=create,subresource=token,input=k8s.io/api/authentication/v1.TokenRequest,result=k8s.io/api/authentication/v1.TokenRequest
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceAccount binds together:
// * a name, understood by users, and perhaps by peripheral systems, for an identity
// * a principal that can be authenticated and authorized
// * a set of secrets
type ServiceAccount struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Secrets is the list of secrets allowed to be used by pods running using this ServiceAccount.
	// More info: https://kubernetes.io/docs/concepts/configuration/secret
	// +optional
	// +patchMergeKey=name
	// +patchStrategy=merge
	Secrets []ObjectReference `json:"secrets,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=secrets"`

	// ImagePullSecrets is a list of references to secrets in the same namespace to use for pulling any images
	// in pods that reference this ServiceAccount. ImagePullSecrets are distinct from Secrets because Secrets
	// can be mounted in the pod, but ImagePullSecrets are only accessed by the kubelet.
	// More info: https://kubernetes.io/docs/concepts/containers/images/#specifying-imagepullsecrets-on-a-pod
	// +optional
	ImagePullSecrets []LocalObjectReference `json:"imagePullSecrets,omitempty" protobuf:"bytes,3,rep,name=imagePullSecrets"`

	// AutomountServiceAccountToken indicates whether pods running as this service account should have an API token automatically mounted.
	// Can be overridden at the pod level.
	// +optional
	AutomountServiceAccountToken *bool `json:"automountServiceAccountToken,omitempty" protobuf:"varint,4,opt,name=automountServiceAccountToken"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceAccountList is a list of ServiceAccount objects
type ServiceAccountList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of ServiceAccounts.
	// More info: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
	Items []ServiceAccount `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Endpoints is a collection of endpoints that implement the actual service. Example:
//   Name: "mysvc",
//   Subsets: [
//     {
//       Addresses: [{"ip": "10.10.1.1"}, {"ip": "10.10.2.2"}],
//       Ports: [{"name": "a", "port": 8675}, {"name": "b", "port": 309}]
//     },
//     {
//       Addresses: [{"ip": "10.10.3.3"}],
//       Ports: [{"name": "a", "port": 93}, {"name": "b", "port": 76}]
//     },
//  ]
type Endpoints struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// The set of all endpoints is the union of all subsets. Addresses are placed into
	// subsets according to the IPs they share. A single address with multiple ports,
	// some of which are ready and some of which are not (because they come from
	// different containers) will result in the address being displayed in different
	// subsets for the different ports. No address will appear in both Addresses and
	// NotReadyAddresses in the same subset.
	// Sets of addresses and ports that comprise a service.
	// +optional
	Subsets []EndpointSubset `json:"subsets,omitempty" protobuf:"bytes,2,rep,name=subsets"`
}

// EndpointSubset is a group of addresses with a common set of ports. The
// expanded set of endpoints is the Cartesian product of Addresses x Ports.
// For example, given:
//   {
//     Addresses: [{"ip": "10.10.1.1"}, {"ip": "10.10.2.2"}],
//     Ports:     [{"name": "a", "port": 8675}, {"name": "b", "port": 309}]
//   }
// The resulting set of endpoints can be viewed as:
//     a: [ 10.10.1.1:8675, 10.10.2.2:8675 ],
//     b: [ 10.10.1.1:309, 10.10.2.2:309 ]
type EndpointSubset struct {
	// IP addresses which offer the related ports that are marked as ready. These endpoints
	// should be considered safe for load balancers and clients to utilize.
	// +optional
	Addresses []EndpointAddress `json:"addresses,omitempty" protobuf:"bytes,1,rep,name=addresses"`
	// IP addresses which offer the related ports but are not currently marked as ready
	// because they have not yet finished starting, have recently failed a readiness check,
	// or have recently failed a liveness check.
	// +optional
	NotReadyAddresses []EndpointAddress `json:"notReadyAddresses,omitempty" protobuf:"bytes,2,rep,name=notReadyAddresses"`
	// Port numbers available on the related IP addresses.
	// +optional
	Ports []EndpointPort `json:"ports,omitempty" protobuf:"bytes,3,rep,name=ports"`
}

// EndpointAddress is a tuple that describes single IP address.
type EndpointAddress struct {
	// The IP of this endpoint.
	// May not be loopback (127.0.0.0/8), link-local (169.254.0.0/16),
	// or link-local multicast ((224.0.0.0/24).
	// IPv6 is also accepted but not fully supported on all platforms. Also, certain
	// kubernetes components, like kube-proxy, are not IPv6 ready.
	// TODO: This should allow hostname or IP, See #4447.
	IP string `json:"ip" protobuf:"bytes,1,opt,name=ip"`
	// The Hostname of this endpoint
	// +optional
	Hostname string `json:"hostname,omitempty" protobuf:"bytes,3,opt,name=hostname"`
	// Optional: Node hosting this endpoint. This can be used to determine endpoints local to a node.
	// +optional
	NodeName *string `json:"nodeName,omitempty" protobuf:"bytes,4,opt,name=nodeName"`
	// Reference to object providing the endpoint.
	// +optional
	TargetRef *ObjectReference `json:"targetRef,omitempty" protobuf:"bytes,2,opt,name=targetRef"`
}

// EndpointPort is a tuple that describes a single port.
type EndpointPort struct {
	// The name of this port.  This must match the 'name' field in the
	// corresponding ServicePort.
	// Must be a DNS_LABEL.
	// Optional only if one port is defined.
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// The port number of the endpoint.
	Port int32 `json:"port" protobuf:"varint,2,opt,name=port"`

	// The IP protocol for this port.
	// Must be UDP, TCP, or SCTP.
	// Default is TCP.
	// +optional
	Protocol Protocol `json:"protocol,omitempty" protobuf:"bytes,3,opt,name=protocol,casttype=Protocol"`

	// The application protocol for this port.
	// This field follows standard Kubernetes label syntax.
	// Un-prefixed names are reserved for IANA standard service names (as per
	// RFC-6335 and http://www.iana.org/assignments/service-names).
	// Non-standard protocols should use prefixed names such as
	// mycompany.com/my-custom-protocol.
	// Field can be enabled with ServiceAppProtocol feature gate.
	// +optional
	AppProtocol *string `json:"appProtocol,omitempty" protobuf:"bytes,4,opt,name=appProtocol"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EndpointsList is a list of endpoints.
type EndpointsList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of endpoints.
	Items []Endpoints `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// NodeSpec describes the attributes that a node is created with.
type NodeSpec struct {
	// PodCIDR represents the pod IP range assigned to the node.
	// +optional
	PodCIDR string `json:"podCIDR,omitempty" protobuf:"bytes,1,opt,name=podCIDR"`

	// podCIDRs represents the IP ranges assigned to the node for usage by Pods on that node. If this
	// field is specified, the 0th entry must match the podCIDR field. It may contain at most 1 value for
	// each of IPv4 and IPv6.
	// +optional
	// +patchStrategy=merge
	PodCIDRs []string `json:"podCIDRs,omitempty" protobuf:"bytes,7,opt,name=podCIDRs" patchStrategy:"merge"`

	// ID of the node assigned by the cloud provider in the format: <ProviderName>://<ProviderSpecificNodeID>
	// +optional
	ProviderID string `json:"providerID,omitempty" protobuf:"bytes,3,opt,name=providerID"`
	// Unschedulable controls node schedulability of new pods. By default, node is schedulable.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#manual-node-administration
	// +optional
	Unschedulable bool `json:"unschedulable,omitempty" protobuf:"varint,4,opt,name=unschedulable"`
	// If specified, the node's taints.
	// +optional
	Taints []Taint `json:"taints,omitempty" protobuf:"bytes,5,opt,name=taints"`
	// If specified, the source to get node configuration from
	// The DynamicKubeletConfig feature gate must be enabled for the Kubelet to use this field
	// +optional
	ConfigSource *NodeConfigSource `json:"configSource,omitempty" protobuf:"bytes,6,opt,name=configSource"`

	// Deprecated. Not all kubelets will set this field. Remove field after 1.13.
	// see: https://issues.k8s.io/61966
	// +optional
	DoNotUseExternalID string `json:"externalID,omitempty" protobuf:"bytes,2,opt,name=externalID"`
}

// NodeConfigSource specifies a source of node configuration. Exactly one subfield (excluding metadata) must be non-nil.
type NodeConfigSource struct {
	// For historical context, regarding the below kind, apiVersion, and configMapRef deprecation tags:
	// 1. kind/apiVersion were used by the kubelet to persist this struct to disk (they had no protobuf tags)
	// 2. configMapRef and proto tag 1 were used by the API to refer to a configmap,
	//    but used a generic ObjectReference type that didn't really have the fields we needed
	// All uses/persistence of the NodeConfigSource struct prior to 1.11 were gated by alpha feature flags,
	// so there was no persisted data for these fields that needed to be migrated/handled.

	// +k8s:deprecated=kind
	// +k8s:deprecated=apiVersion
	// +k8s:deprecated=configMapRef,protobuf=1

	// ConfigMap is a reference to a Node's ConfigMap
	ConfigMap *ConfigMapNodeConfigSource `json:"configMap,omitempty" protobuf:"bytes,2,opt,name=configMap"`
}

// ConfigMapNodeConfigSource contains the information to reference a ConfigMap as a config source for the Node.
type ConfigMapNodeConfigSource struct {
	// Namespace is the metadata.namespace of the referenced ConfigMap.
	// This field is required in all cases.
	Namespace string `json:"namespace" protobuf:"bytes,1,opt,name=namespace"`

	// Name is the metadata.name of the referenced ConfigMap.
	// This field is required in all cases.
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`

	// UID is the metadata.UID of the referenced ConfigMap.
	// This field is forbidden in Node.Spec, and required in Node.Status.
	// +optional
	UID types.UID `json:"uid,omitempty" protobuf:"bytes,3,opt,name=uid"`

	// ResourceVersion is the metadata.ResourceVersion of the referenced ConfigMap.
	// This field is forbidden in Node.Spec, and required in Node.Status.
	// +optional
	ResourceVersion string `json:"resourceVersion,omitempty" protobuf:"bytes,4,opt,name=resourceVersion"`

	// KubeletConfigKey declares which key of the referenced ConfigMap corresponds to the KubeletConfiguration structure
	// This field is required in all cases.
	KubeletConfigKey string `json:"kubeletConfigKey" protobuf:"bytes,5,opt,name=kubeletConfigKey"`
}

// DaemonEndpoint contains information about a single Daemon endpoint.
type DaemonEndpoint struct {
	/*
		The port tag was not properly in quotes in earlier releases, so it must be
		uppercased for backwards compat (since it was falling back to var name of
		'Port').
	*/

	// Port number of the given endpoint.
	Port int32 `json:"Port" protobuf:"varint,1,opt,name=Port"`
}

// NodeDaemonEndpoints lists ports opened by daemons running on the Node.
type NodeDaemonEndpoints struct {
	// Endpoint on which Kubelet is listening.
	// +optional
	KubeletEndpoint DaemonEndpoint `json:"kubeletEndpoint,omitempty" protobuf:"bytes,1,opt,name=kubeletEndpoint"`
}

// NodeSystemInfo is a set of ids/uuids to uniquely identify the node.
type NodeSystemInfo struct {
	// MachineID reported by the node. For unique machine identification
	// in the cluster this field is preferred. Learn more from man(5)
	// machine-id: http://man7.org/linux/man-pages/man5/machine-id.5.html
	MachineID string `json:"machineID" protobuf:"bytes,1,opt,name=machineID"`
	// SystemUUID reported by the node. For unique machine identification
	// MachineID is preferred. This field is specific to Red Hat hosts
	// https://access.redhat.com/documentation/en-US/Red_Hat_Subscription_Management/1/html/RHSM/getting-system-uuid.html
	SystemUUID string `json:"systemUUID" protobuf:"bytes,2,opt,name=systemUUID"`
	// Boot ID reported by the node.
	BootID string `json:"bootID" protobuf:"bytes,3,opt,name=bootID"`
	// Kernel Version reported by the node from 'uname -r' (e.g. 3.16.0-0.bpo.4-amd64).
	KernelVersion string `json:"kernelVersion" protobuf:"bytes,4,opt,name=kernelVersion"`
	// OS Image reported by the node from /etc/os-release (e.g. Debian GNU/Linux 7 (wheezy)).
	OSImage string `json:"osImage" protobuf:"bytes,5,opt,name=osImage"`
	// ContainerRuntime Version reported by the node through runtime remote API (e.g. docker://1.5.0).
	ContainerRuntimeVersion string `json:"containerRuntimeVersion" protobuf:"bytes,6,opt,name=containerRuntimeVersion"`
	// Kubelet Version reported by the node.
	KubeletVersion string `json:"kubeletVersion" protobuf:"bytes,7,opt,name=kubeletVersion"`
	// KubeProxy Version reported by the node.
	KubeProxyVersion string `json:"kubeProxyVersion" protobuf:"bytes,8,opt,name=kubeProxyVersion"`
	// The Operating System reported by the node
	OperatingSystem string `json:"operatingSystem" protobuf:"bytes,9,opt,name=operatingSystem"`
	// The Architecture reported by the node
	Architecture string `json:"architecture" protobuf:"bytes,10,opt,name=architecture"`
}

// NodeConfigStatus describes the status of the config assigned by Node.Spec.ConfigSource.
type NodeConfigStatus struct {
	// Assigned reports the checkpointed config the node will try to use.
	// When Node.Spec.ConfigSource is updated, the node checkpoints the associated
	// config payload to local disk, along with a record indicating intended
	// config. The node refers to this record to choose its config checkpoint, and
	// reports this record in Assigned. Assigned only updates in the status after
	// the record has been checkpointed to disk. When the Kubelet is restarted,
	// it tries to make the Assigned config the Active config by loading and
	// validating the checkpointed payload identified by Assigned.
	// +optional
	Assigned *NodeConfigSource `json:"assigned,omitempty" protobuf:"bytes,1,opt,name=assigned"`
	// Active reports the checkpointed config the node is actively using.
	// Active will represent either the current version of the Assigned config,
	// or the current LastKnownGood config, depending on whether attempting to use the
	// Assigned config results in an error.
	// +optional
	Active *NodeConfigSource `json:"active,omitempty" protobuf:"bytes,2,opt,name=active"`
	// LastKnownGood reports the checkpointed config the node will fall back to
	// when it encounters an error attempting to use the Assigned config.
	// The Assigned config becomes the LastKnownGood config when the node determines
	// that the Assigned config is stable and correct.
	// This is currently implemented as a 10-minute soak period starting when the local
	// record of Assigned config is updated. If the Assigned config is Active at the end
	// of this period, it becomes the LastKnownGood. Note that if Spec.ConfigSource is
	// reset to nil (use local defaults), the LastKnownGood is also immediately reset to nil,
	// because the local default config is always assumed good.
	// You should not make assumptions about the node's method of determining config stability
	// and correctness, as this may change or become configurable in the future.
	// +optional
	LastKnownGood *NodeConfigSource `json:"lastKnownGood,omitempty" protobuf:"bytes,3,opt,name=lastKnownGood"`
	// Error describes any problems reconciling the Spec.ConfigSource to the Active config.
	// Errors may occur, for example, attempting to checkpoint Spec.ConfigSource to the local Assigned
	// record, attempting to checkpoint the payload associated with Spec.ConfigSource, attempting
	// to load or validate the Assigned config, etc.
	// Errors may occur at different points while syncing config. Earlier errors (e.g. download or
	// checkpointing errors) will not result in a rollback to LastKnownGood, and may resolve across
	// Kubelet retries. Later errors (e.g. loading or validating a checkpointed config) will result in
	// a rollback to LastKnownGood. In the latter case, it is usually possible to resolve the error
	// by fixing the config assigned in Spec.ConfigSource.
	// You can find additional information for debugging by searching the error message in the Kubelet log.
	// Error is a human-readable description of the error state; machines can check whether or not Error
	// is empty, but should not rely on the stability of the Error text across Kubelet versions.
	// +optional
	Error string `json:"error,omitempty" protobuf:"bytes,4,opt,name=error"`
}

// NodeStatus is information about the current status of a node.
type NodeStatus struct {
	// Capacity represents the total resources of a node.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#capacity
	// +optional
	Capacity ResourceList `json:"capacity,omitempty" protobuf:"bytes,1,rep,name=capacity,casttype=ResourceList,castkey=ResourceName"`
	// Allocatable represents the resources of a node that are available for scheduling.
	// Defaults to Capacity.
	// +optional
	Allocatable ResourceList `json:"allocatable,omitempty" protobuf:"bytes,2,rep,name=allocatable,casttype=ResourceList,castkey=ResourceName"`
	// NodePhase is the recently observed lifecycle phase of the node.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#phase
	// The field is never populated, and now is deprecated.
	// +optional
	Phase NodePhase `json:"phase,omitempty" protobuf:"bytes,3,opt,name=phase,casttype=NodePhase"`
	// Conditions is an array of current observed node conditions.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#condition
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []NodeCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,4,rep,name=conditions"`
	// List of addresses reachable to the node.
	// Queried from cloud provider, if available.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#addresses
	// Note: This field is declared as mergeable, but the merge key is not sufficiently
	// unique, which can cause data corruption when it is merged. Callers should instead
	// use a full-replacement patch. See http://pr.k8s.io/79391 for an example.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Addresses []NodeAddress `json:"addresses,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,5,rep,name=addresses"`
	// Endpoints of daemons running on the Node.
	// +optional
	DaemonEndpoints NodeDaemonEndpoints `json:"daemonEndpoints,omitempty" protobuf:"bytes,6,opt,name=daemonEndpoints"`
	// Set of ids/uuids to uniquely identify the node.
	// More info: https://kubernetes.io/docs/concepts/nodes/node/#info
	// +optional
	NodeInfo NodeSystemInfo `json:"nodeInfo,omitempty" protobuf:"bytes,7,opt,name=nodeInfo"`
	// List of container images on this node
	// +optional
	Images []ContainerImage `json:"images,omitempty" protobuf:"bytes,8,rep,name=images"`
	// List of attachable volumes in use (mounted) by the node.
	// +optional
	VolumesInUse []UniqueVolumeName `json:"volumesInUse,omitempty" protobuf:"bytes,9,rep,name=volumesInUse"`
	// List of volumes that are attached to the node.
	// +optional
	VolumesAttached []AttachedVolume `json:"volumesAttached,omitempty" protobuf:"bytes,10,rep,name=volumesAttached"`
	// Status of the config assigned to the node via the dynamic Kubelet config feature.
	// +optional
	Config *NodeConfigStatus `json:"config,omitempty" protobuf:"bytes,11,opt,name=config"`
}

type UniqueVolumeName string

// AttachedVolume describes a volume attached to a node
type AttachedVolume struct {
	// Name of the attached volume
	Name UniqueVolumeName `json:"name" protobuf:"bytes,1,rep,name=name"`

	// DevicePath represents the device path where the volume should be available
	DevicePath string `json:"devicePath" protobuf:"bytes,2,rep,name=devicePath"`
}

// AvoidPods describes pods that should avoid this node. This is the value for a
// Node annotation with key scheduler.alpha.kubernetes.io/preferAvoidPods and
// will eventually become a field of NodeStatus.
type AvoidPods struct {
	// Bounded-sized list of signatures of pods that should avoid this node, sorted
	// in timestamp order from oldest to newest. Size of the slice is unspecified.
	// +optional
	PreferAvoidPods []PreferAvoidPodsEntry `json:"preferAvoidPods,omitempty" protobuf:"bytes,1,rep,name=preferAvoidPods"`
}

// Describes a class of pods that should avoid this node.
type PreferAvoidPodsEntry struct {
	// The class of pods.
	PodSignature PodSignature `json:"podSignature" protobuf:"bytes,1,opt,name=podSignature"`
	// Time at which this entry was added to the list.
	// +optional
	EvictionTime metav1.Time `json:"evictionTime,omitempty" protobuf:"bytes,2,opt,name=evictionTime"`
	// (brief) reason why this entry was added to the list.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
	// Human readable message indicating why this entry was added to the list.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,4,opt,name=message"`
}

// Describes the class of pods that should avoid this node.
// Exactly one field should be set.
type PodSignature struct {
	// Reference to controller whose pods should avoid this node.
	// +optional
	PodController *metav1.OwnerReference `json:"podController,omitempty" protobuf:"bytes,1,opt,name=podController"`
}

// Describe a container image
type ContainerImage struct {
	// Names by which this image is known.
	// e.g. ["k8s.gcr.io/hyperkube:v1.0.7", "dockerhub.io/google_containers/hyperkube:v1.0.7"]
	Names []string `json:"names" protobuf:"bytes,1,rep,name=names"`
	// The size of the image in bytes.
	// +optional
	SizeBytes int64 `json:"sizeBytes,omitempty" protobuf:"varint,2,opt,name=sizeBytes"`
}

type NodePhase string

// These are the valid phases of node.
const (
	// NodePending means the node has been created/added by the system, but not configured.
	NodePending NodePhase = "Pending"
	// NodeRunning means the node has been configured and has Kubernetes components running.
	NodeRunning NodePhase = "Running"
	// NodeTerminated means the node has been removed from the cluster.
	NodeTerminated NodePhase = "Terminated"
)

type NodeConditionType string

// These are valid conditions of node. Currently, we don't have enough information to decide
// node condition. In the future, we will add more. The proposed set of conditions are:
// NodeReachable, NodeLive, NodeReady, NodeSchedulable, NodeRunnable.
const (
	// NodeReady means kubelet is healthy and ready to accept pods.
	NodeReady NodeConditionType = "Ready"
	// NodeMemoryPressure means the kubelet is under pressure due to insufficient available memory.
	NodeMemoryPressure NodeConditionType = "MemoryPressure"
	// NodeDiskPressure means the kubelet is under pressure due to insufficient available disk.
	NodeDiskPressure NodeConditionType = "DiskPressure"
	// NodePIDPressure means the kubelet is under pressure due to insufficient available PID.
	NodePIDPressure NodeConditionType = "PIDPressure"
	// NodeNetworkUnavailable means that network for the node is not correctly configured.
	NodeNetworkUnavailable NodeConditionType = "NetworkUnavailable"
)

// NodeCondition contains condition information for a node.
type NodeCondition struct {
	// Type of node condition.
	Type NodeConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=NodeConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// Last time we got an update on a given condition.
	// +optional
	LastHeartbeatTime metav1.Time `json:"lastHeartbeatTime,omitempty" protobuf:"bytes,3,opt,name=lastHeartbeatTime"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
}

type NodeAddressType string

// These are valid address type of node.
const (
	NodeHostName    NodeAddressType = "Hostname"
	NodeExternalIP  NodeAddressType = "ExternalIP"
	NodeInternalIP  NodeAddressType = "InternalIP"
	NodeExternalDNS NodeAddressType = "ExternalDNS"
	NodeInternalDNS NodeAddressType = "InternalDNS"
)

// NodeAddress contains information for the node's address.
type NodeAddress struct {
	// Node address type, one of Hostname, ExternalIP or InternalIP.
	Type NodeAddressType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=NodeAddressType"`
	// The node address.
	Address string `json:"address" protobuf:"bytes,2,opt,name=address"`
}

// ResourceName is the name identifying various resources in a ResourceList.
type ResourceName string

// Resource names must be not more than 63 characters, consisting of upper- or lower-case alphanumeric characters,
// with the -, _, and . characters allowed anywhere, except the first or last character.
// The default convention, matching that for annotations, is to use lower-case names, with dashes, rather than
// camel case, separating compound words.
// Fully-qualified resource typenames are constructed from a DNS-style subdomain, followed by a slash `/` and a name.
const (
	// CPU, in cores. (500m = .5 cores)
	ResourceCPU ResourceName = "cpu"
	// Memory, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceMemory ResourceName = "memory"
	// Volume size, in bytes (e,g. 5Gi = 5GiB = 5 * 1024 * 1024 * 1024)
	ResourceStorage ResourceName = "storage"
	// Local ephemeral storage, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	// The resource name for ResourceEphemeralStorage is alpha and it can change across releases.
	ResourceEphemeralStorage ResourceName = "ephemeral-storage"
)

const (
	// Default namespace prefix.
	ResourceDefaultNamespacePrefix = "kubernetes.io/"
	// Name prefix for huge page resources (alpha).
	ResourceHugePagesPrefix = "hugepages-"
	// Name prefix for storage resource limits
	ResourceAttachableVolumesPrefix = "attachable-volumes-"
)

// ResourceList is a set of (resource name, quantity) pairs.
type ResourceList map[ResourceName]resource.Quantity

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Node is a worker node in Kubernetes.
// Each node will have a unique identifier in the cache (i.e. in etcd).
type Node struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the behavior of a node.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec NodeSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Most recently observed status of the node.
	// Populated by the system.
	// Read-only.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status NodeStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeList is the whole list of all Nodes which have been registered with master.
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of nodes
	Items []Node `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// FinalizerName is the name identifying a finalizer during namespace lifecycle.
type FinalizerName string

// These are internal finalizer values to Kubernetes, must be qualified name unless defined here or
// in metav1.
const (
	FinalizerKubernetes FinalizerName = "kubernetes"
)

// NamespaceSpec describes the attributes on a Namespace.
type NamespaceSpec struct {
	// Finalizers is an opaque list of values that must be empty to permanently remove object from storage.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/namespaces/
	// +optional
	Finalizers []FinalizerName `json:"finalizers,omitempty" protobuf:"bytes,1,rep,name=finalizers,casttype=FinalizerName"`
}

// NamespaceStatus is information about the current status of a Namespace.
type NamespaceStatus struct {
	// Phase is the current lifecycle phase of the namespace.
	// More info: https://kubernetes.io/docs/tasks/administer-cluster/namespaces/
	// +optional
	Phase NamespacePhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=NamespacePhase"`

	// Represents the latest available observations of a namespace's current state.
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []NamespaceCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`
}

type NamespacePhase string

// These are the valid phases of a namespace.
const (
	// NamespaceActive means the namespace is available for use in the system
	NamespaceActive NamespacePhase = "Active"
	// NamespaceTerminating means the namespace is undergoing graceful termination
	NamespaceTerminating NamespacePhase = "Terminating"
)

const (
	// NamespaceTerminatingCause is returned as a defaults.cause item when a change is
	// forbidden due to the namespace being terminated.
	NamespaceTerminatingCause metav1.CauseType = "NamespaceTerminating"
)

type NamespaceConditionType string

// These are valid conditions of a namespace.
const (
	// NamespaceDeletionDiscoveryFailure contains information about namespace deleter errors during resource discovery.
	NamespaceDeletionDiscoveryFailure NamespaceConditionType = "NamespaceDeletionDiscoveryFailure"
	// NamespaceDeletionContentFailure contains information about namespace deleter errors during deletion of resources.
	NamespaceDeletionContentFailure NamespaceConditionType = "NamespaceDeletionContentFailure"
	// NamespaceDeletionGVParsingFailure contains information about namespace deleter errors parsing GV for legacy types.
	NamespaceDeletionGVParsingFailure NamespaceConditionType = "NamespaceDeletionGroupVersionParsingFailure"
	// NamespaceContentRemaining contains information about resources remaining in a namespace.
	NamespaceContentRemaining NamespaceConditionType = "NamespaceContentRemaining"
	// NamespaceFinalizersRemaining contains information about which finalizers are on resources remaining in a namespace.
	NamespaceFinalizersRemaining NamespaceConditionType = "NamespaceFinalizersRemaining"
)

// NamespaceCondition contains details about state of namespace.
type NamespaceCondition struct {
	// Type of namespace controller condition.
	Type NamespaceConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=NamespaceConditionType"`
	// Status of the condition, one of True, False, Unknown.
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,4,opt,name=lastTransitionTime"`
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,6,opt,name=message"`
}

// +genclient
// +genclient:nonNamespaced
// +genclient:skipVerbs=deleteCollection
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Namespace provides a scope for Names.
// Use of multiple namespaces is optional.
type Namespace struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the behavior of the Namespace.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec NamespaceSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status describes the current status of a Namespace.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status NamespaceStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NamespaceList is a list of Namespaces.
type NamespaceList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is the list of Namespace objects in the list.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	Items []Namespace `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Binding ties one object to another; for example, a pod is bound to a node by a scheduler.
// Deprecated in 1.7, please use the bindings subresource of pods instead.
type Binding struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// The target object that you want to bind to the standard object.
	Target ObjectReference `json:"target" protobuf:"bytes,2,opt,name=target"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// A list of ephemeral containers used with the Pod ephemeralcontainers subresource.
type EphemeralContainers struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// A list of ephemeral containers associated with this pod. New ephemeral containers
	// may be appended to this list, but existing ephemeral containers may not be removed
	// or modified.
	// +patchMergeKey=name
	// +patchStrategy=merge
	EphemeralContainers []EphemeralContainer `json:"ephemeralContainers" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,2,rep,name=ephemeralContainers"`
}

// Preconditions must be fulfilled before an operation (update, delete, etc.) is carried out.
// +k8s:openapi-gen=false
type Preconditions struct {
	// Specifies the target UID.
	// +optional
	UID *types.UID `json:"uid,omitempty" protobuf:"bytes,1,opt,name=uid,casttype=k8s.io/apimachinery/pkg/types.UID"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodLogOptions is the query options for a Pod's logs REST call.
type PodLogOptions struct {
	metav1.TypeMeta `json:",inline"`

	// The container for which to stream logs. Defaults to only container if there is one container in the pod.
	// +optional
	Container string `json:"container,omitempty" protobuf:"bytes,1,opt,name=container"`
	// Follow the log stream of the pod. Defaults to false.
	// +optional
	Follow bool `json:"follow,omitempty" protobuf:"varint,2,opt,name=follow"`
	// Return previous terminated container logs. Defaults to false.
	// +optional
	Previous bool `json:"previous,omitempty" protobuf:"varint,3,opt,name=previous"`
	// A relative time in seconds before the current time from which to show logs. If this value
	// precedes the time a pod was started, only logs since the pod start will be returned.
	// If this value is in the future, no logs will be returned.
	// Only one of sinceSeconds or sinceTime may be specified.
	// +optional
	SinceSeconds *int64 `json:"sinceSeconds,omitempty" protobuf:"varint,4,opt,name=sinceSeconds"`
	// An RFC3339 timestamp from which to show logs. If this value
	// precedes the time a pod was started, only logs since the pod start will be returned.
	// If this value is in the future, no logs will be returned.
	// Only one of sinceSeconds or sinceTime may be specified.
	// +optional
	SinceTime *metav1.Time `json:"sinceTime,omitempty" protobuf:"bytes,5,opt,name=sinceTime"`
	// If true, add an RFC3339 or RFC3339Nano timestamp at the beginning of every line
	// of log output. Defaults to false.
	// +optional
	Timestamps bool `json:"timestamps,omitempty" protobuf:"varint,6,opt,name=timestamps"`
	// If set, the number of lines from the end of the logs to show. If not specified,
	// logs are shown from the creation of the container or sinceSeconds or sinceTime
	// +optional
	TailLines *int64 `json:"tailLines,omitempty" protobuf:"varint,7,opt,name=tailLines"`
	// If set, the number of bytes to read from the server before terminating the
	// log output. This may not display a complete final line of logging, and may return
	// slightly more or slightly less than the specified limit.
	// +optional
	LimitBytes *int64 `json:"limitBytes,omitempty" protobuf:"varint,8,opt,name=limitBytes"`

	// insecureSkipTLSVerifyBackend indicates that the apiserver should not confirm the validity of the
	// serving certificate of the backend it is connecting to.  This will make the HTTPS connection between the apiserver
	// and the backend insecure. This means the apiserver cannot verify the log data it is receiving came from the real
	// kubelet.  If the kubelet is configured to verify the apiserver's TLS credentials, it does not mean the
	// connection to the real kubelet is vulnerable to a man in the middle attack (e.g. an attacker could not intercept
	// the actual log data coming from the real kubelet).
	// +optional
	InsecureSkipTLSVerifyBackend bool `json:"insecureSkipTLSVerifyBackend,omitempty" protobuf:"varint,9,opt,name=insecureSkipTLSVerifyBackend"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodAttachOptions is the query options to a Pod's remote attach call.
// ---
// TODO: merge w/ PodExecOptions below for stdin, stdout, etc
// and also when we cut V2, we should export a "StreamOptions" or somesuch that contains Stdin, Stdout, Stder and TTY
type PodAttachOptions struct {
	metav1.TypeMeta `json:",inline"`

	// Stdin if true, redirects the standard input stream of the pod for this call.
	// Defaults to false.
	// +optional
	Stdin bool `json:"stdin,omitempty" protobuf:"varint,1,opt,name=stdin"`

	// Stdout if true indicates that stdout is to be redirected for the attach call.
	// Defaults to true.
	// +optional
	Stdout bool `json:"stdout,omitempty" protobuf:"varint,2,opt,name=stdout"`

	// Stderr if true indicates that stderr is to be redirected for the attach call.
	// Defaults to true.
	// +optional
	Stderr bool `json:"stderr,omitempty" protobuf:"varint,3,opt,name=stderr"`

	// TTY if true indicates that a tty will be allocated for the attach call.
	// This is passed through the container runtime so the tty
	// is allocated on the worker node by the container runtime.
	// Defaults to false.
	// +optional
	TTY bool `json:"tty,omitempty" protobuf:"varint,4,opt,name=tty"`

	// The container in which to execute the command.
	// Defaults to only container if there is only one container in the pod.
	// +optional
	Container string `json:"container,omitempty" protobuf:"bytes,5,opt,name=container"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodExecOptions is the query options to a Pod's remote exec call.
// ---
// TODO: This is largely identical to PodAttachOptions above, make sure they stay in sync and see about merging
// and also when we cut V2, we should export a "StreamOptions" or somesuch that contains Stdin, Stdout, Stder and TTY
type PodExecOptions struct {
	metav1.TypeMeta `json:",inline"`

	// Redirect the standard input stream of the pod for this call.
	// Defaults to false.
	// +optional
	Stdin bool `json:"stdin,omitempty" protobuf:"varint,1,opt,name=stdin"`

	// Redirect the standard output stream of the pod for this call.
	// Defaults to true.
	// +optional
	Stdout bool `json:"stdout,omitempty" protobuf:"varint,2,opt,name=stdout"`

	// Redirect the standard error stream of the pod for this call.
	// Defaults to true.
	// +optional
	Stderr bool `json:"stderr,omitempty" protobuf:"varint,3,opt,name=stderr"`

	// TTY if true indicates that a tty will be allocated for the exec call.
	// Defaults to false.
	// +optional
	TTY bool `json:"tty,omitempty" protobuf:"varint,4,opt,name=tty"`

	// Container in which to execute the command.
	// Defaults to only container if there is only one container in the pod.
	// +optional
	Container string `json:"container,omitempty" protobuf:"bytes,5,opt,name=container"`

	// Command is the remote command to execute. argv array. Not executed within a shell.
	Command []string `json:"command" protobuf:"bytes,6,rep,name=command"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodPortForwardOptions is the query options to a Pod's port forward call
// when using WebSockets.
// The `port` query parameter must specify the port or
// ports (comma separated) to forward over.
// Port forwarding over SPDY does not use these options. It requires the port
// to be passed in the `port` header as part of request.
type PodPortForwardOptions struct {
	metav1.TypeMeta `json:",inline"`

	// List of ports to forward
	// Required when using WebSockets
	// +optional
	Ports []int32 `json:"ports,omitempty" protobuf:"varint,1,rep,name=ports"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PodProxyOptions is the query options to a Pod's proxy call.
type PodProxyOptions struct {
	metav1.TypeMeta `json:",inline"`

	// Path is the URL path to use for the current proxy request to pod.
	// +optional
	Path string `json:"path,omitempty" protobuf:"bytes,1,opt,name=path"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NodeProxyOptions is the query options to a Node's proxy call.
type NodeProxyOptions struct {
	metav1.TypeMeta `json:",inline"`

	// Path is the URL path to use for the current proxy request to node.
	// +optional
	Path string `json:"path,omitempty" protobuf:"bytes,1,opt,name=path"`
}

// +k8s:conversion-gen:explicit-from=net/url.Values
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ServiceProxyOptions is the query options to a Service's proxy call.
type ServiceProxyOptions struct {
	metav1.TypeMeta `json:",inline"`

	// Path is the part of URLs that include service endpoints, suffixes,
	// and parameters to use for the current proxy request to service.
	// For example, the whole request URL is
	// http://localhost/api/v1/namespaces/kube-system/services/elasticsearch-logging/_search?q=user:kimchy.
	// Path is _search?q=user:kimchy.
	// +optional
	Path string `json:"path,omitempty" protobuf:"bytes,1,opt,name=path"`
}

// ObjectReference contains enough information to let you inspect or modify the referred object.
// ---
// New uses of this type are discouraged because of difficulty describing its usage when embedded in APIs.
//  1. Ignored fields.  It includes many fields which are not generally honored.  For instance, ResourceVersion and FieldPath are both very rarely valid in actual usage.
//  2. Invalid usage help.  It is impossible to add specific help for individual usage.  In most embedded usages, there are particular
//     restrictions like, "must refer only to types A and B" or "UID not honored" or "name must be restricted".
//     Those cannot be well described when embedded.
//  3. Inconsistent validation.  Because the usages are different, the validation rules are different by usage, which makes it hard for users to predict what will happen.
//  4. The fields are both imprecise and overly precise.  Kind is not a precise mapping to a URL. This can produce ambiguity
//     during interpretation and require a REST mapping.  In most cases, the dependency is on the group,resource tuple
//     and the version of the actual struct is irrelevant.
//  5. We cannot easily change it.  Because this type is embedded in many locations, updates to this type
//     will affect numerous schemas.  Don't make new APIs embed an underspecified API type they do not control.
// Instead of using this type, create a locally provided and used type that is well-focused on your reference.
// For example, ServiceReferences for admission registration: https://github.com/kubernetes/api/blob/release-1.17/admissionregistration/v1/types.go#L533 .
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type ObjectReference struct {
	// Kind of the referent.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	Kind string `json:"kind,omitempty" protobuf:"bytes,1,opt,name=kind"`
	// Namespace of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/
	// +optional
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,2,opt,name=namespace"`
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,3,opt,name=name"`
	// UID of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#uids
	// +optional
	UID types.UID `json:"uid,omitempty" protobuf:"bytes,4,opt,name=uid,casttype=k8s.io/apimachinery/pkg/types.UID"`
	// API version of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty" protobuf:"bytes,5,opt,name=apiVersion"`
	// Specific resourceVersion to which this reference is made, if any.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#concurrency-control-and-consistency
	// +optional
	ResourceVersion string `json:"resourceVersion,omitempty" protobuf:"bytes,6,opt,name=resourceVersion"`

	// If referring to a piece of an object instead of an entire object, this string
	// should contain a valid JSON/Go field access statement, such as desiredState.manifest.containers[2].
	// For example, if the object reference is to a container within a pod, this would take on a value like:
	// "spec.containers{name}" (where "name" refers to the name of the container that triggered
	// the event) or if no container name is specified "spec.containers[2]" (container with
	// index 2 in this pod). This syntax is chosen only to have some well-defined way of
	// referencing a part of an object.
	// TODO: this design is not final and this field is subject to change in the future.
	// +optional
	FieldPath string `json:"fieldPath,omitempty" protobuf:"bytes,7,opt,name=fieldPath"`
}

// LocalObjectReference contains enough information to let you locate the
// referenced object inside the same namespace.
type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// TODO: Add other useful fields. apiVersion, kind, uid?
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}

// TypedLocalObjectReference contains enough information to let you locate the
// typed referenced object inside the same namespace.
type TypedLocalObjectReference struct {
	// APIGroup is the group for the resource being referenced.
	// If APIGroup is not specified, the specified Kind must be in the core API group.
	// For any other third-party types, APIGroup is required.
	// +optional
	APIGroup *string `json:"apiGroup" protobuf:"bytes,1,opt,name=apiGroup"`
	// Kind is the type of resource being referenced
	Kind string `json:"kind" protobuf:"bytes,2,opt,name=kind"`
	// Name is the name of resource being referenced
	Name string `json:"name" protobuf:"bytes,3,opt,name=name"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SerializedReference is a reference to serialized object.
type SerializedReference struct {
	metav1.TypeMeta `json:",inline"`
	// The reference to an object in the system.
	// +optional
	Reference ObjectReference `json:"reference,omitempty" protobuf:"bytes,1,opt,name=reference"`
}

// EventSource contains information for an event.
type EventSource struct {
	// Component from which the event is generated.
	// +optional
	Component string `json:"component,omitempty" protobuf:"bytes,1,opt,name=component"`
	// Node name on which the event is generated.
	// +optional
	Host string `json:"host,omitempty" protobuf:"bytes,2,opt,name=host"`
}

// Valid values for event types (new types could be added in future)
const (
	// Information only and will not cause any problems
	EventTypeNormal string = "Normal"
	// These events are to warn that something might go wrong
	EventTypeWarning string = "Warning"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Event is a report of an event somewhere in the cluster.
type Event struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata" protobuf:"bytes,1,opt,name=metadata"`

	// The object that this event is about.
	InvolvedObject ObjectReference `json:"involvedObject" protobuf:"bytes,2,opt,name=involvedObject"`

	// This should be a short, machine understandable string that gives the reason
	// for the transition into the object's current status.
	// TODO: provide exact specification for format.
	// +optional
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`

	// A human-readable description of the status of this operation.
	// TODO: decide on maximum length.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,4,opt,name=message"`

	// The component reporting this event. Should be a short machine understandable string.
	// +optional
	Source EventSource `json:"source,omitempty" protobuf:"bytes,5,opt,name=source"`

	// The time at which the event was first recorded. (Time of server receipt is in TypeMeta.)
	// +optional
	FirstTimestamp metav1.Time `json:"firstTimestamp,omitempty" protobuf:"bytes,6,opt,name=firstTimestamp"`

	// The time at which the most recent occurrence of this event was recorded.
	// +optional
	LastTimestamp metav1.Time `json:"lastTimestamp,omitempty" protobuf:"bytes,7,opt,name=lastTimestamp"`

	// The number of times this event has occurred.
	// +optional
	Count int32 `json:"count,omitempty" protobuf:"varint,8,opt,name=count"`

	// Type of this event (Normal, Warning), new types could be added in the future
	// +optional
	Type string `json:"type,omitempty" protobuf:"bytes,9,opt,name=type"`

	// Time when this Event was first observed.
	// +optional
	EventTime metav1.MicroTime `json:"eventTime,omitempty" protobuf:"bytes,10,opt,name=eventTime"`

	// Data about the Event series this event represents or nil if it's a singleton Event.
	// +optional
	Series *EventSeries `json:"series,omitempty" protobuf:"bytes,11,opt,name=series"`

	// What action was taken/failed regarding to the Regarding object.
	// +optional
	Action string `json:"action,omitempty" protobuf:"bytes,12,opt,name=action"`

	// Optional secondary object for more complex actions.
	// +optional
	Related *ObjectReference `json:"related,omitempty" protobuf:"bytes,13,opt,name=related"`

	// Name of the controller that emitted this Event, e.g. `kubernetes.io/kubelet`.
	// +optional
	ReportingController string `json:"reportingComponent" protobuf:"bytes,14,opt,name=reportingComponent"`

	// ID of the controller instance, e.g. `kubelet-xyzf`.
	// +optional
	ReportingInstance string `json:"reportingInstance" protobuf:"bytes,15,opt,name=reportingInstance"`
}

// EventSeries contain information on series of events, i.e. thing that was/is happening
// continuously for some time.
type EventSeries struct {
	// Number of occurrences in this series up to the last heartbeat time
	Count int32 `json:"count,omitempty" protobuf:"varint,1,name=count"`
	// Time of the last occurrence observed
	LastObservedTime metav1.MicroTime `json:"lastObservedTime,omitempty" protobuf:"bytes,2,name=lastObservedTime"`
	// State of this Series: Ongoing or Finished
	// Deprecated. Planned removal for 1.18
	State EventSeriesState `json:"state,omitempty" protobuf:"bytes,3,name=state"`
}

type EventSeriesState string

const (
	EventSeriesStateOngoing  EventSeriesState = "Ongoing"
	EventSeriesStateFinished EventSeriesState = "Finished"
	EventSeriesStateUnknown  EventSeriesState = "Unknown"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EventList is a list of events.
type EventList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of events
	Items []Event `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// List holds a list of objects, which may not be known by the server.
type List metav1.List

// LimitType is a type of object that is limited
type LimitType string

const (
	// Limit that applies to all pods in a namespace
	LimitTypePod LimitType = "Pod"
	// Limit that applies to all containers in a namespace
	LimitTypeContainer LimitType = "Container"
	// Limit that applies to all persistent volume claims in a namespace
	LimitTypePersistentVolumeClaim LimitType = "PersistentVolumeClaim"
)

// LimitRangeItem defines a min/max usage limit for any resource that matches on kind.
type LimitRangeItem struct {
	// Type of resource that this limit applies to.
	Type LimitType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=LimitType"`
	// Max usage constraints on this kind by resource name.
	// +optional
	Max ResourceList `json:"max,omitempty" protobuf:"bytes,2,rep,name=max,casttype=ResourceList,castkey=ResourceName"`
	// Min usage constraints on this kind by resource name.
	// +optional
	Min ResourceList `json:"min,omitempty" protobuf:"bytes,3,rep,name=min,casttype=ResourceList,castkey=ResourceName"`
	// Default resource requirement limit value by resource name if resource limit is omitted.
	// +optional
	Default ResourceList `json:"default,omitempty" protobuf:"bytes,4,rep,name=default,casttype=ResourceList,castkey=ResourceName"`
	// DefaultRequest is the default resource requirement request value by resource name if resource request is omitted.
	// +optional
	DefaultRequest ResourceList `json:"defaultRequest,omitempty" protobuf:"bytes,5,rep,name=defaultRequest,casttype=ResourceList,castkey=ResourceName"`
	// MaxLimitRequestRatio if specified, the named resource must have a request and limit that are both non-zero where limit divided by request is less than or equal to the enumerated value; this represents the max burst for the named resource.
	// +optional
	MaxLimitRequestRatio ResourceList `json:"maxLimitRequestRatio,omitempty" protobuf:"bytes,6,rep,name=maxLimitRequestRatio,casttype=ResourceList,castkey=ResourceName"`
}

// LimitRangeSpec defines a min/max usage limit for resources that match on kind.
type LimitRangeSpec struct {
	// Limits is the list of LimitRangeItem objects that are enforced.
	Limits []LimitRangeItem `json:"limits" protobuf:"bytes,1,rep,name=limits"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LimitRange sets resource usage limits for each kind of resource in a Namespace.
type LimitRange struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the limits enforced.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec LimitRangeSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// LimitRangeList is a list of LimitRange items.
type LimitRangeList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is a list of LimitRange objects.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	Items []LimitRange `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// The following identify resource constants for Kubernetes object types
const (
	// Pods, number
	ResourcePods ResourceName = "pods"
	// Services, number
	ResourceServices ResourceName = "services"
	// ReplicationControllers, number
	ResourceReplicationControllers ResourceName = "replicationcontrollers"
	// ResourceQuotas, number
	ResourceQuotas ResourceName = "resourcequotas"
	// ResourceSecrets, number
	ResourceSecrets ResourceName = "secrets"
	// ResourceConfigMaps, number
	ResourceConfigMaps ResourceName = "configmaps"
	// ResourcePersistentVolumeClaims, number
	ResourcePersistentVolumeClaims ResourceName = "persistentvolumeclaims"
	// ResourceServicesNodePorts, number
	ResourceServicesNodePorts ResourceName = "services.nodeports"
	// ResourceServicesLoadBalancers, number
	ResourceServicesLoadBalancers ResourceName = "services.loadbalancers"
	// CPU request, in cores. (500m = .5 cores)
	ResourceRequestsCPU ResourceName = "requests.cpu"
	// Memory request, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceRequestsMemory ResourceName = "requests.memory"
	// Storage request, in bytes
	ResourceRequestsStorage ResourceName = "requests.storage"
	// Local ephemeral storage request, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceRequestsEphemeralStorage ResourceName = "requests.ephemeral-storage"
	// CPU limit, in cores. (500m = .5 cores)
	ResourceLimitsCPU ResourceName = "limits.cpu"
	// Memory limit, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceLimitsMemory ResourceName = "limits.memory"
	// Local ephemeral storage limit, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	ResourceLimitsEphemeralStorage ResourceName = "limits.ephemeral-storage"
)

// The following identify resource prefix for Kubernetes object types
const (
	// HugePages request, in bytes. (500Gi = 500GiB = 500 * 1024 * 1024 * 1024)
	// As burst is not supported for HugePages, we would only quota its request, and ignore the limit.
	ResourceRequestsHugePagesPrefix = "requests.hugepages-"
	// Default resource requests prefix
	DefaultResourceRequestsPrefix = "requests."
)

// A ResourceQuotaScope defines a filter that must match each object tracked by a quota
type ResourceQuotaScope string

const (
	// Match all pod objects where spec.activeDeadlineSeconds
	ResourceQuotaScopeTerminating ResourceQuotaScope = "Terminating"
	// Match all pod objects where !spec.activeDeadlineSeconds
	ResourceQuotaScopeNotTerminating ResourceQuotaScope = "NotTerminating"
	// Match all pod objects that have best effort quality of service
	ResourceQuotaScopeBestEffort ResourceQuotaScope = "BestEffort"
	// Match all pod objects that do not have best effort quality of service
	ResourceQuotaScopeNotBestEffort ResourceQuotaScope = "NotBestEffort"
	// Match all pod objects that have priority class mentioned
	ResourceQuotaScopePriorityClass ResourceQuotaScope = "PriorityClass"
)

// ResourceQuotaSpec defines the desired hard limits to enforce for Quota.
type ResourceQuotaSpec struct {
	// hard is the set of desired hard limits for each named resource.
	// More info: https://kubernetes.io/docs/concepts/policy/resource-quotas/
	// +optional
	Hard ResourceList `json:"hard,omitempty" protobuf:"bytes,1,rep,name=hard,casttype=ResourceList,castkey=ResourceName"`
	// A collection of filters that must match each object tracked by a quota.
	// If not specified, the quota matches all objects.
	// +optional
	Scopes []ResourceQuotaScope `json:"scopes,omitempty" protobuf:"bytes,2,rep,name=scopes,casttype=ResourceQuotaScope"`
	// scopeSelector is also a collection of filters like scopes that must match each object tracked by a quota
	// but expressed using ScopeSelectorOperator in combination with possible values.
	// For a resource to match, both scopes AND scopeSelector (if specified in spec), must be matched.
	// +optional
	ScopeSelector *ScopeSelector `json:"scopeSelector,omitempty" protobuf:"bytes,3,opt,name=scopeSelector"`
}

// A scope selector represents the AND of the selectors represented
// by the scoped-resource selector requirements.
type ScopeSelector struct {
	// A list of scope selector requirements by scope of the resources.
	// +optional
	MatchExpressions []ScopedResourceSelectorRequirement `json:"matchExpressions,omitempty" protobuf:"bytes,1,rep,name=matchExpressions"`
}

// A scoped-resource selector requirement is a selector that contains values, a scope name, and an operator
// that relates the scope name and values.
type ScopedResourceSelectorRequirement struct {
	// The name of the scope that the selector applies to.
	ScopeName ResourceQuotaScope `json:"scopeName" protobuf:"bytes,1,opt,name=scopeName"`
	// Represents a scope's relationship to a set of values.
	// Valid operators are In, NotIn, Exists, DoesNotExist.
	Operator ScopeSelectorOperator `json:"operator" protobuf:"bytes,2,opt,name=operator,casttype=ScopedResourceSelectorOperator"`
	// An array of string values. If the operator is In or NotIn,
	// the values array must be non-empty. If the operator is Exists or DoesNotExist,
	// the values array must be empty.
	// This array is replaced during a strategic merge patch.
	// +optional
	Values []string `json:"values,omitempty" protobuf:"bytes,3,rep,name=values"`
}

// A scope selector operator is the set of operators that can be used in
// a scope selector requirement.
type ScopeSelectorOperator string

const (
	ScopeSelectorOpIn           ScopeSelectorOperator = "In"
	ScopeSelectorOpNotIn        ScopeSelectorOperator = "NotIn"
	ScopeSelectorOpExists       ScopeSelectorOperator = "Exists"
	ScopeSelectorOpDoesNotExist ScopeSelectorOperator = "DoesNotExist"
)

// ResourceQuotaStatus defines the enforced hard limits and observed use.
type ResourceQuotaStatus struct {
	// Hard is the set of enforced hard limits for each named resource.
	// More info: https://kubernetes.io/docs/concepts/policy/resource-quotas/
	// +optional
	Hard ResourceList `json:"hard,omitempty" protobuf:"bytes,1,rep,name=hard,casttype=ResourceList,castkey=ResourceName"`
	// Used is the current observed total usage of the resource in the namespace.
	// +optional
	Used ResourceList `json:"used,omitempty" protobuf:"bytes,2,rep,name=used,casttype=ResourceList,castkey=ResourceName"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceQuota sets aggregate quota restrictions enforced per namespace
type ResourceQuota struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Spec defines the desired quota.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Spec ResourceQuotaSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`

	// Status defines the actual enforced quota and its current usage.
	// https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#spec-and-status
	// +optional
	Status ResourceQuotaStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ResourceQuotaList is a list of ResourceQuota items.
type ResourceQuotaList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is a list of ResourceQuota objects.
	// More info: https://kubernetes.io/docs/concepts/policy/resource-quotas/
	Items []ResourceQuota `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Secret holds secret data of a certain type. The total bytes of the values in
// the Data field must be less than MaxSecretSize bytes.
type Secret struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Immutable, if set to true, ensures that data stored in the Secret cannot
	// be updated (only object metadata can be modified).
	// If not set to true, the field can be modified at any time.
	// Defaulted to nil.
	// This is an alpha field enabled by ImmutableEphemeralVolumes feature gate.
	// +optional
	Immutable *bool `json:"immutable,omitempty" protobuf:"varint,5,opt,name=immutable"`

	// Data contains the secret data. Each key must consist of alphanumeric
	// characters, '-', '_' or '.'. The serialized form of the secret data is a
	// base64 encoded string, representing the arbitrary (possibly non-string)
	// data value here. Described in https://tools.ietf.org/html/rfc4648#section-4
	// +optional
	Data map[string][]byte `json:"data,omitempty" protobuf:"bytes,2,rep,name=data"`

	// stringData allows specifying non-binary secret data in string form.
	// It is provided as a write-only convenience method.
	// All keys and values are merged into the data field on write, overwriting any existing values.
	// It is never output when reading from the API.
	// +k8s:conversion-gen=false
	// +optional
	StringData map[string]string `json:"stringData,omitempty" protobuf:"bytes,4,rep,name=stringData"`

	// Used to facilitate programmatic handling of secret data.
	// +optional
	Type SecretType `json:"type,omitempty" protobuf:"bytes,3,opt,name=type,casttype=SecretType"`
}

const MaxSecretSize = 1 * 1024 * 1024

type SecretType string

const (
	// SecretTypeOpaque is the default. Arbitrary user-defined data
	SecretTypeOpaque SecretType = "Opaque"

	// SecretTypeServiceAccountToken contains a token that identifies a service account to the API
	//
	// Required fields:
	// - Secret.Annotations["kubernetes.io/service-account.name"] - the name of the ServiceAccount the token identifies
	// - Secret.Annotations["kubernetes.io/service-account.uid"] - the UID of the ServiceAccount the token identifies
	// - Secret.Data["token"] - a token that identifies the service account to the API
	SecretTypeServiceAccountToken SecretType = "kubernetes.io/service-account-token"

	// ServiceAccountNameKey is the key of the required annotation for SecretTypeServiceAccountToken secrets
	ServiceAccountNameKey = "kubernetes.io/service-account.name"
	// ServiceAccountUIDKey is the key of the required annotation for SecretTypeServiceAccountToken secrets
	ServiceAccountUIDKey = "kubernetes.io/service-account.uid"
	// ServiceAccountTokenKey is the key of the required data for SecretTypeServiceAccountToken secrets
	ServiceAccountTokenKey = "token"
	// ServiceAccountKubeconfigKey is the key of the optional kubeconfig data for SecretTypeServiceAccountToken secrets
	ServiceAccountKubeconfigKey = "kubernetes.kubeconfig"
	// ServiceAccountRootCAKey is the key of the optional root certificate authority for SecretTypeServiceAccountToken secrets
	ServiceAccountRootCAKey = "ca.crt"
	// ServiceAccountNamespaceKey is the key of the optional namespace to use as the default for namespaced API calls
	ServiceAccountNamespaceKey = "namespace"

	// SecretTypeDockercfg contains a dockercfg file that follows the same format rules as ~/.dockercfg
	//
	// Required fields:
	// - Secret.Data[".dockercfg"] - a serialized ~/.dockercfg file
	SecretTypeDockercfg SecretType = "kubernetes.io/dockercfg"

	// DockerConfigKey is the key of the required data for SecretTypeDockercfg secrets
	DockerConfigKey = ".dockercfg"

	// SecretTypeDockerConfigJson contains a dockercfg file that follows the same format rules as ~/.docker/config.json
	//
	// Required fields:
	// - Secret.Data[".dockerconfigjson"] - a serialized ~/.docker/config.json file
	SecretTypeDockerConfigJson SecretType = "kubernetes.io/dockerconfigjson"

	// DockerConfigJsonKey is the key of the required data for SecretTypeDockerConfigJson secrets
	DockerConfigJsonKey = ".dockerconfigjson"

	// SecretTypeBasicAuth contains data needed for basic authentication.
	//
	// Required at least one of fields:
	// - Secret.Data["username"] - username used for authentication
	// - Secret.Data["password"] - password or token needed for authentication
	SecretTypeBasicAuth SecretType = "kubernetes.io/basic-auth"

	// BasicAuthUsernameKey is the key of the username for SecretTypeBasicAuth secrets
	BasicAuthUsernameKey = "username"
	// BasicAuthPasswordKey is the key of the password or token for SecretTypeBasicAuth secrets
	BasicAuthPasswordKey = "password"

	// SecretTypeSSHAuth contains data needed for SSH authetication.
	//
	// Required field:
	// - Secret.Data["ssh-privatekey"] - private SSH key needed for authentication
	SecretTypeSSHAuth SecretType = "kubernetes.io/ssh-auth"

	// SSHAuthPrivateKey is the key of the required SSH private key for SecretTypeSSHAuth secrets
	SSHAuthPrivateKey = "ssh-privatekey"
	// SecretTypeTLS contains information about a TLS client or server secret. It
	// is primarily used with TLS termination of the Ingress resource, but may be
	// used in other types.
	//
	// Required fields:
	// - Secret.Data["tls.key"] - TLS private key.
	//   Secret.Data["tls.crt"] - TLS certificate.
	// TODO: Consider supporting different formats, specifying CA/destinationCA.
	SecretTypeTLS SecretType = "kubernetes.io/tls"

	// TLSCertKey is the key for tls certificates in a TLS secert.
	TLSCertKey = "tls.crt"
	// TLSPrivateKeyKey is the key for the private key field in a TLS secret.
	TLSPrivateKeyKey = "tls.key"
	// SecretTypeBootstrapToken is used during the automated bootstrap process (first
	// implemented by kubeadm). It stores tokens that are used to sign well known
	// ConfigMaps. They are used for authn.
	SecretTypeBootstrapToken SecretType = "bootstrap.kubernetes.io/token"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SecretList is a list of Secret.
type SecretList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is a list of secret objects.
	// More info: https://kubernetes.io/docs/concepts/configuration/secret
	Items []Secret `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigMap holds configuration data for pods to consume.
type ConfigMap struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Immutable, if set to true, ensures that data stored in the ConfigMap cannot
	// be updated (only object metadata can be modified).
	// If not set to true, the field can be modified at any time.
	// Defaulted to nil.
	// This is an alpha field enabled by ImmutableEphemeralVolumes feature gate.
	// +optional
	Immutable *bool `json:"immutable,omitempty" protobuf:"varint,4,opt,name=immutable"`

	// Data contains the configuration data.
	// Each key must consist of alphanumeric characters, '-', '_' or '.'.
	// Values with non-UTF-8 byte sequences must use the BinaryData field.
	// The keys stored in Data must not overlap with the keys in
	// the BinaryData field, this is enforced during validation process.
	// +optional
	Data map[string]string `json:"data,omitempty" protobuf:"bytes,2,rep,name=data"`

	// BinaryData contains the binary data.
	// Each key must consist of alphanumeric characters, '-', '_' or '.'.
	// BinaryData can contain byte sequences that are not in the UTF-8 range.
	// The keys stored in BinaryData must not overlap with the ones in
	// the Data field, this is enforced during validation process.
	// Using this field will require 1.10+ apiserver and
	// kubelet.
	// +optional
	BinaryData map[string][]byte `json:"binaryData,omitempty" protobuf:"bytes,3,rep,name=binaryData"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ConfigMapList is a resource containing a list of ConfigMap objects.
type ConfigMapList struct {
	metav1.TypeMeta `json:",inline"`

	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is the list of ConfigMaps.
	Items []ConfigMap `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// Type and constants for component health validation.
type ComponentConditionType string

// These are the valid conditions for the component.
const (
	ComponentHealthy ComponentConditionType = "Healthy"
)

// Information about the condition of a component.
type ComponentCondition struct {
	// Type of condition for a component.
	// Valid value: "Healthy"
	Type ComponentConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=ComponentConditionType"`
	// Status of the condition for a component.
	// Valid values for "Healthy": "True", "False", or "Unknown".
	Status ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=ConditionStatus"`
	// Message about the condition for a component.
	// For example, information about a health check.
	// +optional
	Message string `json:"message,omitempty" protobuf:"bytes,3,opt,name=message"`
	// Condition error code for a component.
	// For example, a health check error code.
	// +optional
	Error string `json:"error,omitempty" protobuf:"bytes,4,opt,name=error"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ComponentStatus (and ComponentStatusList) holds the cluster validation info.
type ComponentStatus struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of component conditions observed
	// +optional
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []ComponentCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,2,rep,name=conditions"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Status of all the conditions for the component as a list of ComponentStatus objects.
type ComponentStatusList struct {
	metav1.TypeMeta `json:",inline"`
	// Standard list metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds
	// +optional
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// List of ComponentStatus objects.
	Items []ComponentStatus `json:"items" protobuf:"bytes,2,rep,name=items"`
}

// DownwardAPIVolumeSource represents a volume containing downward API info.
// Downward API volumes support ownership management and SELinux relabeling.
type DownwardAPIVolumeSource struct {
	// Items is a list of downward API volume file
	// +optional
	Items []DownwardAPIVolumeFile `json:"items,omitempty" protobuf:"bytes,1,rep,name=items"`
	// Optional: mode bits to use on created files by default. Must be a
	// value between 0 and 0777. Defaults to 0644.
	// Directories within the path are not affected by this setting.
	// This might be in conflict with other options that affect the file
	// mode, like fsGroup, and the result can be other mode bits set.
	// +optional
	DefaultMode *int32 `json:"defaultMode,omitempty" protobuf:"varint,2,opt,name=defaultMode"`
}

const (
	DownwardAPIVolumeSourceDefaultMode int32 = 0644
)

// DownwardAPIVolumeFile represents information to create the file containing the pod field
type DownwardAPIVolumeFile struct {
	// Required: Path is  the relative path name of the file to be created. Must not be absolute or contain the '..' path. Must be utf-8 encoded. The first item of the relative path must not start with '..'
	Path string `json:"path" protobuf:"bytes,1,opt,name=path"`
	// Required: Selects a field of the pod: only annotations, labels, name and namespace are supported.
	// +optional
	FieldRef *ObjectFieldSelector `json:"fieldRef,omitempty" protobuf:"bytes,2,opt,name=fieldRef"`
	// Selects a resource of the container: only resources limits and requests
	// (limits.cpu, limits.memory, requests.cpu and requests.memory) are currently supported.
	// +optional
	ResourceFieldRef *ResourceFieldSelector `json:"resourceFieldRef,omitempty" protobuf:"bytes,3,opt,name=resourceFieldRef"`
	// Optional: mode bits to use on this file, must be a value between 0
	// and 0777. If not specified, the volume defaultMode will be used.
	// This might be in conflict with other options that affect the file
	// mode, like fsGroup, and the result can be other mode bits set.
	// +optional
	Mode *int32 `json:"mode,omitempty" protobuf:"varint,4,opt,name=mode"`
}

// Represents downward API info for projecting into a projected volume.
// Note that this is identical to a downwardAPI volume source without the default
// mode.
type DownwardAPIProjection struct {
	// Items is a list of DownwardAPIVolume file
	// +optional
	Items []DownwardAPIVolumeFile `json:"items,omitempty" protobuf:"bytes,1,rep,name=items"`
}

// SecurityContext holds security configuration that will be applied to a container.
// Some fields are present in both SecurityContext and PodSecurityContext.  When both
// are set, the values in SecurityContext take precedence.
type SecurityContext struct {
	// The capabilities to add/drop when running containers.
	// Defaults to the default set of capabilities granted by the container runtime.
	// +optional
	Capabilities *Capabilities `json:"capabilities,omitempty" protobuf:"bytes,1,opt,name=capabilities"`
	// Run container in privileged mode.
	// Processes in privileged containers are essentially equivalent to root on the host.
	// Defaults to false.
	// +optional
	Privileged *bool `json:"privileged,omitempty" protobuf:"varint,2,opt,name=privileged"`
	// The SELinux context to be applied to the container.
	// If unspecified, the container runtime will allocate a random SELinux context for each
	// container.  May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	SELinuxOptions *SELinuxOptions `json:"seLinuxOptions,omitempty" protobuf:"bytes,3,opt,name=seLinuxOptions"`
	// The Windows specific settings applied to all containers.
	// If unspecified, the options from the PodSecurityContext will be used.
	// If set in both SecurityContext and PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	WindowsOptions *WindowsSecurityContextOptions `json:"windowsOptions,omitempty" protobuf:"bytes,10,opt,name=windowsOptions"`
	// The UID to run the entrypoint of the container process.
	// Defaults to user specified in image metadata if unspecified.
	// May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsUser *int64 `json:"runAsUser,omitempty" protobuf:"varint,4,opt,name=runAsUser"`
	// The GID to run the entrypoint of the container process.
	// Uses runtime default if unset.
	// May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsGroup *int64 `json:"runAsGroup,omitempty" protobuf:"varint,8,opt,name=runAsGroup"`
	// Indicates that the container must run as a non-root user.
	// If true, the Kubelet will validate the image at runtime to ensure that it
	// does not run as UID 0 (root) and fail to start the container if it does.
	// If unset or false, no such validation will be performed.
	// May also be set in PodSecurityContext.  If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsNonRoot *bool `json:"runAsNonRoot,omitempty" protobuf:"varint,5,opt,name=runAsNonRoot"`
	// Whether this container has a read-only root filesystem.
	// Default is false.
	// +optional
	ReadOnlyRootFilesystem *bool `json:"readOnlyRootFilesystem,omitempty" protobuf:"varint,6,opt,name=readOnlyRootFilesystem"`
	// AllowPrivilegeEscalation controls whether a process can gain more
	// privileges than its parent process. This bool directly controls if
	// the no_new_privs flag will be set on the container process.
	// AllowPrivilegeEscalation is true always when the container is:
	// 1) run as Privileged
	// 2) has CAP_SYS_ADMIN
	// +optional
	AllowPrivilegeEscalation *bool `json:"allowPrivilegeEscalation,omitempty" protobuf:"varint,7,opt,name=allowPrivilegeEscalation"`
	// procMount denotes the type of proc mount to use for the containers.
	// The default is DefaultProcMount which uses the container runtime defaults for
	// readonly paths and masked paths.
	// This requires the ProcMountType feature flag to be enabled.
	// +optional
	ProcMount *ProcMountType `json:"procMount,omitempty" protobuf:"bytes,9,opt,name=procMount"`
}

type ProcMountType string

const (
	// DefaultProcMount uses the container runtime defaults for readonly and masked
	// paths for /proc.  Most container runtimes mask certain paths in /proc to avoid
	// accidental security exposure of special devices or information.
	DefaultProcMount ProcMountType = "Default"

	// UnmaskedProcMount bypasses the default masking behavior of the container
	// runtime and ensures the newly created /proc the container stays in tact with
	// no modifications.
	UnmaskedProcMount ProcMountType = "Unmasked"
)

// SELinuxOptions are the labels to be applied to the container
type SELinuxOptions struct {
	// User is a SELinux user label that applies to the container.
	// +optional
	User string `json:"user,omitempty" protobuf:"bytes,1,opt,name=user"`
	// Role is a SELinux role label that applies to the container.
	// +optional
	Role string `json:"role,omitempty" protobuf:"bytes,2,opt,name=role"`
	// Type is a SELinux type label that applies to the container.
	// +optional
	Type string `json:"type,omitempty" protobuf:"bytes,3,opt,name=type"`
	// Level is SELinux level label that applies to the container.
	// +optional
	Level string `json:"level,omitempty" protobuf:"bytes,4,opt,name=level"`
}

// WindowsSecurityContextOptions contain Windows-specific options and credentials.
type WindowsSecurityContextOptions struct {
	// GMSACredentialSpecName is the name of the GMSA credential spec to use.
	// +optional
	GMSACredentialSpecName *string `json:"gmsaCredentialSpecName,omitempty" protobuf:"bytes,1,opt,name=gmsaCredentialSpecName"`

	// GMSACredentialSpec is where the GMSA admission webhook
	// (https://github.com/kubernetes-sigs/windows-gmsa) inlines the contents of the
	// GMSA credential spec named by the GMSACredentialSpecName field.
	// +optional
	GMSACredentialSpec *string `json:"gmsaCredentialSpec,omitempty" protobuf:"bytes,2,opt,name=gmsaCredentialSpec"`

	// The UserName in Windows to run the entrypoint of the container process.
	// Defaults to the user specified in image metadata if unspecified.
	// May also be set in PodSecurityContext. If set in both SecurityContext and
	// PodSecurityContext, the value specified in SecurityContext takes precedence.
	// +optional
	RunAsUserName *string `json:"runAsUserName,omitempty" protobuf:"bytes,3,opt,name=runAsUserName"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RangeAllocation is not a public type.
type RangeAllocation struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Range is string that identifies the range represented by 'data'.
	Range string `json:"range" protobuf:"bytes,2,opt,name=range"`
	// Data is a bit array containing all allocated addresses in the previous segment.
	Data []byte `json:"data" protobuf:"bytes,3,opt,name=data"`
}

const (
	// "default-scheduler" is the name of default scheduler.
	DefaultSchedulerName = "default-scheduler"

	// RequiredDuringScheduling affinity is not symmetric, but there is an implicit PreferredDuringScheduling affinity rule
	// corresponding to every RequiredDuringScheduling affinity rule.
	// When the --hard-pod-affinity-weight scheduler flag is not specified,
	// DefaultHardPodAffinityWeight defines the weight of the implicit PreferredDuringScheduling affinity rule.
	DefaultHardPodAffinitySymmetricWeight int32 = 1
)

// Sysctl defines a kernel parameter to be set
type Sysctl struct {
	// Name of a property to set
	Name string `json:"name" protobuf:"bytes,1,opt,name=name"`
	// Value of a property to set
	Value string `json:"value" protobuf:"bytes,2,opt,name=value"`
}

// NodeResources is an object for conveying resource information about a node.
// see http://releases.k8s.io/HEAD/docs/design/resources.md for more details.
type NodeResources struct {
	// Capacity represents the available resources of a node
	Capacity ResourceList `protobuf:"bytes,1,rep,name=capacity,casttype=ResourceList,castkey=ResourceName"`
}

const (
	// Enable stdin for remote command execution
	ExecStdinParam = "input"
	// Enable stdout for remote command execution
	ExecStdoutParam = "output"
	// Enable stderr for remote command execution
	ExecStderrParam = "error"
	// Enable TTY for remote command execution
	ExecTTYParam = "tty"
	// Command to run for remote command execution
	ExecCommandParam = "command"

	// Name of header that specifies stream type
	StreamType = "streamType"
	// Value for streamType header for stdin stream
	StreamTypeStdin = "stdin"
	// Value for streamType header for stdout stream
	StreamTypeStdout = "stdout"
	// Value for streamType header for stderr stream
	StreamTypeStderr = "stderr"
	// Value for streamType header for data stream
	StreamTypeData = "data"
	// Value for streamType header for error stream
	StreamTypeError = "error"
	// Value for streamType header for terminal resize stream
	StreamTypeResize = "resize"

	// Name of header that specifies the port being forwarded
	PortHeader = "port"
	// Name of header that specifies a request ID used to associate the error
	// and data streams for a single forwarded connection
	PortForwardRequestIDHeader = "requestID"
)

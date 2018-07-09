# changelog

### 0.17.1 (2018-03-19)

* vcsim: add Destroy method for Folder and Datacenter types

* In progress.Reader emit final report on EOF.

* vcsim: add EventManager.QueryEvents

### 0.17.0 (2018-02-28)

* Add HostStorageSystem.AttachScsiLun method

* Avoid possible panic in Datastore.Stat (#969)

* Destroy event history collectors (#962)

* Add VirtualDiskManager.CreateChildDisk method

### 0.16.0 (2017-11-08)

* Add support for SOAP request operation ID header

* Moved ovf helpers from govc import.ovf command to ovf and nfc packages

* Added guest/toolbox (client) package

* Added toolbox package and toolbox command

* Added simulator package and vcsim command

### 0.15.0 (2017-06-19)

* WaitOptions.MaxWaitSeconds is now optional

* Support removal of ExtraConfig entries

* GuestPosixFileAttributes OwnerId and GroupId fields are now pointers,
  rather than omitempty ints to allow chown with root uid:gid

* Updated examples/ using view package

* Add DatastoreFile.TailFunc method

* Export VirtualMachine.FindSnapshot method

* Add AuthorizationManager {Enable,Disable}Methods

* Add PBM client

### 0.14.0 (2017-04-08)

* Add view.ContainerView type and methods

* Add Collector.RetrieveWithFilter method

* Add property.Filter type

* Implement EthernetCardBackingInfo for OpaqueNetwork

* Finder: support changing object root in find mode

* Add VirtualDiskManager.QueryVirtualDiskInfo

* Add performance.Manager APIs

### 0.13.0 (2017-03-02)

* Add DatastoreFileManager API wrapper

* Add HostVsanInternalSystem API wrappers

* Add Container support to view package

* Finder supports Folder recursion without specifying a path

* Add VirtualMachine.QueryConfigTarget method

* Add device option to VirtualMachine.WaitForNetIP

* Remove _Task suffix from vapp methods

### 0.12.1 (2016-12-19)

* Add DiagnosticLog helper

* Add DatastorePath helper

### 0.12.0 (2016-12-01)

* Disable use of service ticket for datastore HTTP access by default

* Attach context to HTTP requests for cancellations

* Update to vim25/6.5 API

### 0.11.4 (2016-11-15)

* Add object.AuthorizationManager methods: RetrieveRolePermissions, RetrieveAllPermissions, AddRole, RemoveRole, UpdateRole

### 0.11.3 (2016-11-08)

* Allow DatastoreFile.Follow reader to drain current body after stopping

### 0.11.2 (2016-11-01)

* Avoid possible NPE in VirtualMachine.Device method

* Add support for OpaqueNetwork type to Finder

* Add HostConfigManager.AccountManager support for ESX 5.5

### 0.11.1 (2016-10-27)

* Add Finder.ResourcePoolListAll method

### 0.11.0 (2016-10-25)

* Add object.DistributedVirtualPortgroup.Reconfigure method

### 0.10.0 (2016-10-20)

* Add option to set soap.Client.UserAgent

* Add service ticket thumbprint validation

* Update use of http.DefaultTransport fields to 1.7

* Set default locale to en_US (override with GOVMOMI_LOCALE env var)

* Add object.HostCertificateInfo (types.HostCertificateManagerCertificateInfo helpers)

* Add object.HostCertificateManager type and HostConfigManager.CertificateManager method

* Add soap.Client SetRootCAs and SetDialTLS methods

### 0.9.0 (2016-09-09)

* Add object.DatastoreFile helpers for streaming and tailing datastore files

* Add object VirtualMachine.Unregister method

* Add object.ListView methods: Add, Remove, Reset

* Update to Go 1.7 - using stdlib's context package

### 0.8.0 (2016-06-30)

* Add session.Manager.AcquireLocalTicket

* Include StoragePod in Finder.FolderList

* Add Finder methods for finding by ManagedObjectReference: Element, ObjectReference

* Add mo.ManagedObjectReference methods: Reference, String, FromString

* Add support using SessionManagerGenericServiceTicket.HostName for Datastore HTTP access

### 0.7.1 (2016-06-03)

* Fix object.ObjectName method

### 0.7.0 (2016-06-02)

* Move InventoryPath field to object.Common

* Add HostDatastoreSystem.CreateLocalDatastore method

* Add DatastoreNamespaceManager methods: CreateDirectory, DeleteDirectory

* Add HostServiceSystem

* Add HostStorageSystem methods: MarkAsSdd, MarkAsNonSdd, MarkAsLocal, MarkAsNonLocal

* Add HostStorageSystem.RescanAllHba method

### 0.6.2 (2016-05-11)

* Get complete file details in Datastore.Stat

* SOAP decoding fixes

* Add VirtualMachine.RemoveAllSnapshot

### 0.6.1 (2016-04-30)

* Fix mo.Entity interface

### 0.6.0 (2016-04-29)

* Add Common.Rename method

* Add mo.Entity interface

* Add OptionManager

* Add Finder.FolderList method

* Add VirtualMachine.WaitForNetIP method

* Add VirtualMachine.RevertToSnapshot method

* Add Datastore.Download method

### 0.5.0 (2016-03-30)

Generated fields using xsd type 'int' change to Go type 'int32'

VirtualDevice.UnitNumber field changed to pointer type

### 0.4.0 (2016-02-26)

* Add method to convert virtual device list to array with virtual device
  changes that can be used in the VirtualMachineConfigSpec.

* Make datastore cluster traversable in lister

* Add finder.DatastoreCluster methods (also known as storage pods)

* Add Drone CI check

* Add object.Datastore Type and AttachedClusterHosts methods

* Add finder.*OrDefault methods

### 0.3.0 (2016-01-16)

* Add object.VirtualNicManager wrapper

* Add object.HostVsanSystem wrapper

* Add object.HostSystem methods: EnterMaintenanceMode, ExitMaintenanceMode, Disconnect, Reconnect

* Add finder.Folder method

* Add object.Common.Destroy method

* Add object.ComputeResource.Reconfigure method

* Add license.AssignmentManager wrapper

* Add object.HostFirewallSystem wrapper

* Add object.DiagnosticManager wrapper

* Add LoginExtensionByCertificate support

* Add object.ExtensionManager

...

### 0.2.0 (2015-09-15)

* Update to vim25/6.0 API

* Stop returning children from `ManagedObjectList`

    Change the `ManagedObjectList` function in the `find` package to only
    return the managed objects specified by the path argument and not their
    children. The original behavior was used by govc's `ls` command and is
    now available in the newly added function `ManagedObjectListChildren`.

* Add retry functionality to vim25 package

* Change finder functions to no longer take varargs

    The `find` package had functions to return a list of objects, given a
    variable number of patterns. This makes it impossible to distinguish which
    patterns produced results and which ones didn't.

    In particular for govc, where multiple arguments can be passed from the
    command line, it is useful to let the user know which ones produce results
    and which ones don't.

    To evaluate multiple patterns, the user should call the find functions
    multiple times (either serially or in parallel).

* Make optional boolean fields pointers (`vim25/types`).

    False is the zero value of a boolean field, which means they are not serialized
    if the field is marked "omitempty". If the field is a pointer instead, the zero
    value will be the nil pointer, and both true and false values are serialized.

### 0.1.0 (2015-03-17)

Prior to this version the API of this library was in flux.

Notable changes w.r.t. the state of this library before March 2015 are:

* All functions that may execute a request take a `context.Context` parameter.
* The `vim25` package contains a minimal client implementation.
* The property collector and its convenience functions live in the `property` package.

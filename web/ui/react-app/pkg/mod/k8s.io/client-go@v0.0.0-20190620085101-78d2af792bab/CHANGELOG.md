TODO: This document was manually maintained so might be incomplete. The
automation effort is tracked in
https://github.com/kubernetes/test-infra/issues/5843.

Changes in `k8s.io/api` and `k8s.io/apimachinery` are mentioned here
because `k8s.io/client-go` depends on them.

# v10.0.0

**Breaking Changes:**

* Action required: client-go will no longer have bootstrap
(`k8s.io/client-go/tools/bootstrap`) related code. Any reference to it will
break. Please redirect all references to `k8s.io/bootstrap` instead.
([#67356](https://github.com/kubernetes/kubernetes/pull/67356))

* The methods `NewSelfSignedCACert` and `NewSignedCert` now use `crypto.Signer`
interface instead of `rsa.PrivateKey` for certificate creation. This is done
to allow different kind of private keys (for example: ecdsa).
([#69329](https://github.com/kubernetes/kubernetes/pull/69329))

* `GetScale` and `UpdateScale` methods have been added for `apps/v1` clients
and with this, no-verb scale clients have been removed.
([#70437](https://github.com/kubernetes/kubernetes/pull/70437))

* `k8s.io/client-go/util/cert/triple` package has been removed.
([#70966](https://github.com/kubernetes/kubernetes/pull/70966))

**New Features:**

* `unfinished_work_microseconds` is added to the workqueue metrics.
It can be used to detect stuck worker threads (kube-controller-manager runs many workqueues.).
([#70884](https://github.com/kubernetes/kubernetes/pull/70884))

* A method `GetPorts` is added to expose the ports that were forwarded.
This can be used to retrieve the locally-bound port in cases where the input was port 0.
([#67513](https://github.com/kubernetes/kubernetes/pull/67513))

* Dynamic listers and informers, that work with `runtime.Unstructured` objects,
are added. These are useful for writing generic, non-generated controllers.
([68748](https://github.com/kubernetes/kubernetes/pull/68748))

* The dynamic fake client now supports JSONPatch.
([#69330](https://github.com/kubernetes/kubernetes/pull/69330))

* The `GetBinding` method is added for pods in the fake client.
([#69412](https://github.com/kubernetes/kubernetes/pull/69412))

**Bug fixes and Improvements:**

* The `apiVersion` and action name values for fake evictions are now set.
([#69035](https://github.com/kubernetes/kubernetes/pull/69035))

* PEM files containing both TLS certificate and key can now be parsed in
arbitrary order. Previously key was always required to be first.
([#69536](https://github.com/kubernetes/kubernetes/pull/69536))

* Go clients created from a kubeconfig that specifies a `TokenFile` now
periodically reload the token from the specified file.
([#70606](https://github.com/kubernetes/kubernetes/pull/70606))

* It is now ensured that oversized data frames are not written to
spdystreams in `remotecommand.NewSPDYExecutor`.
([#70999](https://github.com/kubernetes/kubernetes/pull/70999))

* A panic occuring on calling `scheme.Convert` is fixed by populating the fake
dynamic client scheme. ([#69125](https://github.com/kubernetes/kubernetes/pull/69125))

* Add step to correctly setup permissions for the in-cluster-client-configuration example.
([#69232](https://github.com/kubernetes/kubernetes/pull/69232))

* The function `Parallelize` is deprecated. Use `ParallelizeUntil` instead.
([#68403](https://github.com/kubernetes/kubernetes/pull/68403))

* [k8s.io/apimachinery] Restrict redirect following from the apiserver to
same-host redirects, and ignore redirects in some cases.
([#66516](https://github.com/kubernetes/kubernetes/pull/66516))

## API changes

**New Features:**

* GlusterFS PersistentVolumes sources can now reference endpoints in any
namespace using the `spec.glusterfs.endpointsNamespace` field.
Ensure all kubelets are upgraded to 1.13+ before using this capability.
([#60195](https://github.com/kubernetes/kubernetes/pull/60195))

* The [dynamic audit configuration](https://github.com/kubernetes/community/blob/master/keps/sig-auth/0014-dynamic-audit-configuration.md)
API is added. ([#67547](https://github.com/kubernetes/kubernetes/pull/67547))

* A new field `EnableServiceLinks` is added to the `PodSpec` to indicate whether
information about services should be injected into pod's environment variables.
([#68754](https://github.com/kubernetes/kubernetes/pull/68754))

* `CSIPersistentVolume` feature, i.e. `PersistentVolumes` with
`CSIPersistentVolumeSource`, is GA. `CSIPersistentVolume` feature gate is now
deprecated and will be removed according to deprecation policy.
([#69929](https://github.com/kubernetes/kubernetes/pull/69929))

* Raw block volume support is promoted to beta, and enabled by default.
This is accessible via the `volumeDevices` container field in pod specs,
and the `volumeMode` field in persistent volume and persistent volume claims definitions.
([#71167](https://github.com/kubernetes/kubernetes/pull/71167))

**Bug fixes and Improvements:**

* The default value of extensions/v1beta1 Deployment's `RevisionHistoryLimit`
is set to `MaxInt32`. ([#66605](https://github.com/kubernetes/kubernetes/pull/66605))

* `procMount` field is no longer incorrectly marked as required in openapi schema.
([#69694](https://github.com/kubernetes/kubernetes/pull/69694))

* The caBundle and service fields in admission webhook API objects now correctly
indicate they are optional. ([#70138](https://github.com/kubernetes/kubernetes/pull/70138))

# v9.0.0

**Breaking Changes:**

* client-go now supports additional non-alpha-numeric characters in UserInfo
"extra" data keys. It should be updated in order to properly support extra
data containing "/" characters or other characters disallowed in HTTP headers.
Old clients sending keys which were `%`-escaped by the user will have their
values unescaped by new API servers.
([#65799](https://github.com/kubernetes/kubernetes/pull/65799))

* `apimachinery/pkg/watch.Until` has been moved to
`client-go/tools/watch.UntilWithoutRetry`. While switching please consider
using the new `client-go/tools/watch.UntilWithSync` or `client-go/tools/watch.Until`.
([#66906](https://github.com/kubernetes/kubernetes/pull/66906))

* [k8s.io/apimachinery] `Unstructured` metadata accessors now respect omitempty semantics
i.e. a field having zero value will now be removed from the unstructured metadata map.
([#67635](https://github.com/kubernetes/kubernetes/pull/67635))

* [k8s.io/apimachinery] The `ObjectConvertor` interface is now changed such that
`ConvertFieldLabel` func takes GroupVersionKind as an argument instead of just
version and kind. ([#65780](https://github.com/kubernetes/kubernetes/pull/65780))

* [k8s.io/apimachinery] componentconfig `ClientConnectionConfiguration` is
moved to `k8s.io/apimachinery/pkg/apis/config`.
([#66058](https://github.com/kubernetes/kubernetes/pull/66058))

* [k8s.io/apimachinery] Renamed ` KubeConfigFile` to `Kubeconfig` in
`ClientConnectionConfiguration`.
([#67149](https://github.com/kubernetes/kubernetes/pull/67149))

* [k8s.io/apimachinery] JSON patch no longer supports `int`.
([#63522](https://github.com/kubernetes/kubernetes/pull/63522))

**New Features:**

* Add ability to cancel leader election.
This also proves useful in integration tests where the whole app is started and
stopped in each test. ([#57932](https://github.com/kubernetes/kubernetes/pull/57932))

* An example showing how to use fake clients in tests is added.
([#65291](https://github.com/kubernetes/kubernetes/pull/65291))

* [k8s.io/apimachinery] Create and Update now support `CreateOptions` and `UpdateOptions`.
([#65105](https://github.com/kubernetes/kubernetes/pull/65105))

**Bug fixes and Improvements:**

* Decrease the amount of time it takes to modify kubeconfig
files with large amounts of contexts.
([#67093](https://github.com/kubernetes/kubernetes/pull/67093))

* The leader election client now renews timeout.
([#65094](https://github.com/kubernetes/kubernetes/pull/65094))

* Switched certificate data replacement from `REDACTED` to `DATA+OMITTED`.
([#66023](https://github.com/kubernetes/kubernetes/pull/66023))

* Fix listing in the fake dynamic client.
([#66078](https://github.com/kubernetes/kubernetes/pull/66078))

* Fix discovery so that plural names are no longer ignored if a singular name is not specified.
([#66249](https://github.com/kubernetes/kubernetes/pull/66249))

* Fix kubelet startup failure when using `ExecPlugin` in kubeconfig.
([#66395](https://github.com/kubernetes/kubernetes/pull/66395))

* Fix panic in the fake `SubjectAccessReview` client when object is nil.
([#66837](https://github.com/kubernetes/kubernetes/pull/66837))

* Periodically reload `InClusterConfig` token.
([#67359](https://github.com/kubernetes/kubernetes/pull/67359))

* [k8s.io/apimachinery] Report parsing error in json serializer.
([#63668](https://github.com/kubernetes/kubernetes/pull/63668))

* [k8s.io/apimachinery] The `metav1.ObjectMeta` accessor does not deepcopy
owner references anymore. In general, the accessor interface does not enforce
deepcopy nor does it forbid it (e.g. for `unstructured.Unstructured`).
([#64915](https://github.com/kubernetes/kubernetes/pull/64915))

* [k8s.io/apimachinery] Utility functions `SetTransportDefaults` and `DialerFor`
once again respect custom Dial functions set on transports.
([#65547](https://github.com/kubernetes/kubernetes/pull/65547))

* [k8s.io/apimachinery] Speed-up conversion function invocation by avoiding
`reflect.Call`. Action required: regenerated conversion with conversion-gen.
([#65771](https://github.com/kubernetes/kubernetes/pull/65771))

* [k8s.io/apimachinery] Establish "406 Not Acceptable" response for
unmarshable protobuf serialization error.
([#67041](https://github.com/kubernetes/kubernetes/pull/67041))

* [k8s.io/apimachinery] Immediately close the other side of the connection by
exiting once one side closes when proxying.
([#67288](https://github.com/kubernetes/kubernetes/pull/67288))


## API changes

**Breaking Changes:**

* Volume dynamic provisioning scheduling has been promoted to beta.
ACTION REQUIRED: The DynamicProvisioningScheduling alpha feature gate has been removed.
The VolumeScheduling beta feature gate is still required for this feature.
([#67432](https://github.com/kubernetes/kubernetes/pull/67432))

* The CSI file system type is no longer defaulted to ext4.
All the production drivers listed under https://kubernetes-csi.github.io/docs/Drivers.html
were inspected and should not be impacted after this change.
If you are using a driver not in that list,
please test the drivers on an updated test cluster first.
([#65499](https://github.com/kubernetes/kubernetes/pull/65499))

**New Features:**

* Support annotations for remote admission webhooks.
([#58679](https://github.com/kubernetes/kubernetes/pull/58679))

* Support both directory and block device for local volume
plugin `FileSystem` `VolumeMode`.
([#63011](https://github.com/kubernetes/kubernetes/pull/63011))

* Introduce `autoscaling/v2beta2` and `custom_metrics/v1beta2`,
which implement metric selectors for Object and Pods metrics,
as well as allowing AverageValue targets on Objects, similar to External metrics.
([#64097](https://github.com/kubernetes/kubernetes/pull/64097))

*  Add `Lease` API in the `coordination.k8s.io` API group.
([#64246](https://github.com/kubernetes/kubernetes/pull/64246))

* `ProcMount` added to `SecurityContext` and `AllowedProcMounts` added to `PodSecurityPolicy`
to allow paths in the container's `/proc` to not be masked.
([#64283](https://github.com/kubernetes/kubernetes/pull/64283))

* Add the `AuditAnnotations` field to `ImageReviewStatus` to allow the
`ImageReview` backend to return annotations to be added to the created pod.
([#64597](https://github.com/kubernetes/kubernetes/pull/64597))

* SCTP is now supported as additional protocol (alpha) alongside TCP and UDP in
Pod, Service, Endpoint, and NetworkPolicy.
([#64973](https://github.com/kubernetes/kubernetes/pull/64973))

* The `PodShareProcessNamespace` feature to configure PID namespace sharing
within a pod has been promoted to beta.
([#66507](https://github.com/kubernetes/kubernetes/pull/66507))

* Add `TTLSecondsAfterFinished` to `JobSpec` for cleaning up Jobs after they finish.
([#66840](https://github.com/kubernetes/kubernetes/pull/66840))

* Add `DataSource` and `TypedLocalObjectReference` fields to support
restoring a volume from a volume snapshot data source.
([#67087](https://github.com/kubernetes/kubernetes/pull/67087))

* `RuntimeClass` is a new API resource for defining different classes of runtimes
that may be used to run containers in the cluster.
Pods can select a `RunitmeClass` to use via the `RuntimeClassName` field.
This feature is in alpha, and the `RuntimeClass` feature gate must be enabled
in order to use it. ([#67737](https://github.com/kubernetes/kubernetes/pull/67737))

* To address the possibility dry-run requests overwhelming admission webhooks
that rely on side effects and a reconciliation mechanism, a new field is being
added to `admissionregistration.k8s.io/v1beta1.ValidatingWebhookConfiguration`
and `admissionregistration.k8s.io/v1beta1.MutatingWebhookConfiguration` so that
webhooks can explicitly register as having dry-run support.
If a dry-run request is made on a resource that triggers a non dry-run supporting
webhook, the request will be completely rejected, with "400: Bad Request".
Additionally, a new field is being added to the
`admission.k8s.io/v1beta1.AdmissionReview` API object, exposing to webhooks
whether or not the request being reviewed is a dry-run.
([#66936](https://github.com/kubernetes/kubernetes/pull/66936))

**Bug fixes and Improvements:**

* The `DisruptedPods` field in `PodDisruptionBudgetStatus` is now optional.
([#63757](https://github.com/kubernetes/kubernetes/pull/63757))

* `extensions/v1beta1` Deployment's `ProgressDeadlineSeconds` now defaults to `MaxInt32`.
([#66581](https://github.com/kubernetes/kubernetes/pull/66581))

# v8.0.0

**Breaking Changes:**

* `KUBE_API_VERSIONS` has been removed.

    * [https://github.com/kubernetes/kubernetes/pull/63165](https://github.com/kubernetes/kubernetes/pull/63165)

* The client-go/discovery `RESTMapper` has been moved to client-go/restmapper.

    * [https://github.com/kubernetes/kubernetes/pull/63507](https://github.com/kubernetes/kubernetes/pull/63507)

* `CachedDiscoveryClient` has been moved from kubectl to client-go.

    * [https://github.com/kubernetes/kubernetes/pull/63550](https://github.com/kubernetes/kubernetes/pull/63550)

* The `EventRecorder` interface is changed to include an `AnnotatedEventf` method, which can add annotations to an event.

    * [https://github.com/kubernetes/kubernetes/pull/64213](https://github.com/kubernetes/kubernetes/pull/64213)

* [k8s.io/apimachinery] The deprecated `RepairMalformedUpdates` flag has been removed.

    * [https://github.com/kubernetes/kubernetes/pull/61455](https://github.com/kubernetes/kubernetes/pull/61455)

**New Features:**

* A new easy-to-use dynamic client is added and the old dynamic client is now deprecated.

    * [https://github.com/kubernetes/kubernetes/pull/62913](https://github.com/kubernetes/kubernetes/pull/62913)

* client-go and kubectl now detect and report an error on duplicated name for user, cluster and context, while loading the kubeconfig.

    * [https://github.com/kubernetes/kubernetes/pull/60464](https://github.com/kubernetes/kubernetes/pull/60464)

* The informer code-generator now allows specifying a custom resync period for certain informer types and uses the default resync period if none is specified.

    * [https://github.com/kubernetes/kubernetes/pull/61400](https://github.com/kubernetes/kubernetes/pull/61400)

* Exec authenticator plugin now supports TLS client certificates.

    * [https://github.com/kubernetes/kubernetes/pull/61803](https://github.com/kubernetes/kubernetes/pull/61803)

* The discovery client now has a default request timeout of 32 seconds.

    * [https://github.com/kubernetes/kubernetes/pull/62733](https://github.com/kubernetes/kubernetes/pull/62733)

* The OpenStack auth config from is now read from the client config. If the client config is not available, it falls back to reading from the environment variables.

    * [https://github.com/kubernetes/kubernetes/pull/60200](https://github.com/kubernetes/kubernetes/pull/60200)

* The in-tree support for openstack credentials is now deprecated. Please use the `client-keystone-auth` from the cloud-provider-openstack repository. Details on how to use this new capability is documented [here](https://github.com/kubernetes/cloud-provider-openstack/blob/master/docs/using-client-keystone-auth.md)

    * [https://github.com/kubernetes/kubernetes/pull/64346](https://github.com/kubernetes/kubernetes/pull/64346)

**Bug fixes and Improvements:**

* 406 mime-type errors are now tolerated while attempting to load new openapi schema. This improves compatibility with older servers when creating/updating API objects.

    * [https://github.com/kubernetes/kubernetes/pull/61949](https://github.com/kubernetes/kubernetes/pull/61949)

* Removes the generated `DeleteCollection()` method for `Services` since the API does not support it.

    * [https://github.com/kubernetes/kubernetes/pull/63861](https://github.com/kubernetes/kubernetes/pull/63861)

* Event object references with apiversion now report an apiversion, instead of just the group.

    * [https://github.com/kubernetes/kubernetes/pull/63913](https://github.com/kubernetes/kubernetes/pull/63913)

    [https://github.com/kubernetes/kubernetes/pull/62462](https://github.com/kubernetes/kubernetes/pull/62462)

* [k8s.io/apimachinery] `runtime.Unstructured.UnstructuredContent()` no longer mutates the source while returning the contents.

    * [https://github.com/kubernetes/kubernetes/pull/62063](https://github.com/kubernetes/kubernetes/pull/62063)

* [k8s.io/apimachinery] Incomplete support for `uint64` is now removed. This fixes a panic encountered while using `DeepCopyJSON` with `uint64`.

    * [https://github.com/kubernetes/kubernetes/pull/62981](https://github.com/kubernetes/kubernetes/pull/62981)

* [k8s.io/apimachinery] API server can now parse `propagationPolicy` when it sent as a query parameter sent with a delete request.

    * [https://github.com/kubernetes/kubernetes/pull/63414](https://github.com/kubernetes/kubernetes/pull/63414)

* [k8s.io/apimachinery] APIServices with kube-like versions (e.g. v1, v2beta1, etc.) will be sorted appropriately within each group.

    * [https://github.com/kubernetes/kubernetes/pull/64004](https://github.com/kubernetes/kubernetes/pull/64004)

* [k8s.io/apimachinery] `int64` is the only allowed integer for printers.

    * [https://github.com/kubernetes/kubernetes/pull/64639](https://github.com/kubernetes/kubernetes/pull/64639)

## API changes

**Breaking Changes:**

* Support for `alpha.kubernetes.io/nvidia-gpu` resource which was deprecated in 1.10 is removed. Please use the resource exposed by `DevicePlugins` instead (`nvidia.com/gpu`).

    * [https://github.com/kubernetes/kubernetes/pull/61498](https://github.com/kubernetes/kubernetes/pull/61498)

* Alpha annotation for `PersistentVolume` node affinity has been removed. Update your `PersistentVolume`s to use the beta `PersistentVolume.nodeAffinity` field before upgrading.

    * [https://github.com/kubernetes/kubernetes/pull/61816](https://github.com/kubernetes/kubernetes/pull/61816)

* `ObjectMeta ` `ListOptions` `DeleteOptions` are removed from the core api group. Please use the ones in `meta/v1` instead.

    * [https://github.com/kubernetes/kubernetes/pull/61809](https://github.com/kubernetes/kubernetes/pull/61809)

* `ExternalID` in `NodeSpec` is deprecated. The externalID of the node is no longer set in the Node spec.

    * [https://github.com/kubernetes/kubernetes/pull/61877](https://github.com/kubernetes/kubernetes/pull/61877)

* PSP-related types in the `extensions/v1beta1` API group are now deprecated. It is suggested to use the `policy/v1beta1` API group instead.

    * [https://github.com/kubernetes/kubernetes/pull/61777](https://github.com/kubernetes/kubernetes/pull/61777)

**New Features:**

* `PodSecurityPolicy` now supports restricting hostPath volume mounts to be readOnly and under specific path prefixes.

    * [https://github.com/kubernetes/kubernetes/pull/58647](https://github.com/kubernetes/kubernetes/pull/58647)

* `Node.Spec.ConfigSource.ConfigMap.KubeletConfigKey` must be specified when using dynamic Kubelet config to tell the Kubelet which key of the `ConfigMap` identifies its config file.

    * [https://github.com/kubernetes/kubernetes/pull/59847](https://github.com/kubernetes/kubernetes/pull/59847)

* `serverAddressByClientCIDRs` in `meta/v1` APIGroup is now optional.

    * [https://github.com/kubernetes/kubernetes/pull/61963](https://github.com/kubernetes/kubernetes/pull/61963)

* A new field `MatchFields` is added to `NodeSelectorTerm`. Currently, it only supports `metadata.name`.

    * [https://github.com/kubernetes/kubernetes/pull/62002](https://github.com/kubernetes/kubernetes/pull/62002)

* The `PriorityClass` API is promoted to `scheduling.k8s.io/v1beta1`.

    * [https://github.com/kubernetes/kubernetes/pull/63100](https://github.com/kubernetes/kubernetes/pull/63100)

* The status of dynamic Kubelet config is now reported via `Node.Status.Config`, rather than the `KubeletConfigOk` node condition.

    * [https://github.com/kubernetes/kubernetes/pull/63314](https://github.com/kubernetes/kubernetes/pull/63314)

* The `GitRepo` volume type is deprecated. To provision a container with a git repo, mount an `EmptyDir` into an `InitContainer` that clones the repo using git, then mount the `EmptyDir` into the Pod's container.

    * [https://github.com/kubernetes/kubernetes/pull/63445](https://github.com/kubernetes/kubernetes/pull/63445)

* The Sysctls experimental feature has been promoted to beta (enabled by default via the `Sysctls` feature flag). `PodSecurityPolicy` and `Pod` objects now have fields for specifying and controlling sysctls. Alpha sysctl annotations will be ignored by 1.11+ kubelets. All alpha sysctl annotations in existing deployments must be converted to API fields to be effective.

    * [https://github.com/kubernetes/kubernetes/pull/63717](https://github.com/kubernetes/kubernetes/pull/63717)

* The annotation `service.alpha.kubernetes.io/tolerate-unready-endpoints` is deprecated.  Users should use `Service.spec.publishNotReadyAddresses` instead.

    * [https://github.com/kubernetes/kubernetes/pull/63742](https://github.com/kubernetes/kubernetes/pull/63742)

* `VerticalPodAutoscaler` has been added to `autoscaling/v1` API group.

    * [https://github.com/kubernetes/kubernetes/pull/63797](https://github.com/kubernetes/kubernetes/pull/63797)

* Alpha support is added for dynamic volume limits based on node type.

    * [https://github.com/kubernetes/kubernetes/pull/64154](https://github.com/kubernetes/kubernetes/pull/64154)

* `ContainersReady` condition is added to the Pod status.

    * [https://github.com/kubernetes/kubernetes/pull/64646](https://github.com/kubernetes/kubernetes/pull/64646)

**Bug fixes and Improvements:**

* Default mount propagation has changed from `HostToContainer` (`rslave` in Linux terminology) to `None` (`private`) to match the behavior in 1.9 and earlier releases. `HostToContainer` as a default caused regressions in some pods.

    * [https://github.com/kubernetes/kubernetes/pull/62462](https://github.com/kubernetes/kubernetes/pull/62462)

# v7.0.0

**Breaking Changes:**

* Google Cloud Service Account email addresses can now be used in RBAC Role bindings since the default scopes now include the `userinfo.email` scope. This is a breaking change if the numeric uniqueIDs of the Google service accounts were being used in RBAC role bindings. The behavior can be overridden by explicitly specifying the scope values as comma-separated string in the `users[*].config.scopes` field in the `KUBECONFIG` file.

    * [https://github.com/kubernetes/kubernetes/pull/58141](https://github.com/kubernetes/kubernetes/pull/58141)

* [k8s.io/api] The `ConfigOK` node condition has been renamed to `KubeletConfigOk`.

    * [https://github.com/kubernetes/kubernetes/pull/59905](https://github.com/kubernetes/kubernetes/pull/59905)

**New Features:**

* Subresource support is added to the dynamic client.

    * [https://github.com/kubernetes/kubernetes/pull/56717](https://github.com/kubernetes/kubernetes/pull/56717)

* A watch method is added to the Fake Client.

    * [https://github.com/kubernetes/kubernetes/pull/57504](https://github.com/kubernetes/kubernetes/pull/57504)

* `ListOptions` can be modified when creating a `ListWatch`.

    * [https://github.com/kubernetes/kubernetes/pull/57508](https://github.com/kubernetes/kubernetes/pull/57508)

* A `/token` subresource for ServiceAccount is added.

    * [https://github.com/kubernetes/kubernetes/pull/58111](https://github.com/kubernetes/kubernetes/pull/58111)

* If an informer delivery fails, the particular notification is skipped and continued the next time.

    * [https://github.com/kubernetes/kubernetes/pull/58394](https://github.com/kubernetes/kubernetes/pull/58394)

* Certificate manager will no longer wait until the initial rotation succeeds or fails before returning from `Start()`.

    * [https://github.com/kubernetes/kubernetes/pull/58930](https://github.com/kubernetes/kubernetes/pull/58930)

* [k8s.io/api] `VolumeScheduling` and `LocalPersistentVolume` features are beta and enabled by default. The PersistentVolume NodeAffinity alpha annotation is deprecated and will be removed in a future release.

    * [https://github.com/kubernetes/kubernetes/pull/59391](https://github.com/kubernetes/kubernetes/pull/59391)

* [k8s.io/api] The `PodSecurityPolicy` API has been moved to the `policy/v1beta1` API group. The `PodSecurityPolicy` API in the `extensions/v1beta1` API group is deprecated and will be removed in a future release.

    * [https://github.com/kubernetes/kubernetes/pull/54933](https://github.com/kubernetes/kubernetes/pull/54933)

* [k8s.io/api] ConfigMap objects now support binary data via a new `binaryData` field.

    * [https://github.com/kubernetes/kubernetes/pull/57938](https://github.com/kubernetes/kubernetes/pull/57938)

* [k8s.io/api] Service account TokenRequest API is added.

    * [https://github.com/kubernetes/kubernetes/pull/58027](https://github.com/kubernetes/kubernetes/pull/58027)

* [k8s.io/api] FSType is added in CSI volume source to specify filesystems.

    * [https://github.com/kubernetes/kubernetes/pull/58209](https://github.com/kubernetes/kubernetes/pull/58209)

* [k8s.io/api] v1beta1 VolumeAttachment API is added.

    * [https://github.com/kubernetes/kubernetes/pull/58462](https://github.com/kubernetes/kubernetes/pull/58462)

* [k8s.io/api] `v1.Pod` now has a field `ShareProcessNamespace` to configure whether a single process namespace should be shared between all containers in a pod. This feature is in alpha preview.

    * [https://github.com/kubernetes/kubernetes/pull/58716](https://github.com/kubernetes/kubernetes/pull/58716)

* [k8s.io/api] Add `NominatedNodeName` field to `PodStatus`. This field is set when a pod preempts other pods on the node.

    * [https://github.com/kubernetes/kubernetes/pull/58990](https://github.com/kubernetes/kubernetes/pull/58990)

* [k8s.io/api] Promote `CSIPersistentVolumeSourc`e to beta.

    * [https://github.com/kubernetes/kubernetes/pull/59157](https://github.com/kubernetes/kubernetes/pull/59157)

* [k8s.io/api] Promote `DNSPolicy` and `DNSConfig` in `PodSpec` to beta.

    * [https://github.com/kubernetes/kubernetes/pull/59771](https://github.com/kubernetes/kubernetes/pull/59771)

* [k8s.io/api] External metric types are added to the HPA API.

    * [https://github.com/kubernetes/kubernetes/pull/60096](https://github.com/kubernetes/kubernetes/pull/60096)

* [k8s.io/apimachinery] The `meta.k8s.io/v1alpha1` objects for retrieving tabular responses from the server (`Table`) or fetching just the `ObjectMeta` for an object (as `PartialObjectMetadata`) are now beta as part of `meta.k8s.io/v1beta1`. Clients may request alternate representations of normal Kubernetes objects by passing an `Accept` header like `application/json;as=Table;g=meta.k8s.io;v=v1beta1` or `application/json;as=PartialObjectMetadata;g=meta.k8s.io;v1=v1beta1`. Older servers will ignore this representation or return an error if it is not available. Clients may request fallback to the normal object by adding a non-qualified mime-type to their `Accept` header like `application/json` - the server will then respond with either the alternate representation if it is supported or the fallback mime-type which is the normal object response.

    * [https://github.com/kubernetes/kubernetes/pull/59059](https://github.com/kubernetes/kubernetes/pull/59059)


**Bug fixes and Improvements:**

* Port-forwarding of TCP6 ports is fixed.

    * [https://github.com/kubernetes/kubernetes/pull/57457](https://github.com/kubernetes/kubernetes/pull/57457)

* A race condition in SharedInformer that could violate the sequential delivery guarantee and cause panics on shutdown is fixed.

    * [https://github.com/kubernetes/kubernetes/pull/59828](https://github.com/kubernetes/kubernetes/pull/59828)

* [k8s.io/api] PersistentVolume flexVolume sources can now reference secrets in a namespace other than the PersistentVolumeClaim's namespace.

    * [https://github.com/kubernetes/kubernetes/pull/56460](https://github.com/kubernetes/kubernetes/pull/56460)

* [k8s.io/apimachinery] YAMLDecoder Read can now return the number of bytes read.

    * [https://github.com/kubernetes/kubernetes/pull/57000](https://github.com/kubernetes/kubernetes/pull/57000)

* [k8s.io/apimachinery] YAMLDecoder Read now tracks rest of buffer on `io.ErrShortBuffer`.

    * [https://github.com/kubernetes/kubernetes/pull/58817](https://github.com/kubernetes/kubernetes/pull/58817)

* [k8s.io/apimachinery] Prompt required merge key in the error message while applying a strategic merge patch.

    * [https://github.com/kubernetes/kubernetes/pull/57854](https://github.com/kubernetes/kubernetes/pull/57854)

# v6.0.0

**Breaking Changes:**

* If you upgrade your client-go libs and use the `AppsV1() or Apps()` interface, please note that the default garbage collection behavior is changed.

    * [https://github.com/kubernetes/kubernetes/pull/55148](https://github.com/kubernetes/kubernetes/pull/55148)

* Swagger 1.2 retriever `DiscoveryClient.SwaggerSchema` was removed from the discovery client

    * [https://github.com/kubernetes/kubernetes/pull/53441](https://github.com/kubernetes/kubernetes/pull/53441)

* Informers got a NewFilteredSharedInformerFactory to e.g. filter by namespace

    * [https://github.com/kubernetes/kubernetes/pull/54660](https://github.com/kubernetes/kubernetes/pull/54660)

* [k8s.io/api] The dynamic admission webhook is split into two kinds, mutating and validating. 
The kinds have changed completely and old code must be ported to `admissionregistration.k8s.io/v1beta1` - 
`MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`

    * [https://github.com/kubernetes/kubernetes/pull/55282](https://github.com/kubernetes/kubernetes/pull/55282)

* [k8s.io/api] Renamed `core/v1.ScaleIOVolumeSource` to `ScaleIOPersistentVolumeSource`

    * [https://github.com/kubernetes/kubernetes/pull/54013](https://github.com/kubernetes/kubernetes/pull/54013)

* [k8s.io/api] Renamed `core/v1.RBDVolumeSource` to `RBDPersistentVolumeSource`

    * [https://github.com/kubernetes/kubernetes/pull/54302](https://github.com/kubernetes/kubernetes/pull/54302)

* [k8s.io/api] Removed `core/v1.CreatedByAnnotation`

    * [https://github.com/kubernetes/kubernetes/pull/54445](https://github.com/kubernetes/kubernetes/pull/54445)

* [k8s.io/api] Renamed `core/v1.StorageMediumHugepages` to `StorageMediumHugePages`

    * [https://github.com/kubernetes/kubernetes/pull/54748](https://github.com/kubernetes/kubernetes/pull/54748)

* [k8s.io/api] `core/v1.Taint.TimeAdded` became a pointer

   * [https://github.com/kubernetes/kubernetes/pull/43016](https://github.com/kubernetes/kubernetes/pull/43016)

* [k8s.io/api] `core/v1.DefaultHardPodAffinitySymmetricWeight` type changed from int to int32

    * [https://github.com/kubernetes/kubernetes/pull/53850](https://github.com/kubernetes/kubernetes/pull/53850)

* [k8s.io/apimachinery] `ObjectCopier` interface was removed (requires switch to new generators with DeepCopy methods)

    * [https://github.com/kubernetes/kubernetes/pull/53525](https://github.com/kubernetes/kubernetes/pull/53525)

**New Features:**

* Certificate manager was moved from kubelet to `k8s.io/client-go/util/certificates`

   * [https://github.com/kubernetes/kubernetes/pull/49654](https://github.com/kubernetes/kubernetes/pull/49654)

* [k8s.io/api] Workloads api types are promoted to `apps/v1` version

    * [https://github.com/kubernetes/kubernetes/pull/53679](https://github.com/kubernetes/kubernetes/pull/53679)

* [k8s.io/api] Added `storage.k8s.io/v1alpha1` API group

    * [https://github.com/kubernetes/kubernetes/pull/54463](https://github.com/kubernetes/kubernetes/pull/54463)

* [k8s.io/api] Added support for conditions in StatefulSet status

    * [https://github.com/kubernetes/kubernetes/pull/55268](https://github.com/kubernetes/kubernetes/pull/55268)

* [k8s.io/api] Added support for conditions in DaemonSet status

    * [https://github.com/kubernetes/kubernetes/pull/55272](https://github.com/kubernetes/kubernetes/pull/55272)

* [k8s.io/apimachinery] Added polymorphic scale client in `k8s.io/client-go/scale`, which supports scaling of resources in arbitrary API groups

    * [https://github.com/kubernetes/kubernetes/pull/53743](https://github.com/kubernetes/kubernetes/pull/53743)

* [k8s.io/apimachinery] `meta.MetadataAccessor` got API chunking support

    * [https://github.com/kubernetes/kubernetes/pull/53768](https://github.com/kubernetes/kubernetes/pull/53768)

* [k8s.io/apimachinery] `unstructured.Unstructured` got getters and setters

    * [https://github.com/kubernetes/kubernetes/pull/51940](https://github.com/kubernetes/kubernetes/pull/51940)

**Bug fixes and Improvements:**

* The body in glog output is not truncated with log level 10

    * [https://github.com/kubernetes/kubernetes/pull/54801](https://github.com/kubernetes/kubernetes/pull/54801)

* [k8s.io/api] Unset `creationTimestamp` field is output as null if encoded from an unstructured object

    * [https://github.com/kubernetes/kubernetes/pull/53464](https://github.com/kubernetes/kubernetes/pull/53464)

* [k8s.io/apimachinery] Redirect behavior is restored for proxy subresources

    * [https://github.com/kubernetes/kubernetes/pull/52933](https://github.com/kubernetes/kubernetes/pull/52933)

* [k8s.io/apimachinery] Random string generation functions are optimized

    * [https://github.com/kubernetes/kubernetes/pull/53720](https://github.com/kubernetes/kubernetes/pull/53720)

# v5.0.1

Bug fix: picked up a security fix [kubernetes/kubernetes#53443](https://github.com/kubernetes/kubernetes/pull/53443) for `PodSecurityPolicy`.

# v5.0.0

**New features:**

* Added paging support

   * [https://github.com/kubernetes/kubernetes/pull/51876](https://github.com/kubernetes/kubernetes/pull/51876)

* Added support for client-side spam filtering of events

   * [https://github.com/kubernetes/kubernetes/pull/47367](https://github.com/kubernetes/kubernetes/pull/47367)

* Added support for http etag and caching

   * [https://github.com/kubernetes/kubernetes/pull/50404](https://github.com/kubernetes/kubernetes/pull/50404)

* Added priority queue support to informer cache

   * [https://github.com/kubernetes/kubernetes/pull/49752](https://github.com/kubernetes/kubernetes/pull/49752)

* Added openstack auth provider

   * [https://github.com/kubernetes/kubernetes/pull/39587](https://github.com/kubernetes/kubernetes/pull/39587)

* Added metrics for checking reflector health

   * [https://github.com/kubernetes/kubernetes/pull/48224](https://github.com/kubernetes/kubernetes/pull/48224)

* Client-go now includes the leaderelection package

   * [https://github.com/kubernetes/kubernetes/pull/39173](https://github.com/kubernetes/kubernetes/pull/39173)

**API changes:**

* Promoted Autoscaling v2alpha1 to v2beta1

   * [https://github.com/kubernetes/kubernetes/pull/50708](https://github.com/kubernetes/kubernetes/pull/50708)

* Promoted CronJobs to batch/v1beta1

   * [https://github.com/kubernetes/kubernetes/pull/41901](https://github.com/kubernetes/kubernetes/pull/41901)

* Promoted rbac.authorization.k8s.io/v1beta1 to rbac.authorization.k8s.io/v1

   * [https://github.com/kubernetes/kubernetes/pull/49642](https://github.com/kubernetes/kubernetes/pull/49642)

* Added a new API version apps/v1beta2

   * [https://github.com/kubernetes/kubernetes/pull/48746](https://github.com/kubernetes/kubernetes/pull/48746)

* Added a new API version scheduling/v1alpha1

   * [https://github.com/kubernetes/kubernetes/pull/48377](https://github.com/kubernetes/kubernetes/pull/48377)

**Breaking changes:**

* Moved pkg/api and pkg/apis to [k8s.io/api](https://github.com/kubernetes/api). Other kubernetes repositories also import types from there, so they are composable with client-go.

* Removed helper functions in pkg/api and pkg/apis. They are planned to be exported in other repos. The issue is tracked [here](https://github.com/kubernetes/kubernetes/issues/48209#issuecomment-314537745). During the transition, you'll have to copy the helper functions to your projects.

* The discovery client now fetches the protobuf encoded OpenAPI schema and returns `openapi_v2.Document`

   * [https://github.com/kubernetes/kubernetes/pull/46803](https://github.com/kubernetes/kubernetes/pull/46803)

* Enforced explicit references to API group client interfaces in clientsets to avoid ambiguity.

   * [https://github.com/kubernetes/kubernetes/pull/49370](https://github.com/kubernetes/kubernetes/pull/49370)

* The generic RESTClient type (`k8s.io/client-go/rest`) no longer exposes `LabelSelectorParam` or `FieldSelectorParam` methods - use `VersionedParams` with `metav1.ListOptions` instead. The `UintParam` method has been removed. The `timeout` parameter will no longer cause an error when using `Param()`.

   * [https://github.com/kubernetes/kubernetes/pull/48991](https://github.com/kubernetes/kubernetes/pull/48991)

# v4.0.0

No significant changes since v4.0.0-beta.0.

# v4.0.0-beta.0

**New features:**

* Added OpenAPISchema support in the discovery client

    * [https://github.com/kubernetes/kubernetes/pull/44531](https://github.com/kubernetes/kubernetes/pull/44531)

* Added mutation cache filter: MutationCache is able to take the result of update operations and stores them in an LRU that can be used to provide a more current view of a requested object.

    * [https://github.com/kubernetes/kubernetes/pull/45838](https://github.com/kubernetes/kubernetes/pull/45838/commits/f88c7725b4f9446c652d160bdcfab7c6201bddea)

* Moved the remotecommand package (used by `kubectl exec/attach`) to client-go

    * [https://github.com/kubernetes/kubernetes/pull/41331](https://github.com/kubernetes/kubernetes/pull/41331)

* Added support for following redirects to the SpdyRoundTripper

    * [https://github.com/kubernetes/kubernetes/pull/44451](https://github.com/kubernetes/kubernetes/pull/44451)

* Added Azure Active Directory plugin

    * [https://github.com/kubernetes/kubernetes/pull/43987](https://github.com/kubernetes/kubernetes/pull/43987)

**Usability improvements:**

* Added several new examples and reorganized client-go/examples

    * [Related PRs](https://github.com/kubernetes/kubernetes/commits/release-1.7/staging/src/k8s.io/client-go/examples)

**API changes:**

* Added networking.k8s.io/v1 API

    * [https://github.com/kubernetes/kubernetes/pull/39164](https://github.com/kubernetes/kubernetes/pull/39164)

* ControllerRevision type added for StatefulSet and DaemonSet history.

    * [https://github.com/kubernetes/kubernetes/pull/45867](https://github.com/kubernetes/kubernetes/pull/45867)

* Added support for initializers

    * [https://github.com/kubernetes/kubernetes/pull/38058](https://github.com/kubernetes/kubernetes/pull/38058)

* Added admissionregistration.k8s.io/v1alpha1 API

    * [https://github.com/kubernetes/kubernetes/pull/46294](https://github.com/kubernetes/kubernetes/pull/46294)

**Breaking changes:**

* Moved client-go/util/clock to apimachinery/pkg/util/clock 

    * [https://github.com/kubernetes/kubernetes/pull/45933](https://github.com/kubernetes/kubernetes/pull/45933/commits/8013212db54e95050c622675c6706cce5de42b45)

* Some [API helpers](https://github.com/kubernetes/client-go/blob/release-3.0/pkg/api/helpers.go) were removed. 

* Dynamic client takes GetOptions as an input parameter

    * [https://github.com/kubernetes/kubernetes/pull/47251](https://github.com/kubernetes/kubernetes/pull/47251)

**Bug fixes:**

* PortForwarder: don't log an error if net.Listen fails. [https://github.com/kubernetes/kubernetes/pull/44636](https://github.com/kubernetes/kubernetes/pull/44636)

* oidc auth plugin not to override the Auth header if it's already exits. [https://github.com/kubernetes/kubernetes/pull/45529](https://github.com/kubernetes/kubernetes/pull/45529)

* The --namespace flag is now honored for in-cluster clients that have an empty configuration. [https://github.com/kubernetes/kubernetes/pull/46299](https://github.com/kubernetes/kubernetes/pull/46299)

* GCP auth plugin no longer overwrites existing Authorization headers. [https://github.com/kubernetes/kubernetes/pull/45575](https://github.com/kubernetes/kubernetes/pull/45575)

# v3.0.0

Bug fixes:
* Use OS-specific libs when computing client User-Agent in kubectl, etc. (https://github.com/kubernetes/kubernetes/pull/44423)
* kubectl commands run inside a pod using a kubeconfig file now use the namespace specified in the kubeconfig file, instead of using the pod namespace. If no kubeconfig file is used, or the kubeconfig does not specify a namespace, the pod namespace is still used as a fallback. (https://github.com/kubernetes/kubernetes/pull/44570)
* Restored the ability of kubectl running inside a pod to consume resource files specifying a different namespace than the one the pod is running in. (https://github.com/kubernetes/kubernetes/pull/44862)

# v3.0.0-beta.0

* Added dependency on k8s.io/apimachinery. The impacts include changing import path of API objects like `ListOptions` from `k8s.io/client-go/pkg/api/v1` to `k8s.io/apimachinery/pkg/apis/meta/v1`.
* Added generated listers (listers/) and informers (informers/)
* Kubernetes API changes:
  * Added client support for:
    * authentication/v1
    * authorization/v1
    * autoscaling/v2alpha1
    * rbac/v1beta1
    * settings/v1alpha1
    * storage/v1
  * Changed client support for:
    * certificates from v1alpha1 to v1beta1
    * policy from v1alpha1 to v1beta1
  * Deleted client support for:
    * extensions/v1beta1#Job
* CHANGED: pass typed options to dynamic client (https://github.com/kubernetes/kubernetes/pull/41887)

# v2.0.0

* Included bug fixes in k8s.io/kuberentes release-1.5 branch, up to commit 
  bde8578d9675129b7a2aa08f1b825ec6cc0f3420

# v2.0.0-alpha.1

* Removed top-level version folder (e.g., 1.4 and 1.5), switching to maintaining separate versions
  in separate branches.
* Clientset supported multiple versions per API group
* Added ThirdPartyResources example
* Kubernetes API changes
  * Apps API group graduated to v1beta1 
  * Policy API group graduated to v1beta1
  * Added support for batch/v2alpha1/cronjob
  * Renamed PetSet to StatefulSet
  

# v1.5.0

* Included the auth plugin (https://github.com/kubernetes/kubernetes/pull/33334)
* Added timeout field to RESTClient config (https://github.com/kubernetes/kubernetes/pull/33958)

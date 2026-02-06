module github.com/prometheus/prometheus

go 1.25.0

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.20.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.7.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0
	github.com/Code-Hex/go-generics-cache v1.5.1
	github.com/KimMachineGun/automemlimit v0.7.5
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b
	github.com/aws/aws-sdk-go-v2 v1.41.1
	github.com/aws/aws-sdk-go-v2/config v1.32.6
	github.com/aws/aws-sdk-go-v2/credentials v1.19.6
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.279.0
	github.com/aws/aws-sdk-go-v2/service/ecs v1.70.0
	github.com/aws/aws-sdk-go-v2/service/kafka v1.46.7
	github.com/aws/aws-sdk-go-v2/service/lightsail v1.50.10
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.5
	github.com/aws/smithy-go v1.24.0
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/dennwc/varint v1.0.0
	github.com/digitalocean/godo v1.171.0
	github.com/docker/docker v28.5.2+incompatible
	github.com/edsrzf/mmap-go v1.2.0
	github.com/envoyproxy/go-control-plane/envoy v1.36.0
	github.com/envoyproxy/protoc-gen-validate v1.3.0
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb
	github.com/felixge/fgprof v0.9.5
	github.com/fsnotify/fsnotify v1.9.0
	github.com/go-openapi/strfmt v0.25.0
	github.com/go-zookeeper/zk v1.0.4
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v1.0.0
	github.com/google/go-cmp v0.7.0
	github.com/google/pprof v0.0.0-20260111202518-71be6bfdd440
	github.com/google/uuid v1.6.0
	github.com/gophercloud/gophercloud/v2 v2.9.0
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853
	github.com/hashicorp/consul/api v1.32.1
	github.com/hashicorp/nomad/api v0.0.0-20260106084653-e8f2200c7039
	github.com/hetznercloud/hcloud-go/v2 v2.33.0
	github.com/ionos-cloud/sdk-go/v6 v6.3.6
	github.com/json-iterator/go v1.1.12
	github.com/klauspost/compress v1.18.2
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b
	github.com/linode/linodego v1.63.0
	github.com/miekg/dns v1.1.69
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/nsf/jsondiff v0.0.0-20230430225905-43f6cf3098c1
	github.com/oklog/run v1.2.0
	github.com/oklog/ulid/v2 v2.1.1
	github.com/open-telemetry/opentelemetry-collector-contrib/processor/deltatocumulativeprocessor v0.142.0
	github.com/ovh/go-ovh v1.9.0
	github.com/pb33f/libopenapi v0.31.1
	github.com/pb33f/libopenapi-validator v0.10.0
	github.com/prometheus/alertmanager v0.30.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_golang/exp v0.0.0-20260101091701-2cd067eb23c9
	github.com/prometheus/client_model v0.6.2
	github.com/prometheus/common v0.67.4
	github.com/prometheus/common/assets v0.2.0
	github.com/prometheus/exporter-toolkit v0.15.0
	github.com/prometheus/sigv4 v0.3.0
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.36
	github.com/shurcooL/httpfs v0.0.0-20230704072500-f1e31cf0ba5c
	github.com/stackitcloud/stackit-sdk-go/core v0.20.1
	github.com/stretchr/testify v1.11.1
	github.com/vultr/govultr/v2 v2.17.2
	go.opentelemetry.io/collector/component v1.48.0
	go.opentelemetry.io/collector/consumer v1.48.0
	go.opentelemetry.io/collector/pdata v1.48.0
	go.opentelemetry.io/collector/processor v1.48.0
	go.opentelemetry.io/contrib/instrumentation/net/http/httptrace/otelhttptrace v0.64.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0
	go.opentelemetry.io/otel/metric v1.39.0
	go.opentelemetry.io/otel/sdk v1.39.0
	go.opentelemetry.io/otel/trace v1.39.0
	go.uber.org/atomic v1.11.0
	go.uber.org/automaxprocs v1.6.0
	go.uber.org/goleak v1.3.0
	go.yaml.in/yaml/v2 v2.4.3
	go.yaml.in/yaml/v3 v3.0.4
	go.yaml.in/yaml/v4 v4.0.0-rc.3
	golang.org/x/oauth2 v0.34.0
	golang.org/x/sync v0.19.0
	golang.org/x/sys v0.39.0
	golang.org/x/text v0.32.0
	google.golang.org/api v0.258.0
	google.golang.org/genproto/googleapis/api v0.0.0-20251222181119-0a764e51fe1b
	google.golang.org/grpc v1.78.0
	google.golang.org/protobuf v1.36.11
	k8s.io/api v0.34.3
	k8s.io/apimachinery v0.34.3
	k8s.io/client-go v0.34.3
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.130.1
)

require (
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.4 // indirect
	github.com/bahlo/generic-list-go v0.2.0 // indirect
	github.com/basgys/goxml2json v1.1.1-0.20231018121955-e66ee54ceaad // indirect
	github.com/buger/jsonparser v1.1.1 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/pb33f/jsonpath v0.7.0 // indirect
	github.com/pb33f/ordered-map/v2 v2.3.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
)

require (
	cloud.google.com/go/auth v0.17.0 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.16
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.17 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.12 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cncf/xds/go v0.0.0-20251022180443-0feb69152e9f // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.6.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.12.2 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.24.1 // indirect
	github.com/go-openapi/errors v0.22.4 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.3 // indirect
	github.com/go-openapi/loads v0.23.2 // indirect
	github.com/go-openapi/spec v0.22.1 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/validate v0.25.1 // indirect
	github.com/go-resty/resty/v2 v2.17.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.7 // indirect
	github.com/googleapis/gax-go/v2 v2.15.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.3 // indirect
	github.com/hashicorp/cronexpr v1.1.3 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.7 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.8.0 // indirect
	github.com/hashicorp/golang-lru v0.6.0 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/confmap v1.0.0 // indirect
	github.com/knadh/koanf/v2 v2.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mdlayher/socket v0.4.1 // indirect
	github.com/mdlayher/vsock v1.2.1 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/sys/atomicwriter v0.1.0 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/internal/exp/metrics v0.142.0 // indirect
	github.com/open-telemetry/opentelemetry-collector-contrib/pkg/pdatautil v0.142.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/otlptranslator v1.0.0
	github.com/prometheus/procfs v0.16.1 // indirect
	github.com/puzpuzpuz/xsync/v3 v3.5.1 // indirect
	github.com/spf13/pflag v1.0.6 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	go.mongodb.org/mongo-driver v1.17.6 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector/confmap v1.48.0 // indirect
	go.opentelemetry.io/collector/confmap/xconfmap v0.142.0 // indirect
	go.opentelemetry.io/collector/featuregate v1.48.0 // indirect
	go.opentelemetry.io/collector/pipeline v1.48.0 // indirect
	go.opentelemetry.io/proto/otlp v1.9.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/exp v0.0.0-20250808145144-a408d31f581a // indirect
	golang.org/x/mod v0.30.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/term v0.38.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	golang.org/x/tools v0.39.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251213004720-97cd9d5aeac2 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gotest.tools/v3 v3.0.3 // indirect
	k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b // indirect
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
)

// Exclude linodego v1.0.0 as it is no longer published on github.
exclude github.com/linode/linodego v1.0.0

// Exclude grpc v1.30.0 because of breaking changes. See #7621.
exclude (
	github.com/grpc-ecosystem/grpc-gateway v1.14.7
	google.golang.org/api v0.30.0
)

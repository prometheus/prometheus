module github.com/prometheus/prometheus

go 1.21

require (
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.10.0
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.5.1
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5 v5.5.0
	github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v4 v4.3.0
	github.com/Code-Hex/go-generics-cache v1.3.1
	github.com/KimMachineGun/automemlimit v0.5.0
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/alecthomas/units v0.0.0-20231202071711-9a357b53e9c9
	github.com/aws/aws-sdk-go v1.50.32
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3
	github.com/cespare/xxhash/v2 v2.2.0
	github.com/dennwc/varint v1.0.0
	github.com/digitalocean/godo v1.109.0
	github.com/docker/docker v25.0.3+incompatible
	github.com/edsrzf/mmap-go v1.1.0
	github.com/envoyproxy/go-control-plane v0.12.0
	github.com/envoyproxy/protoc-gen-validate v1.0.4
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb
	github.com/fsnotify/fsnotify v1.7.0
	github.com/go-kit/log v0.2.1
	github.com/go-logfmt/logfmt v0.6.0
	github.com/go-openapi/strfmt v0.22.2
	github.com/go-zookeeper/zk v1.0.3
	github.com/gogo/protobuf v1.3.2
	github.com/golang/snappy v0.0.4
	github.com/google/go-cmp v0.6.0
	github.com/google/pprof v0.0.0-20240227163752-401108e1b7e7
	github.com/google/uuid v1.6.0
	github.com/gophercloud/gophercloud v1.8.0
	github.com/grafana/regexp v0.0.0-20221122212121-6b5c0a4cb7fd
	github.com/grpc-ecosystem/grpc-gateway v1.16.0
	github.com/hashicorp/consul/api v1.28.2
	github.com/hashicorp/nomad/api v0.0.0-20240306004928-3e7191ccb702
	github.com/hetznercloud/hcloud-go/v2 v2.6.0
	github.com/ionos-cloud/sdk-go/v6 v6.1.11
	github.com/json-iterator/go v1.1.12
	github.com/klauspost/compress v1.17.7
	github.com/kolo/xmlrpc v0.0.0-20220921171641-a4b6fa1dd06b
	github.com/linode/linodego v1.30.0
	github.com/miekg/dns v1.1.58
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/nsf/jsondiff v0.0.0-20230430225905-43f6cf3098c1
	github.com/oklog/run v1.1.0
	github.com/oklog/ulid v1.3.1
	github.com/ovh/go-ovh v1.4.3
	github.com/prometheus/alertmanager v0.27.0
	github.com/prometheus/client_golang v1.19.0
	github.com/prometheus/client_model v0.6.0
	github.com/prometheus/common v0.49.1-0.20240306132007-4199f18c3e92
	github.com/prometheus/common/assets v0.2.0
	github.com/prometheus/common/sigv4 v0.1.0
	github.com/prometheus/exporter-toolkit v0.11.0
	github.com/scaleway/scaleway-sdk-go v1.0.0-beta.25
	github.com/shurcooL/httpfs v0.0.0-20230704072500-f1e31cf0ba5c
	github.com/stretchr/testify v1.9.0
	github.com/vultr/govultr/v2 v2.17.2
	go.opentelemetry.io/collector/featuregate v1.4.0
	go.opentelemetry.io/collector/pdata v1.4.0
	go.opentelemetry.io/collector/semconv v0.97.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.49.0
	go.opentelemetry.io/otel v1.24.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.24.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.24.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.24.0
	go.opentelemetry.io/otel/sdk v1.24.0
	go.opentelemetry.io/otel/trace v1.24.0
	go.uber.org/atomic v1.11.0
	go.uber.org/automaxprocs v1.5.3
	go.uber.org/goleak v1.3.0
	go.uber.org/multierr v1.11.0
	golang.org/x/net v0.22.0
	golang.org/x/oauth2 v0.18.0
	golang.org/x/sync v0.6.0
	golang.org/x/sys v0.18.0
	golang.org/x/time v0.5.0
	golang.org/x/tools v0.19.0
	google.golang.org/api v0.168.0
	google.golang.org/genproto/googleapis/api v0.0.0-20240304212257-790db918fca8
	google.golang.org/grpc v1.62.1
	google.golang.org/protobuf v1.33.0
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.29.3
	k8s.io/apimachinery v0.29.3
	k8s.io/client-go v0.29.3
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.120.1
)

require (
	cloud.google.com/go/compute v1.23.4 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.5.2 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.2.1 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/armon/go-metrics v0.4.1 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cilium/ebpf v0.11.0 // indirect
	github.com/cncf/xds/go v0.0.0-20231128003011-0fa0005c9caa // indirect
	github.com/containerd/cgroups/v3 v3.0.3 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/distribution/reference v0.5.0 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.11.0 // indirect
	github.com/evanphx/json-patch v5.6.0+incompatible // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-kit/kit v0.12.0 // indirect
	github.com/go-logr/logr v1.4.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/analysis v0.22.2 // indirect
	github.com/go-openapi/errors v0.21.1 // indirect
	github.com/go-openapi/jsonpointer v0.20.2 // indirect
	github.com/go-openapi/jsonreference v0.20.4 // indirect
	github.com/go-openapi/loads v0.21.5 // indirect
	github.com/go-openapi/spec v0.20.14 // indirect
	github.com/go-openapi/swag v0.22.9 // indirect
	github.com/go-openapi/validate v0.23.0 // indirect
	github.com/go-resty/resty/v2 v2.11.0 // indirect
	github.com/godbus/dbus/v5 v5.0.4 // indirect
	github.com/golang-jwt/jwt/v5 v5.2.0 // indirect
	github.com/golang/glog v1.2.0 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/gnostic-models v0.6.8 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/s2a-go v0.1.7 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.2 // indirect
	github.com/googleapis/gax-go/v2 v2.12.2 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.19.0 // indirect
	github.com/hashicorp/cronexpr v1.1.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-hclog v1.5.0 // indirect
	github.com/hashicorp/go-immutable-radix v1.3.1 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/go-retryablehttp v0.7.4 // indirect
	github.com/hashicorp/go-rootcerts v1.0.2 // indirect
	github.com/hashicorp/go-version v1.6.0 // indirect
	github.com/hashicorp/golang-lru v0.6.0 // indirect
	github.com/hashicorp/serf v0.10.1 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runtime-spec v1.0.2 // indirect
	github.com/pbnjay/memory v0.0.0-20210728143218-7b4eea64cf58 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	go.mongodb.org/mongo-driver v1.14.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/otel/metric v1.24.0 // indirect
	go.opentelemetry.io/proto/otlp v1.1.0 // indirect
	golang.org/x/crypto v0.21.0 // indirect
	golang.org/x/exp v0.0.0-20240119083558-1b970713d09a // indirect
	golang.org/x/mod v0.16.0 // indirect
	golang.org/x/term v0.18.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240304161311-37d4d3c04a78 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gotest.tools/v3 v3.0.3 // indirect
	k8s.io/kube-openapi v0.0.0-20231010175941-2dd684a91f00 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	k8s.io/klog => github.com/simonpasquier/klog-gokit v0.3.0
	k8s.io/klog/v2 => github.com/simonpasquier/klog-gokit/v3 v3.3.0
)

// Exclude linodego v1.0.0 as it is no longer published on github.
exclude github.com/linode/linodego v1.0.0

// Exclude grpc v1.30.0 because of breaking changes. See #7621.
exclude (
	github.com/grpc-ecosystem/grpc-gateway v1.14.7
	google.golang.org/api v0.30.0
)

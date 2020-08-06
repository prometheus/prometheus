module github.com/prometheus/prometheus

go 1.14

require (
	github.com/Azure/azure-sdk-for-go v44.2.0+incompatible
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Azure/go-autorest/autorest v0.11.2
	github.com/Azure/go-autorest/autorest/adal v0.9.0
	github.com/Azure/go-autorest/autorest/to v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d
	github.com/armon/go-metrics v0.3.3 // indirect
	github.com/aws/aws-sdk-go v1.33.12
	github.com/cespare/xxhash v1.1.0
	github.com/containerd/containerd v1.3.4 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgryski/go-sip13 v0.0.0-20190329191031-25c5027a8c7b
	github.com/digitalocean/godo v1.42.0
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v17.12.0-ce-rc1.0.20200706150819-a40b877fbb9e+incompatible
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/edsrzf/mmap-go v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/go-logfmt/logfmt v0.5.0
	github.com/go-openapi/strfmt v0.19.5
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.1
	github.com/google/pprof v0.0.0-20200708004538-1a94d8640e99
	github.com/googleapis/gnostic v0.4.0 // indirect
	github.com/gophercloud/gophercloud v0.12.0
	github.com/grpc-ecosystem/grpc-gateway v1.14.6
	github.com/hashicorp/consul/api v1.5.0
	github.com/hashicorp/go-hclog v0.12.2 // indirect
	github.com/hashicorp/go-immutable-radix v1.2.0 // indirect
	github.com/hashicorp/golang-lru v0.5.4 // indirect
	github.com/influxdata/influxdb v1.8.1
	github.com/json-iterator/go v1.1.10
	github.com/mattn/go-colorable v0.1.6 // indirect
	github.com/miekg/dns v1.1.30
	github.com/mitchellh/mapstructure v1.2.2 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/oklog/run v1.1.0
	github.com/oklog/ulid v1.3.1
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opentracing-contrib/go-stdlib v1.0.0
	github.com/opentracing/opentracing-go v1.2.0
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus/alertmanager v0.21.0
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.11.1
	github.com/samuel/go-zookeeper v0.0.0-20200724154423-2164a8ac840e
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/shurcooL/vfsgen v0.0.0-20200627165143-92b8a710ab6c
	github.com/soheilhy/cmux v0.1.4
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	go.mongodb.org/mongo-driver v1.3.2 // indirect
	go.uber.org/atomic v1.6.0
	go.uber.org/goleak v1.0.0
	golang.org/x/net v0.0.0-20200707034311-ab3426394381
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20200625203802-6e8e738ad208
	golang.org/x/sys v0.0.0-20200724161237-0e2f3a69832c
	golang.org/x/time v0.0.0-20200630173020-3af7569d3a1e
	golang.org/x/tools v0.0.0-20200725200936-102e7d357031
	google.golang.org/api v0.29.0
	google.golang.org/appengine v1.6.6 // indirect
	google.golang.org/genproto v0.0.0-20200724131911-43cab4749ae7
	google.golang.org/grpc v1.29.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	gotest.tools v2.2.0+incompatible // indirect
	k8s.io/api v0.18.6
	k8s.io/apimachinery v0.18.6
	k8s.io/client-go v0.18.6
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200414100711-2df71ebbae66 // indirect
)

replace k8s.io/klog => github.com/simonpasquier/klog-gokit v0.1.0

exclude (
	// Exclude grpc v1.30.0 because of breaking changes. See #7621.
	google.golang.org/grpc v1.30.0

	// Exclude pre-go-mod kubernetes tags, as they are older
	// than v0.x releases but are picked when we update the dependencies.
	k8s.io/client-go v1.4.0
	k8s.io/client-go v1.4.0+incompatible
	k8s.io/client-go v1.5.0
	k8s.io/client-go v1.5.0+incompatible
	k8s.io/client-go v1.5.1
	k8s.io/client-go v1.5.1+incompatible
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/client-go v2.0.0+incompatible
	k8s.io/client-go v2.0.0-alpha.1+incompatible
	k8s.io/client-go v3.0.0+incompatible
	k8s.io/client-go v3.0.0-beta.0+incompatible
	k8s.io/client-go v4.0.0+incompatible
	k8s.io/client-go v4.0.0-beta.0+incompatible
	k8s.io/client-go v5.0.0+incompatible
	k8s.io/client-go v5.0.1+incompatible
	k8s.io/client-go v6.0.0+incompatible
	k8s.io/client-go v7.0.0+incompatible
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/client-go v9.0.0+incompatible
	k8s.io/client-go v9.0.0-invalid+incompatible
)

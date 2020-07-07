module github.com/prometheus/prometheus

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v43.0.0+incompatible
	github.com/Azure/go-ansiterm v0.0.0-20170929234023-d6e3b3328b78 // indirect
	github.com/Azure/go-autorest/autorest v0.10.2
	github.com/Azure/go-autorest/autorest/adal v0.8.3
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d
	github.com/aws/aws-sdk-go v1.31.9
	github.com/cespare/xxhash v1.1.0
	github.com/containerd/containerd v1.3.4 // indirect
	github.com/davecgh/go-spew v1.1.1
	github.com/dgryski/go-sip13 v0.0.0-20190329191031-25c5027a8c7b
	github.com/digitalocean/godo v1.37.0
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker v17.12.0-ce-rc1.0.20200706150819-a40b877fbb9e+incompatible
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/edsrzf/mmap-go v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/go-logfmt/logfmt v0.5.0
	github.com/go-openapi/strfmt v0.19.5
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.1
	github.com/google/pprof v0.0.0-20200507031123-427632fa3b1c
	github.com/gophercloud/gophercloud v0.11.0
	github.com/grpc-ecosystem/grpc-gateway v1.14.6
	github.com/hashicorp/consul/api v1.4.0
	github.com/influxdata/influxdb v1.8.0
	github.com/json-iterator/go v1.1.10
	github.com/miekg/dns v1.1.29
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/oklog/run v1.1.0
	github.com/oklog/ulid v1.3.1
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/opentracing-contrib/go-stdlib v0.0.0-20190519235532-cf7a6c988dc9
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/prometheus-community/promql-langserver v0.5.2-0.20200707205213-a45e11c16373
	github.com/prometheus/alertmanager v0.20.0
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.10.0
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/soheilhy/cmux v0.1.4
	github.com/uber/jaeger-client-go v2.23.1+incompatible
	github.com/uber/jaeger-lib v2.2.0+incompatible
	golang.org/x/net v0.0.0-20200602114024-627f9648deb9
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20200317015054-43a5402ce75a
	golang.org/x/sys v0.0.0-20200615200032-f1bc736245b1
	golang.org/x/time v0.0.0-20200416051211-89c76fbcd5d1
	golang.org/x/tools v0.0.0-20200603131246-cc40288be839
	google.golang.org/api v0.26.0
	google.golang.org/genproto v0.0.0-20200603110839-e855014d5736
	google.golang.org/grpc v1.29.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200603094226-e3079894b1e8
	gotest.tools v2.2.0+incompatible // indirect
	k8s.io/api v0.18.3
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v0.18.3
	k8s.io/klog v1.0.0
)

replace k8s.io/klog => github.com/simonpasquier/klog-gokit v0.1.0

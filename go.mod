module github.com/prometheus/prometheus

go 1.13

require (
	github.com/Azure/azure-sdk-for-go v39.1.0+incompatible
	github.com/Azure/go-autorest/autorest v0.9.5
	github.com/Azure/go-autorest/autorest/adal v0.8.2
	github.com/Azure/go-autorest/autorest/to v0.3.0 // indirect
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/alecthomas/units v0.0.0-20190924025748-f65c72e2690d
	github.com/aws/aws-sdk-go v1.29.4
	github.com/cespare/xxhash v1.1.0
	github.com/dgryski/go-sip13 v0.0.0-20190329191031-25c5027a8c7b
	github.com/edsrzf/mmap-go v1.0.0
	github.com/go-kit/kit v0.10.0
	github.com/go-logfmt/logfmt v0.5.0
	github.com/go-openapi/strfmt v0.19.4
	github.com/gogo/protobuf v1.3.1
	github.com/golang/snappy v0.0.1
	github.com/google/pprof v0.0.0-20200212024743-f11f1df84d12
	github.com/gophercloud/gophercloud v0.8.0
	github.com/grpc-ecosystem/grpc-gateway v1.13.0
	github.com/hashicorp/consul/api v1.4.0
	github.com/influxdata/influxdb v1.7.7
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.9
	github.com/miekg/dns v1.1.27
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/oklog/run v1.1.0
	github.com/oklog/ulid v1.3.1
	github.com/opentracing-contrib/go-stdlib v0.0.0-20190519235532-cf7a6c988dc9
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/alertmanager v0.20.0
	github.com/prometheus/client_golang v1.4.1
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.9.1
	github.com/samuel/go-zookeeper v0.0.0-20190923202752-2cc03de413da
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/soheilhy/cmux v0.1.4
	golang.org/x/net v0.0.0-20200202094626-16171245cfb2
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200212091648-12a6c2dcc1e4
	golang.org/x/time v0.0.0-20191024005414-555d28b269f0
	golang.org/x/tools v0.0.0-20200212150539-ea181f53ac56
	google.golang.org/api v0.17.0
	google.golang.org/genproto v0.0.0-20200212174721-66ed5ce911ce
	google.golang.org/grpc v1.27.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	gopkg.in/yaml.v2 v2.2.8
	gopkg.in/yaml.v3 v3.0.0-20200121175148-a6ecf24a6d71
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
	k8s.io/klog v1.0.0
)

replace k8s.io/klog => github.com/simonpasquier/klog-gokit v0.1.0

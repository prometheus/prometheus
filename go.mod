module github.com/prometheus/prometheus

go 1.12

replace k8s.io/klog => github.com/simonpasquier/klog-gokit v0.1.0

require (
	cloud.google.com/go v0.44.1 // indirect
	contrib.go.opencensus.io/exporter/ocagent v0.6.0 // indirect
	github.com/Azure/azure-sdk-for-go v23.2.0+incompatible
	github.com/Azure/go-autorest v11.2.8+incompatible
	github.com/OneOfOne/xxhash v1.2.5 // indirect
	github.com/alecthomas/units v0.0.0-20190717042225-c3de453c63f4
	github.com/armon/go-metrics v0.0.0-20190430140413-ec5e00d3c878 // indirect
	github.com/aws/aws-sdk-go v1.23.12
	github.com/cespare/xxhash v1.1.0
	github.com/dgryski/go-sip13 v0.0.0-20190329191031-25c5027a8c7b
	github.com/edsrzf/mmap-go v1.0.0
	github.com/evanphx/json-patch v4.5.0+incompatible // indirect
	github.com/go-kit/kit v0.9.0
	github.com/go-logfmt/logfmt v0.4.0
	github.com/go-openapi/analysis v0.19.4 // indirect
	github.com/go-openapi/runtime v0.19.3 // indirect
	github.com/go-openapi/strfmt v0.19.2
	github.com/go-openapi/swag v0.19.4 // indirect
	github.com/gogo/protobuf v1.2.2-0.20190730201129-28a6bbf47e48
	github.com/golang/groupcache v0.0.0-20190702054246-869f871628b6 // indirect
	github.com/golang/snappy v0.0.1
	github.com/google/go-cmp v0.3.1 // indirect
	github.com/google/pprof v0.0.0-20190723021845-34ac40c74b70
	github.com/googleapis/gnostic v0.3.0 // indirect
	github.com/gophercloud/gophercloud v0.3.0
	github.com/grpc-ecosystem/grpc-gateway v1.9.5
	github.com/hashicorp/consul/api v1.1.0
	github.com/hashicorp/go-immutable-radix v1.1.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.5 // indirect
	github.com/hashicorp/go-rootcerts v1.0.1 // indirect
	github.com/hashicorp/golang-lru v0.5.3 // indirect
	github.com/hashicorp/memberlist v0.1.4 // indirect
	github.com/hashicorp/serf v0.8.3 // indirect
	github.com/influxdata/influxdb v1.7.7
	github.com/jpillora/backoff v0.0.0-20180909062703-3050d21c67d7 // indirect
	github.com/json-iterator/go v1.1.7
	github.com/mailru/easyjson v0.0.0-20190626092158-b2ccc519800e // indirect
	github.com/miekg/dns v1.1.15
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f
	github.com/oklog/run v1.0.0
	github.com/oklog/ulid v1.3.1
	github.com/opentracing-contrib/go-stdlib v0.0.0-20190519235532-cf7a6c988dc9
	github.com/opentracing/opentracing-go v1.1.0
	github.com/pkg/errors v0.8.1
	github.com/prometheus/alertmanager v0.18.0
	github.com/prometheus/client_golang v1.1.0
	github.com/prometheus/client_model v0.0.0-20190812154241-14fe0d1b01d4
	github.com/prometheus/common v0.7.0
	github.com/samuel/go-zookeeper v0.0.0-20190810000440-0ceca61e4d75
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/soheilhy/cmux v0.1.4
	github.com/spaolacci/murmur3 v1.1.0 // indirect
	github.com/stretchr/testify v1.4.0 // indirect
	go.mongodb.org/mongo-driver v1.0.4 // indirect
	golang.org/x/crypto v0.0.0-20190701094942-4def268fd1a4 // indirect
	golang.org/x/net v0.0.0-20190724013045-ca1201d0de80
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/sync v0.0.0-20190423024810-112230192c58
	golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a
	golang.org/x/time v0.0.0-20190308202827-9d24e82272b4
	golang.org/x/tools v0.0.0-20190813034749-528a2984e271
	google.golang.org/api v0.8.0
	google.golang.org/genproto v0.0.0-20190801165951-fa694d86fc64
	google.golang.org/grpc v1.22.1
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/check.v1 v1.0.0-20190902080502-41f04d3bba15 // indirect
	gopkg.in/fsnotify/fsnotify.v1 v1.4.7
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.2.2
	k8s.io/api v0.0.0-20190813020757-36bff7324fb7
	k8s.io/apimachinery v0.0.0-20190809020650-423f5d784010
	k8s.io/client-go v0.0.0-20190620085101-78d2af792bab
	k8s.io/klog v0.4.0
	k8s.io/kube-openapi v0.0.0-20190722073852-5e22f3d471e6 // indirect
	k8s.io/utils v0.0.0-20190809000727-6c36bc71fc4a // indirect
)

replace github.com/golang/lint => golang.org/x/lint v0.0.0-20190409202823-959b441ac422

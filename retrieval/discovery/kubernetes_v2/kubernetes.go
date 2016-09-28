package kubernetesv2

import (
	"io/ioutil"
	"time"

	"github.com/prometheus/prometheus/config"

	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/api"
	apiv1 "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/cache"
	kubernetes "k8s.io/kubernetes/pkg/client/clientset_generated/release_1_5"
	rest "k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/util/runtime"
)

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel  = metaLabelPrefix + "namespace"
)

// Kubernetes implements the TargetProvider interface for discovering
// targets from Kubernetes.
type Kubernetes struct {
	client kubernetes.Interface
	role   config.KubernetesRole
	logger log.Logger
}

func init() {
	runtime.ErrorHandlers = append(runtime.ErrorHandlers, func(err error) {
		log.With("component", "kube_client_runtime").Errorln(err)
	})
}

// New creates a new Kubernetes discovery for the given role.
func New(l log.Logger, conf *config.KubernetesV2SDConfig) (*Kubernetes, error) {
	var (
		kcfg *rest.Config
		err  error
	)
	if conf.APIServer.String() == "" {
		kcfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
	} else {
		token := conf.BearerToken
		if conf.BearerTokenFile != "" {
			bf, err := ioutil.ReadFile(conf.BearerTokenFile)
			if err != nil {
				return nil, err
			}
			token = string(bf)
		}

		kcfg = &rest.Config{
			Host:        conf.APIServer.String(),
			BearerToken: token,
			TLSClientConfig: rest.TLSClientConfig{
				CAFile: conf.TLSConfig.CAFile,
			},
		}
	}
	kcfg.UserAgent = "prometheus/discovery"

	if conf.BasicAuth != nil {
		kcfg.Username = conf.BasicAuth.Username
		kcfg.Password = conf.BasicAuth.Password
	}
	kcfg.TLSClientConfig.CertFile = conf.TLSConfig.CertFile
	kcfg.TLSClientConfig.KeyFile = conf.TLSConfig.KeyFile
	kcfg.Insecure = conf.TLSConfig.InsecureSkipVerify

	c, err := kubernetes.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return &Kubernetes{
		client: c,
		logger: l,
		role:   conf.Role,
	}, nil
}

const resyncPeriod = 10 * time.Minute

// Run implements the TargetProvider interface.
func (k *Kubernetes) Run(ctx context.Context, ch chan<- []*config.TargetGroup) {
	defer close(ch)

	rclient := k.client.Core().GetRESTClient()

	switch k.role {
	case "pod":
		plw := cache.NewListWatchFromClient(rclient, "pods", api.NamespaceAll, nil)
		pod := NewPods(
			k.logger.With("kubernetes_sd", "pod"),
			cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
		)
		go pod.informer.Run(ctx.Done())

		for !pod.informer.HasSynced() {
			time.Sleep(100 * time.Millisecond)
		}
		pod.Run(ctx, ch)
	default:
		k.logger.Errorf("unknown Kubernetes discovery kind %q", k.role)
	}

	<-ctx.Done()
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}

// Copyright 2018 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"fmt"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	extensionsv1beta1 "k8s.io/client-go/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/tools/cache"
)

type KubernetesShared interface {
	GetSharedInformer(resource string, namespace string) (informer cache.SharedInformer, err error)
	MustGetSharedInformer(resource string, namespace string) cache.SharedInformer
}

type kubernetesShared struct {
	sync.Mutex
	client    kubernetes.Interface
	count     int32
	informers map[string]cache.SharedInformer
}

func newKubernetesShared(client kubernetes.Interface) *kubernetesShared {
	return &kubernetesShared{client: client, count: 1, informers: map[string]cache.SharedInformer{}}
}

func (ks *kubernetesShared) MustGetSharedInformer(resource string, namespace string) cache.SharedInformer {
	informer, err := ks.GetSharedInformer(resource, namespace)
	if err != nil {
		panic(err)
	}
	return informer
}

func (ks *kubernetesShared) createAndRunSharedInformer(resource string, namespace string) (informer cache.SharedInformer, err error) {
	rclient := ks.client.CoreV1().RESTClient()
	reclient := ks.client.ExtensionsV1beta1().RESTClient()
	var lw *cache.ListWatch
	var obj runtime.Object
	switch resource {
	case "endpoints":
		lw = cache.NewListWatchFromClient(rclient, resource, namespace, nil)
		obj = &apiv1.Endpoints{}
	case "services":
		lw = cache.NewListWatchFromClient(rclient, resource, namespace, nil)
		obj = &apiv1.Service{}
	case "pods":
		lw = cache.NewListWatchFromClient(rclient, resource, namespace, nil)
		obj = &apiv1.Pod{}
	case "nodes":
		lw = cache.NewListWatchFromClient(rclient, resource, namespace, nil)
		obj = &apiv1.Node{}
	case "ingresses":
		lw = cache.NewListWatchFromClient(reclient, resource, namespace, nil)
		obj = &extensionsv1beta1.Ingress{}
	default:
		err = fmt.Errorf("unknown Kubernetes discovery kind: %s", resource)
		return
	}
	informer = cache.NewSharedInformer(lw, obj, resyncPeriod)
	return
}

func (ks *kubernetesShared) GetSharedInformer(resource string, namespace string) (informer cache.SharedInformer, err error) {
	ks.Lock()
	defer ks.Unlock()
	key := fmt.Sprintf("%s/%s", resource, namespace)
	informer, ok := ks.informers[key]
	if !ok {
		informer, err = ks.createAndRunSharedInformer(resource, namespace)
		if err != nil {
			return nil, err
		}
		ks.informers[key] = informer
	}
	return informer, nil
}

type KubernetesSharedCache interface {
	GetOrCreate(key string, create func() (*kubernetesShared, error)) (KubernetesShared, error)
	Count() int
	Release(key string)
	Start(stopCh <-chan struct{})
}

type kubernetesSharedCache struct {
	logger log.Logger
	sync.Mutex
	shared map[string]*kubernetesShared
}

func (c *kubernetesSharedCache) GetOrCreate(key string, create func() (*kubernetesShared, error)) (KubernetesShared, error) {
	c.Lock()
	defer c.Unlock()
	shared, ok := c.shared[key]
	if ok {
		shared.count++
	} else {
		var err error
		if create == nil {
			return nil, fmt.Errorf("create func should not be nil")
		}
		shared, err = create()
		if err != nil {
			return nil, err
		}
		c.shared[key] = shared
	}
	return shared, nil
}

func (c *kubernetesSharedCache) Release(key string) {
	c.Lock()
	defer c.Unlock()
	if shared, ok := c.shared[key]; ok {
		shared.count--
		if shared.count <= 0 {
			delete(c.shared, key)
		}
	}
}

func (c *kubernetesSharedCache) Count() int {
	c.Lock()
	defer c.Unlock()
	return len(c.shared)
}

// Start runs all informers. This is called after all discovery instances are
// configured, no need to lock.
func (c *kubernetesSharedCache) Start(stopCh <-chan struct{}) {
	for _, shared := range c.shared {
		for key, informer := range shared.informers {
			go informer.Run(stopCh)
			level.Info(c.logger).Log("msg", "Kubernetes informer started", "key", key)
		}
	}
}

func NewKubernetesSharedCache(l log.Logger) KubernetesSharedCache {
	return &kubernetesSharedCache{
		logger: l,
		shared: make(map[string]*kubernetesShared),
	}
}

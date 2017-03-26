/*
Copyright 2016 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cache

import (
	"fmt"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/labels"
)

//  TODO: generate these classes and methods for all resources of interest using
// a script.  Can use "go generate" once 1.4 is supported by all users.

// Lister makes an Index have the List method.  The Stores must contain only the expected type
// Example:
// s := cache.NewStore()
// lw := cache.ListWatch{Client: c, FieldSelector: sel, Resource: "pods"}
// r := cache.NewReflector(lw, &api.Pod{}, s).Run()
// l := StoreToPodLister{s}
// l.List()

// StoreToPodLister helps list pods
type StoreToPodLister struct {
	Indexer Indexer
}

func (s *StoreToPodLister) List(selector labels.Selector) (ret []*api.Pod, err error) {
	err = ListAll(s.Indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*api.Pod))
	})
	return ret, err
}

func (s *StoreToPodLister) Pods(namespace string) storePodsNamespacer {
	return storePodsNamespacer{Indexer: s.Indexer, namespace: namespace}
}

type storePodsNamespacer struct {
	Indexer   Indexer
	namespace string
}

func (s storePodsNamespacer) List(selector labels.Selector) (ret []*api.Pod, err error) {
	err = ListAllByNamespace(s.Indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*api.Pod))
	})
	return ret, err
}

func (s storePodsNamespacer) Get(name string) (*api.Pod, error) {
	obj, exists, err := s.Indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(api.Resource("pod"), name)
	}
	return obj.(*api.Pod), nil
}

// StoreToServiceLister helps list services
type StoreToServiceLister struct {
	Indexer Indexer
}

func (s *StoreToServiceLister) List(selector labels.Selector) (ret []*api.Service, err error) {
	err = ListAll(s.Indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*api.Service))
	})
	return ret, err
}

func (s *StoreToServiceLister) Services(namespace string) storeServicesNamespacer {
	return storeServicesNamespacer{s.Indexer, namespace}
}

type storeServicesNamespacer struct {
	indexer   Indexer
	namespace string
}

func (s storeServicesNamespacer) List(selector labels.Selector) (ret []*api.Service, err error) {
	err = ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*api.Service))
	})
	return ret, err
}

func (s storeServicesNamespacer) Get(name string) (*api.Service, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(api.Resource("service"), name)
	}
	return obj.(*api.Service), nil
}

// TODO: Move this back to scheduler as a helper function that takes a Store,
// rather than a method of StoreToServiceLister.
func (s *StoreToServiceLister) GetPodServices(pod *api.Pod) (services []*api.Service, err error) {
	allServices, err := s.Services(pod.Namespace).List(labels.Everything())
	if err != nil {
		return nil, err
	}

	for i := range allServices {
		service := allServices[i]
		if service.Spec.Selector == nil {
			// services with nil selectors match nothing, not everything.
			continue
		}
		selector := labels.Set(service.Spec.Selector).AsSelectorPreValidated()
		if selector.Matches(labels.Set(pod.Labels)) {
			services = append(services, service)
		}
	}

	return services, nil
}

// StoreToReplicationControllerLister helps list rcs
type StoreToReplicationControllerLister struct {
	Indexer Indexer
}

func (s *StoreToReplicationControllerLister) List(selector labels.Selector) (ret []*api.ReplicationController, err error) {
	err = ListAll(s.Indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*api.ReplicationController))
	})
	return ret, err
}

func (s *StoreToReplicationControllerLister) ReplicationControllers(namespace string) storeReplicationControllersNamespacer {
	return storeReplicationControllersNamespacer{s.Indexer, namespace}
}

type storeReplicationControllersNamespacer struct {
	indexer   Indexer
	namespace string
}

func (s storeReplicationControllersNamespacer) List(selector labels.Selector) (ret []*api.ReplicationController, err error) {
	err = ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*api.ReplicationController))
	})
	return ret, err
}

func (s storeReplicationControllersNamespacer) Get(name string) (*api.ReplicationController, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(api.Resource("replicationcontroller"), name)
	}
	return obj.(*api.ReplicationController), nil
}

// GetPodControllers returns a list of replication controllers managing a pod. Returns an error only if no matching controllers are found.
func (s *StoreToReplicationControllerLister) GetPodControllers(pod *api.Pod) (controllers []*api.ReplicationController, err error) {
	if len(pod.Labels) == 0 {
		err = fmt.Errorf("no controllers found for pod %v because it has no labels", pod.Name)
		return
	}

	key := &api.ReplicationController{ObjectMeta: api.ObjectMeta{Namespace: pod.Namespace}}
	items, err := s.Indexer.Index(NamespaceIndex, key)
	if err != nil {
		return
	}

	for _, m := range items {
		rc := m.(*api.ReplicationController)
		selector := labels.Set(rc.Spec.Selector).AsSelectorPreValidated()

		// If an rc with a nil or empty selector creeps in, it should match nothing, not everything.
		if selector.Empty() || !selector.Matches(labels.Set(pod.Labels)) {
			continue
		}
		controllers = append(controllers, rc)
	}
	if len(controllers) == 0 {
		err = fmt.Errorf("could not find controller for pod %s in namespace %s with labels: %v", pod.Name, pod.Namespace, pod.Labels)
	}
	return
}

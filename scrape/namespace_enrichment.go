// Copyright 2024 The Prometheus Authors
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

package scrape

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/strutil"
)

// NamespaceEnricher handles enriching metrics with namespace labels and annotations
type NamespaceEnricher struct {
	config      *config.NamespaceEnrichmentConfig
	cache       *NamespaceMetadataCache
	logger      *slog.Logger
	labelPrefix string
}

// NamespaceMetadata holds both labels and annotations for a namespace
type NamespaceMetadata struct {
	Labels      map[string]string
	Annotations map[string]string
}

// NamespaceMetadataCache caches namespace metadata to avoid frequent Kubernetes API calls
type NamespaceMetadataCache struct {
	mu         sync.RWMutex
	namespaces map[string]*NamespaceMetadata // namespace -> metadata
	client     kubernetes.Interface
	informer   cache.SharedInformer
	stopCh     chan struct{}
	logger     *slog.Logger
}

// NewNamespaceEnricher creates a new namespace enricher
func NewNamespaceEnricher(cfg *config.NamespaceEnrichmentConfig, client kubernetes.Interface, logger *slog.Logger) (*NamespaceEnricher, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	nsCache, err := NewNamespaceMetadataCache(client, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace metadata cache: %w", err)
	}

	labelPrefix := cfg.LabelPrefix
	if labelPrefix == "" {
		labelPrefix = "ns_"
	}

	return &NamespaceEnricher{
		config:      cfg,
		cache:       nsCache,
		logger:      logger,
		labelPrefix: labelPrefix,
	}, nil
}

// NewNamespaceMetadataCache creates a new namespace metadata cache
func NewNamespaceMetadataCache(client kubernetes.Interface, logger *slog.Logger) (*NamespaceMetadataCache, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client is required")
	}

	nsCache := &NamespaceMetadataCache{
		namespaces: make(map[string]*NamespaceMetadata),
		client:     client,
		stopCh:     make(chan struct{}),
		logger:     logger,
	}

	// Create informer for watching namespace changes
	listWatcher := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return client.CoreV1().Namespaces().List(context.TODO(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return client.CoreV1().Namespaces().Watch(context.TODO(), options)
		},
	}

	informer := cache.NewSharedInformer(listWatcher, &corev1.Namespace{}, 5*time.Minute)

	// Add event handlers
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				nsCache.updateNamespace(ns)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			if ns, ok := newObj.(*corev1.Namespace); ok {
				nsCache.updateNamespace(ns)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				nsCache.deleteNamespace(ns.Name)
			}
		},
	})

	nsCache.informer = informer
	return nsCache, nil
}

// Start starts the namespace metadata cache
func (nmc *NamespaceMetadataCache) Start(ctx context.Context) {
	go nmc.informer.Run(nmc.stopCh)

	// Wait for cache to sync
	if !cache.WaitForCacheSync(nmc.stopCh, nmc.informer.HasSynced) {
		nmc.logger.Error("Failed to sync namespace cache")
		return
	}

	nmc.logger.Info("Namespace metadata cache synced successfully")
}

// Stop stops the namespace metadata cache
func (nmc *NamespaceMetadataCache) Stop() {
	close(nmc.stopCh)
}

// updateNamespace updates the cached namespace metadata
func (nmc *NamespaceMetadataCache) updateNamespace(ns *corev1.Namespace) {
	nmc.mu.Lock()
	defer nmc.mu.Unlock()

	labels := make(map[string]string)
	for k, v := range ns.Labels {
		labels[k] = v
	}

	annotations := make(map[string]string)
	for k, v := range ns.Annotations {
		annotations[k] = v
	}

	nmc.namespaces[ns.Name] = &NamespaceMetadata{
		Labels:      labels,
		Annotations: annotations,
	}
	nmc.logger.Debug("Updated namespace metadata", "namespace", ns.Name, "labels", len(labels), "annotations", len(annotations))
}

// deleteNamespace removes namespace from cache
func (nmc *NamespaceMetadataCache) deleteNamespace(name string) {
	nmc.mu.Lock()
	defer nmc.mu.Unlock()

	delete(nmc.namespaces, name)
	nmc.logger.Debug("Deleted namespace metadata", "namespace", name)
}

// GetMetadata returns metadata (labels and annotations) for a namespace
func (nmc *NamespaceMetadataCache) GetMetadata(namespace string) *NamespaceMetadata {
	nmc.mu.RLock()
	defer nmc.mu.RUnlock()

	metadata, exists := nmc.namespaces[namespace]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	result := &NamespaceMetadata{
		Labels:      make(map[string]string, len(metadata.Labels)),
		Annotations: make(map[string]string, len(metadata.Annotations)),
	}

	for k, v := range metadata.Labels {
		result.Labels[k] = v
	}

	for k, v := range metadata.Annotations {
		result.Annotations[k] = v
	}

	return result
}

// Start starts the namespace enricher
func (ne *NamespaceEnricher) Start(ctx context.Context) {
	if ne.cache != nil {
		ne.cache.Start(ctx)
	}
}

// Stop stops the namespace enricher
func (ne *NamespaceEnricher) Stop() {
	if ne.cache != nil {
		ne.cache.Stop()
	}
}

// EnrichWithNamespaceMetadata enriches labels with namespace labels and annotations
func (ne *NamespaceEnricher) EnrichWithNamespaceMetadata(lset labels.Labels) labels.Labels {
	if ne == nil || ne.config == nil || !ne.config.Enabled {
		return lset
	}

	// Extract namespace from labels
	namespace := lset.Get("namespace")
	if namespace == "" {
		// Try alternative namespace label names
		namespace = lset.Get("__meta_kubernetes_namespace")
	}

	if namespace == "" {
		return lset
	}

	// Get namespace metadata (labels and annotations)
	metadata := ne.cache.GetMetadata(namespace)
	if metadata == nil {
		return lset
	}

	// Build enriched labels
	lb := labels.NewBuilder(lset)

	// Add selected namespace labels as metric labels
	for _, labelKey := range ne.config.LabelSelector {
		if labelValue, exists := metadata.Labels[labelKey]; exists {
			// Sanitize label key to be a valid label name
			labelName := ne.labelPrefix + strutil.SanitizeLabelName(labelKey)
			lb.Set(labelName, labelValue)
		}
	}

	// Add selected annotations as labels
	for _, annotationKey := range ne.config.AnnotationSelector {
		if annotationValue, exists := metadata.Annotations[annotationKey]; exists {
			// Sanitize annotation key to be a valid label name
			labelName := ne.labelPrefix + strutil.SanitizeLabelName(annotationKey)
			lb.Set(labelName, annotationValue)
		}
	}

	return lb.Labels()
}

// EnrichWithNamespaceAnnotations is maintained for backward compatibility
func (ne *NamespaceEnricher) EnrichWithNamespaceAnnotations(lset labels.Labels) labels.Labels {
	return ne.EnrichWithNamespaceMetadata(lset)
}

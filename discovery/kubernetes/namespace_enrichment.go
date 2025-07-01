// Copyright 2025 The Prometheus Authors
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
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/grafana/regexp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

// MetadataEnricher is a local interface to avoid import cycles.
type MetadataEnricher interface {
	EnrichWithMetadata(labels.Labels) labels.Labels
	Start(context.Context)
	Stop()
}

// ScrapeMetadataEnricherAdapter adapts the local MetadataEnricher to work with external packages
type ScrapeMetadataEnricherAdapter struct {
	enricher MetadataEnricher
}

func (s *ScrapeMetadataEnricherAdapter) EnrichWithMetadata(lset labels.Labels) labels.Labels {
	return s.enricher.EnrichWithMetadata(lset)
}

func (s *ScrapeMetadataEnricherAdapter) Start(ctx context.Context) {
	s.enricher.Start(ctx)
}

func (s *ScrapeMetadataEnricherAdapter) Stop() {
	s.enricher.Stop()
}

// NewScrapeMetadataEnricherAdapter creates an adapter for external use
func NewScrapeMetadataEnricherAdapter(enricher MetadataEnricher) *ScrapeMetadataEnricherAdapter {
	return &ScrapeMetadataEnricherAdapter{enricher: enricher}
}

// NamespaceEnricher implements the MetadataEnricher interface for Kubernetes namespaces.
type NamespaceEnricher struct {
	config      *config.NamespaceEnrichmentConfig
	cache       *NamespaceMetadataCache
	logger      *slog.Logger
	labelPrefix string
}

// NamespaceMetadata holds both labels and annotations for a namespace.
type NamespaceMetadata struct {
	Labels      map[string]string
	Annotations map[string]string
}

// NamespaceMetadataCache caches namespace metadata to avoid frequent Kubernetes API calls.
type NamespaceMetadataCache struct {
	mu         sync.RWMutex
	namespaces map[string]*NamespaceMetadata
	informer   cache.SharedIndexInformer
	stopCh     chan struct{}
}

// NewNamespaceEnricher creates a new Kubernetes namespace enricher.
func NewNamespaceEnricher(cfg *config.NamespaceEnrichmentConfig, client kubernetes.Interface, logger *slog.Logger) (*NamespaceEnricher, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	if client == nil {
		return nil, errors.New("kubernetes client is required")
	}

	prefix := cfg.LabelPrefix
	if prefix == "" {
		prefix = "ns_"
	}

	cache, err := NewNamespaceMetadataCache(client, cfg, logger)
	if err != nil {
		return nil, err
	}

	return &NamespaceEnricher{
		config:      cfg,
		cache:       cache,
		logger:      logger,
		labelPrefix: prefix,
	}, nil
}

// NewNamespaceMetadataCache creates a new namespace metadata cache.
func NewNamespaceMetadataCache(client kubernetes.Interface, cfg *config.NamespaceEnrichmentConfig, logger *slog.Logger) (*NamespaceMetadataCache, error) {
	if client == nil {
		return nil, errors.New("kubernetes client is required")
	}

	factory := informers.NewSharedInformerFactory(client, 0)
	informer := factory.Core().V1().Namespaces().Informer()

	nmc := &NamespaceMetadataCache{
		namespaces: make(map[string]*NamespaceMetadata),
		informer:   informer,
		stopCh:     make(chan struct{}),
	}

	_, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				nmc.updateNamespace(ns, cfg, logger)
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			if ns, ok := newObj.(*corev1.Namespace); ok {
				nmc.updateNamespace(ns, cfg, logger)
			}
		},
		DeleteFunc: func(obj interface{}) {
			if ns, ok := obj.(*corev1.Namespace); ok {
				nmc.deleteNamespace(ns.Name)
			}
		},
	})
	if err != nil {
		return nil, err
	}

	return nmc, nil
}

// Start starts the namespace metadata cache.
func (nmc *NamespaceMetadataCache) Start(_ context.Context) {
	go nmc.informer.Run(nmc.stopCh)

	// Wait for initial sync
	if !cache.WaitForCacheSync(nmc.stopCh, nmc.informer.HasSynced) {
		close(nmc.stopCh)
		return
	}
}

// Stop stops the namespace metadata cache.
func (nmc *NamespaceMetadataCache) Stop() {
	close(nmc.stopCh)
}

// updateNamespace updates the cached namespace metadata.
func (nmc *NamespaceMetadataCache) updateNamespace(ns *corev1.Namespace, cfg *config.NamespaceEnrichmentConfig, logger *slog.Logger) {
	metadata := &NamespaceMetadata{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// Extract selected labels
	for _, labelKey := range cfg.LabelSelector {
		if value, exists := ns.Labels[labelKey]; exists {
			metadata.Labels[labelKey] = value
		}
	}

	// Extract selected annotations
	for _, annotationKey := range cfg.AnnotationSelector {
		if value, exists := ns.Annotations[annotationKey]; exists {
			metadata.Annotations[annotationKey] = value
		}
	}

	nmc.mu.Lock()
	nmc.namespaces[ns.Name] = metadata
	nmc.mu.Unlock()

	logger.Debug("Updated namespace metadata", "namespace", ns.Name, "labels", len(metadata.Labels), "annotations", len(metadata.Annotations))
}

// deleteNamespace removes namespace from cache.
func (nmc *NamespaceMetadataCache) deleteNamespace(name string) {
	nmc.mu.Lock()
	delete(nmc.namespaces, name)
	nmc.mu.Unlock()
}

// GetMetadata returns metadata (labels and annotations) for a namespace.
func (nmc *NamespaceMetadataCache) GetMetadata(namespaceName string) *NamespaceMetadata {
	nmc.mu.RLock()
	defer nmc.mu.RUnlock()

	metadata := nmc.namespaces[namespaceName]
	if metadata == nil {
		return &NamespaceMetadata{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		}
	}

	// Return copy to avoid data races
	result := &NamespaceMetadata{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
	for k, v := range metadata.Labels {
		result.Labels[k] = v
	}
	for k, v := range metadata.Annotations {
		result.Annotations[k] = v
	}
	return result
}

// Start starts the namespace enricher.
func (ne *NamespaceEnricher) Start(ctx context.Context) {
	if ne == nil || ne.cache == nil {
		return
	}
	ne.cache.Start(ctx)
}

// Stop stops the namespace enricher.
func (ne *NamespaceEnricher) Stop() {
	if ne == nil || ne.cache == nil {
		return
	}
	ne.cache.Stop()
}

// EnrichWithMetadata enriches labels with namespace labels and annotations.
func (ne *NamespaceEnricher) EnrichWithMetadata(lset labels.Labels) labels.Labels {
	if ne == nil || ne.config == nil || !ne.config.Enabled {
		return lset
	}

	// Look for namespace in various label keys
	namespaceName := ""
	for _, namespaceLabel := range []string{
		"__meta_kubernetes_namespace",
		"namespace",
		"exported_namespace",
		"__meta_kubernetes_pod_namespace",
		"kubernetes_namespace",
	} {
		if val := lset.Get(namespaceLabel); val != "" {
			namespaceName = val
			break
		}
	}

	if namespaceName == "" {
		return lset
	}

	metadata := ne.cache.GetMetadata(namespaceName)
	if metadata == nil {
		return lset
	}

	// Build new label set
	builder := labels.NewBuilder(lset)

	// Add namespace labels
	for labelKey, labelValue := range metadata.Labels {
		enrichedLabelName := ne.labelPrefix + sanitizeLabelName(labelKey)
		builder.Set(enrichedLabelName, labelValue)
	}

	// Add namespace annotations
	for annotationKey, annotationValue := range metadata.Annotations {
		enrichedLabelName := ne.labelPrefix + sanitizeLabelName(annotationKey)
		builder.Set(enrichedLabelName, annotationValue)
	}

	return builder.Labels()
}

// sanitizeLabelName converts annotation keys to valid Prometheus label names.
var labelNameRegex = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func sanitizeLabelName(name string) string {
	return labelNameRegex.ReplaceAllString(name, "_")
}

// KubernetesEnricherFactory implements enricher factory for Kubernetes.
type KubernetesEnricherFactory struct{}

// CreateEnricher creates a new Kubernetes namespace enricher.
func (f *KubernetesEnricherFactory) CreateEnricher(cfg interface{}, logger *slog.Logger) (interface{}, error) {
	namespaceCfg, ok := cfg.(*config.NamespaceEnrichmentConfig)
	if !ok {
		return nil, errors.New("invalid configuration type for Kubernetes enricher")
	}

	if namespaceCfg == nil || !namespaceCfg.Enabled {
		return nil, nil
	}

	client, err := f.createKubernetesClient()
	if err != nil {
		return nil, err
	}

	enricher, err := NewNamespaceEnricher(namespaceCfg, client, logger)
	if err != nil {
		return nil, err
	}

	if enricher == nil {
		return nil, nil
	}

	// Return adapter that can be used by external packages
	return NewScrapeMetadataEnricherAdapter(enricher), nil
}

// createKubernetesClient attempts to create a Kubernetes client if running in-cluster.
func (f *KubernetesEnricherFactory) createKubernetesClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// KubernetesEnricherFactoryFunc is the factory function for creating Kubernetes enrichers.
// It matches the EnricherFactory signature from the scrape package.
func KubernetesEnricherFactoryFunc(cfg *config.NamespaceEnrichmentConfig, logger *slog.Logger) (interface{}, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	factory := &KubernetesEnricherFactory{}
	return factory.CreateEnricher(cfg, logger)
}

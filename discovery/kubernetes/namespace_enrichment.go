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
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/grafana/regexp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/scrape"
)

// NamespaceEnrichmentConfigInterface provides an interface for enrichment configuration
// This helps avoid import cycles between packages
type NamespaceEnrichmentConfigInterface interface {
	IsEnabled() bool
	GetLabelPrefix() string
	GetLabelSelector() []string
	GetAnnotationSelector() []string
	GetMaxCacheSize() int
	GetCacheTTL() int64
	GetMaxSelectors() int
}

// validateEnrichmentConfig performs comprehensive validation of enrichment configuration
func validateEnrichmentConfig(cfg *NamespaceEnrichmentConfig) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	// Skip validation if not enabled
	if !cfg.Enabled {
		return nil
	}

	// When enabled, must have at least one selector
	if len(cfg.LabelSelector) == 0 && len(cfg.AnnotationSelector) == 0 {
		return fmt.Errorf("at least one label or annotation selector is required when enrichment is enabled")
	}

	// Validate label prefix format
	if cfg.LabelPrefix != "" && !strings.HasSuffix(cfg.LabelPrefix, "_") {
		return fmt.Errorf("label_prefix must end with underscore")
	}

	// Validate cache size limits - prevent memory exhaustion
	if cfg.MaxCacheSize < 0 {
		return fmt.Errorf("max_cache_size cannot be negative")
	}
	if cfg.MaxCacheSize > 100000 {
		return fmt.Errorf("max_cache_size too large (max: 100000), got: %d", cfg.MaxCacheSize)
	}

	// Validate TTL limits - prevent excessive API load or stale data
	if cfg.CacheTTL < 0 {
		return fmt.Errorf("cache_ttl cannot be negative")
	}
	if cfg.CacheTTL > 0 && cfg.CacheTTL < 30 {
		return fmt.Errorf("cache_ttl below minimum (30s), got: %ds", cfg.CacheTTL)
	}
	if cfg.CacheTTL > 3600 {
		return fmt.Errorf("cache_ttl exceeds maximum (3600s), got: %ds", cfg.CacheTTL)
	}

	// Validate selector limits - prevent cardinality explosion
	totalSelectors := len(cfg.LabelSelector) + len(cfg.AnnotationSelector)
	if totalSelectors > 100 {
		return fmt.Errorf("too many selectors (max: 100), got: %d", totalSelectors)
	}

	if cfg.MaxSelectors > 0 && totalSelectors > cfg.MaxSelectors {
		return fmt.Errorf("selector count (%d) exceeds max_selectors (%d)", totalSelectors, cfg.MaxSelectors)
	}

	// Security validation for sensitive selectors
	allSelectors := append(cfg.LabelSelector, cfg.AnnotationSelector...)
	for _, selector := range allSelectors {
		if isSecuritySensitive(selector) {
			return fmt.Errorf("selector '%s' potentially dangerous for security reasons", selector)
		}
	}

	return nil
}

// isSecuritySensitive checks if a selector contains sensitive information
func isSecuritySensitive(selector string) bool {
	sensitivePatterns := []string{
		"password", "secret", "token", "key", "credential",
		"budget", "financial", "salary", "private",
	}

	selectorLower := strings.ToLower(selector)
	for _, pattern := range sensitivePatterns {
		if strings.Contains(selectorLower, pattern) {
			return true
		}
	}
	return false
}

// NamespaceEnrichmentConfig represents namespace enrichment configuration.
// This mirrors the config package struct to avoid import cycles.
type NamespaceEnrichmentConfig struct {
	Enabled            bool     `yaml:"enabled"`
	LabelPrefix        string   `yaml:"label_prefix"`
	LabelSelector      []string `yaml:"label_selector"`
	AnnotationSelector []string `yaml:"annotation_selector"`

	// Resource limits for experimental feature safety
	MaxCacheSize int   `yaml:"max_cache_size,omitempty"` // Maximum number of namespaces to cache (default: 1000)
	CacheTTL     int64 `yaml:"cache_ttl,omitempty"`      // TTL for cache entries in seconds (default: 300)
	MaxSelectors int   `yaml:"max_selectors,omitempty"`  // Maximum total selectors (default: 20)
}

// Interface methods to implement NamespaceEnrichmentConfigInterface
func (c *NamespaceEnrichmentConfig) IsEnabled() bool {
	return c.Enabled
}

func (c *NamespaceEnrichmentConfig) GetLabelPrefix() string {
	return c.LabelPrefix
}

func (c *NamespaceEnrichmentConfig) GetLabelSelector() []string {
	return c.LabelSelector
}

func (c *NamespaceEnrichmentConfig) GetAnnotationSelector() []string {
	return c.AnnotationSelector
}

func (c *NamespaceEnrichmentConfig) GetMaxCacheSize() int {
	return c.MaxCacheSize
}

func (c *NamespaceEnrichmentConfig) GetCacheTTL() int64 {
	return c.CacheTTL
}

func (c *NamespaceEnrichmentConfig) GetMaxSelectors() int {
	return c.MaxSelectors
}

// MetadataEnricher is a local interface to avoid import cycles.
type MetadataEnricher interface {
	EnrichWithMetadata(labels.Labels) labels.Labels
	Start(context.Context)
	Stop()
}

// MetadataEnricherWrapper wraps the local MetadataEnricher to implement the external interface
type MetadataEnricherWrapper struct {
	enricher MetadataEnricher
}

func (m *MetadataEnricherWrapper) EnrichWithMetadata(lset labels.Labels) labels.Labels {
	if m.enricher == nil {
		return lset
	}
	return m.enricher.EnrichWithMetadata(lset)
}

func (m *MetadataEnricherWrapper) Start(ctx context.Context) {
	if m.enricher != nil {
		m.enricher.Start(ctx)
	}
}

func (m *MetadataEnricherWrapper) Stop() {
	if m.enricher != nil {
		m.enricher.Stop()
	}
}

// NewMetadataEnricherWrapper creates a wrapper for external use
func NewMetadataEnricherWrapper(enricher MetadataEnricher) *MetadataEnricherWrapper {
	return &MetadataEnricherWrapper{enricher: enricher}
}

// NamespaceEnricher implements the MetadataEnricher interface for Kubernetes namespaces.
type NamespaceEnricher struct {
	config      *NamespaceEnrichmentConfig
	cache       *NamespaceMetadataCache
	logger      *slog.Logger
	labelPrefix string

	// Graceful degradation state
	mu            sync.RWMutex
	isHealthy     bool
	lastError     error
	errorCount    int
	lastErrorTime time.Time
	maxErrors     int
}

// NamespaceMetadata holds both labels and annotations for a namespace.
type NamespaceMetadata struct {
	Labels      map[string]string
	Annotations map[string]string
	LastUpdated int64 // Unix timestamp for TTL tracking
}

// NamespaceMetadataCache caches namespace metadata to avoid frequent Kubernetes API calls.
type NamespaceMetadataCache struct {
	mu            sync.RWMutex
	namespaces    map[string]*NamespaceMetadata
	informer      cache.SharedIndexInformer
	stopCh        chan struct{}
	maxCacheSize  int
	cacheTTL      int64
	sharedManager *SharedInformerManager

	// Internal tracking
	cacheSize   int
	evictions   int64
	lastCleanup int64
}

// labelBuilderPool provides object pooling for labels.Builder to reduce GC pressure
var labelBuilderPool = sync.Pool{
	New: func() interface{} {
		return labels.NewBuilder(labels.EmptyLabels())
	},
}

// getLabelBuilder gets a labels.Builder from the pool
func getLabelBuilder(lset labels.Labels) *labels.Builder {
	builder := labelBuilderPool.Get().(*labels.Builder)
	builder.Reset(lset)
	return builder
}

// putLabelBuilder returns a labels.Builder to the pool
func putLabelBuilder(builder *labels.Builder) {
	// Clear the builder to prevent memory leaks
	builder.Reset(labels.EmptyLabels())
	labelBuilderPool.Put(builder)
}

// NewNamespaceEnricher creates a new Kubernetes namespace enricher.
func NewNamespaceEnricher(cfg *NamespaceEnrichmentConfig, client kubernetes.Interface, logger *slog.Logger) (*NamespaceEnricher, error) {
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
		isHealthy:   true,
		maxErrors:   10, // Allow up to 10 errors before marking unhealthy
	}, nil
}

// NewNamespaceMetadataCache creates a new namespace metadata cache using shared informer.
func NewNamespaceMetadataCache(client kubernetes.Interface, cfg *NamespaceEnrichmentConfig, logger *slog.Logger) (*NamespaceMetadataCache, error) {
	if client == nil {
		return nil, errors.New("kubernetes client is required")
	}

	// Get shared informer manager
	sharedManager := getSharedInformerManager(client, logger)
	informer, err := sharedManager.getInformer()
	if err != nil {
		return nil, fmt.Errorf("failed to get shared informer: %w", err)
	}

	nmc := &NamespaceMetadataCache{
		namespaces:    make(map[string]*NamespaceMetadata),
		informer:      informer,
		stopCh:        make(chan struct{}),
		maxCacheSize:  cfg.MaxCacheSize,
		cacheTTL:      cfg.CacheTTL,
		sharedManager: sharedManager,
	}

	_, err = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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
		// Release the informer reference on error
		sharedManager.releaseInformer()
		return nil, err
	}

	return nmc, nil
}

// Start starts the namespace metadata cache using shared informer.
func (nmc *NamespaceMetadataCache) Start(ctx context.Context) {
	if nmc.sharedManager == nil {
		return
	}

	// Start the shared informer if not already started
	if err := nmc.sharedManager.startInformer(ctx); err != nil {
		// Log error but don't fail completely - graceful degradation
		return
	}
}

// Stop stops the namespace metadata cache and releases shared informer reference.
func (nmc *NamespaceMetadataCache) Stop() {
	close(nmc.stopCh)

	// Release the shared informer reference
	if nmc.sharedManager != nil {
		nmc.sharedManager.releaseInformer()
	}
}

// updateNamespace updates the cached namespace metadata.
func (nmc *NamespaceMetadataCache) updateNamespace(ns *corev1.Namespace, cfg *NamespaceEnrichmentConfig, logger *slog.Logger) {
	// Set defaults for limits if not configured
	maxCacheSize := cfg.MaxCacheSize
	if maxCacheSize <= 0 {
		maxCacheSize = 1000
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = 300 // 5 minutes default
	}

	metadata := &NamespaceMetadata{
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
		LastUpdated: time.Now().Unix(),
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
	defer nmc.mu.Unlock()

	// Add/update the namespace first
	nmc.namespaces[ns.Name] = metadata

	// Cleanup expired entries and enforce size limits after adding new entry
	now := time.Now().Unix()
	if now-nmc.lastCleanup > 60 { // Cleanup every minute
		nmc.cleanupExpiredEntries(now, cacheTTL, logger)
		nmc.lastCleanup = now
	}

	// Always enforce size limit when adding entries
	nmc.enforceSizeLimit(maxCacheSize, logger)

	nmc.cacheSize = len(nmc.namespaces)

	logger.Debug("Updated namespace metadata",
		"namespace", ns.Name,
		"labels", len(metadata.Labels),
		"annotations", len(metadata.Annotations),
		"cache_size", nmc.cacheSize)
}

// cleanupExpiredEntries removes entries older than TTL
func (nmc *NamespaceMetadataCache) cleanupExpiredEntries(now, cacheTTL int64, logger *slog.Logger) {
	expiredCount := 0
	for name, metadata := range nmc.namespaces {
		if now-metadata.LastUpdated > cacheTTL {
			delete(nmc.namespaces, name)
			expiredCount++
		}
	}
	if expiredCount > 0 {
		logger.Debug("Cleaned up expired namespace cache entries", "count", expiredCount)
	}
}

// enforceSizeLimit removes oldest entries if cache exceeds size limit
func (nmc *NamespaceMetadataCache) enforceSizeLimit(maxCacheSize int, logger *slog.Logger) {
	if len(nmc.namespaces) <= maxCacheSize {
		return
	}

	// Find oldest entries to evict
	type entry struct {
		name        string
		lastUpdated int64
	}

	entries := make([]entry, 0, len(nmc.namespaces))
	for name, metadata := range nmc.namespaces {
		entries = append(entries, entry{name: name, lastUpdated: metadata.LastUpdated})
	}

	// Sort by age (oldest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastUpdated < entries[j].lastUpdated
	})

	// Remove oldest entries until we're under the limit
	toRemove := len(nmc.namespaces) - maxCacheSize
	for i := 0; i < toRemove; i++ {
		delete(nmc.namespaces, entries[i].name)
		nmc.evictions++
	}

	if toRemove > 0 {
		logger.Warn("Evicted oldest namespace cache entries due to size limit",
			"evicted", toRemove, "cache_size", len(nmc.namespaces))
	}
}

// deleteNamespace removes a namespace from the cache.
func (nmc *NamespaceMetadataCache) deleteNamespace(name string) {
	nmc.mu.Lock()
	defer nmc.mu.Unlock()
	delete(nmc.namespaces, name)
	nmc.cacheSize = len(nmc.namespaces)
}

// GetMetadata retrieves namespace metadata from cache.
func (nmc *NamespaceMetadataCache) GetMetadata(namespaceName string) *NamespaceMetadata {
	if nmc == nil {
		return nil
	}

	nmc.mu.RLock()
	defer nmc.mu.RUnlock()

	// Check if namespace exists in cache
	if metadata, exists := nmc.namespaces[namespaceName]; exists {
		// Check if entry has expired
		if nmc.cacheTTL > 0 {
			age := time.Now().Unix() - metadata.LastUpdated
			if age > nmc.cacheTTL {
				// Entry expired but don't remove here (will be cleaned up later)
				return nil
			}
		}
		return metadata
	}

	return nil
}

// hasSyncedNamespaces checks if the informer has synced the initial list
func (nmc *NamespaceMetadataCache) hasSyncedNamespaces() bool {
	if nmc.informer != nil {
		return nmc.informer.HasSynced()
	}
	return false
}

// Start starts the namespace enricher.
func (ne *NamespaceEnricher) Start(ctx context.Context) {
	if ne == nil {
		return
	}

	// Start cache informer
	if ne.cache != nil {
		ne.cache.Start(ctx)
	}

	ne.logger.Info("Started namespace enricher",
		"label_prefix", ne.labelPrefix,
		"label_selectors", len(ne.config.LabelSelector),
		"annotation_selectors", len(ne.config.AnnotationSelector),
		"max_cache_size", ne.config.MaxCacheSize,
		"cache_ttl", ne.config.CacheTTL)
}

// Stop stops the namespace enricher.
func (ne *NamespaceEnricher) Stop() {
	if ne == nil {
		return
	}

	// Stop cache informer
	if ne.cache != nil {
		ne.cache.Stop()
	}

	ne.logger.Info("Stopped namespace enricher")
}

// EnrichWithMetadata enriches labels with namespace metadata.
func (ne *NamespaceEnricher) EnrichWithMetadata(lset labels.Labels) labels.Labels {
	if ne == nil || !ne.isHealthyToEnrich() {
		return lset
	}

	// Graceful degradation with panic recovery
	defer func() {
		if r := recover(); r != nil {
			ne.recordError(fmt.Errorf("panic in enrichment: %v", r))
			ne.logger.Error("Namespace enrichment panic recovered", "panic", r)
		}
	}()

	// Extract namespace from various possible label formats
	var namespaceName string
	lset.Range(func(l labels.Label) {
		switch l.Name {
		case "namespace":
			namespaceName = l.Value
		case "__meta_kubernetes_namespace":
			namespaceName = l.Value
		}
	})

	if namespaceName == "" {
		return lset
	}

	// Skip enrichment for node-level metrics
	metricNameLabel := lset.Get("__name__")
	if isNodeLevelMetric(metricNameLabel, lset) {
		return lset
	}

	// Get namespace metadata with error handling
	metadata, err := ne.getMetadataWithErrorHandling(namespaceName)
	if err != nil {
		ne.recordError(err)
		return lset // Return original labels on error (graceful degradation)
	}

	if metadata == nil {
		return lset
	}

	// Build enriched labels with error handling
	enrichedLabels, err := ne.buildEnrichedLabels(lset, metadata)
	if err != nil {
		ne.recordError(err)
		return lset // Return original labels on error
	}

	ne.recordSuccess()
	return enrichedLabels
}

// getMetadataWithErrorHandling safely retrieves namespace metadata
func (ne *NamespaceEnricher) getMetadataWithErrorHandling(namespaceName string) (*NamespaceMetadata, error) {
	if ne.cache == nil {
		return nil, fmt.Errorf("cache not initialized")
	}

	return ne.cache.GetMetadata(namespaceName), nil
}

// buildEnrichedLabels safely builds enriched labels with namespace metadata
func (ne *NamespaceEnricher) buildEnrichedLabels(lset labels.Labels, metadata *NamespaceMetadata) (labels.Labels, error) {
	// Use object pooling to reduce GC pressure
	builder := getLabelBuilder(lset)
	defer putLabelBuilder(builder)

	// Add namespace labels with configured prefix
	for key, value := range metadata.Labels {
		if key == "" || value == "" {
			continue // Skip empty keys or values
		}
		enrichedKey := ne.labelPrefix + sanitizeLabelName(key)
		builder.Set(enrichedKey, value)
	}

	// Add namespace annotations with configured prefix
	for key, value := range metadata.Annotations {
		if key == "" || value == "" {
			continue // Skip empty keys or values
		}
		enrichedKey := ne.labelPrefix + sanitizeLabelName(key)
		builder.Set(enrichedKey, value)
	}

	return builder.Labels(), nil
}

// isNodeLevelMetric determines if a metric represents node-level information
// Node-level metrics should not be enriched with namespace metadata
func isNodeLevelMetric(metricName string, lset labels.Labels) bool {
	// Node-level metric patterns
	nodeMetricPrefixes := []string{
		"node_",
		"kube_node_",
		"kubelet_",
		"container_fs_",
		"container_network_",
		"machine_",
		"cadvisor_version_info",
	}

	for _, prefix := range nodeMetricPrefixes {
		if strings.HasPrefix(metricName, prefix) {
			return true
		}
	}

	// Check for cluster-level kube-state-metrics (not namespace-scoped)
	clusterLevelPrefixes := []string{
		"kube_persistentvolume_",   // Cluster-level storage (not namespace-scoped)
		"kube_storageclass_",       // Cluster-level storage classes
		"kube_clusterrole_",        // Cluster-level RBAC
		"kube_clusterrolebinding_", // Cluster-level RBAC
		"kube_namespace_",          // Metrics ABOUT namespaces (not scoped TO them)
	}

	for _, prefix := range clusterLevelPrefixes {
		if strings.HasPrefix(metricName, prefix) {
			return true
		}
	}

	// Check for node-related jobs
	job := lset.Get("job")
	if job == "kubernetes-nodes" || job == "kubernetes-node-exporter" {
		return true
	}

	// Check for node-exporter style metrics
	nodeLabel := lset.Get("node")
	instanceLabel := lset.Get("instance")
	if nodeLabel != "" && instanceLabel != "" {
		// Metrics with both node and instance labels are typically node-level
		return true
	}

	return false
}

// sanitizeLabelName ensures label names conform to Prometheus requirements
func sanitizeLabelName(name string) string {
	// Basic sanitization - replace invalid characters with underscores
	return regexp.MustCompile(`[^a-zA-Z0-9_:]`).ReplaceAllString(name, "_")
}

// KubernetesEnricherFactory implements enricher factory for registration
type KubernetesEnricherFactory struct{}

func (f *KubernetesEnricherFactory) CreateEnricher(cfg interface{}, logger *slog.Logger) (scrape.MetadataEnricher, error) {
	config, ok := cfg.(*NamespaceEnrichmentConfig)
	if !ok {
		return nil, fmt.Errorf("invalid config type for Kubernetes enricher")
	}

	if !config.Enabled {
		return nil, nil
	}

	// Validate configuration
	if err := validateEnrichmentConfig(config); err != nil {
		return nil, fmt.Errorf("invalid enrichment configuration: %w", err)
	}

	// Create Kubernetes client
	client, err := f.createKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create enricher
	enricher, err := NewNamespaceEnricher(config, client, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace enricher: %w", err)
	}

	// Return wrapper for external use
	return NewMetadataEnricherWrapper(enricher), nil
}

func (f *KubernetesEnricherFactory) createKubernetesClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// KubernetesEnricherFactoryFunc provides a function-based factory for registration
func KubernetesEnricherFactoryFunc(cfg interface{}, logger *slog.Logger) (scrape.MetadataEnricher, error) {
	// Handle both config package type and local type for flexibility
	var config *NamespaceEnrichmentConfig

	switch c := cfg.(type) {
	case *NamespaceEnrichmentConfig:
		config = c
	case NamespaceEnrichmentConfigInterface:
		// Convert from config package type to local type
		config = &NamespaceEnrichmentConfig{
			Enabled:            c.IsEnabled(),
			LabelPrefix:        c.GetLabelPrefix(),
			LabelSelector:      c.GetLabelSelector(),
			AnnotationSelector: c.GetAnnotationSelector(),
			MaxCacheSize:       c.GetMaxCacheSize(),
			CacheTTL:           c.GetCacheTTL(),
			MaxSelectors:       c.GetMaxSelectors(),
		}
	default:
		return nil, fmt.Errorf("invalid config type for Kubernetes enricher: expected NamespaceEnrichmentConfig or NamespaceEnrichmentConfigInterface, got %T", cfg)
	}

	if !config.Enabled {
		return nil, nil
	}

	// Validate configuration
	if err := validateEnrichmentConfig(config); err != nil {
		return nil, fmt.Errorf("invalid enrichment configuration: %w", err)
	}

	// Create Kubernetes client
	client, err := createKubernetesClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create enricher
	enricher, err := NewNamespaceEnricher(config, client, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create namespace enricher: %w", err)
	}

	// Return wrapper for external use
	return NewMetadataEnricherWrapper(enricher), nil
}

func createKubernetesClient() (kubernetes.Interface, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// Error handling and health checking methods

// recordError records an error and updates health status
func (ne *NamespaceEnricher) recordError(err error) {
	ne.mu.Lock()
	defer ne.mu.Unlock()

	ne.lastError = err
	ne.errorCount++
	ne.lastErrorTime = time.Now()

	// Mark as unhealthy if too many errors
	if ne.errorCount >= ne.maxErrors {
		ne.isHealthy = false
		ne.logger.Warn("Namespace enricher marked unhealthy due to errors",
			"error_count", ne.errorCount,
			"max_errors", ne.maxErrors,
			"last_error", err)
	}
}

// recordSuccess records a successful operation and may restore health
func (ne *NamespaceEnricher) recordSuccess() {
	ne.mu.Lock()
	defer ne.mu.Unlock()

	// Reset error count on success and restore health
	if ne.errorCount > 0 {
		ne.logger.Debug("Namespace enricher recovering",
			"previous_error_count", ne.errorCount)
		ne.errorCount = 0
		ne.lastError = nil
		ne.isHealthy = true
	}
}

// isHealthyToEnrich checks if the enricher is healthy enough to perform enrichment
func (ne *NamespaceEnricher) isHealthyToEnrich() bool {
	ne.mu.RLock()
	defer ne.mu.RUnlock()

	// Allow enrichment if healthy or if enough time has passed since last error
	if ne.isHealthy {
		return true
	}

	// Auto-recovery after 5 minutes of no errors
	if time.Since(ne.lastErrorTime) > 5*time.Minute {
		ne.mu.RUnlock()
		ne.mu.Lock()
		ne.isHealthy = true
		ne.errorCount = 0
		ne.lastError = nil
		ne.mu.Unlock()
		ne.mu.RLock()
		ne.logger.Info("Namespace enricher auto-recovered after timeout")
		return true
	}

	return false
}

// SharedInformerManager manages a shared namespace informer to avoid multiple watchers
type SharedInformerManager struct {
	mu              sync.RWMutex
	informerFactory informers.SharedInformerFactory
	informer        cache.SharedIndexInformer
	stopCh          chan struct{}
	refCount        int
	isStarted       bool
	client          kubernetes.Interface
	logger          *slog.Logger
}

var (
	sharedInformerMutex sync.Mutex
	sharedInformerMap   = make(map[kubernetes.Interface]*SharedInformerManager)
)

func getSharedInformerManager(client kubernetes.Interface, logger *slog.Logger) *SharedInformerManager {
	sharedInformerMutex.Lock()
	defer sharedInformerMutex.Unlock()

	if manager, exists := sharedInformerMap[client]; exists {
		return manager
	}

	// Create new shared informer manager
	factory := informers.NewSharedInformerFactory(client, 5*time.Minute)
	manager := &SharedInformerManager{
		informerFactory: factory,
		informer:        factory.Core().V1().Namespaces().Informer(),
		stopCh:          make(chan struct{}),
		client:          client,
		logger:          logger,
	}

	sharedInformerMap[client] = manager
	return manager
}

// getInformer returns the shared informer and increments reference count
func (sim *SharedInformerManager) getInformer() (cache.SharedIndexInformer, error) {
	sim.mu.Lock()
	defer sim.mu.Unlock()

	sim.refCount++
	return sim.informer, nil
}

// startInformer starts the shared informer if not already started
func (sim *SharedInformerManager) startInformer(ctx context.Context) error {
	sim.mu.Lock()
	defer sim.mu.Unlock()

	if sim.isStarted {
		return nil
	}

	// Start the informer factory
	sim.informerFactory.Start(sim.stopCh)

	// Wait for cache to sync
	if !cache.WaitForCacheSync(ctx.Done(), sim.informer.HasSynced) {
		return fmt.Errorf("failed to sync namespace informer cache")
	}

	sim.isStarted = true
	sim.logger.Debug("Started shared namespace informer")
	return nil
}

// releaseInformer decrements reference count and stops if no more references
func (sim *SharedInformerManager) releaseInformer() {
	sim.mu.Lock()
	defer sim.mu.Unlock()

	sim.refCount--
	if sim.refCount <= 0 {
		close(sim.stopCh)
		sim.isStarted = false

		// Remove from global map
		sharedInformerMutex.Lock()
		delete(sharedInformerMap, sim.client)
		sharedInformerMutex.Unlock()

		sim.logger.Debug("Stopped shared namespace informer")
	}
}

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

package kubernetes

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"

	"github.com/prometheus/prometheus/secrets"
	"github.com/prometheus/prometheus/secrets/provider"
)

// WatchSPConfig configures access to the Kubernetes API server.
// TODO(TheSpiritXIII): https://github.com/GoogleCloudPlatform/prometheus-engine/issues/867
type WatchSPConfig struct {
	ClientConfig
	SecretConfig
}

func init() {
	secrets.RegisterProvider("kubernetes_watch", func(ctx context.Context, opts secrets.ProviderOptions, _ prometheus.Registerer) secrets.Provider[*yaml.Node] {
		return provider.MapNode(provider.NewMultiProvider(
			ctx,
			provider.WithClientConfigFunc(func(config *WatchSPConfig) (*ClientConfig, error) {
				return &config.ClientConfig, nil
			}),
			provider.WithNewProviderFunc[*WatchSPConfig, *ClientConfig](func(ctx context.Context, config *WatchSPConfig) (secrets.Provider[*WatchSPConfig], error) {
				return config.getProvider(ctx, opts)
			}),
		))
	})
}

func (config *WatchSPConfig) getProvider(ctx context.Context, opts secrets.ProviderOptions) (secrets.Provider[*WatchSPConfig], error) {
	client, err := config.ClientConfig.client()
	if err != nil {
		return nil, err
	}
	watch, err := newWatchProvider(ctx, opts.Logger, client), nil
	if err != nil {
		return nil, err
	}
	return provider.Map(func(config *WatchSPConfig) (*SecretConfig, error) {
		return &config.SecretConfig, nil
	}, provider.NewDynamicProvider(watch)), nil
}

type secretWatcher struct {
	// Add, Update and Remove are synchronous. We need to lock everything but `refCount`.
	mu       sync.Mutex
	w        watch.Interface
	s        *corev1.Secret
	refCount uint
	done     bool
}

func newWatcher(ctx context.Context, logger log.Logger, client kubernetes.Interface, config *SecretConfig) (*secretWatcher, error) {
	watcher := &secretWatcher{
		refCount: 1,
		done:     false,
	}

	err := watcher.start(ctx, client, config)
	if err != nil {
		_ = logger.Log("msg", "secret watcher failed to start", "err", err, "namespace", config.Namespace, "name", config.Name)
	}
	started := err == nil

	go func() {
		if !started {
			if ok := watcher.tryRestart(ctx, logger, client, config); !ok {
				return
			}
		}
		for {
			select {
			case e, ok := <-watcher.w.ResultChan():
				if ok {
					watcher.update(logger, e)
					continue
				}

				if ok := watcher.tryRestart(ctx, logger, client, config); !ok {
					return
				}
			case <-ctx.Done():
				// The application shutdown, we don't care about cleaning up.
				watcher.close()
				return
			}
		}
	}()

	return watcher, nil
}

func (w *secretWatcher) update(logger log.Logger, e watch.Event) {
	w.mu.Lock()
	defer w.mu.Unlock()

	switch e.Type {
	case watch.Modified, watch.Added:
		secret := e.Object.(*corev1.Secret)
		w.s = secret
	case watch.Deleted:
		w.s = nil
	case watch.Bookmark:
		// Disabled explicitly when creating the watch interface.
	case watch.Error:
		logger.Log("msg", "watch error event", "namespace", w.s.Namespace, "name", w.s.Name)
	}
}

func (w *secretWatcher) secret(config *SecretConfig) secrets.Secret {
	return secrets.SecretFunc(func(_ context.Context) (string, error) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.s == nil {
			return "", errNotFound(config.Namespace, config.Name)
		}
		return getValue(w.s, config.Key)
	})
}

// start creates the secret watch and returns true if the watch is running.
func (w *secretWatcher) start(ctx context.Context, client kubernetes.Interface, config *SecretConfig) error {
	var err error
	w.w, err = client.CoreV1().Secrets(config.Namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector:       fields.OneTermEqualSelector(metav1.ObjectNameField, config.Name).String(),
		AllowWatchBookmarks: false,
	})
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	// We could wait for the first watch event, but it doesn't notify us if the resource doesn't exist.
	w.s, err = client.CoreV1().Secrets(config.Namespace).Get(ctx, config.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
		defer w.w.Stop()
		return fmt.Errorf("fetch: %w", err)
	}
	return nil
}

// tryRestart restarts the secret watch indefinitely until it succeeds or the context is
// cancelled and returns true if the watcher is still running.
func (w *secretWatcher) tryRestart(ctx context.Context, logger log.Logger, client kubernetes.Interface, config *SecretConfig) bool {
	// If the application shutdown, we don't care about cleanup.
	if ctx.Err() != nil {
		w.mu.Lock()
		defer w.mu.Lock()
		w.s = nil
		return false
	}
	// If closed unintentionally (i.e. network issues), try and restart it.
	for {
		ok, err := w.restart(ctx, client, config)
		if !ok {
			return false
		}
		// If an error occurred trying to watch, keep retrying.
		if err == nil {
			break
		}
		_ = logger.Log("msg", "unable to restart secret watcher", "err", err, "namespace", w.s.Namespace, "name", w.s.Name)
	}
	return true
}

// restart attempts to restart the secret watch. Returns true if the watcher is still
// running, or false if the context is cancelled.
func (w *secretWatcher) restart(ctx context.Context, client kubernetes.Interface, config *SecretConfig) (bool, error) {
	// Check in case the channel cancelled intentionally.
	if w.done {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.s = nil
		return false, nil
	}

	jitter()

	// Lock the watcher so it doesn't cancel before we restart.
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check again in case the watcher cancelled while we were waiting for the mutex.
	if w.done {
		w.s = nil
		return false, nil
	}

	if err := w.start(ctx, client, config); err != nil {
		return false, err
	}
	return true, nil
}

func jitter() {
	// Pseudo-arbitrarily jitter the length of the most common scrape interval.
	// In the future, we may want an increasing jitter.
	jitter := time.Second * time.Duration(1+rand.Intn(30))
	time.Sleep(1*time.Second + jitter)
}

func (w *secretWatcher) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.w.Stop()
	w.s = nil
}

type watchProvider struct {
	ctx                context.Context
	client             kubernetes.Interface
	secretKeyToWatcher map[string]*secretWatcher
	logger             log.Logger
}

func newWatchProvider(ctx context.Context, logger log.Logger, client kubernetes.Interface) *watchProvider {
	return &watchProvider{
		ctx:                ctx,
		client:             client,
		secretKeyToWatcher: map[string]*secretWatcher{},
		logger:             logger,
	}
}

// Add adds a new secret to the provider, starting a new watch if the secret is not already watched.
func (p *watchProvider) Add(config *SecretConfig) (secrets.Secret, error) {
	objKey := config.objectKey().String()
	val, ok := p.secretKeyToWatcher[objKey]
	if ok {
		val.refCount++
		return val.secret(config), nil
	}

	var err error
	val, err = newWatcher(p.ctx, p.logger, p.client, config)
	if err != nil {
		return nil, err
	}

	p.secretKeyToWatcher[objKey] = val
	return val.secret(config), nil
}

// Update updates the secret, restarting the watch if the key changes.
func (p *watchProvider) Update(configBefore, configAfter *SecretConfig) (secrets.Secret, error) {
	objKeyBefore := configBefore.objectKey()
	objKeyAfter := configAfter.objectKey()
	if objKeyBefore == objKeyAfter {
		// If we're using the same secret with a different key, just remap your current watch.
		val := p.secretKeyToWatcher[objKeyAfter.String()]
		if val == nil {
			// Highly unlikely occurrence.
			return nil, errNotFound(configAfter.Namespace, configAfter.Name)
		}
		return val.secret(configAfter), nil
	}
	if err := p.Remove(configBefore); err != nil {
		return nil, err
	}
	return p.Add(configAfter)
}

// Remove removes the secret, stopping the watch if no other keys for the same secret are watched.
func (p *watchProvider) Remove(config *SecretConfig) error {
	objKey := config.objectKey().String()
	val := p.secretKeyToWatcher[objKey]
	if val == nil {
		return nil
	}

	val.refCount--
	if val.refCount > 0 {
		return nil
	}
	delete(p.secretKeyToWatcher, objKey)

	val.mu.Lock()
	defer val.mu.Unlock()
	val.done = true
	val.w.Stop()
	return nil
}

func (p *watchProvider) Close(reg prometheus.Registerer) {
	// no-op
}

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
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/prometheus/prometheus/secrets"
)

func toSecretConfig(s *corev1.Secret, key string) *SecretConfig {
	return &SecretConfig{
		Namespace: s.Namespace,
		Name:      s.Name,
		Key:       key,
	}
}

func deleteKey(s *corev1.Secret, key string) {
	delete(s.Data, key)
	delete(s.StringData, key)
}

func updateKey(s *corev1.Secret, key, value string) {
	if _, ok := s.Data[key]; ok {
		s.Data[key] = []byte(value)
		return
	}
	if _, ok := s.StringData[key]; ok {
		s.StringData[key] = value
		return
	}
	panic(fmt.Errorf("invalid key %q in secret: %s/%s", key, s.Namespace, s.Name))
}

func copyKey(s *corev1.Secret, keyFrom, keyTo string) {
	if _, ok := s.Data[keyFrom]; ok {
		s.Data[keyTo] = s.Data[keyFrom]
		return
	}
	if _, ok := s.StringData[keyFrom]; ok {
		s.StringData[keyTo] = s.StringData[keyFrom]
		return
	}
	panic(fmt.Errorf("invalid key %q in secret: %s/%s", keyFrom, s.Namespace, s.Name))
}

type testCase struct {
	description string
	test        func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider)
}

type entry struct {
	key   string
	value string
}

type testCategory struct {
	name    string
	secret  *corev1.Secret
	entries []entry
}

func requireFetchEquals(ctx context.Context, t testing.TB, s secrets.Secret, expected string) {
	t.Helper()
	if s == nil {
		t.Fatal("nil secret")
	}
	var err error
	pollErr := wait.PollUntilContextTimeout(ctx, time.Millisecond, 100000*time.Second, true, func(ctx context.Context) (bool, error) {
		var val string
		val, err = s.Fetch(ctx)
		if err != nil {
			err = fmt.Errorf("expected nil error but received: %w", err)
			//nolint:nilerr // error is used in the outer scope.
			return false, nil
		}
		if expected != val {
			err = fmt.Errorf("expected value %q but received %q", expected, val)
			return false, nil
		}
		return true, nil
	})
	if pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
			pollErr = err
		}
		t.Fatal(pollErr)
	}
}

func requireFetchFail(ctx context.Context, t testing.TB, s secrets.Secret, expected error) {
	t.Helper()
	if s == nil {
		t.Fatal("nil secret")
	}
	var err error
	pollErr := wait.PollUntilContextTimeout(ctx, time.Millisecond, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		var val string
		val, err = s.Fetch(ctx)
		if val != "" {
			err = fmt.Errorf("expected empty value but received %q", val)
			return false, nil
		}
		if expected.Error() != err.Error() {
			err = fmt.Errorf("expected error %q but received %q", expected.Error(), err.Error())
			return false, nil
		}
		return true, nil
	})
	if pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
			pollErr = err
		}
		t.Fatal(pollErr)
	}
}

func TestProvider(t *testing.T) {
	validEmptySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "s1",
		},
	}
	validBinarySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns1",
			Name:      "s2",
		},
		Data: map[string][]byte{
			"k1": []byte("Hello world!"),
			"k2": []byte("xyz"),
			"k3": []byte("abc"),
		},
	}
	validStringSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns2",
			Name:      "s1",
		},
		StringData: map[string]string{
			"foo":   "bar",
			"alpha": "bravo",
		},
	}
	validMixedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns3",
			Name:      "s2",
		},
		Data: map[string][]byte{
			"red": []byte("green"),
		},
		StringData: map[string]string{
			"orange": "blue",
		},
	}

	testCasesFor := func(main testCategory, others ...testCategory) []testCase {
		testCases := []testCase{
			{
				description: fmt.Sprintf("remove untracked %s secret", main.name),
				test: func(_ context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					provider.Remove(toSecretConfig(main.secret, main.entries[0].key))
				},
			},
			{
				description: fmt.Sprintf("remove tracked %s secret", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					provider.Remove(toSecretConfig(main.secret, key))

					// Attempt to remove twice. Second time does nothing.
					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("watch %s secrets.Secret twice", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					value := main.entries[0].value
					s1, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, s1, value)

					s2, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, s2, value)

					provider.Remove(toSecretConfig(main.secret, key))

					// Secrets are still valid since we added them twice.
					requireFetchEquals(ctx, t, s1, value)
					requireFetchEquals(ctx, t, s2, value)

					provider.Remove(toSecretConfig(main.secret, key))

					err = errNotFound(main.secret.Namespace, main.secret.Name)
					requireFetchFail(ctx, t, s1, err)
					requireFetchFail(ctx, t, s2, err)
				},
			},
			{
				description: fmt.Sprintf("valid %s delete key", main.name),
				test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					secret := main.secret.DeepCopy()
					deleteKey(secret, key)
					_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
					require.NoError(t, err)

					requireFetchFail(ctx, t, providerSecret, errKeyNotFound(main.secret.Namespace, main.secret.Name, key))

					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("valid %s delete secret", main.name),
				test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					err = c.CoreV1().Secrets(main.secret.Namespace).Delete(ctx, main.secret.Name, metav1.DeleteOptions{})
					require.NoError(t, err)

					requireFetchFail(ctx, t, providerSecret, errNotFound(main.secret.Namespace, main.secret.Name))

					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("valid %s update value", main.name),
				test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					secret := main.secret.DeepCopy()
					updateKey(secret, key, "Goodbye")
					_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
					require.NoError(t, err)

					requireFetchEquals(ctx, t, providerSecret, "Goodbye")

					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("valid %s update valid key", main.name),
				test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
					keyFrom := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, keyFrom))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					keyTo := main.entries[1].key
					providerSecret, err = provider.Update(toSecretConfig(main.secret, keyFrom), toSecretConfig(main.secret, keyTo))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[1].value)

					// Update original key.
					secret := main.secret.DeepCopy()
					updateKey(secret, keyFrom, "Goodbye")
					_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
					require.NoError(t, err)

					requireFetchEquals(ctx, t, providerSecret, main.entries[1].value)

					// Update new key.
					updateKey(secret, keyTo, "Sayonara")
					_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
					require.NoError(t, err)

					requireFetchEquals(ctx, t, providerSecret, "Sayonara")

					provider.Remove(toSecretConfig(main.secret, keyTo))
				},
			},
			{
				description: fmt.Sprintf("valid %s update invalid key", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					providerSecret, err = provider.Update(toSecretConfig(main.secret, key), toSecretConfig(main.secret, "x"))
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errKeyNotFound(main.secret.Namespace, main.secret.Name, "x"))

					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("valid %s update invalid secret", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					providerSecret, err = provider.Update(toSecretConfig(main.secret, key), &SecretConfig{Namespace: "x", Name: "y", Key: "z"})
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errNotFound("x", "y"))

					provider.Remove(&SecretConfig{Namespace: "x", Name: "y", Key: "z"})
				},
			},
			{
				description: fmt.Sprintf("invalid %s create key", main.name),
				test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
					key := "kn"
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errKeyNotFound(main.secret.Namespace, main.secret.Name, key))

					secret := main.secret.DeepCopy()
					copyKey(secret, main.entries[0].key, key)
					updateKey(secret, key, "Goodbye")
					_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
					require.NoError(t, err)

					requireFetchEquals(ctx, t, providerSecret, "Goodbye")

					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("invalid %s create secret", main.name),
				test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
					err := c.CoreV1().Secrets(main.secret.Namespace).Delete(ctx, main.secret.Name, metav1.DeleteOptions{})
					require.NoError(t, err)

					key := main.entries[0].key
					providerSecret, err := provider.Add(toSecretConfig(main.secret, key))
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errNotFound(main.secret.Namespace, main.secret.Name))

					secret := main.secret.DeepCopy()
					updateKey(secret, key, "Goodbye")
					_, err = c.CoreV1().Secrets(secret.Namespace).Create(ctx, secret, metav1.CreateOptions{})
					require.NoError(t, err)

					requireFetchEquals(ctx, t, providerSecret, "Goodbye")

					provider.Remove(toSecretConfig(main.secret, key))
				},
			},
			{
				description: fmt.Sprintf("invalid %s update valid key", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					keyFrom := "kn1"
					providerSecret, err := provider.Add(toSecretConfig(main.secret, keyFrom))
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errKeyNotFound(main.secret.Namespace, main.secret.Name, keyFrom))

					keyTo := "kn2"
					providerSecret, err = provider.Update(toSecretConfig(main.secret, keyFrom), toSecretConfig(main.secret, keyTo))
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errKeyNotFound(main.secret.Namespace, main.secret.Name, keyTo))

					provider.Remove(toSecretConfig(main.secret, keyTo))
				},
			},
			{
				description: fmt.Sprintf("invalid update valid %s secret", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					providerSecret, err := provider.Add(&SecretConfig{Namespace: "x", Name: "y", Key: "z"})
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errNotFound("x", "y"))

					keyTo := main.entries[0].key
					providerSecret, err = provider.Update(&SecretConfig{Namespace: "x", Name: "y", Key: "z"}, toSecretConfig(main.secret, keyTo))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					provider.Remove(toSecretConfig(main.secret, keyTo))
				},
			},
			{
				description: fmt.Sprintf("invalid %s update invalid key", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					keyFrom := "kn"
					providerSecret, err := provider.Add(toSecretConfig(main.secret, keyFrom))
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errKeyNotFound(main.secret.Namespace, main.secret.Name, keyFrom))

					keyTo := main.entries[0].key
					providerSecret, err = provider.Update(toSecretConfig(main.secret, keyFrom), toSecretConfig(main.secret, keyTo))
					require.NoError(t, err)
					requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

					provider.Remove(toSecretConfig(main.secret, keyTo))
				},
			},
			{
				description: fmt.Sprintf("invalid %s update invalid secret", main.name),
				test: func(ctx context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
					providerSecret, err := provider.Add(&SecretConfig{Namespace: "x", Name: "y", Key: "z"})
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errNotFound("x", "y"))

					providerSecret, err = provider.Update(&SecretConfig{Namespace: "x", Name: "y", Key: "z"}, &SecretConfig{Namespace: "a", Name: "b", Key: "c"})
					require.NoError(t, err)
					requireFetchFail(ctx, t, providerSecret, errNotFound("a", "b"))

					provider.Remove(&SecretConfig{Namespace: "a", Name: "b", Key: "c"})
				},
			},
		}
		for _, other := range others {
			testCases = append(
				testCases,
				testCase{
					description: fmt.Sprintf("valid %s update valid %s secret", main.name, other.name),
					test: func(ctx context.Context, t testing.TB, c *fake.Clientset, provider *watchProvider) {
						keyFrom := main.entries[0].key
						providerSecret, err := provider.Add(toSecretConfig(main.secret, keyFrom))
						require.NoError(t, err)
						requireFetchEquals(ctx, t, providerSecret, main.entries[0].value)

						keyTo := other.entries[0].key
						providerSecret, err = provider.Update(toSecretConfig(main.secret, keyFrom), toSecretConfig(other.secret, keyTo))
						require.NoError(t, err)
						requireFetchEquals(ctx, t, providerSecret, other.entries[0].value)

						// Update original secret.
						secret := main.secret.DeepCopy()
						updateKey(secret, keyFrom, "Goodbye")
						_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
						require.NoError(t, err)

						requireFetchEquals(ctx, t, providerSecret, other.entries[0].value)

						// Update new secret.
						secret = other.secret.DeepCopy()
						updateKey(secret, keyTo, "Sayonara")
						_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
						require.NoError(t, err)

						requireFetchEquals(ctx, t, providerSecret, "Sayonara")

						provider.Remove(toSecretConfig(other.secret, keyTo))
					},
				},
			)
		}
		return testCases
	}
	typeBinary := testCategory{
		name:   "binary",
		secret: validBinarySecret,
		entries: []entry{
			{"k1", "Hello world!"},
			{"k2", "xyz"},
		},
	}
	typeString := testCategory{
		name:   "string",
		secret: validStringSecret,
		entries: []entry{
			{"foo", "bar"},
			{"alpha", "bravo"},
		},
	}
	typeMixedString := testCategory{
		name:   "mixed (string)",
		secret: validMixedSecret,
		entries: []entry{
			{"orange", "blue"},
			{"red", "green"},
		},
	}
	typeMixedBinary := testCategory{
		name:   "mixed (binary)",
		secret: validMixedSecret,
		entries: []entry{
			{"red", "green"},
			{"orange", "blue"},
		},
	}

	testCases := []testCase{
		{
			description: "add secret with no keys",
			test: func(_ context.Context, t testing.TB, _ *fake.Clientset, provider *watchProvider) {
				provider.Remove(toSecretConfig(validEmptySecret, "k1"))
			},
		},
	}
	testCases = append(testCases, testCasesFor(typeBinary, typeString, typeMixedBinary, typeMixedString)...)
	testCases = append(testCases, testCasesFor(typeString, typeBinary, typeMixedBinary, typeMixedString)...)
	testCases = append(testCases, testCasesFor(typeMixedBinary, typeBinary, typeString, typeMixedString)...)
	testCases = append(testCases, testCasesFor(typeMixedString, typeBinary, typeString, typeMixedBinary)...)

	for _, tc := range testCases {
		// Deep copy resources to ensure all tests are independent.
		c := fake.NewSimpleClientset(
			validEmptySecret.DeepCopy(),
			validBinarySecret.DeepCopy(),
			validStringSecret.DeepCopy(),
			validMixedSecret.DeepCopy(),
		)

		t.Run(tc.description, func(t *testing.T) {
			ctx := context.Background()
			provider := newWatchProvider(ctx, log.NewNopLogger(), c)

			tc.test(ctx, t, c, provider)
			require.True(t, provider.isClean())
		})
	}
	t.Run("valid secret context cancelled", func(t *testing.T) {
		c := fake.NewSimpleClientset(
			validEmptySecret.DeepCopy(),
			validBinarySecret.DeepCopy(),
			validStringSecret.DeepCopy(),
			validMixedSecret.DeepCopy(),
		)

		parentCtx := context.Background()
		ctx, cancel := context.WithCancel(parentCtx)
		provider := newWatchProvider(ctx, log.NewNopLogger(), c)

		key := typeBinary.entries[0].key
		providerSecret, err := provider.Add(toSecretConfig(typeBinary.secret, key))
		require.NoError(t, err)
		requireFetchEquals(ctx, t, providerSecret, typeBinary.entries[0].value)

		cancel()
		requireFetchFail(parentCtx, t, providerSecret, errNotFound(typeBinary.secret.Namespace, typeBinary.secret.Name))

		provider.Remove(toSecretConfig(typeBinary.secret, key))

		require.True(t, provider.isClean())
	})
	t.Run("valid secret watch cancelled", func(t *testing.T) {
		c := fake.NewSimpleClientset(
			validEmptySecret.DeepCopy(),
			validBinarySecret.DeepCopy(),
			validStringSecret.DeepCopy(),
			validMixedSecret.DeepCopy(),
		)

		// Use a proxy to capture the watch object that the secret provider uses.
		proxyClient := fake.NewSimpleClientset()
		proxyClient.PrependReactor("get", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
			obj, err := c.Invokes(action, &corev1.Secret{})
			return true, obj, err
		})
		var w watch.Interface
		waitMutex := sync.Mutex{}
		proxyClient.PrependWatchReactor("secrets", func(action clienttesting.Action) (bool, watch.Interface, error) {
			waitMutex.Lock()
			defer waitMutex.Unlock()
			var err error
			w, err = c.InvokesWatch(action)
			return true, w, err
		})

		ctx := context.Background()
		provider := newWatchProvider(ctx, log.NewNopLogger(), proxyClient)

		key := typeBinary.entries[0].key
		providerSecret, err := provider.Add(toSecretConfig(typeBinary.secret, key))
		require.NoError(t, err)
		requireFetchEquals(ctx, t, providerSecret, typeBinary.entries[0].value)

		// Don't let the watcher reload until we update the secret.
		waitMutex.Lock()
		w.Stop()

		secret := typeBinary.secret.DeepCopy()
		updateKey(secret, key, "Goodbye")
		_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)

		// Now that we've updated the secret, let the watcher reload.
		waitMutex.Unlock()
		requireFetchEquals(ctx, t, providerSecret, "Goodbye")

		// Ensure the new watcher is working.
		updateKey(secret, key, "Hello")
		_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)

		requireFetchEquals(ctx, t, providerSecret, "Hello")

		provider.Remove(toSecretConfig(typeBinary.secret, key))

		require.True(t, provider.isClean())
	})
	t.Run("network error", func(t *testing.T) {
		c := fake.NewSimpleClientset(
			validEmptySecret.DeepCopy(),
			validBinarySecret.DeepCopy(),
			validStringSecret.DeepCopy(),
			validMixedSecret.DeepCopy(),
		)

		networkError := true

		c.PrependReactor("get", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
			if networkError {
				return true, nil, apierrors.NewInternalError(errors.New("network"))
			}
			return false, nil, nil
		})

		ctx := context.Background()
		provider := newWatchProvider(ctx, log.NewNopLogger(), c)

		key := typeBinary.entries[0].key
		providerSecret, err := provider.Add(toSecretConfig(typeBinary.secret, key))
		require.NoError(t, err)
		err = errNotFound(typeBinary.secret.Namespace, typeBinary.secret.Name)
		requireFetchFail(ctx, t, providerSecret, err)

		secret := typeBinary.secret.DeepCopy()
		updateKey(secret, key, "Goodbye")
		_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)

		networkError = false
		requireFetchEquals(ctx, t, providerSecret, "Goodbye")

		// Ensure the new watcher is working.
		updateKey(secret, key, "Hello")
		_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)

		requireFetchEquals(ctx, t, providerSecret, "Hello")

		provider.Remove(toSecretConfig(typeBinary.secret, key))

		require.True(t, provider.isClean())
	})
	t.Run("permission denied", func(t *testing.T) {
		c := fake.NewSimpleClientset(
			validEmptySecret.DeepCopy(),
			validBinarySecret.DeepCopy(),
			validStringSecret.DeepCopy(),
			validMixedSecret.DeepCopy(),
		)

		permissionDenied := true

		// Use a proxy to ensure we can route results.
		proxyClient := fake.NewSimpleClientset()
		proxyClient.PrependReactor("get", "secrets", func(action clienttesting.Action) (bool, runtime.Object, error) {
			obj, err := c.Invokes(action, &corev1.Secret{})
			if permissionDenied {
				o := obj.(*corev1.Secret)
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, o.GetName(), errors.New("forbidden"))
			}
			return true, obj, err
		})
		proxyClient.PrependWatchReactor("secrets", func(action clienttesting.Action) (bool, watch.Interface, error) {
			w, err := c.InvokesWatch(action)
			if permissionDenied {
				defer w.Stop()
				obj, err := c.Invokes(action, &corev1.Secret{})
				if err != nil {
					return true, nil, err
				}
				o := obj.(*corev1.Secret)
				return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, o.GetName(), errors.New("forbidden"))
			}
			return true, w, err
		})

		ctx := context.Background()
		provider := newWatchProvider(ctx, log.NewNopLogger(), proxyClient)

		key := typeBinary.entries[0].key
		providerSecret, err := provider.Add(toSecretConfig(typeBinary.secret, key))
		require.NoError(t, err)
		err = errNotFound(typeBinary.secret.Namespace, typeBinary.secret.Name)
		requireFetchFail(ctx, t, providerSecret, err)

		secret := typeBinary.secret.DeepCopy()
		updateKey(secret, key, "Goodbye")
		_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)

		// Now that we've updated the secret, give permission to view the secret.
		permissionDenied = false
		requireFetchEquals(ctx, t, providerSecret, "Goodbye")

		// Ensure the new watcher is working.
		updateKey(secret, key, "Hello")
		_, err = c.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		require.NoError(t, err)

		requireFetchEquals(ctx, t, providerSecret, "Hello")

		provider.Remove(toSecretConfig(typeBinary.secret, key))

		require.True(t, provider.isClean())
	})
}

func (p *watchProvider) isClean() bool {
	return len(p.secretKeyToWatcher) == 0
}

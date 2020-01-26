package api

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPI_ConfigEntries(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	config_entries := c.ConfigEntries()

	t.Run("Proxy Defaults", func(t *testing.T) {
		global_proxy := &ProxyConfigEntry{
			Kind: ProxyDefaults,
			Name: ProxyConfigGlobal,
			Config: map[string]interface{}{
				"foo": "bar",
				"bar": 1.0,
			},
		}

		// set it
		_, wm, err := config_entries.Set(global_proxy, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		// get it
		entry, qm, err := config_entries.Get(ProxyDefaults, ProxyConfigGlobal, nil)
		require.NoError(t, err)
		require.NotNil(t, qm)
		require.NotEqual(t, 0, qm.RequestTime)

		// verify it
		readProxy, ok := entry.(*ProxyConfigEntry)
		require.True(t, ok)
		require.Equal(t, global_proxy.Kind, readProxy.Kind)
		require.Equal(t, global_proxy.Name, readProxy.Name)
		require.Equal(t, global_proxy.Config, readProxy.Config)

		global_proxy.Config["baz"] = true
		// CAS update fail
		written, _, err := config_entries.CAS(global_proxy, 0, nil)
		require.NoError(t, err)
		require.False(t, written)

		// CAS update success
		written, wm, err = config_entries.CAS(global_proxy, readProxy.ModifyIndex, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)
		require.NoError(t, err)
		require.True(t, written)

		// Non CAS update
		global_proxy.Config["baz"] = "baz"
		_, wm, err = config_entries.Set(global_proxy, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		// list it
		entries, qm, err := config_entries.List(ProxyDefaults, nil)
		require.NoError(t, err)
		require.NotNil(t, qm)
		require.NotEqual(t, 0, qm.RequestTime)
		require.Len(t, entries, 1)
		readProxy, ok = entries[0].(*ProxyConfigEntry)
		require.True(t, ok)
		require.Equal(t, global_proxy.Kind, readProxy.Kind)
		require.Equal(t, global_proxy.Name, readProxy.Name)
		require.Equal(t, global_proxy.Config, readProxy.Config)

		// delete it
		wm, err = config_entries.Delete(ProxyDefaults, ProxyConfigGlobal, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		entry, qm, err = config_entries.Get(ProxyDefaults, ProxyConfigGlobal, nil)
		require.Error(t, err)
	})

	t.Run("Service Defaults", func(t *testing.T) {
		service := &ServiceConfigEntry{
			Kind:     ServiceDefaults,
			Name:     "foo",
			Protocol: "udp",
		}

		service2 := &ServiceConfigEntry{
			Kind:     ServiceDefaults,
			Name:     "bar",
			Protocol: "tcp",
		}

		// set it
		_, wm, err := config_entries.Set(service, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		// also set the second one
		_, wm, err = config_entries.Set(service2, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		// get it
		entry, qm, err := config_entries.Get(ServiceDefaults, "foo", nil)
		require.NoError(t, err)
		require.NotNil(t, qm)
		require.NotEqual(t, 0, qm.RequestTime)

		// verify it
		readService, ok := entry.(*ServiceConfigEntry)
		require.True(t, ok)
		require.Equal(t, service.Kind, readService.Kind)
		require.Equal(t, service.Name, readService.Name)
		require.Equal(t, service.Protocol, readService.Protocol)

		// update it
		service.Protocol = "tcp"

		// CAS fail
		written, _, err := config_entries.CAS(service, 0, nil)
		require.NoError(t, err)
		require.False(t, written)

		// CAS success
		written, wm, err = config_entries.CAS(service, readService.ModifyIndex, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)
		require.True(t, written)

		// update no cas
		service.Protocol = "http"

		_, wm, err = config_entries.Set(service, nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		// list them
		entries, qm, err := config_entries.List(ServiceDefaults, nil)
		require.NoError(t, err)
		require.NotNil(t, qm)
		require.NotEqual(t, 0, qm.RequestTime)
		require.Len(t, entries, 2)

		for _, entry = range entries {
			switch entry.GetName() {
			case "foo":
				// this also verifies that the update value was persisted and
				// the updated values are seen
				readService, ok = entry.(*ServiceConfigEntry)
				require.True(t, ok)
				require.Equal(t, service.Kind, readService.Kind)
				require.Equal(t, service.Name, readService.Name)
				require.Equal(t, service.Protocol, readService.Protocol)
			case "bar":
				readService, ok = entry.(*ServiceConfigEntry)
				require.True(t, ok)
				require.Equal(t, service2.Kind, readService.Kind)
				require.Equal(t, service2.Name, readService.Name)
				require.Equal(t, service2.Protocol, readService.Protocol)
			}

		}

		// delete it
		wm, err = config_entries.Delete(ServiceDefaults, "foo", nil)
		require.NoError(t, err)
		require.NotNil(t, wm)
		require.NotEqual(t, 0, wm.RequestTime)

		// verify deletion
		entry, qm, err = config_entries.Get(ServiceDefaults, "foo", nil)
		require.Error(t, err)
	})
}

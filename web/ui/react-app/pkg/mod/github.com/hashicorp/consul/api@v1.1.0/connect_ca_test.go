package api

import (
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	"github.com/stretchr/testify/require"
)

func TestAPI_ConnectCARoots_empty(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		// Don't bootstrap CA
		c.Connect = nil
	})
	defer s.Stop()

	s.WaitForSerfCheck(t)

	connect := c.Connect()
	_, _, err := connect.CARoots(nil)

	require.Error(err)
	require.Contains(err.Error(), "Connect must be enabled")
}

func TestAPI_ConnectCARoots_list(t *testing.T) {
	t.Parallel()

	c, s := makeClient(t)
	defer s.Stop()

	// This fails occasionally if server doesn't have time to bootstrap CA so
	// retry
	retry.Run(t, func(r *retry.R) {
		connect := c.Connect()
		list, meta, err := connect.CARoots(nil)
		r.Check(err)
		if meta.LastIndex <= 0 {
			r.Fatalf("expected roots raft index to be > 0")
		}
		if v := len(list.Roots); v != 1 {
			r.Fatalf("expected 1 root, got %d", v)
		}
		// connect.TestClusterID causes import cycle so hard code it
		if list.TrustDomain != "11111111-2222-3333-4444-555555555555.consul" {
			r.Fatalf("expected fixed trust domain got '%s'", list.TrustDomain)
		}
	})

}

func TestAPI_ConnectCAConfig_get_set(t *testing.T) {
	t.Parallel()

	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)
	expected := &ConsulCAProviderConfig{
		RotationPeriod: 90 * 24 * time.Hour,
	}
	expected.LeafCertTTL = 72 * time.Hour

	// This fails occasionally if server doesn't have time to bootstrap CA so
	// retry
	retry.Run(t, func(r *retry.R) {
		connect := c.Connect()

		conf, _, err := connect.CAGetConfig(nil)
		r.Check(err)
		if conf.Provider != "consul" {
			r.Fatalf("expected default provider, got %q", conf.Provider)
		}
		parsed, err := ParseConsulCAConfig(conf.Config)
		r.Check(err)
		require.Equal(r, expected, parsed)

		// Change a config value and update
		conf.Config["PrivateKey"] = ""
		conf.Config["RotationPeriod"] = 120 * 24 * time.Hour
		_, err = connect.CASetConfig(conf, nil)
		r.Check(err)

		updated, _, err := connect.CAGetConfig(nil)
		r.Check(err)
		expected.RotationPeriod = 120 * 24 * time.Hour
		parsed, err = ParseConsulCAConfig(updated.Config)
		r.Check(err)
		require.Equal(r, expected, parsed)
	})
}

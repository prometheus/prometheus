package api

import (
	"testing"

	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/consul/sdk/testutil/retry"
)

func TestAPI_OperatorAutopilotGetSetConfiguration(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()
	s.WaitForSerfCheck(t)

	operator := c.Operator()
	config, err := operator.AutopilotGetConfiguration(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !config.CleanupDeadServers {
		t.Fatalf("bad: %v", config)
	}

	// Change a config setting
	newConf := &AutopilotConfiguration{CleanupDeadServers: false}
	if err := operator.AutopilotSetConfiguration(newConf, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	config, err = operator.AutopilotGetConfiguration(nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if config.CleanupDeadServers {
		t.Fatalf("bad: %v", config)
	}
}

func TestAPI_OperatorAutopilotCASConfiguration(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	retry.Run(t, func(r *retry.R) {
		operator := c.Operator()
		config, err := operator.AutopilotGetConfiguration(nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !config.CleanupDeadServers {
			t.Fatalf("bad: %v", config)
		}

		// Pass an invalid ModifyIndex
		{
			newConf := &AutopilotConfiguration{
				CleanupDeadServers: false,
				ModifyIndex:        config.ModifyIndex - 1,
			}
			resp, err := operator.AutopilotCASConfiguration(newConf, nil)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if resp {
				t.Fatalf("bad: %v", resp)
			}
		}

		// Pass a valid ModifyIndex
		{
			newConf := &AutopilotConfiguration{
				CleanupDeadServers: false,
				ModifyIndex:        config.ModifyIndex,
			}
			resp, err := operator.AutopilotCASConfiguration(newConf, nil)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if !resp {
				t.Fatalf("bad: %v", resp)
			}
		}
	})
}

func TestAPI_OperatorAutopilotServerHealth(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		c.RaftProtocol = 3
	})
	defer s.Stop()

	operator := c.Operator()
	retry.Run(t, func(r *retry.R) {
		out, err := operator.AutopilotServerHealth(nil)
		if err != nil {
			r.Fatalf("err: %v", err)
		}

		if len(out.Servers) != 1 ||
			!out.Servers[0].Healthy ||
			out.Servers[0].Name != s.Config.NodeName {
			r.Fatalf("bad: %v", out)
		}
	})
}

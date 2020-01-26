package command

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/serf/cmd/serf/command/agent"
	"github.com/hashicorp/serf/serf"
	"github.com/hashicorp/serf/testutil"
)

func init() {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())
}

func testAgent(t *testing.T) *agent.Agent {
	agentConfig := agent.DefaultConfig()
	serfConfig := serf.DefaultConfig()
	return testAgentWithConfig(t, agentConfig, serfConfig)
}

func testAgentWithConfig(t *testing.T, agentConfig *agent.Config,
	serfConfig *serf.Config) *agent.Agent {

	serfConfig.MemberlistConfig.BindAddr = testutil.GetBindAddr().String()
	serfConfig.MemberlistConfig.ProbeInterval = 50 * time.Millisecond
	serfConfig.MemberlistConfig.ProbeTimeout = 25 * time.Millisecond
	serfConfig.MemberlistConfig.SuspicionMult = 1
	serfConfig.NodeName = serfConfig.MemberlistConfig.BindAddr
	serfConfig.Tags = map[string]string{"role": "test", "tag1": "foo", "tag2": "bar"}

	agent, err := agent.Create(agentConfig, serfConfig, nil)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if err := agent.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	return agent
}

func getRPCAddr() string {
	for i := 0; i < 500; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", rand.Int31n(25000)+1024))
		if err == nil {
			l.Close()
			return l.Addr().String()
		}
	}

	panic("no listener")
}

func testIPC(t *testing.T, a *agent.Agent) (string, *agent.AgentIPC) {
	rpcAddr := getRPCAddr()

	l, err := net.Listen("tcp", rpcAddr)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	lw := agent.NewLogWriter(512)
	mult := io.MultiWriter(os.Stderr, lw)
	ipc := agent.NewAgentIPC(a, "", l, mult, lw)
	return rpcAddr, ipc
}

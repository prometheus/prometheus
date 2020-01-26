package agent

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"time"

	"github.com/hashicorp/serf/serf"
	"github.com/hashicorp/serf/testutil"
)

func init() {
	// Seed the random number generator
	rand.Seed(time.Now().UnixNano())
}

func drainEventCh(ch <-chan string) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
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

func testAgent(logOutput io.Writer) *Agent {
	return testAgentWithConfig(DefaultConfig(), serf.DefaultConfig(), logOutput)
}

func testAgentWithConfig(agentConfig *Config, serfConfig *serf.Config,
	logOutput io.Writer) *Agent {

	if logOutput == nil {
		logOutput = os.Stderr
	}
	serfConfig.MemberlistConfig.ProbeInterval = 100 * time.Millisecond
	serfConfig.MemberlistConfig.BindAddr = testutil.GetBindAddr().String()
	serfConfig.NodeName = serfConfig.MemberlistConfig.BindAddr

	agent, err := Create(agentConfig, serfConfig, logOutput)
	if err != nil {
		panic(err)
	}
	return agent
}

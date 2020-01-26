package command

import (
	"flag"
	"os"

	"github.com/hashicorp/serf/client"
)

// RPCAddrFlag returns a pointer to a string that will be populated
// when the given flagset is parsed with the RPC address of the Serf.
func RPCAddrFlag(f *flag.FlagSet) *string {
	defaultRpcAddr := os.Getenv("SERF_RPC_ADDR")
	if defaultRpcAddr == "" {
		defaultRpcAddr = "127.0.0.1:7373"
	}

	return f.String("rpc-addr", defaultRpcAddr,
		"RPC address of the Serf agent")
}

// RPCAuthFlag returns a pointer to a string that will be populated
// when the given flagset is parsed with the RPC auth token of the Serf.
func RPCAuthFlag(f *flag.FlagSet) *string {
	rpcAuth := os.Getenv("SERF_RPC_AUTH")
	return f.String("rpc-auth", rpcAuth,
		"RPC auth token of the Serf agent")
}

// RPCClient returns a new Serf RPC client with the given address.
func RPCClient(addr, auth string) (*client.RPCClient, error) {
	config := client.Config{Addr: addr, AuthKey: auth}
	return client.ClientFromConfig(&config)
}

// Responsible for managing the ingester lifecycle.

package frankenstein

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"os"

	"github.com/prometheus/common/log"
)

const (
	infName = "eth0"
)

// WriteIngesterConfigToConsul writes ingester config to Consul
func WriteIngesterConfigToConsul(consulClient ConsulClient, listenPort int, numTokens int) error {
	log.Info("Adding ingester to consul")

	desc, err := describeLocalIngester(listenPort, numTokens)
	if err != nil {
		return err
	}
	return writeIngesterConfigToConsul(consulClient, desc)
}

func writeIngesterConfigToConsul(consulClient ConsulClient, desc *IngesterDesc) error {
	buf, err := json.Marshal(desc)
	if err != nil {
		return err
	}
	return consulClient.PutBytes(desc.ID, buf)
}

// describeLocalIngester returns an IngesterDesc for the ingester that is this
// process.
func describeLocalIngester(listenPort, numTokens int) (*IngesterDesc, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	addr, err := getFirstAddressOf(infName)
	if err != nil {
		return nil, err
	}

	return &IngesterDesc{
		ID:       hostname,
		Hostname: fmt.Sprintf("%s:%d", addr, listenPort),
		Tokens:   generateTokens(hostname, numTokens),
	}, nil
}

func generateTokens(id string, numTokens int) []uint32 {
	tokenHasher := fnv.New64()
	tokenHasher.Write([]byte(id))
	r := rand.New(rand.NewSource(int64(tokenHasher.Sum64())))

	tokens := []uint32{}
	for i := 0; i < numTokens; i++ {
		tokens = append(tokens, r.Uint32())
	}
	return tokens
}

// DeleteIngesterConfigFromConsul deletes ingestor config from Consul
func DeleteIngesterConfigFromConsul(consulClient ConsulClient) error {
	log.Info("Removing ingester from consul")
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	return deleteIngesterConfigFromConsul(consulClient, hostname)
}

func deleteIngesterConfigFromConsul(consulClient ConsulClient, id string) error {
	buf, err := json.Marshal(IngesterDesc{
		ID:       id,
		Hostname: "",
		Tokens:   []uint32{},
	})
	if err != nil {
		return err
	}
	return consulClient.PutBytes(id, buf)
}

// getFirstAddressOf returns the first IPv4 address of the supplied interface name.
func getFirstAddressOf(name string) (string, error) {
	inf, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}

	addrs, err := inf.Addrs()
	if err != nil {
		return "", err
	}
	if len(addrs) <= 0 {
		return "", fmt.Errorf("No address found for %s", name)
	}

	for _, addr := range addrs {
		switch v := addr.(type) {
		case *net.IPNet:
			if ip := v.IP.To4(); ip != nil {
				return v.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("No address found for %s", name)
}

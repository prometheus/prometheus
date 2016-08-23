// Responsible for managing the ingester lifecycle.

package frankenstein

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math/rand"
	"net"
	"os"

	consul "github.com/hashicorp/consul/api"
	"github.com/prometheus/common/log"
)

const (
	infName = "eth0"
)

// WriteIngesterConfigToConsul writes ingester config to Consul
func WriteIngesterConfigToConsul(consulClient ConsulClient, consulPrefix string, listenPort int, numTokens int) error {
	log.Info("Adding ingester to consul")

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	addr, err := getFirstAddressOf(infName)
	if err != nil {
		return err
	}

	buf, err := json.Marshal(IngesterDesc{
		ID:       hostname,
		Hostname: fmt.Sprintf("%s:%d", addr, listenPort),
		Tokens:   generateTokens(hostname, numTokens),
	})
	if err != nil {
		return err
	}

	_, err = consulClient.Put(&consul.KVPair{
		Key:   consulPrefix + hostname,
		Value: buf,
	}, &consul.WriteOptions{})
	return err
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
func DeleteIngesterConfigFromConsul(consulClient ConsulClient, consulPrefix string) error {
	log.Info("Removing ingester from consul")

	hostname, err := os.Hostname()
	if err != nil {
		return err
	}

	buf, err := json.Marshal(IngesterDesc{
		ID:       hostname,
		Hostname: "",
		Tokens:   []uint32{},
	})
	if err != nil {
		return err
	}

	_, err = consulClient.Put(&consul.KVPair{
		Key:   consulPrefix + hostname,
		Value: buf,
	}, &consul.WriteOptions{})
	return err
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

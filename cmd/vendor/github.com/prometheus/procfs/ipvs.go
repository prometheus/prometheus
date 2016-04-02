package procfs

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
)

// IPVSStats holds IPVS statistics, as exposed by the kernel in `/proc/net/ip_vs_stats`.
type IPVSStats struct {
	// Total count of connections.
	Connections uint64
	// Total incoming packages processed.
	IncomingPackets uint64
	// Total outgoing packages processed.
	OutgoingPackets uint64
	// Total incoming traffic.
	IncomingBytes uint64
	// Total outgoing traffic.
	OutgoingBytes uint64
}

// IPVSBackendStatus holds current metrics of one virtual / real address pair.
type IPVSBackendStatus struct {
	// The local (virtual) IP address.
	LocalAddress net.IP
	// The local (virtual) port.
	LocalPort uint16
	// The transport protocol (TCP, UDP).
	Proto string
	// The remote (real) IP address.
	RemoteAddress net.IP
	// The remote (real) port.
	RemotePort uint16
	// The current number of active connections for this virtual/real address pair.
	ActiveConn uint64
	// The current number of inactive connections for this virtual/real address pair.
	InactConn uint64
	// The current weight of this virtual/real address pair.
	Weight uint64
}

// NewIPVSStats reads the IPVS statistics.
func NewIPVSStats() (IPVSStats, error) {
	fs, err := NewFS(DefaultMountPoint)
	if err != nil {
		return IPVSStats{}, err
	}

	return fs.NewIPVSStats()
}

// NewIPVSStats reads the IPVS statistics from the specified `proc` filesystem.
func (fs FS) NewIPVSStats() (IPVSStats, error) {
	file, err := fs.open("net/ip_vs_stats")
	if err != nil {
		return IPVSStats{}, err
	}
	defer file.Close()

	return parseIPVSStats(file)
}

// parseIPVSStats performs the actual parsing of `ip_vs_stats`.
func parseIPVSStats(file io.Reader) (IPVSStats, error) {
	var (
		statContent []byte
		statLines   []string
		statFields  []string
		stats       IPVSStats
	)

	statContent, err := ioutil.ReadAll(file)
	if err != nil {
		return IPVSStats{}, err
	}

	statLines = strings.SplitN(string(statContent), "\n", 4)
	if len(statLines) != 4 {
		return IPVSStats{}, errors.New("ip_vs_stats corrupt: too short")
	}

	statFields = strings.Fields(statLines[2])
	if len(statFields) != 5 {
		return IPVSStats{}, errors.New("ip_vs_stats corrupt: unexpected number of fields")
	}

	stats.Connections, err = strconv.ParseUint(statFields[0], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.IncomingPackets, err = strconv.ParseUint(statFields[1], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.OutgoingPackets, err = strconv.ParseUint(statFields[2], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.IncomingBytes, err = strconv.ParseUint(statFields[3], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}
	stats.OutgoingBytes, err = strconv.ParseUint(statFields[4], 16, 64)
	if err != nil {
		return IPVSStats{}, err
	}

	return stats, nil
}

// NewIPVSBackendStatus reads and returns the status of all (virtual,real) server pairs.
func NewIPVSBackendStatus() ([]IPVSBackendStatus, error) {
	fs, err := NewFS(DefaultMountPoint)
	if err != nil {
		return []IPVSBackendStatus{}, err
	}

	return fs.NewIPVSBackendStatus()
}

// NewIPVSBackendStatus reads and returns the status of all (virtual,real) server pairs from the specified `proc` filesystem.
func (fs FS) NewIPVSBackendStatus() ([]IPVSBackendStatus, error) {
	file, err := fs.open("net/ip_vs")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseIPVSBackendStatus(file)
}

func parseIPVSBackendStatus(file io.Reader) ([]IPVSBackendStatus, error) {
	var (
		status       []IPVSBackendStatus
		scanner      = bufio.NewScanner(file)
		proto        string
		localAddress net.IP
		localPort    uint16
		err          error
	)

	for scanner.Scan() {
		fields := strings.Fields(string(scanner.Text()))
		if len(fields) == 0 {
			continue
		}
		switch {
		case fields[0] == "IP" || fields[0] == "Prot" || fields[1] == "RemoteAddress:Port":
			continue
		case fields[0] == "TCP" || fields[0] == "UDP":
			if len(fields) < 2 {
				continue
			}
			proto = fields[0]
			localAddress, localPort, err = parseIPPort(fields[1])
			if err != nil {
				return nil, err
			}
		case fields[0] == "->":
			if len(fields) < 6 {
				continue
			}
			remoteAddress, remotePort, err := parseIPPort(fields[1])
			if err != nil {
				return nil, err
			}
			weight, err := strconv.ParseUint(fields[3], 10, 64)
			if err != nil {
				return nil, err
			}
			activeConn, err := strconv.ParseUint(fields[4], 10, 64)
			if err != nil {
				return nil, err
			}
			inactConn, err := strconv.ParseUint(fields[5], 10, 64)
			if err != nil {
				return nil, err
			}
			status = append(status, IPVSBackendStatus{
				LocalAddress:  localAddress,
				LocalPort:     localPort,
				RemoteAddress: remoteAddress,
				RemotePort:    remotePort,
				Proto:         proto,
				Weight:        weight,
				ActiveConn:    activeConn,
				InactConn:     inactConn,
			})
		}
	}
	return status, nil
}

func parseIPPort(s string) (net.IP, uint16, error) {
	tmp := strings.SplitN(s, ":", 2)

	if len(tmp) != 2 {
		return nil, 0, fmt.Errorf("invalid IP:Port: %s", s)
	}

	if len(tmp[0]) != 8 && len(tmp[0]) != 32 {
		return nil, 0, fmt.Errorf("invalid IP: %s", tmp[0])
	}

	ip, err := hex.DecodeString(tmp[0])
	if err != nil {
		return nil, 0, err
	}

	port, err := strconv.ParseUint(tmp[1], 16, 16)
	if err != nil {
		return nil, 0, err
	}

	return ip, uint16(port), nil
}

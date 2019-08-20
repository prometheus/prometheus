package metadata

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func init() {
	fmt.Println("make sure your ecs is in vpc before you run ```go test```")
}

func send(resource string) (string, error) {

	if strings.Contains(resource, HOSTNAME) {
		return "hostname-test", nil
	}

	if strings.Contains(resource, DNS_NAMESERVERS) {
		return "8.8.8.8\n8.8.4.4", nil
	}

	if strings.Contains(resource, EIPV4) {
		return "1.1.1.1-test", nil
	}

	if strings.Contains(resource, IMAGE_ID) {
		return "image-id-test", nil
	}

	if strings.Contains(resource, INSTANCE_ID) {
		return "instanceid-test", nil
	}

	if strings.Contains(resource, MAC) {
		return "mac-test", nil
	}

	if strings.Contains(resource, NETWORK_TYPE) {
		return "network-type-test", nil
	}

	if strings.Contains(resource, OWNER_ACCOUNT_ID) {
		return "owner-account-id-test", nil
	}

	if strings.Contains(resource, PRIVATE_IPV4) {
		return "private-ipv4-test", nil
	}

	if strings.Contains(resource, REGION) {
		return "region-test", nil
	}

	if strings.Contains(resource, SERIAL_NUMBER) {
		return "serial-number-test", nil
	}

	if strings.Contains(resource, SOURCE_ADDRESS) {
		return "source-address-test", nil
	}

	if strings.Contains(resource, VPC_CIDR_BLOCK) {
		return "vpc-cidr-block-test", nil
	}

	if strings.Contains(resource, VPC_ID) {
		return "vpc-id-test", nil
	}

	if strings.Contains(resource, VSWITCH_CIDR_BLOCK) {
		return "vswitch-cidr-block-test", nil
	}

	if strings.Contains(resource, VSWITCH_ID) {
		return "vswitch-id-test", nil
	}

	if strings.Contains(resource, NTP_CONF_SERVERS) {
		return "ntp1.server.com\nntp2.server.com", nil
	}

	if strings.Contains(resource, ZONE) {
		return "zone-test", nil
	}
	if strings.Contains(resource, RAM_SECURITY) {
		role := `
		{
  "AccessKeyId" : "",
  "AccessKeySecret" : "",
  "Expiration" : "2017-12-05T13:30:01Z",
  "SecurityToken" : "",
  "LastUpdated" : "2017-12-05T07:30:01Z",
  "Code" : "Success"
}
		`
		return role, nil
	}
	return "", errors.New("unknow resource error.")
}

func TestOK(t *testing.T) {
	fmt.Println("ok")
}

func TestHostname(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.HostName()
	if err != nil {
		t.Errorf("hostname err: %s", err.Error())
	}
	if host != "hostname-test" {
		t.Error("hostname not equal hostname-test")
	}
}

func TestEIPV4(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.EIPv4()
	if err != nil {
		t.Errorf("EIPV4 err: %s", err.Error())
	}
	if host != "1.1.1.1-test" {
		t.Error("EIPV4 not equal eipv4-test")
	}
}
func TestImageID(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.ImageID()
	if err != nil {
		t.Errorf("IMAGE_ID err: %s", err.Error())
	}
	if host != "image-id-test" {
		t.Error("IMAGE_ID not equal image-id-test")
	}
}
func TestInstanceID(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.InstanceID()
	if err != nil {
		t.Errorf("IMAGE_ID err: %s", err.Error())
	}
	if host != "instanceid-test" {
		t.Error("IMAGE_ID not equal instanceid-test")
	}
}
func TestMac(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.Mac()
	if err != nil {
		t.Errorf("Mac err: %s", err.Error())
	}
	if host != "mac-test" {
		t.Error("Mac not equal mac-test")
	}
}
func TestNetworkType(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.NetworkType()
	if err != nil {
		t.Errorf("NetworkType err: %s", err.Error())
	}
	if host != "network-type-test" {
		t.Error("networktype not equal network-type-test")
	}
}
func TestOwnerAccountID(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.OwnerAccountID()
	if err != nil {
		t.Errorf("owneraccountid err: %s", err.Error())
	}
	if host != "owner-account-id-test" {
		t.Error("owner-account-id not equal owner-account-id-test")
	}
}
func TestPrivateIPv4(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.PrivateIPv4()
	if err != nil {
		t.Errorf("privateIPv4 err: %s", err.Error())
	}
	if host != "private-ipv4-test" {
		t.Error("privateIPv4 not equal private-ipv4-test")
	}
}
func TestRegion(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.Region()
	if err != nil {
		t.Errorf("region err: %s", err.Error())
	}
	if host != "region-test" {
		t.Error("region not equal region-test")
	}
}
func TestSerialNumber(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.SerialNumber()
	if err != nil {
		t.Errorf("serial number err: %s", err.Error())
	}
	if host != "serial-number-test" {
		t.Error("serial number not equal serial-number-test")
	}
}

func TestSourceAddress(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.SourceAddress()
	if err != nil {
		t.Errorf("source address err: %s", err.Error())
	}
	if host != "source-address-test" {
		t.Error("source address not equal source-address-test")
	}
}
func TestVpcCIDRBlock(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.VpcCIDRBlock()
	if err != nil {
		t.Errorf("vpcCIDRBlock err: %s", err.Error())
	}
	if host != "vpc-cidr-block-test" {
		t.Error("vpc-cidr-block not equal vpc-cidr-block-test")
	}
}
func TestVpcID(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.VpcID()
	if err != nil {
		t.Errorf("vpcID err: %s", err.Error())
	}
	if host != "vpc-id-test" {
		t.Error("vpc-id not equal vpc-id-test")
	}
}
func TestVswitchCIDRBlock(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.VswitchCIDRBlock()
	if err != nil {
		t.Errorf("vswitchCIDRBlock err: %s", err.Error())
	}
	if host != "vswitch-cidr-block-test" {
		t.Error("vswitch-cidr-block not equal vswitch-cidr-block-test")
	}
}
func TestVswitchID(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.VswitchID()
	if err != nil {
		t.Errorf("vswitch id err: %s", err.Error())
	}
	if host != "vswitch-id-test" {
		t.Error("vswitch-id not equal vswitch-id-test")
	}
}

func TestRamToken(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	ram, err := meta.RamRoleToken("list")
	if err != nil {
		t.Errorf("ram role token: error, %s", err.Error())
	}
	if ram.AccessKeySecret == "" {
		t.Error("get ram role token failed")
	}
}

func TestNTPConfigServers(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.NTPConfigServers()
	if err != nil {
		t.Errorf("ntpconfigservers err: %s", err.Error())
	}
	if host[0] != "ntp1.server.com" || host[1] != "ntp2.server.com" {
		t.Error("ntp1.server.com not equal ntp1.server.com")
	}
}
func TestDNSServers(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.DNSNameServers()
	if err != nil {
		t.Errorf("dnsservers err: %s", err.Error())
	}
	if host[0] != "8.8.8.8" || host[1] != "8.8.4.4" {
		t.Error("dns servers not equal 8.8.8.8/8.8.4.4")
	}
}

func TestZone(t *testing.T) {
	meta := NewMockMetaData(nil, send)
	host, err := meta.Zone()
	if err != nil {
		t.Errorf("zone err: %s", err.Error())
	}
	if host != "zone-test" {
		t.Error("zone not equal zone-test")
	}
}

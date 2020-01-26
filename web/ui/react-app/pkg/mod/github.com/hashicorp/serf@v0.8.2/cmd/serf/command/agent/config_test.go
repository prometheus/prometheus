package agent

import (
	"bytes"
	"encoding/base64"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestConfigBindAddrParts(t *testing.T) {
	testCases := []struct {
		Value string
		IP    string
		Port  int
		Error bool
	}{
		{"0.0.0.0", "0.0.0.0", DefaultBindPort, false},
		{"0.0.0.0:1234", "0.0.0.0", 1234, false},
	}

	for _, tc := range testCases {
		c := &Config{BindAddr: tc.Value}
		ip, port, err := c.AddrParts(c.BindAddr)
		if tc.Error != (err != nil) {
			t.Errorf("Bad error: %s", err)
			continue
		}

		if tc.IP != ip {
			t.Errorf("%s: Got IP %#v", tc.Value, ip)
			continue
		}

		if tc.Port != port {
			t.Errorf("%s: Got port %d", tc.Value, port)
			continue
		}
	}
}

func TestConfigEncryptBytes(t *testing.T) {
	// Test with some input
	src := []byte("abc")
	c := &Config{
		EncryptKey: base64.StdEncoding.EncodeToString(src),
	}

	result, err := c.EncryptBytes()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !bytes.Equal(src, result) {
		t.Fatalf("bad: %#v", result)
	}

	// Test with no input
	c = &Config{}
	result, err = c.EncryptBytes()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(result) > 0 {
		t.Fatalf("bad: %#v", result)
	}
}

func TestConfigEventScripts(t *testing.T) {
	c := &Config{
		EventHandlers: []string{
			"foo.sh",
			"bar=blah.sh",
		},
	}

	result := c.EventScripts()
	if len(result) != 2 {
		t.Fatalf("bad: %#v", result)
	}

	expected := []EventScript{
		{EventFilter{"*", ""}, "foo.sh"},
		{EventFilter{"bar", ""}, "blah.sh"},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("bad: %#v", result)
	}
}

func TestDecodeConfig(t *testing.T) {
	// Without a protocol
	input := `{"node_name": "foo"}`
	config, err := DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.NodeName != "foo" {
		t.Fatalf("bad: %#v", config)
	}

	if config.Protocol != 0 {
		t.Fatalf("bad: %#v", config)
	}

	if config.SkipLeaveOnInt != DefaultConfig().SkipLeaveOnInt {
		t.Fatalf("bad: %#v", config)
	}

	if config.LeaveOnTerm != DefaultConfig().LeaveOnTerm {
		t.Fatalf("bad: %#v", config)
	}

	// With a protocol
	input = `{"node_name": "foo", "protocol": 7}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.NodeName != "foo" {
		t.Fatalf("bad: %#v", config)
	}

	if config.Protocol != 7 {
		t.Fatalf("bad: %#v", config)
	}

	// A bind addr
	input = `{"bind": "127.0.0.2"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.BindAddr != "127.0.0.2" {
		t.Fatalf("bad: %#v", config)
	}

	// replayOnJoin
	input = `{"replay_on_join": true}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.ReplayOnJoin != true {
		t.Fatalf("bad: %#v", config)
	}

	// leave_on_terminate
	input = `{"leave_on_terminate": true}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.LeaveOnTerm != true {
		t.Fatalf("bad: %#v", config)
	}

	// skip_leave_on_interrupt
	input = `{"skip_leave_on_interrupt": true}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.SkipLeaveOnInt != true {
		t.Fatalf("bad: %#v", config)
	}

	// tags
	input = `{"tags": {"foo": "bar", "role": "test"}}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.Tags["foo"] != "bar" {
		t.Fatalf("bad: %#v", config)
	}
	if config.Tags["role"] != "test" {
		t.Fatalf("bad: %#v", config)
	}

	// tags file
	input = `{"tags_file": "/some/path"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.TagsFile != "/some/path" {
		t.Fatalf("bad: %#v", config)
	}

	// Discover
	input = `{"discover": "foobar"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.Discover != "foobar" {
		t.Fatalf("bad: %#v", config)
	}

	// Interface
	input = `{"interface": "eth0"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.Interface != "eth0" {
		t.Fatalf("bad: %#v", config)
	}

	// Reconnect intervals
	input = `{"reconnect_interval": "15s", "reconnect_timeout": "48h"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.ReconnectInterval != 15*time.Second {
		t.Fatalf("bad: %#v", config)
	}

	if config.ReconnectTimeout != 48*time.Hour {
		t.Fatalf("bad: %#v", config)
	}

	// RPC Auth
	input = `{"rpc_auth": "foobar"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.RPCAuthKey != "foobar" {
		t.Fatalf("bad: %#v", config)
	}

	// DisableNameResolution
	input = `{"disable_name_resolution": true}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !config.DisableNameResolution {
		t.Fatalf("bad: %#v", config)
	}

	// Tombstone intervals
	input = `{"tombstone_timeout": "48h"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.TombstoneTimeout != 48*time.Hour {
		t.Fatalf("bad: %#v", config)
	}

	// Syslog
	input = `{"enable_syslog": true, "syslog_facility": "LOCAL4"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !config.EnableSyslog {
		t.Fatalf("bad: %#v", config)
	}
	if config.SyslogFacility != "LOCAL4" {
		t.Fatalf("bad: %#v", config)
	}

	// Retry configs
	input = `{"retry_max_attempts": 5, "retry_interval": "60s"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.RetryMaxAttempts != 5 {
		t.Fatalf("bad: %#v", config)
	}

	if config.RetryInterval != 60*time.Second {
		t.Fatalf("bad: %#v", config)
	}

	// Broadcast timeout
	input = `{"broadcast_timeout": "10s"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.BroadcastTimeout != 10*time.Second {
		t.Fatalf("bad: %#v", config)
	}

	// Retry configs
	input = `{"retry_join": ["127.0.0.1", "127.0.0.2"]}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(config.RetryJoin) != 2 {
		t.Fatalf("bad: %#v", config)
	}

	if config.RetryJoin[0] != "127.0.0.1" {
		t.Fatalf("bad: %#v", config)
	}

	if config.RetryJoin[1] != "127.0.0.2" {
		t.Fatalf("bad: %#v", config)
	}

	// Rejoin configs
	input = `{"rejoin_after_leave": true}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if !config.RejoinAfterLeave {
		t.Fatalf("bad: %#v", config)
	}

	// Stats configs
	input = `{"statsite_addr": "127.0.0.1:8123", "statsd_addr": "127.0.0.1:8125"}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.StatsiteAddr != "127.0.0.1:8123" {
		t.Fatalf("bad: %#v", config)
	}

	if config.StatsdAddr != "127.0.0.1:8125" {
		t.Fatalf("bad: %#v", config)
	}

	// Query sizes
	input = `{"query_response_size_limit": 123, "query_size_limit": 456}`
	config, err = DecodeConfig(bytes.NewReader([]byte(input)))
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.QueryResponseSizeLimit != 123 || config.QuerySizeLimit != 456 {
		t.Fatalf("bad: %#v", config)
	}
}

func TestDecodeConfig_unknownDirective(t *testing.T) {
	input := `{"unknown_directive": "titi"}`
	_, err := DecodeConfig(bytes.NewReader([]byte(input)))
	if err == nil {
		t.Fatal("should have err")
	}
}

func TestMergeConfig(t *testing.T) {
	a := &Config{
		NodeName:      "foo",
		Role:          "bar",
		Protocol:      7,
		EventHandlers: []string{"foo"},
		StartJoin:     []string{"foo"},
		ReplayOnJoin:  true,
		RetryJoin:     []string{"zab"},
	}

	b := &Config{
		NodeName:               "bname",
		DisableCoordinates:     true,
		Protocol:               -1,
		EncryptKey:             "foo",
		EventHandlers:          []string{"bar"},
		StartJoin:              []string{"bar"},
		LeaveOnTerm:            true,
		SkipLeaveOnInt:         true,
		Discover:               "tubez",
		Interface:              "eth0",
		ReconnectInterval:      15 * time.Second,
		ReconnectTimeout:       48 * time.Hour,
		RPCAuthKey:             "foobar",
		DisableNameResolution:  true,
		TombstoneTimeout:       36 * time.Hour,
		EnableSyslog:           true,
		RetryJoin:              []string{"zip"},
		RetryMaxAttempts:       10,
		RetryInterval:          120 * time.Second,
		RejoinAfterLeave:       true,
		StatsiteAddr:           "127.0.0.1:8125",
		QueryResponseSizeLimit: 123,
		QuerySizeLimit:         456,
		BroadcastTimeout:       20 * time.Second,
		EnableCompression:      true,
	}

	c := MergeConfig(a, b)

	if c.NodeName != "bname" {
		t.Fatalf("bad: %#v", c)
	}

	if c.Role != "bar" {
		t.Fatalf("bad: %#v", c)
	}

	if c.DisableCoordinates != true {
		t.Fatalf("bad: %#v", c)
	}

	if c.Protocol != 7 {
		t.Fatalf("bad: %#v", c)
	}

	if c.EncryptKey != "foo" {
		t.Fatalf("bad: %#v", c.EncryptKey)
	}

	if c.ReplayOnJoin != true {
		t.Fatalf("bad: %#v", c.ReplayOnJoin)
	}

	if !c.LeaveOnTerm {
		t.Fatalf("bad: %#v", c.LeaveOnTerm)
	}

	if !c.SkipLeaveOnInt {
		t.Fatalf("bad: %#v", c.SkipLeaveOnInt)
	}

	if c.Discover != "tubez" {
		t.Fatalf("Bad: %v", c.Discover)
	}

	if c.Interface != "eth0" {
		t.Fatalf("Bad: %v", c.Interface)
	}

	if c.ReconnectInterval != 15*time.Second {
		t.Fatalf("bad: %#v", c)
	}

	if c.ReconnectTimeout != 48*time.Hour {
		t.Fatalf("bad: %#v", c)
	}

	if c.TombstoneTimeout != 36*time.Hour {
		t.Fatalf("bad: %#v", c)
	}

	if c.RPCAuthKey != "foobar" {
		t.Fatalf("bad: %#v", c)
	}

	if !c.DisableNameResolution {
		t.Fatalf("bad: %#v", c)
	}

	if !c.EnableSyslog {
		t.Fatalf("bad: %#v", c)
	}

	if c.RetryMaxAttempts != 10 {
		t.Fatalf("bad: %#v", c)
	}

	if c.RetryInterval != 120*time.Second {
		t.Fatalf("bad: %#v", c)
	}

	if !c.RejoinAfterLeave {
		t.Fatalf("bad: %#v", c)
	}

	if c.StatsiteAddr != "127.0.0.1:8125" {
		t.Fatalf("bad: %#v", c)
	}

	expected := []string{"foo", "bar"}
	if !reflect.DeepEqual(c.EventHandlers, expected) {
		t.Fatalf("bad: %#v", c)
	}

	if !reflect.DeepEqual(c.StartJoin, expected) {
		t.Fatalf("bad: %#v", c)
	}

	expected = []string{"zab", "zip"}
	if !reflect.DeepEqual(c.RetryJoin, expected) {
		t.Fatalf("bad: %#v", c)
	}

	if c.QueryResponseSizeLimit != 123 || c.QuerySizeLimit != 456 {
		t.Fatalf("bad: %#v", c)
	}

	if c.BroadcastTimeout != 20*time.Second {
		t.Fatalf("bad: %#v", c)
	}

	if !c.EnableCompression {
		t.Fatalf("bad: %#v", c)
	}
}

func TestReadConfigPaths_badPath(t *testing.T) {
	_, err := ReadConfigPaths([]string{"/i/shouldnt/exist/ever/rainbows"})
	if err == nil {
		t.Fatal("should have err")
	}
}

func TestReadConfigPaths_file(t *testing.T) {
	tf, err := ioutil.TempFile("", "serf")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	tf.Write([]byte(`{"node_name":"bar"}`))
	tf.Close()
	defer os.Remove(tf.Name())

	config, err := ReadConfigPaths([]string{tf.Name()})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.NodeName != "bar" {
		t.Fatalf("bad: %#v", config)
	}
}

func TestReadConfigPaths_dir(t *testing.T) {
	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.RemoveAll(td)

	err = ioutil.WriteFile(filepath.Join(td, "a.json"),
		[]byte(`{"node_name": "bar"}`), 0644)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	err = ioutil.WriteFile(filepath.Join(td, "b.json"),
		[]byte(`{"node_name": "baz"}`), 0644)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	// A non-json file, shouldn't be read
	err = ioutil.WriteFile(filepath.Join(td, "c"),
		[]byte(`{"node_name": "bad"}`), 0644)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	config, err := ReadConfigPaths([]string{td})
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if config.NodeName != "baz" {
		t.Fatalf("bad: %#v", config)
	}
}

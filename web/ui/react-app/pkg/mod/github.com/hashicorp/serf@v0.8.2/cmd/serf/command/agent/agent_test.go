package agent

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/serf/serf"
	"github.com/hashicorp/serf/testutil"
)

func TestAgent_eventHandler(t *testing.T) {
	a1 := testAgent(nil)
	defer a1.Shutdown()
	defer a1.Leave()

	handler := new(MockEventHandler)
	a1.RegisterEventHandler(handler)

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	if len(handler.Events) != 1 {
		t.Fatalf("bad: %#v", handler.Events)
	}

	if handler.Events[0].EventType() != serf.EventMemberJoin {
		t.Fatalf("bad: %#v", handler.Events[0])
	}
}

func TestAgentShutdown_multiple(t *testing.T) {
	a := testAgent(nil)
	if err := a.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	for i := 0; i < 5; i++ {
		if err := a.Shutdown(); err != nil {
			t.Fatalf("err: %s", err)
		}
	}
}

func TestAgentUserEvent(t *testing.T) {
	a1 := testAgent(nil)
	defer a1.Shutdown()
	defer a1.Leave()

	handler := new(MockEventHandler)
	a1.RegisterEventHandler(handler)

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	if err := a1.UserEvent("deploy", []byte("foo"), false); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	handler.Lock()
	defer handler.Unlock()

	if len(handler.Events) == 0 {
		t.Fatal("no events")
	}

	e, ok := handler.Events[len(handler.Events)-1].(serf.UserEvent)
	if !ok {
		t.Fatalf("bad: %#v", e)
	}

	if e.Name != "deploy" {
		t.Fatalf("bad: %#v", e)
	}

	if string(e.Payload) != "foo" {
		t.Fatalf("bad: %#v", e)
	}
}

func TestAgentQuery_BadPrefix(t *testing.T) {
	a1 := testAgent(nil)
	defer a1.Shutdown()
	defer a1.Leave()

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	_, err := a1.Query("_serf_test", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "cannot contain") {
		t.Fatalf("err: %s", err)
	}
}

func TestAgentTagsFile(t *testing.T) {
	tags := map[string]string{
		"role":       "webserver",
		"datacenter": "us-east",
	}

	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.RemoveAll(td)

	agentConfig := DefaultConfig()
	agentConfig.TagsFile = filepath.Join(td, "tags.json")

	a1 := testAgentWithConfig(agentConfig, serf.DefaultConfig(), nil)

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer a1.Shutdown()
	defer a1.Leave()

	testutil.Yield()

	err = a1.SetTags(tags)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	a2 := testAgentWithConfig(agentConfig, serf.DefaultConfig(), nil)

	if err := a2.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer a2.Shutdown()
	defer a2.Leave()

	testutil.Yield()

	m := a2.Serf().LocalMember()

	if !reflect.DeepEqual(m.Tags, tags) {
		t.Fatalf("tags not restored: %#v", m.Tags)
	}
}

func TestAgentTagsFile_BadOptions(t *testing.T) {
	agentConfig := DefaultConfig()
	agentConfig.TagsFile = "/some/path"
	agentConfig.Tags = map[string]string{
		"tag1": "val1",
	}

	_, err := Create(agentConfig, serf.DefaultConfig(), nil)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err: %s", err)
	}
}

func TestAgent_MarshalTags(t *testing.T) {
	tags := map[string]string{
		"tag1": "val1",
		"tag2": "val2",
	}

	tagPairs := MarshalTags(tags)

	if !containsKey(tagPairs, "tag1=val1") {
		t.Fatalf("bad: %v", tagPairs)
	}
	if !containsKey(tagPairs, "tag2=val2") {
		t.Fatalf("bad: %v", tagPairs)
	}
}

func TestAgent_UnmarshalTags(t *testing.T) {
	tagPairs := []string{
		"tag1=val1",
		"tag2=val2",
	}

	tags, err := UnmarshalTags(tagPairs)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if v, ok := tags["tag1"]; !ok || v != "val1" {
		t.Fatalf("bad: %v", tags)
	}
	if v, ok := tags["tag2"]; !ok || v != "val2" {
		t.Fatalf("bad: %v", tags)
	}
}

func TestAgent_UnmarshalTagsError(t *testing.T) {
	tagSets := [][]string{
		[]string{"="},
		[]string{"=x"},
		[]string{""},
		[]string{"x"},
	}
	for _, tagPairs := range tagSets {
		if _, err := UnmarshalTags(tagPairs); err == nil {
			t.Fatalf("Expected tag error: %s", tagPairs[0])
		}
	}
}

func TestAgentKeyringFile(t *testing.T) {
	keys := []string{
		"enjTwAFRe4IE71bOFhirzQ==",
		"csT9mxI7aTf9ap3HLBbdmA==",
		"noha2tVc0OyD/2LtCBoAOQ==",
	}

	td, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.RemoveAll(td)

	keyringFile := filepath.Join(td, "keyring.json")

	serfConfig := serf.DefaultConfig()
	agentConfig := DefaultConfig()
	agentConfig.KeyringFile = keyringFile

	encodedKeys, err := json.Marshal(keys)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if err := ioutil.WriteFile(keyringFile, encodedKeys, 0600); err != nil {
		t.Fatalf("err: %s", err)
	}

	a1 := testAgentWithConfig(agentConfig, serfConfig, nil)

	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer a1.Shutdown()

	testutil.Yield()

	totalLoadedKeys := len(serfConfig.MemberlistConfig.Keyring.GetKeys())
	if totalLoadedKeys != 3 {
		t.Fatalf("Expected to load 3 keys but got %d", totalLoadedKeys)
	}
}

func TestAgentKeyringFile_BadOptions(t *testing.T) {
	agentConfig := DefaultConfig()
	agentConfig.KeyringFile = "/some/path"
	agentConfig.EncryptKey = "pL4owv4IE1x+ZXCyd5vLLg=="

	_, err := Create(agentConfig, serf.DefaultConfig(), nil)
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err: %s", err)
	}
}

func TestAgentKeyringFile_NoKeys(t *testing.T) {
	dir, err := ioutil.TempDir("", "serf")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.RemoveAll(dir)

	keysFile := filepath.Join(dir, "keyring")
	if err := ioutil.WriteFile(keysFile, []byte("[]"), 0600); err != nil {
		t.Fatalf("err: %s", err)
	}

	agentConfig := DefaultConfig()
	agentConfig.KeyringFile = keysFile

	_, err = Create(agentConfig, serf.DefaultConfig(), nil)
	if err == nil {
		t.Fatalf("should have errored")
	}
	if !strings.Contains(err.Error(), "contains no keys") {
		t.Fatalf("bad: %s", err)
	}
}

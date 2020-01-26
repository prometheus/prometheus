package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/serf"
)

// Agent starts and manages a Serf instance, adding some niceties
// on top of Serf such as storing logs that you can later retrieve,
// and invoking EventHandlers when events occur.
type Agent struct {
	// Stores the serf configuration
	conf *serf.Config

	// Stores the agent configuration
	agentConf *Config

	// eventCh is used for Serf to deliver events on
	eventCh chan serf.Event

	// eventHandlers is the registered handlers for events
	eventHandlers     map[EventHandler]struct{}
	eventHandlerList  []EventHandler
	eventHandlersLock sync.Mutex

	// logger instance wraps the logOutput
	logger *log.Logger

	// This is the underlying Serf we are wrapping
	serf *serf.Serf

	// shutdownCh is used for shutdowns
	shutdown     bool
	shutdownCh   chan struct{}
	shutdownLock sync.Mutex
}

// Start creates a new agent, potentially returning an error
func Create(agentConf *Config, conf *serf.Config, logOutput io.Writer) (*Agent, error) {
	// Ensure we have a log sink
	if logOutput == nil {
		logOutput = os.Stderr
	}

	// Setup the underlying loggers
	conf.MemberlistConfig.LogOutput = logOutput
	conf.MemberlistConfig.EnableCompression = agentConf.EnableCompression
	conf.LogOutput = logOutput

	// Create a channel to listen for events from Serf
	eventCh := make(chan serf.Event, 64)
	conf.EventCh = eventCh

	// Setup the agent
	agent := &Agent{
		conf:          conf,
		agentConf:     agentConf,
		eventCh:       eventCh,
		eventHandlers: make(map[EventHandler]struct{}),
		logger:        log.New(logOutput, "", log.LstdFlags),
		shutdownCh:    make(chan struct{}),
	}

	// Restore agent tags from a tags file
	if agentConf.TagsFile != "" {
		if err := agent.loadTagsFile(agentConf.TagsFile); err != nil {
			return nil, err
		}
	}

	// Load in a keyring file if provided
	if agentConf.KeyringFile != "" {
		if err := agent.loadKeyringFile(agentConf.KeyringFile); err != nil {
			return nil, err
		}
	}

	return agent, nil
}

// Start is used to initiate the event listeners. It is separate from
// create so that there isn't a race condition between creating the
// agent and registering handlers
func (a *Agent) Start() error {
	a.logger.Printf("[INFO] agent: Serf agent starting")

	// Create serf first
	serf, err := serf.Create(a.conf)
	if err != nil {
		return fmt.Errorf("Error creating Serf: %s", err)
	}
	a.serf = serf

	// Start event loop
	go a.eventLoop()
	return nil
}

// Leave prepares for a graceful shutdown of the agent and its processes
func (a *Agent) Leave() error {
	if a.serf == nil {
		return nil
	}

	a.logger.Println("[INFO] agent: requesting graceful leave from Serf")
	return a.serf.Leave()
}

// Shutdown closes this agent and all of its processes. Should be preceded
// by a Leave for a graceful shutdown.
func (a *Agent) Shutdown() error {
	a.shutdownLock.Lock()
	defer a.shutdownLock.Unlock()

	if a.shutdown {
		return nil
	}

	if a.serf == nil {
		goto EXIT
	}

	a.logger.Println("[INFO] agent: requesting serf shutdown")
	if err := a.serf.Shutdown(); err != nil {
		return err
	}

EXIT:
	a.logger.Println("[INFO] agent: shutdown complete")
	a.shutdown = true
	close(a.shutdownCh)
	return nil
}

// ShutdownCh returns a channel that can be selected to wait
// for the agent to perform a shutdown.
func (a *Agent) ShutdownCh() <-chan struct{} {
	return a.shutdownCh
}

// Returns the Serf agent of the running Agent.
func (a *Agent) Serf() *serf.Serf {
	return a.serf
}

// Returns the Serf config of the running Agent.
func (a *Agent) SerfConfig() *serf.Config {
	return a.conf
}

// Join asks the Serf instance to join. See the Serf.Join function.
func (a *Agent) Join(addrs []string, replay bool) (n int, err error) {
	a.logger.Printf("[INFO] agent: joining: %v replay: %v", addrs, replay)
	ignoreOld := !replay
	n, err = a.serf.Join(addrs, ignoreOld)
	if n > 0 {
		a.logger.Printf("[INFO] agent: joined: %d nodes", n)
	}
	if err != nil {
		a.logger.Printf("[WARN] agent: error joining: %v", err)
	}
	return
}

// ForceLeave is used to eject a failed node from the cluster
func (a *Agent) ForceLeave(node string) error {
	a.logger.Printf("[INFO] agent: Force leaving node: %s", node)
	err := a.serf.RemoveFailedNode(node)
	if err != nil {
		a.logger.Printf("[WARN] agent: failed to remove node: %v", err)
	}
	return err
}

// UserEvent sends a UserEvent on Serf, see Serf.UserEvent.
func (a *Agent) UserEvent(name string, payload []byte, coalesce bool) error {
	a.logger.Printf("[DEBUG] agent: Requesting user event send: %s. Coalesced: %#v. Payload: %#v",
		name, coalesce, string(payload))
	err := a.serf.UserEvent(name, payload, coalesce)
	if err != nil {
		a.logger.Printf("[WARN] agent: failed to send user event: %v", err)
	}
	return err
}

// Query sends a Query on Serf, see Serf.Query.
func (a *Agent) Query(name string, payload []byte, params *serf.QueryParam) (*serf.QueryResponse, error) {
	// Prevent the use of the internal prefix
	if strings.HasPrefix(name, serf.InternalQueryPrefix) {
		// Allow the special "ping" query
		if name != serf.InternalQueryPrefix+"ping" || payload != nil {
			return nil, fmt.Errorf("Queries cannot contain the '%s' prefix", serf.InternalQueryPrefix)
		}
	}
	a.logger.Printf("[DEBUG] agent: Requesting query send: %s. Payload: %#v",
		name, string(payload))
	resp, err := a.serf.Query(name, payload, params)
	if err != nil {
		a.logger.Printf("[WARN] agent: failed to start user query: %v", err)
	}
	return resp, err
}

// RegisterEventHandler adds an event handler to receive event notifications
func (a *Agent) RegisterEventHandler(eh EventHandler) {
	a.eventHandlersLock.Lock()
	defer a.eventHandlersLock.Unlock()

	a.eventHandlers[eh] = struct{}{}
	a.eventHandlerList = nil
	for eh := range a.eventHandlers {
		a.eventHandlerList = append(a.eventHandlerList, eh)
	}
}

// DeregisterEventHandler removes an EventHandler and prevents more invocations
func (a *Agent) DeregisterEventHandler(eh EventHandler) {
	a.eventHandlersLock.Lock()
	defer a.eventHandlersLock.Unlock()

	delete(a.eventHandlers, eh)
	a.eventHandlerList = nil
	for eh := range a.eventHandlers {
		a.eventHandlerList = append(a.eventHandlerList, eh)
	}
}

// eventLoop listens to events from Serf and fans out to event handlers
func (a *Agent) eventLoop() {
	serfShutdownCh := a.serf.ShutdownCh()
	for {
		select {
		case e := <-a.eventCh:
			a.logger.Printf("[INFO] agent: Received event: %s", e.String())
			a.eventHandlersLock.Lock()
			handlers := a.eventHandlerList
			a.eventHandlersLock.Unlock()
			for _, eh := range handlers {
				eh.HandleEvent(e)
			}

		case <-serfShutdownCh:
			a.logger.Printf("[WARN] agent: Serf shutdown detected, quitting")
			a.Shutdown()
			return

		case <-a.shutdownCh:
			return
		}
	}
}

// InstallKey initiates a query to install a new key on all members
func (a *Agent) InstallKey(key string) (*serf.KeyResponse, error) {
	a.logger.Print("[INFO] agent: Initiating key installation")
	manager := a.serf.KeyManager()
	return manager.InstallKey(key)
}

// UseKey sends a query instructing all members to switch primary keys
func (a *Agent) UseKey(key string) (*serf.KeyResponse, error) {
	a.logger.Print("[INFO] agent: Initiating primary key change")
	manager := a.serf.KeyManager()
	return manager.UseKey(key)
}

// RemoveKey sends a query to all members to remove a key from the keyring
func (a *Agent) RemoveKey(key string) (*serf.KeyResponse, error) {
	a.logger.Print("[INFO] agent: Initiating key removal")
	manager := a.serf.KeyManager()
	return manager.RemoveKey(key)
}

// ListKeys sends a query to all members to return a list of their keys
func (a *Agent) ListKeys() (*serf.KeyResponse, error) {
	a.logger.Print("[INFO] agent: Initiating key listing")
	manager := a.serf.KeyManager()
	return manager.ListKeys()
}

// SetTags is used to update the tags. The agent will make sure to
// persist tags if necessary before gossiping to the cluster.
func (a *Agent) SetTags(tags map[string]string) error {
	// Update the tags file if we have one
	if a.agentConf.TagsFile != "" {
		if err := a.writeTagsFile(tags); err != nil {
			a.logger.Printf("[ERR] agent: %s", err)
			return err
		}
	}

	// Set the tags in Serf, start gossiping out
	return a.serf.SetTags(tags)
}

// loadTagsFile will load agent tags out of a file and set them in the
// current serf configuration.
func (a *Agent) loadTagsFile(tagsFile string) error {
	// Avoid passing tags and using a tags file at the same time
	if len(a.agentConf.Tags) > 0 {
		return fmt.Errorf("Tags config not allowed while using tag files")
	}

	if _, err := os.Stat(tagsFile); err == nil {
		tagData, err := ioutil.ReadFile(tagsFile)
		if err != nil {
			return fmt.Errorf("Failed to read tags file: %s", err)
		}
		if err := json.Unmarshal(tagData, &a.conf.Tags); err != nil {
			return fmt.Errorf("Failed to decode tags file: %s", err)
		}
		a.logger.Printf("[INFO] agent: Restored %d tag(s) from %s",
			len(a.conf.Tags), tagsFile)
	}

	// Success!
	return nil
}

// writeTagsFile will write the current tags to the configured tags file.
func (a *Agent) writeTagsFile(tags map[string]string) error {
	encoded, err := json.MarshalIndent(tags, "", "  ")
	if err != nil {
		return fmt.Errorf("Failed to encode tags: %s", err)
	}

	// Use 0600 for permissions, in case tag data is sensitive
	if err = ioutil.WriteFile(a.agentConf.TagsFile, encoded, 0600); err != nil {
		return fmt.Errorf("Failed to write tags file: %s", err)
	}

	// Success!
	return nil
}

// MarshalTags is a utility function which takes a map of tag key/value pairs
// and returns the same tags as strings in 'key=value' format.
func MarshalTags(tags map[string]string) []string {
	var result []string
	for name, value := range tags {
		result = append(result, fmt.Sprintf("%s=%s", name, value))
	}
	return result
}

// UnmarshalTags is a utility function which takes a slice of strings in
// key=value format and returns them as a tag mapping.
func UnmarshalTags(tags []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) != 2 || len(parts[0]) == 0 {
			return nil, fmt.Errorf("Invalid tag: '%s'", tag)
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}

// loadKeyringFile will load a keyring out of a file
func (a *Agent) loadKeyringFile(keyringFile string) error {
	// Avoid passing an encryption key and a keyring file at the same time
	if len(a.agentConf.EncryptKey) > 0 {
		return fmt.Errorf("Encryption key not allowed while using a keyring")
	}

	if _, err := os.Stat(keyringFile); err != nil {
		return err
	}

	// Read in the keyring file data
	keyringData, err := ioutil.ReadFile(keyringFile)
	if err != nil {
		return fmt.Errorf("Failed to read keyring file: %s", err)
	}

	// Decode keyring JSON
	keys := make([]string, 0)
	if err := json.Unmarshal(keyringData, &keys); err != nil {
		return fmt.Errorf("Failed to decode keyring file: %s", err)
	}

	// Decode base64 values
	keysDecoded := make([][]byte, len(keys))
	for i, key := range keys {
		keyBytes, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			return fmt.Errorf("Failed to decode key from keyring: %s", err)
		}
		keysDecoded[i] = keyBytes
	}

	// Guard against empty keyring file
	if len(keysDecoded) == 0 {
		return fmt.Errorf("Keyring file contains no keys")
	}

	// Create the keyring
	keyring, err := memberlist.NewKeyring(keysDecoded, keysDecoded[0])
	if err != nil {
		return fmt.Errorf("Failed to restore keyring: %s", err)
	}
	a.conf.MemberlistConfig.Keyring = keyring
	a.logger.Printf("[INFO] agent: Restored keyring with %d keys from %s",
		len(keys), keyringFile)

	// Success!
	return nil
}

// Stats is used to get various runtime information and stats
func (a *Agent) Stats() map[string]map[string]string {
	local := a.serf.LocalMember()
	event_handlers := make(map[string]string)

	// Convert event handlers from a string slice to a string map
	for _, script := range a.agentConf.EventScripts() {
		script_filter := fmt.Sprintf("%s:%s", script.EventFilter.Event, script.EventFilter.Name)
		event_handlers[script_filter] = script.Script
	}

	output := map[string]map[string]string{
		"agent": map[string]string{
			"name": local.Name,
		},
		"runtime":        runtimeStats(),
		"serf":           a.serf.Stats(),
		"tags":           local.Tags,
		"event_handlers": event_handlers,
	}
	return output
}

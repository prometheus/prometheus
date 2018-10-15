package collins

import (
	"encoding/json"
	"fmt"

	"gopkg.in/tumblr/go-collins.v0/collins/sseclient"
)

// FirehoseService provides functions to communicate with the collins firehose.
type FirehoseService struct {
	client *Client
}

// FirehoseDecoder is passed to the SSE client in order to automatically parse
// the various events received from the firehose. It implements the Decoder
// interface from the sseclient package.
type FirehoseDecoder struct{}

// MakeContainer takes the event name and returns an instance of the correct
// container struct for that event.
func (fd FirehoseDecoder) MakeContainer(str string) (sseclient.Event, error) {
	switch str {
	case "asset_update":
		return &AssetUpdateEvent{}, nil
	case "asset_create":
		return &AssetCreateEvent{}, nil
	case "asset_delete":
		return &AssetDeleteEvent{}, nil
	case "asset_purge":
		return &AssetPurgeEvent{}, nil
	case "ipAddresses_create":
		return &IPAddressCreateEvent{}, nil
	case "ipAddresses_update":
		return &IPAddressUpdateEvent{}, nil
	case "ipAddresses_delete":
		return &IPAddressDeleteEvent{}, nil
	}

	return nil, fmt.Errorf("Unknown event type: %s", str)
}

// AssetEvent represents an event sent by the collins firehose.
type AssetEvent struct {
	Name     string
	Tag      string
	Category string
	Asset    Asset `json:"data"`
}

// AssetUpdateEvent, AssetCreateEvent, AssetDeleteEvent and AssetPurgeEvent are
// used to be able to tell different asset events apart.
type AssetUpdateEvent AssetEvent
type AssetCreateEvent AssetEvent
type AssetDeleteEvent AssetEvent
type AssetPurgeEvent AssetEvent

// IPAddressCreateEvent, IPAddressUpdateEvent and IPAddressDeleteEvent are
// events used when various IPAM actions are performed.
type IPAddressCreateEvent AssetEvent
type IPAddressUpdateEvent AssetEvent
type IPAddressDeleteEvent AssetEvent

// Decode fulfills the Event interface for the various asset event structs by
// unmarshalling the JSON returned by the firehose.
func (aue *AssetUpdateEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, aue)
}
func (ace *AssetCreateEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, ace)
}
func (ade *AssetDeleteEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, ade)
}
func (ape *AssetPurgeEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, ape)
}
func (ice *IPAddressCreateEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, ice)
}
func (iue *IPAddressUpdateEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, iue)
}
func (ide *IPAddressDeleteEvent) Decode(name string, data []byte) {
	json.Unmarshal(data, ide)
}

// FirehoseService.Consume connects to the collins firehose and returns a
// channel of Events.
func (s FirehoseService) Consume() (<-chan sseclient.Event, error) {
	req, err := s.client.NewRequest("GET", "api/firehose")
	if err != nil {
		return nil, err
	}

	sseClient, err := sseclient.New(s.client.client, req)
	if err != nil {
		return nil, err
	}

	decoder := FirehoseDecoder{}
	eventChannel, err, _ := sseClient.Consume(decoder)
	if err != nil {
		return nil, err
	}

	return eventChannel, nil
}

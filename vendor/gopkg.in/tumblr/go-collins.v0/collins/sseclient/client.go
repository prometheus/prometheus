package sseclient

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

// The Decoder interface describes how to implement a SSE stream decoder.
// MakeContainer() should take the name of the event and return a suitable
// struct that satisfies the Event interface.
type Decoder interface {
	MakeContainer(string) (Event, error)
}

// The Event interface is for types of events sent from the SSE source, and
// describes how to decode them. Decode() takes the event name and the raw data
// string and process it.
type Event interface {
	Decode(name string, data []byte)
}

// RawDecoder is the default decoder and doesn't do anything to the data.
type RawDecoder struct{}

// MakeContainer just returns a RawEvent struct that is used to return the data
// to the user.
func (rd RawDecoder) MakeContainer(str string) (Event, error) {
	return &RawEvent{}, nil
}

// RawEvent represents a raw server sent event. The data is not parsed, just
// returned as a raw string. RawEvent satisfies the Event interface.
type RawEvent struct {
	Name string
	Data string
}

// Decode simply copies the event name and string in to the RawEvent struct to
// satisfy the Event interface.
func (e *RawEvent) Decode(name string, data []byte) {
	e.Name = name
	e.Data = string(data)
}

// The Client struct contains information about the request to be made and the
// http.Client to use to make it. It also contains a LoopState struct.
type Client struct {
	request    *http.Request
	httpClient *http.Client
	state      *LoopState
}

// LoopState contains the ID of the last received event (or the empty string if
// no ID has been received) and the time the server has requested the client to
// wait before reconnecting (or 5000 if not specified) in milliseconds.
type LoopState struct {
	ReconnectTime int
	LastId        string
}

// New() constructs a new sseclient.Client from a http.Client with information on
// how to connect to the event source and a http.Request containing information
// about the path and parameters. New() will set the Accpet and Cache-Control
// headers as recommended.
func New(client *http.Client, request *http.Request) (*Client, error) {
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	c := Client{
		request:    request,
		httpClient: client,
	}
	return &c, nil
}

// Consume() opens the request to the event source and starts the event loop. It
// returns a channel of Events, an error and a pointer to a LoopState struct
// with information about the reconnect time and last id seen. The LoopState
// struct can be used in later calls to Consume() in order to pick up where an
// interrupted request left off.
func (c Client) Consume(d Decoder) (<-chan Event, error, *LoopState) {
	if c.state == nil {
		state := LoopState{
			ReconnectTime: 5000,
			LastId:        "",
		}
		c.state = &state
	}

	if d == nil {
		d = RawDecoder{}
	}

	if c.state.LastId != "" {
		c.request.Header.Set("Last-Event-ID", c.state.LastId)
	}

	resp, err := c.httpClient.Do(c.request)
	if err != nil {
		return nil, err, nil
	}

	// HTTP 204 No Content means the SSE source wants the client to stop
	// reconnecting.
	if resp.StatusCode == 204 {
		return nil, fmt.Errorf("SSE source returned %s", resp.Status), nil
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "text/event-stream" {
		return nil, fmt.Errorf("SSE source returned content type %s, should be text/event-stream", contentType), nil
	}

	events := make(chan Event)

	go func() {
		defer close(events)
		defer resp.Body.Close()
		eventLoop(events, resp.Body, c.state, d)
	}()

	return events, nil, c.state
}

func eventLoop(events chan<- Event, data io.Reader, state *LoopState, d Decoder) {
	var buffer bytes.Buffer
	eventName := ""
	scanner := bufio.NewScanner(data)
	for scanner.Scan() {
		line := scanner.Bytes()
		switch {
		case len(line) == 0: // End of event, create struct and send it to channel
			event, err := d.MakeContainer(eventName)
			if err == nil {
				event.Decode(eventName, buffer.Bytes())
				events <- event
			}
			eventName = ""
			buffer.Reset()
		case len(fieldName(line)) == 0: // Comment message, ignore
		case bytes.Equal(fieldName(line), []byte("event")):
			eventName = string(fieldValue(line))
		case bytes.Equal(fieldName(line), []byte("data")):
			buffer.Write(fieldValue(line))
		case bytes.Equal(fieldName(line), []byte("id")):
			state.LastId = string(fieldValue(line))
		case bytes.Equal(fieldName(line), []byte("retry")):
			newTime, err := strconv.Atoi(string(fieldValue(line)))
			if err == nil {
				state.ReconnectTime = newTime
			}
		}
	}
}

// A field name is defined as a string of any unicode characters except LF, CR,
// or colon, followed by a colon. Read all bytes up to ':', and if any of them
// are disallowed, return an empty field name and treat it as a comment message.
func fieldName(line []byte) []byte {
	name := []byte{}
	for _, c := range line {
		if c == '\n' || c == '\r' {
			name = []byte{}
			break
		}

		if c == ':' {
			break
		}
		name = append(name, byte(c))
	}
	return name
}

// The field value is all non-LF and -CR characters after a ':'. If the first
// character is space, remove it. If there is no ':' in the line, the value is
// defined as the empty string.
func fieldValue(line []byte) []byte {
	tokens := bytes.SplitN(line, []byte(":"), 2)
	if len(tokens) != 2 {
		return []byte("")
	}
	val := tokens[1]
	if val[0] == ' ' {
		return val[1:]
	}

	return val
}

package sarama

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Broker represents a single Kafka broker connection. All operations on this object are entirely concurrency-safe.
type Broker struct {
	id   int32
	addr string

	conf          *Config
	correlationID int32
	conn          net.Conn
	connErr       error
	lock          sync.Mutex
	opened        int32

	responses chan responsePromise
	done      chan bool
}

type responsePromise struct {
	correlationID int32
	packets       chan []byte
	errors        chan error
}

// NewBroker creates and returns a Broker targetting the given host:port address.
// This does not attempt to actually connect, you have to call Open() for that.
func NewBroker(addr string) *Broker {
	return &Broker{id: -1, addr: addr}
}

// Open tries to connect to the Broker if it is not already connected or connecting, but does not block
// waiting for the connection to complete. This means that any subsequent operations on the broker will
// block waiting for the connection to succeed or fail. To get the effect of a fully synchronous Open call,
// follow it by a call to Connected(). The only errors Open will return directly are ConfigurationError or
// AlreadyConnected. If conf is nil, the result of NewConfig() is used.
func (b *Broker) Open(conf *Config) error {
	if conf == nil {
		conf = NewConfig()
	}

	err := conf.Validate()
	if err != nil {
		return err
	}

	if !atomic.CompareAndSwapInt32(&b.opened, 0, 1) {
		return ErrAlreadyConnected
	}

	b.lock.Lock()

	if b.conn != nil {
		b.lock.Unlock()
		Logger.Printf("Failed to connect to broker %s: %s\n", b.addr, ErrAlreadyConnected)
		return ErrAlreadyConnected
	}

	go withRecover(func() {
		defer b.lock.Unlock()

		dialer := net.Dialer{
			Timeout:   conf.Net.DialTimeout,
			KeepAlive: conf.Net.KeepAlive,
		}

		if conf.Net.TLS.Enable {
			b.conn, b.connErr = tls.DialWithDialer(&dialer, "tcp", b.addr, conf.Net.TLS.Config)
		} else {
			b.conn, b.connErr = dialer.Dial("tcp", b.addr)
		}
		if b.connErr != nil {
			b.conn = nil
			atomic.StoreInt32(&b.opened, 0)
			Logger.Printf("Failed to connect to broker %s: %s\n", b.addr, b.connErr)
			return
		}

		b.conf = conf
		b.done = make(chan bool)
		b.responses = make(chan responsePromise, b.conf.Net.MaxOpenRequests-1)

		if b.id >= 0 {
			Logger.Printf("Connected to broker at %s (registered as #%d)\n", b.addr, b.id)
		} else {
			Logger.Printf("Connected to broker at %s (unregistered)\n", b.addr)
		}
		go withRecover(b.responseReceiver)
	})

	return nil
}

// Connected returns true if the broker is connected and false otherwise. If the broker is not
// connected but it had tried to connect, the error from that connection attempt is also returned.
func (b *Broker) Connected() (bool, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.conn != nil, b.connErr
}

func (b *Broker) Close() error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.conn == nil {
		return ErrNotConnected
	}

	close(b.responses)
	<-b.done

	err := b.conn.Close()

	b.conn = nil
	b.connErr = nil
	b.done = nil
	b.responses = nil

	atomic.StoreInt32(&b.opened, 0)

	if err == nil {
		Logger.Printf("Closed connection to broker %s\n", b.addr)
	} else {
		Logger.Printf("Error while closing connection to broker %s: %s\n", b.addr, err)
	}

	return err
}

// ID returns the broker ID retrieved from Kafka's metadata, or -1 if that is not known.
func (b *Broker) ID() int32 {
	return b.id
}

// Addr returns the broker address as either retrieved from Kafka's metadata or passed to NewBroker.
func (b *Broker) Addr() string {
	return b.addr
}

func (b *Broker) GetMetadata(request *MetadataRequest) (*MetadataResponse, error) {
	response := new(MetadataResponse)

	err := b.sendAndReceive(request, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) GetConsumerMetadata(request *ConsumerMetadataRequest) (*ConsumerMetadataResponse, error) {
	response := new(ConsumerMetadataResponse)

	err := b.sendAndReceive(request, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) GetAvailableOffsets(request *OffsetRequest) (*OffsetResponse, error) {
	response := new(OffsetResponse)

	err := b.sendAndReceive(request, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) Produce(request *ProduceRequest) (*ProduceResponse, error) {
	var response *ProduceResponse
	var err error

	if request.RequiredAcks == NoResponse {
		err = b.sendAndReceive(request, nil)
	} else {
		response = new(ProduceResponse)
		err = b.sendAndReceive(request, response)
	}

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) Fetch(request *FetchRequest) (*FetchResponse, error) {
	response := new(FetchResponse)

	err := b.sendAndReceive(request, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) CommitOffset(request *OffsetCommitRequest) (*OffsetCommitResponse, error) {
	response := new(OffsetCommitResponse)

	err := b.sendAndReceive(request, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) FetchOffset(request *OffsetFetchRequest) (*OffsetFetchResponse, error) {
	response := new(OffsetFetchResponse)

	err := b.sendAndReceive(request, response)

	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) send(rb requestBody, promiseResponse bool) (*responsePromise, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.conn == nil {
		if b.connErr != nil {
			return nil, b.connErr
		}
		return nil, ErrNotConnected
	}

	req := &request{correlationID: b.correlationID, clientID: b.conf.ClientID, body: rb}
	buf, err := encode(req)
	if err != nil {
		return nil, err
	}

	err = b.conn.SetWriteDeadline(time.Now().Add(b.conf.Net.WriteTimeout))
	if err != nil {
		return nil, err
	}

	_, err = b.conn.Write(buf)
	if err != nil {
		return nil, err
	}
	b.correlationID++

	if !promiseResponse {
		return nil, nil
	}

	promise := responsePromise{req.correlationID, make(chan []byte), make(chan error)}
	b.responses <- promise

	return &promise, nil
}

func (b *Broker) sendAndReceive(req requestBody, res decoder) error {
	promise, err := b.send(req, res != nil)

	if err != nil {
		return err
	}

	if promise == nil {
		return nil
	}

	select {
	case buf := <-promise.packets:
		return decode(buf, res)
	case err = <-promise.errors:
		return err
	}
}

func (b *Broker) decode(pd packetDecoder) (err error) {
	b.id, err = pd.getInt32()
	if err != nil {
		return err
	}

	host, err := pd.getString()
	if err != nil {
		return err
	}

	port, err := pd.getInt32()
	if err != nil {
		return err
	}

	b.addr = net.JoinHostPort(host, fmt.Sprint(port))
	if _, _, err := net.SplitHostPort(b.addr); err != nil {
		return err
	}

	return nil
}

func (b *Broker) encode(pe packetEncoder) (err error) {

	host, portstr, err := net.SplitHostPort(b.addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portstr)
	if err != nil {
		return err
	}

	pe.putInt32(b.id)

	err = pe.putString(host)
	if err != nil {
		return err
	}

	pe.putInt32(int32(port))

	return nil
}

func (b *Broker) responseReceiver() {
	header := make([]byte, 8)
	for response := range b.responses {
		err := b.conn.SetReadDeadline(time.Now().Add(b.conf.Net.ReadTimeout))
		if err != nil {
			response.errors <- err
			continue
		}

		_, err = io.ReadFull(b.conn, header)
		if err != nil {
			response.errors <- err
			continue
		}

		decodedHeader := responseHeader{}
		err = decode(header, &decodedHeader)
		if err != nil {
			response.errors <- err
			continue
		}
		if decodedHeader.correlationID != response.correlationID {
			// TODO if decoded ID < cur ID, discard until we catch up
			// TODO if decoded ID > cur ID, save it so when cur ID catches up we have a response
			response.errors <- PacketDecodingError{fmt.Sprintf("correlation ID didn't match, wanted %d, got %d", response.correlationID, decodedHeader.correlationID)}
			continue
		}

		buf := make([]byte, decodedHeader.length-4)
		_, err = io.ReadFull(b.conn, buf)
		if err != nil {
			// XXX: the above ReadFull call inherits the same ReadDeadline set at the top of this loop, so it may
			// fail with a timeout error. If this happens, our connection is permanently toast since we will no longer
			// be aligned correctly on the stream (we'll be reading garbage Kafka headers from the middle of data).
			// Can we/should we fail harder in that case?
			response.errors <- err
			continue
		}

		response.packets <- buf
	}
	close(b.done)
}

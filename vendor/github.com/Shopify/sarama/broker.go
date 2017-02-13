package sarama

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rcrowley/go-metrics"
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

	incomingByteRate       metrics.Meter
	requestRate            metrics.Meter
	requestSize            metrics.Histogram
	requestLatency         metrics.Histogram
	outgoingByteRate       metrics.Meter
	responseRate           metrics.Meter
	responseSize           metrics.Histogram
	brokerIncomingByteRate metrics.Meter
	brokerRequestRate      metrics.Meter
	brokerRequestSize      metrics.Histogram
	brokerRequestLatency   metrics.Histogram
	brokerOutgoingByteRate metrics.Meter
	brokerResponseRate     metrics.Meter
	brokerResponseSize     metrics.Histogram
}

type responsePromise struct {
	requestTime   time.Time
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
	if !atomic.CompareAndSwapInt32(&b.opened, 0, 1) {
		return ErrAlreadyConnected
	}

	if conf == nil {
		conf = NewConfig()
	}

	err := conf.Validate()
	if err != nil {
		return err
	}

	b.lock.Lock()

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
			Logger.Printf("Failed to connect to broker %s: %s\n", b.addr, b.connErr)
			b.conn = nil
			atomic.StoreInt32(&b.opened, 0)
			return
		}
		b.conn = newBufConn(b.conn)

		b.conf = conf

		// Create or reuse the global metrics shared between brokers
		b.incomingByteRate = metrics.GetOrRegisterMeter("incoming-byte-rate", conf.MetricRegistry)
		b.requestRate = metrics.GetOrRegisterMeter("request-rate", conf.MetricRegistry)
		b.requestSize = getOrRegisterHistogram("request-size", conf.MetricRegistry)
		b.requestLatency = getOrRegisterHistogram("request-latency-in-ms", conf.MetricRegistry)
		b.outgoingByteRate = metrics.GetOrRegisterMeter("outgoing-byte-rate", conf.MetricRegistry)
		b.responseRate = metrics.GetOrRegisterMeter("response-rate", conf.MetricRegistry)
		b.responseSize = getOrRegisterHistogram("response-size", conf.MetricRegistry)
		// Do not gather metrics for seeded broker (only used during bootstrap) because they share
		// the same id (-1) and are already exposed through the global metrics above
		if b.id >= 0 {
			b.brokerIncomingByteRate = getOrRegisterBrokerMeter("incoming-byte-rate", b, conf.MetricRegistry)
			b.brokerRequestRate = getOrRegisterBrokerMeter("request-rate", b, conf.MetricRegistry)
			b.brokerRequestSize = getOrRegisterBrokerHistogram("request-size", b, conf.MetricRegistry)
			b.brokerRequestLatency = getOrRegisterBrokerHistogram("request-latency-in-ms", b, conf.MetricRegistry)
			b.brokerOutgoingByteRate = getOrRegisterBrokerMeter("outgoing-byte-rate", b, conf.MetricRegistry)
			b.brokerResponseRate = getOrRegisterBrokerMeter("response-rate", b, conf.MetricRegistry)
			b.brokerResponseSize = getOrRegisterBrokerHistogram("response-size", b, conf.MetricRegistry)
		}

		if conf.Net.SASL.Enable {
			b.connErr = b.sendAndReceiveSASLPlainAuth()
			if b.connErr != nil {
				err = b.conn.Close()
				if err == nil {
					Logger.Printf("Closed connection to broker %s\n", b.addr)
				} else {
					Logger.Printf("Error while closing connection to broker %s: %s\n", b.addr, err)
				}
				b.conn = nil
				atomic.StoreInt32(&b.opened, 0)
				return
			}
		}

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

	if err == nil {
		Logger.Printf("Closed connection to broker %s\n", b.addr)
	} else {
		Logger.Printf("Error while closing connection to broker %s: %s\n", b.addr, err)
	}

	atomic.StoreInt32(&b.opened, 0)

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

func (b *Broker) JoinGroup(request *JoinGroupRequest) (*JoinGroupResponse, error) {
	response := new(JoinGroupResponse)

	err := b.sendAndReceive(request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) SyncGroup(request *SyncGroupRequest) (*SyncGroupResponse, error) {
	response := new(SyncGroupResponse)

	err := b.sendAndReceive(request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) LeaveGroup(request *LeaveGroupRequest) (*LeaveGroupResponse, error) {
	response := new(LeaveGroupResponse)

	err := b.sendAndReceive(request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) Heartbeat(request *HeartbeatRequest) (*HeartbeatResponse, error) {
	response := new(HeartbeatResponse)

	err := b.sendAndReceive(request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) ListGroups(request *ListGroupsRequest) (*ListGroupsResponse, error) {
	response := new(ListGroupsResponse)

	err := b.sendAndReceive(request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) DescribeGroups(request *DescribeGroupsRequest) (*DescribeGroupsResponse, error) {
	response := new(DescribeGroupsResponse)

	err := b.sendAndReceive(request, response)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (b *Broker) send(rb protocolBody, promiseResponse bool) (*responsePromise, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.conn == nil {
		if b.connErr != nil {
			return nil, b.connErr
		}
		return nil, ErrNotConnected
	}

	if !b.conf.Version.IsAtLeast(rb.requiredVersion()) {
		return nil, ErrUnsupportedVersion
	}

	req := &request{correlationID: b.correlationID, clientID: b.conf.ClientID, body: rb}
	buf, err := encode(req, b.conf.MetricRegistry)
	if err != nil {
		return nil, err
	}

	err = b.conn.SetWriteDeadline(time.Now().Add(b.conf.Net.WriteTimeout))
	if err != nil {
		return nil, err
	}

	requestTime := time.Now()
	bytes, err := b.conn.Write(buf)
	b.updateOutgoingCommunicationMetrics(bytes)
	if err != nil {
		return nil, err
	}
	b.correlationID++

	if !promiseResponse {
		// Record request latency without the response
		b.updateRequestLatencyMetrics(time.Since(requestTime))
		return nil, nil
	}

	promise := responsePromise{requestTime, req.correlationID, make(chan []byte), make(chan error)}
	b.responses <- promise

	return &promise, nil
}

func (b *Broker) sendAndReceive(req protocolBody, res versionedDecoder) error {
	promise, err := b.send(req, res != nil)

	if err != nil {
		return err
	}

	if promise == nil {
		return nil
	}

	select {
	case buf := <-promise.packets:
		return versionedDecode(buf, res, req.version())
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
	var dead error
	header := make([]byte, 8)
	for response := range b.responses {
		if dead != nil {
			response.errors <- dead
			continue
		}

		err := b.conn.SetReadDeadline(time.Now().Add(b.conf.Net.ReadTimeout))
		if err != nil {
			dead = err
			response.errors <- err
			continue
		}

		bytesReadHeader, err := io.ReadFull(b.conn, header)
		requestLatency := time.Since(response.requestTime)
		if err != nil {
			b.updateIncomingCommunicationMetrics(bytesReadHeader, requestLatency)
			dead = err
			response.errors <- err
			continue
		}

		decodedHeader := responseHeader{}
		err = decode(header, &decodedHeader)
		if err != nil {
			b.updateIncomingCommunicationMetrics(bytesReadHeader, requestLatency)
			dead = err
			response.errors <- err
			continue
		}
		if decodedHeader.correlationID != response.correlationID {
			b.updateIncomingCommunicationMetrics(bytesReadHeader, requestLatency)
			// TODO if decoded ID < cur ID, discard until we catch up
			// TODO if decoded ID > cur ID, save it so when cur ID catches up we have a response
			dead = PacketDecodingError{fmt.Sprintf("correlation ID didn't match, wanted %d, got %d", response.correlationID, decodedHeader.correlationID)}
			response.errors <- dead
			continue
		}

		buf := make([]byte, decodedHeader.length-4)
		bytesReadBody, err := io.ReadFull(b.conn, buf)
		b.updateIncomingCommunicationMetrics(bytesReadHeader+bytesReadBody, requestLatency)
		if err != nil {
			dead = err
			response.errors <- err
			continue
		}

		response.packets <- buf
	}
	close(b.done)
}

func (b *Broker) sendAndReceiveSASLPlainHandshake() error {
	rb := &SaslHandshakeRequest{"PLAIN"}
	req := &request{correlationID: b.correlationID, clientID: b.conf.ClientID, body: rb}
	buf, err := encode(req, b.conf.MetricRegistry)
	if err != nil {
		return err
	}

	err = b.conn.SetWriteDeadline(time.Now().Add(b.conf.Net.WriteTimeout))
	if err != nil {
		return err
	}

	requestTime := time.Now()
	bytes, err := b.conn.Write(buf)
	b.updateOutgoingCommunicationMetrics(bytes)
	if err != nil {
		Logger.Printf("Failed to send SASL handshake %s: %s\n", b.addr, err.Error())
		return err
	}
	b.correlationID++
	//wait for the response
	header := make([]byte, 8) // response header
	_, err = io.ReadFull(b.conn, header)
	if err != nil {
		Logger.Printf("Failed to read SASL handshake header : %s\n", err.Error())
		return err
	}
	length := binary.BigEndian.Uint32(header[:4])
	payload := make([]byte, length-4)
	n, err := io.ReadFull(b.conn, payload)
	if err != nil {
		Logger.Printf("Failed to read SASL handshake payload : %s\n", err.Error())
		return err
	}
	b.updateIncomingCommunicationMetrics(n+8, time.Since(requestTime))
	res := &SaslHandshakeResponse{}
	err = versionedDecode(payload, res, 0)
	if err != nil {
		Logger.Printf("Failed to parse SASL handshake : %s\n", err.Error())
		return err
	}
	if res.Err != ErrNoError {
		Logger.Printf("Invalid SASL Mechanism : %s\n", res.Err.Error())
		return res.Err
	}
	Logger.Print("Successful SASL handshake")
	return nil
}

// Kafka 0.10.0 plans to support SASL Plain and Kerberos as per PR #812 (KIP-43)/(JIRA KAFKA-3149)
// Some hosted kafka services such as IBM Message Hub already offer SASL/PLAIN auth with Kafka 0.9
//
// In SASL Plain, Kafka expects the auth header to be in the following format
// Message format (from https://tools.ietf.org/html/rfc4616):
//
//   message   = [authzid] UTF8NUL authcid UTF8NUL passwd
//   authcid   = 1*SAFE ; MUST accept up to 255 octets
//   authzid   = 1*SAFE ; MUST accept up to 255 octets
//   passwd    = 1*SAFE ; MUST accept up to 255 octets
//   UTF8NUL   = %x00 ; UTF-8 encoded NUL character
//
//   SAFE      = UTF1 / UTF2 / UTF3 / UTF4
//                  ;; any UTF-8 encoded Unicode character except NUL
//
// When credentials are valid, Kafka returns a 4 byte array of null characters.
// When credentials are invalid, Kafka closes the connection. This does not seem to be the ideal way
// of responding to bad credentials but thats how its being done today.
func (b *Broker) sendAndReceiveSASLPlainAuth() error {
	if b.conf.Net.SASL.Handshake {
		handshakeErr := b.sendAndReceiveSASLPlainHandshake()
		if handshakeErr != nil {
			Logger.Printf("Error while performing SASL handshake %s\n", b.addr)
			return handshakeErr
		}
	}
	length := 1 + len(b.conf.Net.SASL.User) + 1 + len(b.conf.Net.SASL.Password)
	authBytes := make([]byte, length+4) //4 byte length header + auth data
	binary.BigEndian.PutUint32(authBytes, uint32(length))
	copy(authBytes[4:], []byte("\x00"+b.conf.Net.SASL.User+"\x00"+b.conf.Net.SASL.Password))

	err := b.conn.SetWriteDeadline(time.Now().Add(b.conf.Net.WriteTimeout))
	if err != nil {
		Logger.Printf("Failed to set write deadline when doing SASL auth with broker %s: %s\n", b.addr, err.Error())
		return err
	}

	requestTime := time.Now()
	bytesWritten, err := b.conn.Write(authBytes)
	b.updateOutgoingCommunicationMetrics(bytesWritten)
	if err != nil {
		Logger.Printf("Failed to write SASL auth header to broker %s: %s\n", b.addr, err.Error())
		return err
	}

	header := make([]byte, 4)
	n, err := io.ReadFull(b.conn, header)
	b.updateIncomingCommunicationMetrics(n, time.Since(requestTime))
	// If the credentials are valid, we would get a 4 byte response filled with null characters.
	// Otherwise, the broker closes the connection and we get an EOF
	if err != nil {
		Logger.Printf("Failed to read response while authenticating with SASL to broker %s: %s\n", b.addr, err.Error())
		return err
	}

	Logger.Printf("SASL authentication successful with broker %s:%v - %v\n", b.addr, n, header)
	return nil
}

func (b *Broker) updateIncomingCommunicationMetrics(bytes int, requestLatency time.Duration) {
	b.updateRequestLatencyMetrics(requestLatency)
	b.responseRate.Mark(1)
	if b.brokerResponseRate != nil {
		b.brokerResponseRate.Mark(1)
	}
	responseSize := int64(bytes)
	b.incomingByteRate.Mark(responseSize)
	if b.brokerIncomingByteRate != nil {
		b.brokerIncomingByteRate.Mark(responseSize)
	}
	b.responseSize.Update(responseSize)
	if b.brokerResponseSize != nil {
		b.brokerResponseSize.Update(responseSize)
	}
}

func (b *Broker) updateRequestLatencyMetrics(requestLatency time.Duration) {
	requestLatencyInMs := int64(requestLatency / time.Millisecond)
	b.requestLatency.Update(requestLatencyInMs)
	if b.brokerRequestLatency != nil {
		b.brokerRequestLatency.Update(requestLatencyInMs)
	}
}

func (b *Broker) updateOutgoingCommunicationMetrics(bytes int) {
	b.requestRate.Mark(1)
	if b.brokerRequestRate != nil {
		b.brokerRequestRate.Mark(1)
	}
	requestSize := int64(bytes)
	b.outgoingByteRate.Mark(requestSize)
	if b.brokerOutgoingByteRate != nil {
		b.brokerOutgoingByteRate.Mark(requestSize)
	}
	b.requestSize.Update(requestSize)
	if b.brokerRequestSize != nil {
		b.brokerRequestSize.Update(requestSize)
	}
}

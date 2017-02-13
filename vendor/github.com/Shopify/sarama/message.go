package sarama

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/eapache/go-xerial-snappy"
	"github.com/pierrec/lz4"
)

// CompressionCodec represents the various compression codecs recognized by Kafka in messages.
type CompressionCodec int8

// only the last two bits are really used
const compressionCodecMask int8 = 0x03

const (
	CompressionNone   CompressionCodec = 0
	CompressionGZIP   CompressionCodec = 1
	CompressionSnappy CompressionCodec = 2
	CompressionLZ4    CompressionCodec = 3
)

type Message struct {
	Codec     CompressionCodec // codec used to compress the message contents
	Key       []byte           // the message key, may be nil
	Value     []byte           // the message contents
	Set       *MessageSet      // the message set a message might wrap
	Version   int8             // v1 requires Kafka 0.10
	Timestamp time.Time        // the timestamp of the message (version 1+ only)

	compressedCache []byte
	compressedSize  int // used for computing the compression ratio metrics
}

func (m *Message) encode(pe packetEncoder) error {
	pe.push(&crc32Field{})

	pe.putInt8(m.Version)

	attributes := int8(m.Codec) & compressionCodecMask
	pe.putInt8(attributes)

	if m.Version >= 1 {
		pe.putInt64(m.Timestamp.UnixNano() / int64(time.Millisecond))
	}

	err := pe.putBytes(m.Key)
	if err != nil {
		return err
	}

	var payload []byte

	if m.compressedCache != nil {
		payload = m.compressedCache
		m.compressedCache = nil
	} else if m.Value != nil {
		switch m.Codec {
		case CompressionNone:
			payload = m.Value
		case CompressionGZIP:
			var buf bytes.Buffer
			writer := gzip.NewWriter(&buf)
			if _, err = writer.Write(m.Value); err != nil {
				return err
			}
			if err = writer.Close(); err != nil {
				return err
			}
			m.compressedCache = buf.Bytes()
			payload = m.compressedCache
		case CompressionSnappy:
			tmp := snappy.Encode(m.Value)
			m.compressedCache = tmp
			payload = m.compressedCache
		case CompressionLZ4:
			var buf bytes.Buffer
			writer := lz4.NewWriter(&buf)
			if _, err = writer.Write(m.Value); err != nil {
				return err
			}
			if err = writer.Close(); err != nil {
				return err
			}
			m.compressedCache = buf.Bytes()
			payload = m.compressedCache

		default:
			return PacketEncodingError{fmt.Sprintf("unsupported compression codec (%d)", m.Codec)}
		}
		// Keep in mind the compressed payload size for metric gathering
		m.compressedSize = len(payload)
	}

	if err = pe.putBytes(payload); err != nil {
		return err
	}

	return pe.pop()
}

func (m *Message) decode(pd packetDecoder) (err error) {
	err = pd.push(&crc32Field{})
	if err != nil {
		return err
	}

	m.Version, err = pd.getInt8()
	if err != nil {
		return err
	}

	attribute, err := pd.getInt8()
	if err != nil {
		return err
	}
	m.Codec = CompressionCodec(attribute & compressionCodecMask)

	if m.Version >= 1 {
		millis, err := pd.getInt64()
		if err != nil {
			return err
		}
		m.Timestamp = time.Unix(millis/1000, (millis%1000)*int64(time.Millisecond))
	}

	m.Key, err = pd.getBytes()
	if err != nil {
		return err
	}

	m.Value, err = pd.getBytes()
	if err != nil {
		return err
	}

	// Required for deep equal assertion during tests but might be useful
	// for future metrics about the compression ratio in fetch requests
	m.compressedSize = len(m.Value)

	switch m.Codec {
	case CompressionNone:
		// nothing to do
	case CompressionGZIP:
		if m.Value == nil {
			break
		}
		reader, err := gzip.NewReader(bytes.NewReader(m.Value))
		if err != nil {
			return err
		}
		if m.Value, err = ioutil.ReadAll(reader); err != nil {
			return err
		}
		if err := m.decodeSet(); err != nil {
			return err
		}
	case CompressionSnappy:
		if m.Value == nil {
			break
		}
		if m.Value, err = snappy.Decode(m.Value); err != nil {
			return err
		}
		if err := m.decodeSet(); err != nil {
			return err
		}
	case CompressionLZ4:
		if m.Value == nil {
			break
		}
		reader := lz4.NewReader(bytes.NewReader(m.Value))
		if m.Value, err = ioutil.ReadAll(reader); err != nil {
			return err
		}
		if err := m.decodeSet(); err != nil {
			return err
		}

	default:
		return PacketDecodingError{fmt.Sprintf("invalid compression specified (%d)", m.Codec)}
	}

	return pd.pop()
}

// decodes a message set from a previousy encoded bulk-message
func (m *Message) decodeSet() (err error) {
	pd := realDecoder{raw: m.Value}
	m.Set = &MessageSet{}
	return m.Set.decode(&pd)
}

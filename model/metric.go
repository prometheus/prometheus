package model

import (
	"crypto/md5"
	"encoding/hex"
	"io"
	"time"
)

type Fingerprint string

type LabelPairs map[string]string
type Metric map[string]string

type SampleValue float32

type Sample struct {
	Labels    LabelPairs
	Value     SampleValue
	Timestamp time.Time
}

type Samples struct {
	Value     SampleValue
	Timestamp time.Time
}

type Interval struct {
	OldestInclusive time.Time
	NewestInclusive time.Time
}

func FingerprintFromString(value string) Fingerprint {
	hash := md5.New()
	io.WriteString(hash, value)
	return Fingerprint(hex.EncodeToString(hash.Sum([]byte{})))
}

func FingerprintFromByteArray(value []byte) Fingerprint {
	hash := md5.New()
	hash.Write(value)
	return Fingerprint(hex.EncodeToString(hash.Sum([]byte{})))
}

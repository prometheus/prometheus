package model

// RemoteWriteProcessingStatus status of processing RemoteWrite.
type RemoteWriteProcessingStatus struct {
	Code    int
	Message string
}

// RemoteWriteBuffer buffer []byte for protobuf.
type RemoteWriteBuffer struct {
	b         []byte
	destroyFn func()
}

// NewRemoteWriteBuffer init new RemoteWriteBuffer.
func NewRemoteWriteBuffer(b *[]byte, destroyFn func()) *RemoteWriteBuffer {
	return &RemoteWriteBuffer{
		b:         *b,
		destroyFn: destroyFn,
	}
}

// Bytes returns a slice bytes.
func (rwb *RemoteWriteBuffer) Bytes() []byte {
	return rwb.b
}

// Destroy destroy bufferSnappy, return to pool.
func (rwb *RemoteWriteBuffer) Destroy() {
	rwb.destroyFn()
}

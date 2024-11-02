package agent

// RemoteWrite implement remote write.
type RemoteWrite interface {
	// LowestSentTimestamp returns the lowest sent timestamp across all queues.
	LowestSentTimestamp() int64
}

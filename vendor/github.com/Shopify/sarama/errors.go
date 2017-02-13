package sarama

import (
	"errors"
	"fmt"
)

// ErrOutOfBrokers is the error returned when the client has run out of brokers to talk to because all of them errored
// or otherwise failed to respond.
var ErrOutOfBrokers = errors.New("kafka: client has run out of available brokers to talk to (Is your cluster reachable?)")

// ErrClosedClient is the error returned when a method is called on a client that has been closed.
var ErrClosedClient = errors.New("kafka: tried to use a client that was closed")

// ErrIncompleteResponse is the error returned when the server returns a syntactically valid response, but it does
// not contain the expected information.
var ErrIncompleteResponse = errors.New("kafka: response did not contain all the expected topic/partition blocks")

// ErrInvalidPartition is the error returned when a partitioner returns an invalid partition index
// (meaning one outside of the range [0...numPartitions-1]).
var ErrInvalidPartition = errors.New("kafka: partitioner returned an invalid partition index")

// ErrAlreadyConnected is the error returned when calling Open() on a Broker that is already connected or connecting.
var ErrAlreadyConnected = errors.New("kafka: broker connection already initiated")

// ErrNotConnected is the error returned when trying to send or call Close() on a Broker that is not connected.
var ErrNotConnected = errors.New("kafka: broker not connected")

// ErrInsufficientData is returned when decoding and the packet is truncated. This can be expected
// when requesting messages, since as an optimization the server is allowed to return a partial message at the end
// of the message set.
var ErrInsufficientData = errors.New("kafka: insufficient data to decode packet, more bytes expected")

// ErrShuttingDown is returned when a producer receives a message during shutdown.
var ErrShuttingDown = errors.New("kafka: message received by producer in process of shutting down")

// ErrMessageTooLarge is returned when the next message to consume is larger than the configured Consumer.Fetch.Max
var ErrMessageTooLarge = errors.New("kafka: message is larger than Consumer.Fetch.Max")

// PacketEncodingError is returned from a failure while encoding a Kafka packet. This can happen, for example,
// if you try to encode a string over 2^15 characters in length, since Kafka's encoding rules do not permit that.
type PacketEncodingError struct {
	Info string
}

func (err PacketEncodingError) Error() string {
	return fmt.Sprintf("kafka: error encoding packet: %s", err.Info)
}

// PacketDecodingError is returned when there was an error (other than truncated data) decoding the Kafka broker's response.
// This can be a bad CRC or length field, or any other invalid value.
type PacketDecodingError struct {
	Info string
}

func (err PacketDecodingError) Error() string {
	return fmt.Sprintf("kafka: error decoding packet: %s", err.Info)
}

// ConfigurationError is the type of error returned from a constructor (e.g. NewClient, or NewConsumer)
// when the specified configuration is invalid.
type ConfigurationError string

func (err ConfigurationError) Error() string {
	return "kafka: invalid configuration (" + string(err) + ")"
}

// KError is the type of error that can be returned directly by the Kafka broker.
// See https://cwiki.apache.org/confluence/display/KAFKA/A+Guide+To+The+Kafka+Protocol#AGuideToTheKafkaProtocol-ErrorCodes
type KError int16

// Numeric error codes returned by the Kafka server.
const (
	ErrNoError                         KError = 0
	ErrUnknown                         KError = -1
	ErrOffsetOutOfRange                KError = 1
	ErrInvalidMessage                  KError = 2
	ErrUnknownTopicOrPartition         KError = 3
	ErrInvalidMessageSize              KError = 4
	ErrLeaderNotAvailable              KError = 5
	ErrNotLeaderForPartition           KError = 6
	ErrRequestTimedOut                 KError = 7
	ErrBrokerNotAvailable              KError = 8
	ErrReplicaNotAvailable             KError = 9
	ErrMessageSizeTooLarge             KError = 10
	ErrStaleControllerEpochCode        KError = 11
	ErrOffsetMetadataTooLarge          KError = 12
	ErrNetworkException                KError = 13
	ErrOffsetsLoadInProgress           KError = 14
	ErrConsumerCoordinatorNotAvailable KError = 15
	ErrNotCoordinatorForConsumer       KError = 16
	ErrInvalidTopic                    KError = 17
	ErrMessageSetSizeTooLarge          KError = 18
	ErrNotEnoughReplicas               KError = 19
	ErrNotEnoughReplicasAfterAppend    KError = 20
	ErrInvalidRequiredAcks             KError = 21
	ErrIllegalGeneration               KError = 22
	ErrInconsistentGroupProtocol       KError = 23
	ErrInvalidGroupId                  KError = 24
	ErrUnknownMemberId                 KError = 25
	ErrInvalidSessionTimeout           KError = 26
	ErrRebalanceInProgress             KError = 27
	ErrInvalidCommitOffsetSize         KError = 28
	ErrTopicAuthorizationFailed        KError = 29
	ErrGroupAuthorizationFailed        KError = 30
	ErrClusterAuthorizationFailed      KError = 31
	ErrInvalidTimestamp                KError = 32
	ErrUnsupportedSASLMechanism        KError = 33
	ErrIllegalSASLState                KError = 34
	ErrUnsupportedVersion              KError = 35
	ErrUnsupportedForMessageFormat     KError = 43
)

func (err KError) Error() string {
	// Error messages stolen/adapted from
	// https://cwiki.apache.org/confluence/display/KAFKA/A+Guide+To+The+Kafka+Protocol
	switch err {
	case ErrNoError:
		return "kafka server: Not an error, why are you printing me?"
	case ErrUnknown:
		return "kafka server: Unexpected (unknown?) server error."
	case ErrOffsetOutOfRange:
		return "kafka server: The requested offset is outside the range of offsets maintained by the server for the given topic/partition."
	case ErrInvalidMessage:
		return "kafka server: Message contents does not match its CRC."
	case ErrUnknownTopicOrPartition:
		return "kafka server: Request was for a topic or partition that does not exist on this broker."
	case ErrInvalidMessageSize:
		return "kafka server: The message has a negative size."
	case ErrLeaderNotAvailable:
		return "kafka server: In the middle of a leadership election, there is currently no leader for this partition and hence it is unavailable for writes."
	case ErrNotLeaderForPartition:
		return "kafka server: Tried to send a message to a replica that is not the leader for some partition. Your metadata is out of date."
	case ErrRequestTimedOut:
		return "kafka server: Request exceeded the user-specified time limit in the request."
	case ErrBrokerNotAvailable:
		return "kafka server: Broker not available. Not a client facing error, we should never receive this!!!"
	case ErrReplicaNotAvailable:
		return "kafka server: Replica information not available, one or more brokers are down."
	case ErrMessageSizeTooLarge:
		return "kafka server: Message was too large, server rejected it to avoid allocation error."
	case ErrStaleControllerEpochCode:
		return "kafka server: StaleControllerEpochCode (internal error code for broker-to-broker communication)."
	case ErrOffsetMetadataTooLarge:
		return "kafka server: Specified a string larger than the configured maximum for offset metadata."
	case ErrNetworkException:
		return "kafka server: The server disconnected before a response was received."
	case ErrOffsetsLoadInProgress:
		return "kafka server: The broker is still loading offsets after a leader change for that offset's topic partition."
	case ErrConsumerCoordinatorNotAvailable:
		return "kafka server: Offset's topic has not yet been created."
	case ErrNotCoordinatorForConsumer:
		return "kafka server: Request was for a consumer group that is not coordinated by this broker."
	case ErrInvalidTopic:
		return "kafka server: The request attempted to perform an operation on an invalid topic."
	case ErrMessageSetSizeTooLarge:
		return "kafka server: The request included message batch larger than the configured segment size on the server."
	case ErrNotEnoughReplicas:
		return "kafka server: Messages are rejected since there are fewer in-sync replicas than required."
	case ErrNotEnoughReplicasAfterAppend:
		return "kafka server: Messages are written to the log, but to fewer in-sync replicas than required."
	case ErrInvalidRequiredAcks:
		return "kafka server: The number of required acks is invalid (should be either -1, 0, or 1)."
	case ErrIllegalGeneration:
		return "kafka server: The provided generation id is not the current generation."
	case ErrInconsistentGroupProtocol:
		return "kafka server: The provider group protocol type is incompatible with the other members."
	case ErrInvalidGroupId:
		return "kafka server: The provided group id was empty."
	case ErrUnknownMemberId:
		return "kafka server: The provided member is not known in the current generation."
	case ErrInvalidSessionTimeout:
		return "kafka server: The provided session timeout is outside the allowed range."
	case ErrRebalanceInProgress:
		return "kafka server: A rebalance for the group is in progress. Please re-join the group."
	case ErrInvalidCommitOffsetSize:
		return "kafka server: The provided commit metadata was too large."
	case ErrTopicAuthorizationFailed:
		return "kafka server: The client is not authorized to access this topic."
	case ErrGroupAuthorizationFailed:
		return "kafka server: The client is not authorized to access this group."
	case ErrClusterAuthorizationFailed:
		return "kafka server: The client is not authorized to send this request type."
	case ErrInvalidTimestamp:
		return "kafka server: The timestamp of the message is out of acceptable range."
	case ErrUnsupportedSASLMechanism:
		return "kafka server: The broker does not support the requested SASL mechanism."
	case ErrIllegalSASLState:
		return "kafka server: Request is not valid given the current SASL state."
	case ErrUnsupportedVersion:
		return "kafka server: The version of API is not supported."
	case ErrUnsupportedForMessageFormat:
		return "kafka server: The requested operation is not supported by the message format version."
	}

	return fmt.Sprintf("Unknown error, how did this happen? Error code = %d", err)
}

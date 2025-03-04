package frames

// Protocol versions
const (
	UnknownProtocolVersion uint8 = iota
	ProtocolVersion1
	ProtocolVersion2
	ProtocolVersion3
	ProtocolVersion4
)

const (
	// constant size of type
	sizeOfTypeFrame = 1
	sizeOfUint8     = 1
	sizeOfUint16    = 2
	sizeOfUint32    = 4
	sizeOfUint64    = 8
	sizeOfUUID      = 16

	// default version
	defaultVersion uint8 = ProtocolVersion4
	// magic byte for header
	magicByte byte = 165
)

// Content versions
const (
	UnknownContentVersion uint8 = iota
	ContentVersion1
	ContentVersion2
)

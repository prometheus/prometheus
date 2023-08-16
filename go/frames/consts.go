package frames

const (
	// constant size of type
	sizeOfTypeFrame = 1
	sizeOfUint8     = 1
	sizeOfUint16    = 2
	sizeOfUint32    = 4
	sizeOfUint64    = 8
	sizeOfUUID      = 16

	// default version
	defaultVersion uint8 = 3
	// magic byte for header
	magicByte byte = 165
)

package coding

type Encoder interface {
	Encode() ([]byte, error)
}

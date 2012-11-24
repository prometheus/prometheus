package main

type Encoder interface {
	Encode() ([]byte, error)
}

package main

type Pair struct {
	Left  []byte
	Right []byte
}

type Persistence interface {
	Has(key Encoder) (bool, error)
	Get(key Encoder) ([]byte, error)
	GetAll() ([]Pair, error)
	Drop(key Encoder) error
	Put(key Encoder, value Encoder) error
	Close() error
}

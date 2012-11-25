package main

import (
	"github.com/matttproud/prometheus/coding"
)

type Pair struct {
	Left  []byte
	Right []byte
}

type Persistence interface {
	Has(key coding.Encoder) (bool, error)
	Get(key coding.Encoder) ([]byte, error)
	GetAll() ([]Pair, error)
	Drop(key coding.Encoder) error
	Put(key, value coding.Encoder) error
	Close() error
}

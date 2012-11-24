package main

type MembershipIndex interface {
	Has(key Encoder) (bool, error)
	Put(key Encoder) error
	Drop(key Encoder) error
	Close() error
}

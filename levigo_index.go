package main

import (
	"github.com/matttproud/prometheus/coding"
	data "github.com/matttproud/prometheus/model/generated"
)

var (
	existenceValue = coding.NewProtocolBufferEncoder(&data.MembershipIndexValueDDO{})
)

type LevigoMembershipIndex struct {
	persistence *LevigoPersistence
}

func (l *LevigoMembershipIndex) Close() error {
	return l.persistence.Close()
}

func (l *LevigoMembershipIndex) Has(key coding.Encoder) (bool, error) {
	return l.persistence.Has(key)
}

func (l *LevigoMembershipIndex) Drop(key coding.Encoder) error {
	return l.persistence.Drop(key)
}

func (l *LevigoMembershipIndex) Put(key coding.Encoder) error {
	return l.persistence.Put(key, existenceValue)
}

func NewLevigoMembershipIndex(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (*LevigoMembershipIndex, error) {
	var levigoPersistence *LevigoPersistence
	var levigoPersistenceError error

	if levigoPersistence, levigoPersistenceError = NewLevigoPersistence(storageRoot, cacheCapacity, bitsPerBloomFilterEncoded); levigoPersistenceError == nil {
		levigoMembershipIndex := &LevigoMembershipIndex{
			persistence: levigoPersistence,
		}
		return levigoMembershipIndex, nil
	}

	return nil, levigoPersistenceError
}

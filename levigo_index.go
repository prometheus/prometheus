package main

import (
	data "github.com/matttproud/prometheus/model/generated"
)

type LevigoMembershipIndex struct {
	persistence    *LevigoPersistence
	existenceValue Encoder
}

func (l *LevigoMembershipIndex) Close() error {
	return l.persistence.Close()
}

func (l *LevigoMembershipIndex) Has(key Encoder) (bool, error) {
	return l.persistence.Has(key)
}

func (l *LevigoMembershipIndex) Drop(key Encoder) error {
	return l.persistence.Drop(key)
}

func (l *LevigoMembershipIndex) Put(key Encoder) error {
	return l.persistence.Put(key, l.existenceValue)
}

func NewLevigoMembershipIndex(storageRoot string, cacheCapacity, bitsPerBloomFilterEncoded int) (*LevigoMembershipIndex, error) {
	var levigoPersistence *LevigoPersistence
	var levigoPersistenceError error

	existenceValue := NewProtocolBufferEncoder(&data.MembershipIndexValueDDO{})

	if levigoPersistence, levigoPersistenceError = NewLevigoPersistence(storageRoot, cacheCapacity, bitsPerBloomFilterEncoded); levigoPersistenceError == nil {
		levigoMembershipIndex := &LevigoMembershipIndex{
			persistence:    levigoPersistence,
			existenceValue: existenceValue,
		}
		return levigoMembershipIndex, nil
	}

	return nil, levigoPersistenceError
}

// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package datastore

import (
	"fmt"

	pb "google.golang.org/genproto/googleapis/datastore/v1"
)

// A Mutation represents a change to a Datastore entity.
type Mutation struct {
	key *Key // needed for transaction PendingKeys and to dedup deletions
	mut *pb.Mutation
	err error
}

func (m *Mutation) isDelete() bool {
	_, ok := m.mut.Operation.(*pb.Mutation_Delete)
	return ok
}

// NewInsert creates a mutation that will save the entity src into the datastore with
// key k, returning an error if k already exists.
// See Client.Put for valid values of src.
func NewInsert(k *Key, src interface{}) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	p, err := saveEntity(k, src)
	if err != nil {
		return &Mutation{err: err}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Insert{Insert: p}},
	}
}

// NewUpsert creates a mutation that saves the entity src into the datastore with key
// k, whether or not k exists. See Client.Put for valid values of src.
func NewUpsert(k *Key, src interface{}) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	p, err := saveEntity(k, src)
	if err != nil {
		return &Mutation{err: err}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Upsert{Upsert: p}},
	}
}

// NewUpdate creates a mutation that replaces the entity in the datastore with key k,
// returning an error if k does not exist. See Client.Put for valid values of src.
func NewUpdate(k *Key, src interface{}) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	if k.Incomplete() {
		return &Mutation{err: fmt.Errorf("datastore: can't update the incomplete key: %v", k)}
	}
	p, err := saveEntity(k, src)
	if err != nil {
		return &Mutation{err: err}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Update{Update: p}},
	}
}

// NewDelete creates a mutation that deletes the entity with key k.
func NewDelete(k *Key) *Mutation {
	if !k.valid() {
		return &Mutation{err: ErrInvalidKey}
	}
	if k.Incomplete() {
		return &Mutation{err: fmt.Errorf("datastore: can't delete the incomplete key: %v", k)}
	}
	return &Mutation{
		key: k,
		mut: &pb.Mutation{Operation: &pb.Mutation_Delete{Delete: keyToProto(k)}},
	}
}

func mutationProtos(muts []*Mutation) ([]*pb.Mutation, error) {
	// If any of the mutations have errors, collect and return them.
	var merr MultiError
	for i, m := range muts {
		if m.err != nil {
			if merr == nil {
				merr = make(MultiError, len(muts))
			}
			merr[i] = m.err
		}
	}
	if merr != nil {
		return nil, merr
	}
	var protos []*pb.Mutation
	// Collect protos. Remove duplicate deletions (see deleteMutations).
	seen := map[string]bool{}
	for _, m := range muts {
		if m.isDelete() {
			ks := m.key.String()
			if seen[ks] {
				continue
			}
			seen[ks] = true
		}
		protos = append(protos, m.mut)
	}
	return protos, nil
}

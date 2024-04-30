// Copyright 2024 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alertstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/rules"
)

const (
	// Default BadgerDB discardRatio. It represents the discard ratio for the
	// BadgerDB GC.
	//
	// Ref: https://godoc.org/github.com/dgraph-io/badger#DB.RunValueLogGC
	badgerDiscardRatio = 0.5

	// Default BadgerDB GC interval.
	badgerGCInterval = 2 * time.Minute
)

type BadgerDB struct {
	db         *badger.DB
	logger     log.Logger
	ctx        context.Context
	cancelFunc context.CancelFunc
	ttl        time.Duration
}

// NewBadgerDB returns a new initialized BadgerDB database implementing the AlertStore
// interface. If the database cannot be initialized, an error will be returned.
func NewBadgerDB(dataDir string, logger log.Logger, ttl time.Duration) (*BadgerDB, error) {
	if err := os.MkdirAll(dataDir, 0774); err != nil {
		return nil, err
	}

	opts := badger.DefaultOptions(dataDir)

	badgerDB, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	bdb := &BadgerDB{
		db:     badgerDB,
		logger: logger,
		ttl:    ttl,
	}
	bdb.ctx, bdb.cancelFunc = context.WithCancel(context.Background())

	go bdb.runGC()
	return bdb, nil
}

// SetAlerts stores the alerts for a rule. Returns an error if storing fails.
// A rule is uniquely identified using a combination of groupKey,
// rule name and order it appears in the group.
func (bdb *BadgerDB) SetAlerts(groupKey string, rule string, ruleOrder int, alerts []*rules.Alert) error {
	key := getKey(groupKey, rule, ruleOrder)
	value, err := json.Marshal(alerts)
	if err != nil {
		return err
	}
	err = bdb.set(key, value)
	if err != nil {
		return err
	}
	level.Debug(bdb.logger).Log("msg", "saved alert to store", "key", string(key))
	return nil
}

// GetAlerts returns all the alerts for each rule in the group, returns error if
// it encounters failures while retrieving from store.
// Each alert list belongs to a ruleKey (rule+ruleOrder).
func (bdb *BadgerDB) GetAlerts(groupKey string) (map[string][]*rules.Alert, error) {
	allAlerts := make(map[string][]*rules.Alert)
	err := bdb.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(groupKey)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			err := item.Value(func(v []byte) error {
				ruleKey, err := parseKey(k)
				if err != nil {
					return err
				}
				var alerts []*rules.Alert
				err = json.Unmarshal(v, &alerts)
				if err != nil {
					return err
				}

				allAlerts[ruleKey] = alerts
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return allAlerts, nil
}

// Close implements the DB interface. It closes the connection to the underlying
// BadgerDB database as well as invoking the context's cancel function.
func (bdb *BadgerDB) Close() {
	bdb.cancelFunc()
	err := bdb.db.Close()
	if err != nil {
		level.Error(bdb.logger).Log("msg", "Unable to close badgerDB", "err", err)
	}
}

// runGC triggers the garbage collection for the BadgerDB backend database. It
// should be run in a goroutine.
func (bdb *BadgerDB) runGC() {
	ticker := time.NewTicker(badgerGCInterval)
	for {
		select {
		case <-ticker.C:
			err := bdb.db.RunValueLogGC(badgerDiscardRatio)
			if err != nil {
				// GC didn't result in any cleanup
				if errors.Is(err, badger.ErrNoRewrite) {
					level.Debug(bdb.logger).Log("msg", "no BadgerDB GC occurred", "err", err)
				} else {
					level.Warn(bdb.logger).Log("msg", "failed to GC BadgerDB", "err", err)
				}
			}
		case <-bdb.ctx.Done():
			return
		}
	}
}

// set stores a value for a given key.
// If the key/value pair cannot be saved, an error is returned.
func (bdb *BadgerDB) set(key, value []byte) error {
	err := bdb.db.Update(func(txn *badger.Txn) error {
		e := badger.NewEntry(key, value).WithTTL(bdb.ttl)
		return txn.SetEntry(e)
	})
	if err != nil {
		level.Error(bdb.logger).Log("msg", "failed to set key", "key", key, "err", err)
		return err
	}

	return nil
}

// parses rule name from key.
func parseKey(k []byte) (string, error) {
	// key=file;group;ruleKey.
	key := string(k)
	res := strings.Split(key, ";")
	if len(res) != 3 {
		return "", fmt.Errorf("unexpected format for key: %s", key)
	}
	return res[2], nil
}

// key = file;group;ruleName+ruleOrder.
func getKey(groupKey string, rule string, ruleOrder int) []byte {
	return []byte(groupKey + ";" + rule + strconv.Itoa(ruleOrder))
}

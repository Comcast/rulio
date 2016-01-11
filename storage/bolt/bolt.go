// Copyright 2015 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// End Copyright

package bolt

import (
	"os"
	"time"

	"github.com/boltdb/bolt"

	. "github.com/Comcast/rulio/core"
)

// BoltStorage implements Storage using boltdb
//
// This name stutters because it's convenient to dot-import core,
// which defines 'Storage'.
type BoltStorage struct {
	db       *bolt.DB
	Filename string
}

var DefaultOptions = &bolt.Options{
	Timeout: 5 * time.Second,
}

// NewStorage returns a BoltStorage based Storage
func NewStorage(ctx *Context, filename string) (*BoltStorage, error) {
	Log(INFO|STORAGE, ctx, "Bolt.NewStorage", "filename", filename)
	var err error
	b := BoltStorage{Filename: filename}

	// Need a lock timeout.
	b.db, err = bolt.Open(b.Filename, 0644, DefaultOptions)
	if err != nil {
		Log(CRIT, ctx, "BoltStorage.Open", "error", err, "file", b.Filename)
		return nil, err
	}
	return &b, nil
}

// GetStats returns the stats for a location
// ToDo: store last record times
func (b *BoltStorage) GetStats(ctx *Context, loc string) (StorageStats, error) {
	var stats StorageStats
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(loc))
		if bucket == nil {
			return nil
		}
		bs := bucket.Stats()
		stats.NumRecords = bs.KeyN
		return nil
	})
	return stats, err
}

// Add adds a pair to a location
func (b *BoltStorage) Add(ctx *Context, loc string, data *Pair) error {
	timer := NewTimer(ctx, "BoltStorage.Add")
	defer timer.Stop()
	Log(INFO|STORAGE, ctx, "BoltStorage.Add", "location", loc,
		"key", string(data.K), "val", string(data.V))

	return b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(loc))
		if err != nil {
			Log(CRIT|STORAGE, ctx, "BoltStorage.Add", "error", err, "when", "CreateBucketIfNotExists")
			return err
		}
		err = bucket.Put(data.K, data.V)
		if err != nil {
			Log(CRIT|STORAGE, ctx, "BoltStorage.Add", "error", err, "when", "Put")
		}
		return err
	})
}

// Remove a pair from a location, we only care about the key
func (b *BoltStorage) Remove(ctx *Context, loc string, k []byte) (int64, error) {
	timer := NewTimer(ctx, "BoltStorage.Remove")
	defer timer.Stop()
	Log(INFO|STORAGE, ctx, "BoltStorage.Remove", "location", loc, "key", string(k))

	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(loc))
		if err != nil {
			return err
		}
		err = bucket.Delete(k)
		if err != nil {
			Log(CRIT|STORAGE, ctx, "BoltStorage.Remove", "error", err, "when", "Delete")
		}
		return err
	})
	return 0, err
}

// Clear removes a location and returns the number of records dropped
func (b *BoltStorage) Clear(ctx *Context, loc string) (int64, error) {
	timer := NewTimer(ctx, "BoltStorage.Clear")
	defer timer.Stop()
	Log(INFO|STORAGE, ctx, "BoltStorage.Clear", "location", loc)

	var n int64
	err := b.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(loc))
		if bucket == nil {
			return nil
		}
		stats := bucket.Stats()
		n = int64(stats.KeyN)
		err := tx.DeleteBucket([]byte(loc))
		if err != nil {
			Log(CRIT|STORAGE, ctx, "BoltStorage.Clear", "error", err, "when", "DeleteBucket")
		}
		return err
	})
	return n, err
}

// Delete removes a location and returns the number of records dropped
func (b *BoltStorage) Delete(ctx *Context, loc string) error {
	timer := NewTimer(ctx, "BoltStorage.Delete")
	defer timer.Stop()
	Log(INFO|STORAGE, ctx, "BoltStorage.Delete", "location", loc)

	_, err := b.Clear(ctx, loc)
	return err
}

// Load returns data for a location
// ToDo: should we return an error if there was no bucket for a location?
func (b *BoltStorage) Load(ctx *Context, loc string) ([]Pair, error) {
	timer := NewTimer(ctx, "BoltStorage.Load")
	defer timer.Stop()
	Log(INFO|STORAGE, ctx, "BoltStorage.Load", "location", loc)

	var data []Pair
	err := b.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(loc))
		if bucket == nil {
			return nil
		}
		c := bucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			Log(INFO|STORAGE, ctx, "BoltStorage.Load", "location", loc,
				"key", string(k), "val", string(v))
			data = append(data, Pair{k, v})
		}
		return nil
	})
	return data, err
}

// Destroy closes the database and deletes it
func (b *BoltStorage) Destroy(ctx *Context) error {
	Log(INFO|STORAGE, ctx, "BoltStorage.Destroy")
	err := b.db.Close()
	if err != nil {
		Log(CRIT|STORAGE, ctx, "BoltStorage.Destroy", "error", err, "when", "CloseDB")
		return err
	}
	err = os.Remove(b.Filename)
	if err != nil {
		Log(CRIT|STORAGE, ctx, "BoltStorage.Destroy", "error", err, "when", "RemoveDB")
	}
	return err
}

func (b *BoltStorage) Close(ctx *Context) error {
	Log(INFO|STORAGE, ctx, "BoltStorage.Close")
	return b.db.Close()
}

func (b *BoltStorage) Health(ctx *Context) error {
	Log(INFO|STORAGE, ctx, "BoltStorage.Health")
	// Maybe make a dummy transaction and then use 'Check'
	return nil
}

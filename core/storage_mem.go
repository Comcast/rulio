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

package core

import (
	"sync"
)

// MemStorage is a in-memory-only implementation of Storage.
type MemStorage struct {
	sync.Mutex
	// Since we have a two-level map, just use a plain
	// old lock instead of a RWLock.

	locToPairs map[string]map[string]string
}

func NewMemStorage(ctx *Context) (*MemStorage, error) {
	Log(INFO|STORAGE, ctx, "MemStorage.New")
	return &MemStorage{sync.Mutex{}, make(map[string]map[string]string)}, nil
}

func (s *MemStorage) loc(loc string) map[string]string {
	// Assumes (write) lock
	pairs, have := s.locToPairs[loc]
	if !have {
		pairs = make(map[string]string)
		s.locToPairs[loc] = pairs
	}
	return pairs
}

func (s *MemStorage) Load(ctx *Context, loc string) ([]Pair, error) {
	Log(INFO|STORAGE, ctx, "MemStorage.Load", "location", loc)
	s.Lock()
	acc := make([]Pair, 0, len(s.loc(loc)))
	for k, v := range s.loc(loc) {
		acc = append(acc, Pair{[]byte(k), []byte(v)})
	}
	s.Unlock()
	return acc, nil
}

func (s *MemStorage) Add(ctx *Context, loc string, m *Pair) error {
	Log(INFO|STORAGE, ctx, "MemStorage.Add", "location", loc,
		"key", string(m.K), "val", string(m.V))
	s.Lock()
	s.loc(loc)[string(m.K)] = string(m.V)
	s.Unlock()
	return nil
}

func (s *MemStorage) Remove(ctx *Context, loc string, k []byte) (int64, error) {
	Log(INFO|STORAGE, ctx, "MemStorage.Rem", "location", loc, "key", string(k))
	s.Lock()
	delete(s.loc(loc), string(k))
	s.Unlock()
	return 0, nil
}

func (s *MemStorage) Clear(ctx *Context, loc string) (int64, error) {
	Log(INFO|STORAGE, ctx, "MemStorage.Clear", "location", loc)
	s.Lock()
	delete(s.locToPairs, loc)
	s.Unlock()
	return 0, nil
}

func (s *MemStorage) Delete(ctx *Context, loc string) error {
	Log(INFO|STORAGE, ctx, "MemStorage.Delete", "location", loc)
	_, err := s.Clear(ctx, loc)
	return err
}

// GetStats isn't really implemented.
func (s *MemStorage) GetStats(ctx *Context, loc string) (StorageStats, error) {
	return StorageStats{}, nil
}

func (s *MemStorage) Close(ctx *Context) error {
	Log(INFO|STORAGE, ctx, "MemStorage.Close")
	return nil
}

func (s *MemStorage) Health(ctx *Context) error {
	Log(INFO|STORAGE, ctx, "MemStorage.Health")
	return nil
}

// State is an unholy method that exposes the raw data structure.
//
// Just used for testing.
func (s *MemStorage) State(ctx *Context) map[string]map[string]string {
	return s.locToPairs
}

// SetState is an unholy method that sets the raw data structure.
//
// Just used for testing.
func (s *MemStorage) SetState(ctx *Context, m map[string]map[string]string) {
	s.Lock()
	s.locToPairs = m // Good luck
	s.Unlock()
}

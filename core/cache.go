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
	lru "github.com/hashicorp/golang-lru"
	"sync"
	"time"
)

type Cache struct {
	sync.Mutex
	cache *lru.Cache
	TTL   time.Duration
}

func NewCache(limit int, ttl time.Duration) *Cache {
	var cache *lru.Cache
	if 0 < limit {
		var err error
		cache, err = lru.New(limit)
		if err != nil {
			panic(err)
		}
	}
	return &Cache{cache: cache, TTL: ttl}
}

type cacheEntry struct {
	X       interface{}
	expires time.Time
}

func (c *Cache) newEntry(x interface{}) *cacheEntry {
	return &cacheEntry{x, time.Now().Add(c.TTL)}
}

func (c *Cache) GetWith(key interface{}, thunk func() (interface{}, error)) (interface{}, error) {
	c.Lock()
	x, have := c.Get(key)
	var err error
	if !have {
		if x, err = thunk(); err == nil {
			c.Add(key, x)
		}
	}
	c.Unlock()
	return x, err
}

// Add adds a value to the cache.
func (c *Cache) Add(key, value interface{}) {
	if c.cache == nil {
		return
	}
	c.cache.Add(key, c.newEntry(value))
}

// Get looks up a key's value from the cache.
func (c *Cache) Get(key interface{}) (value interface{}, ok bool) {
	if c.cache == nil {
		return nil, false
	}
	got, have := c.cache.Get(key)
	if have {
		entry := got.(*cacheEntry)
		if entry.expires.Before(time.Now()) {
			c.cache.Remove(key)
			return nil, false
		}
		return entry.X, true
	}
	return nil, false
}

// Keys returns a slice of the keys in the cache.
func (c *Cache) Keys() []interface{} {
	return c.cache.Keys()
}

// Len returns the number of items in the cache.
func (c *Cache) Len() int {
	if c.cache == nil {
		return 0
	}
	return c.cache.Len()
}

// Purge is used to completely clear the cache
func (c *Cache) Purge() {
	if c.cache == nil {
		return
	}
	c.cache.Purge()
}

// Remove removes the provided key from the cache.
func (c *Cache) Remove(key interface{}) {
	if c.cache == nil {
		return
	}
	c.cache.Remove(key)
}

// RemoveOldest removes the oldest item from the cache.
func (c *Cache) RemoveOldest() {
	if c.cache == nil {
		return
	}
	c.cache.RemoveOldest()
}

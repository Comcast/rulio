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
	"fmt"
	"testing"
	"time"
)

func TestCache(t *testing.T) {
	c := NewCache(5, 100*time.Millisecond)

	k := NewHTTPClientSpec()
	got, found := c.Get(k)
	if found {
		t.Fatal("Cache should be empty")
	}

	c.Add(k, "tacos")

	got, found = c.Get(k)
	if !found {
		t.Fatal("Should have found tacos")
	}
	if got.(string) != "tacos" {
		t.Fatal(fmt.Sprintf("got '%v' instead of 'tacos'", got))
	}

	time.Sleep(200 * time.Millisecond)

	got, found = c.Get(k)
	if found {
		t.Fatal("Should have expired entry")
	}

	c.Add(k, "tacos")

	got, found = c.Get(k)
	if !found {
		t.Fatal("Should have found tacos (2)")
	}
	if got.(string) != "tacos" {
		t.Fatal(fmt.Sprintf("got '%v' instead of 'tacos' (2)", got))
	}

}

type CacheTester struct {
	n int
}

func BenchmarkCache(b *testing.B) {
	c := NewCache(100, 100*time.Millisecond)

	for i := 0; i < b.N; i++ {
		k := &CacheTester{}
		k.n = i * 17 % b.N
		c.Add(k, i)
		k = &CacheTester{}
		n := i * 47 % b.N
		k.n = n
		got, found := c.Get(k)
		// Quick check
		if found {
			k = got.(*CacheTester)
			if k.n != n {
				b.Fatal(fmt.Sprintf("Expected %d, not %d", n, k.n))
			}
		}
	}
}

func TestCacheZero(t *testing.T) {
	// Test disabled cache.
	c := NewCache(0, 1*time.Second)
	c.Add("likes", "beer")
	if _, ok := c.Get("likes"); ok {
		t.Fatal("things are not ok")
	}
	if 0 != c.Len() {
		t.Fatal("non-zero length")
	}
	c.Remove("likes")
	c.RemoveOldest()
	c.Purge()
}

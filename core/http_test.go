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
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/robertkrimen/otto"
)

// ToDo: More HTTPRequest tests.

func testHTTPRequest(t *testing.T, url string, shouldPass bool) {
	ctx := TestContext("TestHTTPRequest")

	req := NewHTTPRequest(ctx, "GET", url, "")
	res, _ := req.Do(ctx)
	if shouldPass {
		if res.Error != "" {
			t.Fatal(res.Error)
		}
	} else {
		if res.Error == "" {
			t.Fatal("should have failed")
		}
	}
}

func TestHTTPRequestBasic(t *testing.T) {
	wg := sync.WaitGroup{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Done()
	}))
	defer server.Close()
	wg.Add(1)
	testHTTPRequest(t, server.URL, true)
	wg.Wait()
}

func TestHTTPRequestBadURL(t *testing.T) {
	testHTTPRequest(t, "tacos://www.google.com", false)
}

// Some DNS servers aren't helpful here.
// func TestHTTPRequestBadDNS(t *testing.T) {
// 	testHTTPRequest(t, "http://hopethisnamereallyisbad.com", false)
// }

func TestHTTPBreaker(t *testing.T) {
	// Danger: Don't run concurrently!

	// Start an embedded test HTTP server.
	// Thank you, Go standard libraries.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("handling")
		fmt.Fprintln(w, "hello")
	}))
	defer server.Close()

	// Check that the server is working.
	testHTTPRequest(t, server.URL, true)

	// Here's a breaker for our little test server.  This breaker
	// should trip at any rate exceeding 5 requests per second.
	limit := 5
	breaker, err := NewOutboundBreaker(int64(limit), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	HTTPBreakers[server.URL] = breaker
	defer func() {
		delete(HTTPBreakers, server.URL)
	}()

	// Check that the server is still accessible.
	testHTTPRequest(t, server.URL, true)

	// Wait for the breaker to drain.
	time.Sleep(1 * time.Second)

	// Attempt too many requests.  The first ones should work.
	// The rest should fail after the breaker trips.
	for i := 0; i < limit*2; i++ {
		limited := limit <= i
		testHTTPRequest(t, server.URL, !limited)
	}
}

func TestHTTPHeader(t *testing.T) {
	var headers map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			v1 := r.Header.Get(k)

			if "" == v1 {
				t.Fatal("Do not have header: " + k)
			} else if v1 != v {
				t.Fatalf("Value mismatched for header: %s. Expecting %s, found %s", k, v, v1)
			}
		}
		fmt.Fprintln(w, "OK!")
	}))
	defer server.Close()

	ctx := TestContext("TestHTTPHeader")

	// Test nil header
	headers = nil
	ctx.App = &HeaderApp{headers}
	req := NewHTTPRequest(ctx, "GET", server.URL, "")
	res, err := req.Do(ctx)
	if nil != err {
		t.Fatal(err)
	}
	fmt.Println(res.Body)

	// Test valid headers
	headers = map[string]string{
		"a": "1",
		"b": "2",
		"c": "3",
	}
	ctx.App = &HeaderApp{headers}
	req = NewHTTPRequest(ctx, "GET", server.URL, "")
	res, err = req.Do(ctx)
	if nil != err {
		t.Fatal(err)
	}
	fmt.Println(res.Body)
}

type HeaderApp struct {
	headers map[string]string
}

func (ha *HeaderApp) GenerateHeaders(ctx *Context) map[string]string {
	return ha.headers
}

func (ha *HeaderApp) ProcessBindings(ctx *Context, bs Bindings) Bindings {
	return bs
}

func (ha *HeaderApp) UpdateJavascriptRuntime(ctx *Context, runtime *otto.Otto) error {
	return nil
}

func (ha *HeaderApp) ProcessQuery(_ *Context, _ map[string]interface{}, q Query) Query {
	return q
}

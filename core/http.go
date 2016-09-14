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

// The Go http package's separation between Transport and Request
// means that a caller can't easily specify, say, DisableKeepAlives or
// a ResponseHeaderTimeout.  We probably shouldn't really create
// Transports on the fly because then we'd have no http.Client reuse.
//
// Important: We'd like timeouts to be short.  Instead of waiting a
// long time for a response, it might be better to queue a retry
// (somewhere).  (One concern: keeping a location open increases the
// chances of inconsistencies due to multi-process location mutations
// and however that's coordinated.)
//
// At lower level, network errors are opaque.  If the connection has
// been reset, we almost certainly want to retry.  But we can't easily
// tell when an error is "connection reset".

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPBreakers maps URLs and hosts to OutboundBreakers for HTTPRequests (below).
//
// This state isn't mutexified or atomic.  ToDo: Or just set before
// doing any work.
var HTTPBreakers = make(map[string]*OutboundBreaker)

// getHTTPBreaker finds a breaker (if any) for the given URI in
// HTTPBreakers.
//
// First the URI itself is used for the key.  Failing that, the URI
// host is used.  Returns nil if no breaker was found.
//
// HTTPBreakers isn't synchronized, and this function doesn't get a
// (read) lock.  ToDo or don't write HTTPBreakers when other stuff is
// running.
func getHTTPBreaker(ctx *Context, uri string) *OutboundBreaker {
	b, have := HTTPBreakers[uri]
	if have {
		return b
	}
	u, err := url.Parse(uri)
	if err != nil {
		Log(WARN, ctx, "getHTTPBreaker", "error", err, "url", uri)
	}
	b, _ = HTTPBreakers[u.Host]
	return b
}

// HTTPClientSpec packages up all configuration for an HTTP request.
// This configuration includes settings such as ResponseHeaderTimeout
// that are below the level of Go's http.Request.
//
// Instances of this type will serve as hash keys for the http.Client
// cache.
type HTTPClientSpec struct {
	// Client-level options

	Timeout            Duration `json:"timeout,omitempty"`
	InsecureSkipVerify bool     `json:"insecureSkipVerify,omitempty"`

	// Transport-level options

	DisableKeepAlives     bool     `json:"disableKeepAlives,omitempty"`
	ResponseHeaderTimeout Duration `json:"responseHeaderTimeout,omitempty"`
	MaxIdleConnsPerHost   int      `json:"maxIdleConnsPerHost,omitempty"`
}

// DefaultHTTPClientSpec generates HTTPClientSpec based on defaults
// given by SystemParameters.
func NewHTTPClientSpec() *HTTPClientSpec {
	return &HTTPClientSpec{
		Timeout:               Duration(SystemParameters.HTTPTimeout),
		InsecureSkipVerify:    SystemParameters.InsecureSkipVerify,
		DisableKeepAlives:     SystemParameters.DisableKeepAlives,
		ResponseHeaderTimeout: Duration(SystemParameters.ResponseHeaderTimeout),
		MaxIdleConnsPerHost:   SystemParameters.MaxIdleConnsPerHost,
	}
}

// Client creates a new http.Client for the given spec.
func (cs *HTTPClientSpec) Client() (*http.Client, error) {
	t := http.Transport{
		MaxIdleConnsPerHost:   cs.MaxIdleConnsPerHost,
		ResponseHeaderTimeout: time.Duration(cs.ResponseHeaderTimeout),
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cs.InsecureSkipVerify},
	}
	c := &http.Client{Transport: &t,
		Timeout: time.Duration(cs.Timeout),
	}
	return c, nil

}

// HTTPRequest packages up all data required to make an HTTP request
// (that can retry).  Use 'NewHTTPRequest' to make one.
//
// We no longer do/control retries here.
type HTTPRequest struct {
	// Method is the HTTP request method (e.g., "POST").
	Method string `json:"method,omitempty"`

	// URI is what you expect.
	URI string `json:"uri"`

	// Values to be add to Header for the request
	//
	// ToDo: Probably support proper []string values.
	Headers map[string]string `json:"headers,omitempty"`

	// Body is the request body.
	Body string `json:"body,omitempty"`

	// ContentType is what you expect.
	ContentType string `json:"contentType,omitempty"`

	// Env (if given) provides and receives cookies.
	Env map[string]interface{} `json:"env,omitempty"`

	// ClientSpec controls lower-level aspects of the HTTP request.
	ClientSpec *HTTPClientSpec `json:"client,omitempty"`
}

// NewHTTPRequest generates a basic HTTPRequest using some defaults
// from SystemParameters.
//
// You can customize the returned instance.  For example, you can set
// its 'Env' to deal with cookies.
func NewHTTPRequest(ctx *Context, method string, uri string, body string) *HTTPRequest {
	r := &HTTPRequest{Method: method,
		URI:         uri,
		Body:        body,
		ContentType: "application/json",
	}

	if nil != ctx && nil != ctx.App {
		r.Headers = ctx.App.GenerateHeaders(ctx)
	}

	return r
}

// Request constructs but does not issue a http.Request.
//
// If 'Env' isn't nil, that map will be used to obtain cookie data.
func (r HTTPRequest) Request(ctx *Context) (*http.Request, error) {
	Metric(ctx, "ExternalRequest", "url", r.URI)

	req, err := http.NewRequest(r.Method, r.URI, strings.NewReader(r.Body))
	if err != nil {
		Log(ERROR, ctx, "HTTPRequest.Request", "uri", r.URI, "error", err)
		return nil, err
	}

	req.Header.Add("Accept", `*/*`)
	req.Header.Add("User-Agent", "rulescore/"+Version)
	req.Header.Add("Content-Type", r.ContentType)

	for k, v := range r.Headers {
		req.Header.Add(k, v)
	}

	// Get cookies from env and add to the request.
	if r.Env != nil {
		cookieKey := req.URL.Host
		if cookies, ok := r.Env[cookieKey].(map[string]*http.Cookie); ok {
			for _, c := range cookies {
				if "" == c.Path || strings.Contains(req.URL.Path, c.Path) {
					req.AddCookie(c)
					Log(DEBUG, ctx, "HTTPRequest.do", "uri", r.URI, "useCookie", c)
				}
			}
		}
	}

	return req, nil
}

// HTTPResult packages up everything we return to the caller of an
// HTTPRequest.
//
// We're trying to use only basic field types for the convenience of
// Javascript users.
type HTTPResult struct {
	// Status is the last HTTP status code received.
	Status int

	// Body is the last body (if any) received.
	Body string

	// Attemts is the number of attempts made.
	Attempts int

	// Error is the most recent error message.
	//
	// We use a string here for the convenience of Javascript
	// callers.  Might be a bad idea.
	Error string

	// Headers are the response headers.
	Headers map[string][]string
}

// DoOnce get a client, constructs a request, issues the request, and
// gathers the result.
func (r HTTPRequest) DoOnce(ctx *Context, t *HTTPResult) error {
	timer := NewTimer(ctx, "HTTPRequest.DoOnce")
	// Using r.URI as a tag is a little dangerous due to letting
	// data increase the number of timers we're using.
	defer timer.StopTag(r.URI)

	Log(INFO, ctx, "HTTPRequest.do", "uri", r.URI, "attempt", t.Attempts)

	t.Attempts++

	if t == nil {
		return errors.New("no given HTTPResult")
	}

	breaker := getHTTPBreaker(ctx, r.URI)
	if breaker != nil {
		if !breaker.Zap() {
			Log(INFO, ctx, "HTTPRequest.do", "uri", r.URI, "breaker", "open")
			return Throttled
		}
	}

	client, err := r.Client(ctx)
	if err != nil {
		t.Error = err.Error()
		Log(ERROR, ctx, "HTTPRequest.do", "uri", r.URI, "error", t.Error, "when", "Client")
		return err
	}
	req, err := r.Request(ctx)
	if err != nil {
		t.Error = err.Error()
		Log(ERROR, ctx, "HTTPRequest.do", "uri", r.URI, "error", t.Error, "when", "Request")
		return err
	}

	res, err := client.Do(req)
	if res != nil {
		t.Headers = res.Header
		t.Status = res.StatusCode
		if res.Body != nil {
			got, err := ioutil.ReadAll(res.Body)
			if got != nil {
				t.Body = string(got)
			}
			problem := res.Body.Close()
			if err == nil {
				err = problem
			}
		}
	}

	if err != nil {
		t.Error = err.Error()
		Log(ERROR, ctx, "HTTPRequest.do", "uri", r.URI, "error", t.Error)
		return err
	}

	// Set cookies from response to env if any, if only
	// supports full domain name for now.
	if r.Env != nil {
		cookieKey := req.URL.Host
		cookies := res.Cookies()
		if nil != cookies {
			m, ok := r.Env[cookieKey].(map[string]*http.Cookie)
			if !ok {
				m = make(map[string]*http.Cookie)
				r.Env[cookieKey] = m
			}
			for _, c := range cookies {
				Log(DEBUG, ctx, "HTTPRequest.do", "uri", r.URI, "setCookie", c)
				m[c.Name] = c
			}
		}
	}

	return err
}

func (r HTTPRequest) Do(ctx *Context) (*HTTPResult, error) {
	timer := NewTimer(ctx, "HTTPRequest.Do")

	t := &HTTPResult{}
	err := r.DoOnce(ctx, t)
	Log(INFO, ctx, "HTTPRequest.Do", "uri", r.URI, "status", t.Status)
	if err == Throttled {
		Log(WARN, ctx, "HTTPRequest.Do", "uri", r.URI, "throttled", true, "attempt", t.Attempts)
		// We don't have a good HTTP status code we
		// can use to communicate what as happened to
		// us.  See the conditional retry logic below.
		//
		// We'll use 430, which is unassigned.  Note
		// that 429 is "too many requests", but we
		// want to be able to distinguish a server
		// saying 429 from our own throttling.
		t.Status = 430
		t.Error = err.Error()
	}

	// Using r.URI as a tag is a little dangerous due to letting
	// data increase the number of timers we're using.
	timer.StopTag(r.URI)

	return t, err
}

// HTTPClientCache is a cache for http.Clients.
//
// The need arises due to a caller's potential wish to specify
// settings that are associated with, say, a http.Transport as opposed
// to a http.Request.  We very much (usually) want to cache/reuse
// http.Clients but we also want to allow users to specify, say,
// ResponseHeaderTimeouts.
//
// So we have http.client cache that is keyed by our HTTPClientSpecs,
// which includes http.Transport-level (and other) settings.
//
// This cache is an LRU cached implemented using
// github.com/hashicorp/golang-lru.  The parameters for the cache are
// SystemParameters.HTTPClientCacheSize and
// SystemParameters.HTTPClientCacheTTL.
var HTTPClientCache = NewCache(SystemParameters.HTTPClientCacheSize,
	SystemParameters.HTTPClientCacheTTL)

// Client returns an http.Client appropriate for the given request.
//
// This function first consults HTTPClientCache.  If a client with a
// matching HTTPClientSpec exists, it's returned.  Otherwise a new
// client is created, cached, and returned.
//
// There is probably a race condition in this code, but I don't think
// we care.  Famous last words.
func (r *HTTPRequest) Client(ctx *Context) (*http.Client, error) {
	cache := HTTPClientCache
	spec := r.ClientSpec
	if spec == nil {
		spec = NewHTTPClientSpec()
		r.ClientSpec = spec
	}

	key := *spec
	c, err := cache.GetWith(key, func() (interface{}, error) {
		Log(DEBUG, ctx, "HTTPRequest.Client", "cache", "miss", "cacheKey", key)
		return r.ClientSpec.Client()
	})

	if err != nil {
		Log(WARN, ctx, "HTTPRequest.Client", "error", err, "cacheKey", key)
		return nil, err
	}

	client, ok := c.(*http.Client)
	if !ok {
		return nil, fmt.Errorf("didn't expect a %T", c)
	}
	return client, nil
}

// Post issues a POST to the specified URL.
func Post(ctx *Context, uri string, contentType string, body string) (string, error) {
	req := NewHTTPRequest(ctx, "POST", uri, body)
	req.ContentType = contentType

	res, err := req.Do(ctx)
	got := res.Body
	return got, err
}

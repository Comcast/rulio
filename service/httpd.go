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

package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Comcast/rulio/core"

	"gopkg.in/yaml.v2"
)

var BeGraceful = true // Parameter

type HTTPService struct {
	Ctx        *core.Context
	Service    *Service
	pending    int32
	maxPending int32
	listener   net.Listener
	connStates *ConnStates
}

func NewHTTPService(ctx *core.Context, service *Service) (*HTTPService, error) {
	return &HTTPService{Ctx: ctx, Service: service, connStates: NewConnStates()}, nil
}

func (s *HTTPService) Pending() int32 {
	return atomic.LoadInt32(&s.pending)
}

func (s *HTTPService) incPending(add bool) {
	inc := int32(1)
	if !add {
		inc = -1
	}
	atomic.AddInt32(&s.pending, inc)
}

func (s *HTTPService) MaxPending() int32 {
	return atomic.LoadInt32(&s.maxPending)
}

func (s *HTTPService) SetMaxPending(max int32) {
	core.Log(core.INFO, nil, "service.HTTPService", "maxPending", max)
	atomic.StoreInt32(&s.maxPending, max)
}

func (s *HTTPService) Maxed() (bool, int32) {
	max := s.MaxPending()
	pending := s.Pending()
	if max == 0 {
		return false, pending
	}
	return max <= pending, pending
}

type Listener struct {
	ctx     *core.Context
	l       net.Listener
	service *HTTPService
	ctl     chan string
	mode    string
	admin   bool
}

func NewListener(ctx *core.Context, s *HTTPService, port string, admin bool) (*Listener, error) {
	l, err := net.Listen("tcp", port)
	if err != nil {
		return nil, err
	}
	ctl := make(chan string, 5)
	return &Listener{ctx: ctx, l: l, service: s, ctl: ctl, admin: admin}, nil
}

func (l *Listener) Drain(d time.Duration) int {
	pause := 1 * time.Second

	waited := time.Duration(0)
	var n int32
	for i := 0; true; i++ {
		_, n = l.service.Maxed()
		core.Log(core.INFO, l.ctx, "service.Listener.Stop", "loop", i, "pending", n, "waited", waited.String())

		if n <= 0 {
			break
		}
		time.Sleep(pause)
		waited += pause
		if d <= waited {
			break
		}
	}
	core.Log(core.INFO, l.ctx, "service.Listener.Stop", "pending", n, "waited", waited.String())
	return int(n)
}

func (l *Listener) Stop(d time.Duration) error {

	core.Log(core.INFO, l.ctx, "service.Listener.Stop")
	l.ctl <- "stop"

	n := l.Drain(d)
	core.Log(core.INFO, l.ctx, "service.Listener.Stop", "pending", n)
	l.mode = "stopped"

	return nil
}

func tooMany(c net.Conn) {
	w := bufio.NewWriter(c)
	w.WriteString("HTTP/1.1 429 Too Many Requests\n")
	w.WriteString("Content-Length: 0\n")
	w.WriteString("Connection: close\n")
	w.Flush()
	c.Close()
}

type TooManyConnectionsError struct {
}

func (e *TooManyConnectionsError) Error() string {
	return "too many connections"
}

func (e *TooManyConnectionsError) Temporary() bool {
	return true
}

func (e *TooManyConnectionsError) Timeout() bool {
	return false
}

var TooManyConnections = &TooManyConnectionsError{}

func (l *Listener) Accept() (c net.Conn, err error) {
	select {
	case op := <-l.ctl:
		core.Log(core.INFO, l.ctx, "service.Listener", "op", op)
		l.mode = op
	default:
	}

	if !l.admin {
		maxed, n := l.service.Maxed()

		switch l.mode {
		case "stop":
			err := fmt.Errorf("service stopping (%d)", n)
			core.Log(core.INFO, l.ctx, "service.Listener", "stopping", n)
			return nil, err
		case "stopped":
			err := fmt.Errorf("service stopped")
			core.Log(core.INFO, l.ctx, "service.Listener", "stopped", n)
			return nil, err
		case "":
		default:
			core.Log(core.WARN, l.ctx, "service.Listener", "mode", l.mode)
		}

		if maxed {
			err := fmt.Errorf("too many connections: %d", n)
			core.Log(core.WARN, l.ctx, "service.Listener", "error", err)
			tooMany(c)
			return nil, TooManyConnections
		}
	}

	return l.l.Accept()
}

func (l *Listener) Close() error {
	return l.l.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.l.Addr()
}

var parameterTypes = map[string]string{
	"fact":    "json",
	"rule":    "json",
	"pattern": "json",
	"query":   "json",
	"event":   "json",
	"limit":   "int",
}

var UnknownSyntax = errors.New("unknown syntax")

func MaybeYAML(bs []byte) bool {
	newline := bytes.Index(bs, []byte("\n"))
	return 0 <= newline && newline < len(bs)
}

// StringMaps tries to makes (recursively) a map[string]interface{}
// from a map[interface{}]interface{} (which yaml.Unmarshal tends to
// provide).  When something goes wrong, the original value is
// returned.
func StringMaps(x interface{}) interface{} {
	switch vv := x.(type) {
	case []interface{}:
		for i, x := range vv {
			vv[i] = StringMaps(x)
		}
		return vv
	case map[string]interface{}:
		for k, v := range vv {
			vv[k] = StringMaps(v)
		}
		return vv
	case map[interface{}]interface{}:
		m := make(map[string]interface{}, len(vv))
		for k, v := range vv {
			s, is := k.(string)
			if !is {
				return x
			}
			m[s] = StringMaps(v)
		}
		return m
	default:
		return x
	}
}

func UnmarshalYAML(bs []byte, v interface{}) error {
	v = reflect.Indirect(reflect.ValueOf(v)).Interface()
	err := yaml.Unmarshal(bs, v)
	if err == nil {
		v = StringMaps(v)
	}
	return err
}

func Unmarshal(bs []byte, v interface{}) error {
	if bs[0] == '{' {
		return json.Unmarshal(bs, v)
	}

	// Do we have at least one newline?

	if MaybeYAML(bs) {
		return UnmarshalYAML(bs, v)
	}

	return UnknownSyntax
}

func parseParameter(p, v string) (interface{}, error) {
	typ, custom := parameterTypes[p]
	if !custom {
		return v, nil
	}
	switch typ {
	case "json":
		m := make(map[string]interface{})
		err := Unmarshal([]byte(v), &m)
		return m, err
	case "int":
		return strconv.ParseInt(v, 10, 32)
	default:
		return nil, fmt.Errorf("Unknown parameter type %s", typ)
	}
}

func GetHTTPRequest(ctx *core.Context, r *http.Request) (map[string]interface{}, error) {

	uri := DWIMURI(ctx, r.URL.String())
	core.Log(core.INFO, ctx, "service.GetHTTPRequest", "method", r.Method, "uri", uri)

	m := make(map[string]interface{})

	parseQuery := func(q string) error {
		qvs, err := url.ParseQuery(q)
		if err != nil {
			return err
		}
		for p, vs := range qvs {
			if len(vs) != 1 {
				return fmt.Errorf("need exactly one value (not %d) for %s", len(vs), p)
			}
			var v interface{}
			if v, err = parseParameter(p, vs[0]); err != nil {
				return err
			}
			m[p] = v
		}
		return nil
	}

	if err := parseQuery(r.URL.RawQuery); err != nil {
		return nil, err
	}

	var err error
	switch uri {
	case "/api/json", "/api/yaml":
		var err error
		switch r.Method {
		case "POST":
			var js []byte
			if js, err = ioutil.ReadAll(r.Body); err != nil {
				return nil, err
			}
			switch uri {
			case "/api/json":
				if err = json.Unmarshal(js, &m); err != nil {
					return nil, err
				}
			case "/api/yaml":
				if err = UnmarshalYAML(js, &m); err != nil {
					return nil, err
				}
			}
			given, have := m["uri"]
			if !have {
				return nil, fmt.Errorf("no uri given by %s", js)
			}
			var ok bool
			uri, ok = given.(string)
			if !ok {
				return nil, fmt.Errorf("need a string uri, not a %T (%s)", given, given)
			}
		default:
			return nil, errors.New("no uri given")
		}

	default:
		m["uri"] = r.URL.Path

		switch r.Method {
		case "POST":
			var js []byte
			if js, err = ioutil.ReadAll(r.Body); err != nil {
				return nil, err
			}

			if js[0] == '{' {
				// If the body looks like JSON, treat it as JSON.
				if err = json.Unmarshal(js, &m); err != nil {
					return nil, err
				}
			} else if MaybeYAML(js) {
				if err = UnmarshalYAML(js, &m); err != nil {
					return nil, err
				}
			} else {
				// Try to parse a form.
				if err := parseQuery(string(js)); err != nil {
					return nil, err
				}
			}
		}
	}

	io.Copy(ioutil.Discard, r.Body)

	core.Log(core.INFO, ctx, "service.GetHTTPRequest", "m", m)
	return m, nil
}

func protest(ctx *core.Context, err error, w http.ResponseWriter) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, err.Error())
}

func (s *HTTPService) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	s.incPending(true)
	ctx := s.Ctx.SubContext()

	timer := core.NewTimer(ctx, "ServeHTTP")

	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				core.Log(core.WARN, nil, "ServerHTTP", "error", err, "when", "Close")
			}
		}
		s.incPending(false)
		timer.Stop()

	}()

	m, err := GetHTTPRequest(ctx, r)

	if err != nil {
		core.Log(core.ERROR, ctx, "service.ServeHTTP", "error", err)
		protest(ctx, err, w)
		return
	}

	switch DWIMURI(ctx, m["uri"].(string)) { // Sorry.
	case "/api/sys/admin/connstates":
		counts := s.connStates.Get()
		js, err := json.Marshal(&counts)
		if err != nil {
			w.Write([]byte(err.Error()))
			return
		}
		w.Write(js)
		return

	case "/api/sys/admin/pending":
		max, given := m["max"]
		if given {
			str, ok := max.(string)
			if !ok {
				protest(ctx, err, w)
				return
			}
			n, err := strconv.Atoi(str)
			if err != nil {
				protest(ctx, err, w)
				return
			}
			s.SetMaxPending(int32(n))
		}
		maxed, pending := s.Maxed()
		w.Write([]byte(fmt.Sprintf(`{"pending":%d, "max":%d, "maxed":%v}`, pending, s.MaxPending(), maxed) + "\n"))
		return
	}

	_, err = s.Service.ProcessRequest(ctx, m, w)
	if err != nil {
		if redirect, is := err.(*Redirect); is {
			core.Log(core.INFO, ctx, "service.ServeHTTP", "redirect", redirect.To)
			http.Redirect(w, r, "http://"+redirect.To, 301)
			return
		} else {
			core.Log(core.ERROR, ctx, "service.ServeHTTP", "error", err)
			protest(ctx, err, w)
		}
	}
	w.Write([]byte{byte('\n')})
}

type ConnStates struct {
	sync.Mutex
	counts map[string]int64
}

func NewConnStates() *ConnStates {
	return &ConnStates{counts: make(map[string]int64)}
}

func (cs *ConnStates) Inc(state string) {
	cs.Lock()
	cs.counts[state]++
	cs.Unlock()
}

func (cs *ConnStates) Get() map[string]int64 {
	acc := make(map[string]int64)
	cs.Lock()
	for p, v := range cs.counts {
		acc[p] = v
	}
	cs.Unlock()
	return acc
}

func (s *HTTPService) Start(ctx *core.Context, servicePort string) error {

	server := &http.Server{
		Handler: s,
		// ReadTimeout:    60 * time.Second,
		// WriteTimeout:   60 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ConnState: func(c net.Conn, state http.ConnState) {
			s.connStates.Inc(state.String())
			loc := c.LocalAddr()
			rem := c.RemoteAddr()
			core.Log(core.INFO, ctx, "service.Server", "conn", c, "state", state, "local", loc, "remote", rem)
		},
	}
	core.Log(core.INFO, ctx, "service.HTTPService", "port", servicePort)

	if !BeGraceful {
		return server.ListenAndServe()
	} else {
		l, err := NewListener(ctx, s, servicePort, false)
		if err != nil {
			return err
		}
		s.Service.Stopper = func(ctx *core.Context, d time.Duration) error {
			return l.Stop(d)
		}
		s.listener = l
		server.Serve(l)
		n := l.Drain(5 * time.Second)
		if n == 0 {
			return nil
		}
		return fmt.Errorf("killing %d pending requests", n)
	}
}

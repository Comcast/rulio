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
	"time"
)

// Version is the Core version.
const Version = "0.0.1"

// SystemParameters packages up misc almost const parameters that the
// entire process will use.
//
// ToDo: Probably make accessors for this pointer.
//
// ToDoLater: Demote to a field in a higher-level struct somewhere?
var SystemParameters = SetParameters(DefaultParameters())

var SystemParameterHooks = make([]func(*Parameters) error, 0, 0)

// ParametersAddHook installs a function that is called when
// SystemParameters change.
//
// Not thread-safe.
func ParametersAddHook(f func(*Parameters) error) {
	SystemParameterHooks = append(SystemParameterHooks, f)
}

func SetParameters(p *Parameters) *Parameters {
	for _, hook := range SystemParameterHooks {
		if err := hook(p); err != nil {
			panic(err)
		}
	}
	return p
}

// Parameters is a package of almost const parameters.
type Parameters struct {
	// ToDo: Add mutex?

	// SlurpCacheSize is the maximum number of cached Slurp results
	SlurpCacheSize int

	// SlurpTimeout is that.
	SlurpTimeout time.Duration

	// SlurpCacheTTL is the TTL for SlurpCache entries.
	SlurpCacheTTL time.Duration

	// Timeout enables Javascript timeouts.
	JavascriptTimeouts bool

	// LogAccumulatorSize is the size of log accumulator buffers.
	LogAccumulatorSize int

	// HTTPClientCacheSize is the maximum number of HTTPClients to
	// cache.
	HTTPClientCacheSize int

	// HTTPClientCacheTTL is the TTL for a HTTPClientCache entry.
	HTTPClientCacheTTL time.Duration

	// IdLengthLimit is the maximum size for an id.
	IdLengthLimit int

	// IdInjectionTime determines the id injection mode.
	IdInjectionTime int

	// TimerHistorySize limits how many entries are stored for a given timer.
	TimerHistorySize int

	// TimerWarningLimit, when exceeded, generates a log warning.
	TimerWarningLimit time.Duration

	// MaxTimers limits the number of timers we track so bad data doesn't kill us.
	// This limit is not currently enforced!
	MaxTimers int

	// LogRecordValueLimit is the maximum value length in a LogRecord.
	// See MakeLogRecord() below
	LogRecordValueLimit int

	// LogCallerLine adds the line number of the callers to log records.
	LogCallerLine bool

	// ConcurrentWalks turns concurrent EventWorkWalking on or off.
	ConcurrentWalks bool

	// DefaultControl is the default Location.Control
	DefaultControl *Control

	// StringLengthTermLimit is the maximum length of a string that gets indexed.
	// If your string (in an FDS) is longer that this limit, it won't get
	// indexed.
	StringLengthTermLimit int

	// DefaultJavascriptTimeout does what you'd think.
	//
	// A Location's control can override this value (if
	// SystemParameters.Timeout is true).
	DefaultJavascriptTimeout time.Duration

	// MaxIdleConnsPerHost is that parameter for HTTP clients.
	MaxIdleConnsPerHost int

	// HTTPTimeout
	HTTPTimeout time.Duration

	// InsecureSkipVerify
	InsecureSkipVerify bool

	// DisableKeepAlives
	DisableKeepAlives bool

	// ResponseHeaderTimeout
	ResponseHeaderTimeout time.Duration

	// HTTPRetryInterval
	HTTPRetryInterval time.Duration

	// HTTPRetryOn
	HTTPRetryOn StringSet
}

// Copy makes a shallow (except for DefaultControl) copy.
//
// The DefaultControl is Copy()ed.
func (ps *Parameters) Copy() *Parameters {
	// ToDo: Make atomic.
	var target Parameters
	target = *ps
	if ps.DefaultControl != nil {
		target.DefaultControl = ps.DefaultControl.Copy()
	}
	return &target
}

// Note that our use of 430 is for throttling on our side.  See 'http.go'.
var defaultRetryOn = EmptyStringSet().AddStrings("404", "408", "423", "429", "430", "500", "502", "503", "504", "507", "509")

func DefaultParameters() *Parameters {
	ps := Parameters{}

	ps.SlurpCacheSize = 1000
	ps.SlurpTimeout = 60 * time.Second
	ps.JavascriptTimeouts = true
	ps.LogAccumulatorSize = 255
	ps.HTTPClientCacheSize = 64
	ps.HTTPClientCacheTTL = 5 * time.Minute
	ps.IdLengthLimit = 1024
	ps.IdInjectionTime = InjectIdNever
	ps.TimerHistorySize = 1024
	ps.TimerWarningLimit = 5 * time.Second
	ps.MaxTimers = 1024
	ps.LogRecordValueLimit = 1020
	ps.LogCallerLine = true
	ps.ConcurrentWalks = true
	ps.DefaultControl = DefaultControl()
	ps.StringLengthTermLimit = 1024
	ps.DefaultJavascriptTimeout = 60 * time.Second
	ps.MaxIdleConnsPerHost = 1000
	ps.HTTPTimeout = 60 * time.Second
	ps.InsecureSkipVerify = true
	ps.DisableKeepAlives = false
	ps.ResponseHeaderTimeout = 20 * time.Second
	ps.HTTPRetryInterval = 20 * time.Second
	ps.HTTPRetryOn = defaultRetryOn
	return &ps
}

func TightParameters() *Parameters {
	ps := Parameters{}

	ps.SlurpCacheSize = 0
	ps.SlurpTimeout = 60 * time.Second
	ps.JavascriptTimeouts = false
	ps.LogAccumulatorSize = 0
	ps.HTTPClientCacheSize = 0
	ps.HTTPClientCacheTTL = 5 * time.Minute
	ps.IdLengthLimit = 1024
	ps.IdInjectionTime = InjectIdNever
	ps.TimerHistorySize = 0
	ps.TimerWarningLimit = 5 * time.Second
	ps.MaxTimers = 0
	ps.LogRecordValueLimit = 32
	ps.LogCallerLine = false
	ps.ConcurrentWalks = false
	ps.DefaultControl = DefaultControl()
	ps.StringLengthTermLimit = 1024
	ps.DefaultJavascriptTimeout = 60 * time.Second
	ps.MaxIdleConnsPerHost = 32
	ps.HTTPTimeout = 60 * time.Second
	ps.InsecureSkipVerify = true
	ps.DisableKeepAlives = false
	ps.ResponseHeaderTimeout = 20 * time.Second
	ps.HTTPRetryInterval = 20 * time.Second
	ps.HTTPRetryOn = defaultRetryOn

	return &ps
}

func (ps *Parameters) Log(ctx *Context) {
	if ps == nil {
		Log(WARN, ctx, "Parameters.Log", "ps", nil)
		return
	}
	m, err := StructToMap(ps)
	if err != nil {
		Log(ERROR, ctx, "Parameters.Log", "error", err)
		return
	}
	args := make([]interface{}, 0, 1+len(m)*2)
	args = append(args, "Paramters")
	for p, v := range m {
		args = append(args, p, v)
	}
	Log(INFO, ctx, args...)
}

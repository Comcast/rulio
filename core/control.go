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
	"regexp"
	"time"
)

// Config are read-only, boot-time settings.  Once specified, these
// settings cannot be changed.
type Config struct {
}

// SystemControl represents ephemeral control options.
//
// These settings are process-specific and not stored.  Note that all
// of these values are simple (not maps or arrays or structs).  You
// can change them (hopefully atomically) at will.
type Control struct {
	// Turn off timer logging.
	//
	// The property is "NoTiming" to make the natural default do
	// what we usually want.  Also see 'Control.Defaults()'.
	NoTiming bool

	// Control what's logged.
	//
	// Examples: 'ANYINFO', 'ANYWARN', 'EVERYTHING', 'NOTHING'.
	// See contants defined in this file.  Also see
	// 'ParseVerbosity()'.
	Verbosity LogLevel

	// Logging format: "CSV" (one-line JSON using external logger
	// -- not comma-separated values!), "pretty" (which spews
	// pretty-printed JSON), or "none" (which doesn't emit
	// anything).  Default is "CSV".
	//
	// The environment variable 'RULES_LOGS' overrides this
	// setting, which makes it easy to do quick performance tests
	// with no logging.
	Logging string

	// The maximum number of facts this Location will store.
	MaxFacts int

	// BindingsWarningLimit sets the threshold for a warning when
	// that many bindings are found during a fact search.
	BindingsWarningLimit int

	// Run fact expiration before every search.
	ExpireFactsDuringSearch bool

	// Before returning search results, check for fact expirations.
	CheckForFactExpiration bool

	// A directory of services used for finding action endpoints
	// and external fact service endpoints.  A value is an array
	// of URLs, one of which is picked at random for cheap load
	// balancing.
	//
	// Also see InternalFactServices.
	Services map[string][]string

	// InternalFactServices maps logical names to FactServices
	// (that are implemented within this process).
	//
	// Also see Services.
	InternalFactServices map[string]FactService `json:"-"`

	// Libraries of Javascript code made available to in-process
	// Javascript (both in conditions and in actions).  Each value
	// should be either a URL that returns Javascript code or a
	// string of Javascript code.
	Libraries map[string]string

	// Misc props that are available to in-process Javascript
	// (in conditions and actions).
	CodeProps map[string]interface{}

	// What it says.  Value is a string that can be parsed by Go's
	// http://golang.org/pkg/time/#ParseDuration.  This value can
	// be overridden during `ingest`.
	//
	// Note the type: a 'Duration', not a 'time.Duration'.  Why?
	// Because we want to parse JSON into this struct.  See
	// 'Duration' below.
	ExternalFactServiceTimeout Duration

	// ActionTimeout (nanoseconds) greater than zero will attempt
	// to terminate any action execution that lasts longer than
	// the timeout.  Not implemented yet.
	ActionTimeout Duration

	// JavascriptTimeout (nanoseconds) greater than zero will
	// attempt to terminate any action execution that lasts longer
	// than the timeout.
	JavascriptTimeout Duration

	// DisableExecFunction removes the 'shell' function from the
	// Javascript environment.
	DisableExecFunction bool
	// UseDefaultVariableValue will give DefaultVariableValue to
	// unbound variables.  Otherwise and unbound variable will
	// (hopefully) result in an error.
	UseDefaultVariableValue bool

	// DefaultVariableValue will be assigned to any unbound
	// variables if 'UseDefaultVariableValue' is true.
	DefaultVariableValue interface{}
}

// Duration allows us to parse strings into durations.
// See Duration.UnmarshalJSON().
type Duration time.Duration

// UnmarshalJSON parses a string into a Duration.
//
// Go says, "cannot define new methods on non-local type
// time.Duration", so we have to work a little indirectly.  Double
// quotes are stripped, and a string consisting entire of numbers is
// interpreted as nanoseconds.  Otherwise the string is parsed as a Go
// time.Duration.
func (d *Duration) UnmarshalJSON(data []byte) error {

	if matched, _ := regexp.Match(`^".*"$`, data); matched {
		data = data[1 : len(data)-1]
	}

	if matched, _ := regexp.Match(`^\d+$`, data); matched {
		data = []byte(fmt.Sprintf("%sns", data))
	}
	x, err := time.ParseDuration(string(data))
	if err != nil {
		Log(ERROR, nil, "Duration.UnmarshalJSON", "data", data)
		return err
	}
	*d = Duration(x)
	return nil
}

// Defaults sets up some reasonable values.
//
// ToDo: Move to SystemParameters
func (c *Control) Defaults() *Control {
	c.NoTiming = false // Implied.
	c.Verbosity = EVERYTHING
	c.CheckForFactExpiration = true
	c.MaxFacts = 1000
	c.BindingsWarningLimit = 20
	c.UseDefaultVariableValue = true
	c.DefaultVariableValue = "undefined"
	return c
}

// DefaultControl makes a Control using Control.Defaults()
func DefaultControl() *Control {
	return (&Control{}).Defaults()
}

// Copy makes a shallow copy.
func (c *Control) Copy() *Control {
	var target Control
	target = *c
	return &target
}

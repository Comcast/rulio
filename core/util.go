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
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"time"
)

// ParseJSON parses a map from bytes.
func ParseJSON(ctx *Context, bs []byte) (map[string]interface{}, error) {
	var pattern map[string]interface{}
	err := json.Unmarshal(bs, &pattern)
	if err != nil {
		// convert golang error to rules specific one for proper error handling
		err = NewSyntaxError(err.Error())
		Log(UERR, ctx, "core.ParseJSON", "error", err, "bs", string(bs))
	}
	return pattern, err
}

// ParseJSONString parses a map from a string.
func ParseJSONString(ctx *Context, s string) (map[string]interface{}, error) {
	m, err := ParseJSON(ctx, []byte(s))
	return m, err
}

// StringSet represents a set of strings.
//
// A StringSet is not synchronized.
type StringSet map[string]struct{}

// NewStringSet does what you'd expect.
func NewStringSet(xs []string) StringSet {
	ss := make(StringSet)
	if xs != nil {
		for _, x := range xs {
			ss.Add(x)
		}
	}
	return ss
}

// EmptyStringSet makes one of those.
func EmptyStringSet() StringSet {
	ss := make(StringSet)
	return ss
}

// Nothing really is nothing.
var Nothing = struct{}{}

// Add adds the given string to the set.
func (s StringSet) Add(x string) StringSet {
	s[x] = Nothing
	return s
}

// AddAll adds all elements of the given set to the set.
func (s StringSet) AddAll(more StringSet) StringSet {
	for x, _ := range more {
		s.Add(x)
	}
	return s
}

func (s StringSet) AddStrings(xs ...string) StringSet {
	for _, x := range xs {
		s.Add(x)
	}
	return s
}

// Rem does what you'd think.
func (s StringSet) Rem(x string) StringSet {
	delete(s, x)
	return s
}

// Contains reports whether the given string is in the set
func (s StringSet) Contains(x string) bool {
	_, have := s[x]
	return have
}

// Insert removes elements not in the given set.
//
// The receiver is modified.
func (s StringSet) Intersect(t StringSet) {
	for x, _ := range s {
		_, have := t[x]
		if !have {
			delete(s, x)
		}
	}
}

func (xs StringSet) json() string {
	bs, err := json.Marshal(xs.Array())
	if err != nil {
		panic(err)
	}
	return string(bs)
}

// Array returns a pointer to an array of the set's elements.
func (xs StringSet) Array() []string {
	acc := make([]string, 0, len(xs))
	for x, _ := range xs {
		acc = append(acc, x)
	}
	return acc
}

// Difference returns a new set containing the elements in receiver
// that are not in the given set.
//
// No sets are harmed in this operation.
func (xs StringSet) Difference(ys StringSet) (StringSet, StringSet) {
	left := make(StringSet)
	right := make(StringSet)
	for x, _ := range xs {
		if !ys.Contains(x) {
			left.Add(x)
		}
	}
	for y, _ := range ys {
		if !xs.Contains(y) {
			right.Add(y)
		}
	}
	return left, right
}

func (s StringSet) UnmarshalJSON(data []byte) error {
	// Just a JSON array
	var xs []string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	s.AddStrings(xs...)
	return nil
}

func (s StringSet) MarshalJSON() ([]byte, error) {
	// Just a JSON array
	return json.Marshal(s.Array())
}

// Now returns the current time in UTC nanoseconds.
func Now() int64 {
	return time.Now().UTC().UnixNano()
}

// NowMicros returns the current time in UTC microseconds.
func NowMicros() int64 {
	return int64(Now() / 1000)
}

// NowSecs returns the current time in UTC seconds.
func NowSecs() int64 {
	return time.Now().UTC().Unix()
}

// Timestamp returns a string representing the given time in UTC in
// the 'RFC3339Nano' representation.
func Timestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// GetTimestamp makes a timestamp from UNIX nanoseconds.
func GetTimestamp(ts int64) string {
	return fmt.Sprintf("%s", time.Unix(ts/1000000, 0).Format(time.RFC3339Nano))
}

// NowString returns a string representing the current time in UTC in
// the 'RFC3339Nano' representation.
func NowString() string {
	return Timestamp(time.Now())
}

// NanoStringToMilliString drops microseconds from the string, which
// should be in the 'RFC3339Nano' representation.
func NanoStringToMilliString(ns string) string {
	if 24 > len(ns) {
		return ns + "Z"
	} else {
		return ns[0:23] + "Z"
	}
}

// IncCounter safely returns an increasing int.
func IncCounter() uint64 {
	return atomic.AddUint64(&IncCounterBase, uint64(1))
}

// IncCounterBase is the state for IncCounter().
var IncCounterBase = uint64(0)

// CheckErr is a utility function to log an error if any.
//
// Useful in goroutines or in other places where there is no caller to
// bother but something inconvenient might have occured.  Ideally, we
// never use this function.
func CheckErr(ctx *Context, op string, err error) {
	if err != nil {
		Log(ERROR, ctx, op, "error", err)
	}
}

// Inc atomically updates the given counter.
func Inc(p *uint64, d int64) {
	atomic.AddUint64(p, uint64(d))
}

// The Moscow Rules
//
// 1. Assume nothing.
// 2. Never go against your gut.
// 3. Everyone is potentially under opposition control.
// 4. Don't look back; you are never completely alone.
// 5. Go with the flow, blend in.
// 6. Vary your pattern and stay within your cover.
// 7. Lull them into a sense of complacency.
// 8. Don't harass the opposition.
// 9. Pick the time and place for action.
// 10. Keep your options open.
//
//   --International Spy Museum (via https://en.wikipedia.org/wiki/The_Moscow_rules)

// UUID probably returns a version 4 UUID.
//
// See https://groups.google.com/forum/#!topic/golang-nuts/Rn13T6BZpgE
var Random *os.File

func init() {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		log.Fatal(err)
	}
	Random = f
}

// UUID generates one using data from '/dev/urandom'.
func UUID() string {
	b := make([]byte, 16)
	Random.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Accumulator is sliding buffer.
//
// As it fills, older entries slide off the back.
//
// Not synchronized.
type Accumulator struct {
	// sync.Mutex

	// Acc is the buffer.
	Acc []interface{}

	// Limit is the capacity.
	Limit int

	// Dumped is the number of entries that have been dumped to
	// make room for other entries.
	Dumped int
}

// NewAccumulator returns an Accumulator with the given size.
func NewAccumulator(limit int) *Accumulator {
	buf := make([]interface{}, 0, limit)
	return &Accumulator{buf, limit, 0}
}

// Add adds the thing to the Accumulator.
//
// If there isn't room, then room.
func (acc *Accumulator) Add(x interface{}) {
	// acc.Lock()
	dump := len(acc.Acc) - acc.Limit
	if 0 < dump {
		acc.Acc = acc.Acc[dump:]
		acc.Dumped += dump
	}
	if len(acc.Acc) < acc.Limit {
		acc.Acc = append(acc.Acc, x)
	} else {
		acc.Dumped++
	}
	// acc.Unlock()
}

// Profile starts CPU and memory profiling and returns a function that will stop that.
//
//
// Writes "cpu" + filename and "mem" + filename.
//
// Usage:
//
//   defer Profile("logfast.prof")()
//
func Profile(filename string) func() {
	cpu, err := os.Create("cpu" + filename)
	if err != nil {
		panic(err)
	}

	mem, err := os.Create("mem" + filename)
	if err != nil {
		panic(err)
	}

	then := time.Now()
	log.Printf("Profile starting %s\n", filename)
	pprof.StartCPUProfile(cpu)
	pprof.WriteHeapProfile(mem)

	return func() {
		elapsed := time.Now().Sub(then)
		log.Printf("Profile stopping %s (elapsed %v)\n", filename, elapsed)
		pprof.StopCPUProfile()
		if err := mem.Close(); err != nil {
			log.Printf("error closing mem profile (%v)", err)
		}
	}
}

// Gorep returns a string that represents the given thing in Go --
// except for plain strings.
//
// This function is used in logging generic data.  All log records
// should have consistent types for a given property value.  If
// property can actually have different values, use this function to
// homogenize the values.  This function is slow and otherwise
// distasteful, but perhaps it's better than nothing.
func Gorep(x interface{}) string {
	if s, ok := x.(string); ok {
		return s
	} else {
		return fmt.Sprintf("%#v", x)
	}
}

// UseCores will use all cores unless the environment variable
// 'GOMAXPROCS' is set.
//
// If 'silent', then do not make a 'Log()' call.
//
// There is a proposal for Go 1.5 to make GOMAXPROCS default to the
// number of available cores.
func UseCores(ctx *Context, silent bool) {
	cores := os.Getenv("GOMAXPROCS")
	if cores == "" {
		n := runtime.NumCPU()
		if !silent {
			Log(INFO, ctx, "UseCores", "cores", n, "from", "NumCPU")
		}
		runtime.GOMAXPROCS(n)
		cores = strconv.Itoa(n)
	} else {
		if !silent {
			Log(INFO, ctx, "UseCores", "cores", cores, "from", "env")
		}
	}
}

// StructToMap converts a struct to a map.
func StructToMap(x interface{}) (map[string]interface{}, error) {
	m := make(map[string]interface{})

	v := reflect.ValueOf(x)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("not a struct (%T)", v)
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		p := t.Field(i).Name
		m[p] = v.Field(i).Interface()
	}
	return m, nil
}

// TextContextWithLocation creates a location with indexed state on memory
// storage.  Also returns a Context with its location se.
func TestContextWithLocation(name string) (*Context, error) {
	ctx := TestContext(name)
	store, err := NewMemStorage(ctx)
	if err != nil {
		return nil, err
	}
	state, err := NewIndexedState(ctx, name, store)
	if err != nil {
		return nil, err
	}
	loc, err := NewLocation(ctx, name, state, nil)
	if err != nil {
		return nil, err
	}
	ctx.SetLoc(loc)

	return ctx, nil
}

// CoerceFakeFloats will make ints out of floats when possible.
//
// Since al JSON all numbers are floats, big "integers" can cause
// trouble. Example: millisecond timestamps get float representation:
// 1416505007395 becomes 1.416505007395e+12, which we don't want.
//
// Rather than using json.Decode with UseNumber, which results in
// numbers of the type json.Number (which could cause trouble with
// reflective code), we coerce what we can outbound.
//
// Inspirational code from Boris.
func CoerceFakeFloats(x interface{}) interface{} {
	switch v := x.(type) {
	case float64:
		if v == float64(int(v)) {
			x = int(v)
		}
	case map[string]interface{}:
		for p, val := range v {
			v[p] = CoerceFakeFloats(val)
		}
	}
	return x
}

// IpAddresses tries to find a machine's (non-loopback) network
// interfaces.
//
// 127.0.0.1 and ::1 are not included in the results.
//
// Uses 'net.Interfaces()'.
func IpAddresses() ([]string, error) {
	ifaces, err := net.Interfaces()
	acc := make([]string, 0, 0)
	if err != nil {
		return acc, err
	}

	skip := func(s string) bool {
		switch s {
		case "127.0.0.1", "::1":
			return true
		default:
			return false
		}
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			return acc, err
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				s := v.IP.String()
				if !skip(s) {
					acc = append(acc, s)
				}
			case *net.IPAddr:
				s := v.IP.String()
				if !skip(s) {
					acc = append(acc, s)
				}
			}
		}
	}
	return acc, nil
}

// IpAddress tries to return a (non-loopback) IP address for this machine.
//
// If it can't find one, it returns 127.0.0.1.
//
// Exactly what this function returns is not really defined.  Don't
// rely on it for anything important.
func IpAddress() string {
	ips, _ := IpAddresses()
	if len(ips) == 0 {
		return "127.0.0.1"
	}
	return ips[0]
}

// AnIpAddress is maybe an IP address for this machine.
//
// Will try to find a non-loopback interface.  Failing that, it's
// 127.0.0.1.
var AnIpAddress = IpAddress()

// ISlice attempts to convert the given thing to an array of interface{}s.
//
// If the given thing isn't array, the thing is just returned.
//
// Uses reflection.
func ISlice(xs interface{}) (interface{}, bool) {
	v := reflect.ValueOf(xs)
	switch v.Kind() {
	case reflect.Slice:
		acc := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			acc[i] = v.Index(i).Interface()
		}
		return acc, true
	}
	return v, false
}

// Map is a generic event, pattern, or fact.
//
// Kinda wants to be a transparent typedef.
type Map map[string]interface{}

// ParseMap tries to parse a Map from JSON.
func ParseMap(js string) (m Map, err error) {
	err = json.Unmarshal([]byte(js), &m)
	return m, nil
}

func (m Map) JSON() (string, error) {
	bs, err := json.Marshal(&m)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func Who(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	return fmt.Sprintf("who %s %d %v", file, line, ok)
}

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

package sys

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	. "rulio/core"
	"rulio/cron"
	"rulio/storage/bolt"
	"rulio/storage/cassandra"
	"rulio/storage/dynamodb"
)

// LocToGroup maps a location name to a group name.
//
// Used to be called 'GroupMap'.
type LocToGroup func(string) string

// GroupControls maps a group to a 'core.Control'.
//
// Used to be called 'LocControls'
type GroupControls map[string]*Control

// System is a location container.
//
// A System is responsible for loading locations and directing API
// requests to then.
type System struct {
	sync.Mutex
	cron    cron.Cronner
	storage Storage
	config  SystemConfig
	control unsafe.Pointer // *SystemControl
	stats   ServiceStats
	*CachedLocations
	ProfCPUFile *os.File
}

// CachedLocations is just that: a location cache keyed by name.
//
// We have this struct in part to have a mutex dedicated to this
// state.
type CachedLocations struct {
	sync.Mutex
	locs map[string]*CachedLocation
}

func NewCachedLocations(ctx *Context) *CachedLocations {
	return &CachedLocations{
		sync.Mutex{},
		make(map[string]*CachedLocation),
	}
}

func (cl *CachedLocations) Count() int {
	cl.Lock()
	n := len(cl.locs)
	cl.Unlock()
	return n
}

// expire checks the cache for the given location and determines if
// the cache value has expired.  Either way, returns the location (if
// any).  If the cached location has expired, it is removed from the
// cache.
//
// A cached location is expired if it is not pending and its
// expiration time is before the current time.
//
// Assumes a lock for the CachedLocations.
func (cls *CachedLocations) expire(ctx *Context, sys *System, name string, released bool) (*Location, bool) {
	// Assumes lock
	Log(INFO, ctx, "CachedLocations.expire", "name", name)
	cl, have := cls.locs[name]
	var loc *Location
	dead := false
	if have {
		cl.Lock()
		cl.Pending = !released
		Log(INFO, ctx, "CachedLocations.expire", "name", name, "cached", "exists")
		if cl.Pending || cl.Expires.After(time.Now()) {
			Log(INFO, ctx, "CachedLocations.expire", "name", name, "cached", "live")
			loc = cl.Location
		} else {
			Log(INFO, ctx, "CachedLocations.expire", "name", name, "cached", "expired")
			delete(cls.locs, name)
		}
		cl.Unlock()
	}
	return loc, dead
}

// Open gets a location from the cache after first creating it.
//
// This function the top-level location cache API, and it uses
// 'CachedLocations.Get()' to do the real work.
//
// TTL can be 'Never', 'Forever', or anything in between.
func (cls *CachedLocations) Open(ctx *Context, sys *System, name string, check bool) (*Location, error) {
	Log(INFO, ctx, "CachedLocations.Open", "name", name)
	cls.Lock()

	loc, dead := cls.expire(ctx, sys, name, false)

	var err error
	if loc == nil || dead {
		Log(INFO, ctx, "CachedLocations.Open", "name", name, "cached", "empty")
		ctl := sys.Control()
		ttl := ctl.LocationTTL

		expires := EndOfTime
		if ttl != Forever {
			expires = time.Now().Add(ttl)
		}
		Log(INFO, ctx, "CachedLocations.Open", "name", name, "expires", expires.String())
		cl := &CachedLocation{
			Expires: expires,
		}

		if ttl != Never || ctl.CachePending {
			cls.locs[name] = cl
		}

		// The clever (?) move here: now we only need a lock
		// for the given location (instead of system-wide
		// lock).  That's important because loading a location
		// can take a long time.  We'd like to be able to open
		// locations concurrently.
		cls.Unlock()
		return cl.Get(ctx, sys, name, check)
	}

	cls.Unlock()
	return loc, err
}

// Release checks whether the location has expired and, if so, closes
// it.
func (cls *CachedLocations) Release(ctx *Context, sys *System, name string) error {
	Log(INFO, ctx, "CachedLocations.Release", "name", name)
	var err error
	cls.Lock()
	loc, dead := cls.expire(ctx, sys, name, true)
	if dead {
		Log(INFO, ctx, "CachedLocations.Release", "name", name, "cached", "expired")
		if loc != nil {
			err = sys.CloseLocation(ctx, name)
		}
	} else {
		Log(INFO, ctx, "CachedLocations.Release", "name", name, "cached", "live")
	}
	cls.Unlock()
	return err
}

// CachedLocation mostly provides a mutex associated with the target
// location.
//
// That mutex allows use to block only on this specific location when
// opening it.  That's way better than having a system-wide lock held
// when opening each location because opening a location could take a
// long time.
type CachedLocation struct {
	sync.Mutex
	Expires time.Time
	Pending bool
	*Location
}

// OpenLocation wraps 'newLocation' to check for existence (optionally).
func (sys *System) OpenLocation(ctx *Context, name string, checkExists bool) (*Location, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.NewLocations, uint64(1))
	Log(INFO, ctx, "System.OpenLocation", "name", name, "checkExists", checkExists)
	timer := NewTimer(ctx, "SystemOpenLocation")
	loc, err := sys.newLocation(ctx, name)
	if err == nil {
		if checkExists {
			var created bool
			created, err = locationCreated(ctx, loc)
			if err == nil {
				if !created {
					err = NewNotFoundError("%s", name)
				}
			}
		}
	}
	if err != nil {
		Log(ERROR, ctx, "System.openLocation", "name", name, "error", err)
	}

	timer.Stop()
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return loc, sys.stats.IncErrors(err)
}

// Get returns the location after opening it once.
func (cl *CachedLocation) Get(ctx *Context, sys *System, name string, checkExists bool) (*Location, error) {
	Log(INFO, ctx, "CachedLocation.Get", "name", name, "checking", checkExists)
	cl.Lock()
	loc := cl.Location
	var err error
	if loc == nil {
		Log(INFO, ctx, "CachedLocation.Get", "name", name, "opening", true)
		loc, err = sys.OpenLocation(ctx, name, checkExists)
		if err != nil {
			Log(WARN, ctx, "CachedLocation.Get", "name", name, "when", "OpenLocation", "error", err)
		} else {
			cl.Location = loc

			// See if we have a 'cacheTTL' property.  If so, try to use it.

			x, have, err := loc.GetProp(ctx, "cacheTTL", 0)
			if err != nil {
				Log(WARN, ctx, "CachedLocation.Get", "name", name, "when", "GetProp", "error", err)
			} else if have {
				var ms int
				if err == nil {
					Log(WARN, ctx, "CachedLocation.Get", "name", name, "factTTL", x)
					switch vv := x.(type) {
					case float64:
						ms = int(vv)
					case int:
						ms = vv
					default:
						err = fmt.Errorf("%#v isn't a TTL (ms)", x)
					}
				}
				if err != nil {
					Log(WARN, ctx, "CachedLocation.Get", "name", name, "when", "cacheTTL", "error", err)
				} else {
					then := time.Now().Add(time.Duration(ms) * time.Millisecond)
					Log(DEBUG, ctx, "CachedLocation.Get", "name", name, "computedExpiration", then)
					cl.Expires = then
				}
			}

		}
	} else {
		Log(DEBUG, ctx, "CachedLocation.Get", "name", name, "opening", false)
		ctx.SetLoc(loc)
	}
	cl.Unlock()

	// Remove from cache if location does not exist so the cache does not explode
	if nil == cl.Location {
		sys.CachedLocations.Lock()
		delete(sys.CachedLocations.locs, name)
		sys.CachedLocations.Unlock()
	}

	return loc, err
}

// Control atomically gets the System's controls.
func (s *System) Control() *SystemControl {
	return (*SystemControl)(atomic.LoadPointer(&s.control))
}

// SetControl atomically gets the System's controls.
func (s *System) SetControl(control SystemControl) {
	atomic.StorePointer(&s.control, unsafe.Pointer(&control))
	// SetDefaultHTTPClientSpec(control.InsecureSkipVerify)
}

// LocControl gets a location controls.
//
// A location's controls originate with the System's controls, which
// have a 'DefaultLocControl' that's used if 'LocToGroup' together
// with 'GroupControls' don't provide the required controls.
func (s *System) LocControl(ctx *Context, loc string) *Control {
	sysCtl := s.Control()
	ctl := sysCtl.DefaultLocControl
	if sysCtl.LocToGroup != nil {
		g := sysCtl.LocToGroup(loc)
		Log(DEBUG, ctx, "System.LocControl", "location", loc, "group", g)
		if g != "" && sysCtl.GroupControls != nil {
			if o, has := sysCtl.GroupControls[g]; has {
				ctl = o
			}
		}
	}

	return ctl
}

// Forever is a location TTL that is infinite.
const Forever = -1 * time.Second

// Never is a location TTL that is effectively zero.
const Never = 0 * time.Second

// EndOfTime is not actually the end of time, but it is a long time from now.
//
// If you are wondering, this value is in the year 146138514283.
var EndOfTime = time.Unix(1<<62, 0)

// ParseLocationTTL tries to parse a duration.
//
// "never" parses to 'Never' and "forever" parses to 'Forever'.
// Otherwise you should provide a duration in 'time.Duration' syntax.
func ParseLocationTTL(ttl string) (time.Duration, error) {
	switch strings.ToLower(ttl) {
	case "forever":
		return Forever, nil
	case "never":
		return Never, nil
	default:
		return time.ParseDuration(ttl)
	}
}

// SystemConfig are read-only, boot-time settings for a System.
//
// Once specified, these settings cannot be changed.
type SystemConfig struct {
	// Type of storage: "dynamodb", "cassandra", "bolt", "memory", or "none"
	Storage string `json:"storage"`

	// How to configure storage.  Examples:
	//
	// For "cassandra", the nodes: []interface{}{"localhost:9042"}
	//   Those ports should talk CQL (for better or worse).
	//
	// For "bolt", the filename: "test"
	//
	StorageConfig interface{}

	// LinearState switches between IndexedState and LinearState.
	//
	// UnindexedState means LinearState.  Maybe I should just say
	// that.
	UnindexedState bool

	// CheckExistence requires that a location is explicitly
	// created before it can be used.
	CheckExistence bool
}

// ExampleConfig just generates a simple config for an in-memory-only
// System.
func ExampleConfig() *SystemConfig {
	conf := SystemConfig{}
	// conf.Storage = "bolt"
	// conf.StorageConfig = "test.db"
	conf.Storage = "memory"
	return &conf
}

// checkingExistence now just looks to sys.config.CheckExistence.
func (sys *System) checkingExistence(ctx *Context) bool {
	return sys.config.CheckExistence
}

// GetStorage attempts to create a Storage.
//
// See '../storage/' for some implementations.
//
// Possible storage types:
//
//   "none": This Storage doesn't remember anything.
//   "mem": In-memory-only storage.  Never writes to disk.
//   "bolt": BoltDB Storage.  'storageConfig' should be a filename.
//   "dynamodb": DynamoDB Storage. 'storageConfig' should be
//      REGION:TABLE_NAME:CONSISTENT_READS, where CONSISTENT_READS is
//      either 'true' or 'false'.
//
func GetStorage(ctx *Context, storageType string, storageConfig interface{}) (Storage, error) {
	Log(INFO, ctx, "System.GetStorage", "storageType", storageType, "storageConfig", storageConfig)

	switch storageType {
	case "none":
		return NewNoStorage(ctx)

	case "memory", "mem":
		return NewMemStorage(ctx)

	case "cassandra":
		nodes := storageConfig
		switch nodes.(type) {
		case []interface{}:
			is := nodes.([]interface{})
			ns := make([]string, 0, len(is))
			for _, n := range is {
				ns = append(ns, n.(string))
			}
			Log(INFO, ctx, "core.GetStorage", "nodes", ns)
			return cassandra.NewStorage(ctx, ns)
		default:
			return nil, fmt.Errorf("bad type for nodes %v (%T); should be []interface{}.", nodes, nodes)
		}

	case "bolt":
		filename := storageConfig
		switch s := filename.(type) {
		case string:
			return bolt.NewStorage(ctx, s)
		default:
			return nil,
				fmt.Errorf("Bad type for filenames %v (%T); should be a string", filename, filename)
		}

	case "dynamodb":
		region := storageConfig
		switch vv := region.(type) {
		case string:
			config, err := dynamodb.ParseConfig(vv)
			if err != nil {
				return nil, err
			}
			return dynamodb.GetStorage(ctx, *config)
		default:
			return nil, fmt.Errorf("Bad type for DynamoDB region %v (%T); should be string.",
				storageConfig,
				storageConfig)
		}
	default:
		return nil, fmt.Errorf("Unknown storage '%s'", storageType)
	}

}

// SystemControl represents ephemeral control options.
//
// These settings are process-specific and not stored.  Note that all
// of these values are simple (not maps or arrays or structs).  You
// can change them (hopefully atomically) at will.
//
// Set and get these controls with 'System.SetControl()' and
// 'System.Control()'.
//
// Also see core.Control.
type SystemControl struct {
	// Turn on some timing in various places.
	Timing bool

	// The maximum number of locations that this System will serve.
	MaxLocations int

	// Whether to skip ssh/https insecure keys verification, should only be set true for testing
	InsecureSkipVerify bool

	// LocationTTL is the TTL for cached locations.
	//
	// Often either Forever or Never.
	LocationTTL time.Duration

	// LocToGroup maps a location to a group.
	LocToGroup `json:"-"`

	// GroupControls maps a group to a location control.
	GroupControls

	// DefaultLocationControl is exactly what you think.
	//
	// If LocToGroup and GroupControls don't find a control, we
	// use 'DefaultLocationControl'.
	DefaultLocControl *Control

	// CachedPending will use the location cache to keep pending
	// locations around.
	CachePending bool
}

// ExampleSystemControl uses default SystemControl fields, except for
// Timing, which is turned on.
func ExampleSystemControl() *SystemControl {
	control := SystemControl{}
	control.Timing = true
	return &control
}

// OverlayControl parses the given JSON as a SystemControl, which is
// then added to the given control.
func OverlayControl(js []byte, control SystemControl) (*SystemControl, error) {
	err := json.Unmarshal(js, &control)
	if err != nil {
		return nil, err
	}
	return &control, nil
}

func ExampleSystem(name string) (*System, *Context) {
	ctx := NewContext(name)
	return SimpleSystem(ctx), ctx
}

func SystemForTest(name string) (*System, *Context) {
	ctx := TestContext(name)
	return SimpleSystem(ctx), ctx
}

// SimpleSystem ia a basic system with 'FOREVER' TTL and 'DefaultVerbosity' Verbosity.
func SimpleSystem(ctx *Context) *System {
	conf := ExampleConfig()
	cont := ExampleSystemControl()
	cont.LocationTTL = Forever
	// ToDo: Expose this internal cron limit
	cr, _ := cron.NewCron(nil, time.Second, "intcron", 1000000)
	go cr.Start(ctx)
	internalCron := &cron.InternalCron{Cron: cr}

	locCtl := &Control{
		MaxFacts:  1000,
		Verbosity: DefaultVerbosity,
	}
	cont.DefaultLocControl = locCtl

	sys, err := NewSystem(ctx, *conf, *cont, internalCron)
	if err != nil {
		panic(err)
	}
	return sys
}

// BenchSystem makes a System that might be appropriate for
// benchmarks.
//
// Verbosity is 'NOTHING'.  Based on 'ExampleSystem'.
func BenchSystem(name string) (*System, *Context) {
	sys, _ := ExampleSystem(name)
	ctl := sys.Control()
	ctl.MaxLocations = 100000
	ctl.Timing = false
	locCtl := &Control{
		NoTiming:  true,
		Verbosity: NOTHING,
		MaxFacts:  100000,
	}
	ctl.DefaultLocControl = locCtl
	return sys, BenchContext(name)
}

// GetRuntimes get a list of current the System's runtime data.
func GetRuntimes(ctx *Context) (map[string]interface{}, error) {
	m := make(map[string]interface{})
	// m["locationCount"] = len(ctx.Sys.locationsMap)
	m["goroutines"] = runtime.NumGoroutine()
	m["cgos"] = runtime.NumCgoCall()
	m["goversion"] = runtime.Version()

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m["memstats"] = memStats

	return m, nil
}

// RuntimeLogLoop runs a loop that logs Go runtime stats.
//
// The "scope" parameter is opaque.  Might be useful in downstream
// log/metric processing.
func (sys *System) RuntimeLogLoop(level LogLevel, ctx *Context, scope string, interval time.Duration, iterations int) {
	lastGC := uint64(0)
	for i := 0; iterations < 0 || i < iterations; i++ {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		last := mem.LastGC
		lastGCDelta := last - lastGC
		lastGC = last

		Log(level, ctx, "System.RuntimeLogLoop",
			"scope", scope,
			"goroutines", runtime.NumGoroutine(),
			"Alloc", mem.Alloc,
			"BuckHashSys", mem.BuckHashSys,
			"Frees", mem.Frees,
			"GCSys", mem.GCSys,
			"LastGCDelta", lastGCDelta/1000000, // millis
			"HeapAlloc", mem.HeapAlloc,
			"HeapIdle", mem.HeapIdle,
			"HeapInuse", mem.HeapInuse,
			"HeapObjects", mem.HeapObjects,
			"HeapReleased", mem.HeapReleased,
			"HeapSys", mem.HeapSys,
			"Lookups", mem.Lookups,
			"MCacheInuse", mem.MCacheInuse,
			"MCacheSys", mem.MCacheSys,
			"MSpanInuse", mem.MSpanInuse,
			"MSpanSys", mem.MSpanSys,
			"Mallocs", mem.Mallocs,
			"NumGC", mem.NumGC,
			"OtherSys", mem.OtherSys,
			"PauseTotalNs", mem.PauseTotalNs,
			"StackInuse", mem.StackInuse,
			"StackSys", mem.StackSys,
			"Sys", mem.Sys,
			"TotalAlloc", mem.TotalAlloc,
			"PauseNs0", mem.PauseNs[0],
			"PauseNs1", mem.PauseNs[1],
			"PauseNs2", mem.PauseNs[2],
			"PauseNs3", mem.PauseNs[3])

		time.Sleep(interval)
	}
}

// GetStats get a clean copy of the System's current stats.
func (sys *System) GetStats(ctx *Context) (*ServiceStats, error) {
	Log(INFO, ctx, "System.GetStats")
	return sys.stats.Clone(), nil
}

// ClearStats clear the System's stats.
func (sys *System) ClearStats(ctx *Context) error {
	Log(INFO, ctx, "System.ClearStats")
	sys.stats = ServiceStats{}
	return nil
}

// LogLoop starts a loop that logs sys.stats at the given interval.
//
// If iterations is negative, the loop is endless.  Otherwise the loop
// terminates after the specified number of iteratins.
func (sys *System) LogLoop(level LogLevel, ctx *Context, scope string, interval time.Duration, iterations int) {
	sys.stats.LogLoop(level, ctx, scope, interval, iterations)
}

// NewSystem does about what you'd think.
func NewSystem(ctx *Context, conf SystemConfig, cont SystemControl, cron cron.Cronner) (*System, error) {
	Log(INFO, ctx, "NewSystem")
	if !cron.Persistent() && cont.LocationTTL != Forever && os.Getenv("RULES_CRON_OVERRIDE") == "" {
		err := errors.New("can't use an ephemeral cron and finite location TTLs")
		Log(WARN, ctx, "NewSystem", "error", err)
		return nil, err
	}
	if !cont.CachePending {
		Log(WARN, ctx, "NewSystem", "forcingCachePending", true)
		cont.CachePending = true
	}
	sys := &System{
		config:          conf,
		cron:            cron,
		CachedLocations: NewCachedLocations(ctx),
	}
	Log(DEBUG, ctx, "DEBUG.NewSystem", "sys", *sys)
	sys.SetControl(cont)

	SystemParameters.DefaultControl = cont.DefaultLocControl
	SystemParameters.Log(ctx)
	return sys, nil
}

// GetCachedLocations gets the set of cached locations.
//
// Gets the System's mutex.
func (sys *System) GetCachedLocations(ctx *Context) []string {
	cls := sys.CachedLocations
	cls.Lock()
	acc := make([]string, 0, len(cls.locs))
	for name, _ := range cls.locs {
		acc = append(acc, name)
	}
	cls.Unlock()
	return acc
}

func (sys *System) ensureStorage(ctx *Context) (Storage, error) {
	// Assumes we have the sys lock
	if sys.storage != nil {
		return sys.storage, nil
	}
	config := sys.config
	Log(INFO, ctx, "System.ensureStorage", "storage", config.Storage, "config", config.StorageConfig)
	storage, err := GetStorage(ctx, config.Storage, config.StorageConfig)
	if err != nil {
		Log(ERROR, ctx, "System.ensureStorage", "error", err)
		return nil, err
	}
	sys.storage = storage
	return storage, nil
}

// Close should shut down things.
//
// Currently this method just calls 'storage.Close()', which itself
// might not really do anything (depending on the Storage, of course).
func (sys *System) Close(ctx *Context) error {
	Log(INFO, ctx, "System.Close")
	if sys.storage != nil {
		err := sys.storage.Close(ctx)
		sys.storage = nil // ?
		return err
	}
	return nil
}

// newLocation is a primitive function to load a location in
// isolation.  No caches are touched and no checks are made.  Just
// create a location from storage.
//
// See 'openLocation()', which wraps this function to check for
// existence (optionally).  You probably don't want to call this
// function.
func (sys *System) newLocation(ctx *Context, name string) (*Location, error) {

	storage, err := sys.ensureStorage(ctx)
	if err != nil {
		return nil, err
	}

	var state State
	if sys.config.UnindexedState {
		Log(INFO, ctx, "System.newLocation", "stateType", "linear")
		state, err = NewLinearState(ctx, name, storage)
	} else {
		Log(INFO, ctx, "System.newLocation", "stateType", "indexed")
		state, err = NewIndexedState(ctx, name, storage)
	}

	if err != nil {
		return nil, err
	}

	cron.AddHooks(ctx, sys.cron, state)

	loc, err := NewLocation(ctx, name, state, sys.LocControl(ctx, name))
	if err != nil {
		Log(ERROR, ctx, "System.newLocation", "error", err)
		return nil, err
	}

	loc.Provider = sys

	return loc, nil
}

// CreateLocation exists solely to mark a location as created.
func (sys *System) CreateLocation(ctx *Context, location string) (bool, error) {
	Log(INFO, ctx, "System.CreateLocation", "location", location)
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))

	loc, err := sys.findLocation(ctx, location, false)
	ctx.SetLoc(loc)

	var exists bool
	if err != nil {
		Log(ERROR, ctx, "System.CreateLocation", "error", err, "location", location, "when", "findLocation")
	} else {
		exists, err = locationCreated(ctx, loc)
		if err != nil {
			Log(ERROR, ctx, "System.CreateLocation", "error", err, "location", location, "when", "locationCreated")
		} else {
			Log(INFO, ctx, "System.CreateLocation", "location", location, "exists", exists)
			if !exists {
				err = markLocationCreated(ctx, loc)
				if err != nil {
					Log(ERROR, ctx, "System.CreateLocation", "error", err, "location", location, "when", "markLocationCreated")
				}
			}
		}
	}

	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return !exists, sys.stats.IncErrors(err)
}

// createdMarker is the name of the property used to mark that a
// location has been created.
const createdMarker = "createdAt"

func locationCreated(ctx *Context, loc *Location) (bool, error) {
	_, have, err := loc.GetPropString(ctx, createdMarker, "")
	Log(DEBUG, ctx, "System.locationCreated", "name", loc.Name, "have", have, "err", err)
	return have, err
}

func markLocationCreated(ctx *Context, loc *Location) error {
	marked, err := locationCreated(ctx, loc)
	if err != nil {
		return err
	}
	if marked {
		return nil
	}
	err = loc.SetProp(ctx, "", createdMarker, NowString())
	Log(DEBUG, ctx, "System.markLocationCreated", "name", loc.Name, "err", err)
	return err
}

func legalFact(ctx *Context, fact string) error {
	return legalFactWithout(ctx, fact, createdMarker)
}

// legalFact will return an error if the fact includes the given
// property at the top level.
func legalFactWithout(ctx *Context, fact string, prop string) error {
	if 0 < strings.Index(fact, prop) {
		// Little optimization
		var m map[string]interface{}
		err := json.Unmarshal([]byte(fact), &m)
		if err != nil {
			return err
		}
		if _, have := m[prop]; have {
			return fmt.Errorf("property '%s' not allowed at top level", prop)
		}
	}
	return nil
}

// GetLocation implements core.LocationProvider.
//
// Just calls 'findLocation(,,false)'.
func (sys *System) GetLocation(ctx *Context, name string) (*Location, error) {
	return sys.findLocation(ctx, name, false)
}

// findLocation is the main function for getting a location.
//
// This function delegates the hard work to 'sys.CachedLocations.Open()'.
func (sys *System) findLocation(ctx *Context, name string, check bool) (*Location, error) {
	Log(DEBUG, ctx, "System.findLocation", "name", name, "check", check)
	check = check && sys.checkingExistence(ctx)
	return sys.CachedLocations.Open(ctx, sys, name, check)
}

// releaseLocation is the counterpart to 'findLocation'.  Call this
// function to let the cache know that you are done with some work for
// a location.
func (sys *System) releaseLocation(ctx *Context, name string) error {
	Log(INFO, ctx, "System.findLocation", "name", name)
	return sys.CachedLocations.Release(ctx, sys, name)
	// return sys.CloseLocation(ctx, name)
}

// CloseLocation currently does nothing.
func (sys *System) CloseLocation(ctx *Context, loc string) error {
	return nil
}

// GetSize returns the number of facts (including rules) stored in the given location.
func (sys *System) GetSize(ctx *Context, location string) (int, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.GetSize", "location", location)
	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var n int
	if err == nil {
		n, err = loc.StateSize(ctx)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return n, sys.stats.IncErrors(err)
}

// GetLastUpdatedMem reports what this system thinks the location's
// last updated timestamp is.
//
// Does not query DynamoDB to find out.  Just returns what is
// currently known in memory.
func (sys *System) GetLastUpdatedMem(ctx *Context, location string) (string, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.GetSize", "location", location)
	loc, err := sys.findLocation(ctx, location, true)
	updated := ""
	if err == nil {
		updated = loc.Updated(ctx)
	}
	sys.releaseLocation(ctx, location)
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return updated, sys.stats.IncErrors(err)
}

// AddFact add a fact to the given location.  Returns the ID for the fact.
func (sys *System) AddFact(ctx *Context, location string, id string, fact string) (string, error) {
	m, err := ParseMap(fact)
	if err != nil {
		return id, err
	}

	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.AddFacts, uint64(1))
	Log(INFO, ctx, "System.AddFact", "location", location, "id", id, "factjs", fact)
	timer := NewTimer(ctx, "SystemAddFact")
	defer timer.Stop()

	var loc *Location
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		if err = legalFact(ctx, fact); err != nil {
			Log(UERR, ctx, "System.AddFact", "error", err, "location", location, "id", id, "factjs", fact)
		} else {
			Metric(ctx, "System.AddFact", "AddFact", "location", location, "id", id, "factjs", fact)
			id, err = loc.AddFact(ctx, id, m)
		}
	}
	if err != nil {
		Log(ERROR, ctx, "System.AddFact", "error", err, "location", location, "id", id, "factjs", fact)
	}

	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return id, sys.stats.IncErrors(err)
}

func (sys *System) AddFactJS(ctx *Context, location string, id string, fact string) map[string]interface{} {
	id, err := sys.AddFact(ctx, location, id, fact)
	acc := make(map[string]interface{})
	if err == nil {
		acc["id"] = id
	} else {
		acc["error"] = err.Error()
	}
	return acc
}

// RemFact remove a fact from the given location.  Returns the ID of the removed fact.
func (sys *System) RemFact(ctx *Context, location string, id string) (string, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.RemFacts, uint64(1))
	Log(INFO, ctx, "System.RemFact", "location", location, "id", id)
	timer := NewTimer(ctx, "SystemRemFact")
	defer timer.Stop()

	var err error
	var loc *Location
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.RemFact", "location", location, "id", id)
		id, err = loc.RemFact(ctx, id)
	}
	if err != nil {
		Log(ERROR, ctx, "System.RemFact", "error", err, "location", location, "id", id)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return id, sys.stats.IncErrors(err)
}

// GetFact returns JSON representation of a fact from the given location.
func (sys *System) GetFact(ctx *Context, location string, id string) (js string, err error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.GetFacts, uint64(1))
	Log(INFO, ctx, "System.GetFact", "location", location, "id", id)
	timer := NewTimer(ctx, "SystemGetFact")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.GetFact", "GetFact", "location", location, "id", id)
		var m Map
		m, err = loc.GetFact(ctx, id)
		if err == nil {
			js, err = m.JSON()
		}
	}
	if err != nil {
		Log(ERROR, ctx, "System.GetFact", "error", err, "location", location, "id", id)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return js, sys.stats.IncErrors(err)
}

// AddRule adds a rule to the given location.  Returns the ID for the rule.
func (sys *System) AddRule(ctx *Context, location string, id string, rule string) (string, error) {
	m, err := ParseMap(rule)
	if err != nil {
		return id, err
	}

	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.AddRules, uint64(1))
	Log(INFO, ctx, "System.AddRule", "location", location, "ruleId", id, "rule", rule)
	timer := NewTimer(ctx, "SystemAddRule")
	defer timer.Stop()

	var loc *Location
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.AddRule", "AddRule", "location", location, "ruleId", id, "rule", rule)
		id, err = loc.AddRule(ctx, id, m)
	}
	if err != nil {
		Log(ERROR, ctx, "System.AddRule", "location", location, "ruleId", id, "rule", rule, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return id, sys.stats.IncErrors(err)
}

// EnableRule enables or disables a rule.
func (sys *System) EnableRule(ctx *Context, location string, id string, enable bool) error {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.EnableRule", "location", location, "ruleId", id, "enable", enable)
	var err error
	var loc *Location
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.EnableRule", "location", location, "ruleId", id, "enable", enable)
		err = loc.EnableRule(ctx, id, enable)
	}
	if err != nil {
		Log(ERROR, ctx, "System.EnableRule", "location", location, "ruleId", id, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return sys.stats.IncErrors(err)
}

// EnableRule enables or disables a rule.
func (sys *System) RuleEnabled(ctx *Context, location string, id string) (bool, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.RuleEnabled", "location", location, "ruleId", id)
	var err error
	var loc *Location
	var enabled bool
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.RuleEnabled", "location", location, "ruleId", id)
		enabled, err = loc.RuleEnabled(ctx, id)
	}
	if err != nil {
		Log(ERROR, ctx, "System.RuleEnabled", "location", location, "ruleId", id, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return enabled, sys.stats.IncErrors(err)
}

// RemRule removes a rule from the given location.  Returns the ID of the removed rule.
func (sys *System) RemRule(ctx *Context, location string, id string) (string, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.RemRules, uint64(1))
	Log(INFO, ctx, "System.RemRule", "location", location, "id", id)
	timer := NewTimer(ctx, "SystemRemRule")
	defer timer.Stop()

	var err error
	var loc *Location
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.RemRule", "location", location, "id", id)
		id, err = loc.RemRule(ctx, id)
	}
	if err != nil {
		Log(ERROR, ctx, "System.RemRule", "location", location, "id", id, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return id, sys.stats.IncErrors(err)
}

// GetRule returns JSON representation of a rule from the given location.
func (sys *System) GetRule(ctx *Context, location string, id string) (js string, err error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.GetRules, uint64(1))
	Log(INFO, ctx, "System.GetRule", "location", location, "id", id)
	timer := NewTimer(ctx, "SystemGetRule")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Metric(ctx, "System.GetRule", "location", location, "id", id)
		var m Map
		m, err = loc.GetRule(ctx, id)
		if err == nil {
			js, err = m.JSON()
		}
	}
	if err != nil {
		Log(ERROR, ctx, "System.GetRule", "location", location, "id", id, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return js, sys.stats.IncErrors(err)
}

// ClearLocation should (!) clear all the state for the given location.
func (sys *System) ClearLocation(ctx *Context, location string) error {
	// This method will skew TotalTime.
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.ClearLocation", "location", location)

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err != nil {
		Log(ERROR, ctx, "System.ClearLocation", "location", location, "error", err, "when", "findLocation")
	} else {
		Log(DEBUG, ctx, "System.ClearLocation", "location", location)
		Metric(ctx, "System.ClearLocation", "location", location)
		err = loc.Clear(ctx)
		if err != nil {
			Log(ERROR, ctx, "System.ClearLocation", "location", location, "error", err, "when", "clear")
		} else {
			Log(DEBUG, ctx, "System.ClearLocation", "location", location, "clear", "done")
		}
	}

	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return sys.stats.IncErrors(err)
}

// DeleteLocation should (!) clear all the state for the given location.
func (sys *System) DeleteLocation(ctx *Context, location string) error {
	// This method will skew TotalTime.
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.DeleteLocation", "location", location)

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err == nil {
		Log(WARN, ctx, "System.DeleteLocation", "location", location)
		Metric(ctx, "System.DeleteLocation", "DeleteLocation", "location", location)
		err = loc.Delete(ctx)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return sys.stats.IncErrors(err)
}

// SearchFacts finds facts that match the given pattern (JSON).
func (sys *System) SearchFacts(ctx *Context, location string, pattern string, includeInherited bool) (*SearchResults, error) {
	m, err := ParseMap(pattern)
	if err != nil {
		return nil, err
	}

	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.SearchFacts, uint64(1))
	Log(INFO, ctx, "System.SearchFacts", "location", location, "pattern", pattern)
	timer := NewTimer(ctx, "SystemSearchFacts")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var sr *SearchResults
	if err == nil {
		Metric(ctx, "System.SearchFacts", "location", location, "pat", pattern)
		sr, err = loc.SearchFacts(ctx, m, includeInherited)
	}
	if err != nil {
		Log(ERROR, ctx, "System.SearchFacts", "location", location, "pat", pattern, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return sr, sys.stats.IncErrors(err)
}

// Query runs a query in the given location.
//
// For testing.
func (sys *System) Query(ctx *Context, location string, query string) (*QueryResult, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.Query", "location", location, "query", query)
	timer := NewTimer(ctx, "SystemQuery")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var qr *QueryResult
	if err == nil {
		qr, err = loc.Query(ctx, query)
	}
	if err != nil {
		Log(ERROR, ctx, "System.Query", "location", location, "query", query, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return qr, sys.stats.IncErrors(err)
}

// SearchRules finds rules with 'when' patterns that match the given event (JSON).
func (sys *System) SearchRules(ctx *Context, location string, event string, includeInherited bool) (map[string]string, error) {
	m, err := ParseMap(event)
	if err != nil {
		return nil, err
	}

	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.SearchRules, uint64(1))
	Log(INFO, ctx, "System.SearchRules", "location", location, "event", event)
	timer := NewTimer(ctx, "SystemSearchRules")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var acc map[string]string
	if err == nil {
		Metric(ctx, "System.SearchRules", "location", location, "event", event)
		var rules map[string]Map
		rules, err = loc.SearchRules(ctx, m, includeInherited)
		if err == nil {
			acc = make(map[string]string, len(rules))
			for id, rule := range rules {
				var js string
				if js, err = rule.JSON(); err != nil {
					break
				}
				acc[id] = js
			}
		}
	}
	if err != nil {
		Log(ERROR, ctx, "System.SearchRules", "location", location, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return acc, sys.stats.IncErrors(err)
}

func (sys *System) ProcessEvent(ctx *Context, location string, event string) (*FindRules, error) {

	m, err := ParseMap(event)
	if err != nil {
		return nil, err
	}

	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.ProcessEvents, uint64(1))
	Log(INFO, ctx, "System.ProcessEvent", "location", location, "event", event)
	timer := NewTimer(ctx, "SystemProcessEvent")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err != nil {
		return nil, err
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	fr, cond := loc.ProcessEvent(ctx, m)
	if cond == nil || cond == Complete {
		err = nil
	} else {
		err = errors.New(cond.Msg)
	}
	return fr, err
}

func (sys *System) RetryEventWork(ctx *Context, location string, work *FindRules) error {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.ProcessEvents, uint64(1))
	Log(INFO, ctx, "System.RetryEventWork", "location", location, "work", *work)
	timer := NewTimer(ctx, "SystemRetryEventWork")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err != nil {
		return err
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return loc.RetryEventWork(ctx, work)
}

// ListRules returns all rules (JSON) stored in the given location.
func (sys *System) ListRules(ctx *Context, location string, includeInherited bool) ([]string, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	atomic.AddUint64(&sys.stats.ListRules, uint64(1))
	timer := NewTimer(ctx, "SystemListRules")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var ss []string
	if err == nil {
		Metric(ctx, "System.ListRules", "location", location)
		ss, err = loc.ListRules(ctx, includeInherited)
	}
	if err != nil {
		Log(ERROR, ctx, "System.ListRules", "location", location)
	}

	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return ss, sys.stats.IncErrors(err)
}

// GetParents gets the location's parents (if any).
func (sys *System) GetParents(ctx *Context, location string) ([]string, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.GetParents", "location", location)
	var loc *Location
	var err error
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var parents []string
	if err == nil {
		Metric(ctx, "System.GetParents", "location", location)
		parents, err = loc.GetParents(ctx)
	}
	if err != nil {
		Log(ERROR, ctx, "System.GetParents", "location", location, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return parents, sys.stats.IncErrors(err)
}

// SetParents sets the location's parents.
//
// Returns the id of the property (if any).
func (sys *System) SetParents(ctx *Context, location string, parents []string) (string, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	Log(INFO, ctx, "System.SetParents", "location", location, "parents", parents)
	var loc *Location
	var err error
	loc, err = sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	var id string
	if err == nil {
		Metric(ctx, "System.SetParents", "location", location)
		id, err = loc.SetParents(ctx, parents)
	}
	if err != nil {
		Log(ERROR, ctx, "System.SetParents", "location", location, "error", err)
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return id, sys.stats.IncErrors(err)
}

// GetLocationStats returns ServiceStats for the given location.
func (sys *System) GetLocationStats(ctx *Context, location string) (*ServiceStats, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err != nil {
		Log(INFO, ctx, "System.GetLocationStats", "location", location)
		return nil, err
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return loc.Stats(), nil
}

// ClearLocationStats clears stats for the given location.
func (sys *System) ClearLocationStats(ctx *Context, location string) error {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err != nil {
		Log(ERROR, ctx, "System.ClearLocationStats", "location", location, "error", err)
	}
	loc.ClearStats()
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return nil
}

// RunJavascript allows location-specific Javascript testing.
func (sys *System) RunJavascript(ctx *Context, location string, code string, libraries []string, bs *Bindings, props map[string]interface{}) (interface{}, error) {
	then := Now()
	atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
	timer := NewTimer(ctx, "SystemRunJavascript")
	defer timer.Stop()

	loc, err := sys.findLocation(ctx, location, true)
	defer sys.releaseLocation(ctx, location)
	if err != nil {
		Log(ERROR, ctx, "System.RunJavascript", "location", location, "error", err)
		return nil, err
	}
	x, err := loc.RunJavascript(ctx, code, libraries, bs, props)
	if nil != err {
		Log(ERROR, ctx, "System.RunJavascript", "location", location, "error", err)
		return nil, err
	}
	atomic.AddUint64(&sys.stats.TotalTime, uint64(Now()-then))
	return x, err
}

const ProfCPUFileName = "pprof_cpu"

// StartProfileCPU starts pprof cpu.
func (sys *System) StartProfileCPU(ctx *Context) error {
	Log(INFO, ctx, "System.StartProfileCPU")
	var err error
	sys.Lock()
	if nil == sys.ProfCPUFile {
		err = sys.startProfiling(ctx)
	}
	sys.Unlock()

	return err
}

func (sys *System) startProfiling(ctx *Context) error {
	Log(INFO, ctx, "System.StartProfiling")
	f, err := os.Create(ProfCPUFileName)
	if nil == err {
		pprof.StartCPUProfile(f)
		sys.ProfCPUFile = f
	}
	return err
}

func (sys *System) stopProfiling(ctx *Context) (err error) {
	Log(INFO, ctx, "System.StopProfiling")
	pprof.StopCPUProfile()
	err = sys.ProfCPUFile.Close()
	sys.ProfCPUFile = nil
	return err
}

// GetProfileCPU gets/stops pprof cpu.
func (sys *System) GetProfileCPU(ctx *Context, stop bool) (prof []byte, err error) {
	Log(INFO, ctx, "System.GetProfileCPU")
	sys.Lock()
	if nil == sys.ProfCPUFile {
		err = fmt.Errorf("no cpu profile available")
		Log(ERROR, ctx, "System.GetProfileCPU", "error", err)
	} else {
		sys.stopProfiling(ctx)
		prof, err = ioutil.ReadFile(ProfCPUFileName)
		if !stop {
			sys.startProfiling(ctx)
		}
		if err != nil {
			Log(ERROR, ctx, "System.GetProfileCPU", "error", err, "when", "readfile")
		}
	}
	sys.Unlock()

	return prof, err
}

// GetProfileMem returns pprof mem.
func (sys *System) GetProfileMem(ctx *Context) (string, error) {
	Log(INFO, ctx, "System.GetProfileMem")
	buf := new(bytes.Buffer)
	err := pprof.WriteHeapProfile(buf)
	if err != nil {
		Log(ERROR, ctx, "System.GetProfileMem", "error", err)
		return "", err
	}

	return buf.String(), nil
}

// GetProfileBlock returns pprof block.
func (sys *System) GetProfileBlock(ctx *Context) (string, error) {
	Log(INFO, ctx, "System.GetProfileBlock")
	profile := pprof.Lookup("block")
	if profile == nil {
		err := fmt.Errorf("No block profile")
		Log(ERROR, ctx, "System.GetProfileBlock", "error", err, "when", "lookup")
		return "", err
	}
	buf := new(bytes.Buffer)
	err := profile.WriteTo(buf, 0)
	if err != nil {
		Log(ERROR, ctx, "System.GetProfileBlock", "error", err, "when", "writ")
		return "", err
	}

	return buf.String(), nil
}

// PeekStorage is an unholy API to expose underlying storage.
//
// Used by 'service' for testing purposes.
func (sys *System) PeekStorage(ctx *Context) (Storage, error) {
	return sys.storage, nil
}

// ToDo: EnableLocation(location string, enabled bool)
// ToDo: LocationEnabled(location string) bool
// ToDo: QuitLocation(location string)

// ServiceAvailable checks to see if what's at 'url' is accessible and at least semi-functional.
//
// The URL should include the full path (ideally to a health check).
func ServiceAvailable(ctx *Context, url string, timeout time.Duration) bool {

	Log(INFO, ctx, "ServiceAvailable", "url", url)

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		Log(INFO, ctx, "ServiceAvailable", "url", url, "error", err, "available", false)
		Log(ERROR, ctx, "ServiceAvailable", "url", url, "error", err, "available", false)
		return false
	}

	if resp.StatusCode != 200 {
		Log(INFO, ctx, "ServiceAvailable", "url", url, "code", resp.StatusCode, "available", false)
		return false
	}

	Log(INFO, ctx, "ServiceAvailable", "url", url, "available", true)
	return true
}

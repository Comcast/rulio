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
	"sync/atomic"
	"time"
)

// ServiceStats contains statistics that a System and each Location
// can track.  Each stat should, I guess, be updated atomically:
//
//   atomic.AddUint64(&sys.stats.TotalCalls, uint64(1))
//
// Though of course one wonders about when that atomic call is really
// needed.
//
// Since we are using atomic ops instead of a struct-wide mutex (for
// speed), we will be unsure what the data really means.  ToDo:
// Reconsider.
//
// We also want to track stats on a per-app basis.  Code that uses
// this package could do that.
type ServiceStats struct {
	TotalCalls       uint64
	ErrorCount       uint64
	TotalTime        uint64
	NewLocations     uint64
	ListLocations    uint64
	AddRules         uint64
	UpdateRules      uint64
	RemRules         uint64
	GetRules         uint64
	AddFacts         uint64
	RemFacts         uint64
	GetFacts         uint64
	SearchRules      uint64
	SearchFacts      uint64
	ListRules        uint64
	ProcessEvents    uint64
	GetEventHistorys uint64
	ActionExecs      uint64
}

// Clone gives a clean copy of the given ServiceStats.
func (stats *ServiceStats) Clone() *ServiceStats {
	// Make a copy carefully.
	clone := ServiceStats{}
	clone.TotalCalls = atomic.LoadUint64(&stats.TotalCalls)
	clone.ErrorCount = atomic.LoadUint64(&stats.ErrorCount)
	clone.TotalTime = atomic.LoadUint64(&stats.TotalTime)
	clone.NewLocations = atomic.LoadUint64(&stats.NewLocations)
	clone.ListLocations = atomic.LoadUint64(&stats.ListLocations)
	clone.AddRules = atomic.LoadUint64(&stats.AddRules)
	clone.RemRules = atomic.LoadUint64(&stats.RemRules)
	clone.GetRules = atomic.LoadUint64(&stats.GetRules)
	clone.AddFacts = atomic.LoadUint64(&stats.AddFacts)
	clone.RemFacts = atomic.LoadUint64(&stats.RemFacts)
	clone.GetFacts = atomic.LoadUint64(&stats.GetFacts)
	clone.SearchRules = atomic.LoadUint64(&stats.SearchRules)
	clone.SearchFacts = atomic.LoadUint64(&stats.SearchFacts)
	clone.ListRules = atomic.LoadUint64(&stats.ListRules)
	clone.ProcessEvents = atomic.LoadUint64(&stats.ProcessEvents)
	clone.GetEventHistorys = atomic.LoadUint64(&stats.GetEventHistorys)
	clone.ActionExecs = atomic.LoadUint64(&stats.ActionExecs)
	return &clone
}

// Reset resets all fields to zeros.
//
// Stores are done with atomics, so ...
func (stats *ServiceStats) Reset() {
	// Make a copy carefully.
	atomic.StoreUint64(&stats.TotalCalls, 0)
	atomic.StoreUint64(&stats.TotalCalls, 0)
	atomic.StoreUint64(&stats.ErrorCount, 0)
	atomic.StoreUint64(&stats.TotalTime, 0)
	atomic.StoreUint64(&stats.NewLocations, 0)
	atomic.StoreUint64(&stats.ListLocations, 0)
	atomic.StoreUint64(&stats.AddRules, 0)
	atomic.StoreUint64(&stats.RemRules, 0)
	atomic.StoreUint64(&stats.GetRules, 0)
	atomic.StoreUint64(&stats.AddFacts, 0)
	atomic.StoreUint64(&stats.RemFacts, 0)
	atomic.StoreUint64(&stats.GetFacts, 0)
	atomic.StoreUint64(&stats.SearchRules, 0)
	atomic.StoreUint64(&stats.SearchFacts, 0)
	atomic.StoreUint64(&stats.ListRules, 0)
	atomic.StoreUint64(&stats.ProcessEvents, 0)
	atomic.StoreUint64(&stats.GetEventHistorys, 0)
	atomic.StoreUint64(&stats.ActionExecs, 0)
}

// Subtract subtracts the given stats from the receiver.
//
// The receiver is cloned first, and the updated clone is returned.
func (stats *ServiceStats) Subtract(y *ServiceStats) *ServiceStats {
	y = y.Clone()
	x := stats.Clone()
	x.TotalCalls -= y.TotalCalls
	x.ErrorCount -= y.ErrorCount
	x.TotalTime -= y.TotalTime
	x.NewLocations -= y.NewLocations
	x.ListLocations -= y.ListLocations
	x.AddRules -= y.AddRules
	x.RemRules -= y.RemRules
	x.GetRules -= y.GetRules
	x.AddFacts -= y.AddFacts
	x.RemFacts -= y.RemFacts
	x.GetFacts -= y.GetFacts
	x.SearchRules -= y.SearchRules
	x.SearchFacts -= y.SearchFacts
	x.ListRules -= y.ListRules
	x.ProcessEvents -= y.ProcessEvents
	x.GetEventHistorys -= y.GetEventHistorys
	x.ActionExecs -= y.ActionExecs
	return x
}

// Aggregate from given ServiceStats.
//
// Updates the receiver.
func (stats *ServiceStats) Aggregate(src *ServiceStats) {
	src = src.Clone()
	atomic.AddUint64(&stats.TotalCalls, src.TotalCalls)
	atomic.AddUint64(&stats.ErrorCount, src.ErrorCount)
	atomic.AddUint64(&stats.TotalTime, src.TotalTime)
	atomic.AddUint64(&stats.NewLocations, src.NewLocations)
	atomic.AddUint64(&stats.ListLocations, src.ListLocations)
	atomic.AddUint64(&stats.AddRules, src.AddRules)
	atomic.AddUint64(&stats.RemRules, src.RemRules)
	atomic.AddUint64(&stats.GetRules, src.GetRules)
	atomic.AddUint64(&stats.AddFacts, src.AddFacts)
	atomic.AddUint64(&stats.RemFacts, src.RemFacts)
	atomic.AddUint64(&stats.GetFacts, src.GetFacts)
	atomic.AddUint64(&stats.SearchRules, src.SearchRules)
	atomic.AddUint64(&stats.SearchFacts, src.SearchFacts)
	atomic.AddUint64(&stats.ListRules, src.ListRules)
	atomic.AddUint64(&stats.ProcessEvents, src.ProcessEvents)
	atomic.AddUint64(&stats.GetEventHistorys, src.GetEventHistorys)
	atomic.AddUint64(&stats.ActionExecs, src.ActionExecs)
}

// IncErrors increments stats.ErrorCount if err isn't nil.
func (stats *ServiceStats) IncErrors(err error) error {
	if err != nil {
		atomic.AddUint64(&stats.ErrorCount, 1)
	}
	return err
}

// Log emits a log line for the given stats.  The 'scope' is included
// in the log record.  Example scopes: 'system', location.Name.
func (stats *ServiceStats) Log(level LogLevel, ctx *Context, scope string) {
	copy := stats.Clone()
	Log(level, ctx, "ServiceStats.Log",
		"scope", scope,
		"TotalCalls", copy.TotalCalls,
		"ErrorCount", copy.ErrorCount,
		"TotalTime", copy.TotalTime,
		"NewLocations", copy.NewLocations,
		"ListLocations", copy.ListLocations,
		"AddRules", copy.AddRules,
		"RemRules", copy.RemRules,
		"GetRules", copy.GetRules,
		"AddFacts", copy.AddFacts,
		"RemFacts", copy.RemFacts,
		"GetFacts", copy.GetFacts,
		"SearchRules", copy.SearchRules,
		"SearchFacts", copy.SearchFacts,
		"ListRules", copy.ListRules,
		"ProcessEvents", copy.ProcessEvents,
		"GetEventHistorys", copy.GetEventHistorys,
		"ActionExecs", copy.ActionExecs)
}

// LogLoop starts a loop that calls stats.Log at the given interval.
//
// If iterations is negative, the loop is endless.  Otherwise the loop
// terminates after the specified number of iteratins.
func (stats *ServiceStats) LogLoop(level LogLevel, ctx *Context, scope string, interval time.Duration, iterations int) {
	previous := stats.Clone()
	for i := 0; iterations < 0 || i < iterations; i++ {
		time.Sleep(interval)
		latest := stats.Clone()
		latest.Log(level, ctx, scope)

		delta := latest.Subtract(previous)
		previous = latest

		namespace := ""
		Point(ctx, namespace, "TotalCalls", delta.TotalCalls, "Count")
		Point(ctx, namespace, "ErrorCount", delta.ErrorCount, "Count")
		Point(ctx, namespace, "TotalTime", delta.TotalTime/1000000000, "Seconds")
		Point(ctx, namespace, "NewLocations", delta.NewLocations, "Count")
		Point(ctx, namespace, "ListLocations", delta.ListLocations, "Count")
		Point(ctx, namespace, "AddRules", delta.AddRules, "Count")
		Point(ctx, namespace, "RemRules", delta.RemRules, "Count")
		Point(ctx, namespace, "GetRules", delta.GetRules, "Count")
		Point(ctx, namespace, "AddFacts", delta.AddFacts, "Count")
		Point(ctx, namespace, "RemFacts", delta.RemFacts, "Count")
		Point(ctx, namespace, "GetFacts", delta.GetFacts, "Count")
		Point(ctx, namespace, "SearchRules", delta.SearchRules, "Count")
		Point(ctx, namespace, "SearchFacts", delta.SearchFacts, "Count")
		Point(ctx, namespace, "ListRules", delta.ListRules, "Count")
		Point(ctx, namespace, "ProcessEvents", delta.ProcessEvents, "Count")
		Point(ctx, namespace, "GetEventHistorys", delta.GetEventHistorys, "Count")
		Point(ctx, namespace, "ActionExecs", delta.ActionExecs, "Count")
	}
}

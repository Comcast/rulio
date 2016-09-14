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

// A request over any transport needs to be transformed in a generic
// service request.  That generic service request is (perhaps
// unfortunately) currently just 'map[string]interface{}'.  The name
// of the function to invoke is at the key 'uri' for obvious
// historical reasons.
//
// Typical other parameters include 'fact', 'pattern', and 'rule'.
// Those are all also 'map[string]interface{}'s.  A request that has a
// 'location' parameter is subsequently processed by 'ProcessRequest()'

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"rulio/core"
	. "rulio/sys"
)

const APIVersion = "0.0.9"

type Service struct {
	System *System

	// Router is a placeholder for a router, a component which can
	// determine what process hosts a given location.  We'll
	// introduce some router implementations in the future.
	Router interface{}

	// Stopper is function we call when we want to shut ourselves down.
	//
	// Typically this function is defined by the HTTP server to
	// provide a hook to shut down that server.
	Stopper func(*core.Context, time.Duration) error
}

func (s *Service) Start(ctx *core.Context, enginePort string) error {
	return nil
}

func DWIMURI(ctx *core.Context, uri string) string {
	// http://en.wikipedia.org/wiki/DWIM

	// Clean up URI to help with dispatch.

	// We have "/api" in front of all our APIs can their calls to
	// make it easier to search/replace those names in code and
	// docs.

	given := uri
	dropParams, _ := regexp.Compile("[?].*")
	uri = dropParams.ReplaceAllString(uri, "")
	// Strip any pesky version, which we can ignore at the momemnt.
	dropVersion, _ := regexp.Compile("^/v?[.0-9]+")
	uri = dropVersion.ReplaceAllString(uri, "")
	if !strings.HasPrefix(uri, "/api") {
		uri = "/api" + uri
	}
	core.Log(core.DEBUG, ctx, "service.DWIMURI", "from", given, "to", uri)
	return uri
}

// Return a 'map[string]interface{}' value, whether the value was
// found, and any error.  If 'required' is true and the property is
// missing, return an error.  If the property's value is not a
// 'map[string]interface{}', return an err.
func getMapParam(m map[string]interface{}, prop string, required bool) (map[string]interface{}, bool, error) {
	v, have := m[prop]
	if !have {
		if required {
			return nil, false, fmt.Errorf("Parameter %s missing", prop)
		}
		return nil, false, nil
	}
	switch v.(type) {
	case map[string]interface{}:
		return v.(map[string]interface{}), true, nil
	default:
		return nil, true, fmt.Errorf("Parameter %s type %T wrong", prop, v)
	}
}

func getBoolParam(m map[string]interface{}, prop string, required bool) (bool, bool, error) {
	v, have := m[prop]
	if !have {
		if required {
			return false, false, fmt.Errorf("Parameter %s missing", prop)
		}
		return false, false, nil
	}
	switch vv := v.(type) {
	case bool:
		return vv, true, nil
	case string:
		return strings.ToLower(vv) == "true", true, nil
	default:
		return false, true, fmt.Errorf("Parameter %s type %T wrong", prop, v)
	}
}

func GetStringParam(m map[string]interface{}, p string, required bool) (string, bool, error) {
	v, have := m[p]
	if !have {
		if required {
			return "", false, fmt.Errorf("Parameter %s missing", p)
		}
		return "", false, nil
	}
	switch v.(type) {
	case string:
		return v.(string), true, nil
	case []interface{}:
		var acc string
		for _, x := range v.([]interface{}) {
			switch x.(type) {
			case string:
				acc += x.(string)
			default:
				return "", true, fmt.Errorf("Parameter %s type %T wrong at %v %T", p, v, x, x)
			}
		}
		return acc, true, nil
	default:
		return "", true, fmt.Errorf("Parameter %s type %T wrong", p, v)
	}
}

// checkLocal is a little utility wrapper around Manager.Disposition.
//
// This function will return an error with a JSON message if the given
// location shouldn't be served by us.
//
// The current implementation is a no-op.  Waiting on a Router.
func (s *Service) checkLocal(ctx *core.Context, loc string) error {
	return nil
}

// Redirect packaged up directive to send a client to another process.
//
// Not currently used.  Will be used when we have a Router.
type Redirect struct {
	To string
}

func (r *Redirect) Error() string {
	return "redirect to " + r.To
}

func (s *Service) ProcessRequest(ctx *core.Context, m map[string]interface{}, out io.Writer) (map[string]interface{}, error) {
	core.Log(core.INFO, ctx, "service.ProcessRequest", "m", m)

	timer := core.NewTimer(ctx, "ProcessRequest")
	defer func() {
		elapsed := timer.Stop()
		core.Point(ctx, "rulesservice", "requestTime", elapsed/1000, "Microseconds")
	}()

	u, given := m["uri"]
	if !given {
		return nil, fmt.Errorf("No uri.")
	}

	uri := DWIMURI(ctx, u.(string))

	switch uri {

	// case "/api/sys/config":
	// 	mutationStore,have,_ := getStringParam(m, "MutationStore", false)
	// 	if have {
	// 		return fmt.Errorf("You can't do that.")
	// 	}

	// 	current s.System.GetConfig()

	// 	logging,have,_ := getStringParam(m, "logging", false)
	// 	if have {
	// 		current.Logging = logging
	// 	}
	// 	err := StoreConfig(current)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	err = s.System.SetConfig(current)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	return `{"status":"happy"}`

	case "/api/version":
		fmt.Fprintf(out, `{"version":"%s","go":"%s"}`,
			APIVersion,
			runtime.Version())

	case "/api/health":
		// Let's have a simple URI.
		fmt.Fprintf(out, `{"status":"good"}`)

	case "/api/health/shallow":
		fmt.Fprintf(out, `{"status":"good"}`)

	case "/api/health/deep":
		if err := s.HealthDeep(ctx); err != nil {
			return m, nil
		}
		fmt.Fprintf(out, `{"status":"good"}`)

	case "/api/health/deeper":
		if err := s.HealthDeeper(ctx); err != nil {
			return m, nil
		}
		fmt.Fprintf(out, `{"status":"good"}`)

	case "/api/sys/control":
		// Temporary implementation so I can test other things.
		control, given, _ := GetStringParam(m, "control", false)
		target := s.System.Control()
		if given {
			err := json.Unmarshal([]byte(control), target)
			if err != nil {
				return nil, err
			}
			s.System.SetControl(*target)

			target = s.System.Control()
			js, err := json.Marshal(target)
			if err != nil {
				return nil, err
			}
			out.Write([]byte(fmt.Sprintf(`{"result":"okay","control":%s}`, js)))
		} else {
			js, err := json.Marshal(target)
			if err != nil {
				return nil, err
			}
			out.Write(js)
		}

	case "/api/sys/params":
		control, given, _ := GetStringParam(m, "params", false)
		target := core.SystemParameters.Copy()
		if given {
			err := json.Unmarshal([]byte(control), target)
			if err != nil {
				return nil, err
			}
			core.SystemParameters = target

			target = core.SystemParameters
			js, err := json.Marshal(target)
			if err != nil {
				return nil, err
			}
			out.Write([]byte(fmt.Sprintf(`{"result":"okay","params":%s}`, js)))
		} else {
			js, err := json.Marshal(target)
			if err != nil {
				return nil, err
			}
			out.Write(js)
		}

	case "/api/sys/cachedlocations":
		js, err := json.Marshal(map[string]interface{}{
			"locations": s.System.GetCachedLocations(ctx),
		})
		if err != nil {
			return nil, err
		}
		out.Write(js)

	case "/api/sys/loccontrol":
		control, given, _ := GetStringParam(m, "control", false)
		ctl := s.System.Control()
		target := ctl.DefaultLocControl
		if given {
			err := json.Unmarshal([]byte(control), target)
			if err != nil {
				return nil, err
			}
			js, err := json.Marshal(target)
			if err != nil {
				return nil, err
			}
			ctl.DefaultLocControl = target
			out.Write([]byte(fmt.Sprintf(`{"result":"okay","control":%s}`, js)))
		} else {
			js, err := json.Marshal(target)
			if err != nil {
				return nil, err
			}
			out.Write(js)
		}

	case "/api/sys/stats":
		stats, err := s.System.GetStats(ctx)
		if err != nil {
			return nil, err
		}
		js, err := json.Marshal(&stats)
		if err != nil {
			return nil, err
		}
		out.Write(js)

	case "/api/sys/runtime":
		m, err := GetRuntimes(ctx)
		if nil != err {
			return nil, err
		}
		// m["stats"] = GetStats()

		js, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		out.Write(js)

	case "/api/sys/util/nowsecs":
		fmt.Fprintf(out, `{"secs":%d}`, core.NowSecs())

	case "/api/sys/util/js":
		code, _, err := GetStringParam(m, "code", true)
		bs := make(core.Bindings)
		x, err := core.RunJavascript(ctx, &bs, nil, code)
		if err != nil {
			return m, err
		}
		js, err := json.Marshal(&x)
		if err != nil {
			return m, err
		}
		fmt.Fprintf(out, `{"result":%s}`, js)

	case "/api/sys/util/setJavascriptTestValue":
		js, _, err := GetStringParam(m, "value", true)
		if err != nil {
			return nil, err
		}
		var x interface{}
		if err := json.Unmarshal([]byte(js), &x); err != nil {
			return nil, err
		}
		core.JavascriptTestValue = x
		fmt.Fprintf(out, `{"result":%s}`, js)

	case "/api/sys/admin/panic": // No required params
		message, _, _ := GetStringParam(m, "message", false)
		panic(message)

	case "/api/sys/admin/sleep": // Option d=duration
		duration, given, _ := GetStringParam(m, "d", false)
		if !given {
			duration = "1s"
		}
		d, err := time.ParseDuration(duration)
		if err != nil {
			return nil, err
		}
		time.Sleep(d)
		out.Write([]byte(fmt.Sprintf(`{"slept":"%s"}`, d.String())))

	case "/api/sys/admin/shutdown":
		duration, given, _ := GetStringParam(m, "d", false)
		if given && s.Stopper == nil {
			return nil, errors.New("no Stopper for given duration")
		}
		if !given {
			duration = "1s"
		}
		d, err := time.ParseDuration(duration)
		if err != nil {
			return nil, err
		}

		go func() {
			if s.Stopper != nil {
				core.Log(core.INFO, ctx, "/api/admin/shutdown", "Stopper", true)
				if err := s.Stopper(ctx, d); err != nil {
					core.Log(core.ERROR, ctx, "/api/admin/shutdown", "error", err)
				}
			}
			core.Log(core.INFO, ctx, "/api/admin/shutdown", "Stopper", false)
			if err := s.Shutdown(ctx); err != nil {
				core.Log(core.ERROR, ctx, "/api/admin/shutdown", "error", err)
			}
		}()

		out.Write([]byte(`{"status":"okay"}`))

	case "/api/sys/admin/gcpercent":
		percent, _, _ := GetStringParam(m, "percent", true)
		n, err := strconv.Atoi(percent)
		if err != nil {
			return nil, err
		}
		was := debug.SetGCPercent(n)
		fmt.Fprintf(out, `{"status":"okay","was":%d,"now":%d}`, was, n)

	case "/api/sys/admin/freemem":
		debug.FreeOSMemory()
		fmt.Fprintf(out, `{"status":"okay"}`)

	case "/api/sys/admin/purgeslurpcache":
		core.SlurpCache.Purge()
		fmt.Fprintf(out, `{"status":"okay"}`)

	case "/api/sys/admin/purgehttppcache":
		core.HTTPClientCache.Purge()
		fmt.Fprintf(out, `{"status":"okay"}`)

	case "/api/sys/admin/purgecaches":
		core.SlurpCache.Purge()
		core.HTTPClientCache.Purge()
		fmt.Fprintf(out, `{"status":"okay"}`)

	case "/api/sys/admin/gc":
		runtime.GC()
		fmt.Fprintf(out, `{"status":"okay"}`)

	case "/api/sys/admin/heapdump":
		filename, _, _ := GetStringParam(m, "filename", false)
		if filename == "" {
			filename = "heap.dump"
		}
		f, err := os.Create(filename)
		if err != nil {
			return nil, err
		}
		debug.WriteHeapDump(f.Fd())
		if err = f.Close(); err != nil {
			return nil, err
		}
		fmt.Fprintf(out, `{"status":"okay","filename":"%s"}`, filename)

	case "/api/sys/util/match": // Params: fact or event,pattern
		fact, have, err := getMapParam(m, "fact", false)
		if err != nil {
			return nil, err
		}
		if !have {
			// Maybe we were given an 'event'.  Fine.
			fact, _, err = getMapParam(m, "event", true)
		}
		if err != nil {
			return nil, err
		}

		pattern, _, err := getMapParam(m, "pattern", true)
		if err != nil {
			return nil, err
		}

		bss, err := core.Matches(ctx, pattern, fact)
		if err != nil {
			return nil, err
		}

		js, err := json.Marshal(&bss)
		if err != nil {
			return nil, err
		}
		out.Write(js)

	case "/api/sys/admin/timers/names": // No params
		names := core.GetTimerNames()
		js, err := json.Marshal(names)
		if err != nil {
			return nil, err
		}
		out.Write(js)

	case "/api/sys/admin/timers/get":
		// Param: "name", optional "after" int, optional "limit" int

		name, _, err := GetStringParam(m, "name", true)
		if err != nil {
			return nil, err
		}
		after, given, _ := GetStringParam(m, "after", false)
		if !given {
			after = "-1"
		}

		aft, err := strconv.Atoi(after)
		if err != nil {
			return nil, err
		}

		limit, given := m["limit"]
		if !given {
			limit = float64(-1)
		}

		history := core.GetTimerHistory(name, aft, int(limit.(float64)))
		js, err := json.Marshal(history)
		if err != nil {
			return m, err
		}
		out.Write(js)

	case "/api/sys/storage/get": // For testing
		storage, err := s.System.PeekStorage(ctx)
		if err != nil {
			return nil, err
		}
		var acc string
		switch impl := storage.(type) {
		case *core.MemStorage:
			state := impl.State(ctx)
			js, err := json.Marshal(&state)
			if err != nil {
				acc = fmt.Sprintf(`{"type":"%T","error":"%s"}`,
					storage,
					err.Error())
			} else {
				acc = fmt.Sprintf(`{"type":"%T","state":%s}`,
					storage,
					js)
			}
		default:
			acc = fmt.Sprintf(`{"type":"%T"}`, storage)
		}
		if _, err = out.Write([]byte(acc)); err != nil {
			core.Log(core.ERROR, ctx, "/api/sys/storage", "error", err)
		}

	case "/api/sys/storage/set": // For testing
		state, _, err := getMapParam(m, "state", true)
		if err != nil {
			return nil, err
		}

		storage, err := s.System.PeekStorage(ctx)
		if err != nil {
			return nil, err
		}
		switch impl := storage.(type) {
		case *core.MemStorage:
			mms := make(map[string]map[string]string)
			for loc, pairs := range state {
				m, ok := pairs.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("bad pairs %#v (%T)", pairs, pairs)
				}
				locPairs := make(map[string]string)
				for id, val := range m {
					s, ok := val.(string)
					if !ok {
						return nil, fmt.Errorf("bad value %#v (%T)", val, val)
					}
					locPairs[id] = s

				}
				mms[loc] = locPairs
			}
			impl.SetState(ctx, mms)
			out.Write([]byte(`{"status":"okay"}`))
		default:
			return nil, fmt.Errorf(`{"error":"set not supported for %T"}`, storage)
		}

	case "/api/sys/util/batch": // For testing
		// Execute a batch of requests
		batch, given := m["requests"]
		if !given {
			return nil, errors.New("missing 'requests' parameter")
		}
		var err error
		switch xs := batch.(type) {
		case []interface{}:
			_, err = out.Write([]byte("["))
			for i, x := range xs {
				if 0 < i {
					_, err = out.Write([]byte(","))
				}
				switch m := x.(type) {
				case map[string]interface{}:
					_, err = s.ProcessRequest(ctx, m, out)
					if err != nil {
						problem := fmt.Sprintf(`{"error":"%s"}`, err.Error())
						_, err = out.Write([]byte(problem))
					}
				default:
					problem := fmt.Sprintf(`"bad type %T"`, x)
					_, err = out.Write([]byte(problem))
				}
			}
			_, err = out.Write([]byte("]"))
		default:
			return nil, errors.New("'requests' not an array")
		}

		if err != nil {
			core.Log(core.ERROR, ctx, "/api/sys/batch", "error", err)
		}

	case "/api/loc/admin/size":
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}
		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}
		n, err := s.System.GetSize(ctx, location)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(out, `{"size":%d}`, n)

	case "/api/loc/admin/stats":
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		stats, err := s.System.GetLocationStats(ctx, location)
		if err != nil {
			return nil, err
		}
		js, err := json.Marshal(&stats)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/stats", "warning", err)
		}

	case "/api/loc/util/js":
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		code, _, err := GetStringParam(m, "code", true)

		encoding, provided, err := GetStringParam(m, "encoding", false)
		if provided {
			code, err = core.DecodeString(encoding, code)
			if err != nil {
				return nil, err
			}
		}

		bs := make(core.Bindings)

		var props map[string]interface{}
		ctl := s.System.LocControl(ctx, location)
		if ctl != nil {
			props = ctl.CodeProps
		}

		libraries := make([]string, 0, 0)
		libs, given := m["libraries"]
		if given {
			switch vv := libs.(type) {
			case []interface{}:
				for _, lib := range vv {
					switch s := lib.(type) {
					case string:
						libraries = append(libraries, s)
					default:
						err := fmt.Errorf("Bad library type %T (value= %#v)", lib, lib)
						core.Log(core.UERR, ctx, "/api/loc/util/js", "error", err)
						return nil, err
					}
				}
			default:
				err := fmt.Errorf("Bad 'libraries' type %T (value= %#v)",
					libs, libs)
				core.Log(core.UERR, ctx, "/api/loc/util/js", "error", err)
				return nil, err
			}
		}

		x, err := s.System.RunJavascript(ctx, location, code, libraries, &bs, props)
		if err != nil {
			return m, err
		}

		js, err := json.Marshal(&x)
		if err != nil {
			return m, err
		}
		fmt.Fprintf(out, `{"result":%s}`, js)

	case "/api/loc/admin/create": // Params: location
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		created, err := s.System.CreateLocation(ctx, location)
		if err != nil {
			return nil, err
		}
		if !created {
			return nil, fmt.Errorf("%s already exists", location)
		}
		if _, err = out.Write([]byte(`{"status":"okay"}`)); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/admin/create", "warning", err)
		}

	case "/api/loc/admin/clear": // Params: location
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		err = s.System.ClearLocation(ctx, location)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write([]byte(`{"status":"okay"}`)); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/admin/clear", "warning", err)
		}

	case "/api/loc/admin/updatedmem": // Params: location
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		updated, err := s.System.GetLastUpdatedMem(ctx, location)
		if err != nil {
			return nil, err
		}

		resp := fmt.Sprintf(`{"lastUpdated":"%s","source":"memory"}`, updated)
		if _, err = out.Write([]byte(resp)); err != nil {
			core.Log(core.INFO, ctx, "/api/loc/admin/updatedmem", "warning", err)
		}

	case "/api/loc/admin/delete": // Params: location
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		err = s.System.DeleteLocation(ctx, location)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write([]byte(`{"status":"okay"}`)); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/admin/delete", "warning", err)
		}

	case "/api/loc/events/ingest": // Params: event
		event, _, err := getMapParam(m, "event", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		// ToDo: Not this.
		js, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}

		ctx.LogAccumulatorLevel = core.EVERYTHING
		work, err := s.System.ProcessEvent(ctx, location, string(js))
		if err != nil {
			return nil, err
		}

		js, err = json.Marshal(work)
		if err != nil {
			return nil, err
		}

		s := fmt.Sprintf(`{"id":"%s","result":%s}`, ctx.Id(), js)
		core.Log(core.INFO, ctx, "/api/loc/events/ingest", "got", s)
		if _, err = out.Write([]byte(s)); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/events/ingest", "warning", err)
		}

	case "/api/loc/events/retry": // Params: work
		workStr, _, err := GetStringParam(m, "work", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		fr := core.FindRules{}
		err = json.Unmarshal([]byte(workStr), &fr)
		if err != nil {
			return nil, err
		}

		ctx.LogAccumulatorLevel = core.EVERYTHING
		// ToDo: Support number of steps to take.
		err = s.System.RetryEventWork(ctx, location, &fr)
		js, err := json.Marshal(fr)
		if err != nil {
			return nil, err
		}

		s := fmt.Sprintf(`{"id":"%s","result":%s}`, ctx.Id(), js)
		core.Log(core.INFO, ctx, "/api/loc/events/retry", "got", s)
		if _, err = out.Write([]byte(s)); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/events/retry", "warning", err)
		}

	case "/api/loc/facts/add": // Params: fact
		fact, _, err := getMapParam(m, "fact", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		id, _, err := GetStringParam(m, "id", false)

		// ToDo: Not this.
		js, err := json.Marshal(fact)
		if err != nil {
			return nil, err
		}

		id, err = s.System.AddFact(ctx, location, id, string(js))
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"id": id}

		js, err = json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/facts/add", "warning", err)
		}

	case "/api/loc/facts/rem": // Params: id
		id, _, err := GetStringParam(m, "id", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		rid, err := s.System.RemFact(ctx, location, id)
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"removed": rid, "given": id}

		js, err := json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/facts/rem", "warning", err)
		}

	case "/api/loc/facts/get": // Params: id
		id, _, err := GetStringParam(m, "id", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		js, err := s.System.GetFact(ctx, location, id)
		if err != nil {
			return nil, err
		}
		bs := []byte(fmt.Sprintf(`{"fact":%s,"id":"%s"}`, js, id))

		if _, err = out.Write(bs); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/facts/get", "warning", err)
		}

	case "/api/loc/facts/search": // Params: pattern, inherited
		pattern, _, err := getMapParam(m, "pattern", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		includedInherited, _, err := getBoolParam(m, "inherited", false)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		// ToDo: Not this.
		js, err := json.Marshal(pattern)
		if err != nil {
			return nil, err
		}

		sr, err := s.System.SearchFacts(ctx, location, string(js), includedInherited)
		if err != nil {
			return nil, err
		}

		_, take := m["take"]
		if take {
			// Warning: Not (yet) atomic!
			for _, found := range sr.Found {
				_, err := s.System.RemFact(ctx, location, found.Id)
				if err != nil {
					core.Log(core.ERROR, ctx, "service.ProcessRequest", "app_tag", "/api/loc/facts/search", "error", err, "RemFact", found.Id)
				}
				// ToDo: Something with error.
			}
		}

		js, err = json.Marshal(sr)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/facts/take", "warning", err)
		}

	case "/api/loc/facts/take": // Params: pattern
		m["uri"] = "/api/loc/facts/search"
		m["take"] = true
		s.ProcessRequest(ctx, m, out)

	case "/api/loc/facts/replace": // Params: pattern, fact
		// Really a 'take' followed by a 'add'.
		m["uri"] = "/api/loc/facts/search"
		m["take"] = true
		core.Log(core.INFO, ctx, "service.ProcessRequest", "app_tag", "/api/loc/facts/replace", "phase", "take")
		s.ProcessRequest(ctx, m, ioutil.Discard)

		core.Log(core.INFO, ctx, "service.ProcessRequest", "app_tag", "/api/loc/facts/replace", "phase", "add")
		m["uri"] = "/api/loc/facts/add"
		s.ProcessRequest(ctx, m, out)

	case "/api/loc/facts/query": // Params: query
		query, _, err := getMapParam(m, "query", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		// ToDo: Not this.
		js, err := json.Marshal(query)
		if err != nil {
			return nil, err
		}

		qr, err := s.System.Query(ctx, location, string(js))
		if err != nil {
			return nil, err
		}

		js, err = json.Marshal(qr)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/facts/query", "warning", err)
		}

	case "/api/loc/rules/list": // Params: inherited
		location, _, err := GetStringParam(m, "location", true)
		if nil != err {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		includedInherited, _, err := getBoolParam(m, "inherited", false)
		if err != nil {
			return nil, err
		}

		ss, err := s.System.ListRules(ctx, location, includedInherited)
		if nil != err {
			return nil, err
		}

		js, err := json.Marshal(map[string][]string{"ids": ss})
		if nil != err {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/rules/list", "warning", err)
		}

	case "/api/loc/rules/add": // Params: rule
		rule, _, err := getMapParam(m, "rule", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		id, _, err := GetStringParam(m, "id", false)

		// ToDo: Not this.
		js, err := json.Marshal(rule)
		if err != nil {
			return nil, err
		}

		id, err = s.System.AddRule(ctx, location, id, string(js))
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"id": id}

		js, err = json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/rules/add", "warning", err)
		}

	case "/api/loc/rules/rem": // Params: id
		id, _, err := GetStringParam(m, "id", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		rid, err := s.System.RemRule(ctx, location, id)
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"removed": rid, "given": id}

		js, err := json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/rules/rem", "warning", err)
		}

	case "/api/loc/rules/disable": // Params: id
		id, _, err := GetStringParam(m, "id", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		err = s.System.EnableRule(ctx, location, id, false)
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"disabled": id}

		js, err := json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/rules/disable", "warning", err)
		}

	case "/api/loc/rules/enable": // Params: id
		id, _, err := GetStringParam(m, "id", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		err = s.System.EnableRule(ctx, location, id, true)
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"enabled": id}

		js, err := json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/rules/enable", "warning", err)
		}

	case "/api/loc/rules/enabled": // Params: id
		id, _, err := GetStringParam(m, "id", true)
		if err != nil {
			return nil, err
		}

		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		enabled, err := s.System.RuleEnabled(ctx, location, id)
		if err != nil {
			return nil, err
		}
		m := map[string]interface{}{"ruleId": id, "enabled": enabled}

		js, err := json.Marshal(&m)
		if err != nil {
			return nil, err
		}
		if _, err = out.Write(js); err != nil {
			core.Log(core.ERROR, ctx, "/api/loc/rules/enabled", "warning", err)
		}

	case "/api/loc/parents": // Params: none means get; "set=[x,y]" means set.
		location, _, err := GetStringParam(m, "location", true)
		if err != nil {
			return nil, err
		}

		if err := s.checkLocal(ctx, location); err != nil {
			return nil, err
		}

		js, given, err := GetStringParam(m, "set", false)

		if given {
			var parents []string
			if err = json.Unmarshal([]byte(js), &parents); err != nil {
				return nil, err
			}

			_, err := s.System.SetParents(ctx, location, parents)
			if err != nil {
				return nil, err
			}
			js := fmt.Sprintf(`{"result": %s}`, js)
			if _, err = out.Write([]byte(js)); err != nil {
				core.Log(core.ERROR, ctx, "/api/loc/parents/set", "warning", err)
			}
		} else {
			ps, err := s.System.GetParents(ctx, location)
			if err != nil {
				return nil, err
			}
			bs, err := json.Marshal(&ps)
			if err != nil {
				return nil, err
			}
			bs = []byte(fmt.Sprintf(`{"result":%s}`, bs))
			if _, err = out.Write(bs); err != nil {
				core.Log(core.ERROR, ctx, "/api/loc/parents/get", "warning", err)
			}
		}

	default:
		return nil, fmt.Errorf("Unknown URI '%s'", u)

	}

	return nil, nil
}

func (s *Service) Shutdown(ctx *core.Context) error {
	core.Log(core.WARN, ctx, "Service.Shutdown")
	storage, err := s.System.PeekStorage(ctx)
	if err != nil {
		return err
	}
	if storage != nil {
		core.Log(core.WARN, ctx, "Service.Shutdown", "closing", "storage")
		err = storage.Close(ctx)
		if err != nil {
			return err
		}
	}
	rc := 0
	if err != nil {
		rc = 1
	}
	os.Exit(rc)
	return nil
}

func (s *Service) HealthDeep(ctx *core.Context) error {
	core.Log(core.INFO, ctx, "HealthDeep")
	// Just try some Javascript.
	code := "'go' + 'od'"
	x, err := core.RunJavascript(ctx, nil, nil, code)
	if err == nil {
		var js []byte
		js, err = json.Marshal(&x)
		if err == nil {
			if string(js) != `"good"` {
				err = fmt.Errorf("Unexpected %s", js)
			}
		}
	}

	core.Log(core.INFO, ctx, "HealthDeep", "err", err)
	return err
}

func (s *Service) HealthDeeper(ctx *core.Context) error {
	core.Log(core.INFO, ctx, "HealthDeeper")
	err := s.HealthDeep(ctx)
	if err == nil {
		core.Log(core.INFO, ctx, "HealthDeeper", "looking", "storage")
		storage, err := s.System.PeekStorage(ctx)
		if err == nil && storage != nil {
			core.Log(core.INFO, ctx, "HealthDeeper", "checking", "storage")
			err = storage.Health(ctx)
		}
	}
	core.Log(core.INFO, ctx, "HealthDeeper", "err", err)
	return err
}

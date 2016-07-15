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

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"

	"github.com/Comcast/rulio/core"
	"github.com/Comcast/rulio/cron"
	"github.com/Comcast/rulio/service"
	"github.com/Comcast/rulio/storage/dynamodb"
	"github.com/Comcast/rulio/sys"
)

var genericFlags = flag.NewFlagSet("generic", flag.ExitOnError)
var cpuprofile = genericFlags.String("cpuprofile", "", "write cpu profile to this file")
var memProfileRate = genericFlags.Int("memprofilerate", 512*1024, "runtime.MemProfileRate")
var blockProfileRate = genericFlags.Int("blockprofilerate", 0, "runtime.SetBlockProfileRate")
var httpProfilePort = genericFlags.String("httpprofileport", "localhost:6060", "Run an HTTP server that serves profile data; 'none' to turn off")
var verbosity = genericFlags.String("verbosity", "EVERYTHING", "Logging verbosity.")

var engineFlags = flag.NewFlagSet("engine", flag.ExitOnError)
var linearState = engineFlags.Bool("linear-state", false, "linear (or indexed) state?")
var maxPending = engineFlags.Int("max-pending", 0, "max pending requests; 0 means no max")
var maxLocations = engineFlags.Int("max-locations", 1000, "Max locations")
var maxFacts = engineFlags.Int("max-facts", 1000, "Max facts per location")
var accVerbosity = engineFlags.String("acc-verbosity", "EVERYTHING", "Log accumulator verbosity.")
var storageType = engineFlags.String("storage", "mem", "storage type")
var storageConfig = engineFlags.String("storage-config", "", "storage config")
var bashActions = engineFlags.Bool("bash-actions", false, "enable Bash script actions")

var enginePort = engineFlags.String("engine-port", ":8001", "port engine will serve")
var locationTTL = engineFlags.String("ttl", "forever", "Location TTL, a duration, 'forever', or 'never'")
var cronURL = engineFlags.String("cron-url", "", "Optional URL for external cron service")
var rulesURL = engineFlags.String("rules-url", "http://localhost:8001/", "Optional URL for external cron service to reach rules engine")
var checkState = engineFlags.Bool("check-state", false, "Whether to check for state consistency")

// Direct storage examination an manipulation
//
// This CLI exposes the complete storage API, so you can do surgery on
// locations if you really want to.
var storeFlags = flag.NewFlagSet("storage", flag.ExitOnError)
var storeType = storeFlags.String("storage", "dynamodb", "storage type")
var storeConfig = storeFlags.String("storage-config", "us-west-2:rulestest", "storage type")
var storeClose = storeFlags.Bool("close", false, "close storage")
var storeHealth = storeFlags.Bool("health", false, "check storage health")

// The following args will apply to all locations provided on the command line.
//
// Example
//
//    ./rulesys -httpprofileport none storage \
//         -storage dynamodb -storage-config us-west-1:rulesstress \
//         -get here there somewhere_else
//
var storeGet = storeFlags.Bool("get", true, "get location")
var storeRem = storeFlags.String("rem", "", "remove fact/rule with this id")
var storeDel = storeFlags.Bool("del", false, "remove location")
var storeClear = storeFlags.Bool("clear", false, "clear location")
var storeStats = storeFlags.Bool("stats", false, "get stats")
var storeAdd = storeFlags.String("add", "", "add these facts: {id1:fact1,id2:fact2,...}")

func generic(args []string) []string {
	genericFlags.Parse(args)

	{
		runtime.MemProfileRate = *memProfileRate

		if *cpuprofile != "" {
			f, err := os.Create(*cpuprofile)
			if err != nil {
				panic(err)
			}
			// service.ProfFilename = *cpuprofile
			pprof.StartCPUProfile(f)
			// defer pprof.StopCPUProfile()
		}

		if 0 < *blockProfileRate {
			runtime.SetBlockProfileRate(*blockProfileRate)
		}

	}

	{
		if *httpProfilePort != "" && *httpProfilePort != "none" {
			go func() {
				if err := http.ListenAndServe(*httpProfilePort, nil); err != nil {
					panic(err)
				}
			}()
		}
	}

	return genericFlags.Args()
}

func engine(args []string, wg *sync.WaitGroup) []string {

	engineFlags.Parse(args)

	ctx := core.NewContext("main")

	dynamodb.CheckLastUpdated = *checkState

	conf := sys.ExampleConfig()
	if *linearState {
		conf.UnindexedState = true
	}
	conf.Storage = *storageType
	conf.StorageConfig = *storageConfig

	cont := sys.ExampleSystemControl()
	cont.MaxLocations = *maxLocations

	ttl, err := sys.ParseLocationTTL(*locationTTL)
	if err != nil {
		panic(err)
	}
	cont.LocationTTL = ttl

	// ToDo: Expose this internal cron limit
	var cronner cron.Cronner
	if *cronURL == "" {
		// Here we're using a single internal cron for all
		// served locations.  That's not (according to the
		// nice comments in cron.go) ideal.  However, it's (a)
		// easy and (b) not used in production server-side
		// deployments, which will use an external cron, which
		// provides persistency.
		cr, _ := cron.NewCron(nil, time.Second, "intcron", 1000000)
		go cr.Start(ctx)
		cronner = &cron.InternalCron{Cron: cr}
	} else {
		cronner = &cron.CroltSimple{
			CroltURL: *cronURL,
			RulesURL: *rulesURL,
		}
	}

	verb, err := core.ParseVerbosity(*verbosity)
	if err != nil {
		panic(err)
	}
	locCtl := &core.Control{
		MaxFacts:  *maxFacts,
		Verbosity: verb,
	}
	if *bashActions {
		locCtl.ActionInterpreters = map[string]core.ActionInterpreter{
			"bash": &BashActionInterpreter{},
		}
	}
	cont.DefaultLocControl = locCtl

	sys, err := sys.NewSystem(ctx, *conf, *cont, cronner)
	if err != nil {
		panic(err)
	}

	ctx.SetLogValue("app.id", "rulesys")
	core.UseCores(ctx, false)

	ctx.Verbosity = verb
	ctx.LogAccumulatorLevel = verb

	engine := &service.Service{sys, nil, nil}
	engine.Start(ctx, *rulesURL)

	serv, err := service.NewHTTPService(ctx, engine)
	if err != nil {
		panic(err)
	}
	serv.SetMaxPending(int32(*maxPending))
	wg.Add(1)
	go func() {
		err = serv.Start(ctx, *enginePort)
		if err != nil {
			panic(err)
		}
	}()

	return engineFlags.Args()
}

func storage(args []string, wg *sync.WaitGroup) []string {

	storeFlags.Parse(args)

	ctx := core.NewContext("main")
	store, err := sys.GetStorage(ctx, *storeType, *storeConfig)
	if err != nil {
		panic(err)
	}

	if *storeHealth {
		err := store.Health(ctx)
		if err != nil {
			panic(err)
		}
	}

	if *storeClose {
		err := store.Close(ctx)
		if err != nil {
			panic(err)
		}
	}

	locations := storeFlags.Args()
	if len(locations) == 0 {
		panic(errors.New("give at least one location on command line"))
	}

	for _, name := range locations {
		loc, err := core.NewLocation(ctx, name, nil, nil)
		if err != nil {
			panic(err)
		}
		ctx.SetLoc(loc)
		if *storeGet {
			pairs, err := store.Load(ctx, name)
			if err != nil {
				panic(err)
			}
			fmt.Print("{")
			for i, pair := range pairs {
				if 0 < i {
					fmt.Print(",")
				}
				fmt.Printf(`"%s":%s`, pair.K, pair.V)
			}
			fmt.Println("}")
		}
		if *storeRem != "" {
			if _, err = store.Remove(ctx, name, []byte(*storeRem)); err != nil {
				panic(err)
			}
		}
		if *storeDel {
			if err := store.Delete(ctx, name); err != nil {
				panic(err)
			}
		}
		if *storeClear {
			if _, err := store.Clear(ctx, name); err != nil {
				panic(err)
			}
		}
		if *storeStats {
			stats, err := store.GetStats(ctx, name)
			if err != nil {
				panic(err)
			}
			js, err := json.Marshal(&stats)
			if err != nil {
				panic(err)
			}
			fmt.Println(string(js))
		}
		if *storeAdd != "" {
			var facts map[string]map[string]interface{}
			if err := json.Unmarshal([]byte(*storeAdd), &facts); err != nil {
				panic(err)
			}
			for id, fact := range facts {
				js, err := json.Marshal(fact)
				if err != nil {
					panic(err)
				}
				pair := &core.Pair{[]byte(id), js}
				if err := store.Add(ctx, name, pair); err != nil {
					panic(err)
				}
			}
		}
	}

	return []string{}
}

func usage() {
	fmt.Fprintf(os.Stderr, "\ngeneric flags:\n\n")
	genericFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nengine subcommand:\n\n")
	engineFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nstorage subcommand: List locations as final args\n\n")
	storeFlags.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {

	wg := sync.WaitGroup{}

	args := os.Args[1:]
	args = generic(args)
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Error: Need a subcommand (engine|storage)\n\n")
		usage()
		os.Exit(1)
	}

	switch args[0] {
	case "engine":
		args = engine(args[1:], &wg)
	case "storage":
		args = storage(args[1:], &wg)
	case "help":
		usage()
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "bad subcommand '%s'\n", args[0])
		os.Exit(1)

	}

	wg.Wait()
}

// BashActionInterpreter is an example action interpreter for Bash scripts.
//
// Bindings are added to the environment with variable names that
// start with an underscore.  The variable names are also upper-cased
// (and the leading question mark is stripped).  The value of the
// variable _EVENT is JSON.
//
// On error, the error is returned throught the standard path.
//
// Example use:
//
//   curl -s "$ENDPOINT/api/loc/admin/clear?location=$LOCATION"
//
//   cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
//   {"rule": {"when":{"pattern":{"wants":"?x"}},
//             "action":{"code":"uptime; env; touch \"/tmp/\$_X\"",
//                       "endpoint":"bash"}}}
//   EOF
//
//   curl -d 'event={"wants":"tacos"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
//      python -mjson.tool
//
//
type BashActionInterpreter struct {
}

func (i *BashActionInterpreter) GetName() string {
	return "bash"
}

func (i *BashActionInterpreter) GetThunk(ctx *core.Context, loc *core.Location, bs core.Bindings, a core.Action) (func() (interface{}, error), error) {

	return func() (interface{}, error) {
		core.Log(core.DEBUG, ctx, "BashActionIntpreter.GetThunk", "action", a, "bs", bs)
		code, err := core.GetCode(a.Code)
		if err != nil {
			return nil, err
		}

		env := make([]string, 0, len(bs))
		for p, v := range bs {
			p = strings.ToUpper(p[1:])
			if p == "EVENT" {
				js, err := json.Marshal(&v)
				if err != nil {
					return nil, err
				}
				v = string(js)
			}
			env = append(env, fmt.Sprintf("_%s=%s", p, v))
		}

		cmd := exec.Command("bash")
		cmd.Stdin = strings.NewReader(code)
		cmd.Env = env
		var out bytes.Buffer
		cmd.Stdout = &out
		if err = cmd.Run(); err != nil {
			core.Log(core.INFO, ctx, "BashActionIntpreter.GetThunk", "action", a, "error", err)
			return nil, err
		}
		return out.String(), nil
	}, nil

}

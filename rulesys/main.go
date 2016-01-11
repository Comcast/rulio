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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
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

var engineFlags = flag.NewFlagSet("engine", flag.PanicOnError)
var linearState = engineFlags.Bool("linear-state", false, "linear (or indexed) state?")
var maxPending = engineFlags.Int("max-pending", 0, "max pending requests; 0 means no max")
var maxLocations = engineFlags.Int("max-locations", 1000, "Max locations")
var maxFacts = engineFlags.Int("max-facts", 1000, "Max facts per location")
var accVerbosity = engineFlags.String("acc-verbosity", "EVERYTHING", "Log accumulator verbosity.")
var storageType = engineFlags.String("storage", "mem", "storage type")
var storageConfig = engineFlags.String("storage-config", "", "storage config")

var enginePort = engineFlags.String("engine-port", ":8001", "port engine will serve")
var locationTTL = engineFlags.String("ttl", "forever", "Location TTL, a duration, 'forever', or 'never'")
var cronURL = engineFlags.String("cron-url", "", "Optional URL for external cron service")
var rulesURL = engineFlags.String("rules-url", "http://localhost:8001/", "Optional URL for external cron service to reach rules engine")
var checkState = engineFlags.Bool("check-state", false, "Whether to check for state consistency")

// Direct storage examination an manipulation
//
// This CLI exposes the complete storage API, so you can do surgery on
// locations if you really want to.
var storeFlags = flag.NewFlagSet("storage", flag.PanicOnError)
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
		MaxFacts:  1000,
		Verbosity: verb,
	}
	cont.DefaultLocControl = locCtl

	sys, err := sys.NewSystem(ctx, *conf, *cont, cronner)
	if err != nil {
		panic(err)
	}

	locCont := &core.Control{
		MaxFacts: *maxFacts,
	}
	ctl := sys.Control()
	ctl.DefaultLocControl = locCont
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
	fmt.Println("generic flags:")
	genericFlags.Usage()
	fmt.Println("engine subcommand:")
	engineFlags.Usage()
	fmt.Println("storage subcommand: List locations as final args")
	storeFlags.Usage()
}

func main() {

	wg := sync.WaitGroup{}

	args := os.Args[1:]
	args = generic(args)
	if len(args) == 0 {
		usage()
		os.Exit(1)
	}
	switch args[0] {
	case "engine":
		args = engine(args[1:], &wg)
	case "storage":
		args = storage(args[1:], &wg)
	}
	wg.Wait()

	if len(args) != 0 {
		// left-over args are bad.  Freak out (even though we
		// might have started or done something previously.
		fmt.Printf("bad command line: extra args %#v\n", args)
		os.Exit(1)
	}
}

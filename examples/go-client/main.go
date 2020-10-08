package main

import (
	"encoding/json"
	"fmt"
	"github.com/Comcast/rulio/core"
	"github.com/Comcast/rulio/examples/go-client/configuration"
	"github.com/Comcast/rulio/examples/go-client/ruleEngine"
)

func main() {
	fmt.Println("Starting rule engine...")
	envConfig, err := configuration.ParseEnvConfiguration()
	if err != nil {
		fmt.Errorf("Could not parse config: %s", err)
	}

	ctx := core.NewContext("main")
	engine, err := ruleEngine.NewEngine(envConfig, ctx)
	if err != nil {
		panic(err)
	}
	addFact(engine)
	searchFact(engine)
	createRule(engine)
	listRule(engine)
	processEvent(engine)
}

func addFact(engine *ruleEngine.EnginePoc) {
	// Add Facts
	//curl -s -d 'fact={"city":"London"}' "$ENDPOINT/api/loc/facts/add?location=$LOCATION"
	fmt.Print("\nAdding Facts city=London")
	m := make(map[string]interface{})
	v := make(map[string]interface{})

	b := []byte(`{"city":"London"}`)
	err := json.Unmarshal(b, &v)
	if err != nil {
		fmt.Printf("\n Unmarshal has failed with error: %v\n", err)
	}

	m["fact"] = v
	m["location"] = "here"
	ctx := engine.Ctx.SubContext()
	err = engine.Service.AddFact(ctx, m)
	if err != nil {
		fmt.Printf("\n Adding Fact failed with error: %v\n", err)
	}
}

func searchFact(engine *ruleEngine.EnginePoc) {
	//curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=$LOCATION"
	fmt.Println("\n\nSearching Facts city:?x ")
	pattern := make(map[string]interface{})
	v := make(map[string]interface{})

	b := []byte(`{"city":"?x"}`)
	err := json.Unmarshal(b, &v)
	if err != nil {
		fmt.Printf("\n Unmarshal has failed with error: %v\n", err)
	}

	pattern["pattern"] = v
	pattern["location"] = "here"
	ctx := engine.Ctx.SubContext()
	err = engine.Service.SearchFact(ctx, pattern)
	if err != nil {
		fmt.Printf("\n Searching Fact failed with error: %v\n", err)
	}
}

func createRule(engine *ruleEngine.EnginePoc) {
	//{ "name":"John", "age":30, "city":"London", "code":"SEC-3423" }

	/*	cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
		{"rule": {"when":{"pattern":{"code":"SEC-3423"},"age":"30"},
			"condition":{"pattern":{"city":"?x"}},
			"action":{"code":"var msg = 'city ' + x; console.log(msg); msg;"}}}
		EOF*/

	rule := make(map[string]interface{})
	r := make(map[string]interface{})
	when := []byte(`{"when":{"pattern":{"code":"SEC-3423"},"age":"30"},
		"condition":{"pattern":{"city":"?x"}},
		"action":{"code":"var msg = 'city ' + x; console.log(msg); msg;"}}`)
	err := json.Unmarshal(when, &r)
	if err != nil {
		fmt.Printf("\n Unmarshal has failed with error: %v\n", err)
	}

	rule["rule"] = r
	rule["location"] = "here"
	ctx := engine.Ctx.SubContext()
	err = engine.Service.AddRule(ctx, rule)
	if err != nil {
		fmt.Printf("\n Rule creation failed with error: %v\n", err)
	}
}

func listRule(engine *ruleEngine.EnginePoc) {
	//curl -s  "$ENDPOINT/loc/rules/list?location=$LOCATION"
	fmt.Println("\n\nListing Rules available rule")
	rule := make(map[string]interface{})
	rule["location"] = "here"
	ctx := engine.Ctx.SubContext()
	err := engine.Service.ListRule(ctx, rule)
	if err != nil {
		fmt.Printf("\n Rule creation failed with error: %v\n", err)
	}
}

func processEvent(engine *ruleEngine.EnginePoc) {
	fmt.Println("\n\nProcessing for incoming events --> { \"name\":\"John\", \"age\":30, \"city\":\"London\", \"code\":\"SEC-3423\" } ")

	/*curl -d 'event={ "name":"John", "age":30, "city":"London", "code":"SEC-3423" }'
	"$ENDPOINT/api/loc/events/ingest?location=$LOCATION" |   python3 -mjson.tool
	*/

	event := make(map[string]interface{})
	r := make(map[string]interface{})
	e := []byte(`{ "name":"John", "age":30, "city":"London", "code":"SEC-3423" }`)
	err := json.Unmarshal(e, &r)
	if err != nil {
		fmt.Printf("\n Unmarshal has failed with error: %v\n", err)
	}
	event["event"] = r
	event["location"] = "here"
	ctx := engine.Ctx.SubContext()
	err = engine.Service.ProcessEvent(ctx, event)
	if err != nil {
		fmt.Printf("\n Event Processing failed with error: %v\n", err)
	}
}

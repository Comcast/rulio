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

package dynamodb

import (
	"fmt"
	"github.com/AdRoll/goamz/dynamodb"
	core "github.com/Comcast/rulio/core"
	"log"
	"testing"
	"time"
)

var testConfig = DynamoDBConfig{"local", DefaultTableName, DefaultConsistent}

func TestParseConfig(t *testing.T) {
	{
		c, err := ParseConfig("")
		if err != nil {
			t.Error(err)
		}
		if c.Region != DefaultRegion {
			t.Errorf("got wrong region: %s", c.Region)
		}
		if c.TableName != DefaultTableName {
			t.Errorf("got wrong table: %s", c.TableName)
		}
		if c.Consistent != DefaultConsistent {
			t.Errorf("got wrong consistency: %v", c.Consistent)
		}
	}

	{
		_, err := ParseConfig("foo:bar:bad")
		if err == nil {
			t.Error("ad consistency should have gotten an error")
		}
	}

	{
		c, err := ParseConfig("foo:bar")
		if err != nil {
			t.Error(err)
		}
		if c.Region != "foo" {
			t.Errorf("got wrong region: %s", c.Region)
		}
		if c.TableName != "bar" {
			t.Errorf("got wrong table: %s", c.TableName)
		}
		if c.Consistent != DefaultConsistent {
			t.Errorf("got wrong consistency: %v", c.Consistent)
		}
	}

	{
		c, err := ParseConfig("foo:bar:false")
		if err != nil {
			t.Error(err)
		}
		if c.Region != "foo" {
			t.Errorf("got wrong region: %s", c.Region)
		}
		if c.TableName != "bar" {
			t.Errorf("got wrong table: %s", c.TableName)
		}
		if c.Consistent != false {
			t.Errorf("got wrong consistency: %v", c.Consistent)
		}
	}

}

func TestDynamoBasic(t *testing.T) {
	ctx := core.NewContext("test")

	ddb, err := NewStorage(ctx, testConfig)
	if err != nil {
		t.Fatal(err)
	}
	server := ddb.server

	{
		tables, err := server.ListTables()
		if err != nil {
			t.Fatal(err)
		}

		for _, t := range tables {
			log.Println("table " + t)
		}
	}

	table := ddb.table

	{
		attrs := []dynamodb.Attribute{
			*dynamodb.NewStringAttribute("likes", "beer"),
		}

		if ok, err := table.PutItem("homer", "", attrs); !ok {
			t.Fatal(err)
		}
	}

	{
		attrs := []dynamodb.Attribute{
			*dynamodb.NewStringAttribute("likes", "tacos"),
		}

		k := dynamodb.Key{HashKey: "homer"}
		if ok, err := table.UpdateAttributes(&k, attrs); !ok {
			t.Fatal(err)
		}
	}

	{
		k := dynamodb.Key{HashKey: "homer"}
		as, err := table.GetItem(&k)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range as {
			log.Println(v)
		}
	}
}

func TestDynamoFacts(t *testing.T) {
	location := "there"

	ctx := core.NewContext("test")

	store, err := NewStorage(ctx, testConfig)
	if err != nil {
		t.Fatal(err)
	}

	state, err := core.NewLinearState(ctx, location, store)
	if err != nil {
		t.Fatal(err)
	}

	loc, err := core.NewLocation(ctx, location, state, nil)
	if err != nil {
		t.Fatal(err)
	}

	sr, err := loc.SearchFacts(ctx, `{"likes":"?x"}`)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("# SearchFacts %v %v\n", *sr, err)

	loc.Clear(ctx)

	id, err := loc.AddFact(ctx, `{"likes":"chips"}`, "")
	log.Printf("# AddFact %s %v\n", id, err)
	id, err = loc.AddFact(ctx, `{"likes":"beer"}`, "")
	log.Printf("# AddFact %s %v\n", id, err)
	id, err = loc.AddFact(ctx, `{"wants":"tacos"}`, "")
	log.Printf("# AddFact %s %v\n", id, err)
	id, err = loc.AddFact(ctx, `{"wants":"tacos"}`, "")
	log.Printf("# AddFact %s %v\n", id, err)

	sr, err = loc.SearchFacts(ctx, `{"wants":"?x"}`)
	log.Printf("# SearchFacts 1 %v %v\n", *sr, err)

	id, err = loc.RemFact(ctx, id)
	log.Printf("# RemFact %v %v\n", id, err)

	sr, err = loc.SearchFacts(ctx, `{"wants":"?x"}`)
	log.Printf("# SearchFacts 2 %v %v\n", *sr, err)
}

func TestDynamoThroughput(t *testing.T) {
	location := "there"
	n := 100

	ctx := core.NewContext(location)
	store, err := NewStorage(ctx, testConfig)
	if err != nil {
		t.Fatal(err)
	}

	state, err := core.NewLinearState(ctx, location, store)
	if err != nil {
		t.Fatal(err)
	}

	loc, err := core.NewLocation(ctx, location, state, nil)
	if err != nil {
		t.Fatal(err)
	}

	loc.Clear(ctx)

	then := time.Now().UTC().UnixNano()
	for i := 0; i < n; i++ {
		fact := fmt.Sprintf(`{"likes":"beer %d"}`, i)
		_, err := loc.AddFact(ctx, "", fact)
		if err != nil {
			// ProvisionedThroughputExceededException: The
			// level of configured provisioned throughput
			// for the table was exceeded. Consider
			// increasing your provisioning level with the
			// UpdateTable API
			log.Printf("Died at iteration %d", i)
			log.Fatal(err)
		}
	}
	elapsed := time.Now().UTC().UnixNano() - then
	log.Printf("elapsed %d us", elapsed/1000)

	then = time.Now().UTC().UnixNano()
	sr, err := loc.SearchFacts(ctx, `{"likes":"?x"}`)
	if err != nil {
		log.Fatal(err)
	}
	elapsed = time.Now().UTC().UnixNano() - then
	log.Printf("elapsed %d us", elapsed/1000)
	log.Printf("# SearchFacts found %d\n", len(sr.Found))
}

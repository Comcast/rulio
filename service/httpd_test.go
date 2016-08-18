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

package service

import (
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Comcast/rulio/sys"
)

func TestHTTP(t *testing.T) {
	port := "localhost:9999" // Sorry.

	sys, ctx := sys.ExampleSystem("Test")
	service := &Service{
		System: sys,
	}

	server, err := NewHTTPService(ctx, service)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		if err = server.Start(ctx, port); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(time.Second)

	serviceRequest := func(m map[string]interface{}) (map[string]interface{}, string) {
		out := &bytes.Buffer{}
		result, err := service.ProcessRequest(ctx, m, out)
		if err != nil {
			t.Fatal(err)
		}
		return result, out.String()
	}

	_, version := serviceRequest(map[string]interface{}{
		"uri": "/api/version",
	})
	log.Printf("version: %s", version)

	httpRequest := func(uri string, body string) string {
		if body == "" {
			body = "{}"
		}
		buf := bytes.NewBufferString(body)

		resp, err := http.Post("http://"+port+"/"+uri, "application/json", buf)
		if err != nil {
			t.Fatal(err)
		}
		bs, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		return string(bs)
	}

	version = httpRequest("api/version", "")
	log.Printf("version: %s", version)

	request := `location: here
rule: 
  when:
    pattern:
      person: "?x"
      latlon: "?there"
  condition:
    and:
      - pattern:
          person: "?y"
          latlon: "?near"
      - code: |
          console.log("checking " + x + "," + y + " at " + JSON.stringify(there) + "," + JSON.stringify(near)); true;
      - code: "x != y"
  action:
    code: |
      console.log(x + " is near " + y);
      var json = JSON.stringify({uri: "/api/loc/events/ingest", location: "there", event: {near: [x,y]}});
      console.log("emitting " + json);
      got = Env.http("POST", "$ENDPOINT" + "/api/json", json);
`
	rid := httpRequest("api/loc/rules/add", request)
	log.Printf("rid: %s", rid)
	if !strings.HasPrefix(rid, `{"id":`) {
		t.Fatalf("unexpected '%s'", rid)
	}

	request = `{"location":"here",
"rule": {"when":{"pattern":{"wants":"?x"}},
         "action":{"code":"var msg = 'eat ' + x; console.log(msg); msg;"}}}`

	rid = httpRequest("api/loc/rules/add", request)
	log.Printf("rid: %s", rid)
	if !strings.HasPrefix(rid, `{"id":`) {
		t.Fatalf("unexpected '%s'", rid)
	}

	request = `{"event": {"wants":"tacos"}}`
	got := httpRequest("api/loc/events/ingest?location=here", request)
	if strings.Index(got, `"values":["eat tacos"]`) < 0 {
		t.Fatalf("unexpected '%s'", got)
	}

	request = `location: there
rule: 
  when:
    pattern:
      wants: "?x"
  action:
    code: |
      var msg = 'eat ' + x; console.log(msg); msg;
`
	rid = httpRequest("api/loc/rules/add", request)
	log.Printf("rid: %s", rid)
	if !strings.HasPrefix(rid, `{"id":`) {
		t.Fatalf("unexpected '%s'", rid)
	}

	request = `{"event": {"wants":"tacos"}}`
	got = httpRequest("api/loc/events/ingest?location=there", request)
	if strings.Index(got, `"values":["eat tacos"]`) < 0 {
		t.Fatalf("unexpected '%s'", got)
	}

	defer func() {
		serviceRequest(map[string]interface{}{
			"uri": "/api/sys/admin/shutdown",
		})
		log.Printf("stopping")
	}()
}

<!--

Copyright 2015 Comcast Cable Communications Management, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

End Copyright -->

## Overview

This is a sample go client. It has following use cases.
* Add a Fact
* Search a Fact
* List Rules
* Process an Event



## Usage

### Starting
    To run execute go run main.go
    .../rulio\examples\go-client>go run main.go


### Shell variant 

```Shell
# Write a fact.
curl -s -d 'fact={"city":"London"}' "$ENDPOINT/api/loc/facts/add?location=$LOCATION"

# Search a fact.
curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=$LOCATION"

# Create a simple rule.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
		{"rule": {"when":{"pattern":{"code":"SEC-3423"},"age":"30"},
			"condition":{"pattern":{"city":"?x"}},
			"action":{"code":"var msg = 'city ' + x; console.log(msg); msg;"}}}
		EOF

# List a rule
curl -s  "$ENDPOINT/loc/rules/list?location=$LOCATION"

# Process a event.
curl -d 'event={ "name":"John", "age":30, "city":"London", "code":"SEC-3423" }'
	"$ENDPOINT/api/loc/events/ingest?location=$LOCATION" |   python3 -mjson.tool

```
### Sample output
```Shell
*Adding Facts city=London
Facts input arguments are : map[fact:map[city:London] location:here].
Facts is created with id 6a86e47a-e9af-49d1-80f5-c2bd4f0f1fd1.

*Searching Facts city:?x
Fact found with value &{[{{"city":"London"} 6a86e47a-e9af-49d1-80f5-c2bd4f0f1fd1 [map[?x:London]]}] 1 0 0}
Rule created with id:  a7b6bf9d-5c0e-4f56-a046-023aab0bb01b

*Listing Rules available rule
Available Rules [a7b6bf9d-5c0e-4f56-a046-023aab0bb01b]

*Processing for incoming events --> { "name":"John", "age":30, "city":"London", "code":"SEC-3423" }
city London
{"id":"ID is: ","result":"498a3b4e-5c44-4942-bab3-9380f8361bff Work is: ""}%!(EXTRA *core.FindRules=&{map[age:30 city:London code:SEC-3423 name:John] complete [0xc0002fe280] [city Lond
on]})
Result is: {
  "event": {
    "age": 30,
    "city": "London",
    "code": "SEC-3423",
    "name": "John"
  },
  "disposition": {
    "msg": "complete",
    "status": "complete"
  },
  "children": [
    {
      "rule": {
        "id": "a7b6bf9d-5c0e-4f56-a046-023aab0bb01b",
        "when": {
          "pattern": {
            "code": "SEC-3423"
          }
        },
        "condition": {
          "pattern": {
            "city": "?x"
          }
        },
        "actions": [
          {
            "code": "var msg = 'city ' + x; console.log(msg); msg;",
            "endpoint": "javascript",
            "subvars": true
          }
        ],
        "once": false,
        "props": null,
        "expires": 0
      },
      "bindingss": [
        {
          "?event": {
            "age": 30,
            "city": "London",
            "code": "SEC-3423",
            "name": "John"
          },
          "?location": "here",
          "?ruleId": "a7b6bf9d-5c0e-4f56-a046-023aab0bb01b"
        }
      ],
      "disposition": {
        "msg": "complete",
        "status": "complete"
      },
      "children": [
        {
          "bindings": {
            "?event": {
              "age": 30,
              "city": "London",
              "code": "SEC-3423",
              "name": "John"
            },
            "?location": "here",
            "?ruleId": "a7b6bf9d-5c0e-4f56-a046-023aab0bb01b"
          },
          "disposition": {
            "msg": "complete",
            "status": "complete"
          },
          "children": [
            {
              "bindings": {
                "?event": {
                  "age": 30,
                  "city": "London",
                  "code": "SEC-3423",
                  "name": "John"
                },
                "?location": "here",
                "?ruleId": "a7b6bf9d-5c0e-4f56-a046-023aab0bb01b",
                "?x": "London"
              },
              "action": {
                "code": "var msg = 'city ' + x; console.log(msg); msg;",
                "endpoint": "javascript",
                "subvars": true
              },
              "disposition": {
                "msg": "complete",
                "status": "complete"
              },
              "value": "city London"
            }
          ]
        }
      ],
      "DoneWork": {
        "disposition": {
          "msg": "complete",
          "status": "complete"
        }
      }
    }
  ],
  "values": ["city London"]
}



```
#!/bin/bash

# Copyright 2015 Comcast Cable Communications Management, LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# End Copyright


# Rough example of counting things in a sliding time window.

ENDPOINT=${ENDPOINT:-http://localhost:8001}
LOCATION=${LOCATION:-here}

curl "$ENDPOINT/loc/admin/clear?location=$LOCATION"

# Write a rule that keeps track of a sliding window of counts of
# events.  Uses a fact to store that state.  
#
# For fun, we allow the threshold ("max") to be passed with the event.
# Probably would not really do that.  Instead, the threshold would be
# a parameter of a rule template (or equivalent).
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"$LOCATION",
 "rule": {"when":{"pattern":{"event":"?x", "ts":"?ts", "max":"?max"}},
          "action":{"code":["var t = new Date(ts).getTime();",
                            "var then = t - 3 * 1000; // 3-sec window ",
                            "var found = Env.Search({event: x, window: '?w'});",
                            "var window = [];",
                            "if (0 < found.Found.length) {",
                            "   tss = found.Found[0].Bindingss[0]['?w'];",
                            "   for (var i = 0; i < tss.length; i++) {",
                            "       var u = tss[i];",
                            "       if (then <= u) {",
                            "          window.push(u); ",
                            "       }", 
                            "   }",
                            "}",
                            "window.push(t);",
                            "var fact = JSON.parse(JSON.stringify({event: x, window: window}));",
                            "console.log('fact', JSON.stringify(fact));",
                            "Env.AddFact(x + 'Window', fact);",
                            "var exceeded = max <= window.length;",
                            "var summary = {last: ts, count: window.length, exceeded: exceeded};",
                            "summary"]}}}
EOF

# Generate some events with increasing timestamps.
for M in $(seq 0 5); do
    cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | bin/jq -c .values
{"location":"$LOCATION", "event":{"event":"broken","max":3, "ts":"2015-08-10T13:55:0$M-05:00"}}
EOF
done

# Generate some events with increasing timestamps a little later than
# the last batch.
for M in $(seq 0 5); do
    cat <<EOF | curl -d -d "@-" $ENDPOINT/api/loc/events/ingest | bin/jq -c .values
{"location":"$LOCATION", "event":{"event":"broken","max":3, "ts":"2015-08-10T13:55:1$M-05:00"}}
EOF
done

# . examples/counting.sh
# {"status":"okay"}
# {"id":"11a3ba35-0944-45fc-86c5-f1c544b06164"}
# [{"count":1,"exceeded":false,"last":"2015-08-10T13:55:00-05:00"}]
# [{"count":2,"exceeded":false,"last":"2015-08-10T13:55:01-05:00"}]
# [{"count":3,"exceeded":true,"last":"2015-08-10T13:55:02-05:00"}]
# [{"count":4,"exceeded":true,"last":"2015-08-10T13:55:03-05:00"}]
# [{"count":4,"exceeded":true,"last":"2015-08-10T13:55:04-05:00"}]
# [{"count":4,"exceeded":true,"last":"2015-08-10T13:55:05-05:00"}]
# [{"count":1,"exceeded":false,"last":"2015-08-10T13:55:10-05:00"}]
# [{"count":2,"exceeded":false,"last":"2015-08-10T13:55:11-05:00"}]
# [{"count":3,"exceeded":true,"last":"2015-08-10T13:55:12-05:00"}]
# [{"count":4,"exceeded":true,"last":"2015-08-10T13:55:13-05:00"}]
# [{"count":4,"exceeded":true,"last":"2015-08-10T13:55:14-05:00"}]
# [{"count":4,"exceeded":true,"last":"2015-08-10T13:55:15-05:00"}]

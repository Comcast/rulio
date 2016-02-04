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


# Examples of in-process actions and their uses.


ENDPOINT=${ENDPOINT:-http://localhost:8001}
LOCATION=${LOCATION:here}

WEATHER="http://api.openweathermap.org/data/2.5/weather?units=metric&appid=$WEATHER_KEY"
if [ -z "$WEATHER_KEY" ]; then
    echo "Need WEATHER_KEY for http://openweathermap.org/current" >&2
    exit 1
fi

curl -s "$ENDPOINT/loc/admin/clear?location=$LOCATION"

# Use the utility '/sys/util/js' to see what you can do with in-process Javascript.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":"console.log('hello'); 'hello';"}
EOF

# See what's in our environment.
cat <<EOF | curl -s -d "@-" $ENDPOINT/loc/util/js
{"location":"$LOCATION",
 "code":["var acc = [];",
         "for (var p in Env) { acc.push(p); }",
         "acc;"]}
EOF

# There's a builtin `encode()` that does URL query encoding.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":"Env.encode(JSON.stringify({a:1}))"}
EOF

# Use the builtin 'http()' to get some weather data via an HTTP GET.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":"body = Env.http('GET','$WEATHER&q=Austin,TX'); console.log(body); weather = JSON.parse(body); weather.wind;"}
EOF
# You should see something like '{"result": map[speed:7.06 deg:178.504]}'

# Code can be an array of strings to help with readability (since JSON
# can't handle multiline strings).
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":["body = Env.http('GET','$WEATHER&&q=Austin,TX');",
         "weather = JSON.parse(body);",
         "weather.wind;"]}
EOF
# You should see something like '{"result": map[speed:7.06 deg:178.504]}'

# Get the actual wind speed.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":"body = Env.http('GET','$WEATHER&q=Austin,TX'); weather = JSON.parse(body); weather.wind.speed;"}
EOF
# You should see something like '{"result": 7.06}'

# We can use the builtin 'http()' to POST data to an HTTP server.
# Let's store a fact (locally).  
# 
# In real life, rather than hardcode an engine endpoint, the endpoint
# could be exposed via `Env` by the system.  For convenience, we just
# hardcode the endpoint.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":["json = JSON.stringify({uri: '/api/loc/facts/add', location: '$LOCATION', fact: {a: 1+2}});",
         "body = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
         "body;"]}
EOF

# Check that that fact it's there via Javascript.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":["json = JSON.stringify({uri: '/api/loc/facts/search', location: '$LOCATION', pattern: {a: '?x'}});",
         "body = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
         "body;"]}
EOF

# Get the actual value.
# Note that we have to parse the response body.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":["json = JSON.stringify({uri: '/api/loc/facts/search', location: '$LOCATION', pattern: {a: '?x'}});",
         "body = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
         "JSON.parse(body).Found[0].Bindingss[0]['?x'];"]}
EOF
# You should get '{"result": 3}'.

# We can use facts to store Javascript libraries.  In typical
# application, a Javascript library would be served by an external
# process.  See the example in the top-level README.  That example
# uses a library at 'http://localhost:6669/libs/tester.js'.  For
# quick, local testing, we can use a fact.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/facts/add
{"location":"$LOCATION",
 "fact":{"javascriptLibrary":"foo",
         "code": "function foo(x,y) { return x+y; }"}}
EOF

# We can manually use that library by getting, eval-ing, and calling
# it.  Just an odd example.
cat <<EOF | curl -s -d "@-" $ENDPOINT/sys/util/js
{"code":["json = JSON.stringify({uri: '/api/loc/facts/search', location: '$LOCATION', pattern: {javascriptLibrary:'foo',code:'?code'}});",
         "body = Env.http('POST', '$ENDPOINT' + '/api/json', json);",
		 "eval(JSON.parse(body).Found[0].Bindingss[0]['?code']);",
		 "foo(1,2);"]}
EOF
# You should get '{"result": 3}'.


# As noted above, it's usually better to serve your Javascript
# libraries.  For an example, see 'examples/wind.js'.  That code can
# be served by
#
#   (cd examples && ./libraries.py) &
#
# In this example, we're running Javascript relative to a location.
# (That way we could resolve a logical library name to a URL, but we
# don't do that here.)
cat <<EOF | curl -s -d "@-" $ENDPOINT/loc/util/js
{"location":"$LOCATON",
 "code":"wind('Austin, TX','$WEATHER_KEY')",
 "libraries":["http://localhost:6669/libs/wind.js"]}
EOF

# Use that library in a rule condition.  Here we use 'Env.out' to emit
# a result from the action.  That result is then returned in the
# response to event ingestion.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"$LOCATION",
  "rule": {"when":{"pattern":{"bought":"?x"}},
           "condition":{"code":"0 < wind(x,'$WEATHER_KEY')", "libraries":["http://localhost:6669/libs/wind.js"]},
           "action":{"code":"var msg = 'go use ' + x; console.log(msg); Env.out(msg);"}}}
EOF

# Send an event to trigger the rule.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | python -mjson.tool
{"location":"$LOCATION", "event":{"bought":"kite"}}
EOF


# We have an example library for computing haversine distance.  See
# 'examples/libs/haversine.js' Test that library.

cat <<EOF | curl -s -d "@-" $ENDPOINT/loc/util/js
{"location":"$LOCATON",
 "code":"haversine(97.73,30.28,122.0,37.38)",
 "libraries":["http://localhost:6669/libs/haversine.js"]}
EOF

# Compute the distance between Austin and Sunnyvale.
cat <<EOF | curl -s -d "@-" $ENDPOINT/loc/util/js
{"location":"$LOCATON",
 "libraries":["http://localhost:6669/libs/haversine.js"],
 "code":["body = Env.http('GET','https://maps.googleapis.com/maps/api/geocode/json?address=Sunnyvale,+CA'); ",
         "var sunnyvale = JSON.parse(body).results[0].geometry.location; ",
         "body = Env.http('GET','https://maps.googleapis.com/maps/api/geocode/json?address=Austin,+TX'); ",
         "var austin = JSON.parse(body).results[0].geometry.location; ",
         "haversine(austin.lng,austin.lat, sunnyvale.lng,sunnyvale.lat);"]}
EOF
# {"result": 2370.965973202771}


# And now for something completely different.

# The follow two rules implement a FOR loop using chaining.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"there",
 "rule": {"when":{"pattern":{"for":"?v", "from":"?from", "to":"?to"}},
          "condition":{"code":"from < to"},
          "action":{"code":["console.log('iteration ' + v + '=' + from); ",
		            "request = {event: {for:v, from: from+1, to:to}, location:'there'}; ",
                            "request.uri = '/api/loc/events/ingest'; ",
                            "json = JSON.stringify(request); ",
                            "console.log('emitting ' + json); ",
                            "got = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
                            "console.log('send result ' + got);"]}}}
EOF

# Let's also say when this loop is complete.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"there",
 "rule": {"when":{"pattern":{"for":"?v", "from":"?from", "to":"?from"}},
          "action":{"code":"console.log(v + ' loop complete at ' + from);"}}}
EOF

# Start a loop.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | python -mjson.tool
{"location":"there", "event":{"for":"i", "from":2, "to":5}}
EOF
# You should see 'iteration i=...' in the engine output.


# A rule that stores geolocations of people when they check in.
# This rule also emits a geolocation event.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"there",
 "rule": {"when":{"pattern":{"person":"?who", "at":"?there"}},
          "action":{"code":["body = Env.http('GET','https://maps.googleapis.com/maps/api/geocode/json?address=' + there); ",
                            "var location = JSON.parse(body).results[0].geometry.location; ",
                            "json = JSON.stringify({uri: '/api/loc/facts/add', location: 'there', fact: {person: who, latlon: location}}); ",
                            "console.log('emitting ' + json);", 
                            "got = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
                            "console.log('facts/add result ' + got); ", 
		            "json = JSON.stringify({uri: '/api/loc/events/ingest', location: 'there', event: {person: who, latlon: location}}); ",
                            "console.log('emitting ' + json); ",
                            "got = Env.http('POST', '$ENDPOINT' + '/api/json', json); "]}}}
EOF

# A rule that reports when two people are near each other.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"there",
  "rule": {"when":{"pattern":{"person":"?x","latlon":"?there"}},
           "condition":{"and":[{"pattern":{"person":"?y","latlon":"?near"}},
                               {"code":"console.log('checking ' + x + ',' + y + ' at ' + JSON.stringify(there) + ',' + JSON.stringify(near)); true;"},
                               {"code":"x != y"},
                               {"code":"d = haversine(there.lng,there.lat,near.lng,near.lat); console.log('dist ' + d + ' ' + x + ' ' + y); d < 1000;", 
                                "libraries":["http://localhost:6669/libs/haversine.js"]}]},
           "action":{"code":["console.log(x + ' is near ' + y); ",
                             "json = JSON.stringify({uri: '/api/loc/events/ingest', location: 'there', event: {near: [x,y]}}); ",
                             "console.log('emitting ' + json); ",
                             "got = Env.http('POST', '$ENDPOINT' + '/api/json', json); "]}}}

EOF

# A diagnostic rule that just reports geolocations.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"there",
  "rule": {"when":{"pattern":{"person":"?who","latlon":"?there"}},
           "action":{"code":"var msg = who + ' is at ' + JSON.stringify(there); console.log(msg); Env.out(msg)"}}}
EOF

# A rule that does something boring when people are near each other.
cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/rules/add
{"location":"there",
  "rule": {"when":{"pattern":{"near":"?people"}},
           "action":{"code":"var msg = '' + people + ' are near each other.'; console.log(msg); Env.out(msg)"}}}
EOF



cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | python -mjson.tool
{"location":"there", "event":{"person":"Homer", "at":"Austin,TX"}}
EOF

cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | python -mjson.tool
{"location":"there", "event":{"person":"Bart", "at":"Houston,TX"}}
EOF

cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | python -mjson.tool
{"location":"there", "event":{"person":"Lisa", "at":"Dallas,TX"}}
EOF

cat <<EOF | curl -s -d "@-" $ENDPOINT/api/loc/events/ingest | python -mjson.tool
{"location":"there", "event":{"person":"Marge", "at":"Sunnyvale,CA"}}
EOF

# Look for "Bart,Lisa are near each other." in the huge pile of engine output.


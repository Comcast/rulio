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


![Rulio is a rules engine](https://raw.githubusercontent.com/Comcast/rulio/master/doc/Rulio_logo_400x124.png)

## Overview

A rules engine.  You write rules and send events.  You can also write
some facts that rules can use.  When an event arrives, the system
finds candidate rules.  A candidate rule's condition is evaluated to
find zero or more sets of variable bindings.  For each set of variable
bindings, the rule's actions are executed.

See the [docs](doc) for more.  In particular, see `doc/Manual.md`.
There are lots of examples in `examples/`.

## License

This software is released under the Apache License, Version 2.0.  See
`LICENSE` in this repo.


## Usage

### Starting

To compile, you need [Go](https://golang.org/).  Then

```Shell
(cd rulesys && go get . && go install)
bin/startengine.sh &
ENDPOINT=http://localhost:8001
LOCATION=here
curl -s $ENDPOINT/version
```

If you see some JSON, the engine is probably running.  Check
`engine.log` to see some logging.


### A simple rule

Now let's use that engine.  In these examples, we'll talk to the
engine using is primitive HTTP API.

```Shell
# Get this handy tool.
if [ ! -x bin/jq ]; then (cd bin && wget -nc http://stedolan.github.io/jq/download/linux64/jq && chmod 755 jq); fi

# Write a fact.
curl -s -d 'fact={"have":"tacos"}' "$ENDPOINT/api/loc/facts/add?location=$LOCATION"

# Query for the fun of it.
curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=$LOCATION" | \
  python -mjson.tool

# Write a simple rule.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"when":{"pattern":{"wants":"?x"}},
          "condition":{"pattern":{"have":"?x"}},
          "action":{"code":"var msg = 'eat ' + x; console.log(msg); msg;"}}}
EOF

# Send an event.
curl -d 'event={"wants":"tacos"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool
```

The `events/ingest` output is pretty big.  This data contains
sufficient information to enable you to reattempt/resume event
processing in the case the engine encountered one or more errors
during the previous processing.


### Scheduled rule

Now let's write a little scheduled rule.

```Shell
# First a quick check to see if a Javascript action can give us a timestamp.
curl -d 'code=new Date().toISOString()' $ENDPOINT/api/sys/util/js

# Write a scheduled rule.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"schedule":"+3s",
          "condition":{"pattern":{"have":"?x"}},
          "action":{"code":"console.log('eating ' + x + ' at ' + (new Date().toISOString()) + '.');"}}}
EOF
```

Look for a line starting with `eating tacos` in the engine output.

```Shell
grep -F 'eating tacos' engine.log
```

That rule runs only once.  Three seconds from when it was created.
(We can also use full cron syntax to specify a repeating schedule.)


### Action talking to an external service

Now let's make a rule with an action that talks to an external
service.  We'll start a dummy service that just prints out what it
hears.

```Shell
# Start our dummy service.  Use another window.
(cd examples && ./endpoint.py) &

# See if it works.
curl "http://localhost:6668/foo?likes=tacos"
# Should see some data in that service's window.

# Write the rule.  This rule has no condition.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"when":{"pattern":{"wants":"?x"}},
          "action":{"code":"Env.http('GET','http://localhost:6668/do?order=' + Env.encode(x))"}}}
EOF

# Send an event.  Should trigger that action.
curl -d 'event={"wants":"Higher Math"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool
# You should see an order in the `endpoint.py' output.
```


### Rule condition querying an external service

The rules engine can query external services during rule condition
evaluation.  Such a service is called an "external fact service".  We
have a few example fact services in `examples/`.  Here's one that can
report the weather.

```Shell
(cd examples && ./weatherfs.py) &

# Test it.
curl -d '{"locale":"Austin,TX","temp":"?x"}' 'http://localhost:6666/facts/search'

# Write a rule that uses that source of facts.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"when":{"pattern":{"visitor":"?who"}, "location":"here"},
          "condition":{"and":[{"pattern":{"locale":"Austin,TX","temp":"?temp"},
                               "locations":["http://localhost:6666/facts/search"]},
                              {"code":"console.log('temp: ' + temp); 0 < temp"}]},
          "action":{"code":"Env.http('GET','http://localhost:6668/report?weather=' + Env.encode('warm enough'))"}}}
EOF

# Send an event.  Should trigger that action.
curl -d 'event={"visitor":"Homer"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool
```

### Javascript libraries

Let's use a library of Javascript code in a rule action.

```Shell
# Start a library server.
(cd examples && ./libraries.py) &

# Check that we can get a library.
curl http://localhost:6669/libs/tester.js

# Write the rule.
cat <<EOF | curl -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"when":{"pattern":{"wants":"?x"}},
          "condition":{"code":"isGood(x)",
                       "libraries":["http://localhost:6669/libs/tester.js"]},
          "action":{"code":"var msg = \"Serve \" + x; console.log(msg); msg;"}}}
EOF

# Send an event.  Should trigger that action.
curl -d 'event={"wants":"Higher Math"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool

# Send another event.  Should not trigger that action.
curl -d 'event={"wants":"Duff Light"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool
```

You can also use libraries in Javascript rule actions.

Your libraries can be pretty fancy (see `example/libs/haversine.js`),
but be cautious about efficiency and robustness.  If you find yourself
wanting to do a lot of work in action Javascript, think about writing
an action executor instead.


### Action executors

If you don't want to write your actions in Javascript, which runs
inside the rules engine, you can use *action executors*.  An action
executor is an external service that is given rule actions to execute.
In a serious deployment, an action executor endpoint would probably
just queue those actions for a pool of workers to process.

An action executor can do or execute anything in any language or
specification.  Up to the author of the executor.

We have an example action executor in Python in `examples/executor.py`.

```Shell
# Run the toy action executor.
(cd examples && ./executor.py) &

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule":{"when":{"pattern":{"drinks":"?x"}},
         "action":{"endpoint":"http://localhost:8081/execbash",
                   "code":{"order":"?x"}}}}
EOF

# Send an event.
curl -d 'event={"drinks":"milk"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
  python -mjson.tool
```

That event should generate a request to the example action executor,
which doesn't actually do anything.


### Getting some statistics

Finally, let's get some statistics.

```Shell
curl -s "$ENDPOINT/api/loc/admin/stats?location=$LOCATION" | python -mjson.tool
curl -s "$ENDPOINT/api/sys/stats" | python -mjson.tool
```

## Conclusion

Take a look at the `doc/Manual.md` for more infomation.


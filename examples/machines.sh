
curl -s -d 'fact={"have":"tacos"}' "$ENDPOINT/api/loc/facts/add?location=$LOCATION"

curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=$LOCATION" | \
  python -mjson.tool

curl -s "$ENDPOINT/api/loc/admin/clear?location=$LOCATION"

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"when":{"patterns":[{"wants":"?x"},{"needs":"?x"}]},
          "action":{"code":"var msg = 'eat ' + x + ' ' + _pn; console.log(msg); msg;"}}}
EOF

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"when":{"patterns":[{"wants":"?x"},{"needs":"?x"}]},
          "condition": {"code": "(_pn == 0) && x.length < 10"},
          "action":{"code":"var msg = 'eat ' + x + ' ' + _pn; console.log(msg); msg;"}}}
EOF

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"rule": {"props":{"foo":"bar"},
           "when":{"patterns":[{"wants":"?x"},{"needs":"?x"}]},
          "condition": {"code": "(_pn == 0 && x.length < 10) || (_pn == 1)"},
          "action":{"code":"var msg = 'eat ' + x + ' ' + _pn + ' ' + Env.ruleProps.foo; console.log(msg); msg;"}}}
EOF

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"id":"1",
 "rule": {"props":{"foo":"bar"},
           "when":{"patterns":[{"wants":"?x"},{"needs":"?x"}]},
          "condition": {"code": "(_pn == 0 && x.length < 10) || (_pn == 1)"},
          "action":{"code":"var msg = 'eat ' + x + ' ' + _pn + ' ' + Env.ruleProps.foo; console.log(msg); msg;"}}}
EOF

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"id":"1",
 "rule": {"props":{"foo":"bar"},
           "when":{"patterns":[{"wants":"?x"},{"needs":"?x"}]},
          "condition": {"code": "(_pn == 0 && x.length < 10) || (_pn == 1)"},
          "action":{"code":"evolve('1')",
                    "opts":{"libraries":["http://localhost:6669/libs/machine.js"]}}}}
EOF

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"id":"1",
 "rule": {"props":{"state":"start","bs":{"foo":"bar"}},
          "when":{"patterns":[{"wants":"?x"},{"needs":"?x"}]},
          "condition": {"code": "(_pn == 0 && x.length < 10) || (_pn == 1)"},
          "action":{"code":"evolve('1')",
                    "opts":{"libraries":["http://localhost:6669/libs/machine.js"]}}}}
EOF

cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=$LOCATION"
{"id":"1",
 "rule": {"props":{"state":"start","bs":{"foo":"bar"}},
          "policies":{"cutting":true},
          "when":{"patterns":[{"wants":["?x"]},{"needs":["?x"]}]},
          "condition": {"code": "(_pn == 0 && x.length < 10) || (_pn == 1)"},
          "action":{"code":"evolve('1',x)",
                    "opts":{"libraries":["http://localhost:6669/libs/machine.js"]}}}}
EOF

curl -d 'event={"wants":"tacos"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool

curl -d 'event={"wants":"terribletacos"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool

curl -d 'event={"needs":"terribletacos"}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool

curl -d 'event={"needs":["terribletacos","TERRIBLETACOS"]}' "$ENDPOINT/api/loc/events/ingest?location=$LOCATION" | \
   python -mjson.tool

curl -s "$ENDPOINT/api/loc/admin/clear?location=$LOCATION"


# A short, incomplete tour.

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


# First check out the examples in the top-level README.md.

ENDPOINT=${ENDPOINT:-http://localhost:8001}

# Basic facts

# First remove any facts and rules.
curl "$ENDPOINT/loc/admin/clear?location=there"


# Write a fact.
curl -d 'fact={"a":1}' "$ENDPOINT/loc/facts/add?location=there"
# Get a binding.
curl -d 'pattern={"a":"?x"}' "$ENDPOINT/loc/facts/search?location=there" | python -mjson.tool
# Write another fact.
curl -d 'fact={"a":2}' "$ENDPOINT/loc/facts/add?location=there"
# Get two bindings.
curl -d 'pattern={"a":"?x"}' "$ENDPOINT/loc/facts/search?location=there" | python -mjson.tool
# "Replace" fact(s).
curl -d 'fact={"a":3}' -d 'pattern={"a":"?x"}' "$ENDPOINT/loc/facts/replace?location=there"
# Get new binding.
curl -d 'pattern={"a":"?x"}' "$ENDPOINT/loc/facts/search?location=there" | python -mjson.tool
# "Take" a fact.
curl -d 'pattern={"a":"?x"}' "$ENDPOINT/loc/facts/take?location=there" | python -mjson.tool
# No other fact to take.
curl -d 'pattern={"a":"?x"}' "$ENDPOINT/loc/facts/take?location=there" | python -mjson.tool
# Write another fact.
curl -d 'fact={"b":1}' -d 'id=f1' "$ENDPOINT/loc/facts/add?location=there"
# Get a binding.
curl -d 'pattern={"b":"?x"}' "$ENDPOINT/loc/facts/search?location=there" | python -mjson.tool
# Replace the fact using the explicit fact ID.
curl -d 'fact={"b":2}' -d 'id=f1' "$ENDPOINT/loc/facts/add?location=there"
# See the updated binding.
curl -d 'pattern={"b":"?x"}' "$ENDPOINT/loc/facts/search?location=there" | python -mjson.tool


# Basic rules

# This rule will fire on an event like {"b":"tacos"}.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=there"
{"rule": {"when":{"pattern":{"b":"?x"}},
          "action":{"code":"console.log('b = ' + x);"}}}
EOF

# Send such an event.
curl -d 'event={"b":"tacos"}' "$ENDPOINT/api/loc/events/ingest?location=there"
# You should see 'b = tacos" in the engine process stdout.

# Write some more facts to demostrate rule conditions.
curl -d 'fact={"have":"tacos"}' "$ENDPOINT/loc/facts/add?location=there"
curl -d 'fact={"have":"chips"}' "$ENDPOINT/loc/facts/add?location=there"

# Check those facts.
curl -d 'pattern={"have":"?x"}' "$ENDPOINT/loc/facts/search?location=there" | \
  python -mjson.tool

# Write a new rule with a condition.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=there"
{"rule": {"when":{"pattern":{"wants":"?x"}},
          "condition":{"pattern":{"have":"?x"}},
          "action":{"code":"console.log('serve ' + x);"}}}
EOF

# Send an important event.
curl -d 'event={"wants":"tacos"}' "$ENDPOINT/api/loc/events/ingest?location=there" | \
  python -mjson.tool
# You should see "serve tacos" in the engine process stdout.


# Scheduled rules

# Write another fact for a rule condition.
curl -d 'fact={"likes":"tacos"}' "$ENDPOINT/api/loc/facts/add?location=there"

# Write a scheduled rule.
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add?location=there"
{"rule": {"schedule":"+10s",
          "condition":{"pattern":{"likes":"?x"}},
          "actions":[{"code":"console.log(\"eating \" + x);"}]}}
EOF
# In 10 seconds, you should see "eating tacos" in the engine process stdou.

# Get some location stats
curl "$ENDPOINT/loc/stats?location=there" | python -mjson.tool
curl "$ENDPOINT/sys/stats" | python -mjson.tool



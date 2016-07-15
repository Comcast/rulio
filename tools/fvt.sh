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


# Dummy functional verification test
#
# Ain't much but better than nothing.

ENDPOINT=${1:-http://localhost:8001}
ACCOUNT=${2:-here}

# mkdir -p tmp && DIR=`mktemp -d tmp/prof.XXXX`
DIR=tmp/fvt
mkdir -p $DIR && rm -f $DIR/*.txt
echo "DIR=$DIR"
N=1
FAILURES=0
PASSES=0

function fail {
    echo $N failed
    ((N++))
    ((FAILURES++))
    return 0
}

function pass {
    echo $N passed
    ((N++))
    ((PASSES++))
    return 0
}

echo "# $N       Get version"
curl -s $ENDPOINT/version | tee $DIR/$N.txt | \
    grep -q "version" && pass || fail

echo "# $N       Make a request to a bad URI"
curl -s $ENDPOINT/bad 2>&1 | tee $DIR/$N.txt | \
   grep -q "Unknown" && pass || fail

echo "# $N       Clear location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Search for no facts"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

echo "# $N       Add a fact"
curl -s --data-urlencode 'fact={"have":"tacos"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       Search for fact we added"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "tacos" && pass || fail

echo "# $N       Add another fact"
curl -s --data-urlencode 'fact={"have":"chips"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       Search for facts we added"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "chips" && pass || fail

echo "# $N       Add a rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
  "rule": {"when":{"pattern":{"wants":"?x"}},
           "condition":{"pattern":{"have":"?x"}},
           "action":{"code":"console.log(\"eat \" + x); \"eat \" + x;"}}}
EOF

echo "# $N       Send an event to trigger that rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["eat tacos"]' && pass || fail
 

# A one-shot scheduled rule that writes a fact so we can check that
# the rule fired.

echo "# $N       Verify that the cron rule is NOT there"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=onceTest" | tee $DIR/$N.txt | \
    grep -vq "fact" && pass || fail

echo "# $N       Write a cron rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
 "id": "onceTest",
 "rule": {"schedule":"+2s",
          "action":{"code":["console.log('eating at ' + (new Date().toISOString()) + '.'); ",
                            "json = JSON.stringify({uri: '/api/loc/facts/add', location: '$ACCOUNT', id: 'likesChips', fact: {likes: 'chips'}}); ",
                            "got = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
                            "console.log('facts/add result ' + got); ", 
                            "got; "]}}}
EOF

echo "# $N       Verify that the cron rule is there"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=onceTest" | tee $DIR/$N.txt | \
    grep -q "fact" && pass || fail

echo "# $N       Query the location for the fun of it"
curl -s "$ENDPOINT/api/loc/rules/list?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "onceTest" && pass || fail


# Wait for it to fire (possibly more than once if there's a bug).
sleep 6

echo "# $N       Find the fact that the rule wrote"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "chips" && pass || fail

echo "# $N       Check again to make sure we see only one such fact"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    jq '.Found|length' | grep -q 1 && pass || fail

echo "# $N       Verify that the cron rule is gone"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=onceTest" | tee $DIR/$N.txt | \
    grep -vqF "fact" && pass || fail

echo "# $N       Remove the fact the rule wrote"
curl -s "$ENDPOINT/api/loc/facts/rem?location=$ACCOUNT&id=likesChips" | tee $DIR/$N.txt | \
    grep -qF '"removed":"likesChips"' && pass || fail

echo "# $N       Verify that the fact is gone"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail


# Now we'll try another cron rule, but we're going to delete it before
# it should run.

echo "# $N       Check we don't already have the cron rule"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

echo "# $N       Write a cron rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
 "id": "deleteCronRuleTest",
 "rule": {"schedule":"+2s",
          "action":{"code":["console.log('drinking at ' + (new Date().toISOString()) + '.'); ",
                            "json = JSON.stringify({uri: '/api/loc/facts/add', location: '$ACCOUNT', id: 'likesChips', fact: {likes: 'beer'}}); ",
                            "got = Env.http('POST', '$ENDPOINT' + '/api/json', json); ",
                            "console.log('facts/add result ' + got); ", 
                            "got; "]}}}
EOF

echo "# $N       Verify that the cron rule is there"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=deleteCronRuleTest" | tee $DIR/$N.txt | \
    grep -q "fact" && pass || fail

echo "# $N       Remove the rule"
curl -s "$ENDPOINT/api/loc/facts/rem?location=$ACCOUNT&id=deleteCronRuleTest" | tee $DIR/$N.txt | \
    grep -qF '"removed":"deleteCronRuleTest"' && pass || fail

# Wait for it NOT to fire.
sleep 6

echo "# $N       Do not find the fact that the rule did not write"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail


# Test DeleteLocation

echo "# $N       DeleteLocation: Add a fact"
curl -s --data-urlencode 'fact={"have":"tacos"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       DeleteLocation: Search for fact we added"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "tacos" && pass || fail

echo "# $N       DeleteLocation: Delete location"
curl -s "$ENDPOINT/api/loc/admin/delete?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       DeleteLocation: Search for no facts"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

# Event processing time limit

THEN=$(TZ=UTC date '+%FT%T.%NZ')

echo "# $N       Event time limit: Add a rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
  "rule": {"when":{"pattern":{"wants":"?x","submitted":"?then"}},
           "condition":{"code":"Env.secsFromNow(then) < 2"},
           "action":{"code":"console.log(\"deliver \" + x); \"deliver \" + x;"}}}
EOF

echo "# $N       Event time limit: Send an event to trigger that rule and get an action"
curl -s --data-urlencode "event={\"wants\":\"tacos\",\"submitted\":\"$THEN\"}" \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["deliver tacos"]' && pass || fail

sleep 4
echo "# $N       Event time limit: Send an event to trigger that rule and get no action"
curl -s --data-urlencode "event={\"wants\":\"tacos\",\"submitted\":\"$THEN\"}" \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -vqF '"values":["deliver tacos"]' && pass || fail


# Id props

echo "# $N       Clear location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Search for no facts"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

echo "# $N       Add an id prop"
curl -s --data-urlencode 'fact={"id":"homer","!likes":"tacos"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       Overwrite it"
curl -s --data-urlencode 'fact={"id":"homer","!likes":"beer"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       Make sure we get just one value"
curl -s --data-urlencode 'pattern={"id":"homer","!likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    jq '.Found|length' | grep -q 1 && pass || fail

echo "# $N       Remove the fact"
curl -s "$ENDPOINT/api/loc/facts/rem?location=$ACCOUNT&id="'!homer.likes' | tee $DIR/$N.txt | \
    grep -qF '"removed":"!homer.likes"' && pass || fail

echo "# $N       Search for no facts"
curl -s --data-urlencode 'pattern={"!likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

# Fact expiration

echo "# $N       Clear location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Search for no facts"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

echo "# $N       Add an expiring fact"
curl -s --data-urlencode 'fact={"person":"homer","likes":"pie", "ttl":"2s"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       Search for fact we added"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "pie" && pass || fail

sleep 3

echo "# $N       Search for no facts (because previous fact expired)"
curl -s --data-urlencode 'pattern={"likes":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail


# Rule expiration

echo "# $N       Clear location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Add a rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
 "rule": {"ttl":"2s",
          "when":{"pattern":{"wants":"?x"}},
          "action":{"code":"console.log(\"eat \" + x); \"eat \" + x;"}}}
EOF

echo "# $N       Send an event to trigger that rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["eat tacos"]' && pass || fail
 
sleep 3

echo "# $N       Send an event to expired rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -vqF '"values":["eat tacos"]' && pass || fail
 

# Rule "updating"

echo "# $N       Clear location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Add a rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
 "id": "catch-22",
 "rule": {"when":{"pattern":{"wants":"?x"}},
          "action":{"code":"console.log(\"eat \" + x); \"eat \" + x;"}}}
EOF

echo "# $N       Overwrite that rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
 "id": "catch-22",
 "rule": {"when":{"pattern":{"wants":"?x"}},
          "action":{"code":"console.log(\"serve \" + x); \"serve \" + x;"}}}
EOF


echo "# $N       Send an event to trigger that rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["serve tacos"]' && pass || fail
 

# Disable a rule

echo "# $N       Clear location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Add a rule (to be disabled later)"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
 "id": "rdis",
 "rule": {"when":{"pattern":{"wants":"?x"}},
          "action":{"code":"console.log(\"eat \" + x); \"eat \" + x;"}}}
EOF

echo "# $N       Is the rule enabled?"
curl -s "$ENDPOINT/api/loc/rules/enabled?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF 'true' && pass || fail

echo "# $N       Send an event to trigger that rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["eat tacos"]' && pass || fail

echo "# $N       Disable the rule"
curl -s "$ENDPOINT/api/loc/rules/disable?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF '"disabled"' && pass || fail

echo "# $N       Is the rule disabled?"
curl -s "$ENDPOINT/api/loc/rules/enabled?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF 'false' && pass || fail

echo "# $N       Send an event not to trigger that rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":[]' && pass || fail

echo "# $N       Enable the rule"
curl -s "$ENDPOINT/api/loc/rules/enable?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF '"enabled"' && pass || fail

echo "# $N       Send an event to trigger that rule"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["eat tacos"]' && pass || fail

echo "# $N       Disable the rule"
curl -s "$ENDPOINT/api/loc/rules/disable?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF '"disabled"' && pass || fail

echo "# $N       Verify the rule is disable"
curl -s "$ENDPOINT/api/loc/rules/enabled?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF 'false' && pass || fail

echo "# $N       Delete the rule (which should cause the deletion of the property)"
curl -s "$ENDPOINT/api/loc/rules/rem?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF '"removed"' && pass || fail

echo "# $N       Search for no facts (since the rule deletion should have also deleted the prop)"
curl -s --data-urlencode 'pattern={"id":"rdis"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF '"Found":[]' && pass || fail

# With parent locations, we don't (currently) get an error.
echo "# $N       Try to check if the deleted rule is enabled (expecting a default of true)"
curl -s "$ENDPOINT/api/loc/rules/enabled?location=$ACCOUNT&id=rdis" | tee $DIR/$N.txt | \
    grep -qF 'true' && pass || fail


# Rule policy: RetryFromCondition

# This policy is a pain to test.

echo "# $N       Clear the location"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | \
    tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Add a fact that a rule condition will check"
curl -s --data-urlencode 'fact={"have":"tacos"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT&id=tacos" | \
    tee $DIR/$N.txt | \
    grep -q "tacos" && pass || fail

echo "# $N       Verify we have that fact"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
  "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | \
    tee $DIR/$N.txt | \
    grep -q "tacos" && pass || fail

echo "# $N       Set a magic test value"
curl -s --data-urlencode 'value=0' \
   "$ENDPOINT/api/sys/util/setJavascriptTestValue" | \
    tee $DIR/$N.txt | \
    grep -q 0 && pass || fail

echo "# $N       Create a rule with a condition and and a questionable action"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && pass || fail
{"location":"$ACCOUNT",
  "rule": {"when":{"pattern":{"wants":"?x"}},
	       "condition":{"pattern":{"have":"tacos"}},
		   "policies":{"retryFromCondition":true},
           "action":{"code":["if (Env.getJavascriptTestValue() == 0) { throw('problem'); }",
		                     "'serve ' + x;"]}}}
EOF

echo "# $N       Submit an event but get an (expected) error from the action"
curl -s --data-urlencode 'event={"wants":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | \
    tee $DIR/$N.txt | \
    grep -q '"msg":"problem"' && pass || fail

cat $DIR/$((N-1)).txt | jq -c .result > $DIR/$N.json
R=$N
echo "# $N       Retry the event processing but the action should still fail"
curl -s --data-urlencode "work@$DIR/$R.json" \
   "$ENDPOINT/api/loc/events/retry?location=$ACCOUNT" | \
    tee $DIR/$N.txt | \
    grep -q '"msg":"problem"' && pass || fail

echo "# $N       Prevent the condition from being satisfied"
curl -s "$ENDPOINT/api/loc/facts/rem?location=$ACCOUNT&id=tacos" | \
    tee $DIR/$N.txt | \
    grep -q '"removed":"tacos"' && pass || fail

echo "# $N       Now prevent the action from throwing an exception"
curl -s --data-urlencode 'value=1' \
   "$ENDPOINT/api/sys/util/setJavascriptTestValue" | \
    tee $DIR/$N.txt | \
    grep -q '"result":1' && pass || fail

echo "# $N       Re-attempt the event processing work"
curl -s --data-urlencode "work@$DIR/$R.json" \
   "$ENDPOINT/api/loc/events/retry?location=$ACCOUNT" | \
    grep -qF '"values":[]' && pass || fail 

echo "# $N       Add that fact back so that the condition is satisfied"
curl -s --data-urlencode 'fact={"have":"tacos"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT&id=tacos" | \
    tee $DIR/$N.txt | \
    grep -q '"id":"tacos"' && pass || fail

echo "# $N       Should see a value from the action"
curl -s --data-urlencode "work@$DIR/$R.json" \
   "$ENDPOINT/api/loc/events/retry?location=$ACCOUNT" | \
    grep -qF '"values":["serve tacos"]' && pass || fail

## Inherited rules (parent locations)

PARENT="$ACCOUNT.mom"

echo "#        Clear location"
((N++)); curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "#        Clear location"
((N++)); curl -s "$ENDPOINT/api/loc/admin/clear?location=$PARENT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "#        Add a rule"
cat <<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add" | tee $DIR/$N.txt | grep -q '"id"' && echo $N passed || echo $N failed
{"location":"$PARENT",
 "id":"momRule",
 "rule": {"when":{"pattern":{"wants":"?x"}},
          "action":{"code":"console.log(\"eat \" + x); \"eat \" + x;"}}}
EOF

echo "#        Declare a parent"
curl -s --data-urlencode 'set=["'"$PARENT"'"]' \
    "$ENDPOINT/api/loc/parents?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "$PARENT" && pass || fail

echo "#        Read the parent"
curl -s "$ENDPOINT/api/loc/parents?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -qF "$PARENT" && pass || fail

echo "#        Send an event to trigger that rule"
curl -s --data-urlencode 'event={"wants":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":["eat chips"]' && pass || fail
 
echo "#        Disable the rule"
curl -s "$ENDPOINT/api/loc/rules/disable?location=$ACCOUNT&id=momRule" | tee $DIR/$N.txt | \
    grep -qF '"disabled"' && pass || fail

echo "#        Send an event not to trigger that rule"
curl -s --data-urlencode 'event={"wants":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=$ACCOUNT" | tee $DIR/$N.txt | \
   grep -qF '"values":[]' && pass || fail

## deleteWith

echo "#        Clear location"
((N++)); curl -s "$ENDPOINT/api/loc/admin/clear?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "okay" && pass || fail

echo "# $N       Add a fact"
curl -s --data-urlencode 'fact={"have":"tacos"}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT&id=f1" | tee $DIR/$N.txt | \
    grep -q '"id"' && pass || fail

echo "# $N       Search for fact we added"
curl -s --data-urlencode 'pattern={"have":"?x"}' \
    "$ENDPOINT/api/loc/facts/search?location=$ACCOUNT" | tee $DIR/$N.txt | \
    grep -q "tacos" && pass || fail

echo "# $N       Add a dependent fact"
curl -s --data-urlencode 'fact={"likes":"chips","deleteWith":["f1"]}' \
    "$ENDPOINT/api/loc/facts/add?location=$ACCOUNT&id=f2" | tee $DIR/$N.txt | \
    grep -q '"f2"' && pass || fail

echo "# $N       Get the fact by id"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=f2" | tee $DIR/$N.txt | \
    grep -q 'chips' && pass || fail

echo "# $N       Delete the dependency"
curl -s "$ENDPOINT/api/loc/facts/rem?location=$ACCOUNT&id=f1" | tee $DIR/$N.txt | \
    grep -q 'removed' && pass || fail

echo "# $N       Verify that we deleted the dependency"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=f1" | tee $DIR/$N.txt | \
    grep -q 'not found' && pass || fail

echo "# $N       Verify that we deleted the second fact"
curl -s "$ENDPOINT/api/loc/facts/get?location=$ACCOUNT&id=f2" | tee $DIR/$N.txt | \
    grep -q 'not found' && pass || fail


echo "Passed: $PASSES"
echo "Failed: $FAILURES"

[ $FAILURES -eq 0 ] || exit 1

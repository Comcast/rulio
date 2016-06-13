#!/bin/bash

# This little script is a low-level demo of parent rules and facts.

set -e

CHILD=bart
PARENT="$CHILD.mom"

# Clear the locations we'll use.
curl -s "$ENDPOINT/api/loc/admin/clear?location=&CHILD"
curl -s "$ENDPOINT/api/loc/admin/clear?location=$PARENT"

# Add a rule to the parent.
cat<<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add"
{"location":"$PARENT",
 "id":"momRule1",
 "rule": {"when":{"pattern":{"wants":"?x"}},
          "action":{"code":"console.log(\"eat \" + x); \"eat \" + x;"}}}
EOF

# Set the child's parent.
curl -s --data-urlencode 'set=["'"$PARENT"'"]' \
    "$ENDPOINT/api/loc/parents?location=&CHILD"

# Use the 'parents' API to get the child's parent.
curl -s "$ENDPOINT/api/loc/parents?location=&CHILD"

# Send an event to the child and hope the parent's rule is triggered.
curl -s --data-urlencode 'event={"wants":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=&CHILD" | \
    python -mjson.tool

# Disable the parent's rule in the child. 
curl -s "$ENDPOINT/api/loc/rules/disable?location=&CHILD&id=momRule1"

# Send another event and hope that the parent's rule, which is
# disabled in the child, isn't triggered.
curl -s --data-urlencode 'event={"wants":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=&CHILD" | \
    python -mjson.tool

# Enable the parent's rule in the child. 
curl -s "$ENDPOINT/api/loc/rules/enable?location=&CHILD&id=momRule1"

# Send an event to the child and hope the parent's rule is triggered.
curl -s --data-urlencode 'event={"wants":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=&CHILD" | \
    python -mjson.tool


# Now let's try a fact in the parent.

# Add a rule to the parent.
cat<<EOF | curl -s -d "@-" "$ENDPOINT/api/loc/rules/add"
{"location":"$PARENT",
 "id":"momRule2",
 "rule": {"when":{"pattern":{"demands":"?x"}},
          "condition":{"pattern":{"have":"?x"}},
          "action":{"code":"console.log(\"get \" + x); \"get \" + x;"}}}
EOF

# Send an event to the child and hope the parent's rule isn't
# triggered because the condition fails.
curl -s --data-urlencode 'event={"demands":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=&CHILD" | \
    python -mjson.tool

# Write a parent fact.
curl -s -d 'fact={"have":"chips"}' "$ENDPOINT/api/loc/facts/add?location=$PARENT&id=chips"

# Query the parent for the fun of it.
curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=$PARENT" | \
  python -mjson.tool

# Query the child for the parent fact.
curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=&CHILD" | \
  python -mjson.tool
# Oops!  Not there.

# Query the child for the parent fact (and say we want to find
# inherited facts).
curl -s -d 'pattern={"have":"?x"}' "$ENDPOINT/api/loc/facts/search?location=&CHILD&inherited=true" | \
  python -mjson.tool
# Fact found.  Better.

# Send an event to the child and hope the parent's rule is triggered.
curl -s --data-urlencode 'event={"demands":"chips"}' \
   "$ENDPOINT/api/loc/events/ingest?location=&CHILD" | \
    python -mjson.tool


# Now try a fact in the child.

# Write a child fact.
curl -s -d 'fact={"have":"tacos"}' "$ENDPOINT/api/loc/facts/add?location=$PARENT&id=chips"

# Send an event to the child and hope the parent's rule is triggered.
curl -s --data-urlencode 'event={"demands":"tacos"}' \
   "$ENDPOINT/api/loc/events/ingest?location=&CHILD" | \
    python -mjson.tool

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


# Start an engine, run the FVTs, kill the engine.

# Default args to 'engine'.
args=("$@")
if [ ${#args} -eq 0 ]; then
    args=("${args[@]}" "engine")
fi

trap 'echo "Cleaning up"; kill $(jobs -p)' EXIT

echo "Building engine"
(cd rulesys && go build)

echo "Starting engine: rulesys ${args[@]}"
rulesys/rulesys "${args[@]}" > engine.log 2>&1 &

sleep 10
echo "Starting tests"
tools/fvt.sh
EXIT=$?
echo "Tests done: $EXIT"

exit $EXIT


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


set -e

TARGET=${1:-Manual.md}

cat<<EOF > contents.md
# Rules Core Manual

$(TZ=UTC date '+%FT%T%:z')

EOF

grep -P '^###* .*[^.]$' "$TARGET" | \
  sed 's/^## /1. /' | \
  sed 's/^### /  1. /' | \
  sed 's/^#### /    1. /' | \
  awk -F '\\. ' 'BEGIN { OFS=""} { link=$2; gsub(/ /, "-",link); print $1,". ", "[", $2, "](#", tolower(link), ")"}' | \
  tee -a contents.md

cp "$TARGET" Manual.backup

cat<<EOF > "$TARGET"
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

EOF
cat contents.md >> "$TARGET"
echo >> "$TARGET"
cat Manual.backup | awk '{if ($0 ~ /## Introduction/) body=1; if (body==1) print($0)}' >> "$TARGET"

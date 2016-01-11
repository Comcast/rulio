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


cat<<EOF | curl -d "@-" "http://localhost:8080/add"
{"account":"homer",
 "id":"1",
 "schedule":"10s",
 "url":"http://localhost:8080/test"
}
EOF

cat<<EOF | curl -d "@-" "http://localhost:8080/add"
{"account":"lisa",
 "id":"1",
 "schedule":"* * * * *",
 "url":"http://localhost:8080/test"
}
EOF


for ID in $(seq 10); do 
    cat<<EOF | curl -d "@-" "http://localhost:8080/add"
{"account":"bart",
 "id":"$ID",
 "schedule":"* * * * *",
 "url":"http://localhost:8080/test"
}
EOF
done

curl -s "http://localhost:8080/get?account=homer&id=1"
curl -s "http://localhost:8080/get?account=bart&id=1"
curl -s "http://localhost:8080/rem?account=lisa&id=1"

sleep 180
for ID in $(seq 10); do 
    curl -s "http://localhost:8080/rem?account=bart&id=$ID"
done


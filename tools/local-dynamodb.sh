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


# http://docs.aws.amazon.com/amazondynamodb/latest/developerguide/Tools.DynamoDBLocal.html

set -e

JAR=DynamoDBLocal.jar

if [ ! -f $JAR ]; then
    wget -nc http://dynamodb-local.s3-website-us-west-2.amazonaws.com/dynamodb_local_latest.tar.gz
    tar zxf dynamodb_local_latest.tar.gz  $JAR DynamoDBLocal_lib
fi

[ -f $JAR ] || (echo "No $JAR" >&2; exit 1)

exec java -Djava.library.path=./DynamoDBLocal_lib -jar $JAR -inMemory

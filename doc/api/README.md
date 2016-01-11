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


Generated API docs are at [`rules-core-http-api.md`](rules-core-http-api.md).

All of this API documentation is generated from files in this
directory.  The tool `rulesapi` (see below) hits a live
`rulessys` process to get example output, which is automatically
included in the generated documentation.

If you change the API, update `api.json`.

Then make sure you have an engine running at `http://localhost:9001/`.  

Then:

```Shell
go build -o rulesapi rulesapi.go
./rulesapi -format raml > rules-core-http-api.raml
./rulesapi -format markdown > rules-core-http-api.md
pandoc -o rules-core-http-api.html rules-core-http-api.md
```

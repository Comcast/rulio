#!/usr/bin/python

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


# An example action endpoint for rules with language="POST".  This
# example is NOT an action executor.  Instead, it's just an endpoint
# in the role of any external system that deals directly with JSON
# bodies.

# curl -d '{"likes":"tacos"}' http://localhost:6667/

from BaseHTTPServer import BaseHTTPRequestHandler,HTTPServer
# import json

PORT = 6667

def protest (response, message):
    response.send_response(200)
    response.send_header('Content-type','application/json')
    response.end_headers()
    response.wfile.write(message)

class handler(BaseHTTPRequestHandler):
    def do_GET(self):
        protest(self, "You should POST with json.\n")
        return
    def do_POST(self):
        try:
            content_len = int(self.headers.getheader('content-length'))
            body = self.rfile.read(content_len)
            print 'body ', body
            self.send_response(200)
            self.send_header('Content-type','application/json')
            self.end_headers()
            response = '{"Got":%s}' % (body)
            self.wfile.write(response)
        
        except Exception as broke:
            protest(self, str(broke))

try:
    server = HTTPServer(('', PORT), handler)
    print 'Started example action endpoint on port ' , PORT
    server.serve_forever()

except KeyboardInterrupt:
    print '^C received, shutting down example action endpoint on ', PORT
    server.socket.close()

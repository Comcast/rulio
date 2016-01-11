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


# An example external service.  Not an action executor.  Just an
# external service that a Javascript action can POST to.  This service
# tries to make JSON from what it hears.  Then writes that JSON to
# stdout and the client

# curl -d '{"likes":"tacos"}' http://localhost:6668/

from BaseHTTPServer import BaseHTTPRequestHandler,HTTPServer
from urlparse import urlparse, parse_qs
import json
import logging

PORT = 6668

logging.basicConfig(level=logging.DEBUG)

def protest (response, message):
    logging.warning(message)
    response.send_response(200)
    response.send_header('Content-type','application/json')
    response.end_headers()
    response.wfile.write(message)

def respond (out, js):
    logging.info(js)
    print 'data ', js
    out.send_response(200)
    out.send_header('Content-type','application/json')
    out.end_headers()
    out.wfile.write(js + "\n")

class handler(BaseHTTPRequestHandler):
    def do_GET(self):
        try:
            args = parse_qs(urlparse(self.path).query)
            respond(self, json.dumps(args))
        except Exception as broke:
            protest(self, str(broke))
    def do_POST(self):
        try:
            content_len = int(self.headers.getheader('content-length'))
            body = self.rfile.read(content_len)
            logging.info("body: " + body)
            if body[0] == '{':
                respond(self, body)
            else:
                args = parse_qs(body)
                respond(self, json.dumps(args))
        except Exception as broke:
            protest(self, str(broke))

try:
    server = HTTPServer(('', PORT), handler)
    print 'Started example action endpoint on port ' , PORT
    server.serve_forever()

except KeyboardInterrupt:
    print '^C received, shutting down example action endpoint on ', PORT
    server.socket.close()

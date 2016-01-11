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


# Numeric addition as a fact service. 

# curl -d '{"x":1,"y":2,"z":"?z"}' 'http://localhost:6666/facts/search'
# {"Found":[{"Bindingss":[{"?z":3}]}]}

from BaseHTTPServer import BaseHTTPRequestHandler,HTTPServer
import cgi # Better way now?
import json
import urllib2
import urllib

PORT = 6666

def protest (response, message):
    response.send_response(200)
    response.send_header('Content-type','text/plain')
    response.end_headers()
    response.wfile.write('{"Error":"%s"}' % (message))

class handler(BaseHTTPRequestHandler):
    def do_GET(self):
        protest(self, "You should POST with json.\n")
        return
    def do_POST(self):
        if not self.path == '/facts/search':
            protest(self, "Only can do /facts/search.\n")
            return

        try:
            content_length = int(self.headers['Content-Length'])
            js = self.rfile.read(content_length)

            # We want {"x":number,"y":number,"z":variable}. 
            m = json.loads(js)
    
            if 'x' not in m:
                protest(self, "Need x (constant: number).\n")
                return
            x = m["x"]
            print 'x ', x
    
            if 'y' not in m:
                protest(self, "Need y (constant: number).\n")
                return
            y = m["y"]
            print 'x ', x
    
    
            if 'z' not in m:
                protest(self, "Need z (variable).\n")
                return
            z = str(m["z"])
            print 'z ', z
    
            if not z.startswith("?"):
                protest(self, "z must be a variable.\n")
                return
    
            if len(z) < 2:
                protest(self, "Need an named variable for z.\n")
                return

            got = x + y

            self.send_response(200)
            self.send_header('Content-type','application/json')
            self.end_headers()
            response = '{"Found":[{"Bindingss":[{"%s":%f}]}]}' % (z, got)
            print 'response ', response
            self.wfile.write(response)
            
        except Exception as broke:
            protest(self, str(broke))

try:
    server = HTTPServer(('', PORT), handler)
    print 'Started addition FS on port ' , PORT
    server.serve_forever()

except KeyboardInterrupt:
    print '^C received, shutting down the weather FS on ', PORT
    server.socket.close()

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


# Little external fact service example

# Wrap part of IMDB as an FS.

# Pattern must specify a movie query string like "t=True%20Grit&y=1969".

# Pattern must specify at least one additional property from the set

legalProperties = {"Actors","Awards","Country","Director","Genre","Language",
                   "Metascore","Plot","Poster","Rated","Released","Response",
                   "Runtime","Title","Type","Writer","Year","imdbID","imdbRating",
                   "imdbVotes"}

# A more principled approach would allow the pattern to specify only a
# single additional property, but that decision is a separate
# discussion.

# Usage:
#
# curl -d '{"titleQuery":"t=True%20Grit&y=1969","Actors":"?actors","Runtime":"?runtime"}' 'http://localhost:6666/facts/search'
#

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
    response.wfile.write(message) # Should probably be JSON

def getMovie (query):
    uri = "http://www.omdbapi.com/?" + query
    # uri += urllib.quote_plus(locale)
    print "uri ", uri
    report = json.load(urllib2.urlopen(uri))
    print json.dumps(report, sort_keys=True, indent=2, separators=(',', ': '))
    return report

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

            m = json.loads(js)
    
            if 'titleQuery' not in m:
                protest(self, "Need titleQuery.\n")
                return
            titleQuery = m["titleQuery"]
            del m["titleQuery"]
    
            for p in m:
                if p not in legalProperties:
                    protest(self, "Illegal property " + p + ".\n")
                    return
                v = m[p]
                if not v.startswith("?"):
                    protest(self, "Value " + v + " must be a variable.\n")
                    return
                if len(v) < 2:
                    protest(self, "Need an named variable for " + v  + ".\n")
                    return

            o = getMovie(titleQuery)
            print o

            bindings = {}
            satisfied = True
            for p in m:
                print p, ": ", o[p], "\n"
                if p in o:
                    bindings[m[p]] = o[p]
                else:
                    satisfied = False
                    break

            if satisfied:
                js = json.dumps(bindings)
                response = '{"Found":[{"Bindingss":[%s]}]}' % (js)
            else:
                response = '{"Found":[{"Bindingss":[]}]}'

            self.send_response(200)
            self.send_header('Content-type','application/json')
            self.end_headers()
            print 'response ', response
            self.wfile.write(response)
            
        except Exception as broke:
            protest(self, str(broke))

try:
    server = HTTPServer(('', PORT), handler)
    print 'Started weather FS on port ' , PORT
    server.serve_forever()

except KeyboardInterrupt:
    print '^C received, shutting down the weather FS on ', PORT
    server.socket.close()

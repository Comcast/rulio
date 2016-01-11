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


# Little external FS (fact service) example

# Wrap Yahoo stock quotes as an FS.

# Pattern must specify a single ticker symbol "symbol".

# Pattern must specify at least one additional property from the set

legalProperties = {"bid", "ask", "change", "percentChange", "lastTradeSize"}

# curl 'http://download.finance.yahoo.com/d/quotes.csv?s=CMCSA&f=abc1p2k3&e=.csv'

# http://www.canbike.ca/information-technology/yahoo-finance-url-download-to-a-csv-file.html

# A more principled approach would allow the pattern to specify only a
# single additional property, but that decision is a separate
# discussion.

# Usage:
#
# curl -d '{"symbol":"CMCSA","bid":"?bid","ask":"?ask"}' 'http://localhost:6666/facts/search'
#

from BaseHTTPServer import BaseHTTPRequestHandler,HTTPServer
import cgi # Better way now?
import json
import urllib2
import urllib
import re

PORT = 6666

def protest (response, message):
    response.send_response(200)
    response.send_header('Content-type','text/plain')
    response.end_headers()
    response.wfile.write(message) # Should probably be JSON

def getQuote (symbol):
    uri = "http://download.finance.yahoo.com/d/quotes.csv?s=" + symbol + "&f=abc1p2k3&e=.csv"
    print "uri ", uri
    line = urllib2.urlopen(uri).read().strip()
    print "got ", line, "\n"
    line = re.sub(r'[%"\n]+', "", line)
    print "clean ", line, "\n"
    data = line.split(",")
    ns = map(float, data)
    q = {}
    q["bid"] = ns[0]
    q["ask"] = ns[1]
    q["change"] = ns[2]
    q["percentChange"] = ns[3]
    q["lastTradeSize"] = ns[4]
    return q

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
    
            if 'symbol' not in m:
                protest(self, "Need symbol.\n")
                return
            symbol = m["symbol"]
            del m["symbol"]
    
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

            q = getQuote(symbol)
            print q, "\n"

            bindings = {}
            satisfied = True
            for p in m:
                print p, ": ", q[p], "\n"
                if p in q:
                    bindings[m[p]] = q[p]
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
            print broke, "\n"
            protest(self, str(broke))

try:
    server = HTTPServer(('', PORT), handler)
    print 'Started weather FS on port ' , PORT
    server.serve_forever()

except KeyboardInterrupt:
    print '^C received, shutting down the weather FS on ', PORT
    server.socket.close()

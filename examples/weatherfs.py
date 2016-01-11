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

print "Please get an API key from http://openweathermap.org/appid"
APPID="2de143494c0b295cca9337e1e96b00e0"

# Wrap a weather service as an FS.  Only gives temperature.

# Note: You really don't want to use this thing.  If you want weather
# information, get it a better way (as code in a condition or as an
# event or ... ).

# Again: Just as an example external FS.

# curl -d '{"locale":"Austin,TX","temp":"?x"}' 'http://localhost:6666/facts/search'
# {"Found":[{"Bindingss":[{"?x":11.500000}]}]}

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

def getWeather (locale):
    uri = "http://api.openweathermap.org/data/2.5/weather?appid=" + APPID
    uri += "&units=metric&q="
    uri += urllib.quote_plus(locale)
    print "uri ", uri
    report = json.load(urllib2.urlopen(uri))
    print 'Got response'
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
    
            # We want {"locale":constant,"temp":variable}. 
            m = json.loads(js)
            print m, "\n"
    
            if 'locale' not in m:
                protest(self, "Need locale (constant: string).\n")
                return
            locale = m["locale"]
            print 'locale ', locale
    
            if 'temp' not in m:
                protest(self, "Need temp (variable).\n")
                return
            tempVar = str(m["temp"])
            print 'tempVar ', tempVar
    
            if not tempVar.startswith("?"):
                protest(self, "Temp must be a variable.\n")
                return
    
            if len(tempVar) < 2:
                protest(self, "Need an named variable for temp.\n")
                return

            w = getWeather(locale)
            temp = w['main']['temp']
            print 'temp ', temp

            self.send_response(200)
            self.send_header('Content-type','application/json')
            self.end_headers()
            response = '{"Found":[{"Bindingss":[{"%s":%f}]}]}' % (tempVar, temp)
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

{"Types":
 {"thermostat":
  {"Events":
   {"temp":
    {"Event":{"temp":{"Generate":{"Uniform":{"Min":70,"Max":90}}}},
     "Frequency":{"Generate":{"Uniform":{"Min":30,"Max":300}}}}}},

  "garageDoor":
  {"Events":
   {"changed":
    {"Event":{"state":{"Generate":{"Choose": {"open":0.60, "closed":0.40}}}},
     "Frequency":{"Generate":{"Uniform":{"Min":600,"Max":1200}}}}}},

  "doorLock":
  {"Events":
   {"changed":
    {"Event":{"state":{"Generate":{"Choose": {"locked":0.50, "unlocked":0.50}}}},
     "Frequency":{"Generate":{"Uniform":{"Min":600,"Max":1200}}}}}},

  "light":
  {"Events":{}},

  "elements":
  {"Description":"A stand-in for some Elements events",
   "Comment":"See https://github.comcast.com/XfinityRulesService/csv-rules-eel/tree/master/events",
   "Events":
   {"tbd":
    {"Event":
     {"topic": "/xhs/tps/ACCOUNT/zone/door",
      "content": {
          "eventId": {"Generate":{"ShortId":{}}},
          "eventName": null,
          "mediaType": "zone/door",
          "channel": "B",
          "instance": "113.0",
          "siteId": "ACCOUNT",
          "timestamp": {"Generate":{"TimestampMillis":{}}},
          "value": "true",
          "alert": null,
          "alertTitle": null,
          "cause": null,
          "metadata": {
	      "battery/voltage": "2600",
	      "wireless/nearFarRF": "-25/-25",
	      "sensor/temperature": "2500",
	      "wireless/nearFarSignal": "255/255"
          }
      },
      "sequence": {"Generate":{"TimestampMillis":{}}},
      "timestamp": {"Generate":{"TimestampMillis":{}}},
      "expires": 0},
     "Comments":["Need to generate timestamps.",
		 "Should generate some of the data.",
		 "Need to generate eventId."],
     "Frequency":{"Generate":{"Uniform":{"Min":60.0,"Max":600.0}},
		  "Comment":"Reportedly really about 200/day."}},
    
    "motion":
    {"Event":
     {"eventId": {"Generate":{"ShortId":{}}},
      "eventName": null,
      "mediaType": "zone/motion",
      "channel": "B",
      "instance": "114.0",
      "siteId": "102",
      "timestamp": {"Generate":{"TimestampMillis":{}}},
      "value": {"Generate":{"Choose": {"true":0.60, "false":0.40}}},
      "alert": null,
      "alertTitle": null,
      "cause": null,
      "metadata": {
	  "battery/voltage": "2900",
	  "wireless/nearFarRF": "-13/-16",
	  "sensor/temperature": "1940",
	  "wireless/nearFarSignal": "255/255"
      }},
     "Comments":["Need to generate timestamps.",
		 "Should generate some of the data.",
		 "Need to generate eventId."],
     "Frequency":{"Generate":{"Uniform":{"Min":60.0,"Max":600.0}},
		  "Comment":"Reportedly really about 200/day."}},

   "nest01":
    {"Event":
     {"topic":"/iot/ACCOUNT.A9F8AA/Molecule-Nest-Adapter/command/initiated",
      "content":{
	  "eventId": {"Generate":{"UUID":{}}},
	  "accountId":"ACCOUNT.A9F8AA",
	  "adapterId":"Molecule-Nest-Adapter",
	  "deviceId":"dTyEkPZuimRHPt0mbs3GsCHm9qt2LLQHagZwGEB67cxFYguhDWgAbQ",
	  "type":"command/initiated",
	  "value":{"Generate":{"Choose": {"home":0.60, "away":0.40}}},
	  "timestamp":1426630898428,
	  "name":"away",
	  "title":"Set Away/Home State",
	  "description":"uuid=a73d5fee-89b0-4491-842e-4fd0af61e7cf"
      },
      "sequence":1426630898428,
      "timestamp":1426630898430
     },
     "Frequency":{"Generate":{"Uniform":{"Min":60.0,"Max":600.0}}}}}},


  "alien":
  {"Description":"An alien in the house",
   "Events":
    {"signal":
     {"Event":{"signal":{"Generate":{"Zipf": {"S":2.0, "V":3.0, "Imax":10}}}},
      "Frequency":{"Generate":{"Normal":{"Min":5.0,"Mean":10.0,"Stddev":2.0}}}}}},

  "motionSensor":
   {"Events":
    {"changed":
     {"Event":{"state":{"Generate":{"Choose": {"motion":0.80, "noMotion":0.20}}}},
      "Frequency":{"Generate":{"Uniform":{"Min":60,"Max":90}}}}}}}}


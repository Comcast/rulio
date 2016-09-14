package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"rulio/core"
)

var elapsedMinThreshold = flag.Duration("mintime",
	10000*time.Nanosecond,
	"log if elasped less than this time")

var elapsedMaxThreshold = flag.Duration("maxtime",
	60*time.Second,
	"log if elasped greater than this time")

var statsInterval = flag.Duration("stats",
	1*time.Second,
	"stats loop interval")

var sizeThreshold = flag.Int("minsize",
	10,
	"panic if response less than this many bytes")

var maxDials = flag.Int("maxdials", 0, "max number of Dials to do")
var maxProblems = flag.Int("maxproblems", 0, "max number of problems to tolerate")
var maxIdleConnsPerHost = flag.Int("maxidleperhost", 2000, "MaxIdleConnsPerHost")
var verboseDialing = flag.Bool("verbosedialing", true, "log Dials")
var keepAlive = flag.Duration("keepalive", 600*time.Second, "KeepAlive")
var timeout = flag.Duration("timeout", 30*time.Second, "request timeout")
var sharedClient = flag.Bool("shared", true, "HTTP client is shared")

var panics = flag.Bool("panic", true, "panic on error?")

var engine = flag.String("engine",
	"http://localhost:8001",
	"URL for engine endpoint")

var templateFilename = flag.String("template",
	"template1.js",
	"Filename for rules template")

var deviceTypesFilename = flag.String("types",
	"devicetypes1.js",
	"Filename for device types")

var configFilename = flag.String("config",
	"",
	"Optional filename for configuration params")

var accounts = flag.Int("accounts", 10, "Number of accounts to simulate")

var speed = flag.Float64("speed",
	50.0,
	"Multiplier to make time run faster")

var durationSecs = flag.Int("duration",
	60,
	"Run for this many seconds")

var pending int32

var stats = make(chan time.Duration, 100)

func Stats(interval time.Duration) {
	ticker := time.NewTicker(interval)
	bufSize := 10000
	lats := NewLatencies(bufSize)
	i := 0
	for {
		select {
		case <-ticker.C:
			i++
			go func(copy Latencies) {
				n := copy.Count()
				stats, err := copy.Stats()
				if err != nil {
					log.Printf("error %v", err)
					return
				}
				// ToDo: Compute exact interval.
				hertz := float64(1000000000*n) / float64(interval)
				fmt.Printf("latencies,%d,%f,%s\n", i, hertz, stats.CSVDiv(1000000))
			}(*lats)
			lats = NewLatencies(bufSize)
			fmt.Printf("pending,%d,%d\n", i, atomic.LoadInt32(&pending))
		case elapsed := <-stats:
			lats.Add(int(elapsed.Nanoseconds()))
		}
	}
}

func Send(client *http.Client, name string, uri string, js []byte) error {
	log.Printf("send %s %s", name, js)
	buf := bytes.NewBuffer(js)
	then := time.Now()
	atomic.AddInt32(&pending, 1)

	req, err := http.NewRequest("POST", *engine+"/api/json", buf)
	if err != nil {
		return err
	}
	// resp, err := client.Post(*engine+"/api/json", "application/json", buf)
	req.Header = http.Header{"Content-Type": []string{"application/json"}}
	// req.Close = true
	resp, err := client.Do(req)

	log.Printf("post %s %s elapsed %d", name, js, time.Now().Sub(then).Nanoseconds())

	var bs []byte
	if err != nil {
		err = fmt.Errorf("warning post %v %s %s", err, *engine, js)
	} else {
		if resp.StatusCode != 200 {
			err = fmt.Errorf("warning statuscode %s %v %s %s", name, resp.Status, *engine, js)
		} else {
			var oops error
			bs, oops = ioutil.ReadAll(resp.Body)
			if err != nil {
				err = fmt.Errorf("warning readerr %s %v %s %s %v", name, resp.Status, *engine, js, oops)
			} else {
				log.Printf("got %s %s", name, bs)
				if len(bs) < *sizeThreshold {
					err = fmt.Errorf("warning toosmall %s %s %v %v", name, uri, len(bs), *sizeThreshold)
				}
			}
		}
	}

	if resp != nil {
		log.Printf("post %s %s closing", name, js)
		io.Copy(ioutil.Discard, resp.Body)
		if problem := resp.Body.Close(); problem != nil {
			log.Printf("post %s %s close error %v", name, js, problem)
			if err == nil {
				err = problem
			}
		}
	} else {
		log.Printf("warning post %s %s nil resp", name, js)
	}

	elapsed := time.Now().Sub(then)
	atomic.AddInt32(&pending, -1)
	stats <- elapsed
	log.Printf("elapsed %s %s %v", name, uri, elapsed)

	if elapsed < *elapsedMinThreshold {
		err = fmt.Errorf("warning toofast %s %s %v %v", name, uri, elapsed, *elapsedMinThreshold)
	}
	if *elapsedMaxThreshold < elapsed {
		err = fmt.Errorf("warning tooslowst %s %s %v %v", name, uri, elapsed, *elapsedMaxThreshold)
	}

	if err != nil {
		log.Printf("generror %v %s", err, bs)
		log.Print(err)
		problemCounter.inc()
		if *panics {
			panic(err)
		}
	}
	return err
}

type Counter struct {
	label string
	max   int
	n     int64
}

var dialCounter = Counter{label: "dials"}
var problemCounter = Counter{label: "problems"}

func (c *Counter) inc() int64 {
	n := atomic.LoadInt64(&c.n)
	n++
	atomic.StoreInt64(&c.n, n)
	if 0 < c.max && c.max <= int(n) {
		err := fmt.Sprintf("too many %s: %d", c.label, n)
		panic(err)
	}
	return n
}

type Dialer struct {
	net.Dialer
}

func (d *Dialer) Dial(network string, address string) (net.Conn, error) {
	n := dialCounter.inc()
	if *verboseDialing {
		log.Printf("Dialing %d %s %s", n, network, address)
	}
	return d.Dialer.Dial(network, address)
}

func NewDialer(d *net.Dialer) Dialer {
	return Dialer{*d}
}

var Client *http.Client

func makeClient() *http.Client {

	dialer := NewDialer(&net.Dialer{
		KeepAlive: *keepAlive,
	})

	log.Printf("MaxIdleConnsPerHost %d", *maxIdleConnsPerHost)

	transport := &http.Transport{
		Dial:                dialer.Dial,
		MaxIdleConnsPerHost: *maxIdleConnsPerHost,
	}

	client := &http.Client{Transport: transport}

	if *timeout != time.Duration(0) {
		client.Timeout = *timeout
		transport.ResponseHeaderTimeout = *timeout // Better than nothing.
	}

	return client
}

func initClient() {
	Client = makeClient()
}

func getClient() *http.Client {
	if *sharedClient {
		return Client
	}
	return makeClient()
}

func (a *Account) Start(deviceTypes *DeviceTypes, template *Template, config map[string]interface{}) error {
	log.Println(a.Pretty())

	rules, facts, err := a.Rules(template, config)
	if err != nil {
		return err
	}

	client := getClient()

	uri := "/loc/admin/clear"
	js := fmt.Sprintf(`{"location":"%s","uri":"%s"}`, a.Name, uri)
	Send(client, a.Name, uri, []byte(js))

	uri = "/loc/rules/add"
	for id, rule := range rules {
		req := make(map[string]interface{})
		req["location"] = a.Name
		req["uri"] = uri
		req["rule"] = rule
		req["id"] = id

		js, err := json.Marshal(&req)
		if err != nil {
			return err
		}

		Send(client, a.Name, uri, js)
	}

	uri = "/loc/facts/add"
	for id, fact := range facts {
		req := make(map[string]interface{})
		req["location"] = a.Name
		req["uri"] = uri
		req["fact"] = fact
		req["id"] = id

		js, err := json.Marshal(&req)
		if err != nil {
			return err
		}

		Send(client, a.Name, uri, js)
	}

	a.Emit(deviceTypes, func(req interface{}) error {
		js, err := json.Marshal(&req)
		if err == nil {
			Send(client, a.Name, "/api/loc/events/ingest", js)
		}
		return err
	})

	return nil
}

func main() {

	log.SetFlags(log.Lmicroseconds | log.Ldate)

	flag.Parse()
	Speed = *speed
	dialCounter.max = *maxDials
	problemCounter.max = *maxProblems

	initClient()

	core.UseCores(nil, true)

	go Stats(*statsInterval)

	var config map[string]interface{}
	if *configFilename != "" {
		js, err := getJS(*configFilename)
		if err != nil {
			panic(err)
		}
		if err = json.Unmarshal(js, &config); err != nil {
			panic(err)
		}
	}

	types, err := NewDeviceTypes(*deviceTypesFilename)
	if err != nil {
		panic(err)
	}

	if false {
		for typeName, deviceType := range types.Types {
			log.Println(typeName)
			device := typeName + "_1"
			log.Printf("%s %#v", device, deviceType)
			f := func(e interface{}) error {
				log.Printf("emit %v", e)
				return nil
			}
			deviceType.Emit(device, f)
		}
		time.Sleep(30 * time.Second)
	}

	log.Printf("Initializing %d accounts", *accounts)
	for i := 0; i < *accounts; i++ {
		go func(i int) {
			account, err := types.GenAccount(i)
			if err != nil {
				panic(err)
			}

			template, err := NewTemplate(*templateFilename)
			if err != nil {
				panic(err)
			}

			if err = account.Start(types, template, config); err != nil {
				panic(err)
			}
		}(i)
	}

	log.Printf("Sleeping for %v", *durationSecs)
	time.Sleep(time.Duration(*durationSecs) * time.Second)
	log.Print("Done")
}

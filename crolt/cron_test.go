// Copyright 2015 Comcast Cable Communications Management, LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// End Copyright

package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/boltdb/bolt"
)

func TestGenAId(t *testing.T) {
	if _, err := genAId("", ""); err == nil {
		t.Fatal("empty args not allowed")
	}

	if _, err := genAId("homer", ""); err == nil {
		t.Fatal("empty id not allowed")
	}

	if _, err := genAId("", "42"); err == nil {
		t.Fatal("empty account not allowed")
	}

	if aid, err := genAId("homer", "42"); err != nil {
		t.Fatal("legal args")
	} else {
		account, id := parseAId(aid)
		if account != "homer" {
			t.Fatal("messed up account")
		}
		if id != "42" {
			t.Fatal("messed up id")
		}
	}
}

func TestJitter(t *testing.T) {
	IgnorableDB = true
	c, err := NewDefaultCron(nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Jitter()
}

func TestPartition(t *testing.T) {
	IgnorableDB = true
	c, err := NewDefaultCron(nil)
	if err != nil {
		t.Fatal(err)
	}
	c.Partition("homer")
}

func DumpBucket(s *Cron, bucket string) {
	err := s.Scan(bucket, func(b, k, v string) (bool, error) {
		fmt.Printf("%s %s %s\n", b, k, v)
		return false, nil
	})
	if err != nil {
		panic(err)
	}
}

func TestCron(t *testing.T) {
	duration := 90 * time.Second

	db, err := bolt.Open("my.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		log.Printf("closing database")
		db.Close()
	}()

	cron, err := NewDefaultCron(db)
	if err != nil {
		log.Fatal(err)
	}
	cron.TTL = 10 * time.Second

	stop, problems, err := cron.WorkLoops()
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			problem := <-problems
			log.Printf("problem %s with %s",
				problem.Error.Error(),
				problem.Partition)
		}
	}()
	wait := sync.WaitGroup{}
	wait.Add(1)
	go func() {
		time.Sleep(duration)
		stop <- true
		wait.Done()
	}()

	requests := make(chan string, 100)

	// Use this function to verify we got what we wanted.
	go func() {
		for {
			body := <-requests
			log.Printf("test endpoint: body %s", body)
		}
	}()

	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("test endpoint: request URI %s", r.RequestURI)
		bs, err := ioutil.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		requests <- string(bs)
		fmt.Fprintf(w, "%s", bs)
	}))
	defer endpoint.Close()

	service := endpoint.URL
	log.Printf("test endpoint at %s", service)

	{
		job, err := NewJob("homer", "1", "* * * * *")
		if err != nil {
			t.Fatal(err)
		}
		job.URL = service
		job.RequestBody = "hello"

		if err := cron.Add(job); err != nil {
			t.Fatal(err)
		}
	}

	{
		job, err := NewJob("bart", "1", "5s")
		if err != nil {
			t.Fatal(err)
		}
		job.URL = service
		job.RequestBody = "world"

		if err := cron.Add(job); err != nil {
			t.Fatal(err)
		}
	}

	go func() {
		for {
			cron.DoBuckets(func(bucket string) error {
				DumpBucket(cron, bucket)
				return nil
			})
			time.Sleep(5 * time.Second)
		}
	}()

	wait.Wait()
}

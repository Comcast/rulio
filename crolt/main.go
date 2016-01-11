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
	"log"
	"net/http"

	"github.com/boltdb/bolt"
)

func main() {
	db, err := bolt.Open("my.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	cron, err := NewDefaultCron(db)
	if err != nil {
		log.Fatal(err)
	}

	_, problems, err := cron.WorkLoops()
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			problem := <-problems
			log.Printf("problem %s with %s",
				problem.Error.Error(),
				problem.Partition)
		}
	}()

	http.HandleFunc("/add", cron.AddHandler)
	http.HandleFunc("/rem", cron.DeleteHandler)
	http.HandleFunc("/get", cron.GetHandler)

	http.HandleFunc("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "hello\n")
	}))

	if err = http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}

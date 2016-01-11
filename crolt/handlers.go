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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

func protest(w http.ResponseWriter, fm string, args ...interface{}) {
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, fm+"\n", args...)
}

func (c *Cron) AddHandler(w http.ResponseWriter, r *http.Request) {
	js, err := ioutil.ReadAll(r.Body)
	if err != nil {
		protest(w, "error reading request: %v", err)
		return
	}
	// ToDo: Close?

	var job Job
	if err := json.Unmarshal(js, &job); err != nil {
		protest(w, "error parsing body: %v", err)
		return
	}

	if err := c.Add(&job); err != nil {
		protest(w, "error creating job: %v", err)
		return
	}

	if js, err = json.Marshal(&job); err != nil {
		protest(w, "error serializing job: %v", err)
		return
	}

	fmt.Fprintf(w, `{"job":%s}`, js)
}

func (c *Cron) DeleteHandler(w http.ResponseWriter, r *http.Request) {
	account := r.FormValue("account")
	if account == "" {
		protest(w, "need an account")
		return
	}
	id := r.FormValue("id")
	if id == "" {
		protest(w, "need an id")
		return
	}
	_, err := ioutil.ReadAll(r.Body)
	if err != nil {
		protest(w, "error reading request: %v", err)
		return
	}

	if err = c.Delete(account, id); err != nil {
		protest(w, "error deleting job: %v", err)
	}

	fmt.Fprintf(w, `{"status":"ok"}`)
}

func (c *Cron) GetHandler(w http.ResponseWriter, r *http.Request) {
	account := r.FormValue("account")
	if account == "" {
		protest(w, "need an account")
		return
	}
	id := r.FormValue("id")
	if id == "" {
		protest(w, "need an id")
		return
	}
	_, err := ioutil.ReadAll(r.Body)
	if err != nil {
		protest(w, "error reading request: %v", err)
		return
	}

	job, err := c.Get(account, id)
	if err != nil && err != NotFound {
		protest(w, "error getting job: %v", err)
	}
	if err == NotFound {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "not found\n")
		return
	}

	js, err := json.Marshal(&job)
	if err != nil {
		protest(w, "error serializing job: %v", err)
		return
	}

	fmt.Fprintf(w, `{"job":%s}`, js)
}

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
	"bytes"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
)

type Work struct {
	Error        string `json:"error"`
	Code         int    `json:"code"`
	ResponseBody string `json:"responseBody,omitempty"`
}

func (j *Job) Request() (*http.Request, error) {
	u, err := url.Parse(j.URL)
	if err != nil {
		return nil, err
	}
	body := bytes.NewBuffer([]byte(j.RequestBody))
	req := &http.Request{
		Method:        j.Method,
		URL:           u,
		Body:          ioutil.NopCloser(body),
		ContentLength: int64(len(j.RequestBody)),
		Header:        j.Header,
	}
	return req, nil
}

func (j *Job) Do(client *http.Client) (w *Work) {
	log.Printf("Job.Do %s", j.aid)

	if client == nil {
		client = http.DefaultClient
	}

	w = &Work{}
	req, err := j.Request()
	if err != nil {
		log.Printf("Job.Do %s error %v", j.aid, err)
		w.Error = err.Error()
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Job.Do %s error %v", j.aid, err)
		w.Error = err.Error()
		return
	}
	w.Code = resp.StatusCode
	if resp.Body != nil {
		got, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Job.Do %s error %v", j.aid, err)
			// ToDo: Use another error field.
			w.Error = err.Error()
		}
		w.ResponseBody = string(got)
	}

	log.Printf("Job.Do %s done", j.aid)
	return
}

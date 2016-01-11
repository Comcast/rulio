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
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/boltdb/bolt"
	"github.com/gorhill/cronexpr"
)

var DefaultPartitions = 4
var DefaultMaxJitter = 5 * time.Second
var DefaultTTL = 60 * 60 * time.Second

var NotFound = errors.New("not found")
var Exists = errors.New("job exists")

type Cron struct {
	DB              *bolt.DB
	Partitions      int
	MaxJitter       time.Duration
	PollingInterval time.Duration
	TTL             time.Duration
}

func NewDefaultCron(db *bolt.DB) (*Cron, error) {
	return NewCron(db, DefaultPartitions, DefaultMaxJitter, DefaultTTL)
}

func (c *Cron) DoBuckets(f func(bucket string) error) error {
	for i := 0; i < c.Partitions; i++ {
		for _, base := range []string{"jobs", "time"} {
			bucket := fmt.Sprintf("%s%d", base, i)
			if err := f(bucket); err != nil {
				return err
			}
		}
	}
	return nil
}

var IgnorableDB = false

func NewCron(db *bolt.DB, partitions int, maxJitter time.Duration, ttl time.Duration) (*Cron, error) {
	c := &Cron{
		DB:              db,
		Partitions:      partitions,
		MaxJitter:       maxJitter,
		PollingInterval: time.Second,
		TTL:             ttl,
	}

	if db == nil && !IgnorableDB {
		return nil, errors.New("no DB")
	}

	if db != nil {
		err := db.Update(func(tx *bolt.Tx) error {
			c.DoBuckets(func(bucket string) error {
				if _, err := tx.CreateBucketIfNotExists([]byte(bucket)); err != nil {
					return err
				}
				return nil
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Cron) Jitter() time.Duration {
	max := float64(c.MaxJitter)
	d := time.Duration(rand.Float64()*max - max/2)
	log.Printf("Cron.Jitter %v", d)
	return d
}

func (c *Cron) Partition(account string) string {
	h := fnv.New64a().Sum([]byte(account))
	return strconv.Itoa(int((h[0] ^ h[1] ^ h[2] ^ h[3]) % byte(c.Partitions)))
}

type Job struct {
	Account    string `json:"account"`
	Id         string `json:"id"`
	Expression string `json:"schedule"`

	Method      string      `json:"method,omitempty"`
	URL         string      `json:"url"`
	RequestBody string      `json:"requestBody,omitempty"`
	Header      http.Header `json:"header,omitempty"`

	Once  bool   `json:"once,omitempty"`
	TId   string `json:"tid,omitempty"`
	Work  *Work  `json:"work,omitempty"`
	Evict bool   `json:"evict,omitempty"`

	at  time.Time
	aid string
	// Maybe cache part(ition), too.
}

func NewJob(account string, id string, expr string) (*Job, error) {
	job := &Job{
		Account:    account,
		Id:         id,
		Expression: expr,
	}
	if err := job.init(); err != nil {
		return nil, err
	}
	return job, nil
}

const Separator = ","

func genAId(account, id string) (string, error) {
	if account == "" {
		return "", fmt.Errorf("need a non-zero account")
	}
	if id == "" {
		return "", fmt.Errorf("need a non-zero id")
	}
	if strings.Contains(account, Separator) {
		return "", fmt.Errorf("no '%s' allowed in '%s'", Separator, account)
	}
	if strings.Contains(id, Separator) {
		return "", fmt.Errorf("no '%s' allowed in '%s'", Separator, id)
	}
	return fmt.Sprintf("%s%s%s", account, Separator, id), nil
}

func parseAId(aid string) (account string, id string) {
	parts := strings.SplitN(aid, Separator, 2)
	account = parts[0]
	id = parts[1]
	return
}

func (j *Job) init() error {
	var err error
	j.aid, err = genAId(j.Account, j.Id)
	return err
}

var Zero = time.Time{}

func (c *Cron) set(j *Job) error {

	if j.Evict {
		j.at = time.Now().UTC().Add(c.TTL)
		return nil
	}

	if d, err := time.ParseDuration(j.Expression); err == nil {
		j.at = time.Now().UTC().Add(d)
		j.Once = true
		return nil
	}

	if t, err := time.Parse(j.Expression, time.RFC3339); err == nil {
		j.at = t
		j.Once = true
		return nil
	}

	schedule, err := cronexpr.Parse(j.Expression)
	if err != nil {
		return err
	}

	j.at = schedule.Next(time.Now().UTC()).Add(c.Jitter())

	return nil
}

func (s *Cron) Add(j *Job) error {
	if err := j.init(); err != nil {
		log.Printf("Cron.Add init error: %v", err)
		return err
	}

	{
		part := s.Partition(j.Account)
		jobs := "jobs" + part
		exists := false
		err := s.DB.View(func(tx *bolt.Tx) error {
			js := tx.Bucket([]byte(jobs)).Get([]byte(j.aid))
			if 0 < len(js) {
				exists = true
			}
			return nil
		})
		if err != nil {
			return err
		}
		if exists {
			return Exists
		}
	}

	if err := s.set(j); err != nil {
		log.Printf("Cron.Add set error: %v", err)
		return err
	}

	f, err := s.update(j)
	if err != nil {
		log.Printf("Cron.Add update error: %v", err)
		return err
	}

	return s.DB.Update(f)
}

func (s *Cron) update(j *Job) (func(*bolt.Tx) error, error) {
	part := s.Partition(j.Account)
	jobs := "jobs" + part
	tim := "time" + part
	oldTid := j.TId

	next := j.at
	ts := next.Format(time.RFC3339Nano)
	later := next.Sub(time.Now().UTC())
	log.Printf("Cron.update %s to %s (%v) evict=%v", j.aid, ts, later, j.Evict)

	tid := fmt.Sprintf("%s,%s", ts, j.aid)
	j.TId = tid

	js, err := json.Marshal(j)
	if err != nil {
		return nil, err
	}

	return func(tx *bolt.Tx) error {
		if err := tx.Bucket([]byte(jobs)).Put([]byte(j.aid), js); err != nil {
			return err
		}
		if oldTid != "" {
			log.Printf("Cron.update resetting %s in %s", oldTid, tim)
			if err := tx.Bucket([]byte(tim)).Delete([]byte(oldTid)); err != nil {
				return err
			}
		}
		log.Printf("Cron.update updating %s in %s", tid, tim)
		return tx.Bucket([]byte(tim)).Put([]byte(tid), js)
	}, nil
}

func (s *Cron) Delete(account, id string) error {
	f, err := s.delete(account, id)
	if err != nil {
		return err
	}
	return s.DB.Update(f)
}

func (s *Cron) Get(account, id string) (*Job, error) {
	log.Printf("Cron.get %s %s", account, id)
	aid, err := genAId(account, id)
	if err != nil {
		return nil, err
	}
	part := s.Partition(account)
	jobs := "jobs" + part
	var js []byte
	err = s.DB.View(func(tx *bolt.Tx) error {
		js = tx.Bucket([]byte(jobs)).Get([]byte(aid))
		return nil
	})
	if err != nil {
		return nil, err
	}
	if js == nil {
		return nil, NotFound
	}
	var j Job
	if err := json.Unmarshal(js, &j); err != nil {
		return nil, err
	}

	return &j, nil
}

func (s *Cron) delete(account, id string) (func(*bolt.Tx) error, error) {
	log.Printf("Cron.delete %s %s", account, id)
	aid, err := genAId(account, id)
	if err != nil {
		return nil, err
	}
	part := s.Partition(account)
	jobs := "jobs" + part
	tim := "time" + part
	return func(tx *bolt.Tx) error {
		js := tx.Bucket([]byte(jobs)).Get([]byte(aid))
		if js == nil {
			log.Printf("Cron.delete warning: %s has no job in %s", aid, jobs)
			return nil
		}
		var j Job
		if err := json.Unmarshal(js, &j); err != nil {
			return err
		}
		tid := j.TId
		log.Printf("Cron.Delete %s is at %s", aid, tid)
		if err := tx.Bucket([]byte(tim)).Delete([]byte(tid)); err != nil {
			return err
		}
		if err := tx.Bucket([]byte(jobs)).Delete([]byte(aid)); err != nil {
			return err
		}
		return nil
	}, nil
}

func (s *Cron) DeleteAccount(account string) error {
	jobs := "jobs" + s.Partition(account)
	return s.DB.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(jobs)).Cursor()

		prefix := []byte(account + ",")
		for k, _ := c.Seek(prefix); bytes.HasPrefix(k, prefix); k, _ = c.Next() {
			_, id := parseAId(string(k))
			f, err := s.delete(account, id)
			if err != nil {
				return err
			}
			if err := f(tx); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Cron) work(part string) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte("time" + part)).Cursor()

		min := []byte("")
		max := []byte(time.Now().UTC().Format(time.RFC3339Nano))
		limit := 10

		for k, v := c.Seek(min); k != nil && bytes.Compare(k, max) <= 0; k, v = c.Next() {
			log.Printf("Cron.work %s %s %s", part, string(k), string(v))
			if limit <= 0 {
				break
			}
			limit--

			var job Job
			err := json.Unmarshal(v, &job)
			if err != nil {
				log.Printf("Cron.work error: %v k=%s json=%s", err, k, v)
				continue
				// return err
			}
			if err = job.init(); err != nil {
				return err
			}
			log.Printf("Cron.work job %s", job.TId)

			if job.Evict {
				log.Printf("Cron.work evicting %s", job.aid)
				f, err := s.delete(job.Account, job.Id)
				if err != nil {
					return err
				}
				return f(tx)
			}

			if job.Once {
				job.Evict = true
			}

			if err = s.set(&job); err != nil {
				return err
			}

			// Do the work after setting up the next time.
			// That way the work can override that next
			// time if the work wants.
			job.Work = job.Do(nil)

			f, err := s.update(&job)
			if err != nil {
				return err
			}
			if err = f(tx); err != nil {
				return err
			}
		}

		return nil
	}
}

func (s *Cron) Scan(bucket string, f func(b, k, v string) (bool, error)) error {
	return s.DB.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			stop, err := f(bucket, string(k), string(v))
			if err != nil {
				return err
			}
			if stop {
				break
			}
		}
		return nil
	})
}

type Problem struct {
	Error     error
	Partition string
}

func (s *Cron) WorkLoops() (chan bool, chan Problem, error) {
	problems := make(chan Problem)
	stop := make(chan bool)
	for i := 0; i < s.Partitions; i++ {
		go func(part string) {
			for {
				if err := s.DB.Update(s.work(part)); err != nil {
					log.Printf("Cron.WorkLoops error %v on %s", err, part)
					problems <- Problem{err, part}
				}
				// ToDo: report timeout.
				timer := time.NewTimer(s.PollingInterval)
				select {
				case <-stop:
					log.Printf("Cron.WorkLoops stopping work on %s", part)
					break
				case <-timer.C:
				}
			}
		}(strconv.Itoa(i))
	}
	return stop, problems, nil
}

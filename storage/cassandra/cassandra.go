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

package cassandra

// Implementation of Storage using cql.
// Note: cql session is synchronized so no need to protect by mutex.

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gocql/gocql"

	. "github.com/Comcast/rulio/core"
)

var cassStoreMutex sync.Mutex
var cassStore *CassStorage

// CassStorage implements Storage using Cassandra.
//
// This name stutters because it's convenient to dot-import core,
// which defines 'Storage'.
type CassStorage struct {
	cluster *gocql.ClusterConfig
	session *gocql.Session
}

// CassandraDBConfig does what'd you'd think.
//
// This name stutters because it's convenient to dot-import core,
// which defines 'Storage'.
type CassandraDBConfig struct {
	nodes    []string
	username string
	password string
	keyspace string
}

// ParseConfig generates a CassandraDBConfig from a string.
// Configuration is parsed from a string of the follow format:
// host:port,host:port;username;password;keyspace
func ParseConfig(config string) (*CassandraDBConfig, error) {
	var ns []string
	user := ""
	pass := ""
	ks := ""

	parts := strings.SplitN(config, ";", 4)

	if 0 < len(parts) && parts[0] != "" {
		hostPorts := strings.Split(parts[0], ",")
		ns = make([]string, 0, len(hostPorts))
		for _, hp := range hostPorts {
			ns = append(ns, hp)
		}

	}

	if 1 < len(parts) && parts[1] != "" {
		user = parts[1]
	}

	if 2 < len(parts) && parts[2] != "" {
		pass = parts[2]
	}

	if 3 < len(parts) && parts[3] != "" {
		ks = parts[3]
	}

	c := CassandraDBConfig{
		nodes:    ns,
		username: user,
		password: pass,
		keyspace: ks,
	}

	return &c, nil
}

// NewStorage creates new Storage implementation based on Cassandra.
//
// The given nodes should have the form ADDRESS:PORT, where the PORT
// is the CQL port (normally 9042, I think).
func NewStorage(ctx *Context, config CassandraDBConfig) (*CassStorage, error) {
	cassStoreMutex.Lock()
	defer cassStoreMutex.Unlock()

	if nil == cassStore {
		s := CassStorage{}
		err := s.init(ctx, config)
		if nil != err {
			return nil, err
		}
		cassStore = &s
	}

	return cassStore, nil
}

func (s *CassStorage) init(ctx *Context, config CassandraDBConfig) error {
	Log(INFO, ctx, "CassStorage.init", "config", config)

	// ToDo: Expose more/better Cass connection parameters
	s.cluster = gocql.NewCluster(config.nodes...)
	s.cluster.Consistency = gocql.Quorum
	if config.username != "" {
		s.cluster.Authenticator = gocql.PasswordAuthenticator{
			Username: config.username,
			Password: config.password,
		}
	}
	// How to create Cassandra data structures.

	// ToDo: Probably move so that things like replication can be
	// configured outside of this code.
	var (
		CassandraKeyspace = `rules`

		CassandraKeyspaceDDL = `
		CREATE KEYSPACE %v WITH REPLICATION = { 'class' : 'SimpleStrategy', 'replication_factor' : 1 }
		`

		CassandraStateDDL = `
		CREATE TABLE locstate (
			loc text,
			data map<text, text>,
			last_modified timestamp,
			PRIMARY KEY (loc)
		)
		`
	)

	if config.keyspace != "" {
		CassandraKeyspace = config.keyspace
	}
	//create keyspace if not exists
	session, err := s.cluster.CreateSession()
	if nil != err {
		return err
	} else {
		defer session.Close()

		q := fmt.Sprintf(CassandraKeyspaceDDL, CassandraKeyspace)
		err = session.Query(q).Exec()
		if nil != err && 0 > strings.Index(err.Error(), "existing keyspace") {
			Log(CRIT, ctx, "CassStorage.init", "ddl", q, "error", err)
		}
	}

	//switch to session with created keyspace
	s.cluster.Keyspace = CassandraKeyspace
	session, err = s.cluster.CreateSession()
	if nil != err {
		return err
	}

	//create tables
	err = session.Query(CassandraStateDDL).Exec()
	if nil != err && 0 > strings.Index(err.Error(), "existing column family") {
		Log(CRIT, ctx, "CassStorage.init", "ddl", CassandraStateDDL, "error", err)
	}

	s.session = session

	return nil
}

func (s *CassStorage) Load(ctx *Context, loc string) ([]Pair, error) {
	Log(INFO, ctx, "CassStorage.Load", "location", loc)

	iter := s.session.Query(`SELECT data FROM locstate WHERE loc = ?`, loc).Iter()
	acc := make([]Pair, 0, 1024)

	var pairs map[string]string

	if iter.Scan(&pairs) {
		for k, v := range pairs {
			d := Pair{[]byte(k), []byte(v)}
			Log(DEBUG, ctx, "CassStorage.Load", "pair", d)
			acc = append(acc, d)
		}
	}

	return acc, nil
}

func (s *CassStorage) Add(ctx *Context, loc string, m *Pair) error {
	Log(INFO, ctx, "CassStorage.Add", "location", loc, "pair", m.String())

	err := s.session.Query(`UPDATE locstate SET data[?] = ?, last_modified = ? where loc = ?`, string(m.K), string(m.V), NowMicros(), loc).Exec()
	// ToDo: Retry on error.

	return err
}

func (s *CassStorage) Remove(ctx *Context, loc string, k []byte) (int64, error) {
	Log(INFO, ctx, "CassStorage.Remove", "location", loc, "id", string(k))

	err := s.session.Query(`DELETE data[?] FROM locstate where loc = ?`, string(k), loc).Exec()
	// ToDo: Retry on error.

	return -1, err
}

func (s *CassStorage) Clear(ctx *Context, loc string) (int64, error) {
	Log(INFO, ctx, "CassStorage.Clear", "location", loc)

	err := s.session.Query(`DELETE FROM locstate where loc = ?`, loc).Exec()
	// ToDo: Retry on error.

	return -1, err
}

func (s *CassStorage) Delete(ctx *Context, loc string) error {
	Log(INFO, ctx, "CassStorage.Delete", "location", loc)
	_, err := s.Clear(ctx, loc)
	return err
}

func (s *CassStorage) GetStats(ctx *Context, loc string) (StorageStats, error) {
	Log(INFO, ctx, "CassStorage.GetStats", "location", loc)
	mss := StorageStats{}

	iter := s.session.Query(`SELECT data, last_modified FROM locstate WHERE loc = ?`, loc).Iter()

	var pairs map[string]string
	var ts int64
	if iter.Scan(&pairs, &ts) {
		mss.DateOfLastRecord = GetTimestamp(ts)
		mss.NumRecords = len(pairs)
	}

	return mss, nil
}

func (s *CassStorage) Close(ctx *Context) error {
	return nil
}

func (s *CassStorage) Health(ctx *Context) error {
	Log(INFO, ctx, "CassStorage.Health")
	// I don't think we don't want to attempt to check the health
	// of the cluster.  Instead, just our access.
	session, err := s.cluster.CreateSession()
	if err == nil {
		loc := "anywhere"
		i := session.Query(`SELECT data FROM locstate WHERE loc = ?`,
			loc).Iter()
		err = i.Close()
		session.Close()
	}
	if err != nil {
		Log(ERROR, ctx, "CassStorage.Health", "error", err)
	}
	return err
}

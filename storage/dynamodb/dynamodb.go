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

package dynamodb

// This code represents location state in DynamoDB as a large item.
// Each fact is an "attribute" of the item.  This representation is
// poor if you want to use the (beta) DynamoDB streaming API, which
// streams entire items, not item mutations.
//
// Consider the alternate representation ("skinny") using a hash key
// for the location and range keys for the facts.  That represention
// would work reasonably well with the (beta) AWS DynamoDB streaming
// API.

// ToDo: Switch to AWS SDK

// go test -run=Dynamo
//
// This code will use AWS credentials in the standard environment
// variables:
//
//  export AWS_ACCESS_KEY=...
//  export AWS_SECRET_ACCESS_KEY=...
//
// ToDo: Encrypt those and use a pass phrase.
//
// The AWS region and table name is specified via
// 'conf.MutationStoreConfig'.
//
//   conf.StorageConfig = "us-west-1:rules:true"
//
// That boolean indicates consistent ops.
//
// You tell the system to use DynamoDB with 'conf.MutationStore':
//
//  conf.Storage = "dynamodb"
//
// This code will create the necessary table(s) if necessary.
//
// See ../tools/local-dynamodb, which can get and run a local, mock
// DynamoDB.  If you use "local" as the region, then the code will
// talk to a local DynamoDB.
//
// github.com/crowdmob/goamz/aws doesn't implement the UpdateTable
// API, which we'd like to have in order to raise a lower provisioned
// throughput.
//
// Will need to think on how to manage provisioned throughput.  Might
// need to resort to multiple tables to get enough throughput if AWS
// won't give us what we need for one table.
//
// Probably not worth doing snappy compression and then base64 encoding.
//
// Consider a BatchGetItems during rules engine start-up (if we don't
// just load locations lazily, which might be the better option).
//

// Utilities:
//
//   aws dynamodb scan --table-name rfds --endpoint-url http://...
//   aws dynamodb update-item --table-name rfds --endpoint-url http://... --key '{"id":{"S":"here"}}' --attribute-updates '{"wants":{"Value":{"S":"tacos"}}}'
//   aws dynamodb get-item --table-name rfds --endpoint-url http://... --key '{"id":{"S":"here"}}'
//   aws dynamodb delete-item --table-name rfds --endpoint-url http://... --key '{"id":{"S":"here"}}'

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/Comcast/rulio/core"

	"github.com/AdRoll/goamz/aws"
	"github.com/AdRoll/goamz/dynamodb" // See MaxIdleConnsPerHost below
)

// CheckLastUpdated enables the experimental optimistic locking (sort
// of) of locations in DynamoDB.
//
// The idea: The last updated timestamp, if any, is read from DynamoDB
// at location load time.  We remember that timestamp (in
// 'Location.lastUpdated', which is accessed via 'Location.Updated()'
// and 'Location.Update()'.  When we attempt to change the location's
// state, we verify that the timestamp hasn't changed in DynamoDB.  If
// it has, we return a 'ConcurrentStateChange' error.  Otherwise, we
// atomically update the timestamp with a new one.
//
// ToDo: This code needs test cases badly.
var CheckLastUpdated = true

var (
	DefaultRegion     = "us-west-1"
	DefaultTableName  = "rules"
	DefaultConsistent = false
)

// Instrumented Dialer.
type Dialer struct {
	net.Dialer
}

func (d *Dialer) Dial(network string, address string) (net.Conn, error) {
	// log.Printf("dialing,%s,%s,%s", NowString(), network, address)
	return d.Dialer.Dial(network, address)
}

func NewDialer(d *net.Dialer) Dialer {
	return Dialer{*d}
}

// init sets up http.DefaultClient to have a large
// MaxIdleConnsPerHost, which we almost always want.  In particular,
// we want it with crowdmob/amz/dynamodb, which uses
// http.DefaultClient.
func init() {
	dialer := NewDialer(&net.Dialer{
		Timeout:   30 * time.Second, // ToDo: Expose
		KeepAlive: 30 * time.Second, // ToDo: Expose
	})

	http.DefaultTransport = &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		Dial:                dialer.Dial,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 1000, // ToDo: Expose
	}
}

// sysDB serves as an optional cache for DynamoDBStorages.
//
// If 'caching' is true, 'GetStorage' will either populate (if nil) or
// return sysDB.
var sysDB struct {
	sync.Mutex
	caching bool
	storage *DynamoDBStorage
}

// ConcurrentStateChange is a non-fatal error resulting from two
// concurrent attempts to modify a location's state.
//
// This error can only occur if CheckLastUpdated is true.
//
// Not currently used!
var ConcurrentStateChange = &Condition{"concurrent state change", "nonfatal"}

// LastUpdatedKey is the attribute name for a timestamp (augmented
// with an IP address maybe) of the last state update.
//
// ToDo: Create a space for internal ids.
var LastUpdatedKey = "LAST_UPDATED"

// DynamoDBStorage implements Storage using DynamoDB.  Duh.
//
// This name stutters because it's convenient to dot-import core,
// which defines 'Storage'.
type DynamoDBStorage struct {
	server *dynamodb.Server
	// We keep a pointer to a table here as an optimization.
	tableName string
	table     *dynamodb.Table

	// Consistent determines we we call GetItemConsistent or just GetItem.
	//
	// Note that DynamoDBStorage isn't synchronized, so beware.
	//
	// ToDo: Ideally we move this property elsewhere, but changing
	// the Storage interface just for this capabitity is
	// questionable.  Perhaps generalize Storage to support
	// arbitrary properties (including transaction id)?  Or
	// perhaps this property is part of a DynamoDBConfig.
	Consistent bool
}

func getDynamoServer(ctx *Context, region string) (*dynamodb.Server, error) {

	Log(INFO, ctx, "getDynamoServer", "region", region)

	// If we don't have real access keys, just try local.  See
	// ../local-dynamodb/.  Otherwise go for real DynamoDB.  ToDo:
	// insist on using the real DynamoDB.

	// ToDo: Combine with router/aws.go FindAWS()

	if region == "local" {
		r := aws.Region{DynamoDBEndpoint: "http://127.0.0.1:8000"}
		auth := aws.Auth{AccessKey: "DUMMY_KEY", SecretKey: "DUMMY_SECRET"}
		return dynamodb.New(auth, r), nil
	} else if strings.HasPrefix(region, "http:") {
		r := aws.Region{DynamoDBEndpoint: region}
		auth, err := aws.GetAuth("", "", "", time.Now().Add(100000*time.Hour))
		if err != nil {
			Log(INFO, ctx, "router.FindAWS", "warning", err)
			return nil, err
		}
		return dynamodb.New(auth, r), nil
	} else {
		auth, err := aws.EnvAuth()
		if err != nil {
			Log(INFO, ctx, "getDynamoServer", "warning", err, "when", "aws.EnvAuth")
			// return nil, nil, err
			// ToDo: Fix 100000 ...
			auth, err = aws.GetAuth("", "", "", time.Now().Add(100000*time.Hour))
			if err != nil {
				Log(INFO, ctx, "router.FindAWS", "warning", err)
				return nil, err
			}
		}
		r, found := aws.Regions[region]
		if !found {
			err = fmt.Errorf("Bad region name '%s'", region)
			Log(INFO, ctx, "getDynamoServer", "error", err)
			return nil, err
		}

		return dynamodb.New(auth, r), nil
	}
}

// DynamoDBConfig does what'd you'd think.
//
// This name stutters because it's convenient to dot-import core,
// which defines 'Storage'.
type DynamoDBConfig struct {
	Region    string
	TableName string
	// Consistent determines GetItemConsistent(cy).
	Consistent bool
}

// ParseConfig generates a DynamoDBConfig from a string.
//
// Input should look like region[:tableName[:(true|false)]], where the
// boolean indicates whether to do consistent reads.  Defaults: the
// vars DefaultRegion, DefaultTableName, DefaultConsistent.
func ParseConfig(config string) (*DynamoDBConfig, error) {
	region := DefaultRegion
	name := DefaultTableName
	con := DefaultConsistent

	parts := strings.SplitN(config, ":", 3)

	if 0 < len(parts) && parts[0] != "" {
		region = parts[0]
	}

	if 1 < len(parts) && parts[1] != "" {
		name = parts[1]
	}

	if 2 < len(parts) && parts[2] != "" {
		consistent := parts[2]
		if consistent != "" {
			switch consistent {
			case "true":
				con = true
			case "false":
				con = false
			default:
				return nil, fmt.Errorf("bad consistency in '%s'", config)
			}
		}
	}

	c := DynamoDBConfig{
		Region:     region,
		TableName:  name,
		Consistent: con,
	}

	return &c, nil
}

func NewStorage(ctx *Context, config DynamoDBConfig) (*DynamoDBStorage, error) {
	Log(INFO, ctx, "NewStorage", "table", config.TableName, "region", config.Region)

	// "TableName must be at least 3 characters long and at most
	// 255 characters long"

	// Let's check that right now.

	if len(config.TableName) < 3 || 255 < len(config.TableName) {
		return nil, errors.New("table name must be at least 3 characters long and at most 255 characters long")
	}

	server, err := getDynamoServer(ctx, config.Region)
	if err != nil {
		return nil, err
	}
	s := DynamoDBStorage{server: server,
		tableName: config.TableName,
	}
	s.Consistent = config.Consistent
	err = s.init(ctx, config.TableName)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetStorage will return a DynamoDBStorage, creating one if necessary.
func GetStorage(ctx *Context, config DynamoDBConfig) (*DynamoDBStorage, error) {
	Log(INFO, ctx, "GetStorage", "region", config.Region)
	sysDB.Lock()
	defer sysDB.Unlock()

	if sysDB.caching && sysDB.storage != nil {
		Log(INFO, ctx, "GetStorage", "cached", config.Region)
		return sysDB.storage, nil
	}

	var err error
	sysDB.storage, err = NewStorage(ctx, config)
	return sysDB.storage, err
}

// dynamodbTableDescription generates a standard table definition.
func dynamodbTableDescription(name string) *dynamodb.TableDescriptionT {
	return &dynamodb.TableDescriptionT{
		TableName: name,
		AttributeDefinitions: []dynamodb.AttributeDefinitionT{
			dynamodb.AttributeDefinitionT{
				Name: "id",
				Type: "S",
			},
		},
		KeySchema: []dynamodb.KeySchemaT{
			dynamodb.KeySchemaT{
				AttributeNam: "id",
				KeyType:      "HASH",
			},
		},
		ProvisionedThroughput: dynamodb.ProvisionedThroughputT{
			// ToDo: Change provisioned throughput.
			ReadCapacityUnits:  1,
			WriteCapacityUnits: 1,
		},
	}
}

func (s *DynamoDBStorage) Server(ctx *Context) *dynamodb.Server {
	return s.server
}

func (s *DynamoDBStorage) Table(ctx *Context) *dynamodb.Table {
	return s.table
}

// init creates the given table if it doesn't exist.
func (s *DynamoDBStorage) init(ctx *Context, table string) error {
	Log(INFO, ctx, "DynamoDBStorage.init", "table", table)
	td, err := s.server.DescribeTable(table)
	if err != nil {
		Log(INFO, ctx, "DynamoDBStorage.init", "creating", table)
		td = dynamodbTableDescription(table)
		_, err = s.server.CreateTable(*td)
		if err != nil {
			Log(INFO, ctx, "DynamoDBStorage.init", "error", err)
			return err
		}
	}
	Log(INFO, ctx, "DynamoDBStorage.init", "td", td)

	pk, err := td.BuildPrimaryKey()
	if err != nil {
		panic(err)
	}
	if pk.KeyAttribute == nil {
		// Puslar issue?
		name := td.AttributeDefinitions[0].Name
		attr := dynamodb.NewStringAttribute(name, "")
		attr.Type = td.AttributeDefinitions[0].Type
		pk.KeyAttribute = attr
		Log(INFO, ctx, "DynamoDBStorage.init", "pulsar", *pk.KeyAttribute)
	}
	s.table = s.server.NewTable(table, pk)
	return nil
}

// Load queries DynamoDB to get a location's state.
//
// This function also calls the location's 'Update()' function to
// remember the time the state was last updated (and when we read that
// state).
func (s *DynamoDBStorage) Load(ctx *Context, loc string) ([]Pair, error) {
	Log(INFO, ctx, "DynamoDBStorage.Load", "location", loc)
	t := NewTimer(ctx, "DynamoDBStorage.Load")
	defer t.Stop()

	k := dynamodb.Key{HashKey: loc}
	// ToDo: Reconsider GetItemConsistent()

	timer := NewTimer(ctx, "DynamoDBStorage.GetItem")
	as, err := s.table.GetItemConsistent(&k, s.Consistent)
	timer.Stop()

	if err == dynamodb.ErrNotFound {
		return make([]Pair, 0, 0), nil
	}
	if err != nil {
		return nil, err
	}
	acc := make([]Pair, 0, len(as)-1)
	// ToDo: Find the last update timestamp, if any.  Call
	// loc.update() to with the value or "" of no lock.
	for key, v := range as {
		if key == "id" {
			continue
		}
		if key == LastUpdatedKey {
			Log(INFO, ctx, "DynamoDBStorage.Load", "location", loc, "lastUpdated", v.Value)
			ctx.Location().Update(ctx, v.Value)
			continue
		}
		acc = append(acc, Pair{[]byte(v.Name), []byte(v.Value)})
	}

	return acc, nil
}

// peek prints out a location's state.
//
// Obviously just for debugging.
func (s *DynamoDBStorage) peek(ctx *Context, loc string) {
	fmt.Printf("peeking at %s\n", loc)
	k := dynamodb.Key{HashKey: loc}
	as, err := s.table.GetItemConsistent(&k, s.Consistent)
	if err == dynamodb.ErrNotFound {
		fmt.Printf("  not found\n")
		return
	}
	if err != nil {
		fmt.Printf("  error %v\n", err)
		return
	}
	for key, v := range as {
		fmt.Printf("  %v %s: %s\n", key, v.Name, v.Value)
	}
}

func ConditionalCheckFailedException(err error) bool {
	// Yuck!
	return err != nil && strings.HasPrefix(err.Error(), "ConditionalCheckFailedException")
}

// Add writes the given additional state to DynamoDB.
//
// If 'CheckLastUpdated' is true, this function attempts to verify
// that the state in DynamoDB hasn't changed since we last loaded it
// or changed it.  If the state has changed unexpectedly, you get an
// error (which should be ConcurrentStateChange, but currently is just
// the error returned by the SDK).
func (s *DynamoDBStorage) Add(ctx *Context, loc string, m *Pair) error {
	Log(INFO, ctx, "DynamoDBStorage.Add", "location", loc, "m", m.String())

	lastUpdated, updating := ctx.Location().Update(ctx, "")
	Log(DEBUG, ctx, "DynamoDBStorage.Add", "location", loc, "lastUpdated", lastUpdated, "updatingAt", updating)

	attrs := []dynamodb.Attribute{
		*dynamodb.NewStringAttribute(string(m.K), string(m.V)),
		*dynamodb.NewStringAttribute(LastUpdatedKey, updating),
	}
	k := dynamodb.Key{HashKey: loc}

	var err error
	var ok bool
	if CheckLastUpdated {
		// Maybe retry a little to see if we get the state we expect?
		// Probably no point, so don't actually loop.
		for i := 0; i < 1; i++ {
			// Ugh: "ExpressionAttributeValues contains invalid
			// value: One or more parameter values were invalid:
			// An AttributeValue may not contain an empty string
			// for key :Expected0."
			if lastUpdated == "" {
				cond := dynamodb.Expression{
					Text: "attribute_not_exists(#u)",
					AttributeNames: map[string]string{
						"#u": LastUpdatedKey,
					},
				}
				ok, err = s.table.ConditionExpressionUpdateAttributes(&k, attrs, &cond)
			} else {
				expected := []dynamodb.Attribute{
					*dynamodb.NewStringAttribute(LastUpdatedKey, lastUpdated),
				}
				ok, err = s.table.ConditionalUpdateAttributes(&k, attrs, expected)
			}
			if !ConditionalCheckFailedException(err) {
				break
			}
			// s.peek(ctx, loc)
			Log(WARN, ctx, "DynamoDBStorage.Add", "location", loc, "error", err, "retry", i)
			time.Sleep(10 * time.Millisecond)
		}
	} else {
		ok, err = s.table.UpdateAttributes(&k, attrs)
	}

	if err != nil {
		Log(WARN, ctx, "DynamoDBStorage.Add", "location", loc, "error", err, "ok", ok)
		return err
	}
	return nil
}

// Remove removes the given data from DynamoDB.
//
// If 'CheckLastUpdated' is true, this function attempts to verify
// that the state in DynamoDB hasn't changed since we last loaded it
// or changed it.  If the state has changed unexpectedly, you get an
// error (which should be ConcurrentStateChange, but currently is just
// the error returned by the SDK).
func (s *DynamoDBStorage) Remove(ctx *Context, loc string, id []byte) (int64, error) {
	Log(INFO, ctx, "DynamoDBStorage.Remove", "location", loc, "id", string(id))

	lastUpdated, updating := ctx.Location().Update(ctx, "")
	Log(DEBUG, ctx, "DynamoDBStorage.Add", "location", loc, "lastUpdated", lastUpdated, "updatingAt", updating)

	attrs := []dynamodb.Attribute{
		*dynamodb.NewStringAttribute(string(id), ""),
		*dynamodb.NewStringAttribute(LastUpdatedKey, updating),
	}
	k := dynamodb.Key{HashKey: loc}

	var err error
	n := int64(1)
	var ok bool
	if CheckLastUpdated {
		// See comments elsewhere near CheckLastUpdated uses.
		update := dynamodb.Expression{
			Text: "REMOVE #a SET #u = :u",
			AttributeNames: map[string]string{
				"#a": string(id),
				"#u": lastUpdated,
			},
			AttributeValues: []dynamodb.Attribute{
				*dynamodb.NewStringAttribute(":u", updating),
			},
		}
		var cond dynamodb.Expression
		if lastUpdated == "" {
			cond = dynamodb.Expression{
				Text: "attribute_not_exists(#u)",
				AttributeNames: map[string]string{
					"#u": LastUpdatedKey,
				},
			}
		} else {
			cond = dynamodb.Expression{
				Text: "#u = :v",
				AttributeNames: map[string]string{
					"#u": LastUpdatedKey,
				},
				AttributeValues: []dynamodb.Attribute{
					*dynamodb.NewStringAttribute(":v", lastUpdated),
				},
			}
		}
		ok, err = s.table.UpdateExpressionUpdateAttributes(&k, &cond, &update)
	} else {
		ok, err = s.table.DeleteAttributes(&k, attrs)
	}
	if !ok {
		n = 0
	}
	return n, err
}

func (s *DynamoDBStorage) Clear(ctx *Context, loc string) (int64, error) {
	Log(INFO, ctx, "DynamoDBStorage.Clear", "location", loc)

	lastUpdated, updating := ctx.Location().Update(ctx, "clear")
	Log(DEBUG, ctx, "DynamoDBStorage.Clear", "location", loc, "lastUpdated", lastUpdated, "updatingAt", updating)

	k := dynamodb.Key{HashKey: loc}
	var ok bool
	var err error
	n := int64(0)
	if CheckLastUpdated && lastUpdated != "" {
		// See comments elsewhere near CheckLastUpdated uses.
		cond := []dynamodb.Attribute{
			*dynamodb.NewStringAttribute(LastUpdatedKey, lastUpdated),
		}
		ok, err = s.table.ConditionalDeleteItem(&k, cond)
	} else {
		ok, err = s.table.DeleteItem(&k)
	}
	if ok {
		n = 1
	}
	return n, err
}

func (s *DynamoDBStorage) Delete(ctx *Context, loc string) error {
	Log(INFO, ctx, "DynamoDBStorage.Delete", "location", loc)
	_, err := s.Clear(ctx, loc)
	return err
}

func (s *DynamoDBStorage) GetStats(ctx *Context, loc string) (StorageStats, error) {
	Log(INFO, ctx, "DynamoDBStorage.GetStats", "string", loc)
	td, err := s.server.DescribeTable(s.tableName)
	ss := StorageStats{}
	if err != nil {
		return ss, err
	}

	ss.NumRecords = int(td.ItemCount) // ToDo: Should be int64

	return ss, nil
}

// Purge should never be called.
// ToDo: Remove this method from the interface, and kill the old mutation code.
func (ms *DynamoDBStorage) Purge(ctx *Context, loc string, t int64) (int64, error) {
	Log(INFO, ctx, "DynamoDBStorage.Purge", "location", loc, "t", t)
	panic(fmt.Errorf("DynamoDBStorage.Purge not implemented"))
}

func (ms *DynamoDBStorage) Close(ctx *Context) error {
	return nil
}

func (s *DynamoDBStorage) Health(ctx *Context) error {
	// Check that the table actually exists.  Not sure that's
	// really what we want.
	Log(INFO, ctx, "DynamoDBStorage.Health", "table", s.tableName)
	_, err := s.server.DescribeTable(s.tableName)
	if err != nil {
		Log(ERROR, ctx, "DynamoDBStorage.Health", "table", s.tableName, "error", err)
	}
	return err
}

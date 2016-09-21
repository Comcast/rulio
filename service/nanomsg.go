// Copyright 2016 Comcast Cable Communications Management, LLC
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

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Comcast/rulio/core"

	"github.com/go-mangos/mangos"
	"github.com/go-mangos/mangos/protocol/pub"
	"github.com/go-mangos/mangos/protocol/sub"
	"github.com/go-mangos/mangos/transport/ipc"
	"github.com/go-mangos/mangos/transport/tcp"

	"github.com/robertkrimen/otto"
)

// NanomsgService provided Nanomsg pub/sub transport around a rules
// Service.
//
// Fun fact: The original rules transport was 0MQ, which was dropped
// in favor of HTTP (sadly).
type NanomsgService struct {
	Ctx *core.Context

	// Service is the underlying rules service
	Service *Service

	// Name is the name of the Nanomsg subscriber.
	Name string

	// FromURL gives the subscriber's Nanomsg URL.
	FromURL string

	// FromPrefix is the subscription prefix.
	FromPrefix string

	// ToURL is an optional URL for publishing results of the
	// requests sent to rules.
	ToURL string

	// ToPrefix is an optional message prefix for those outbound
	// results published to ToURL (if given).
	ToPrefix string

	// AuxURL is an optional URL for publishing messages via the
	// Javascript 'nanomsg' function.
	AuxURL string

	// AuxPrefix is an optional message prefix for messages
	// published via the Javascript 'nanomsg' function.
	AuxPrefix string
}

// go starts a listener in a new goroutine.
//
// Provide a channel of errors if you want to learn about problems.
//
// Close the returned channel to stop the listener.
func (ns *NanomsgService) Go(errs chan error) (error, chan bool) {

	core.Log(core.INFO, ns.Ctx, "service.NanomsgService", "from", ns.FromURL, "fromPrefix", ns.FromPrefix)
	core.Log(core.INFO, ns.Ctx, "service.NanomsgService", "to", ns.ToURL, "toPrefix", ns.ToPrefix)

	if err := ns.initAux(); err != nil {
		return err, nil
	}

	// See https://github.com/go-mangos/mangos/blob/master/examples/pubsub/pubsub.go

	protest := func(err error) {
		if errs != nil {
			errs <- err
		} else {
			log.Printf("NanomsgService %s error %v", ns.Name, err)
		}
	}

	emit := func(msg string) {
		log.Printf("NanomsgService %s result: '%s'", ns.Name, msg)
	}

	if ns.ToURL != "" {
		// Redefine emit() to send results to the right place.

		sock, err := pub.NewSocket()
		if err != nil {
			return err, nil
		}
		sock.AddTransport(ipc.NewTransport())
		sock.AddTransport(tcp.NewTransport())
		if err = sock.Listen(ns.ToURL); err != nil {
			return err, nil
		}

		emit = func(msg string) {
			if ns.ToPrefix != "" {
				msg = ns.ToPrefix + ":" + msg
			}
			if err = sock.Send([]byte(msg)); err != nil {
				protest(err)
			}
		}
	}

	sock, err := sub.NewSocket()
	if err != nil {
		return err, nil
	}
	sock.AddTransport(ipc.NewTransport())
	sock.AddTransport(tcp.NewTransport())
	if err = sock.Dial(ns.FromURL); err != nil {
		return err, nil
	}

	err = sock.SetOption(mangos.OptionSubscribe, []byte(ns.FromPrefix))
	if err != nil {
		return err, nil
	}

	ctl := make(chan bool)

	go func() {
		for {
			msg, err := sock.Recv()
			if err != nil {
				protest(err)
			}
			log.Printf("NanomsgService %s heard %s", ns.Name, msg)

			// We might have a PREFIX:{...}.
			brace := []byte{'{'}
			if at := bytes.Index(msg, brace); 1 < at {
				// We do.  Strip it.
				// prefix := string(msg[0:at])
				msg = msg[at:]
			}

			// "The more you know, the less you need."
			//
			//  -- Yvon Chouinard

			var m map[string]interface{}
			if err = json.Unmarshal(msg, &m); err != nil {
				protest(err)
			}

			x, _ := m["uri"]
			uri := x.(string)
			m["uri"] = DWIMURI(nil, uri)
			out := bytes.NewBuffer(nil)

			if _, err = ns.Service.ProcessRequest(ns.Ctx, m, out); err != nil {
				protest(err)
			}
			emit(out.String())
		}
	}()

	return nil, ctl
}

func (ns *NanomsgService) initAux() error {
	if ns.AuxURL == "" {
		return nil
	}

	core.Log(core.INFO, ns.Ctx, "service.NanomsgService", "aus", ns.AuxURL, "toPrefix", ns.AuxPrefix)

	sock, err := pub.NewSocket()
	if err != nil {
		return err
	}
	sock.AddTransport(ipc.NewTransport())
	sock.AddTransport(tcp.NewTransport())
	if err = sock.Listen(ns.AuxURL); err != nil {
		return err
	}

	emit := func(msg string) error {
		if ns.AuxPrefix != "" {
			msg = ns.AuxPrefix + ":" + msg
		}
		log.Printf("emitting '%s'", msg)
		if err = sock.Send([]byte(msg)); err != nil {
			return err
		}
		return nil
	}

	// Let's define a Javascript function that will allow
	// us to publish to 'aux'.
	ns.Ctx.App = &core.BindingApp{
		JavascriptBindings: map[string]interface{}{
			"nanomsg": func(call otto.FunctionCall) otto.Value {
				x := call.Argument(0)
				msg, err := x.ToString()
				if err != nil {
					core.ThrowJavascript(call.Otto.Call("new Error", nil, fmt.Sprintf("No bad first arg: %v", err)))
				} else {
					if err = emit(msg); err != nil {
						core.ThrowJavascript(call.Otto.Call("new Error", nil, err.Error()))
					}
				}
				return x
			},
		},
	}

	return nil
}

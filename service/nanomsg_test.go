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
	"fmt"
	"log"
	"testing"
	"time"

	"github.com/Comcast/rulio/core"
	"github.com/Comcast/rulio/sys"

	"github.com/robertkrimen/otto"

	"github.com/go-mangos/mangos"
	"github.com/go-mangos/mangos/protocol/pub"
	"github.com/go-mangos/mangos/protocol/sub"
	"github.com/go-mangos/mangos/transport/ipc"
	"github.com/go-mangos/mangos/transport/tcp"
)

func TestNanomsg(t *testing.T) {
	// Publish in-bound messages to
	in := "tcp://127.0.0.1:40899"
	inboundPrefix := "jokes"

	// Publish message processing results to
	out := "tcp://127.0.0.1:40900"
	outboundPrefix := "insults"

	// From Javascript, publish messages to
	aux := "tcp://127.0.0.1:40901"
	auxPrefix := "lies"

	{
		// Start up our rules service

		sys, ctx := sys.ExampleSystem("Test")

		// Let's make a Nanomsg-emitting function we can call from Javascript.

		sock, err := pub.NewSocket()
		if err != nil {
			t.Fatal(err)
		}
		sock.AddTransport(ipc.NewTransport())
		sock.AddTransport(tcp.NewTransport())
		if err = sock.Listen(aux); err != nil {
			t.Fatal(err)
		}

		emit := func(msg string) {
			msg = auxPrefix + ":" + msg
			log.Printf("emitting '%s'", msg)
			if err = sock.Send([]byte(msg)); err != nil {
				t.Fatal(err)
			}
		}

		// Let's define a Javascript function that will allow
		// us to publish to 'aux'.
		ctx.App = &core.BindingApp{
			JavascriptBindings: map[string]interface{}{
				"nanomsg": func(call otto.FunctionCall) otto.Value {
					x := call.Argument(0)
					msg, err := x.ToString()
					if err != nil {
						t.Fatal(err)
					} else {
						emit(msg)
					}
					return x
				},
			},
		}
		service := &Service{
			System: sys,
		}
		ns := &NanomsgService{
			Ctx:        ctx,
			Name:       "test",
			FromURL:    in,
			FromPrefix: inboundPrefix,
			Service:    service,
			ToURL:      out,
			ToPrefix:   outboundPrefix,
		}
		errs := make(chan error)
		go func() {
			for {
				err := <-errs
				if err != nil {
					t.Fatal(err)
				}
			}
		}()

		ns.Go(errs)
	}

	{
		// Start up another listener for 'aux' to hear the
		// results of message processing.

		sock, err := sub.NewSocket()
		if err != nil {
			t.Fatal(err)
		}
		sock.AddTransport(ipc.NewTransport())
		sock.AddTransport(tcp.NewTransport())
		if err = sock.Dial(out); err != nil {
			t.Fatal(err)
		}

		if err = sock.SetOption(mangos.OptionSubscribe, []byte(outboundPrefix)); err != nil {
			t.Fatal(err)
		}

		go func() {
			for {
				msg, err := sock.Recv()
				if err != nil {
					t.Fatal(err)
				}
				log.Printf("aux heard %s\n", string(msg))
			}
		}()
	}

	{
		// Start up *another* listener to hear messages we
		// emit from Javascript via the 'nanomsg' function.

		sock, err := sub.NewSocket()
		if err != nil {
			t.Fatal(err)
		}
		sock.AddTransport(ipc.NewTransport())
		sock.AddTransport(tcp.NewTransport())
		if err = sock.Dial(aux); err != nil {
			t.Fatal(err)
		}

		if err = sock.SetOption(mangos.OptionSubscribe, []byte(auxPrefix)); err != nil {
			t.Fatal(err)
		}

		go func() {
			for {
				msg, err := sock.Recv()
				if err != nil {
					t.Fatal(err)
				}
				log.Printf("monitor heard result %s\n", string(msg))
			}
		}()
	}

	// Start sending requests to the rules service (via Nanomsg).
	// We are our own source of in-bound test messages.
	//
	// Note that we can perform *any* simple API call (not just
	// event processing).  In this test, we'll execute some
	// Javascript that will call our special 'nanomsg()' function.
	// We'll also return a result, which should should also be
	// emitted.

	sock, err := pub.NewSocket()
	if err != nil {
		t.Fatal(err)
	}
	sock.AddTransport(ipc.NewTransport())
	sock.AddTransport(tcp.NewTransport())
	if err = sock.Listen(in); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	for i := 0; i < 5; i++ {
		msg := fmt.Sprintf(`{"uri": "sys/util/js", "code":"nanomsg('n=' + (1+%d)); %d"}`, i, i)
		if inboundPrefix != "" {
			msg = inboundPrefix + ":" + msg
		}
		log.Printf("sending '%s'", msg)
		if err = sock.Send([]byte(msg)); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Second)
	}
}

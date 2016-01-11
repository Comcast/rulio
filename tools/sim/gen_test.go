package main

import (
	"encoding/json"
	"log"
	"testing"
)

func TestChooseEmit(t *testing.T) {
	cs := Choose{"foo": 0.20, "bar": 0.50, "beer": 0.30}
	n := 1000

	histogram := make(map[string]int)
	for i := 0; i < n; i++ {
		x, err := cs.Emit()
		if err != nil {
			t.Fatal(err)
		}
		k := x.(string)
		histogram[k]++
	}
	for p, v := range histogram {
		log.Printf("%0.2f %s", float64(v)/float64(n), p)
	}
}

func TestGenWalk(t *testing.T) {
	e := map[string]interface{}{
		"foo":   1,
		"likes": Generate{Choose: map[string]float64{"tacos": 0.70, "chips": 0.30}},

		"bar": Generate{Uniform: &Uniform{10.0, 20.0}},
	}
	out, err := GenWalk(e)
	log.Printf("out %#v (%v)", out, err)
	out, err = GenWalk(e)
	log.Printf("out %#v (%v)", out, err)
}

func TestRuleInstantiation(t *testing.T) {
	var rule Rule
	if err := json.Unmarshal([]byte(`{"likes":"Beer"}`), &rule); err != nil {
		t.Fatal(err)
	}

	dogfish := "Dogfish"
	r, err := rule.instantiate(map[string]string{"Beer": dogfish})
	if err != nil {
		t.Fatal(err)
	}

	foo, have := (*r)["likes"]
	if !have {
		t.Fatal("no beer")
	}
	s, okay := foo.(string)
	if !okay {
		t.Fatal("not a string")
	}
	if s != dogfish {
		t.Fatal("got " + s)
	}
}

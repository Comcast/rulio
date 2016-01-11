package main

import (
	"fmt"
	"testing"
)

func TestPercentiles(t *testing.T) {
	size := 1000
	ns := NewLatencies(size)
	for n := 0; n < size; n++ {
		ns.Add(size - n)
	}
	stats, err := ns.Stats()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%#v\n", *stats)
}

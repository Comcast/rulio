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

package core

// In-memory term index
//
// An inverted index: term -> set of fact IDs.
//
// Index a fact ID under the fact's terms.  Each term has a set of
// fact IDs.  A query for a set of terms returns the intersection of
// those terms' ID sets.
//
// We'll want some warnings or even errors when certain soft or hard
// limits are exceeded.  For example, if some fool sends us a really
// large number of terms (e.g., numbers in a wide range), then we
// should at some point protest rather than killing ourselves.

import (
	"encoding/json"
	"errors"
	"fmt"
)

// The main structure: A map from terms to sets of IDs.  A fact is
// indexed by its terms.  This implementation uses hash tables, so we
// cannot do prefix searches.  (We could if we used some sort of
// ordered set such as a B*tree.)  Therefore, our terms must no have
// structure (like 'a.b.c' from a fact '{a:{b:c}}').  We'll use
// strings as our fact IDs.
type TermIndex struct {
	Index map[string]StringSet
}

func NewTermIndex() *TermIndex {
	return &TermIndex{make(map[string]StringSet)}
}

// Basic statistics about a TermIndex.  'entryCount' is the sum of the
// sizes of the IDs sets for each term.  'idCount' is the number of
// IDs put into the index.  'idDups' is the number of repeated IDs.
type TermIndexMetrics struct {
	TermCount  int
	EntryCount int
	IdCount    int // Slow
	IdDups     int // Slow
}

// Populate TermIndexMetrics with only metrics that are quick to
// compute.  Also see 'SlowTermIndexMetrics'.
func (ti *TermIndex) FastMetrics(ctx *Context) *TermIndexMetrics {
	metrics := TermIndexMetrics{}
	metrics.TermCount = len(ti.Index)
	for _, m := range ti.Index {
		metrics.EntryCount += len(m)
	}
	metrics.IdCount = -1 // Not valid.
	metrics.IdDups = -1  // Not valid.
	return &metrics
}

// Fully populate TermIndexMetrics.  This function traverses all of
// the sets in the index, so this function can take a while to
// execute.  Also see 'FastMetrics'.
func (ti *TermIndex) SlowMetrics(ctx *Context) *TermIndexMetrics {
	metrics := ti.FastMetrics(ctx)
	ids := make(StringSet)
	for _, m := range ti.Index {
		for id, _ := range m {
			if ids.Contains(id) {
				metrics.IdDups++
			}
			ids.Add(id)
		}
	}
	metrics.IdCount = len(ids)
	return metrics

}

// Index the given fact ID at the given term.
func (ti *TermIndex) Add(ctx *Context, term string, id string) {
	// ToDo: Return an error if the size of the index is greater than some
	// specified limit.
	Log(DEBUG, ctx, "TermIndex.Add", "term", term, "id", id)
	ids, ok := ti.Index[term]
	if !ok {
		ids = make(map[string]struct{})
		ti.Index[term] = ids
	}
	Log(DEBUG, ctx, "TermIndex.Add", "id", id, "term", term)
	ids.Add(id)
}

// Remove the given ID at the given term.
func (ti *TermIndex) Rem(ctx *Context, term string, id string) {
	Log(DEBUG, ctx, "TermIndex.Rem", "term", term, "id", id)
	ids, ok := ti.Index[term]
	if !ok {
		Log(DEBUG, ctx, "TermIndex.Rem", "warning", "term empty", "term", term, "id", id)
	} else {
		// ToDo: Check if ID is really there and warn if not.
		Log(DEBUG, ctx, "TermIndex.Rem", "id", id, "term", term)
		ids.Rem(id)
		if len(ids) == 0 {
			Log(DEBUG, ctx, "TermIndex.Rem", "term", term, "id", id, "note", "Removing empty ids set")
			delete(ti.Index, term)
		}
	}
}

// Remove all entries for the given fact ID.  Must check every term,
// so this operation is slow.  Use 'RemIdTerms' if you know a fact's
// terms.
func (ti *TermIndex) RemID(ctx *Context, id string) {
	Log(DEBUG, ctx, "TermIndex.RemID", "id", id) // Basically a warning.
	for term, _ := range ti.Index {
		ti.Rem(ctx, term, id)
	}
}

// Remove all entries for the given ID at the given terms.
func (ti *TermIndex) RemIdTerms(ctx *Context, terms []string, id string) {
	Log(DEBUG, ctx, "TermIndex.RemIdTerms", "id", id, "terms", terms)
	for _, term := range terms {
		ti.Rem(ctx, term, id)
	}
}

// How big is the set of IDs for the given term?
func (ti *TermIndex) TermCard(ctx *Context, term string) int {
	ids, ok := ti.Index[term]
	if ok {
		return len(ids)
	}
	return 0
}

// Search the index.  The algorithm is current pretty naive but
// reasonably efficient.  We start with a term that has the lowest
// cardinality.  The set of IDs for that term becomes the set of
// candidate IDs.  For each remaining given term, eliminate a
// candidate if it does not appear in every set of remaining terms.
// func Search(ctx *Context, ti *TermIndex, terms []string) ([]string,
// error) {
func (ti *TermIndex) Search(ctx *Context, terms []string) ([]string, error) {
	Log(DEBUG, ctx, "TermIndex.Search", "terms", terms)
	timer := NewTimer(ctx, "TermIndex")
	defer timer.Stop()

	if len(terms) == 0 {
		// We could try to return all IDs, but instead we refused to.
		return nil, errors.New("No terms given.")
	}

	none := []string{}

	// Find the smallest given term.  Probably not worth sorting.
	lowestCount := ti.TermCard(ctx, terms[0])
	smallest := 0
	for i := 1; i < len(terms); i++ {
		count := ti.TermCard(ctx, terms[i])
		if count == 0 {
			return none, nil
		}
		if count < lowestCount {
			smallest = i
		}
	}

	// Get our set of candidates
	termIDs, _ := ti.Index[terms[smallest]]
	// Make sure we copy the set.
	candidates := make(StringSet)
	for id, _ := range termIDs {
		candidates.Add(id)
	}

	// Now check that each candidate is in every remaining term's
	// IDs.  We're iterating over terms, but maybe it'd be better
	// to iterate over the candidates first.
	for i, term := range terms[0:] {
		Log(DEBUG, ctx, "TermIndex.Search", "term", term, "candidates", len(candidates))
		if i == smallest { // Already did it.
			continue
		}
		ids, ok := ti.Index[term]
		if ok {
			Log(DEBUG, ctx, "TermIndex.Search", "term", term, "ids", len(ids))
			for id, _ := range candidates {
				_, present := ids[id]
				if !present {
					// I wonder if this delete during iteration will work.
					candidates.Rem(id)
				}
			}

		} else {
			return none, nil
		}
	}

	acc := candidates.Array()
	Log(DEBUG, ctx, "TermIndex.Search", "facts", acc)
	return acc, nil

}

// Print a JSON representation of the index.
func (ti *TermIndex) Show() {
	js, _ := json.MarshalIndent(ti, "", "  ")
	fmt.Printf("%s\n", js)
}

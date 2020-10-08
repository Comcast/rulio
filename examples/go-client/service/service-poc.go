// +build sample_client

package service

import (
	"encoding/json"
	"fmt"
	"github.com/Comcast/rulio/core"
	"github.com/Comcast/rulio/sys"
	"github.com/tidwall/pretty"
	"strings"
	"time"
)

type Service struct {
	System  *sys.System
	Router  interface{}
	Stopper func(*core.Context, time.Duration) error
}

func (s *Service) SearchFact(ctx *core.Context, m map[string]interface{}) error {
	core.Log(core.INFO, ctx, "SearchFacts map is", m)
	pattern, _, err := getMapParam(m, "pattern", true)
	if err != nil {
		return err
	}

	location, _, err := GetStringParam(m, "location", true)
	if err != nil {
		return err
	}

	includedInherited, _, err := getBoolParam(m, "inherited", false)
	if err != nil {
		return err
	}

	if err := s.checkLocal(ctx, location); err != nil {
		return err
	}
	js, err := json.Marshal(pattern)
	if err != nil {
		return err
	}

	sr, err := s.System.SearchFacts(ctx, location, string(js), includedInherited)
	if err != nil {
		return err
	}

	_, take := m["take"]
	if take {
		// Warning: Not (yet) atomic!
		for _, found := range sr.Found {
			_, err := s.System.RemFact(ctx, location, found.Id)
			if err != nil {
				core.Log(core.ERROR, ctx, "service.ProcessRequest", "app_tag", "/api/loc/facts/search", "error", err, "RemFact", found.Id)
			}
		}
	}

	js, err = json.Marshal(sr)
	if err != nil {
		return err
	}

	fmt.Printf("Fact found with value %v", sr)
	core.Log(core.INFO, ctx, "====Facts found.====>", "Fact", sr)
	return nil

}

func (s *Service) AddFact(ctx *core.Context, m map[string]interface{}) error {
	fmt.Printf("\nFacts input arguments are : %v.", m)
	core.Log(core.INFO, ctx, "map is", m)
	fact, _, err := getMapParam(m, "fact", true)
	if err != nil {
		return err
	}
	core.Log(core.INFO, ctx, "facts is", fact)
	location, _, err := GetStringParam(m, "location", true)
	if err != nil {
		return err
	}
	if err := s.checkLocal(ctx, location); err != nil {
		return err
	}
	id, _, err := GetStringParam(m, "id", false)
	// ToDo: Not this.
	js, err := json.Marshal(fact)
	if err != nil {
		return err
	}
	id, err = s.System.AddFact(ctx, location, id, string(js))
	if err != nil {
		return err
	}
	i := map[string]interface{}{"id": id}
	js, err = json.Marshal(&i)
	if err != nil {
		return err
	}
	fmt.Printf("\nFacts is created with id %v.", id)
	core.Log(core.INFO, ctx, "addFacts", "Id", id)
	return nil
}

func getMapParam(m map[string]interface{}, prop string, required bool) (map[string]interface{}, bool, error) {
	v, have := m[prop]
	if !have {
		if required {
			return nil, false, fmt.Errorf("Parameter %s missing", prop)
		}
		return nil, false, nil
	}
	switch v.(type) {
	case map[string]interface{}:
		return v.(map[string]interface{}), true, nil
	default:
		return nil, true, fmt.Errorf("Parameter %s type %T wrong", prop, v)
	}
}

func getBoolParam(m map[string]interface{}, prop string, required bool) (bool, bool, error) {
	v, have := m[prop]
	if !have {
		if required {
			return false, false, fmt.Errorf("Parameter %s missing", prop)
		}
		return false, false, nil
	}
	switch vv := v.(type) {
	case bool:
		return vv, true, nil
	case string:
		return strings.ToLower(vv) == "true", true, nil
	default:
		return false, true, fmt.Errorf("Parameter %s type %T wrong", prop, v)
	}
}

func GetStringParam(m map[string]interface{}, p string, required bool) (string, bool, error) {
	v, have := m[p]
	if !have {
		if required {
			return "", false, fmt.Errorf("Parameter %s missing", p)
		}
		return "", false, nil
	}
	switch v.(type) {
	case string:
		return v.(string), true, nil
	case []interface{}:
		var acc string
		for _, x := range v.([]interface{}) {
			switch x.(type) {
			case string:
				acc += x.(string)
			default:
				return "", true, fmt.Errorf("Parameter %s type %T wrong at %v %T", p, v, x, x)
			}
		}
		return acc, true, nil
	default:
		return "", true, fmt.Errorf("Parameter %s type %T wrong", p, v)
	}
}

// checkLocal is a little utility wrapper around Manager.Disposition.
//
// This function will return an error with a JSON message if the given
// location shouldn't be served by us.
//
// The current implementation is a no-op.  Waiting on a Router.
func (s *Service) checkLocal(ctx *core.Context, loc string) error {
	return nil
}

func (s *Service) AddRule(ctx *core.Context, input map[string]interface{}) error {
	rule, _, err := getMapParam(input, "rule", true)
	if err != nil {
		return err
	}

	location, _, err := GetStringParam(input, "location", true)
	if err != nil {
		return err
	}

	if err := s.checkLocal(ctx, location); err != nil {
		return err
	}

	id, _, err := GetStringParam(input, "id", false)

	// ToDo: Not this.
	js, err := json.Marshal(rule)
	if err != nil {
		return err
	}

	id, err = s.System.AddRule(ctx, location, id, string(js))
	if err != nil {
		return err
	}
	fmt.Printf("\nRule created with id:  %v", id)
	core.Log(core.INFO, ctx, "Rule created with id", id)
	return nil
}

func (s *Service) ListRule(ctx *core.Context, input map[string]interface{}) error {
	location, _, err := GetStringParam(input, "location", true)
	if nil != err {
		return err
	}

	if err := s.checkLocal(ctx, location); err != nil {
		return err
	}

	includedInherited, _, err := getBoolParam(input, "inherited", false)
	if err != nil {
		return err
	}

	ss, err := s.System.ListRules(ctx, location, includedInherited)
	if nil != err {
		return err
	}

	fmt.Printf("Available Rules %v", ss)
	return nil
}

func (s *Service) ProcessEvent(ctx *core.Context, input map[string]interface{}) error {
	event, _, err := getMapParam(input, "event", true)
	if err != nil {
		return err
	}

	location, _, err := GetStringParam(input, "location", true)
	if err != nil {
		return err
	}

	if err := s.checkLocal(ctx, location); err != nil {
		return err
	}

	js, err := json.Marshal(event)
	if err != nil {
		return err
	}

	ctx.LogAccumulatorLevel = core.EVERYTHING
	work, err := s.System.ProcessEvent(ctx, location, string(js))
	if err != nil {
		return err
	}

	js, err = json.Marshal(work)
	if err != nil {
		return err
	}
	//fmt.Println(fmt.Sprintf(`{"id":"%s","result":"%s""}`, "ID is: ", ctx.Id()+" Work is: ", work))

	result := fmt.Sprintf(`{"id":"%s","result":%s}`, ctx.Id(), js)
	core.Log(core.INFO, ctx, "/api/loc/events/ingest", "got", result)

	fmt.Printf("Result is: %v", string(pretty.Pretty(js)))
	return nil
}

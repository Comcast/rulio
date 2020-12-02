package core

import "github.com/Comcast/sheens/match"

// DefaultMatcher is the Matcher used by the core package.
var (
	AllowPropertyVariables       = true
	CheckForBadPropertyVariables = true

	DefaultMatcher = MapCastMatcher{SheensMatcher{&match.Matcher{
		AllowPropertyVariables:       AllowPropertyVariables,
		CheckForBadPropertyVariables: CheckForBadPropertyVariables,
	}}}
)

// Match provides backwards compatibility around the Matcher interface.
func Match(ctx *Context, pattern, fact interface{}, bs Bindings) ([]Bindings, error) {
	return DefaultMatcher.Match(pattern, fact, bs)
}

// Matches provides backwards compatibility around the Matcher interface.
func Matches(ctx *Context, pattern, fact interface{}) ([]Bindings, error) {
	return DefaultMatcher.Match(pattern, fact, map[string]interface{}{})
}

// Bindings is a map from variables (strings starting with a '?') to their
// values.
type Bindings map[string]interface{}

// Matcher defines an interface for pattern matching algorithms to be used by
// core.
type Matcher interface {
	// Match takes a pattern and a fact built from map[string]interface{} and
	// []interface{} structures, along with an initial set of bindings (may be
	// empty), and returns a slice of sets of bindings representing the sets of
	// matches.
	Match(pattern, fact interface{}, bs Bindings) ([]Bindings, error)
}

// SheensMatcher wraps the matcher provided by the
// github.com/Comcast/sheens/match for use by rulio.
type SheensMatcher struct {
	*match.Matcher
}

// Match implements the Matcher interface.
func (m SheensMatcher) Match(pattern, fact interface{}, bs Bindings) ([]Bindings, error) {
	bssExt, err := m.Matcher.Match(pattern, fact, match.Bindings(bs))
	bss := make([]Bindings, len(bssExt))
	for i := range bssExt {
		bss[i] = Bindings(bssExt[i])
	}
	return bss, err
}

// MapCastMatcher is a wrapper for casting Map values to
// map[string]interface{}, and slices to []interface{}.
type MapCastMatcher struct {
	Matcher
}

// Match implements the Matcher interface.
func (m MapCastMatcher) Match(pattern, fact interface{}, bs Bindings) ([]Bindings, error) {
	var cast func(interface{}) interface{}

	cast = func(iface interface{}) interface{} {
		switch v := iface.(type) {
		case Map, map[string]interface{}:
			m, ok := v.(map[string]interface{})
			if !ok {
				m = map[string]interface{}(v.(Map))
			}
			for k, v := range m {
				m[k] = cast(v)
			}
			iface = m
		case []interface{}:
			for i := range v {
				v[i] = cast(v[i])
			}
			iface = v
		default:
			if v, ok := ISlice(v); ok {
				iface = v
			}
		}
		return iface
	}

	return m.Matcher.Match(cast(pattern), cast(fact), bs)
}

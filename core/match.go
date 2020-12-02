package core

import "github.com/Comcast/sheens/match"

type Bindings match.Bindings

var (
	Matcher                = match.DefaultMatcher
	AllowPropertyVariables = Matcher.AllowPropertyVariables
)

func Match(ctx *Context, pattern, fact interface{}, bs Bindings) ([]Bindings, error) {
	return castBss(Matcher.Match(castMap(pattern), castMap(fact), match.Bindings(bs)))
}

func Matches(ctx *Context, pattern, fact interface{}) ([]Bindings, error) {
	return Match(ctx, pattern, fact, map[string]interface{}{})
}

func IsVariable(s string) bool {
	return Matcher.IsVariable(s)
}

func castBss(bssExt []match.Bindings, err error) ([]Bindings, error) {
	bss := make([]Bindings, len(bssExt))
	for i := range bssExt {
		bss[i] = Bindings(bssExt[i])
	}
	return bss, err
}

func castMap(iface interface{}) interface{} {
	switch v := iface.(type) {
	case Map, map[string]interface{}:
		m, ok := v.(map[string]interface{})
		if !ok {
			m = map[string]interface{}(v.(Map))
		}
		for k, v := range m {
			m[k] = castMap(v)
		}
		iface = m
	case []interface{}:
		for i := range v {
			v[i] = castMap(v[i])
		}
		iface = v
	default:
		if v, ok := ISlice(v); ok {
			iface = v
		}
	}

	return iface
}

package core

import "github.com/Comcast/sheens/match"

type Bindings match.Bindings

var (
	matcher                = match.DefaultMatcher
	AllowPropertyVariables = matcher.AllowPropertyVariables
)

func Match(ctx *Context, pattern, fact interface{}, bs Bindings) ([]Bindings, error) {
	bssExt, err := matcher.Match(pattern, fact, match.Bindings(bs))
	bss := make([]Bindings, len(bssExt))
	for i := range bssExt {
		bss[i] = Bindings(bssExt[i])
	}
	return bss, err
}

func Matches(ctx *Context, pattern, fact interface{}) ([]Bindings, error) {
	return Match(ctx, pattern, fact, map[string]interface{}{})
}

func IsVariable(s string) bool {
	return matcher.IsVariable(s)
}

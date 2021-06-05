package core

import (
	"github.com/robertkrimen/otto"
	"sync"
)

var _vmPool sync.Pool

func init() {
	_vmPool = sync.Pool{
		New: func() interface{}{
			return otto.New()
		},
	}
}

func getVM() *otto.Otto {
	if !SystemParameters.ScopedJavascriptRuntimes {
		return otto.New()
	}
	return _vmPool.Get().(*otto.Otto)
}
func returnVM(o *otto.Otto) {
	if !SystemParameters.ScopedJavascriptRuntimes {
		return
	}
	_vmPool.Put(o)
}
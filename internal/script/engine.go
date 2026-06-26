package script

import "github.com/dop251/goja"

type Engine struct {
	vm *goja.Runtime
}

func New() *Engine {
	return &Engine{vm: goja.New()}
}

func (e *Engine) RunPreRequest(src string, ctx *PreContext) error {
	return nil
}

func (e *Engine) RunPostResponse(src string, ctx *PostContext) ([]*TestResult, error) {
	return nil, nil
}

package script

import (
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
)


type Engine struct{}

func New() *Engine { return &Engine{} }

func (e *Engine) RunPreRequest(src string, ctx *PreContext) error {
	vm := goja.New()
	setPreContext(vm, ctx)
	vm.Set("log", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) > 0 {
			ctx.Logs = append(ctx.Logs, formatLogValue(vm, call.Arguments[0]))
		}
		return goja.Undefined()
	})
	if _, err := vm.RunString(src); err != nil {
		return err
	}
	readPreContext(vm, ctx)
	return nil
}

func (e *Engine) RunPostResponse(src string, ctx *PostContext) ([]*TestResult, error) {
	vm := goja.New()
	var results []*TestResult
	registerStdlib(vm, &results, ctx.Env, &ctx.Logs)
	setPostContext(vm, ctx)
	_, err := vm.RunString(src)
	return results, err
}

func setPreContext(vm *goja.Runtime, ctx *PreContext) {
	reqObj := vm.NewObject()
	_ = reqObj.Set("method", ctx.Request.Method)
	_ = reqObj.Set("url", ctx.Request.URL)
	_ = reqObj.Set("body", ctx.Request.Body)

	headersObj := vm.NewObject()
	for k, v := range ctx.Request.Headers {
		_ = headersObj.Set(k, v)
	}
	_ = reqObj.Set("headers", headersObj)
	_ = vm.Set("request", reqObj)

	envObj := vm.NewObject()
	for k, v := range ctx.Env {
		_ = envObj.Set(k, v)
	}
	_ = vm.Set("env", envObj)
}

func readPreContext(vm *goja.Runtime, ctx *PreContext) {
	reqVal := vm.Get("request")
	reqObj, ok := reqVal.(*goja.Object)
	if !ok {
		return
	}
	if v := reqObj.Get("method"); v != nil {
		ctx.Request.Method = v.String()
	}
	if v := reqObj.Get("url"); v != nil {
		ctx.Request.URL = v.String()
	}
	if v := reqObj.Get("body"); v != nil {
		ctx.Request.Body = v.String()
	}
	if headersVal := reqObj.Get("headers"); headersVal != nil {
		if headersObj, ok := headersVal.(*goja.Object); ok {
			newHeaders := make(map[string]string, len(headersObj.Keys()))
			for _, key := range headersObj.Keys() {
				newHeaders[key] = headersObj.Get(key).String()
			}
			ctx.Request.Headers = newHeaders
		}
	}
}

func setPostContext(vm *goja.Runtime, ctx *PostContext) {
	reqObj := vm.NewObject()
	_ = reqObj.Set("method", ctx.Request.Method)
	_ = reqObj.Set("url", ctx.Request.URL)
	_ = reqObj.Set("body", ctx.Request.Body)
	reqHeadersObj := vm.NewObject()
	for k, v := range ctx.Request.Headers {
		_ = reqHeadersObj.Set(k, v)
	}
	_ = reqObj.Set("headers", reqHeadersObj)
	_ = vm.Set("request", reqObj)

	respObj := vm.NewObject()
	_ = respObj.Set("status", ctx.Response.Status)
	_ = respObj.Set("statusText", ctx.Response.StatusText)
	_ = respObj.Set("body", ctx.Response.Body)
	_ = respObj.Set("duration", ctx.Response.Duration)

	respHeadersObj := vm.NewObject()
	for k, v := range ctx.Response.Headers {
		_ = respHeadersObj.Set(k, v)
	}
	_ = respObj.Set("headers", respHeadersObj)

	body := ctx.Response.Body
	_ = respObj.Set("json", func(call goja.FunctionCall) goja.Value {
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			panic(vm.NewGoError(fmt.Errorf("response.json(): %w", err)))
		}
		return vm.ToValue(data)
	})

	_ = vm.Set("response", respObj)

	envObj := vm.NewObject()
	for k, v := range ctx.Env {
		_ = envObj.Set(k, v)
	}
	_ = vm.Set("env", envObj)
}

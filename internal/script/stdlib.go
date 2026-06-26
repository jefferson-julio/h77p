package script

import "github.com/dop251/goja"

func registerStdlib(vm *goja.Runtime, results *[]*TestResult, env map[string]string) {
	vm.Set("test", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		name := call.Arguments[0].String()
		fn, ok := goja.AssertFunction(call.Arguments[1])
		if !ok {
			return goja.Undefined()
		}
		tr := &TestResult{Name: name}
		_, err := fn(goja.Undefined())
		if err != nil {
			tr.Passed = false
			if ex, ok := err.(*goja.Exception); ok {
				tr.Error = ex.Value().String()
			} else {
				tr.Error = err.Error()
			}
		} else {
			tr.Passed = true
		}
		*results = append(*results, tr)
		return goja.Undefined()
	})

	vm.Set("assert", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 || call.Arguments[0].ToBoolean() {
			return goja.Undefined()
		}
		msg := "assertion failed"
		if len(call.Arguments) > 1 {
			msg = call.Arguments[1].String()
		}
		// Panic with a plain JS string so the exception message is clean (no goja stack trace).
		panic(vm.ToValue(msg))
	})

	vm.Set("set", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		env[call.Arguments[0].String()] = call.Arguments[1].String()
		return goja.Undefined()
	})
}

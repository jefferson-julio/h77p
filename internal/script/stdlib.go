package script

import "github.com/dop251/goja"

// formatLogValue converts a goja value to a human-readable string.
// Strings and primitives pass through directly. Objects and arrays are
// serialized with JSON.stringify(v, null, 2) from within the JS runtime so
// that non-serializable properties (functions, etc.) are silently dropped,
// matching what a developer would expect from a browser console.
func formatLogValue(vm *goja.Runtime, v goja.Value) string {
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return v.String()
	}
	// Primitives: pass through without JSON wrapping.
	if exported := v.Export(); exported != nil {
		switch ev := exported.(type) {
		case string:
			return ev
		case bool, int64, float64:
			return v.String()
		}
	}
	// Objects/arrays: use JS JSON.stringify so function-valued properties are
	// skipped cleanly instead of causing a marshal error.
	if jsonObj := vm.GlobalObject().Get("JSON"); jsonObj != nil {
		if stringify, ok := goja.AssertFunction(jsonObj.ToObject(vm).Get("stringify")); ok {
			if result, err := stringify(goja.Undefined(), v, goja.Null(), vm.ToValue(2)); err == nil &&
				!goja.IsUndefined(result) && !goja.IsNull(result) {
				return result.String()
			}
		}
	}
	return v.String()
}

func registerStdlib(vm *goja.Runtime, results *[]*TestResult, env map[string]string, logs *[]string) {
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

	vm.Set("log", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			*logs = append(*logs, "")
			return goja.Undefined()
		}
		*logs = append(*logs, formatLogValue(vm, call.Arguments[0]))
		return goja.Undefined()
	})
}

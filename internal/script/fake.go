package script

import (
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/dop251/goja"
)

func registerFake(vm *goja.Runtime) {
	f := gofakeit.New(0)

	fakeObj := vm.NewObject()

	_ = fakeObj.Set("name", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Name())
	})
	_ = fakeObj.Set("firstName", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.FirstName())
	})
	_ = fakeObj.Set("lastName", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.LastName())
	})
	_ = fakeObj.Set("email", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Email())
	})
	_ = fakeObj.Set("uuid", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.UUID())
	})
	_ = fakeObj.Set("phone", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Phone())
	})
	_ = fakeObj.Set("url", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.URL())
	})
	_ = fakeObj.Set("ipv4", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.IPv4Address())
	})
	_ = fakeObj.Set("int", func(call goja.FunctionCall) goja.Value {
		min, max := 0, 100
		if len(call.Arguments) >= 1 {
			min = int(call.Arguments[0].ToInteger())
		}
		if len(call.Arguments) >= 2 {
			max = int(call.Arguments[1].ToInteger())
		}
		return vm.ToValue(f.IntRange(min, max))
	})
	_ = fakeObj.Set("float", func(call goja.FunctionCall) goja.Value {
		min, max := 0.0, 100.0
		if len(call.Arguments) >= 1 {
			min = call.Arguments[0].ToFloat()
		}
		if len(call.Arguments) >= 2 {
			max = call.Arguments[1].ToFloat()
		}
		return vm.ToValue(f.Float64Range(min, max))
	})
	_ = fakeObj.Set("bool", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Bool())
	})
	_ = fakeObj.Set("word", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Word())
	})
	_ = fakeObj.Set("sentence", func(call goja.FunctionCall) goja.Value {
		n := 6
		if len(call.Arguments) >= 1 {
			n = int(call.Arguments[0].ToInteger())
		}
		return vm.ToValue(f.Sentence(n))
	})
	_ = fakeObj.Set("paragraph", func(call goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Paragraph(1, 3, 10, " "))
	})
	_ = fakeObj.Set("username", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Username())
	})
	_ = fakeObj.Set("password", func(call goja.FunctionCall) goja.Value {
		length := 12
		if len(call.Arguments) >= 1 {
			length = int(call.Arguments[0].ToInteger())
		}
		return vm.ToValue(f.Password(true, true, true, false, false, length))
	})
	_ = fakeObj.Set("hex", func(call goja.FunctionCall) goja.Value {
		length := 8
		if len(call.Arguments) >= 1 {
			length = int(call.Arguments[0].ToInteger())
		}
		const hexChars = "0123456789abcdef"
		buf := make([]byte, length)
		for i := range buf {
			buf[i] = hexChars[f.IntRange(0, 15)]
		}
		return vm.ToValue(string(buf))
	})
	_ = fakeObj.Set("color", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.HexColor())
	})
	_ = fakeObj.Set("date", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Date().Format(time.RFC3339))
	})
	_ = fakeObj.Set("pastDate", func(_ goja.FunctionCall) goja.Value {
		d := f.DateRange(time.Now().AddDate(-5, 0, 0), time.Now())
		return vm.ToValue(d.Format(time.RFC3339))
	})
	_ = fakeObj.Set("futureDate", func(_ goja.FunctionCall) goja.Value {
		d := f.DateRange(time.Now(), time.Now().AddDate(5, 0, 0))
		return vm.ToValue(d.Format(time.RFC3339))
	})
	_ = fakeObj.Set("city", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.City())
	})
	_ = fakeObj.Set("country", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(f.Country())
	})
	_ = fakeObj.Set("address", func(_ goja.FunctionCall) goja.Value {
		a := f.Address()
		obj := vm.NewObject()
		_ = obj.Set("street", a.Address)
		_ = obj.Set("city", a.City)
		_ = obj.Set("state", a.State)
		_ = obj.Set("zip", a.Zip)
		_ = obj.Set("country", a.Country)
		return obj
	})

	_ = vm.Set("fake", fakeObj)
}

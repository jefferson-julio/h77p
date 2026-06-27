package script

import (
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// momentToGoLayout converts a subset of moment.js format tokens to Go's reference-time
// layout string. Tokens are matched greedily left-to-right (longer tokens first).
func momentToGoLayout(format string) string {
	tokens := []struct{ moment, goFmt string }{
		{"YYYY", "2006"},
		{"YY", "06"},
		{"MM", "01"},
		{"DD", "02"},
		{"HH", "15"},
		{"hh", "03"},
		{"mm", "04"},
		{"ss", "05"},
		{"M", "1"},
		{"D", "2"},
		{"h", "3"},
		{"m", "4"},
		{"s", "5"},
		{"A", "PM"},
		{"a", "pm"},
		{"Z", "-07:00"},
	}
	var out strings.Builder
	i := 0
	for i < len(format) {
		matched := false
		for _, t := range tokens {
			if strings.HasPrefix(format[i:], t.moment) {
				out.WriteString(t.goFmt)
				i += len(t.moment)
				matched = true
				break
			}
		}
		if !matched {
			out.WriteByte(format[i])
			i++
		}
	}
	return out.String()
}

func addDuration(t time.Time, n float64, unit string) time.Time {
	switch unit {
	case "year", "years", "y":
		return t.AddDate(int(n), 0, 0)
	case "month", "months", "M":
		return t.AddDate(0, int(n), 0)
	case "week", "weeks", "w":
		return t.AddDate(0, 0, int(n)*7)
	case "day", "days", "d":
		return t.AddDate(0, 0, int(n))
	case "hour", "hours", "h":
		return t.Add(time.Duration(n * float64(time.Hour)))
	case "minute", "minutes", "m":
		return t.Add(time.Duration(n * float64(time.Minute)))
	case "second", "seconds", "s":
		return t.Add(time.Duration(n * float64(time.Second)))
	case "millisecond", "milliseconds", "ms":
		return t.Add(time.Duration(n * float64(time.Millisecond)))
	}
	return t
}

func durationInUnit(d time.Duration, unit string) float64 {
	switch unit {
	case "year", "years", "y":
		return d.Hours() / (24 * 365.25)
	case "month", "months", "M":
		return d.Hours() / (24 * 30.44)
	case "week", "weeks", "w":
		return d.Hours() / (24 * 7)
	case "day", "days", "d":
		return d.Hours() / 24
	case "hour", "hours", "h":
		return d.Hours()
	case "minute", "minutes", "m":
		return d.Minutes()
	case "second", "seconds", "s":
		return d.Seconds()
	default:
		return float64(d.Milliseconds())
	}
}

func startOf(t time.Time, unit string) time.Time {
	switch unit {
	case "year", "y":
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	case "month", "M":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case "week", "w":
		offset := int(t.Weekday())
		return time.Date(t.Year(), t.Month(), t.Day()-offset, 0, 0, 0, 0, t.Location())
	case "day", "d":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case "hour", "h":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location())
	case "minute", "m":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location())
	}
	return t
}

func endOf(t time.Time, unit string) time.Time {
	switch unit {
	case "year", "y":
		return time.Date(t.Year(), 12, 31, 23, 59, 59, 999999999, t.Location())
	case "month", "M":
		return time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location()).Add(-time.Nanosecond)
	case "week", "w":
		offset := 6 - int(t.Weekday())
		return time.Date(t.Year(), t.Month(), t.Day()+offset, 23, 59, 59, 999999999, t.Location())
	case "day", "d":
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
	case "hour", "h":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 59, 59, 999999999, t.Location())
	case "minute", "m":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 59, 999999999, t.Location())
	}
	return t
}

// jsToTime extracts a time.Time from a JS date object by calling toISOString() on it,
// or by parsing the value directly as a string.
func jsToTime(v goja.Value) time.Time {
	if obj, ok := v.(*goja.Object); ok {
		if isoFn := obj.Get("toISOString"); isoFn != nil {
			if fn, ok := goja.AssertFunction(isoFn); ok {
				if result, err := fn(v); err == nil {
					if t, err := time.Parse(time.RFC3339, result.String()); err == nil {
						return t
					}
				}
			}
		}
	}
	t, _ := time.Parse(time.RFC3339, v.String())
	return t
}

var parseDateFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"01/02/2006",
	"02-01-2006",
}

func dateToJS(vm *goja.Runtime, t time.Time) *goja.Object {
	obj := vm.NewObject()

	_ = obj.Set("format", func(call goja.FunctionCall) goja.Value {
		layout := time.RFC3339
		if len(call.Arguments) > 0 {
			layout = momentToGoLayout(call.Arguments[0].String())
		}
		return vm.ToValue(t.Format(layout))
	})
	_ = obj.Set("add", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return dateToJS(vm, t)
		}
		return dateToJS(vm, addDuration(t, call.Arguments[0].ToFloat(), call.Arguments[1].String()))
	})
	_ = obj.Set("subtract", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return dateToJS(vm, t)
		}
		return dateToJS(vm, addDuration(t, -call.Arguments[0].ToFloat(), call.Arguments[1].String()))
	})
	_ = obj.Set("isBefore", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		return vm.ToValue(t.Before(jsToTime(call.Arguments[0])))
	})
	_ = obj.Set("isAfter", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		return vm.ToValue(t.After(jsToTime(call.Arguments[0])))
	})
	_ = obj.Set("diff", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(0)
		}
		unit := "ms"
		if len(call.Arguments) > 1 {
			unit = call.Arguments[1].String()
		}
		return vm.ToValue(durationInUnit(t.Sub(jsToTime(call.Arguments[0])), unit))
	})
	_ = obj.Set("startOf", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return dateToJS(vm, t)
		}
		return dateToJS(vm, startOf(t, call.Arguments[0].String()))
	})
	_ = obj.Set("endOf", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return dateToJS(vm, t)
		}
		return dateToJS(vm, endOf(t, call.Arguments[0].String()))
	})
	_ = obj.Set("unix", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(t.Unix())
	})
	_ = obj.Set("unixMs", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(t.UnixMilli())
	})
	_ = obj.Set("toISOString", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(t.UTC().Format(time.RFC3339))
	})
	_ = obj.Set("toString", func(_ goja.FunctionCall) goja.Value {
		return vm.ToValue(t.String())
	})
	_ = obj.Set("year", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(t.Year()) })
	_ = obj.Set("month", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(int(t.Month())) })
	_ = obj.Set("day", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(t.Day()) })
	_ = obj.Set("weekday", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(int(t.Weekday())) })
	_ = obj.Set("hour", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(t.Hour()) })
	_ = obj.Set("minute", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(t.Minute()) })
	_ = obj.Set("second", func(_ goja.FunctionCall) goja.Value { return vm.ToValue(t.Second()) })

	return obj
}

func registerDate(vm *goja.Runtime) {
	dateObj := vm.NewObject()

	_ = dateObj.Set("now", func(_ goja.FunctionCall) goja.Value {
		return dateToJS(vm, time.Now())
	})
	_ = dateObj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.NewGoError(fmt.Errorf("date.parse() requires a string argument")))
		}
		s := call.Arguments[0].String()
		for _, layout := range parseDateFormats {
			if t, err := time.Parse(layout, s); err == nil {
				return dateToJS(vm, t)
			}
		}
		panic(vm.NewGoError(fmt.Errorf("date.parse(): cannot parse %q", s)))
	})
	_ = dateObj.Set("unix", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return dateToJS(vm, time.Now())
		}
		return dateToJS(vm, time.Unix(call.Arguments[0].ToInteger(), 0))
	})
	_ = dateObj.Set("unixMs", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return dateToJS(vm, time.Now())
		}
		ms := call.Arguments[0].ToInteger()
		return dateToJS(vm, time.UnixMilli(ms))
	})

	_ = vm.Set("date", dateObj)
}

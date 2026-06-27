package script

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

func decodeJWTPart(part string) (map[string]interface{}, error) {
	// JWT uses base64url without padding — add it back before decoding.
	switch len(part) % 4 {
	case 2:
		part += "=="
	case 3:
		part += "="
	}
	raw, err := base64.URLEncoding.DecodeString(part)
	if err != nil {
		return nil, fmt.Errorf("jwt: base64 decode failed: %w", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("jwt: JSON decode failed: %w", err)
	}
	return out, nil
}

func registerJWT(vm *goja.Runtime) {
	jwtObj := vm.NewObject()

	// jwt.decode(token) → { header, payload, signature }
	// header and payload are plain JS objects. Throws on malformed input.
	_ = jwtObj.Set("decode", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.NewGoError(fmt.Errorf("jwt.decode() requires a token argument")))
		}
		token := strings.TrimSpace(call.Arguments[0].String())
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			panic(vm.NewGoError(fmt.Errorf("jwt.decode(): invalid token — expected 3 parts, got %d", len(parts))))
		}

		header, err := decodeJWTPart(parts[0])
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("jwt.decode() header: %w", err)))
		}
		payload, err := decodeJWTPart(parts[1])
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("jwt.decode() payload: %w", err)))
		}

		obj := vm.NewObject()
		_ = obj.Set("header", vm.ToValue(header))
		_ = obj.Set("payload", vm.ToValue(payload))
		_ = obj.Set("signature", parts[2])
		return obj
	})

	// jwt.isExpired(token) → bool
	// Returns true when the payload's "exp" claim is in the past.
	// Returns false when exp is absent (can't determine expiry).
	_ = jwtObj.Set("isExpired", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return vm.ToValue(false)
		}
		token := strings.TrimSpace(call.Arguments[0].String())
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			return vm.ToValue(false)
		}
		payload, err := decodeJWTPart(parts[1])
		if err != nil {
			return vm.ToValue(false)
		}
		exp, ok := payload["exp"]
		if !ok {
			return vm.ToValue(false)
		}
		expF, ok := exp.(float64)
		if !ok {
			return vm.ToValue(false)
		}
		return vm.ToValue(time.Now().Unix() > int64(expF))
	})

	// jwt.expiresAt(token) → date object (or null when exp absent)
	_ = jwtObj.Set("expiresAt", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		token := strings.TrimSpace(call.Arguments[0].String())
		parts := strings.Split(token, ".")
		if len(parts) != 3 {
			return goja.Null()
		}
		payload, err := decodeJWTPart(parts[1])
		if err != nil {
			return goja.Null()
		}
		exp, ok := payload["exp"]
		if !ok {
			return goja.Null()
		}
		expF, ok := exp.(float64)
		if !ok {
			return goja.Null()
		}
		return dateToJS(vm, time.Unix(int64(expF), 0))
	})

	_ = vm.Set("jwt", jwtObj)
}

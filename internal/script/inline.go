package script

import (
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

var inlineExprRe = regexp.MustCompile(`\$\{\{([^}]+)\}\}`)

// InlineEvaluator replaces ${{expr}} tokens in strings by running each
// expression in a shared goja runtime with fake, date, jwt, xml, and env
// globals. One evaluator per HTTP request keeps the runtime allocation cheap.
type InlineEvaluator struct {
	vm *goja.Runtime
}

// NewInlineEvaluator creates a runtime pre-loaded with the util libs and an
// env object populated from vars. Returns nil when vars contains no ${{
// expressions (caller may check with HasInlineExprs before allocating).
func NewInlineEvaluator(vars map[string]string) *InlineEvaluator {
	vm := goja.New()
	registerUtilLibs(vm)
	envObj := vm.NewObject()
	for k, v := range vars {
		_ = envObj.Set(k, v)
	}
	vm.Set("env", envObj)
	return &InlineEvaluator{vm: vm}
}

// HasInlineExprs reports whether s contains any ${{ token.
func HasInlineExprs(s string) bool {
	return strings.Contains(s, "${{")
}

// Eval replaces every ${{expr}} in s with the string result of evaluating
// expr. Tokens that fail to evaluate are left unchanged.
func (e *InlineEvaluator) Eval(s string) string {
	if !strings.Contains(s, "${{") {
		return s
	}
	return inlineExprRe.ReplaceAllStringFunc(s, func(match string) string {
		expr := strings.TrimSpace(match[3 : len(match)-2]) // strip "${{" and "}}"
		val, err := e.vm.RunString(expr)
		if err != nil {
			return match // leave token unchanged on error
		}
		return jsValueToString(e.vm, val)
	})
}

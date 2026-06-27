package script

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

type xmlNode struct {
	Name     string
	Attrs    map[string]string
	Text     string
	Children []*xmlNode
}

func parseXML(src string) (*xmlNode, error) {
	dec := xml.NewDecoder(strings.NewReader(src))
	var stack []*xmlNode
	var root *xmlNode

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			node := &xmlNode{
				Name:  t.Name.Local,
				Attrs: make(map[string]string, len(t.Attr)),
			}
			for _, a := range t.Attr {
				node.Attrs[a.Name.Local] = a.Value
			}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.Children = append(parent.Children, node)
			}
			stack = append(stack, node)

		case xml.EndElement:
			if len(stack) == 0 {
				break
			}
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			node.Text = strings.TrimSpace(node.Text)
			if len(stack) == 0 {
				root = node
			}

		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].Text += string(t)
			}
		}
	}

	if root == nil {
		return nil, fmt.Errorf("xml.parse: no root element found")
	}
	return root, nil
}

func xmlNodeToJS(vm *goja.Runtime, node *xmlNode) *goja.Object {
	obj := vm.NewObject()
	_ = obj.Set("name", node.Name)

	attrsObj := vm.NewObject()
	for k, v := range node.Attrs {
		_ = attrsObj.Set(k, v)
	}
	_ = obj.Set("attrs", attrsObj)
	_ = obj.Set("text", node.Text)

	children := make([]interface{}, len(node.Children))
	for i, c := range node.Children {
		children[i] = xmlNodeToJS(vm, c)
	}
	_ = obj.Set("children", vm.ToValue(children))

	// find(name) → first child whose name matches
	_ = obj.Set("find", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			return goja.Null()
		}
		name := call.Arguments[0].String()
		for _, c := range node.Children {
			if c.Name == name {
				return xmlNodeToJS(vm, c)
			}
		}
		return goja.Null()
	})

	// findAll(name) → all children whose name matches
	_ = obj.Set("findAll", func(call goja.FunctionCall) goja.Value {
		name := ""
		if len(call.Arguments) > 0 {
			name = call.Arguments[0].String()
		}
		var found []interface{}
		for _, c := range node.Children {
			if name == "" || c.Name == name {
				found = append(found, xmlNodeToJS(vm, c))
			}
		}
		if found == nil {
			found = []interface{}{}
		}
		return vm.ToValue(found)
	})

	return obj
}

func registerXML(vm *goja.Runtime) {
	xmlObj := vm.NewObject()
	_ = xmlObj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(vm.NewGoError(fmt.Errorf("xml.parse() requires a string argument")))
		}
		node, err := parseXML(call.Arguments[0].String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return xmlNodeToJS(vm, node)
	})
	_ = vm.Set("xml", xmlObj)
}

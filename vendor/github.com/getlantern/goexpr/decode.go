package goexpr

import (
	"bytes"
)

// Decode compares the value of the first expression to a series of key/value pairs
// and returns the matching value. It optionally takes a final default value to use
// if none of the keys match.
func Decode(exprs ...Expr) Expr {
	if len(exprs) < 2 {
		return &noop{}
	}
	source := exprs[0]
	kvs := exprs[1:]
	d := &decode{Source: source}
	for i := 0; i < len(kvs); i += 2 {
		k := kvs[i]
		if i+1 < len(kvs) {
			v := kvs[i+1]
			d.Keys = append(d.Keys, k)
			d.Values = append(d.Values, v)
		} else {
			d.Default = k
		}
	}
	return d
}

type decode struct {
	Source  Expr
	Keys    []Expr
	Values  []Expr
	Default Expr
}

func (e *decode) Eval(params Params) interface{} {
	source := e.Source.Eval(params)
	for i, k := range e.Keys {
		key := k.Eval(params)
		if source == key {
			return e.Values[i].Eval(params)
		}
	}
	if e.Default != nil {
		return e.Default.Eval(params)
	}
	return nil
}

func (e *decode) WalkParams(cb func(string)) {
	e.Source.WalkParams(cb)
	for _, k := range e.Keys {
		k.WalkParams(cb)
	}
	for _, v := range e.Values {
		v.WalkParams(cb)
	}
	if e.Default != nil {
		e.Default.WalkParams(cb)
	}
}

func (e *decode) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *decode) WalkLists(cb func(List)) {
	e.Source.WalkLists(cb)
	for _, k := range e.Keys {
		k.WalkLists(cb)
	}
	for _, v := range e.Values {
		v.WalkLists(cb)
	}
	if e.Default != nil {
		e.Default.WalkLists(cb)
	}
}

func (e *decode) String() string {
	buf := &bytes.Buffer{}
	buf.WriteString("DECODE(")
	buf.WriteString(e.Source.String())
	for i, k := range e.Keys {
		v := e.Values[i]
		buf.WriteString(", ")
		buf.WriteString(k.String())
		buf.WriteString(", ")
		buf.WriteString(v.String())
	}
	if e.Default != nil {
		buf.WriteString(", ")
		buf.WriteString(e.Default.String())
	}
	buf.WriteString(")")
	return buf.String()
}

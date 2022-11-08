package goexpr

import (
	"bytes"
	"fmt"
)

// Concat joins a list of values using the first as a delimiter.
func Concat(exprs ...Expr) Expr {
	return &concat{exprs[0], exprs[1:]}
}

type concat struct {
	Delim   Expr
	Wrapped []Expr
}

func (e *concat) Eval(params Params) interface{} {
	delim := e.Delim.Eval(params)
	buf := &bytes.Buffer{}
	for i, wrapped := range e.Wrapped {
		first := i == 0
		if !first {
			fmt.Fprint(buf, delim)
		}
		val := wrapped.Eval(params)
		if val == nil {
			// replace nil with empty string
			val = ""
		}
		fmt.Fprint(buf, val)
	}
	return buf.String()
}

func (e *concat) WalkParams(cb func(string)) {
	e.Delim.WalkParams(cb)
	for _, wrapped := range e.Wrapped {
		wrapped.WalkParams(cb)
	}
}

func (e *concat) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *concat) WalkLists(cb func(List)) {
	e.Delim.WalkLists(cb)
	for _, wrapped := range e.Wrapped {
		wrapped.WalkLists(cb)
	}
}

func (e *concat) String() string {
	buf := &bytes.Buffer{}
	buf.WriteString("CONCAT(")
	buf.WriteString(e.Delim.String())
	for _, wrapped := range e.Wrapped {
		buf.WriteString(", ")
		fmt.Fprint(buf, wrapped.String())
	}
	buf.WriteString(")")
	return buf.String()
}

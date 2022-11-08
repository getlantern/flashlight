package goexpr

import (
	"bytes"
)

// Any is an expression that returns the first non-nil, non-empty string value
// from evaluating the exprs.
func Any(exprs ...Expr) Expr {
	return &any{exprs}
}

type any struct {
	Exprs []Expr
}

func (e *any) Eval(params Params) interface{} {
	for _, expr := range e.Exprs {
		v := expr.Eval(params)
		if v != nil && v != "" {
			return v
		}
	}
	return nil
}

func (e *any) WalkParams(cb func(string)) {
	for _, wrapped := range e.Exprs {
		wrapped.WalkParams(cb)
	}
}

func (e *any) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *any) WalkLists(cb func(List)) {
	for _, e := range e.Exprs {
		e.WalkLists(cb)
	}
}

func (e *any) String() string {
	result := &bytes.Buffer{}
	result.WriteString("ANY(")
	first := true
	for _, expr := range e.Exprs {
		if !first {
			result.WriteByte(',')
		}
		first = false
		result.WriteString(expr.String())
	}
	result.WriteByte(')')
	return result.String()
}

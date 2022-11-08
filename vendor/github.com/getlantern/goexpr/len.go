package goexpr

import (
	"fmt"
)

// Len calculates the length of the string representation of the given value.
// If value is nil, returns nil.
func Len(source Expr) Expr {
	return &length{source}
}

type length struct {
	Source Expr
}

func (e *length) Eval(params Params) interface{} {
	source := e.Source.Eval(params)
	if source == nil {
		return nil
	}
	switch t := source.(type) {
	case string:
		return len(t)
	default:
		return len(fmt.Sprint(t))
	}
}

func (e *length) WalkParams(cb func(string)) {
	e.Source.WalkParams(cb)
}

func (e *length) WalkOneToOneParams(cb func(string)) {
	e.Source.WalkOneToOneParams(cb)
}

func (e *length) WalkLists(cb func(List)) {
	e.Source.WalkLists(cb)
}

func (e *length) String() string {
	return fmt.Sprintf("LEN(%v)", e.Source.String())
}

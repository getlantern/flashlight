package goexpr

import (
	"fmt"
)

// Substr takes a substring of the given source starting at the given index
// capped to the given length.
func Substr(source Expr, from Expr, length Expr) Expr {
	return &substr{source, from, length}
}

type substr struct {
	Source Expr
	From   Expr
	Length Expr
}

func (e *substr) Eval(params Params) interface{} {
	source := e.Source.Eval(params)
	if source == nil {
		return nil
	}
	result := source.(string)
	from := e.From.Eval(params).(int)
	if from >= len(result) {
		return nil
	}
	result = result[from:]
	length := e.Length.Eval(params).(int)
	if length > 0 && length < len(result) {
		result = result[:length]
	}
	return result
}

func (e *substr) WalkParams(cb func(string)) {
	e.Source.WalkParams(cb)
	e.From.WalkParams(cb)
	e.Length.WalkParams(cb)
}

func (e *substr) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *substr) WalkLists(cb func(List)) {
	e.Source.WalkLists(cb)
	e.From.WalkLists(cb)
	e.Length.WalkLists(cb)
}

func (e *substr) String() string {
	return fmt.Sprintf("substr(%v,%v,%v)", e.Source.String(), e.From.String(), e.Length.String())
}

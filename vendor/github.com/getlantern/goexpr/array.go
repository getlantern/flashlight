package goexpr

import (
	"strings"
)

// ArrayExpr is just a placeholder expression for grouping some other
// expressions together.
type ArrayExpr struct {
	Items []Expr
}

// Array creates a new ArrayExpr
func Array(items ...Expr) Expr {
	return &ArrayExpr{items}
}

func (e *ArrayExpr) Eval(params Params) interface{} {
	return nil
}

func (e *ArrayExpr) WalkParams(cb func(string)) {
	for _, item := range e.Items {
		item.WalkParams(cb)
	}
}

func (e *ArrayExpr) WalkOneToOneParams(cb func(string)) {
	for _, item := range e.Items {
		item.WalkOneToOneParams(cb)
	}
}

func (e *ArrayExpr) WalkLists(cb func(List)) {
	for _, item := range e.Items {
		item.WalkLists(cb)
	}
}

func (e *ArrayExpr) String() string {
	parts := make([]string, 0, len(e.Items))
	for _, item := range e.Items {
		parts = append(parts, item.String())
	}
	return strings.Join(parts, ",")
}

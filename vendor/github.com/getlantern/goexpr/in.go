package goexpr

import (
	"bytes"
	"fmt"
)

type List interface {
	Values() []Expr
}

type ArrayList []Expr

func (al ArrayList) Values() []Expr {
	return al
}

func (al ArrayList) String() string {
	buf := &bytes.Buffer{}
	for i, e := range al {
		if i > 0 {
			buf.WriteString(", ")
		}
		fmt.Fprint(buf, e.String())
	}
	return buf.String()
}

func In(val Expr, candidates List) Expr {
	return &in{Val: val, Candidates: candidates}
}

type in struct {
	Val         Expr
	Candidates  List
	initialized bool
	candidates  []Expr
}

func (e *in) Eval(params Params) interface{} {
	v := e.Val.Eval(params)
	if !e.initialized {
		e.candidates = e.Candidates.Values()
		e.initialized = true
	}
	for _, candidate := range e.candidates {
		c := candidate.Eval(params)
		if doEq(c, v) {
			return true
		}
	}
	return false
}

func (e *in) WalkParams(cb func(string)) {
	e.Val.WalkParams(cb)
}

func (e *in) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *in) WalkLists(cb func(List)) {
	cb(e.Candidates)
	e.Val.WalkLists(cb)
}

func (e *in) String() string {
	return fmt.Sprintf("%v IN(%v)", e.Val.String(), e.Candidates)
}

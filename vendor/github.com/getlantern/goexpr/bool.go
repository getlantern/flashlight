package goexpr

import (
	"fmt"

	"github.com/getlantern/msgpack"
)

// Boolean accepts the operators AND, OR and returns a short-circuiting
// expression that evaluates left first and right second.
func Boolean(operator string, left Expr, right Expr) (Expr, error) {
	var bfn boolFN
	switch operator {
	case "AND":
		bfn = and
	case "OR":
		bfn = or
	default:
		return nil, fmt.Errorf("Unknown boolean operator %v", operator)
	}
	return &booleanExpr{operator, bfn, left, right}, nil
}

type boolFN func(a Expr, b Expr, params Params) bool

func and(a Expr, b Expr, params Params) bool {
	aVal, aok := a.Eval(params).(bool)
	if !aok || !aVal {
		return false
	}
	bVal, bok := b.Eval(params).(bool)
	return bok && bVal
}

func or(a Expr, b Expr, params Params) bool {
	aVal, aok := a.Eval(params).(bool)
	if aok && aVal {
		return true
	}
	bVal, bok := b.Eval(params).(bool)
	return (aok && aVal) || (bok && bVal)
}

type booleanExpr struct {
	OperatorString string
	operator       boolFN
	Left           Expr
	Right          Expr
}

func (e *booleanExpr) Eval(params Params) interface{} {
	return e.operator(e.Left, e.Right, params)
}

func (e *booleanExpr) WalkParams(cb func(string)) {
	e.Left.WalkParams(cb)
	e.Right.WalkParams(cb)
}

func (e *booleanExpr) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *booleanExpr) WalkLists(cb func(List)) {
	e.Left.WalkLists(cb)
	e.Right.WalkLists(cb)
}

func (e *booleanExpr) String() string {
	return fmt.Sprintf("(%v %v %v)", e.Left, e.OperatorString, e.Right)
}

func (e *booleanExpr) DecodeMsgpack(dec *msgpack.Decoder) error {
	m := make(map[string]interface{})
	err := dec.Decode(&m)
	if err != nil {
		return err
	}
	_e2, err := Boolean(m["OperatorString"].(string), m["Left"].(Expr), m["Right"].(Expr))
	if err != nil {
		return err
	}
	if _e2 == nil {
		return fmt.Errorf("Unknown boolean expression %v", m["OperatorString"])
	}
	e2 := _e2.(*booleanExpr)
	e.OperatorString = e2.OperatorString
	e.operator = e2.operator
	e.Left = e2.Left
	e.Right = e2.Right
	return nil
}

package goexpr

// noop is an expression that does nothing (always returns nil)
type noop struct{}

func (e *noop) Eval(params Params) interface{} {
	return nil
}

func (e *noop) WalkParams(cb func(string)) {
}

func (e *noop) WalkOneToOneParams(cb func(string)) {
}

func (e *noop) WalkLists(cb func(List)) {
}

func (e *noop) String() string {
	return "NOOP"
}

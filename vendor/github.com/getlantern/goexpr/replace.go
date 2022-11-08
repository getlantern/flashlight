package goexpr

import (
	"fmt"
	"regexp"

	"github.com/getlantern/msgpack"
)

// ReplaceAll replaces all occurrences of the regex with the replacement.
func ReplaceAll(source Expr, regex Expr, replacement Expr) Expr {
	regexString := ""
	_regex, ok := regex.(*constant)
	if ok {
		regexString = fmt.Sprint(_regex.Eval(nil))
	} else {
		fmt.Println("Regex is not a constant!")
		regexString = ""
	}
	e := &replaceAll{Source: source, Regex: regexString, Replacement: replacement}
	e.initReplacer()
	return e
}

func (e *replaceAll) initReplacer() {
	if e.Regex == "" {
		e.replacer = func(in string, replacement string) string { return in }
		return
	}
	re, err := regexp.Compile(e.Regex)
	if err != nil {
		fmt.Printf("Error compiling regex, using noop %v: %v\n", e.Regex, err)
		e.replacer = func(in string, replacement string) string { return in }
		return
	}
	e.replacer = re.ReplaceAllString
}

type replaceAll struct {
	Source      Expr
	Regex       string
	Replacement Expr
	replacer    func(string, string) string
}

func (e *replaceAll) Eval(params Params) interface{} {
	source := e.Source.Eval(params)
	if source == nil {
		return nil
	}
	replacement := e.Replacement.Eval(params)
	replacementString := ""
	if replacement != nil {
		replacementString = fmt.Sprint(replacement)
	}
	return e.replacer(fmt.Sprint(source), replacementString)
}

func (e *replaceAll) WalkParams(cb func(string)) {
	e.Source.WalkParams(cb)
	e.Replacement.WalkParams(cb)
}

func (e *replaceAll) WalkOneToOneParams(cb func(string)) {
	// this function is not one-to-one, stop
}

func (e *replaceAll) WalkLists(cb func(List)) {
	e.Source.WalkLists(cb)
	e.Replacement.WalkLists(cb)
}

func (e *replaceAll) String() string {
	return fmt.Sprintf("REPLACEALL(%v, %v, %v)", e.Source.String(), e.Regex, e.Replacement.String())
}

func (e *replaceAll) DecodeMsgpack(dec *msgpack.Decoder) error {
	m := make(map[string]interface{})
	err := dec.Decode(&m)
	if err != nil {
		return err
	}
	e2 := ReplaceAll(m["Source"].(Expr), Constant(m["Regex"].(string)), m["Replacement"].(Expr)).(*replaceAll)
	e.Source = e2.Source
	e.Regex = e2.Regex
	e.Replacement = e2.Replacement
	e.initReplacer()
	return nil
}
